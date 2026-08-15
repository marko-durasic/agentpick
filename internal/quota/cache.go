package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type cacheFile struct {
	SavedAt time.Time            `json:"saved_at"`
	Items   map[string]Snapshot  `json:"items"`
}

func cachePath() string {
	if dir := os.Getenv("AGENTPICK_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "quota.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "agentpick-quota.json")
	}
	return filepath.Join(home, ".cache", "agentpick", "quota.json")
}

func loadCache(now time.Time) (map[string]Snapshot, bool) {
	path := cachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}
	if cf.SavedAt.IsZero() || now.Sub(cf.SavedAt) > CacheTTL {
		return nil, false
	}
	if cf.Items == nil {
		return nil, false
	}
	return cf.Items, true
}

func saveCache(items map[string]Snapshot, now time.Time) {
	path := cachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// Merge with any existing fresh entries so partial probes don't wipe siblings.
	merged := map[string]Snapshot{}
	if prev, ok := loadCache(now); ok {
		for k, v := range prev {
			merged[k] = v
		}
	}
	for k, v := range items {
		if v.RemainingPct != nil || v.Source != "unknown" {
			merged[k] = v
		} else if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	cf := cacheFile{SavedAt: now, Items: merged}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
