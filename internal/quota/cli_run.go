package quota

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runCLI runs a binary with args under ctx, returning combined stdout+stderr.
func runCLI(ctx context.Context, bin string, args ...string) (string, error) {
	return runCLIEnv(ctx, nil, bin, args...)
}

func runCLIEnv(ctx context.Context, extraEnv []string, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// lookCLI is overridable in tests.
var lookCLI = exec.LookPath

func combinedOut(stdout, stderr []byte) string {
	s := string(stdout)
	if s == "" {
		return string(stderr)
	}
	if len(stderr) == 0 {
		return s
	}
	return s + "\n" + string(stderr)
}

func shortResetFromUnix(sec int64) string {
	if sec <= 0 {
		return ""
	}
	t := time.Unix(sec, 0)
	return "resets " + t.Format("Jan 2, 3:04pm")
}

func windowFromSeconds(sec float64) string {
	switch {
	case sec >= 28*24*3600:
		return "month"
	case sec >= 6*24*3600:
		return "week"
	case sec >= 3*3600:
		return "session"
	default:
		return "window"
	}
}

func containsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}
