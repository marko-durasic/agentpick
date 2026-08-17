package cao

import (
	"strings"
	"testing"
)

func TestSpawnTerminalURLLoopback(t *testing.T) {
	u := spawnTerminalURL("http://127.0.0.1:9889", "agentpick", DevProfile, "claude_code", "/ws")
	if strings.Contains(u, "0.0.0.0") || strings.Contains(u, "--yolo") {
		t.Fatalf("unsafe url %s", u)
	}
	if !strings.Contains(u, "agent_profile="+DevProfile) || !strings.Contains(u, "provider=claude_code") {
		t.Fatalf("missing query %s", u)
	}
	if !strings.Contains(u, "defer_init=true") {
		t.Fatalf("want defer_init %s", u)
	}
}

func TestCAOAssignableSkipsDispatch(t *testing.T) {
	got := caoAssignable(Workers{
		Implement: Worker{Via: ViaDispatch, Provider: "grok", Profile: DevProfile, CAOProvider: ""},
		Review:    Worker{Via: ViaCAO, Provider: "claude", Profile: ReviewProfile, CAOProvider: "claude_code"},
		Tiny:      Worker{Via: ViaDispatch, Provider: "ollama", Profile: ""},
	})
	if len(got) != 1 || got[0].Profile != ReviewProfile {
		t.Fatalf("got %+v", got)
	}
}

func TestCAOAssignableAllowsSupervisorClone(t *testing.T) {
	got := caoAssignable(Workers{
		Implement: Worker{Via: ViaCAO, Provider: "cursor", Profile: DevProfile, CAOProvider: "cursor_cli"},
		Review:    Worker{Via: ViaCAO, Provider: "claude", Profile: ReviewProfile, CAOProvider: "claude_code"},
	})
	if len(got) != 2 {
		t.Fatalf("second cursor pane + review should both spawn, got %+v", got)
	}
}
