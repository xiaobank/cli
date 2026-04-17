package claudecode

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

func TestGenerateTextStreaming_SuccessEmitsPhases(t *testing.T) {
	t.Parallel()
	body := strings.Join([]string{
		`{"type":"system","subtype":"status","status":"requesting"}`,
		`{"type":"stream_event","event":{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":2000}}},"ttft_ms":1500}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello world"}}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"hello world","duration_ms":1700}`,
	}, "\n")
	ag := newAgentWithStdout(body)
	var phases []agent.ProgressPhase
	got, err := ag.GenerateTextStreaming(context.Background(), "p", "", func(p agent.GenerationProgress) {
		phases = append(phases, p.Phase)
	})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if got != "hello world" {
		t.Fatalf("got = %q; want %q", got, "hello world")
	}
	want := []agent.ProgressPhase{agent.PhaseConnecting, agent.PhaseFirstToken, agent.PhaseGenerating, agent.PhaseDone}
	if !slicesEqual(phases, want) {
		t.Fatalf("phases = %v; want %v", phases, want)
	}
}

func TestGenerateTextStreaming_EnvelopeErrorReturnsError(t *testing.T) {
	t.Parallel()
	body := `{"type":"result","subtype":"success","is_error":true,"api_error_status":401,"result":"Auth required"}`
	ag := newAgentWithStdout(body)
	_, err := ag.GenerateTextStreaming(context.Background(), "p", "", nil)
	if err == nil {
		t.Fatal("err = nil; want non-nil for envelope error")
	}
	if !strings.Contains(err.Error(), "Auth required") {
		t.Errorf("err = %v; want error mentioning result text", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v; want error mentioning HTTP status", err)
	}
}

func TestGenerateTextStreaming_FallsBackToLegacy(t *testing.T) {
	t.Parallel()
	// Simulate an older CLI that rejects stream-json, then succeeds on legacy json.
	callCount := 0
	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
			callCount++
			for _, a := range args {
				if a == "stream-json" {
					return exec.CommandContext(ctx, "sh", "-c",
						"printf 'error: unknown option stream-json' 1>&2; exit 1")
				}
			}
			return exec.CommandContext(ctx, "sh", "-c",
				`printf '{"type":"result","result":"legacy result"}'`)
		},
	}
	got, err := ag.GenerateTextStreaming(context.Background(), "p", "haiku", nil)
	if err != nil {
		t.Fatalf("err = %v; want nil (legacy fallback should succeed)", err)
	}
	if got != "legacy result" {
		t.Fatalf("got = %q; want %q", got, "legacy result")
	}
	if callCount < 2 {
		t.Errorf("callCount = %d; want >= 2 (streaming + legacy fallback)", callCount)
	}
}

func TestGenerateTextStreaming_ContextCanceledPassesThrough(t *testing.T) {
	t.Parallel()
	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "sleep 5")
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ag.GenerateTextStreaming(ctx, "p", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v; want context.Canceled", err)
	}
}

func TestGenerateTextStreaming_ContextDeadlineExceededPassesThrough(t *testing.T) {
	t.Parallel()
	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "sleep 5")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := ag.GenerateTextStreaming(ctx, "p", "", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v; want context.DeadlineExceeded", err)
	}
}

func TestGenerateTextStreaming_EnvelopeErrorBeatsCtxCancel(t *testing.T) {
	t.Parallel()
	// A specific envelope error should be preserved even if ctx was canceled
	// after the result event arrived — otherwise users see generic "canceled"
	// instead of the specific failure (auth / rate-limit / HTTP 401 / etc.).
	body := `{"type":"result","subtype":"success","is_error":true,"api_error_status":401,"result":"Auth required"}`
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // parent ctx is dead before the call
	ag := &ClaudeCodeAgent{
		CommandRunner: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			// Detach the subprocess from the canceled parent ctx so cmd.Start
			// and the scan-to-EOF complete, letting us exercise the
			// "envelope-present but ctx.Err() != nil" branch.
			return exec.CommandContext(context.Background(), "sh", "-c",
				"cat <<'ENDOFSTREAM'\n"+body+"\nENDOFSTREAM")
		},
	}
	_, err := ag.GenerateTextStreaming(ctx, "p", "", nil)
	if err == nil {
		t.Fatal("err = nil; want envelope error surfaced despite cancellation")
	}
	if !strings.Contains(err.Error(), "Auth required") {
		t.Errorf("err = %q; want envelope result text preserved over cancellation", err)
	}
}

func TestLooksLikeUnrecognizedFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"stream-json unknown", "error: unknown option 'stream-json'", true},
		{"verbose unrecognized", "unrecognized option: --verbose", true},
		{"include-partial invalid", "invalid option: --include-partial-messages", true},
		{"unrelated unknown option", "error: unknown option 'foobar'", false},
		{"auth error", "Invalid API key", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeUnrecognizedFlag(tc.stderr); got != tc.want {
				t.Errorf("looksLikeUnrecognizedFlag(%q) = %v; want %v", tc.stderr, got, tc.want)
			}
		})
	}
}

// newAgentWithStdout returns a ClaudeCodeAgent whose CommandRunner produces a
// subprocess that prints the given body to stdout and exits 0.
func newAgentWithStdout(body string) *ClaudeCodeAgent {
	return &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "cat <<'ENDOFSTREAM'\n"+body+"\nENDOFSTREAM")
		},
	}
}

func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
