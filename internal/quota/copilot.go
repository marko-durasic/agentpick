package quota

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var aiCreditsRE = regexp.MustCompile(`(?i)AI Credits\s+(\d+)`)

func probeCopilot(ctx context.Context) Snapshot {
	bin, err := lookCLI("copilot")
	if err != nil {
		s := Snapshot{
			Provider:          "copilot",
			Source:            "unknown",
			UnavailableReason: "copilot not on PATH",
		}
		s.Label = FormatLabel(s)
		return s
	}
	// Tiny prompt; footer often includes "AI Credits N" and/or monthly quota errors.
	// Flags must come before -p so the prompt is not eaten as an option value.
	out, _ := runCLIEnv(ctx, []string{"COPILOT_ALLOW_ALL=1"}, bin, "--allow-all-tools", "-p", "PONG")
	if out == "" {
		s := Snapshot{
			Provider:          "copilot",
			Source:            "unknown",
			UnavailableReason: "copilot probe returned no output",
		}
		s.Label = FormatLabel(s)
		return s
	}
	return parseCopilotOut(out)
}

func parseCopilotOut(out string) Snapshot {
	lower := strings.ToLower(out)
	credits := parseAICredits(out)

	if strings.Contains(lower, "exceeded your monthly quota") || strings.Contains(lower, "monthly quota") {
		remaining := 0.0
		s := Snapshot{
			Provider:     "copilot",
			RemainingPct: &remaining,
			Window:       "month",
			Source:       "copilot-cli",
		}
		if credits >= 0 {
			s.Detail = "AI credits " + strconv.Itoa(credits)
		}
		s.Label = FormatLabel(s)
		return s
	}

	if credits == 0 && (strings.Contains(lower, "quota") || strings.Contains(lower, "limit")) {
		remaining := 0.0
		s := Snapshot{
			Provider:     "copilot",
			RemainingPct: &remaining,
			Window:       "month",
			Source:       "copilot-cli",
			Detail:       "AI credits 0",
		}
		s.Label = FormatLabel(s)
		return s
	}

	if credits > 0 {
		return Snapshot{
			Provider: "copilot",
			Source:   "copilot-cli",
			Label:    "available · AI credits " + strconv.Itoa(credits) + " (no monthly %)",
		}
	}

	// Successful reply without credits line.
	if !strings.Contains(lower, "error") {
		return Snapshot{
			Provider: "copilot",
			Source:   "copilot-cli",
			Label:    "available · no % exposed by CLI",
		}
	}

	s := Snapshot{
		Provider:          "copilot",
		Source:            "unknown",
		UnavailableReason: "copilot probe inconclusive",
	}
	s.Label = FormatLabel(s)
	return s
}

func parseAICredits(out string) int {
	m := aiCreditsRE.FindStringSubmatch(out)
	if len(m) < 2 {
		return -1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return -1
	}
	return n
}
