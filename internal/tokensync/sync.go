// Package tokensync runs `tokensave sync` across indexed projects before
// launching an agent so the code graph stays current.
package tokensync

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Options controls the preflight sync.
type Options struct {
	// NoTokensave skips the sync entirely.
	NoTokensave bool
	// DryRun prints planned syncs without executing them.
	DryRun bool
	// Out receives status lines (defaults to os.Stderr).
	Out io.Writer
}

// Result summarizes what happened.
type Result struct {
	Skipped string
	Paths   []string
	Failed  []string // path: reason
}

var (
	lookPath = exec.LookPath
	runCmd   = defaultRunCmd
	listPathRE = regexp.MustCompile(`(?m)^\s+(/\S+)\s+`)
)

func defaultRunCmd(name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.Command(name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}

// Available reports whether tokensave is on PATH.
func Available() bool {
	_, err := lookPath("tokensave")
	return err == nil
}

// SyncAll lists every tokensave project (`tokensave list -a`) and runs
// `tokensave sync <path>` on each. Missing tokensave or list failures are
// non-fatal so agent launch still proceeds.
func SyncAll(opt Options) Result {
	out := opt.Out
	if out == nil {
		out = os.Stderr
	}

	if opt.NoTokensave {
		return Result{Skipped: "forced by --no-tokensave"}
	}

	bin, err := lookPath("tokensave")
	if err != nil {
		return Result{Skipped: "tokensave not on PATH; skipping graph sync"}
	}

	stdout, stderr, err := runCmd(bin, "list", "-a")
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = err.Error()
		}
		fmt.Fprintf(out, "agentpick: tokensave list failed (%s); skipping graph sync\n", msg)
		return Result{Skipped: "tokensave list failed"}
	}

	paths := parseListPaths(string(stdout))
	if len(paths) == 0 {
		// Fall back to cwd when list is empty but tokensave exists —
		// syncing cwd is still useful for a freshly indexed tree.
		if cwd, err := os.Getwd(); err == nil {
			paths = []string{cwd}
		}
	}
	if len(paths) == 0 {
		return Result{Skipped: "no tokensave projects found"}
	}

	sort.Strings(paths)
	res := Result{Paths: append([]string{}, paths...)}

	if opt.DryRun {
		for _, p := range paths {
			fmt.Fprintf(out, "dry-run: tokensave sync %s\n", p)
		}
		return res
	}

	fmt.Fprintf(out, "agentpick: tokensave sync %d project(s)…\n", len(paths))

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range paths {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errOut, err := runCmd(bin, "sync", p)
			if err != nil {
				reason := strings.TrimSpace(firstLine(string(errOut)))
				if reason == "" {
					reason = err.Error()
				}
				mu.Lock()
				res.Failed = append(res.Failed, fmt.Sprintf("%s: %s", p, reason))
				mu.Unlock()
				fmt.Fprintf(out, "agentpick: tokensave sync failed for %s: %s\n", p, reason)
				return
			}
			fmt.Fprintf(out, "agentpick: tokensave sync ok %s\n", displayPath(p))
		}()
	}
	wg.Wait()
	sort.Strings(res.Failed)
	return res
}

func parseListPaths(out string) []string {
	seen := map[string]struct{}{}
	var paths []string
	for _, m := range listPathRE.FindAllStringSubmatch(out, -1) {
		if len(m) < 2 {
			continue
		}
		p := filepath.Clean(m[1])
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	return paths
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func displayPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(p, home+"/") {
		return p
	}
	return "~" + strings.TrimPrefix(p, home)
}
