package quota

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const codexUsageURL = "https://chatgpt.com/backend-api/codex/usage"

type codexAuthFile struct {
	Tokens *struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

type codexUsageResponse struct {
	PlanType  string `json:"plan_type"`
	RateLimit *struct {
		Allowed      bool `json:"allowed"`
		LimitReached bool `json:"limit_reached"`
		PrimaryWindow *struct {
			UsedPercent        float64 `json:"used_percent"`
			LimitWindowSeconds float64 `json:"limit_window_seconds"`
			ResetAfterSeconds  float64 `json:"reset_after_seconds"`
			ResetAt            int64   `json:"reset_at"`
		} `json:"primary_window"`
	} `json:"rate_limit"`
	Credits *struct {
		Balance string `json:"balance"`
	} `json:"credits"`
}

var (
	codexAuthPath = defaultCodexAuthPath
	codexHTTPDo   = defaultHTTPDo
)

func defaultCodexAuthPath() string {
	if p := os.Getenv("AGENTPICK_CODEX_AUTH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".codex", "auth.json")
}

func probeCodex(ctx context.Context) Snapshot {
	if s, ok := probeCodexAPI(ctx); ok {
		return s
	}
	if s, ok := probeCodexExec(ctx); ok {
		return s
	}
	s := Snapshot{
		Provider:          "codex",
		Source:            "unknown",
		UnavailableReason: "Codex usage API + exec probe failed",
	}
	s.Label = FormatLabel(s)
	return s
}

func probeCodexAPI(ctx context.Context) (Snapshot, bool) {
	path := codexAuthPath()
	if path == "" {
		return Snapshot{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, false
	}
	var auth codexAuthFile
	if err := json.Unmarshal(data, &auth); err != nil || auth.Tokens == nil || auth.Tokens.AccessToken == "" {
		return Snapshot{}, false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexUsageURL, nil)
	if err != nil {
		return Snapshot{}, false
	}
	req.Header.Set("Authorization", "Bearer "+auth.Tokens.AccessToken)
	if auth.Tokens.AccountID != "" {
		req.Header.Set("ChatGPT-Account-ID", auth.Tokens.AccountID)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://chatgpt.com/codex/settings/usage")
	req.Header.Set("Origin", "https://chatgpt.com")

	resp, err := codexHTTPDo(req)
	if err != nil {
		return Snapshot{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, false
	}
	var body codexUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Snapshot{}, false
	}
	if body.RateLimit == nil || body.RateLimit.PrimaryWindow == nil {
		return Snapshot{}, false
	}
	w := body.RateLimit.PrimaryWindow
	remaining := clampPct(100 - w.UsedPercent)
	s := Snapshot{
		Provider:     "codex",
		RemainingPct: &remaining,
		Window:       windowFromSeconds(w.LimitWindowSeconds),
		ResetHint:    shortResetFromUnix(w.ResetAt),
		Source:       "codex-api",
	}
	var bits []string
	if body.PlanType != "" {
		bits = append(bits, body.PlanType+" plan")
	}
	if body.Credits != nil && body.Credits.Balance != "" && body.Credits.Balance != "0" {
		bits = append(bits, "credits "+body.Credits.Balance)
	}
	s.Detail = strings.Join(bits, " · ")
	s.Label = FormatLabel(s)
	return s, true
}

func probeCodexExec(ctx context.Context) (Snapshot, bool) {
	bin, err := lookCLI("codex")
	if err != nil {
		return Snapshot{}, false
	}
	out, _ := runCLI(ctx, bin, "exec", "--skip-git-repo-check", "ping")
	if out == "" {
		return Snapshot{}, false
	}
	return parseCodexExecOut(out)
}

func parseCodexExecOut(out string) (Snapshot, bool) {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "usage limit") || strings.Contains(lower, "hit your usage limit") {
		remaining := 0.0
		s := Snapshot{
			Provider:     "codex",
			RemainingPct: &remaining,
			Window:       "week",
			Source:       "codex-exec",
			ResetHint:    parseTryAgainAt(out),
		}
		s.Label = FormatLabel(s)
		return s, true
	}
	if strings.Contains(lower, "error:") {
		return Snapshot{}, false
	}
	return Snapshot{
		Provider: "codex",
		Source:   "codex-exec",
		Label:    "available · no % in CLI (API preferred)",
	}, true
}

func parseTryAgainAt(out string) string {
	const marker = "try again at "
	lower := strings.ToLower(out)
	i := strings.Index(lower, marker)
	if i < 0 {
		return ""
	}
	rest := out[i+len(marker):]
	if j := strings.IndexAny(rest, "\r\n"); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSpace(rest)
	rest = strings.TrimRight(rest, ".) ")
	if rest == "" {
		return ""
	}
	if len(rest) > 32 {
		rest = rest[:32]
	}
	return "resets " + rest
}
