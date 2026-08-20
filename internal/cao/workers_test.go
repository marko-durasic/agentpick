package cao

import (
	"strings"
	"testing"

	"github.com/marko-durasic/agentpick/internal/quota"
	"github.com/marko-durasic/agentpick/internal/route"
)

func TestPickCAOPeerSkipsSupervisorGrokAndEmptyQuota(t *testing.T) {
	zero := 0.0
	ranked := []route.Candidate{
		{Provider: "cursor"},
		{Provider: "grok"},
		{Provider: "codex", Quota: quota.Snapshot{RemainingPct: &zero}},
		{Provider: "agy"},
	}
	got := PickCAOPeer(ranked, "cursor")
	if got != "agy" {
		t.Fatalf("got %q want agy", got)
	}
}

func TestPickRoutedPeerIncludesGrokViaDispatch(t *testing.T) {
	ranked := []route.Candidate{
		{Provider: "cursor"},
		{Provider: "grok"},
		{Provider: "agy"},
	}
	if got := PickCAOPeer(ranked, "cursor"); got != "agy" {
		t.Fatalf("CAO peer %q want agy", got)
	}
	if got := PickRoutedPeer(ranked, "cursor", false, true); got != "grok" {
		t.Fatalf("routed peer excluding self %q want grok", got)
	}
	if got := PickRoutedPeer(ranked, "cursor", false, false); got != "cursor" {
		t.Fatalf("routed peer allowing self clone %q want cursor", got)
	}
	tiny := []route.Candidate{{Provider: "claude"}, {Provider: "ollama"}}
	if got := PickRoutedPeer(tiny, "cursor", true, true); got != "ollama" {
		t.Fatalf("tiny %q want ollama", got)
	}
	if got := PickRoutedPeer(tiny, "cursor", false, true); got != "claude" {
		t.Fatalf("non-tiny must skip ollama, got %q", got)
	}
}

func TestSupervisorMarkdownKeepsSlashCommandsAndNoManualRoute(t *testing.T) {
	body := supervisorMarkdown("cursor", Workers{
		Implement: Worker{Role: "implement", Provider: "agy", CAOProvider: "antigravity_cli", Profile: DevProfile, Via: ViaCAO},
		Review:    Worker{Role: "review", Provider: "claude", CAOProvider: "claude_code", Profile: ReviewProfile, Via: ViaCAO},
		Tiny:      Worker{Provider: "ollama", Role: "tiny", Via: ViaDispatch},
		Extra: []Worker{
			{Role: "peer", Provider: "copilot", CAOProvider: "copilot_cli", Profile: "agentpick_copilot", Via: ViaCAO},
			{Role: "peer", Provider: "grok", Via: ViaDispatch},
		},
	}, "/tmp/ws")
	for _, want := range []string{DevProfile, ReviewProfile, "agy", "claude", "full-featured", "/start", "/wrap-up", "agentpick dispatch", "--prefer ollama", "agentpick_copilot", "--prefer grok", "every healthy installed CLI", "role/model rank", "Ready panes are available, not busy", "not a one-time 1:1", "at most **4 active specialist tasks**"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !strings.Contains(body, "send_message") {
		t.Fatal("must tell supervisor to send_message pre-spawned workers")
	}
	if !strings.Contains(body, "Divide and conquer") {
		t.Fatal("must tell supervisor to divide and conquer")
	}
	if !strings.Contains(body, "another instance of yourself") {
		t.Fatal("must allow send_message to a second supervisor instance")
	}
	clone := supervisorMarkdown("cursor", Workers{
		Implement: Worker{Role: "implement", Provider: "cursor", CAOProvider: "cursor_cli", Profile: DevProfile, Via: ViaCAO, Why: "80% billing-period"},
		Review:    Worker{Role: "review", Provider: "claude", CAOProvider: "claude_code", Profile: ReviewProfile, Via: ViaCAO},
	}, "/tmp/ws")
	if !strings.Contains(clone, "second instance of you") {
		t.Fatal("when implement is cursor, supervisor must treat agentpick_dev as a clone worker")
	}
	worker := workerMarkdown(DevProfile, "developer", "cursor_cli", "cursor")
	if !strings.Contains(worker, "send_message the supervisor") {
		t.Fatal("workers must send_message results back")
	}
	if strings.Contains(body, "those exist only") || strings.Contains(body, "There is **no** `/start`") {
		t.Fatal("must not tell the supervisor slash commands are missing")
	}
	if strings.Contains(strings.ToLower(body), "run `agentpick route`") && !strings.Contains(body, "Never ask") {
		t.Fatal("must tell supervisor not to ask for route")
	}
}

func TestDispatchCmdGrok(t *testing.T) {
	w := Worker{Provider: "grok", Role: "implement", Via: ViaDispatch}
	got := w.DispatchCmd("/proj")
	if !strings.Contains(got, "--prefer grok") || !strings.Contains(got, "--dir /proj") {
		t.Fatalf("got %q", got)
	}
}

func TestSupervisorProfileAllowsExecuteBashAndWorkerHasNoAllowedTools(t *testing.T) {
	body := supervisorMarkdown("claude", Workers{
		Implement: Worker{Role: "implement", Provider: "agy", CAOProvider: "antigravity_cli", Profile: DevProfile, Via: ViaCAO},
	}, "/tmp/ws")
	if !strings.Contains(body, "allowedTools:") {
		t.Fatal("supervisor profile must emit allowedTools so CAO does not fall back to ROLE_TOOL_DEFAULTS")
	}
	if !strings.Contains(body, `"execute_bash"`) {
		t.Fatal("supervisor needs execute_bash to run agentpick dispatch commands")
	}
	if strings.Contains(body, `"fs_write"`) {
		t.Fatal("supervisor must not get fs_write; it coordinates, it does not implement")
	}
	if !strings.Contains(body, "This **is** the `claude` CLI") {
		t.Fatal("supervisor prompt must name its own CLI, not hardcode Cursor")
	}
	worker := workerMarkdown(DevProfile, "developer", "cursor_cli", "cursor")
	if strings.Contains(worker, "allowedTools:") {
		t.Fatal("worker profiles must keep CAO role defaults, not carry allowedTools")
	}
}

func TestMaxActiveWorkersIsConfigurableAndBounded(t *testing.T) {
	t.Setenv("AGENTPICK_MAX_ACTIVE_AGENTS", "")
	if got := maxActiveWorkers(); got != DefaultMaxActive {
		t.Fatalf("default=%d want %d", got, DefaultMaxActive)
	}
	t.Setenv("AGENTPICK_MAX_ACTIVE_AGENTS", "2")
	if got := maxActiveWorkers(); got != 2 {
		t.Fatalf("configured=%d want 2", got)
	}
	t.Setenv("AGENTPICK_MAX_ACTIVE_AGENTS", "99")
	if got := maxActiveWorkers(); got != DefaultMaxActive {
		t.Fatalf("upper bound=%d want %d", got, DefaultMaxActive)
	}
	t.Setenv("AGENTPICK_MAX_ACTIVE_AGENTS", "0")
	if got := maxActiveWorkers(); got != 1 {
		t.Fatalf("lower bound=%d want 1", got)
	}
}
