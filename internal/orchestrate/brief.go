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
	b.WriteString("You coordinate. **send_message** specialists; do not do specialist work yourself.\n")
	b.WriteString("Workers talk to you and each other. Divide and conquer. Pick the worker by role and leftover usage.\n")
	b.WriteString("You may send_message **another instance of yourself** when that was the quota pick.\n")
	b.WriteString("Stay the conversation owner. Summarize worker output back to the human.\n")
	b.WriteString("Do not spawn extra agents for a trivial one-file edit you can do yourself.\n\n")
	b.WriteString("## How to delegate (automatic)\n\n")
	b.WriteString("agentpick **already routed** workers for this session. Do **not** ask the human to run `agentpick route`.\n")
	b.WriteString("This **is** Cursor CLI with the workspace loaded. Slash commands from `.cursor/commands`\n")
	b.WriteString("(`/start`, `/wrap-up`, `/do-next`, …) work. Use them. Orchestration adds workers; it does not remove CLI features.\n\n")
	b.WriteString("When specialist/parallel work is needed:\n")
	b.WriteString("- **send_message** **agentpick_dev** / **agentpick_review** (assign only if missing). Dev may be a second instance of you.\n")
	b.WriteString("- If a worker is **grok** or **ollama**, they are not in Spawn Agent. Run\n")
	b.WriteString("  `agentpick dispatch --role <role> --prefer <provider> -p \"<task>\"` yourself. Do not ask the human.\n")
	b.WriteString("  Tiny/local classify-format → `--role tiny --prefer ollama`.\n\n")
	b.WriteString("This session is AWS CAO (tmux). Do not start dureef-sprint.\n")
	b.WriteString("Ports: 8787 Cursor OAuth, 8788 Headroom, 9889 CAO localhost only.\n\n")
	b.WriteString("Roles:\n\n")
	b.WriteString("| role | When | Typical winner |\n")
	b.WriteString("|------|------|----------------|\n")
	b.WriteString("| plan | architecture, ambiguous design | Claude |\n")
	b.WriteString("| review | independent review of *your* changes | Codex (never yourself) |\n")
	b.WriteString("| debug | hard / ambiguous bugs | Claude or Codex |\n")
	b.WriteString("| implement | extra write worker in parallel | another healthy peer |\n")
	b.WriteString("| tiny | classify / format / yes-no | local ollama |\n\n")
	b.WriteString("## Hard rules\n\n")
	b.WriteString("1. Never self-review — assign **agentpick_review**.\n")
	b.WriteString("2. Give workers a **self-contained** task (paths, goal, constraints).\n")
	b.WriteString("3. After assign, read the result and integrate or reject — do not ignore it.\n")
	b.WriteString("4. Escalate to the human only for credentials, legal, production secrets, or irreversible public actions.\n")
	b.WriteString("5. Prefer finishing the current slice over opening more parallel work.\n\n")
	b.WriteString("When the human starts a task, delegate automatically if needed — then do the work.\n")
	return b.String()
}

// ExtraArgs injects the briefing into an interactive CLI launch.
func ExtraArgs(provider, briefPath string) []string {
	provider = strings.TrimSpace(strings.ToLower(provider))
	prompt := fmt.Sprintf(
		"You are the session orchestrator launched by agentpick via CAO. Read and follow %s. Full Cursor CLI. send_message agentpick_dev/review (dev may be a second instance of you); agentpick dispatch --prefer grok / --role tiny --prefer ollama when those workers were routed. Never ask the human to run agentpick route. Wait for my task.",
		briefPath,
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
