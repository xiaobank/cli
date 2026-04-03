package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectAuthMethod_TokenOverride(t *testing.T) {
	t.Setenv("ENTIRE_CHECKPOINT_TOKEN", "ghp_abc123")
	method := detectAuthMethod(context.Background(), fetchProtocolHTTPS)
	assert.Equal(t, "ENTIRE_CHECKPOINT_TOKEN", method)
}

func TestDetectAuthMethod_TokenIgnoredForSSH(t *testing.T) {
	t.Setenv("ENTIRE_CHECKPOINT_TOKEN", "ghp_abc123")
	method := detectAuthMethod(context.Background(), fetchProtocolSSH)
	assert.Equal(t, "ENTIRE_CHECKPOINT_TOKEN set (ignored: remote uses SSH)", method)
}

func TestDetectAuthMethod_SSHWithAgent(t *testing.T) {
	t.Setenv("ENTIRE_CHECKPOINT_TOKEN", "")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/ssh-agent.sock")
	method := detectAuthMethod(context.Background(), fetchProtocolSSH)
	assert.Equal(t, "SSH agent", method)
}

func TestDetectAuthMethod_SSHNoAgent(t *testing.T) {
	t.Setenv("ENTIRE_CHECKPOINT_TOKEN", "")
	t.Setenv("SSH_AUTH_SOCK", "")
	method := detectAuthMethod(context.Background(), fetchProtocolSSH)
	assert.Equal(t, "SSH (no agent detected)", method)
}

func TestResolveProtocolForTarget_HTTPS(t *testing.T) {
	t.Parallel()

	protocol := resolveProtocolForTarget(context.Background(), "https://github.com/org/repo.git")
	assert.Equal(t, fetchProtocolHTTPS, protocol)
}

func TestResolveProtocolForTarget_SSH(t *testing.T) {
	t.Parallel()

	protocol := resolveProtocolForTarget(context.Background(), "git@github.com:org/repo.git")
	assert.Equal(t, fetchProtocolSSH, protocol)
}

func TestResolveProtocolForTarget_UnknownRemoteName(t *testing.T) {
	t.Parallel()

	// A non-existent remote name should return empty protocol.
	protocol := resolveProtocolForTarget(context.Background(), "nonexistent-remote")
	assert.Empty(t, protocol)
}

func TestResolveRemoteInfo_HTTPS(t *testing.T) {
	t.Parallel()

	info := resolveRemoteInfo(context.Background(), "https://github.com/myorg/my-repo.git")
	assert.Equal(t, "github.com", info.domain)
	assert.Equal(t, "myorg/my-repo", info.ownerRepo)
}

func TestResolveRemoteInfo_SSH(t *testing.T) {
	t.Parallel()

	info := resolveRemoteInfo(context.Background(), "git@github.com:myorg/my-repo.git")
	assert.Equal(t, "github.com", info.domain)
	assert.Equal(t, "myorg/my-repo", info.ownerRepo)
}

func TestResolveRemoteInfo_UnresolvableRemoteName(t *testing.T) {
	t.Parallel()

	info := resolveRemoteInfo(context.Background(), "nonexistent-remote")
	assert.Equal(t, "nonexistent-remote", info.domain)
	assert.Empty(t, info.ownerRepo)
}

func TestIsDebugMode(t *testing.T) {
	t.Setenv("ENTIRE_LOG_LEVEL", "DEBUG")
	assert.True(t, isDebugMode())
}

func TestIsDebugMode_NotDebug(t *testing.T) {
	t.Setenv("ENTIRE_LOG_LEVEL", "INFO")
	assert.False(t, isDebugMode())
}

func TestIsFetchURL(t *testing.T) {
	t.Parallel()

	assert.True(t, isFetchURL("https://github.com/org/repo.git"))
	assert.True(t, isFetchURL("git@github.com:org/repo.git"))
	assert.False(t, isFetchURL("origin"))
}
