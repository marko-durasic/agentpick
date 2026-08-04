package defaults

import (
	"testing"
)

func TestLoad(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Version < 1 {
		t.Fatalf("version: got %d", reg.Version)
	}
	want := []string{"agy", "claude", "codex", "copilot", "grok"}
	got := reg.Names()
	if len(got) != len(want) {
		t.Fatalf("names: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names[%d]: got %q want %q", i, got[i], want[i])
		}
	}
	claude, ok := reg.Get("claude")
	if !ok {
		t.Fatal("missing claude")
	}
	if claude.Binary != "claude" || claude.HeadroomWrap != "claude" {
		t.Fatalf("claude: %+v", claude)
	}
	if claude.Env["ANTHROPIC_MODEL"] != "claude-opus-5" {
		t.Fatalf("claude env: %v", claude.Env)
	}
	agy, ok := reg.Get("agy")
	if !ok {
		t.Fatal("missing agy")
	}
	if agy.HeadroomWrap != "" {
		t.Fatalf("agy should not headroom-wrap, got %q", agy.HeadroomWrap)
	}
}
