package external

import (
	"context"
	"io"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
)

// Wrap returns the Agent wrapped with the appropriate interface
// implementations based on the capabilities declared in its info response.
// The returned value satisfies agent.Agent and whichever optional interfaces
// the external binary declared support for.
//
// We cannot use Go embedding because *Agent implements ALL optional interface
// methods, and embedding would expose all of them regardless of capabilities.
// Instead, each wrapper explicitly forwards only the interfaces it should expose.
func Wrap(ea *Agent) agent.Agent {
	caps := ea.info.Capabilities

	base := baseAgent{ea: ea}

	switch {
	case caps.Hooks && caps.TranscriptAnalyzer && caps.TokenCalculator && caps.TextGenerator && caps.HookResponseWriter && caps.SubagentAwareExtractor:
		return &fullAgent{baseAgent: base}
	case caps.Hooks && caps.TranscriptAnalyzer:
		return &hooksAnalyzerAgent{baseAgent: base}
	case caps.Hooks:
		return &hooksAgent{baseAgent: base}
	case caps.TranscriptAnalyzer:
		return &analyzerAgent{baseAgent: base}
	default:
		return &base
	}
}

// --- baseAgent: only agent.Agent methods ---

type baseAgent struct{ ea *Agent }

func (b *baseAgent) Name() types.AgentName { return b.ea.Name() }
func (b *baseAgent) Type() types.AgentType { return b.ea.Type() }
func (b *baseAgent) Description() string   { return b.ea.Description() }
func (b *baseAgent) IsPreview() bool       { return b.ea.IsPreview() }
func (b *baseAgent) DetectPresence(ctx context.Context) (bool, error) {
	return b.ea.DetectPresence(ctx)
}
func (b *baseAgent) ProtectedDirs() []string                   { return b.ea.ProtectedDirs() }
func (b *baseAgent) ReadTranscript(ref string) ([]byte, error) { return b.ea.ReadTranscript(ref) }
func (b *baseAgent) ChunkTranscript(ctx context.Context, c []byte, m int) ([][]byte, error) {
	return b.ea.ChunkTranscript(ctx, c, m)
}
func (b *baseAgent) ReassembleTranscript(chunks [][]byte) ([]byte, error) {
	return b.ea.ReassembleTranscript(chunks)
}
func (b *baseAgent) GetSessionID(input *agent.HookInput) string { return b.ea.GetSessionID(input) }
func (b *baseAgent) GetSessionDir(repoPath string) (string, error) {
	return b.ea.GetSessionDir(repoPath)
}
func (b *baseAgent) ResolveSessionFile(dir, id string) string {
	return b.ea.ResolveSessionFile(dir, id)
}
func (b *baseAgent) ReadSession(input *agent.HookInput) (*agent.AgentSession, error) {
	return b.ea.ReadSession(input)
}
func (b *baseAgent) WriteSession(ctx context.Context, s *agent.AgentSession) error {
	return b.ea.WriteSession(ctx, s)
}
func (b *baseAgent) FormatResumeCommand(id string) string { return b.ea.FormatResumeCommand(id) }

// --- hooksAgent: agent.Agent + HookSupport ---

type hooksAgent struct{ baseAgent }

func (h *hooksAgent) HookNames() []string { return h.ea.HookNames() }
func (h *hooksAgent) ParseHookEvent(ctx context.Context, name string, stdin io.Reader) (*agent.Event, error) {
	return h.ea.ParseHookEvent(ctx, name, stdin)
}
func (h *hooksAgent) InstallHooks(ctx context.Context, localDev bool, force bool) (int, error) {
	return h.ea.InstallHooks(ctx, localDev, force)
}
func (h *hooksAgent) UninstallHooks(ctx context.Context) error   { return h.ea.UninstallHooks(ctx) }
func (h *hooksAgent) AreHooksInstalled(ctx context.Context) bool { return h.ea.AreHooksInstalled(ctx) }

var _ agent.HookSupport = (*hooksAgent)(nil)

// --- analyzerAgent: agent.Agent + TranscriptAnalyzer ---

type analyzerAgent struct{ baseAgent }

func (a *analyzerAgent) GetTranscriptPosition(path string) (int, error) {
	return a.ea.GetTranscriptPosition(path)
}
func (a *analyzerAgent) ExtractModifiedFilesFromOffset(path string, offset int) ([]string, int, error) {
	return a.ea.ExtractModifiedFilesFromOffset(path, offset)
}
func (a *analyzerAgent) ExtractPrompts(ref string, offset int) ([]string, error) {
	return a.ea.ExtractPrompts(ref, offset)
}
func (a *analyzerAgent) ExtractSummary(ref string) (string, error) {
	return a.ea.ExtractSummary(ref)
}

var _ agent.TranscriptAnalyzer = (*analyzerAgent)(nil)

// --- hooksAnalyzerAgent: agent.Agent + HookSupport + TranscriptAnalyzer ---

type hooksAnalyzerAgent struct{ baseAgent }

func (ha *hooksAnalyzerAgent) HookNames() []string { return ha.ea.HookNames() }
func (ha *hooksAnalyzerAgent) ParseHookEvent(ctx context.Context, name string, stdin io.Reader) (*agent.Event, error) {
	return ha.ea.ParseHookEvent(ctx, name, stdin)
}
func (ha *hooksAnalyzerAgent) InstallHooks(ctx context.Context, localDev bool, force bool) (int, error) {
	return ha.ea.InstallHooks(ctx, localDev, force)
}
func (ha *hooksAnalyzerAgent) UninstallHooks(ctx context.Context) error {
	return ha.ea.UninstallHooks(ctx)
}
func (ha *hooksAnalyzerAgent) AreHooksInstalled(ctx context.Context) bool {
	return ha.ea.AreHooksInstalled(ctx)
}
func (ha *hooksAnalyzerAgent) GetTranscriptPosition(path string) (int, error) {
	return ha.ea.GetTranscriptPosition(path)
}
func (ha *hooksAnalyzerAgent) ExtractModifiedFilesFromOffset(path string, offset int) ([]string, int, error) {
	return ha.ea.ExtractModifiedFilesFromOffset(path, offset)
}
func (ha *hooksAnalyzerAgent) ExtractPrompts(ref string, offset int) ([]string, error) {
	return ha.ea.ExtractPrompts(ref, offset)
}
func (ha *hooksAnalyzerAgent) ExtractSummary(ref string) (string, error) {
	return ha.ea.ExtractSummary(ref)
}

var (
	_ agent.HookSupport        = (*hooksAnalyzerAgent)(nil)
	_ agent.TranscriptAnalyzer = (*hooksAnalyzerAgent)(nil)
)

// --- fullAgent: all optional interfaces ---

type fullAgent struct{ baseAgent }

// HookSupport
func (f *fullAgent) HookNames() []string { return f.ea.HookNames() }
func (f *fullAgent) ParseHookEvent(ctx context.Context, name string, stdin io.Reader) (*agent.Event, error) {
	return f.ea.ParseHookEvent(ctx, name, stdin)
}
func (f *fullAgent) InstallHooks(ctx context.Context, localDev bool, force bool) (int, error) {
	return f.ea.InstallHooks(ctx, localDev, force)
}
func (f *fullAgent) UninstallHooks(ctx context.Context) error   { return f.ea.UninstallHooks(ctx) }
func (f *fullAgent) AreHooksInstalled(ctx context.Context) bool { return f.ea.AreHooksInstalled(ctx) }

// TranscriptAnalyzer
func (f *fullAgent) GetTranscriptPosition(path string) (int, error) {
	return f.ea.GetTranscriptPosition(path)
}
func (f *fullAgent) ExtractModifiedFilesFromOffset(path string, offset int) ([]string, int, error) {
	return f.ea.ExtractModifiedFilesFromOffset(path, offset)
}
func (f *fullAgent) ExtractPrompts(ref string, offset int) ([]string, error) {
	return f.ea.ExtractPrompts(ref, offset)
}
func (f *fullAgent) ExtractSummary(ref string) (string, error) {
	return f.ea.ExtractSummary(ref)
}

// TokenCalculator
func (f *fullAgent) CalculateTokenUsage(data []byte, offset int) (*agent.TokenUsage, error) {
	return f.ea.CalculateTokenUsage(data, offset)
}

// TextGenerator
func (f *fullAgent) GenerateText(ctx context.Context, prompt string, model string) (string, error) {
	return f.ea.GenerateText(ctx, prompt, model)
}

// HookResponseWriter
func (f *fullAgent) WriteHookResponse(message string) error {
	return f.ea.WriteHookResponse(message)
}

// SubagentAwareExtractor
func (f *fullAgent) ExtractAllModifiedFiles(data []byte, offset int, dir string) ([]string, error) {
	return f.ea.ExtractAllModifiedFiles(data, offset, dir)
}
func (f *fullAgent) CalculateTotalTokenUsage(data []byte, offset int, dir string) (*agent.TokenUsage, error) {
	return f.ea.CalculateTotalTokenUsage(data, offset, dir)
}

var (
	_ agent.HookSupport            = (*fullAgent)(nil)
	_ agent.TranscriptAnalyzer     = (*fullAgent)(nil)
	_ agent.TokenCalculator        = (*fullAgent)(nil)
	_ agent.TextGenerator          = (*fullAgent)(nil)
	_ agent.HookResponseWriter     = (*fullAgent)(nil)
	_ agent.SubagentAwareExtractor = (*fullAgent)(nil)
)
