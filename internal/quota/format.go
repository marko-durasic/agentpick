package quota

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Snapshot is one provider's remaining quota (or why it is unavailable).
type Snapshot struct {
	Provider string
	// RemainingPct is the primary remaining percentage (nil = unknown).
	RemainingPct *float64
	// Window names the quota clock: "week", "session", "billing-period".
	Window string
	// ResetHint is a short human reset string, e.g. "resets Tue 8am".
	ResetHint string
	// Detail is optional secondary signal (e.g. Claude session alongside week).
	Detail string
	// UnavailableReason explains why RemainingPct is nil (shown instead of n/a).
	UnavailableReason string
	Label             string // rendered primary quota column
	Source            string // cursor-api, claude-usage, claude-session, claude-history, cache, unknown
	Err               string
}

func unknownSnapshot(name string) Snapshot {
	s := Snapshot{
		Provider:          name,
		Source:            "unknown",
		UnavailableReason: unavailableReason(name),
	}
	s.Label = FormatLabel(s)
	return s
}

func unavailableReason(name string) string {
	switch name {
	case "codex", "copilot", "grok", "agy", "claude", "cursor":
		return "usage probe failed"
	default:
		return "no quota probe yet"
	}
}

// FormatLabel builds the primary quota cell for a snapshot.
func FormatLabel(s Snapshot) string {
	if s.RemainingPct == nil {
		if s.Label != "" {
			return s.Label
		}
		if s.UnavailableReason != "" {
			return s.UnavailableReason
		}
		return "no quota probe yet"
	}
	pct := clampPct(*s.RemainingPct)
	window := s.Window
	if window == "" {
		window = defaultWindow(s)
	}
	primary := fmt.Sprintf("%s %.0f%% left", window, pct)
	if s.ResetHint != "" {
		primary += " · " + s.ResetHint
	}
	if s.Detail != "" {
		primary += " · " + s.Detail
	}
	return primary
}

func defaultWindow(s Snapshot) string {
	switch s.Source {
	case "claude-usage":
		return "week"
	case "claude-session", "claude-history":
		return "session"
	case "cursor-api":
		return "billing-period"
	case "codex-api", "codex-exec":
		return "week"
	case "copilot-cli":
		return "month"
	default:
		if s.Provider == "claude" {
			return "week"
		}
		if s.Provider == "cursor" {
			return "billing-period"
		}
		if s.Provider == "codex" {
			return "week"
		}
		if s.Provider == "copilot" {
			return "month"
		}
		return "quota"
	}
}

// PickerColumns are fixed widths for the interactive table.
const (
	colNum    = 3
	colAgent  = 8
	colModel  = 34
	colQuota  = 52
)

// FormatPickerHeader returns the column header line (without leading indent).
func FormatPickerHeader() string {
	return pad(" #", colNum) + " " +
		pad("agent", colAgent) + " " +
		pad("default model", colModel) + " " +
		"remaining quota"
}

// FormatPickerRow renders one aligned picker row.
func FormatPickerRow(num int, agent, model string, snap Snapshot) string {
	q := snap.Label
	if q == "" {
		q = FormatLabel(snap)
	}
	return fmt.Sprintf("%s %s %s %s",
		pad(fmt.Sprintf("%d)", num), colNum),
		pad(agent, colAgent),
		pad(truncate(model, colModel), colModel),
		q,
	)
}

// FormatLegend explains windows and unavailable rows.
func FormatLegend(snaps map[string]Snapshot) string {
	var b strings.Builder
	b.WriteString("How to read remaining quota\n")
	b.WriteString("  week           Claude / Codex primary window (often ~7 days)\n")
	b.WriteString("  session        Claude rolling session window (~5h)\n")
	b.WriteString("  billing-period Cursor plan usage for the current billing cycle\n")
	b.WriteString("  month          Copilot monthly quota (when CLI reports it)\n")
	b.WriteString("  available…     Probe ran OK but CLI/API exposed no percentage\n")
	if sug := Suggest(snaps, orderedKnown(snaps)); sug != "" {
		b.WriteString("Suggested: ")
		b.WriteString(sug)
		b.WriteString(" (most remaining among known quotas)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func orderedKnown(snaps map[string]Snapshot) []string {
	order := []string{"agy", "claude", "codex", "copilot", "cursor", "grok"}
	out := make([]string, 0, len(order))
	for _, n := range order {
		if _, ok := snaps[n]; ok {
			out = append(out, n)
		}
	}
	for n := range snaps {
		found := false
		for _, o := range out {
			if o == n {
				found = true
				break
			}
		}
		if !found {
			out = append(out, n)
		}
	}
	return out
}

func pad(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func truncate(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(s)
	return string(runes[:width-1]) + "…"
}
