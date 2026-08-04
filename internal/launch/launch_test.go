package launch

import (
	"strings"
	"testing"

	"github.com/marko-durasic/agentpick/internal/defaults"
)

func TestResolveClaudeHeadroom(t *testing.T) {
	p := defaults.Provider{
		Binary:        "claude",
		HeadroomWrap:  "claude",
		HeadroomFlags: []string{"--1m"},
		Env:           map[string]string{"ANTHROPIC_MODEL": "claude-opus-5"},
		Passthrough:   []string{"--effort", "high"},
	}
	// Force no-headroom path so CI without headroom still exercises argv.
	plan, err := Resolve(p, Options{NoHeadroom: true, ExtraArgs: []string{"--resume", "x"}})
	if err != nil {
		// claude may be missing in CI — that's OK if LookPath fails
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("Resolve: %v", err)
		}
		t.Skip("claude not installed")
	}
	if plan.UsedHeadroom {
		t.Fatal("expected no headroom")
	}
	joined := strings.Join(plan.Argv, " ")
	if !strings.Contains(joined, "--effort") || !strings.Contains(joined, "high") {
		t.Fatalf("argv missing effort: %v", plan.Argv)
	}
	if !strings.Contains(joined, "--resume") {
		t.Fatalf("argv missing extra: %v", plan.Argv)
	}
}

func TestResolveAgyNoWrap(t *testing.T) {
	p := defaults.Provider{
		Binary:      "agy",
		Passthrough: []string{"--model", "gemini-3.6-flash-high", "--effort", "high"},
	}
	plan, err := Resolve(p, Options{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			t.Skip("agy not installed")
		}
		t.Fatalf("Resolve: %v", err)
	}
	if plan.UsedHeadroom {
		t.Fatal("agy must not use headroom wrap")
	}
}

func TestResolveHeadroomArgvShape(t *testing.T) {
	if !HeadroomAvailable() {
		t.Skip("headroom not installed")
	}
	p := defaults.Provider{
		Binary:        "claude",
		HeadroomWrap:  "claude",
		HeadroomFlags: []string{"--1m"},
		Passthrough:   []string{"--effort", "high"},
	}
	plan, err := Resolve(p, Options{ExtraArgs: []string{"hello"}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !plan.UsedHeadroom {
		t.Fatal("expected headroom")
	}
	// headroom wrap claude --1m -- --effort high hello
	if len(plan.Argv) < 7 {
		t.Fatalf("short argv: %v", plan.Argv)
	}
	if plan.Argv[1] != "wrap" || plan.Argv[2] != "claude" {
		t.Fatalf("wrap shape: %v", plan.Argv)
	}
	foundSep := false
	for _, a := range plan.Argv {
		if a == "--" {
			foundSep = true
		}
	}
	if !foundSep {
		t.Fatalf("missing -- separator: %v", plan.Argv)
	}
}
