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
	want := []string{"agy", "claude", "codex", "copilot", "cursor", "grok", "ollama"}
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
	cursor, ok := reg.Get("cursor")
	if !ok {
		t.Fatal("missing cursor")
	}
	if cursor.Binary != "cursor-agent" || cursor.HeadroomWrap != "" {
		t.Fatalf("cursor: %+v", cursor)
	}
	if len(cursor.Passthrough) < 2 || cursor.Passthrough[0] != "--model" || cursor.Passthrough[1] != "auto" {
		t.Fatalf("cursor passthrough: %v", cursor.Passthrough)
	}
	agy, ok := reg.Get("agy")
	if !ok {
		t.Fatal("missing agy")
	}
	if agy.HeadroomWrap != "" {
		t.Fatalf("agy should not headroom-wrap, got %q", agy.HeadroomWrap)
	}
	grok, ok := reg.Get("grok")
	if !ok {
		t.Fatal("missing grok")
	}
	if grok.HeadroomWrap != "" {
		t.Fatalf("grok must launch native (empty headroom_wrap), got %q", grok.HeadroomWrap)
	}
	ollama, ok := reg.Get("ollama")
	if !ok {
		t.Fatal("missing ollama")
	}
	if ollama.Binary != "ollama" || ollama.HeadroomWrap != "" {
		t.Fatalf("ollama: %+v", ollama)
	}
	if len(ollama.Passthrough) < 2 || ollama.Passthrough[0] != "run" || ollama.Passthrough[1] != "qwen3.5:4b" {
		t.Fatalf("ollama passthrough: %v", ollama.Passthrough)
	}
}

func TestRoleOrders(t *testing.T) {
	reg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reg.Version < 6 {
		t.Fatalf("expected version >= 6, got %d", reg.Version)
	}
	order := reg.RoleOrder("review")
	if len(order) < 2 || order[0] != "codex" {
		t.Fatalf("review order: %v", order)
	}
	claude, ok := reg.Get("claude")
	if !ok || len(claude.Roles) == 0 {
		t.Fatal("claude roles missing")
	}
}
