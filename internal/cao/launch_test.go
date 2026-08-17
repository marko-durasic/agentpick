package cao

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestProviderID(t *testing.T) {
	cases := map[string]string{
		"cursor":  "cursor_cli",
		"claude":  "claude_code",
		"codex":   "codex",
		"agy":     "antigravity_cli",
		"copilot": "copilot_cli",
	}
	for in, want := range cases {
		got, err := ProviderID(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
	if _, err := ProviderID("grok"); err == nil {
		t.Fatal("grok should be unsupported on CAO 2.4.1")
	}
	if _, err := ProviderID("ollama"); err == nil {
		t.Fatal("ollama should be rejected")
	}
}

func TestResolveLaunchArgvNoYoloLoopback(t *testing.T) {
	if _, err := exec.LookPath("cao"); err != nil {
		t.Skip("cao not on PATH")
	}
	if _, err := exec.LookPath("cao-server"); err != nil {
		t.Skip("cao-server not on PATH")
	}
	t.Setenv("AGENTPICK_CAO_HOST", "")
	t.Setenv("AGENTPICK_CAO_PORT", "")
	plan, err := Resolve(Options{Provider: "cursor", WorkDir: "/tmp/wd", BriefPath: "/tmp/brief.md"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Host != DefaultHost || plan.Port != DefaultPort {
		t.Fatalf("bind %s:%d", plan.Host, plan.Port)
	}
	joined := strings.Join(plan.LaunchArgv, " ")
	if strings.Contains(joined, "--yolo") {
		t.Fatalf("must not pass --yolo: %s", joined)
	}
	if !strings.Contains(joined, "--provider cursor_cli") {
		t.Fatalf("missing provider: %s", joined)
	}
	if !strings.Contains(joined, "--working-directory /tmp/wd") {
		t.Fatalf("missing wd: %s", joined)
	}
	if !strings.Contains(joined, "--agents agentpick_supervisor") {
		t.Fatalf("missing profile: %s", joined)
	}
	if !strings.Contains(joined, "--session-name agentpick") {
		t.Fatalf("missing session name: %s", joined)
	}
	if !strings.Contains(joined, "/start") || strings.Contains(joined, "no /start") {
		t.Fatalf("launch prompt should keep slash commands: %s", joined)
	}
	server := strings.Join(plan.ServerArgv, " ")
	if !strings.Contains(server, "--host 127.0.0.1") || !strings.Contains(server, "--port 9889") {
		t.Fatalf("server bind: %s", server)
	}
}

func TestResolveHostPortRejectsReservedAndNonLoopback(t *testing.T) {
	t.Setenv("AGENTPICK_CAO_HOST", "0.0.0.0")
	if _, _, err := ResolveHostPort(); err == nil {
		t.Fatal("0.0.0.0 must be rejected")
	}
	t.Setenv("AGENTPICK_CAO_HOST", "127.0.0.1")
	t.Setenv("AGENTPICK_CAO_PORT", "8787")
	if _, _, err := ResolveHostPort(); err == nil {
		t.Fatal("8787 must be rejected")
	}
	t.Setenv("AGENTPICK_CAO_PORT", "8788")
	if _, _, err := ResolveHostPort(); err == nil {
		t.Fatal("8788 must be rejected")
	}
}

func TestResolveGrokError(t *testing.T) {
	_, err := Resolve(Options{Provider: "grok", WorkDir: os.TempDir()})
	if err == nil {
		t.Fatal("expected grok error")
	}
}
