package quota

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	cursorAPIBase       = "https://api2.cursor.sh"
	cursorDashboardSvc  = "/aiserver.v1.DashboardService"
	cursorAccessKey     = "cursorAuth/accessToken"
	cursorRESTUsageURL  = "https://cursor.com/api/usage"
)

var (
	sqliteLookPath = exec.LookPath
	sqliteRun      = defaultSqliteRun
	httpDo         = defaultHTTPDo
)

func defaultSqliteRun(dbPath, sql string) (string, error) {
	bin, err := sqliteLookPath("sqlite3")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, dbPath, sql)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func defaultHTTPDo(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	return client.Do(req)
}

func probeCursor(ctx context.Context) Snapshot {
	token, err := readCursorAccessToken()
	if err != nil || token == "" {
		s := Snapshot{
			Provider:          "cursor",
			Source:            "unknown",
			UnavailableReason: "Cursor not signed in locally (no state.vscdb token)",
			Err:               errString(err, "no cursor token"),
		}
		s.Label = FormatLabel(s)
		return s
	}

	info, err := fetchCursorUsage(ctx, token)
	if err != nil {
		s := Snapshot{
			Provider:          "cursor",
			Source:            "unknown",
			UnavailableReason: "Cursor usage API unavailable",
			Err:               err.Error(),
		}
		s.Label = FormatLabel(s)
		return s
	}
	remaining := clampPct(100 - info.PercentUsed)
	s := Snapshot{
		Provider:     "cursor",
		RemainingPct: &remaining,
		Window:       "billing-period",
		ResetHint:    info.ResetHint,
		Source:       "cursor-api",
	}
	s.Label = FormatLabel(s)
	return s
}

type cursorUsageInfo struct {
	PercentUsed float64
	ResetHint   string
}

func errString(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func defaultStateDBPath() string {
	if override := strings.TrimSpace(os.Getenv("AGENTPICK_CURSOR_STATE_DB")); override != "" {
		return override
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return ""
		}
		return filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
	default:
		return filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	}
}

func readCursorAccessToken() (string, error) {
	dbPath := defaultStateDBPath()
	if dbPath == "" {
		return "", fmt.Errorf("cursor state.vscdb path unknown")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return "", fmt.Errorf("cursor state.vscdb missing")
	}
	sql := fmt.Sprintf("SELECT value FROM ItemTable WHERE key='%s' LIMIT 1;", strings.ReplaceAll(cursorAccessKey, "'", "''"))
	raw, err := sqliteRun(dbPath, sql)
	if err != nil {
		return "", fmt.Errorf("sqlite read: %w", err)
	}
	return normalizeToken(raw), nil
}

func normalizeToken(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
}

type planUsageRaw struct {
	TotalSpend       *float64 `json:"totalSpend"`
	IncludedSpend    *float64 `json:"includedSpend"`
	Remaining        *float64 `json:"remaining"`
	Limit            *float64 `json:"limit"`
	TotalPercentUsed *float64 `json:"totalPercentUsed"`
}

type currentPeriodUsageRaw struct {
	BillingCycleStart *string       `json:"billingCycleStart"`
	BillingCycleEnd   *string       `json:"billingCycleEnd"`
	PlanUsage         *planUsageRaw `json:"planUsage"`
	Enabled           *bool         `json:"enabled"`
}

type requestUsageRaw struct {
	GPT4 *struct {
		NumRequests     *float64 `json:"numRequests"`
		MaxRequestUsage *float64 `json:"maxRequestUsage"`
	} `json:"gpt-4"`
}

func fetchCursorPercentUsed(ctx context.Context, token string) (float64, error) {
	info, err := fetchCursorUsage(ctx, token)
	if err != nil {
		return 0, err
	}
	return info.PercentUsed, nil
}

func fetchCursorUsage(ctx context.Context, token string) (cursorUsageInfo, error) {
	usageURL := cursorAPIBase + cursorDashboardSvc + "/GetCurrentPeriodUsage"
	planURL := cursorAPIBase + cursorDashboardSvc + "/GetPlanInfo"

	usageRaw, err := postCursorJSON(ctx, usageURL, token)
	if err != nil {
		return cursorUsageInfo{}, err
	}
	var usage currentPeriodUsageRaw
	if err := json.Unmarshal(usageRaw, &usage); err != nil {
		return cursorUsageInfo{}, err
	}

	planName := "Pro"
	if planRaw, err := postCursorJSON(ctx, planURL, token); err == nil {
		var plan struct {
			PlanInfo *struct {
				PlanName string `json:"planName"`
			} `json:"planInfo"`
		}
		if json.Unmarshal(planRaw, &plan) == nil && plan.PlanInfo != nil && strings.TrimSpace(plan.PlanInfo.PlanName) != "" {
			planName = strings.TrimSpace(plan.PlanInfo.PlanName)
		}
	}

	reset := billingResetHint(usage.BillingCycleEnd)

	if usage.PlanUsage != nil && usage.PlanUsage.TotalPercentUsed != nil {
		return cursorUsageInfo{PercentUsed: *usage.PlanUsage.TotalPercentUsed, ResetHint: reset}, nil
	}
	if usage.PlanUsage != nil {
		if pct := derivePercentUsed(usage.PlanUsage); pct != nil {
			return cursorUsageInfo{PercentUsed: *pct, ResetHint: reset}, nil
		}
	}

	if isTeamPlan(planName) || usage.Enabled != nil && !*usage.Enabled || usage.PlanUsage == nil {
		if reqPct, err := fetchCursorRequestPercentUsed(ctx, token); err == nil {
			return cursorUsageInfo{PercentUsed: reqPct, ResetHint: reset}, nil
		}
	}
	return cursorUsageInfo{}, fmt.Errorf("cursor usage unavailable")
}

func billingResetHint(endMs *string) string {
	if endMs == nil || strings.TrimSpace(*endMs) == "" {
		return ""
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(*endMs), 10, 64)
	if err != nil || ms <= 0 {
		return ""
	}
	t := time.UnixMilli(ms)
	return "resets " + t.Format("Jan 2")
}

func derivePercentUsed(p *planUsageRaw) *float64 {
	if p.TotalPercentUsed != nil {
		return p.TotalPercentUsed
	}
	if p.Limit != nil && *p.Limit > 0 {
		var used float64
		if p.TotalSpend != nil {
			used = *p.TotalSpend
		} else if p.IncludedSpend != nil {
			used = *p.IncludedSpend
		} else if p.Remaining != nil {
			used = *p.Limit - *p.Remaining
		} else {
			return nil
		}
		pct := (used / *p.Limit) * 100
		return &pct
	}
	return nil
}

func isTeamPlan(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "team" || n == "enterprise" || n == "business"
}

func postCursorJSON(ctx context.Context, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	resp, err := httpDo(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cursor API %s: %d", url, resp.StatusCode)
	}
	return body, nil
}

func fetchCursorRequestPercentUsed(ctx context.Context, token string) (float64, error) {
	session := buildWorkosSessionCookie(token)
	if session == nil {
		return 0, fmt.Errorf("jwt missing sub")
	}
	u := cursorRESTUsageURL + "?user=" + session.userID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Cookie", "WorkosCursorSessionToken="+session.cookie)
	resp, err := httpDo(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("cursor usage REST: %d", resp.StatusCode)
	}
	var raw requestUsageRaw
	if err := json.Unmarshal(body, &raw); err != nil {
		return 0, err
	}
	if raw.GPT4 == nil || raw.GPT4.MaxRequestUsage == nil || *raw.GPT4.MaxRequestUsage <= 0 {
		return 0, fmt.Errorf("no request quota")
	}
	used := 0.0
	if raw.GPT4.NumRequests != nil {
		used = *raw.GPT4.NumRequests
	}
	return (used / *raw.GPT4.MaxRequestUsage) * 100, nil
}

type workosSession struct {
	userID string
	cookie string
}

func buildWorkosSessionCookie(accessToken string) *workosSession {
	payload := decodeJWTPayload(accessToken)
	if payload == nil {
		return nil
	}
	sub, _ := payload["sub"].(string)
	if sub == "" {
		return nil
	}
	parts := strings.Split(sub, "|")
	userID := parts[0]
	if len(parts) > 1 {
		userID = parts[1]
	}
	if userID == "" {
		return nil
	}
	return &workosSession{
		userID: userID,
		cookie: userID + "%3A%3A" + accessToken,
	}
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// try padded StdEncoding
		raw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
