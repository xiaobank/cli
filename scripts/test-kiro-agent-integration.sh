#!/usr/bin/env bash
# test-kiro-agent-integration.sh — validates Kiro hook firing and stdin format.
#
# Prerequisites:
#   - kiro-cli installed and on PATH
#   - A git repo with `entire enable --agent kiro` already run
#
# Usage:
#   cd /path/to/test-repo
#   bash scripts/test-kiro-agent-integration.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Kiro Agent Integration Test ==="
echo ""

# Check prerequisites
if ! command -v kiro-cli &>/dev/null; then
    echo "ERROR: kiro-cli not found on PATH"
    exit 1
fi

if ! command -v entire &>/dev/null; then
    echo "WARNING: entire CLI not on PATH, using go run"
    ENTIRE_CMD="go run ${REPO_ROOT}/cmd/entire/main.go"
else
    ENTIRE_CMD="entire"
fi

# Verify hooks are installed
echo "1. Checking hook installation..."
if [ -f ".kiro/agents/entire.json" ]; then
    echo "   ✓ Hook config exists at .kiro/agents/entire.json"
    echo "   Contents:"
    cat .kiro/agents/entire.json | head -20
else
    echo "   ✗ Hook config not found. Run: ${ENTIRE_CMD} enable --agent kiro"
    exit 1
fi

echo ""
echo "2. Testing hook stdin capture..."

# Create a temp directory for captured payloads
CAPTURE_DIR=$(mktemp -d)
trap 'rm -rf "$CAPTURE_DIR"' EXIT

# Install capture hooks (replace entire hooks with capture scripts)
mkdir -p .kiro/agents
cat > .kiro/agents/capture.json <<EOF
{
  "agentSpawn": [{"command": "cat > ${CAPTURE_DIR}/agent-spawn.json"}],
  "userPromptSubmit": [{"command": "cat > ${CAPTURE_DIR}/user-prompt-submit.json"}],
  "stop": [{"command": "cat > ${CAPTURE_DIR}/stop.json"}]
}
EOF

echo "   Capture hooks installed. Run kiro-cli and submit a prompt, then exit."
echo "   Captured payloads will be in: ${CAPTURE_DIR}/"
echo ""
echo "   After running kiro-cli, check:"
echo "     cat ${CAPTURE_DIR}/agent-spawn.json"
echo "     cat ${CAPTURE_DIR}/user-prompt-submit.json"
echo "     cat ${CAPTURE_DIR}/stop.json"
echo ""
echo "   Expected format:"
echo '   {"hook_event_name": "agentSpawn", "cwd": "/path/to/repo"}'
echo '   {"hook_event_name": "userPromptSubmit", "cwd": "/path/to/repo", "prompt": "user message"}'
echo '   {"hook_event_name": "stop", "cwd": "/path/to/repo"}'
echo ""
echo "3. Cleanup: Remove .kiro/agents/capture.json when done"
