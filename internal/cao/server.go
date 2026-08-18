package cao

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type healthBody struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Healthy reports whether cao-server is up on host:port.
func Healthy(ctx context.Context, host string, port int) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/health", host, port), nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body healthBody
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return false
	}
	return body.Status == "ok"
}

// EnsureServer starts cao-server detached on loopback if needed.
func EnsureServer(ctx context.Context, plan Plan) error {
	if Healthy(ctx, plan.Host, plan.Port) {
		return nil
	}
	if len(plan.ServerArgv) < 1 {
		return fmt.Errorf("empty cao-server argv")
	}
	logPath := serverLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(plan.ServerArgv[0], plan.ServerArgv[1:]...)
	cmd.Stdout = logF
	cmd.Stderr = logF
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = logF.Close()
		return fmt.Errorf("start cao-server: %w (see %s)", err, logPath)
	}
	// Detach: do not Wait; init reaps. Close our copy of the log fd.
	_ = logF.Close()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if Healthy(ctx, plan.Host, plan.Port) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("cao-server did not become healthy on http://%s:%d/health (log: %s)", plan.Host, plan.Port, logPath)
}

func serverLogPath() string {
	cache := strings.TrimSpace(os.Getenv("AGENTPICK_CACHE_DIR"))
	if cache == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			cache = filepath.Join(home, ".cache", "agentpick")
		} else {
			cache = os.TempDir()
		}
	}
	return filepath.Join(cache, "cao-server.log")
}
