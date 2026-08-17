package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/marko-durasic/agentpick/internal/route"
)

// Record is one auditable routing decision (#750 learning loop).
type Record struct {
	Time       string            `json:"time"`
	Role       string            `json:"role"`
	Provider   string            `json:"provider"`
	Reason     string            `json:"reason"`
	Action     string            `json:"action"`
	TaskClass  string            `json:"task_class,omitempty"`
	Lane       string            `json:"lane,omitempty"`
	Ranked     []route.Candidate `json:"ranked,omitempty"`
}

// DefaultPath is the route history JSONL file.
func DefaultPath() string {
	if v := strings.TrimSpace(os.Getenv("AGENTPICK_ROUTE_HISTORY")); v != "" {
		return v
	}
	cache := strings.TrimSpace(os.Getenv("AGENTPICK_CACHE_DIR"))
	if cache == "" {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			cache = filepath.Join(home, ".cache", "agentpick")
		}
	}
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "agentpick")
	}
	return filepath.Join(cache, "route-history.jsonl")
}

// Append writes one decision record (fail-soft).
func Append(dec route.Decision) error {
	rec := Record{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Role:      dec.Role,
		Provider:  dec.Provider,
		Reason:    dec.Reason,
		Action:    dec.Action,
		TaskClass: dec.TaskClass,
		Lane:      dec.Lane,
		Ranked:    dec.Ranked,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	path := DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return err
	}
	return nil
}

// MaybeFeedRankings writes a lightweight signal file for dureef model-ranker when enabled.
func MaybeFeedRankings(dec route.Decision) {
	if strings.TrimSpace(os.Getenv("AGENTPICK_FEED_RANKINGS")) != "1" {
		return
	}
	path := strings.TrimSpace(os.Getenv("DUREEF_MODEL_RANKINGS_PATH"))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return
		}
		path = filepath.Join(home, ".cache", "dureef", "model-rankings.json")
	}
	// Fail-soft overlay: bump route winner note only — full ranker remains authoritative.
	type overlay struct {
		Source string `json:"source"`
		Notes  string `json:"notes"`
		At     string `json:"at"`
		Role   string `json:"role"`
		Winner string `json:"winner"`
	}
	payload := overlay{
		Source: "agentpick-route",
		Notes:  dec.Reason,
		At:     time.Now().UTC().Format(time.RFC3339),
		Role:   dec.Role,
		Winner: dec.Provider,
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	sidecar := path + ".agentpick-route.jsonl"
	f, err := os.OpenFile(sidecar, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", line)
}
