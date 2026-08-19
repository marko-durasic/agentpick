package cao

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// ScrollHistoryLimit is tmux pane history for CAO sessions. Agent TUIs still
// keep their own conversation; this is for process output in copy-mode.
const ScrollHistoryLimit = "100000"

func tmuxSessionNameOK(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func tmuxScrollArgv(session string) ([][]string, error) {
	if !tmuxSessionNameOK(session) {
		return nil, fmt.Errorf("unsafe tmux session name %q", session)
	}
	return [][]string{
		{"tmux", "set-option", "-t", session, "mouse", "on"},
		{"tmux", "set-option", "-t", session, "history-limit", ScrollHistoryLimit},
	}, nil
}

// EnableSessionScroll waits until the CAO tmux session exists, then turns on
// mouse + a large history-limit so wheel/PageUp reach Cursor/Claude TUIs
// (CAO 2.4.1 leaves mouse off; alt-screen TUIs then have nothing to scroll).
func EnableSessionScroll(ctx context.Context, session string) error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux not on PATH")
	}
	cmds, err := tmuxScrollArgv(session)
	if err != nil {
		return err
	}
	if err := waitForTmuxSession(ctx, session); err != nil {
		return err
	}
	for _, argv := range cmds {
		out, runErr := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("%s: %w (%s)", strings.Join(argv, " "), runErr, strings.TrimSpace(string(out)))
		}
	}
	fmt.Fprintf(os.Stderr, "agentpick: tmux mouse on for %s (wheel / PageUp-PageDown in the pane; Ctrl-b [ for copy-mode)\n", session)
	return nil
}

func waitForTmuxSession(ctx context.Context, session string) error {
	if !tmuxSessionNameOK(session) {
		return fmt.Errorf("unsafe tmux session name %q", session)
	}
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", session)
		if cmd.Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("tmux session %q did not appear", session)
}
