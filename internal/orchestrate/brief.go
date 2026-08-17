package orchestrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const briefFileName = "orchestrator-brief.md"

// BriefPath is where the session briefing is written.
func BriefPath() string {
	if v := strings.TrimSpace(os.Getenv("AGENTPICK_ORCHESTRATOR_BRIEF")); v != "" {
		return v
	}
	cache := strings.TrimSpace(os.Getenv("AGENTPICK_CACHE_DIR"))
	if cache == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			cache = filepath.Join(home, ".cache", "agentpick")
		}
	}
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "agentpick")
	}
	return filepath.Join(cache, briefFileName)
}

// WriteBrief writes the orchestrator instructions for this session.
func WriteBrief(provider, reason string) (string, error) {
	path := BriefPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := RenderBrief(provider, reason)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// RenderBrief is the instruction the chosen CLI should follow while vibe-coding.
func RenderBrief(provider, reason string) string {
	if provider == "" {
		provider = "unknown"
	}
	if reason == "" {
		reason = "interactive orchestrator pick"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# agentpick orchestrator session\n\n")
	fmt.Fprintf(&b, "You are the **session orchestrator** for this vibe-coding session.\n\n")
	fmt.Fprintf(&b, "- Launched by: agentpick\n")
	fmt.Fprintf(&b, "- You are: **%s**\n", provider)
	fmt.Fprintf(&b, "- Why you were picked: %s\n", reason)
	fmt.Fprintf(&b, "- Brief written: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	b.WriteString("## Your job\n\n")
	b.WriteString("Talk to the human and do routine coding yourself when you are a good fit.\n")
	b.WriteString("When a subtask is a better fit for another coding CLI, **delegate** — do not\n")
	b.WriteString("fake a specialist review or burn your own quota on work another peer does better.\n\n")
	b.WriteString("Stay the conversation owner. Summarize delegated output back to the human.\n")
	b.WriteString("Do not spawn extra agents for a trivial one-file edit you can do yourself.\n\n")
	b.WriteString("## How to delegate (run as shell commands)\n\n")
	b.WriteString("Pick first (optional):\n\n")
	fmt.Fprintf(&b, "    agentpick route --role <role> --exclude %s\n\n", provider)
	b.WriteString("Then run:\n\n")
	fmt.Fprintf(&b, "    agentpick dispatch --role <role> --exclude %s -p \"<self-contained task>\" --dir <repo>\n\n", provider)
	b.WriteString("Roles:\n\n")
	b.WriteString("| role | When | Typical winner |\n")
	b.WriteString("|------|------|----------------|\n")
	b.WriteString("| plan | architecture, ambiguous design | Claude |\n")
	b.WriteString("| review | independent review of *your* changes | Codex (never yourself) |\n")
	b.WriteString("| debug | hard / ambiguous bugs | Claude or Codex |\n")
	b.WriteString("| implement | extra write worker in parallel | another healthy peer |\n")
	b.WriteString("| tiny | classify / format / yes-no | local ollama |\n\n")
	b.WriteString("## Hard rules\n\n")
	fmt.Fprintf(&b, "1. On **review**, always --exclude %s (no self-review).\n", provider)
	b.WriteString("2. Give dispatch a **self-contained** prompt (paths, goal, constraints). The peer has no chat history.\n")
	b.WriteString("3. After dispatch, read stdout and integrate or reject — do not ignore it.\n")
	b.WriteString("4. Escalate to the human only for credentials, legal, production secrets, or irreversible public actions.\n")
	b.WriteString("5. Prefer finishing the current slice over opening more parallel work.\n\n")
	b.WriteString("When the human greets you or starts a task, acknowledge you are the orchestrator\n")
	b.WriteString("and that you will delegate specialist work via agentpick as needed — then do the work.\n")
	return b.String()
}

// ExtraArgs injects the briefing into an interactive CLI launch.
func ExtraArgs(provider, briefPath string) []string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	prompt := fmt.Sprintf(
		"You are the session orchestrator launched by agentpick. Read and follow %s. You will vibe-code with me and delegate specialist work via agentpick dispatch / agentpick route as needed (always --exclude %s on review). Acknowledge briefly, then wait for my task.",
		briefPath, provider,
	)
	if provider == "claude" {
		return []string{"--append-system-prompt", prompt}
	}
	return []string{prompt}
}

// EnvVars marks this process as an orchestrator session for child CLIs.
func EnvVars(provider, briefPath string) map[string]string {
	return map[string]string{
		"AGENTPICK_ORCHESTRATOR":          "1",
		"AGENTPICK_ORCHESTRATOR_PROVIDER": provider,
		"AGENTPICK_ORCHESTRATOR_BRIEF":    briefPath,
	}
}
