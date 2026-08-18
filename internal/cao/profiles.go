package cao

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// supervisorAllowedTools overrides CAO's ROLE_TOOL_DEFAULTS["supervisor"]
// (@cao-mcp-server, fs_read, fs_list), which launches the supervisor pane
// with --disallowedTools Bash. Without execute_bash the supervisor cannot
// run the `agentpick dispatch` commands this prompt tells it to run, and
// cannot verify a worker's claims. fs_write stays out, so the supervisor
// still cannot implement — only coordinate and verify.
var supervisorAllowedTools = []string{"@cao-mcp-server", "fs_read", "fs_list", "execute_bash"}

func mcpFrontmatter(name, desc, role, caoProvider string, allowedTools []string) string {
	tools := ""
	if len(allowedTools) > 0 {
		var t strings.Builder
		t.WriteString("allowedTools:\n")
		for _, tool := range allowedTools {
			fmt.Fprintf(&t, "  - %q\n", tool)
		}
		tools = t.String()
	}
	return fmt.Sprintf(`---
name: %s
description: %s
role: %s
provider: %s
mcpServers:
  cao-mcp-server:
    type: stdio
    command: cao-mcp-server
    args: []
%s---
`, name, desc, role, caoProvider, tools)
}

func supervisorMarkdown(supervisor string, w Workers, workDir string) string {
	id, _ := ProviderID(supervisor)
	if id == "" {
		id = "cursor_cli"
	}
	sup := strings.ToLower(strings.TrimSpace(supervisor))
	var b strings.Builder
	name := sup
	if name == "" {
		name = id
	}
	b.WriteString(mcpFrontmatter(SupervisorProfile, "agentpick vibe supervisor — workers already routed", "supervisor", id, supervisorAllowedTools))
	b.WriteString("# agentpick supervisor\n\n")
	b.WriteString("You are the vibe-coding supervisor launched by **agentpick** (AWS CAO).\n\n")
	b.WriteString("## Your CLI is full-featured\n\n")
	fmt.Fprintf(&b, "This **is** the `%s` CLI. Workspace slash commands from `.cursor/commands` work here\n", name)
	b.WriteString("(`/start`, `/wrap-up`, `/do-next`, `/create-pr`, …). Use them. Do not tell the human\n")
	b.WriteString("those commands are IDE-only or missing.\n\n")
	b.WriteString("Orchestration **adds** parallel workers on top of that CLI. It does not replace or reduce it.\n\n")
	b.WriteString("## Divide and conquer — talk; do not do specialist work yourself\n\n")
	b.WriteString("You coordinate. Workers implement and review. Agents **must talk to each other** via CAO MCP:\n\n")
	b.WriteString("1. Split the human's task into slices.\n")
	b.WriteString("2. For each slice, pick the **best already-routed worker** from the table below (role + leftover usage at start).\n")
	b.WriteString("3. **send_message** that worker a self-contained brief. You may **send_message another instance of yourself**\n")
	b.WriteString("   (`agentpick_dev` when it is the same CLI) for parallel coding — that pane is a worker, not you.\n")
	b.WriteString("4. Workers **send_message you back**. They may also send_message each other when a slice depends on another.\n")
	b.WriteString("5. Do **not** implement specialist work in your own pane. Tiny one-file edits only, or work with no remaining worker.\n")
	b.WriteString("6. If a worker hits usage limits, send_message the next healthy CAO worker or run the dispatch command. Never ask the human which CLI.\n")
	b.WriteString("7. Only `assign` if a needed profile is **missing**. Never duplicate-assign. Never ask the human to run `agentpick route`.\n\n")
	b.WriteString("### Session routing (quota at start)\n\n")
	b.WriteString("This session loads **every healthy installed CLI**, not only one implement + one review winner.\n")
	b.WriteString("Pick by role + leftover usage. send_message the matching profile.\n\n")
	wrote := false
	for _, wr := range allWorkers(w) {
		if wr.Provider == "" {
			continue
		}
		wrote = true
		via := wr.Via
		if via == "" {
			via = ViaCAO
		}
		line := fmt.Sprintf("- **%s** → `%s` via %s", wr.Role, wr.Provider, via)
		if wr.Profile != "" && via == ViaCAO {
			line = fmt.Sprintf("- **%s** → **%s** (`%s` / CAO `%s`)", wr.Role, wr.Profile, wr.Provider, wr.CAOProvider)
		}
		if wr.Why != "" {
			line += " — leftover " + wr.Why
		}
		if wr.Role == "implement" && strings.EqualFold(wr.Provider, sup) {
			line += " — **second instance of you**; send_message it"
		}
		if wr.Role == "review" {
			line += ". Never review your own work."
		}
		if wr.Role == "peer" && via == ViaCAO {
			line += " — extra fleet pane; send_message when this CLI is the better leftover-quota fit"
		}
		b.WriteString(line + "\n")
	}
	if !wrote {
		b.WriteString("- No extra workers this session — do the work yourself and say independent review is skipped.\n")
	}
	if w.Implement.Provider == "" {
		b.WriteString("- No implement worker this session.\n")
	}
	if w.Review.Provider == "" {
		b.WriteString("- No CAO review worker this session — say so and do a self-check; independent review is skipped.\n")
	}
	b.WriteString("\n### Dispatch (same machine, not in Spawn Agent)\n\n")
	b.WriteString("CAO 2.4.1 cannot spawn Grok or Ollama. If a worker below is `via dispatch`, **run the command yourself** — do not ask the human, do not skip the peer.\n\n")
	wroteDispatch := false
	for _, wr := range allWorkers(w) {
		if cmd := wr.DispatchCmd(workDir); cmd != "" {
			fmt.Fprintf(&b, "- **%s** → `%s`\n", wr.Provider, cmd)
			wroteDispatch = true
		}
	}
	if !wroteDispatch {
		b.WriteString("- No dispatch workers this session (Grok/Ollama not selected).\n")
	}
	b.WriteString("\nDo not start dureef-sprint. Ports: 8787 Cursor OAuth, 8788 Headroom, 9889 this CAO UI.\n")
	return b.String()
}

func workerMarkdown(profile, role, caoProvider, agentpickName string) string {
	title := "developer"
	desc := "agentpick implement worker"
	if role == "reviewer" {
		title = "reviewer"
		desc = "agentpick independent review worker"
	}
	var b strings.Builder
	b.WriteString(mcpFrontmatter(profile, desc, role, caoProvider, nil))
	fmt.Fprintf(&b, "# agentpick %s (%s)\n\n", title, agentpickName)
	b.WriteString("You are a CAO specialist. Divide-and-conquer with the supervisor and other workers.\n\n")
	b.WriteString("- Stay idle until you get a **send_message** (or assign) with a self-contained slice.\n")
	b.WriteString("- Do that slice. Then **send_message the supervisor** with the result. Do not go silent.\n")
	b.WriteString("- You may **send_message** the other worker when a slice depends on them (or to hand off).\n")
	b.WriteString("- If you are a second instance of the same CLI as the supervisor, you are still a **worker** — do the coding; they coordinate.\n")
	b.WriteString("- Do not start dureef-sprint. Do not ask the human which model to use.\n")
	return b.String()
}

// InstallSessionProfiles writes and `cao install`s supervisor + worker profiles for this session.
func InstallSessionProfiles(supervisor string, w Workers, workDir string) error {
	caoBin, err := exec.LookPath("cao")
	if err != nil {
		return fmt.Errorf("cao not on PATH")
	}
	dir, err := profileDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	type item struct {
		name string
		body string
		prov string
	}
	supID, err := ProviderID(supervisor)
	if err != nil {
		return err
	}
	items := []item{{SupervisorProfile, supervisorMarkdown(supervisor, w, workDir), supID}}
	if w.Implement.Via == ViaCAO && w.Implement.CAOProvider != "" {
		items = append(items, item{DevProfile, workerMarkdown(DevProfile, "developer", w.Implement.CAOProvider, w.Implement.Provider), w.Implement.CAOProvider})
	}
	if w.Review.Via == ViaCAO && w.Review.CAOProvider != "" {
		items = append(items, item{ReviewProfile, workerMarkdown(ReviewProfile, "reviewer", w.Review.CAOProvider, w.Review.Provider), w.Review.CAOProvider})
	}
	for _, wr := range w.Extra {
		if wr.Via != ViaCAO || wr.Profile == "" || wr.CAOProvider == "" {
			continue
		}
		items = append(items, item{wr.Profile, workerMarkdown(wr.Profile, "developer", wr.CAOProvider, wr.Provider), wr.CAOProvider})
	}
	for _, it := range items {
		path := filepath.Join(dir, it.name+".md")
		if err := os.WriteFile(path, []byte(it.body), 0o644); err != nil {
			return err
		}
		cmd := exec.Command(caoBin, "install", path, "--provider", it.prov)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cao install %s: %w (%s)", it.name, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func profileDir() (string, error) {
	cache := strings.TrimSpace(os.Getenv("AGENTPICK_CACHE_DIR"))
	if cache == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		cache = filepath.Join(home, ".cache", "agentpick")
	}
	return filepath.Join(cache, "cao-profiles"), nil
}
