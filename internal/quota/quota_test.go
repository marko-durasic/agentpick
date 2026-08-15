package quota

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatLabel(t *testing.T) {
	pct := 72.4
	s := Snapshot{Provider: "cursor", RemainingPct: &pct, Window: "billing-period", Source: "cursor-api", ResetHint: "resets Sep 1"}
	if got := FormatLabel(s); got != "billing-period 72% left · resets Sep 1" {
		t.Fatalf("cursor: got %q", got)
	}
	s = Snapshot{Provider: "claude", RemainingPct: &pct, Window: "week", Source: "claude-usage", ResetHint: "resets Tue", Detail: "session 0% left"}
	if got := FormatLabel(s); !strings.Contains(got, "week 72% left") || !strings.Contains(got, "session 0% left") {
		t.Fatalf("claude week: got %q", got)
	}
	s = Snapshot{Provider: "claude", RemainingPct: &pct, Window: "session", Source: "claude-history"}
	if got := FormatLabel(s); got != "session 72% left" {
		t.Fatalf("claude session: got %q", got)
	}
	s = Snapshot{Provider: "codex", UnavailableReason: "no public quota API (ChatGPT usage blocked)"}
	if got := FormatLabel(s); !strings.Contains(got, "no public quota API") {
		t.Fatalf("unknown: got %q", got)
	}
}

func TestFormatPickerRow(t *testing.T) {
	pct := 42.0
	row := FormatPickerRow(2, "claude", "Opus 5 · 1M · effort high", Snapshot{
		RemainingPct: &pct,
		Window:       "week",
		ResetHint:    "resets Aug 18",
		Source:       "claude-usage",
	})
	if !strings.Contains(row, "2)") || !strings.Contains(row, "claude") || !strings.Contains(row, "week 42% left") {
		t.Fatalf("row: %q", row)
	}
}

func TestParseClaudeWeekUsed(t *testing.T) {
	out := `Current session: 100% used · resets Aug 15, 11:20am (Asia/Taipei)
Current week (all models): 58% used · resets Aug 18, 8am (Asia/Taipei)
`
	pct, ok := parseClaudeWeekUsed(out)
	if !ok || pct != 58 {
		t.Fatalf("got pct=%v ok=%v", pct, ok)
	}
	sess, ok := parseClaudeSessionUsed(out)
	if !ok || sess != 100 {
		t.Fatalf("session: got pct=%v ok=%v", sess, ok)
	}
	reset := cleanReset(parseReset(weekResetRE, out))
	if !strings.Contains(reset, "resets") || !strings.Contains(reset, "Aug 18") {
		t.Fatalf("reset: %q", reset)
	}
}

func TestSuggest(t *testing.T) {
	c, cl := 72.0, 42.0
	snaps := map[string]Snapshot{
		"cursor": {Provider: "cursor", RemainingPct: &c},
		"claude": {Provider: "claude", RemainingPct: &cl},
		"codex":  {Provider: "codex"},
	}
	if got := Suggest(snaps, []string{"claude", "codex", "cursor"}); got != "cursor" {
		t.Fatalf("suggest: got %q", got)
	}
	if got := Suggest(map[string]Snapshot{"cursor": {RemainingPct: &c}}, []string{"cursor"}); got != "" {
		t.Fatalf("need >=2 known, got %q", got)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTPICK_CACHE_DIR", dir)
	now := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	pct := 80.0
	saveCache(map[string]Snapshot{
		"cursor": {Provider: "cursor", RemainingPct: &pct, Label: "billing-period 80% left", Source: "cursor-api", Window: "billing-period"},
	}, now)
	got, ok := loadCache(now.Add(30 * time.Second))
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got["cursor"].RemainingPct == nil || *got["cursor"].RemainingPct != 80 {
		t.Fatalf("cached: %+v", got["cursor"])
	}
	if _, ok := loadCache(now.Add(CacheTTL + time.Second)); ok {
		t.Fatal("expected cache miss after TTL")
	}
}

func TestSaveCacheKeepsGoodOnFailedProbe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTPICK_CACHE_DIR", dir)
	now := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	pct := 42.0
	saveCache(map[string]Snapshot{
		"claude": {Provider: "claude", RemainingPct: &pct, Label: "week 42% left", Source: "claude-usage"},
	}, now)
	saveCache(map[string]Snapshot{
		"claude": {Provider: "claude", Label: FormatLabel(Snapshot{UnavailableReason: "failed"}), Source: "unknown", Err: "signal: killed"},
	}, now.Add(time.Second))
	got, ok := loadCache(now.Add(2 * time.Second))
	if !ok || got["claude"].RemainingPct == nil || *got["claude"].RemainingPct != 42 {
		t.Fatalf("should keep good claude: ok=%v %+v", ok, got["claude"])
	}
}

func TestDerivePercentUsed(t *testing.T) {
	limit := 1000.0
	remaining := 250.0
	p := &planUsageRaw{Limit: &limit, Remaining: &remaining}
	pct := derivePercentUsed(p)
	if pct == nil || *pct != 75 {
		t.Fatalf("got %v", pct)
	}
}

func TestFetchAllUsesClaudeProbe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTPICK_CACHE_DIR", dir)
	t.Setenv("AGENTPICK_CURSOR_STATE_DB", filepath.Join(dir, "missing.vscdb"))

	oldLook, oldRun := claudeLookPath, claudeRun
	t.Cleanup(func() { claudeLookPath = oldLook; claudeRun = oldRun })
	claudeLookPath = func(string) (string, error) { return "/bin/claude", nil }
	claudeRun = func(ctx context.Context, bin string, args ...string) ([]byte, []byte, error) {
		return []byte("Current week (all models): 40% used · resets Aug 18, 8am\n"), nil, nil
	}

	snaps := FetchAll(context.Background(), FetchOptions{
		Providers: []string{"claude", "codex"},
		SkipCache: true,
		Timeout:   2 * time.Second,
	})
	cl := snaps["claude"]
	if cl.RemainingPct == nil || *cl.RemainingPct != 60 {
		t.Fatalf("claude snap: %+v", cl)
	}
	if !strings.Contains(cl.Label, "week") {
		t.Fatalf("label: %q", cl.Label)
	}
	if !strings.Contains(snaps["codex"].Label, "no public") {
		t.Fatalf("codex should explain unavailability: %+v", snaps["codex"])
	}
}

func TestClaudeHistoryFallback(t *testing.T) {
	dir := t.TempDir()
	hist := filepath.Join(dir, "plan-usage-history.json")
	if err := os.WriteFile(hist, []byte(`{"version":2,"samples":[{"t":1,"u":{"fh":90,"sd":20}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldPath, oldLook := claudeHistoryPath, claudeLookPath
	t.Cleanup(func() { claudeHistoryPath = oldPath; claudeLookPath = oldLook })
	claudeHistoryPath = func() string { return hist }
	claudeLookPath = func(string) (string, error) { return "", os.ErrNotExist }

	s := probeClaude(context.Background())
	if s.RemainingPct == nil || *s.RemainingPct != 10 {
		t.Fatalf("history fallback: %+v", s)
	}
	if !strings.Contains(s.Label, "session 10% left") {
		t.Fatalf("label: %q", s.Label)
	}
}

func TestBuildWorkosSession(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"auth0|abc"}`))
	token := "hdr." + payload + ".sig"
	sess := buildWorkosSessionCookie(token)
	if sess == nil || sess.userID != "abc" {
		t.Fatalf("session: %+v", sess)
	}
}

func TestDefaultTimeoutAllowsClaudeUsage(t *testing.T) {
	if DefaultTimeout < 6*time.Second {
		t.Fatalf("DefaultTimeout too short for claude /usage: %v", DefaultTimeout)
	}
}

func TestFormatLegend(t *testing.T) {
	leg := FormatLegend(map[string]Snapshot{})
	for _, want := range []string{"week", "session", "billing-period", "Claude"} {
		if !strings.Contains(leg, want) {
			t.Fatalf("legend missing %q:\n%s", want, leg)
		}
	}
}
