package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	claudeLookPath    = exec.LookPath
	claudeRun         = defaultClaudeRun
	weekUsedRE        = regexp.MustCompile(`(?i)Current week[^\n]*?(\d+(?:\.\d+)?)\s*%\s*used`)
	sessionUsedRE     = regexp.MustCompile(`(?i)Current session[^\n]*?(\d+(?:\.\d+)?)\s*%\s*used`)
	weekResetRE       = regexp.MustCompile(`(?i)Current week[^\n]*?resets\s+([^\n(]+)`)
	sessionResetRE    = regexp.MustCompile(`(?i)Current session[^\n]*?resets\s+([^\n(]+)`)
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
	if s, ok := probeClaudeUsage(ctx); ok {
		return s
	}
	if s, ok := probeClaudeSessionHistory(); ok {
		return s
	}
	return Snapshot{
		Provider:          "claude",
		Source:            "unknown",
		UnavailableReason: "Claude /usage unavailable (not on PATH or timed out)",
		Label:             FormatLabel(Snapshot{UnavailableReason: "Claude /usage unavailable (not on PATH or timed out)"}),
		Err:               "claude usage unavailable",
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

	week, weekOK := parseClaudeWeekUsed(out)
	session, sessionOK := parseClaudeSessionUsed(out)
	weekReset := cleanReset(parseReset(weekResetRE, out))
	sessionReset := cleanReset(parseReset(sessionResetRE, out))

	if weekOK {
		remaining := clampPct(100 - week)
		s := Snapshot{
			Provider:     "claude",
			RemainingPct: &remaining,
			Window:       "week",
			ResetHint:    weekReset,
			Source:       "claude-usage",
		}
		if sessionOK {
			sessLeft := clampPct(100 - session)
			detail := fmtSessionDetail(sessLeft, sessionReset)
			s.Detail = detail
		}
		if err != nil {
			s.Err = err.Error()
		}
		s.Label = FormatLabel(s)
		return s, true
	}
	if sessionOK {
		remaining := clampPct(100 - session)
		s := Snapshot{
			Provider:     "claude",
			RemainingPct: &remaining,
			Window:       "session",
			ResetHint:    sessionReset,
			Source:       "claude-session",
		}
		s.Label = FormatLabel(s)
		return s, true
	}
	_ = err
	return Snapshot{}, false
}

func fmtSessionDetail(left float64, reset string) string {
	d := fmt.Sprintf("session %.0f%% left", left)
	if reset != "" {
		d += " (" + reset + ")"
	}
	return d
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
				FH *float64 `json:"fh"`
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
		Window:       "session",
		Source:       "claude-history",
		Detail:       "from local Claude history",
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

func parseReset(re *regexp.Regexp, out string) string {
	m := re.FindStringSubmatch(out)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func cleanReset(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "· ")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(s), "resets") {
		s = "resets " + s
	}
	// Keep it short for the table.
	if len(s) > 28 {
		s = s[:28]
	}
	return s
}
