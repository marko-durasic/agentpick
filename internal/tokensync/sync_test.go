package tokensync

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestParseListPaths(t *testing.T) {
	out := `Found 5 tokensave project(s):

  /home/ddjura/personal/tech/DuReef                            111.5 MB      457.1k tokens
  /home/ddjura/personal/tech/DuReef/products/cloud-coach        40.5 MB           — tokens
  /tmp/dup                                                     1.0 MB
  /tmp/dup                                                     1.0 MB

Total: 170.3 MB on disk · 457.1k tokens saved
`
	got := parseListPaths(out)
	if len(got) != 3 {
		t.Fatalf("got %d paths: %v", len(got), got)
	}
	if got[0] != "/home/ddjura/personal/tech/DuReef" {
		t.Fatalf("first path: %q", got[0])
	}
	if got[2] != "/tmp/dup" {
		t.Fatalf("dedupe failed: %v", got)
	}
}

func TestSyncAllSkippedFlags(t *testing.T) {
	res := SyncAll(Options{NoTokensave: true})
	if res.Skipped == "" {
		t.Fatal("expected skip reason")
	}

	oldLook := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("missing") }
	defer func() { lookPath = oldLook }()

	res = SyncAll(Options{})
	if !strings.Contains(res.Skipped, "not on PATH") {
		t.Fatalf("skip: %q", res.Skipped)
	}
}

func TestSyncAllDryRun(t *testing.T) {
	oldLook, oldRun := lookPath, runCmd
	defer func() { lookPath, runCmd = oldLook, oldRun }()

	lookPath = func(string) (string, error) { return "/bin/tokensave", nil }
	runCmd = func(name string, args ...string) ([]byte, []byte, error) {
		if len(args) >= 1 && args[0] == "list" {
			return []byte("  /proj/a  1 MB\n  /proj/b  2 MB\n"), nil, nil
		}
		t.Fatalf("unexpected run: %s %v", name, args)
		return nil, nil, nil
	}

	buf := &bytes.Buffer{}
	res := SyncAll(Options{DryRun: true, Out: buf})
	if len(res.Paths) != 2 {
		t.Fatalf("paths: %v", res.Paths)
	}
	out := buf.String()
	if !strings.Contains(out, "dry-run: tokensave sync /proj/a") {
		t.Fatalf("dry-run output:\n%s", out)
	}
	if !strings.Contains(out, "dry-run: tokensave sync /proj/b") {
		t.Fatalf("dry-run output:\n%s", out)
	}
}

func TestSyncAllRunsSyncPerPath(t *testing.T) {
	oldLook, oldRun := lookPath, runCmd
	defer func() { lookPath, runCmd = oldLook, oldRun }()

	lookPath = func(string) (string, error) { return "/bin/tokensave", nil }
	var mu sync.Mutex
	seen := map[string]int{}
	runCmd = func(_ string, args ...string) ([]byte, []byte, error) {
		if len(args) >= 1 && args[0] == "list" {
			return []byte("  /proj/a  1 MB\n  /proj/b  2 MB\n"), nil, nil
		}
		if len(args) >= 2 && args[0] == "sync" {
			mu.Lock()
			seen[args[1]]++
			mu.Unlock()
			return nil, nil, nil
		}
		return nil, []byte("bad"), errors.New("unexpected")
	}

	buf := &bytes.Buffer{}
	res := SyncAll(Options{Out: buf})
	if len(res.Failed) != 0 {
		t.Fatalf("failed: %v", res.Failed)
	}
	if seen["/proj/a"] != 1 || seen["/proj/b"] != 1 {
		t.Fatalf("seen: %v", seen)
	}
}
