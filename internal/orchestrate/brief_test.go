package orchestrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderBriefContainsDelegateCommands(t *testing.T) {
	got := RenderBrief("cursor", "quota+orchestrator")
	for _, want := range []string{
		"session orchestrator",
		"agentpick dispatch",
		"--exclude cursor",
		"review",
		"plan",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("brief missing %q", want)
		}
	}
}

func TestExtraArgsClaudeUsesAppendSystemPrompt(t *testing.T) {
	got := ExtraArgs("claude", "/tmp/brief.md")
	if len(got) != 2 || got[0] != "--append-system-prompt" || !strings.Contains(got[1], "/tmp/brief.md") {
		t.Fatalf("got %v", got)
	}
}

func TestExtraArgsCursorUsesPrompt(t *testing.T) {
	got := ExtraArgs("cursor", "/tmp/brief.md")
	if len(got) != 1 || !strings.Contains(got[0], "/tmp/brief.md") {
		t.Fatalf("got %v", got)
	}
}

func TestWriteBrief(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTPICK_ORCHESTRATOR_BRIEF", filepath.Join(dir, "brief.md"))
	path, err := WriteBrief("claude", "test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "You are: **claude**") {
		t.Fatalf("unexpected brief:\n%s", b)
	}
}
