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

const scrollReapplyFor = 12 * time.Second

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

// tmuxExactTarget is tmux's exact session match (`=name`) so a short prefix
// cannot hit a leftover cao-agentpick-* session.
func tmuxExactTarget(session string) string {
	return "=" + session
}

func tmuxScrollArgv(session string) ([][]string, error) {
	if !tmuxSessionNameOK(session) {
		return nil, fmt.Errorf("unsafe tmux session name %q", session)
	}
	return [][]string{
		{"tmux", "set-option", "-t", session, "mouse", "on"},
		{"tmux", "set-option", "-t", session, "history-limit", ScrollHistoryLimit},
		{"tmux", "set-hook", "-t", session, "client-attached", "set-option mouse on"},
	}, nil
}

func applyTmuxArgv(ctx context.Context, cmds [][]string) error {
	for _, argv := range cmds {
		out, runErr := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("%s: %w (%s)", strings.Join(argv, " "), runErr, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func refreshTmuxClients(ctx context.Context, session string) {
	out, err := exec.CommandContext(ctx, "tmux", "list-clients", "-t", session, "-F", "#{client_tty}").Output()
	if err != nil {
		return
	}
	for _, tty := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tty = strings.TrimSpace(tty)
		if tty == "" {
			continue
		}
		_ = exec.CommandContext(ctx, "tmux", "refresh-client", "-t", tty).Run()
	}
}

// EnableSessionScroll waits until the CAO tmux session exists, then turns on
// mouse + a large history-limit so wheel/PageUp reach Cursor/Claude TUIs
// (CAO 2.4.1 leaves mouse off; alt-screen TUIs then have nothing to scroll).
// Options are re-applied for a few seconds because cao launch attaches and
// adds worker windows after the session first appears.
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
	deadline := time.Now().Add(scrollReapplyFor)
	var lastErr error
	applied := false
	for {
		if err := applyTmuxArgv(ctx, cmds); err != nil {
			lastErr = err
		} else {
			applied = true
			refreshTmuxClients(ctx, session)
		}
		if !time.Now().Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			if applied {
				return nil
			}
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	if !applied {
		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("tmux scroll options were not applied for %s", session)
	}
	fmt.Fprintf(os.Stderr, "agentpick: tmux mouse on for %s (wheel / PageUp-PageDown in the pane; Ctrl-b [ for copy-mode)\n", session)
	return nil
}

func waitForTmuxSession(ctx context.Context, session string) error {
	if !tmuxSessionNameOK(session) {
		return fmt.Errorf("unsafe tmux session name %q", session)
	}
	target := tmuxExactTarget(session)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", target)
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
