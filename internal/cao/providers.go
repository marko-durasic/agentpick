package cao

import (
	"fmt"
	"strings"
)

// PinVersion is the PyPI release we dogfood. Do not install @main.
const PinVersion = "2.4.1"

// DefaultHost is loopback only. Never 0.0.0.0.
const DefaultHost = "127.0.0.1"

// DefaultPort is the CAO local UI / API. Do not use 8787 (Cursor OAuth) or 8788 (Headroom).
const DefaultPort = 9889

const (
	oauthPort    = 8787
	headroomPort = 8788
)

// agentProfile is the built-in supervisor CAO ships.
const agentProfile = "code_supervisor"

// ProviderID is CAO's --provider value for an agentpick name.
func ProviderID(agentpickName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(agentpickName)) {
	case "cursor":
		return "cursor_cli", nil
	case "claude":
		return "claude_code", nil
	case "codex":
		return "codex", nil
	case "agy":
		return "antigravity_cli", nil
	case "copilot":
		return "copilot_cli", nil
	case "grok":
		return "", fmt.Errorf("CAO %s has no grok provider; pick cursor, claude, agy, copilot, or codex (or run agentpick grok for a single-CLI session)", PinVersion)
	case "ollama":
		return "", fmt.Errorf("ollama is a tiny helper, not a CAO supervisor; pick a coding CLI")
	default:
		if agentpickName == "" {
			return "", fmt.Errorf("empty provider")
		}
		return "", fmt.Errorf("unknown agentpick provider %q", agentpickName)
	}
}
