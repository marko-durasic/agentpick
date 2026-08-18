package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseTryAgainAtStopsAtNewline(t *testing.T) {
	out := "try again at Aug 20th, 2026 4:10 PM.\nERROR: You've hit your usage limit."
	got := parseTryAgainAt(out)
	if strings.Contains(got, "ERROR") || !strings.Contains(got, "Aug 20") {
		t.Fatalf("got %q", got)
	}
}

func TestParseCodexExecOutLimit(t *testing.T) {
	out := `ERROR: You've hit your usage limit. Upgrade to Pro, visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 20th, 2026 4:10 PM.`
	s, ok := parseCodexExecOut(out)
	if !ok || s.RemainingPct == nil || *s.RemainingPct != 0 {
		t.Fatalf("snap: ok=%v %+v", ok, s)
	}
	if !strings.Contains(s.ResetHint, "Aug 20") {
		t.Fatalf("reset: %q", s.ResetHint)
	}
}

func TestParseCodexAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"plan_type":"plus",
			"rate_limit":{"limit_reached":true,"primary_window":{"used_percent":100,"limit_window_seconds":604800,"reset_at":1787213408}},
			"credits":{"balance":"0"}
		}`))
	}))
	t.Cleanup(srv.Close)

	oldDo, oldPath := codexHTTPDo, codexAuthPath
	t.Cleanup(func() { codexHTTPDo = oldDo; codexAuthPath = oldPath })
	codexHTTPDo = func(req *http.Request) (*http.Response, error) {
		req2, err := http.NewRequestWithContext(req.Context(), req.Method, srv.URL, nil)
		if err != nil {
			return nil, err
		}
		req2.Header = req.Header
		return http.DefaultClient.Do(req2)
	}
	dir := t.TempDir()
	auth := dir + "/auth.json"
	if err := os.WriteFile(auth, []byte(`{"tokens":{"access_token":"t","account_id":"a"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	codexAuthPath = func() string { return auth }

	s, ok := probeCodexAPI(context.Background())
	if !ok || s.RemainingPct == nil || *s.RemainingPct != 0 {
		t.Fatalf("api: ok=%v %+v", ok, s)
	}
	if s.Window != "week" {
		t.Fatalf("window: %q", s.Window)
	}
}

func TestParseCopilotMonthlyExhausted(t *testing.T) {
	out := "You have exceeded your monthly quota (Request ID: abc)\n\nAI Credits 0 (7s)\n"
	s := parseCopilotOut(out)
	if s.RemainingPct == nil || *s.RemainingPct != 0 || s.Window != "month" {
		t.Fatalf("%+v", s)
	}
	if !strings.Contains(s.Detail, "AI credits 0") {
		t.Fatalf("detail: %q", s.Detail)
	}
}

func TestParseCopilotBYOKNoModelUnavailable(t *testing.T) {
	out := "BYOK providers require an explicit model. Run `copilot help providers` for configuration details."
	s := parseCopilotOut(out)
	if s.UnavailableReason == "" {
		t.Fatalf("byok refusal must be unavailable: %+v", s)
	}
	if strings.Contains(s.Label, "available") {
		t.Fatalf("label must not advertise availability: %q", s.Label)
	}
}

func TestParseCopilotProviderAuthFailedUnavailable(t *testing.T) {
	out := "Authentication failed with provider at http://127.0.0.1:8788 (HTTP 401)."
	s := parseCopilotOut(out)
	if s.UnavailableReason == "" {
		t.Fatalf("auth failure must be unavailable: %+v", s)
	}
	if strings.Contains(s.Label, "available") {
		t.Fatalf("label must not advertise availability: %q", s.Label)
	}
}

func TestParseCopilotMonthlyStillWinsOverNewBranches(t *testing.T) {
	out := "You have exceeded your monthly quota (Request ID: abc)\n\nAI Credits 0 (7s)\n"
	s := parseCopilotOut(out)
	if s.RemainingPct == nil || *s.RemainingPct != 0 || s.Window != "month" {
		t.Fatalf("%+v", s)
	}
}

func TestParseGrokAvailable(t *testing.T) {
	s := parseLimitOrAvailable("grok", "grok-cli", "PONG\n")
	if s.RemainingPct != nil || !strings.Contains(s.Label, "available") {
		t.Fatalf("%+v", s)
	}
}

func TestParseAgyTimeoutInconclusive(t *testing.T) {
	s := parseLimitOrAvailable("agy", "agy-cli", "Error: timeout waiting for response\n")
	if s.RemainingPct != nil || !strings.Contains(s.Label, "inconclusive") {
		t.Fatalf("%+v", s)
	}
}

func TestParseLimitEmptyIsAvailable(t *testing.T) {
	s := parseLimitOrAvailable("agy", "agy-cli", "  \n")
	if s.RemainingPct != nil || !strings.Contains(s.Label, "available") {
		t.Fatalf("empty print probe must not look like no quota: %+v", s)
	}
}

func TestParseAgyUsagePanelGeminiWeekly(t *testing.T) {
	panel := `
Models & Quota
Account: you@gmail.com

GEMINI MODELS
Gemini Flash, Gemini Pro
Weekly Limit Remaining
████████████████ 96.63%
97% remaining · Refreshes in 49h 21m
Five Hour Limit Remaining
████████████████ 100.00%
Quota available.

CLAUDE AND GPT MODELS
Claude Opus, Claude Sonnet, GPT-OSS
Weekly Limit Remaining
████████████████ 100.00%
Quota available.
Five Hour Limit Remaining
████████████████ 100.00%
Quota available.
`
	s, ok := parseAgyUsagePanel(panel)
	if !ok || s.RemainingPct == nil {
		t.Fatalf("ok=%v %+v", ok, s)
	}
	if *s.RemainingPct < 96.6 || *s.RemainingPct > 97.1 {
		t.Fatalf("gemini weekly remaining %+v", s)
	}
	if s.Window != "week" {
		t.Fatalf("window %q", s.Window)
	}
	if !strings.Contains(s.ResetHint, "49h") {
		t.Fatalf("reset %q", s.ResetHint)
	}
}

func TestParseAgyUsageJSONGeminiWeekly(t *testing.T) {
	raw := `{
	  "groups": [
	    {
	      "name": "GEMINI MODELS",
	      "buckets": [
	        {"kind":"weekly","label":"Weekly Limit","remainingFraction":0.9663,"resetsInSeconds":177660,"available":false},
	        {"kind":"5h","label":"Five Hour Limit","remainingFraction":1,"available":true}
	      ]
	    }
	  ]
	}`
	s, ok := parseAgyUsageJSON(raw)
	if !ok || s.RemainingPct == nil || *s.RemainingPct < 96.6 || *s.RemainingPct > 96.7 {
		t.Fatalf("ok=%v %+v", ok, s)
	}
	if !strings.Contains(s.Detail, "5h") {
		t.Fatalf("detail %q", s.Detail)
	}
}
