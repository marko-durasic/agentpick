package quota

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// probeAgy does not use `agy -p` — print mode never renders the /usage TUI
// (Models & Quota). Empty print output was wrongly treated as "no quota".
func probeAgy(ctx context.Context) Snapshot {
	if _, err := lookCLI("agy"); err != nil {
		s := Snapshot{Provider: "agy", Source: "unknown", UnavailableReason: "agy not on PATH"}
		s.Label = FormatLabel(s)
		return s
	}
	if s, ok := probeAgyUsageJSON(); ok {
		return s
	}
	if s, ok := probeAgyUsageCLI(ctx); ok {
		return s
	}
	return availableCLI("agy", "agy-cli")
}

func probeAgyUsageCLI(ctx context.Context) (Snapshot, bool) {
	for _, name := range []string{"agy-cli-usage", "agy-usage"} {
		bin, err := lookCLI(name)
		if err != nil {
			continue
		}
		out, _ := runCLI(ctx, bin, "--json")
		if s, ok := parseAgyUsageJSON(out); ok {
			s.Provider = "agy"
			s.Source = name
			s.Label = FormatLabel(s)
			return s, true
		}
	}
	return Snapshot{}, false
}

func probeAgyUsageJSON() (Snapshot, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Snapshot{}, false
	}
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		cache = filepath.Join(home, ".cache")
	}
	raw, err := os.ReadFile(filepath.Join(cache, "agy-usage", "quota.json"))
	if err != nil {
		return Snapshot{}, false
	}
	s, ok := parseAgyUsageJSON(string(raw))
	if !ok {
		return Snapshot{}, false
	}
	s.Provider = "agy"
	s.Source = "agy-usage-cache"
	s.Label = FormatLabel(s)
	return s, true
}

type agyUsageSnapshot struct {
	Groups []agyUsageGroup `json:"groups"`
}

type agyUsageGroup struct {
	Name    string           `json:"name"`
	Buckets []agyUsageBucket `json:"buckets"`
}

type agyUsageBucket struct {
	Kind              string   `json:"kind"`
	Label             string   `json:"label"`
	RemainingFraction *float64 `json:"remainingFraction"`
	ResetsInSeconds   *int     `json:"resetsInSeconds"`
	Available         bool     `json:"available"`
}

func parseAgyUsageJSON(raw string) (Snapshot, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] != '{' {
		return Snapshot{}, false
	}
	var snap agyUsageSnapshot
	if json.Unmarshal([]byte(raw), &snap) != nil || len(snap.Groups) == 0 {
		return Snapshot{}, false
	}
	var geminiWeek, anyWeek *agyUsageBucket
	var gemini5h *agyUsageBucket
	for _, g := range snap.Groups {
		isGemini := strings.Contains(strings.ToUpper(g.Name), "GEMINI")
		for i := range g.Buckets {
			b := g.Buckets[i]
			kind := strings.ToLower(b.Kind + " " + b.Label)
			if strings.Contains(kind, "week") {
				if isGemini && geminiWeek == nil {
					geminiWeek = &g.Buckets[i]
				}
				if anyWeek == nil {
					anyWeek = &g.Buckets[i]
				}
			}
			if isGemini && (strings.Contains(kind, "5h") || strings.Contains(kind, "five hour") || strings.Contains(kind, "5-hour")) {
				if gemini5h == nil {
					gemini5h = &g.Buckets[i]
				}
			}
		}
	}
	pick := geminiWeek
	if pick == nil {
		pick = anyWeek
	}
	if pick == nil {
		return Snapshot{}, false
	}
	pct, ok := bucketPct(*pick)
	if !ok {
		return Snapshot{}, false
	}
	s := Snapshot{
		Provider:     "agy",
		RemainingPct: &pct,
		Window:       "week",
		Source:       "agy-usage",
	}
	if pick.ResetsInSeconds != nil && *pick.ResetsInSeconds > 0 {
		s.ResetHint = "resets " + formatAgyDuration(*pick.ResetsInSeconds)
	}
	if gemini5h != nil {
		if p, ok := bucketPct(*gemini5h); ok {
			s.Detail = "5h " + formatPct(p) + "% left"
		}
	}
	s.Label = FormatLabel(s)
	return s, true
}

func bucketPct(b agyUsageBucket) (float64, bool) {
	if b.RemainingFraction != nil {
		return *b.RemainingFraction * 100, true
	}
	if b.Available {
		return 100, true
	}
	return 0, false
}

var (
	agyGroupRe   = regexp.MustCompile(`(?i)^([A-Z][A-Z0-9 &/]*MODELS)\s*$`)
	agyBucketRe  = regexp.MustCompile(`(?i)^(Weekly Limit|Five Hour Limit|5[- ]?Hour Limit)`)
	agyPctRe     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	agyRefreshRe = regexp.MustCompile(`(?i)Refreshes in (.+)$`)
)

// parseAgyUsagePanel reads the Models & Quota TUI (/usage or /quota).
// Primary remaining is Gemini weekly (agy's default coding group).
func parseAgyUsagePanel(text string) (Snapshot, bool) {
	type bucket struct {
		kind    string
		pct     *float64
		refresh string
	}
	type group struct {
		name    string
		buckets []bucket
	}
	var groups []group
	var cur *group
	var bkt *bucket
	flushB := func() {
		if cur != nil && bkt != nil {
			cur.buckets = append(cur.buckets, *bkt)
			bkt = nil
		}
	}
	flushG := func() {
		flushB()
		if cur != nil {
			groups = append(groups, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(stripANSI(line))
		if t == "" {
			continue
		}
		if m := agyGroupRe.FindStringSubmatch(t); m != nil {
			flushG()
			g := group{name: strings.Join(strings.Fields(m[1]), " ")}
			cur = &g
			continue
		}
		if agyBucketRe.MatchString(t) {
			flushB()
			kind := "5h"
			if strings.Contains(strings.ToLower(t), "week") {
				kind = "week"
			}
			b := bucket{kind: kind}
			bkt = &b
			continue
		}
		if bkt == nil {
			continue
		}
		if strings.Contains(strings.ToLower(t), "quota available") {
			pct := 100.0
			if bkt.pct == nil {
				bkt.pct = &pct
			}
		}
		if m := agyPctRe.FindStringSubmatch(t); m != nil && bkt.pct == nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				bkt.pct = &v
			}
		}
		if m := agyRefreshRe.FindStringSubmatch(t); m != nil && bkt.refresh == "" {
			bkt.refresh = strings.TrimSpace(m[1])
		}
	}
	flushG()
	var geminiWeek, anyWeek, gemini5h *bucket
	for i := range groups {
		g := &groups[i]
		isGemini := strings.Contains(strings.ToUpper(g.name), "GEMINI")
		for j := range g.buckets {
			b := &g.buckets[j]
			if b.kind == "week" {
				if isGemini && geminiWeek == nil {
					geminiWeek = b
				}
				if anyWeek == nil {
					anyWeek = b
				}
			}
			if isGemini && b.kind == "5h" && gemini5h == nil {
				gemini5h = b
			}
		}
	}
	pick := geminiWeek
	if pick == nil {
		pick = anyWeek
	}
	if pick == nil || pick.pct == nil {
		return Snapshot{}, false
	}
	pct := *pick.pct
	s := Snapshot{
		Provider:     "agy",
		RemainingPct: &pct,
		Window:       "week",
		Source:       "agy-usage-panel",
	}
	if pick.refresh != "" {
		s.ResetHint = "resets " + pick.refresh
	}
	if gemini5h != nil && gemini5h.pct != nil {
		s.Detail = "5h " + formatPct(*gemini5h.pct) + "% left"
	}
	s.Label = FormatLabel(s)
	return s, true
}

func availableCLI(provider, source string) Snapshot {
	s := Snapshot{
		Provider: provider,
		Source:   source,
		Label:    "available · no % exposed by CLI",
	}
	return s
}

func formatAgyDuration(sec int) string {
	if sec <= 0 {
		return ""
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 {
		return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
	}
	return strconv.Itoa(m) + "m"
}

func formatPct(p float64) string {
	if p == float64(int(p)) {
		return strconv.Itoa(int(p))
	}
	return strconv.FormatFloat(p, 'f', 2, 64)
}

func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && (s[i+1] == '[' || s[i+1] == ']') {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == 0x07 {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
