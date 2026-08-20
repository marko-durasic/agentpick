package quota

import (
	"context"
	"strings"
)

func probeGrok(ctx context.Context) Snapshot {
	bin, err := lookCLI("grok")
	if err != nil {
		s := Snapshot{Provider: "grok", Source: "unknown", UnavailableReason: "grok not on PATH"}
		s.Label = FormatLabel(s)
		return s
	}
	out, _ := runCLI(ctx, bin, "--single", "PONG")
	return parseLimitOrAvailable("grok", "grok-cli", out)
}

func parseLimitOrAvailable(provider, source, out string) Snapshot {
	if strings.TrimSpace(out) == "" {
		// Binary was on PATH; print/single probes often emit nothing.
		// That is not "no quota" — agy's /usage panel is a TUI, not -p.
		return availableCLI(provider, source)
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "http_status\": 401") ||
		strings.Contains(lower, "authenticated inference requests still rejected (401)") ||
		strings.Contains(lower, "authentication failed") {
		s := Snapshot{
			Provider:          provider,
			Source:            "unknown",
			UnavailableReason: provider + " authentication failed (HTTP 401)",
		}
		s.Label = FormatLabel(s)
		return s
	}
	if strings.Contains(lower, "usage limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota") && (strings.Contains(lower, "exceed") || strings.Contains(lower, "reached")) {
		remaining := 0.0
		s := Snapshot{
			Provider:     provider,
			RemainingPct: &remaining,
			Window:       "quota",
			Source:       source,
			ResetHint:    parseTryAgainAt(out),
		}
		s.Label = FormatLabel(s)
		return s
	}
	if strings.Contains(lower, "timeout waiting") ||
		strings.Contains(lower, "timed out") ||
		(strings.Contains(lower, "error:") && !strings.Contains(lower, "pong")) {
		s := Snapshot{
			Provider:          provider,
			Source:            "unknown",
			UnavailableReason: provider + " probe inconclusive",
		}
		s.Label = FormatLabel(s)
		return s
	}
	return availableCLI(provider, source)
}
