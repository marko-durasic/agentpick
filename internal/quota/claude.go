package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
)

var (
	claudeLookPath = exec.LookPath
	claudeRun      = defaultClaudeRun
	weekUsedRE     = regexp.MustCompile(`(?i)Current week[^\n]*?(\d+(?:\.\d+)?)\s*%\s*used`)
	sessionUsedRE  = regexp.MustCompile(`(?i)Current session[^\n]*?(\d+(?:\.\d+)?)\s*%\s*used`)
	claudeHistoryPath = defaultClaudeHistoryPath
)

func defaultClaudeRun(ctx context.Context, bin string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func defaultClaudeHistoryPath() string {
	if p := os.Getenv("AGENTPICK_CLAUDE_USAGE_HISTORY"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "Claude", "plan-usage-history.json")
}

func probeClaude(ctx context.Context) Snapshot {
	// Prefer live /usage (week is the decision signal). Local history is a fast
	// session fallback when the CLI is slow or killed by timeout.
	if s, ok := probeClaudeUsage(ctx); ok {
		return s
	}
	if s, ok := probeClaudeSessionHistory(); ok {
		return s
	}
	return Snapshot{
		Provider: "claude",
		Label:    "n/a",
		Source:   "unknown",
		Err:      "claude usage unavailable",
	}
}

func probeClaudeUsage(ctx context.Context) (Snapshot, bool) {
	bin, err := claudeLookPath("claude")
	if err != nil {
		return Snapshot{}, false
	}
	stdout, stderr, err := claudeRun(ctx, bin, "/usage")
	out := string(stdout)
	if out == "" {
		out = string(stderr)
	}
	if out == "" {
		return Snapshot{}, false
	}
	// Prefer week even if the process was killed after printing (partial OK).
	if week, ok := parseClaudeWeekUsed(out); ok {
		remaining := clampPct(100 - week)
		s := Snapshot{
			Provider:     "claude",
			RemainingPct: &remaining,
			Source:       "claude-usage",
		}
		if err != nil {
			s.Err = err.Error()
		}
		s.Label = FormatLabel(s)
		return s, true
	}
	if session, ok := parseClaudeSessionUsed(out); ok {
		remaining := clampPct(100 - session)
		s := Snapshot{
			Provider:     "claude",
			RemainingPct: &remaining,
			Source:       "claude-session",
		}
		s.Label = FormatLabel(s)
		return s, true
	}
	_ = err
	return Snapshot{}, false
}

func probeClaudeSessionHistory() (Snapshot, bool) {
	path := claudeHistoryPath()
	if path == "" {
		return Snapshot{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, false
	}
	var doc struct {
		Samples []struct {
			U struct {
				FH *float64 `json:"fh"` // five-hour / session window % used
			} `json:"u"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || len(doc.Samples) == 0 {
		return Snapshot{}, false
	}
	last := doc.Samples[len(doc.Samples)-1]
	if last.U.FH == nil {
		return Snapshot{}, false
	}
	remaining := clampPct(100 - *last.U.FH)
	s := Snapshot{
		Provider:     "claude",
		RemainingPct: &remaining,
		Source:       "claude-history",
	}
	s.Label = FormatLabel(s)
	return s, true
}

func parseClaudeWeekUsed(out string) (float64, bool) {
	m := weekUsedRE.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseClaudeSessionUsed(out string) (float64, bool) {
	m := sessionUsedRE.FindStringSubmatch(out)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
