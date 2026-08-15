// Package quota fetches best-effort remaining usage for coding-agent CLIs
// so the interactive picker can show % left without blocking launch.
package quota

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultTimeout caps how long FetchAll waits for live probes.
	DefaultTimeout = 3500 * time.Millisecond
	// CacheTTL keeps re-picks fast.
	CacheTTL = 120 * time.Second
)

// Snapshot is one provider's remaining quota (or unknown).
type Snapshot struct {
	Provider     string
	RemainingPct *float64 // nil = unknown
	Label        string   // display fragment, e.g. "72% left" or "week 42% left" or "—"
	Source       string   // "cursor-api", "claude-usage", "cache", "unknown"
	Err          string   // non-fatal probe error (empty when OK / unknown)
}

// FetchOptions controls FetchAll.
type FetchOptions struct {
	// Providers to probe (e.g. installed picker rows). Empty = cursor + claude only.
	Providers []string
	// Timeout overrides DefaultTimeout when > 0.
	Timeout time.Duration
	// SkipCache forces live probes.
	SkipCache bool
	// Now overrides time for tests.
	Now func() time.Time
}

// FetchAll returns snapshots for the requested providers. Never errors — failures
// become Label "—" so the picker stays usable.
func FetchAll(ctx context.Context, opt FetchOptions) map[string]Snapshot {
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	names := opt.Providers
	if len(names) == 0 {
		names = []string{"cursor", "claude"}
	}

	out := make(map[string]Snapshot, len(names))
	for _, name := range names {
		out[name] = unknownSnapshot(name)
	}

	if !opt.SkipCache {
		if cached, ok := loadCache(now()); ok {
			for _, name := range names {
				if s, hit := cached[name]; hit {
					s.Source = "cache"
					out[name] = s
				}
			}
			// If every requested provider with a live probe is already cached, skip network.
			if allKnownCached(names, cached) {
				return out
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range names {
		name := name
		probe := probeFor(name)
		if probe == nil {
			continue
		}
		// Skip live probe if cache already has a fresh value for this name.
		if !opt.SkipCache {
			if cached, ok := loadCache(now()); ok {
				if s, hit := cached[name]; hit && s.RemainingPct != nil {
					continue
				}
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := probe(ctx)
			s.Provider = name
			if s.Label == "" {
				s.Label = FormatLabel(s)
			}
			mu.Lock()
			out[name] = s
			mu.Unlock()
		}()
	}
	wg.Wait()

	saveCache(out, now())
	return out
}

func allKnownCached(names []string, cached map[string]Snapshot) bool {
	for _, name := range names {
		if probeFor(name) == nil {
			continue
		}
		s, ok := cached[name]
		if !ok || s.RemainingPct == nil {
			return false
		}
	}
	return true
}

type probeFn func(ctx context.Context) Snapshot

func probeFor(name string) probeFn {
	switch name {
	case "cursor":
		return probeCursor
	case "claude":
		return probeClaude
	default:
		return nil
	}
}

func unknownSnapshot(name string) Snapshot {
	return Snapshot{
		Provider: name,
		Label:    "—",
		Source:   "unknown",
	}
}

// Suggest returns the provider name with the highest known remaining %, or "" if
// fewer than two providers have known remaining.
func Suggest(snaps map[string]Snapshot, order []string) string {
	type scored struct {
		name string
		pct  float64
	}
	var known []scored
	for _, name := range order {
		s, ok := snaps[name]
		if !ok || s.RemainingPct == nil {
			continue
		}
		known = append(known, scored{name: name, pct: *s.RemainingPct})
	}
	if len(known) < 2 {
		return ""
	}
	best := known[0]
	for _, k := range known[1:] {
		if k.pct > best.pct {
			best = k
		}
	}
	// Only suggest when the leader is clearly ahead (or tied at top — still name it).
	return best.name
}

// FormatLabel builds the short quota fragment for a snapshot.
func FormatLabel(s Snapshot) string {
	if s.RemainingPct == nil {
		return "—"
	}
	pct := clampPct(*s.RemainingPct)
	if s.Provider == "claude" || s.Source == "claude-usage" {
		return fmt.Sprintf("week %.0f%% left", pct)
	}
	return fmt.Sprintf("%.0f%% left", pct)
}
