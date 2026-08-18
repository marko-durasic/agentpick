package cao

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const startCommandRel = ".cursor/commands/start.md"

// WorkDirChoice is the project root CAO/Cursor CLI should open.
type WorkDirChoice struct {
	Path   string
	Reason string
	HasCLI bool // .cursor/commands/start.md present
}

// ResolveWorkDir picks a workspace that keeps Cursor CLI slash commands.
// Explicit --dir / AGENTPICK_CAO_WORKDIR wins. The agentpick source tree is
// never used as the vibe project (it has no DuReef commands). Prefer an
// isolated DuReef clone when present so workers do not dirty owner-stable main.
func ResolveWorkDir(explicit string) (WorkDirChoice, error) {
	if p := strings.TrimSpace(explicit); p != "" {
		abs, err := filepath.Abs(p)
		if err != nil {
			return WorkDirChoice{}, err
		}
		return WorkDirChoice{Path: abs, Reason: "explicit --dir", HasCLI: hasStartCommand(abs)}, nil
	}
	if p := strings.TrimSpace(os.Getenv("AGENTPICK_CAO_WORKDIR")); p != "" {
		abs, err := filepath.Abs(p)
		if err != nil {
			return WorkDirChoice{}, err
		}
		return WorkDirChoice{Path: abs, Reason: "AGENTPICK_CAO_WORKDIR", HasCLI: hasStartCommand(abs)}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return WorkDirChoice{}, err
	}
	if !isAgentpickCheckout(cwd) && hasStartCommand(cwd) {
		return WorkDirChoice{Path: cwd, Reason: "cwd has .cursor/commands", HasCLI: true}, nil
	}
	dureef := findDureefRoot(cwd)
	if dureef != "" {
		iso := isolatedWorkspace(dureef)
		if hasStartCommand(iso) {
			return WorkDirChoice{Path: iso, Reason: "DuReef isolated workspace clone (slash commands + keep owner main clean)", HasCLI: true}, nil
		}
		if hasStartCommand(dureef) {
			return WorkDirChoice{Path: dureef, Reason: "DuReef workspace root", HasCLI: true}, nil
		}
	}
	return WorkDirChoice{Path: cwd, Reason: "cwd", HasCLI: hasStartCommand(cwd)}, nil
}

func hasStartCommand(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, startCommandRel))
	return err == nil && !st.IsDir()
}

func isAgentpickCheckout(dir string) bool {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), "module github.com/marko-durasic/agentpick")
}

func findDureefRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if fileExists(filepath.Join(dir, "config", "company.yaml")) && hasStartCommand(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isolatedWorkspace(dureefRoot string) string {
	slug := strings.TrimSpace(os.Getenv("DUREEF_AGENT_CLONE_SLUG"))
	if slug == "" {
		slug = "cursor-chat"
	}
	return filepath.Join(dureefRoot, "tmp", slug, "git_repos", "workspace")
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// FormatWorkDir explains the chosen workspace on stderr.
func FormatWorkDir(c WorkDirChoice) string {
	if c.HasCLI {
		return fmt.Sprintf("workspace=%s (%s) — Cursor CLI slash commands loaded (/start, /wrap-up, …)", c.Path, c.Reason)
	}
	return fmt.Sprintf("workspace=%s (%s) — no .cursor/commands/start.md here; pass --dir to a DuReef checkout for /start /wrap-up", c.Path, c.Reason)
}
