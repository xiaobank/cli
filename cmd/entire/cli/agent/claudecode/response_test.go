package claudecode

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseGenerateTextResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stdout  string
		want    string
		wantErr string
	}{
		{
			name:   "legacy object result",
			stdout: `{"result":"hello"}`,
			want:   "hello",
		},
		{
			name:   "legacy object empty result",
			stdout: `{"result":""}`,
			want:   "",
		},
		{
			name:   "array result",
			stdout: `[{"type":"system"},{"type":"result","result":"hello"}]`,
			want:   "hello",
		},
		{
			name:   "array empty result",
			stdout: `[{"type":"system"},{"type":"result","result":""}]`,
			want:   "",
		},
		{
			name:    "missing result item",
			stdout:  `[{"type":"system"},{"type":"assistant","message":"working"}]`,
			wantErr: "missing result item",
		},
		{
			name:    "invalid json",
			stdout:  `not json`,
			wantErr: "unsupported Claude CLI JSON response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseGenerateTextResponse([]byte(tt.stdout))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseGenerateTextResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStreamClaudeResponse_SuccessFixture(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/stream_success.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var phases []string
	final, _, err := streamClaudeResponse(bytes.NewReader(raw), func(ev streamEvent) {
		phases = append(phases, ev.Type+"/"+ev.Subtype+"/"+ev.Status)
	})
	if err != nil {
		t.Fatalf("streamClaudeResponse error = %v; want nil", err)
	}
	if final == nil {
		t.Fatal("final event = nil; want result envelope")
	}
	const wantType = "result"
	if final.Type != wantType {
		t.Errorf("final.Type = %q; want %q", final.Type, wantType)
	}
	if final.IsError {
		t.Errorf("final.IsError = true; want false for success fixture")
	}
	if final.Result == nil || *final.Result == "" {
		t.Error("final.Result = nil/empty; want non-empty result string")
	}
	if len(phases) < 3 {
		t.Errorf("observed %d events; want >= 3 phases", len(phases))
	}
}

func TestStreamClaudeResponse_ErrorFixture(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/stream_error_404.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	final, _, err := streamClaudeResponse(bytes.NewReader(raw), nil)
	if err != nil {
		t.Fatalf("streamClaudeResponse error = %v; want nil (parsing succeeded)", err)
	}
	if final == nil {
		t.Fatal("final event = nil; want result envelope with is_error:true")
	}
	if !final.IsError {
		t.Error("final.IsError = false; want true for invalid-model fixture")
	}
	if final.APIErrorStatus == nil || *final.APIErrorStatus != 404 {
		t.Errorf("APIErrorStatus = %v; want *404", final.APIErrorStatus)
	}
}

func TestStreamClaudeResponse_SkipsMalformedLine(t *testing.T) {
	t.Parallel()
	body := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`not valid json at all`,
		`{"type":"result","subtype":"success","is_error":false,"result":"ok"}`,
	}, "\n")
	final, malformed, err := streamClaudeResponse(strings.NewReader(body), nil)
	if err != nil {
		t.Fatalf("err = %v; want nil (malformed lines skipped)", err)
	}
	if final == nil || final.Result == nil || *final.Result != "ok" {
		t.Errorf("final.Result = %v; want \"ok\"", final.Result)
	}
	if malformed != 1 {
		t.Errorf("malformed = %d; want 1 (one invalid line)", malformed)
	}
}

func TestStreamClaudeResponse_NoFinalEvent(t *testing.T) {
	t.Parallel()
	body := `{"type":"system","subtype":"init"}`
	final, _, err := streamClaudeResponse(strings.NewReader(body), nil)
	if final != nil {
		t.Errorf("final = %v; want nil when stream has no result event", final)
	}
	if err == nil {
		t.Error("err = nil; want non-nil for stream with no result")
	}
}

func TestStreamClaudeResponse_ReaderError(t *testing.T) {
	t.Parallel()
	r := errReader{err: errors.New("boom")}
	if _, _, err := streamClaudeResponse(r, nil); err == nil {
		t.Error("err = nil; want propagated reader error")
	}
}

type errReader struct{ err error }

func (e errReader) Read(_ []byte) (int, error) { return 0, e.err }
