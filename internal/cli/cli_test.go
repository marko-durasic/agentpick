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

	os.Args = []string{"agentpick", "--dry-run", "--no-headroom", "--no-tokensave", "claude", "hi"}
	noHR, noTS, dry := globalsFromOSArgs(providers)
	if !noHR || !noTS || !dry {
		t.Fatalf("got noHR=%v noTS=%v dry=%v", noHR, noTS, dry)
	}

	os.Args = []string{"agentpick", "claude", "--dry-run"}
	noHR, noTS, dry = globalsFromOSArgs(providers)
	if noHR || noTS || dry {
		t.Fatalf("flags after provider must be ignored, got noHR=%v noTS=%v dry=%v", noHR, noTS, dry)
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
	for _, want := range []string{"claude", "codex", "cursor", "grok", "copilot", "agy", "why:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list missing %q in:\n%s", want, out)
		}
	}
}

func TestStripGlobalFlags(t *testing.T) {
	noHR, noTS, dry, rest := stripGlobalFlags([]string{"--dry-run", "--no-headroom", "--no-tokensave", "hi", "--effort", "max"})
	if !noHR || !noTS || !dry {
		t.Fatalf("flags: noHR=%v noTS=%v dry=%v", noHR, noTS, dry)
	}
	if strings.Join(rest, " ") != "hi --effort max" {
		t.Fatalf("rest: %v", rest)
	}
}
