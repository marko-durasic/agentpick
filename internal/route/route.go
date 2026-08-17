package route

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/launch"
	"github.com/marko-durasic/agentpick/internal/quota"
)

// Role aliases map DuReef / sprint role names to catalog roles.
var roleAliases = map[string]string{
	"independent_review": "review",
	"idea_proposal":      "plan",
	"tiny_task":          "tiny",
	"local_helper":       "tiny",
	"classify":           "tiny",
	"orchestrator":       "orchestrator",
}

// Request controls role-based provider ranking.
type Request struct {
	Role           string
	Exclude        []string
	Prefer         []string
	RequireHealthy bool
	TaskClass      string // optional #750 hint (debug, plan, …)
	Lane           string // optional business lane (revenue, product, …)
	SkipQuota      bool
}

// Candidate is one ranked provider option.
type Candidate struct {
	Provider string          `json:"provider"`
	Score    float64         `json:"score"`
	Priority int             `json:"priority"`
	Quota    quota.Snapshot  `json:"quota"`
	Healthy  bool            `json:"healthy"`
	Reason   string          `json:"reason"`
}

// Decision is the routing outcome.
type Decision struct {
	Role     string      `json:"role"`
	Provider string      `json:"provider"`
	Reason   string      `json:"reason"`
	Action   string      `json:"action"` // use | defer
	Quota    quota.Snapshot `json:"quota"`
	Ranked   []Candidate `json:"ranked"`
	TaskClass string     `json:"task_class,omitempty"`
	Lane      string     `json:"lane,omitempty"`
}

// Resolve ranks providers for a role and returns the best eligible choice.
func Resolve(ctx context.Context, reg *defaults.Registry, req Request) (Decision, error) {
	if reg == nil {
		return Decision{Action: "defer", Reason: "nil registry"}, nil
	}
	role := NormalizeRole(req.Role)
	if role == "" {
		return Decision{Role: req.Role, Action: "defer", Reason: "unknown role"}, nil
	}

	order := req.Prefer
	if len(order) == 0 {
		order = reg.RoleOrder(role)
	}
	if len(order) == 0 {
		return Decision{Role: role, Action: "defer", Reason: "no role_orders for " + role}, nil
	}

	exclude := normalizeExcludeSet(req.Exclude)
	snaps := map[string]quota.Snapshot{}
	if !req.SkipQuota {
		snaps = quota.FetchAll(ctx, quota.FetchOptions{Providers: order})
	}

	var ranked []Candidate
	for _, name := range order {
		name = strings.TrimSpace(name)
		if name == "" || exclude[name] {
			continue
		}
		p, ok := reg.Get(name)
		if !ok {
			continue
		}
		healthy := launch.Available(p)
		if req.RequireHealthy && !healthy {
			continue
		}
		if !providerSupportsRole(p, role) {
			continue
		}
		pri := rolePriority(p, role)
		snap := snaps[name]
		score := scoreCandidate(pri, snap)
		reason := candidateReason(role, pri, snap, healthy)
		ranked = append(ranked, Candidate{
			Provider: name,
			Score:    score,
			Priority: pri,
			Quota:    snap,
			Healthy:  healthy,
			Reason:   reason,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Priority < ranked[j].Priority
	})

	d := Decision{
		Role:      role,
		Action:    "defer",
		Reason:    "no eligible provider",
		Ranked:    ranked,
		TaskClass: strings.TrimSpace(req.TaskClass),
		Lane:      strings.TrimSpace(req.Lane),
	}
	if len(ranked) == 0 {
		return d, nil
	}
	best := ranked[0]
	if req.RequireHealthy && !best.Healthy {
		d.Reason = "no healthy provider"
		return d, nil
	}
	d.Action = "use"
	d.Provider = best.Provider
	d.Quota = best.Quota
	d.Reason = formatDecisionReason(role, best)
	return d, nil
}

// NormalizeRole canonicalizes role names and aliases.
func NormalizeRole(role string) string {
	r := strings.TrimSpace(strings.ToLower(role))
	if r == "" {
		return ""
	}
	if alias, ok := roleAliases[r]; ok {
		return alias
	}
	return r
}

func normalizeExcludeSet(exclude []string) map[string]bool {
	out := make(map[string]bool, len(exclude)*2)
	for _, raw := range exclude {
		raw = strings.TrimSpace(strings.ToLower(raw))
		if raw == "" {
			continue
		}
		switch raw {
		case "cursor-agent", "agent", "auto":
			out["cursor"] = true
		default:
			out[raw] = true
		}
	}
	return out
}

func providerSupportsRole(p defaults.Provider, role string) bool {
	if len(p.Roles) == 0 {
		return true
	}
	for _, r := range p.Roles {
		if NormalizeRole(r) == role {
			return true
		}
	}
	return false
}

func rolePriority(p defaults.Provider, role string) int {
	if p.RolePriority != nil {
		if pri, ok := p.RolePriority[role]; ok && pri > 0 {
			return pri
		}
	}
	return 50
}

func scoreCandidate(priority int, snap quota.Snapshot) float64 {
	// Lower priority number is better; invert to score base.
	base := 1000.0 - float64(priority)*10
	if snap.RemainingPct != nil {
		base += *snap.RemainingPct
	}
	return base
}

func candidateReason(role string, pri int, snap quota.Snapshot, healthy bool) string {
	parts := []string{role + "_role", fmt.Sprintf("priority=%d", pri)}
	if !healthy {
		parts = append(parts, "missing")
	}
	if snap.RemainingPct != nil {
		parts = append(parts, fmt.Sprintf("%.0fpct_%s", *snap.RemainingPct, snap.Window))
	} else if snap.Label != "" {
		parts = append(parts, snap.Label)
	}
	return strings.Join(parts, "+")
}

func formatDecisionReason(role string, c Candidate) string {
	return fmt.Sprintf("route=%s reason=%s", c.Provider, c.Reason)
}

// RegistryProviderID maps agentpick provider name to DuReef registry id when needed.
func RegistryProviderID(name string) string {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "cursor":
		return "cursor-agent"
	default:
		return strings.TrimSpace(name)
	}
}
