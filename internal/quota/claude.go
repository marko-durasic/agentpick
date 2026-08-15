package quota

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strconv"
)

var (
	claudeLookPath = exec.LookPath
	claudeRun      = defaultClaudeRun
	weekUsedRE     = regexp.MustCompile(`(?i)Current week[^\n]*?(\d+(?:\.\d+)?)\s*%\s*used`)
)

func defaultClaudeRun(ctx context.Context, bin string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

func probeClaude(ctx context.Context) Snapshot {
	bin, err := claudeLookPath("claude")
	if err != nil {
		return Snapshot{
			Provider: "claude",
			Label:    "—",
			Source:   "unknown",
			Err:      "claude not on PATH",
		}
	}
	// Prefer slash-command form; -p "/usage" also works on recent Claude Code.
	stdout, stderr, err := claudeRun(ctx, bin, "/usage")
	out := string(stdout)
	if out == "" {
		out = string(stderr)
	}
	if err != nil && out == "" {
		return Snapshot{
			Provider: "claude",
			Label:    "—",
			Source:   "unknown",
			Err:      err.Error(),
		}
	}
	pct, ok := parseClaudeWeekUsed(out)
	if !ok {
		return Snapshot{
			Provider: "claude",
			Label:    "—",
			Source:   "unknown",
			Err:      "could not parse claude /usage",
		}
	}
	remaining := clampPct(100 - pct)
	s := Snapshot{
		Provider:     "claude",
		RemainingPct: &remaining,
		Source:       "claude-usage",
	}
	s.Label = FormatLabel(s)
	return s
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
