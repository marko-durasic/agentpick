package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestGlobalsFromOSArgs(t *testing.T) {
	providers := map[string]struct{}{"claude": {}, "codex": {}}
	old := os.Args
	defer func() { os.Args = old }()

	os.Args = []string{"agentpick", "--dry-run", "--no-headroom", "claude", "hi"}
	noHR, dry := globalsFromOSArgs(providers)
	if !noHR || !dry {
		t.Fatalf("got noHR=%v dry=%v", noHR, dry)
	}

	os.Args = []string{"agentpick", "claude", "--dry-run"}
	noHR, dry = globalsFromOSArgs(providers)
	if noHR || dry {
		t.Fatalf("flags after provider must be ignored, got noHR=%v dry=%v", noHR, dry)
	}
}

func TestListSmoke(t *testing.T) {
	cmd := NewRoot()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"claude", "codex", "grok", "copilot", "agy", "why:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q in:\n%s", want, out)
		}
	}
}

func TestStripGlobalFlags(t *testing.T) {
	noHR, dry, rest := stripGlobalFlags([]string{"--dry-run", "--no-headroom", "hi", "--effort", "max"})
	if !noHR || !dry {
		t.Fatalf("flags: noHR=%v dry=%v", noHR, dry)
	}
	if strings.Join(rest, " ") != "hi --effort max" {
		t.Fatalf("rest: %v", rest)
	}
}
