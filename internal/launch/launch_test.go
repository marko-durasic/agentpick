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

func TestResolveGrokNativeNoWrap(t *testing.T) {
	// Embedded defaults must not set headroom_wrap for grok.
	reg, err := defaults.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := reg.Get("grok")
	if !ok {
		t.Fatal("missing grok provider")
	}
	if p.HeadroomWrap != "" {
		t.Fatalf("grok must launch native (empty headroom_wrap), got %q", p.HeadroomWrap)
	}
	plan, err := Resolve(p, Options{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			t.Skip("grok not installed")
		}
		t.Fatalf("Resolve: %v", err)
	}
	if plan.UsedHeadroom {
		t.Fatal("grok must not use headroom wrap")
	}
	joined := strings.Join(plan.Argv, " ")
	if strings.Contains(joined, "wrap") {
		t.Fatalf("grok argv must be native CLI, got %v", plan.Argv)
	}
}

func TestBuildHeadroomArgvPortLongForm(t *testing.T) {
	argv := BuildHeadroomArgv("/usr/bin/headroom", "claude", 8788,
		[]string{"--1m"},
		[]string{"--effort", "high"},
		[]string{"hello"},
	)
	// headroom wrap claude --port 8788 --1m -- --effort high hello
	wantPrefix := []string{"/usr/bin/headroom", "wrap", "claude", "--port", "8788", "--1m", "--"}
	if len(argv) < len(wantPrefix) {
		t.Fatalf("short argv: %v", argv)
	}
	for i, w := range wantPrefix {
		if argv[i] != w {
			t.Fatalf("argv[%d]=%q want %q; full=%v", i, argv[i], w, argv)
		}
	}
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, " -p ") || strings.HasPrefix(joined, "-p ") {
		t.Fatalf("must not use bare -p: %v", argv)
	}
	if !strings.Contains(joined, "--effort high") {
		t.Fatalf("missing passthrough: %v", argv)
	}
	if argv[len(argv)-1] != "hello" {
		t.Fatalf("missing extra: %v", argv)
	}
}

func TestBuildHeadroomArgvRemapsOAuthPort(t *testing.T) {
	argv := BuildHeadroomArgv("headroom", "codex", OAuthCallbackPort, nil, nil, nil)
	// Must not pin Headroom to Cursor OAuth :8787
	found := false
	for i := 0; i < len(argv)-1; i++ {
		if argv[i] == "--port" {
			if argv[i+1] != "8788" {
				t.Fatalf("oauth port remap failed: %v", argv)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing --port: %v", argv)
	}
}

func TestBuildHeadroomArgvStripsYAMLPortFlags(t *testing.T) {
	argv := BuildHeadroomArgv("headroom", "copilot", 8788,
		[]string{"--subscription", "-p", "9999", "--port", "7777"},
		[]string{"--model", "gpt-5.6-luna"},
		nil,
	)
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "9999") || strings.Contains(joined, "7777") {
		t.Fatalf("yaml port flags must be stripped: %v", argv)
	}
	if !strings.Contains(joined, "--port 8788") {
		t.Fatalf("expected injected --port 8788: %v", argv)
	}
	if !strings.Contains(joined, "--subscription") {
		t.Fatalf("expected --subscription kept: %v", argv)
	}
}

func TestResolveHeadroomPortDefaultAndEnv(t *testing.T) {
	t.Setenv("DUREEF_HEADROOM_PORT", "")
	t.Setenv("HEADROOM_PORT", "")
	if got := ResolveHeadroomPort(0); got != DefaultHeadroomPort {
		t.Fatalf("default port=%d want %d", got, DefaultHeadroomPort)
	}
	t.Setenv("DUREEF_HEADROOM_PORT", "8790")
	if got := ResolveHeadroomPort(0); got != 8790 {
		t.Fatalf("DUREEF_HEADROOM_PORT=%d want 8790", got)
	}
	// Explicit wins over env
	if got := ResolveHeadroomPort(8791); got != 8791 {
		t.Fatalf("explicit=%d want 8791", got)
	}
	// :8787 always remapped
	if got := ResolveHeadroomPort(8787); got != DefaultHeadroomPort {
		t.Fatalf("8787 remap=%d want %d", got, DefaultHeadroomPort)
	}
	t.Setenv("DUREEF_HEADROOM_PORT", "8787")
	if got := ResolveHeadroomPort(0); got != DefaultHeadroomPort {
		t.Fatalf("env 8787 remap=%d want %d", got, DefaultHeadroomPort)
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
	if plan.HeadroomPort != DefaultHeadroomPort && plan.HeadroomPort != ResolveHeadroomPort(0) {
		t.Fatalf("HeadroomPort=%d", plan.HeadroomPort)
	}
	// headroom wrap claude --port N --1m -- --effort high hello
	if len(plan.Argv) < 8 {
		t.Fatalf("short argv: %v", plan.Argv)
	}
	if plan.Argv[1] != "wrap" || plan.Argv[2] != "claude" {
		t.Fatalf("wrap shape: %v", plan.Argv)
	}
	if plan.Argv[3] != "--port" {
		t.Fatalf("expected long-form --port after tool: %v", plan.Argv)
	}
	foundSep := false
	for _, a := range plan.Argv {
		if a == "--" {
			foundSep = true
		}
		if a == "-p" {
			t.Fatalf("bare -p must not appear: %v", plan.Argv)
		}
	}
	if !foundSep {
		t.Fatalf("missing -- separator: %v", plan.Argv)
	}
}
