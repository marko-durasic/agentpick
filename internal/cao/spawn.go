package cao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultSessionName is the sticky name used only when AGENTPICK_CAO_SESSION is set
// to this value (or empty override handling). Live launches mint a unique slug.
const DefaultSessionName = "agentpick"

// caoSessionPrefix is CAO 2.4.1's tmux/API prefix (SESSION_PREFIX).
const caoSessionPrefix = "cao-"

func sessionName() string {
	raw := strings.TrimSpace(os.Getenv("AGENTPICK_CAO_SESSION"))
	if raw == "" {
		raw = newSessionSlug()
	}
	return canonicalCAOSession(raw)
}

// newSessionSlug is unique per launch so a second `agentpick` does not collide
// with an already-running session. Override with AGENTPICK_CAO_SESSION to reuse.
func newSessionSlug() string {
	n := time.Now().UTC()
	return fmt.Sprintf("agentpick-%s-%x", n.Format("20060102-150405"), n.UnixNano())
}

// canonicalCAOSession matches CAO's create_terminal prefixing so spawn
// POSTs to the session that actually exists on GET /sessions.
func canonicalCAOSession(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultSessionName
	}
	if strings.HasPrefix(name, caoSessionPrefix) {
		return name
	}
	return caoSessionPrefix + name
}

func sessionNameMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" {
		return false
	}
	if got == want {
		return true
	}
	return canonicalCAOSession(got) == canonicalCAOSession(want)
}

func loopbackBase(host string, port int) string {
	h := strings.TrimSpace(host)
	if h == "localhost" || h == "::1" {
		h = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", h, port)
}

func spawnTerminalURL(base, session, profile, provider, workDir string) string {
	q := url.Values{}
	q.Set("agent_profile", profile)
	if provider != "" {
		q.Set("provider", provider)
	}
	if workDir != "" {
		q.Set("working_directory", workDir)
	}
	q.Set("defer_init", "true")
	return strings.TrimRight(base, "/") + "/sessions/" + url.PathEscape(session) + "/terminals?" + q.Encode()
}

func caoAssignable(w Workers) []Worker {
	var out []Worker
	seen := map[string]bool{}
	for _, wr := range allWorkers(w) {
		if wr.Via != ViaCAO || wr.Profile == "" || wr.CAOProvider == "" {
			continue
		}
		if seen[wr.Profile] {
			continue
		}
		seen[wr.Profile] = true
		out = append(out, wr)
	}
	return out
}

type sessionRow struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type terminalRow struct {
	AgentProfile string `json:"agent_profile"`
}

// SpawnSessionWorkers waits for the CAO session then creates tmux workers so
// the dashboard lists the healthy fleet. Grok/Ollama stay on dispatch.
func SpawnSessionWorkers(ctx context.Context, plan Plan, opt Options) error {
	workers := caoAssignable(opt.Workers)
	if len(workers) == 0 {
		return nil
	}
	base := loopbackBase(plan.Host, plan.Port)
	session := plan.SessionName
	if session == "" {
		session = sessionName()
	}
	wd := strings.TrimSpace(opt.WorkDir)
	client := &http.Client{Timeout: 5 * time.Second}
	live, err := waitForSession(ctx, client, base, session)
	if err != nil {
		return err
	}
	if live != "" {
		session = live
	}
	existing, _ := listProfiles(ctx, client, base, session)
	for _, wr := range workers {
		if existing[wr.Profile] {
			fmt.Fprintf(os.Stderr, "agentpick: CAO worker %s already in session\n", wr.Profile)
			continue
		}
		if err := postTerminal(ctx, client, base, session, wr, wd); err != nil {
			fmt.Fprintf(os.Stderr, "agentpick: spawn %s: %v\n", wr.Profile, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "agentpick: spawned CAO worker %s (%s) — send_message, do not duplicate assign\n", wr.Profile, wr.CAOProvider)
		existing[wr.Profile] = true
	}
	return nil
}

func waitForSession(ctx context.Context, client *http.Client, base, session string) (string, error) {
	u := strings.TrimRight(base, "/") + "/sessions"
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				var rows []sessionRow
				if json.Unmarshal(body, &rows) == nil {
					for _, r := range rows {
						if sessionNameMatches(r.Name, session) || sessionNameMatches(r.ID, session) {
							if strings.TrimSpace(r.Name) != "" {
								return r.Name, nil
							}
							return r.ID, nil
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("CAO session %q did not appear on %s", canonicalCAOSession(session), u)
}

func listProfiles(ctx context.Context, client *http.Client, base, session string) (map[string]bool, error) {
	u := strings.TrimRight(base, "/") + "/sessions/" + url.PathEscape(session) + "/terminals"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	out := map[string]bool{}
	var rows []terminalRow
	if json.Unmarshal(body, &rows) != nil {
		return out, nil
	}
	for _, r := range rows {
		if r.AgentProfile != "" {
			out[r.AgentProfile] = true
		}
	}
	return out, nil
}

func postTerminal(ctx context.Context, client *http.Client, base, session string, wr Worker, workDir string) error {
	u := spawnTerminalURL(base, session, wr.Profile, wr.CAOProvider, workDir)
	payload, _ := json.Marshal(map[string]string{
		"initial_message": "Stay idle until send_message. Then do that slice and send_message the supervisor (and peers if needed) with the result. Do not invent work. Do not start dureef-sprint.",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
