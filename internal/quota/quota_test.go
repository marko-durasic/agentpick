package quota

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatLabel(t *testing.T) {
	pct := 72.4
	s := Snapshot{Provider: "cursor", RemainingPct: &pct, Source: "cursor-api"}
	if got := FormatLabel(s); got != "72% left" {
		t.Fatalf("cursor: got %q", got)
	}
	s = Snapshot{Provider: "claude", RemainingPct: &pct, Source: "claude-usage"}
	if got := FormatLabel(s); got != "week 72% left" {
		t.Fatalf("claude: got %q", got)
	}
	if got := FormatLabel(Snapshot{Provider: "codex"}); got != "—" {
		t.Fatalf("unknown: got %q", got)
	}
}

func TestParseClaudeWeekUsed(t *testing.T) {
	out := `Current session: 100% used · resets Aug 15
Current week (all models): 58% used · resets Aug 18
`
	pct, ok := parseClaudeWeekUsed(out)
	if !ok || pct != 58 {
		t.Fatalf("got pct=%v ok=%v", pct, ok)
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
		"cursor": {Provider: "cursor", RemainingPct: &pct, Label: "80% left", Source: "cursor-api"},
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
		return []byte("Current week (all models): 40% used · resets tomorrow\n"), nil, nil
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
	if snaps["codex"].Label != "—" {
		t.Fatalf("codex should be unknown: %+v", snaps["codex"])
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
