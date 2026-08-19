package cao

import (
	"strings"
	"testing"
)

func TestTmuxSessionNameOK(t *testing.T) {
	if !tmuxSessionNameOK("cao-agentpick-20260819-023107-18cd140b91f0cf82") {
		t.Fatal("typical CAO session must be accepted")
	}
	if tmuxSessionNameOK("") || tmuxSessionNameOK("foo; rm -rf /") || tmuxSessionNameOK("a b") {
		t.Fatal("must reject empty and shell metacharacters")
	}
}

func TestTmuxScrollArgv(t *testing.T) {
	got, err := tmuxScrollArgv("cao-agentpick")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 commands, got %d", len(got))
	}
	mouse := strings.Join(got[0], " ")
	hist := strings.Join(got[1], " ")
	if mouse != "tmux set-option -t cao-agentpick mouse on" {
		t.Fatalf("mouse: %s", mouse)
	}
	if !strings.Contains(hist, "history-limit "+ScrollHistoryLimit) {
		t.Fatalf("history: %s", hist)
	}
	if _, err := tmuxScrollArgv("bad;name"); err == nil {
		t.Fatal("expected unsafe name error")
	}
}
