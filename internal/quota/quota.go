// Package quota fetches best-effort remaining usage for coding-agent CLIs
// so the interactive picker can show % left without blocking launch.
package quota

import (
	"context"
	"sync"
	"time"
)

const (
	// DefaultTimeout caps how long FetchAll waits for live probes.
	// Claude /usage ~4–5s; Copilot -p scrape ~7s. Parallel, so wall clock ≈ slowest.
	DefaultTimeout = 20 * time.Second
	// CacheTTL keeps re-picks fast.
	CacheTTL = 120 * time.Second
)

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
// become UnavailableReason so the picker stays usable.
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
					if s.Label == "" {
						s.Label = FormatLabel(s)
					}
					out[name] = s
				}
			}
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
		if !opt.SkipCache {
			if cached, ok := loadCache(now()); ok {
				if s, hit := cached[name]; hit && quotaKnown(s) {
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
		if !ok || !quotaKnown(s) {
			return false
		}
	}
	return true
}

func quotaKnown(s Snapshot) bool {
	if s.RemainingPct != nil {
		return true
	}
	// Status-only probes (e.g. "available · no % exposed").
	return s.Source != "" && s.Source != "unknown" && s.Label != ""
}

type probeFn func(ctx context.Context) Snapshot

func probeFor(name string) probeFn {
	switch name {
	case "cursor":
		return probeCursor
	case "claude":
		return probeClaude
	case "codex":
		return probeCodex
	case "copilot":
		return probeCopilot
	case "grok":
		return probeGrok
	case "agy":
		return probeAgy
	default:
		return nil
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
	return best.name
}
