package cao

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/launch"
	"github.com/marko-durasic/agentpick/internal/quota"
	"github.com/marko-durasic/agentpick/internal/route"
)

const (
	// SupervisorProfile is the CAO agent launched by agentpick.
	SupervisorProfile = "agentpick_supervisor"
	DevProfile        = "agentpick_dev"
	ReviewProfile     = "agentpick_review"

	ViaCAO           = "cao"
	ViaDispatch      = "dispatch"
	DefaultMaxActive = 4
)

// Worker is one pre-routed specialist for this session.
type Worker struct {
	Role        string
	Provider    string // agentpick name
	CAOProvider string // empty when Via is dispatch
	Profile     string
	Via         string // cao (tmux assign) or dispatch (agentpick dispatch)
	Why         string // leftover usage at session start
}

// Workers are warm-started at agentpick launch. The supervisor re-routes each
// new slice with fresh quota and current capacity; the human never runs route.
type Workers struct {
	Implement Worker
	Review    Worker
	Tiny      Worker
	// Extra is every other installed CLI: CAO panes for cursor/claude/agy/copilot/codex,
	// plus Grok via dispatch (CAO 2.4.1 has no grok Spawn Agent provider).
	Extra []Worker
	// MaxActive is the global specialist concurrency ceiling. Ready panes may
	// stay idle; the ceiling is not a target.
	MaxActive int
}

// fleetCAONames are CLIs CAO 2.4.1 can launch in tmux (not grok/ollama).
var fleetCAONames = []string{"cursor", "claude", "agy", "copilot", "codex"}

// PickWorkers chooses warm-start role placements, then loads the rest of the
// fleet. The supervisor re-routes live before each assignment. Role winners
// keep agentpick_dev / agentpick_review; other healthy CAO CLIs get
// agentpick_<name> panes. Grok (and tiny ollama) stay on dispatch.
func PickWorkers(ctx context.Context, reg *defaults.Registry, supervisor string) (Workers, error) {
	out := Workers{MaxActive: maxActiveWorkers()}
	// Implement may be a second pane of the supervisor CLI when that is the quota winner.
	impl, err := pickRole(ctx, reg, "implement", supervisor, false, false)
	if err != nil {
		return out, err
	}
	// Review never uses the supervisor (independent review).
	rev, err := pickRole(ctx, reg, "review", supervisor, false, true)
	if err != nil {
		return out, err
	}
	tiny, err := pickRole(ctx, reg, "tiny", supervisor, true, true)
	if err != nil {
		return out, err
	}
	out.Implement = namedFrom(impl, "implement", DevProfile)
	out.Review = namedFrom(rev, "review", ReviewProfile)
	out.Tiny = namedFrom(tiny, "tiny", "")
	out.Extra = extraFleet(ctx, reg, supervisor, out)
	return out, nil
}

func maxActiveWorkers() int {
	raw := strings.TrimSpace(os.Getenv("AGENTPICK_MAX_ACTIVE_AGENTS"))
	if raw == "" {
		return DefaultMaxActive
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultMaxActive
	}
	if n < 1 {
		return 1
	}
	if n > DefaultMaxActive {
		return DefaultMaxActive
	}
	return n
}

func extraFleet(ctx context.Context, reg *defaults.Registry, supervisor string, w Workers) []Worker {
	if reg == nil {
		return nil
	}
	names := append([]string{}, fleetCAONames...)
	names = append(names, "grok")
	snaps := quota.FetchAll(ctx, quota.FetchOptions{Providers: names})
	return extraFleetFrom(supervisor, w, snaps, func(name string) bool {
		p, ok := reg.Get(name)
		return ok && launch.Available(p)
	})
}

func extraFleetFrom(supervisor string, w Workers, snaps map[string]quota.Snapshot, installed func(string) bool) []Worker {
	sup := strings.ToLower(strings.TrimSpace(supervisor))
	spawned := map[string]bool{}
	if w.Implement.Via == ViaCAO && w.Implement.Provider != "" {
		spawned[strings.ToLower(w.Implement.Provider)] = true
	}
	if w.Review.Via == ViaCAO && w.Review.Provider != "" {
		spawned[strings.ToLower(w.Review.Provider)] = true
	}
	spawned[sup] = true // supervisor pane already exists
	var extra []Worker
	for _, name := range append(append([]string{}, fleetCAONames...), "grok") {
		if spawned[name] || !installed(name) {
			continue
		}
		snap := snaps[name]
		if quotaExhausted(snap) {
			continue
		}
		if name == "grok" {
			wr := named(name, "peer", "")
			wr.Why = quotaWhy(snap)
			extra = append(extra, wr)
			continue
		}
		wr := named(name, "peer", "agentpick_"+name)
		wr.Why = quotaWhy(snap)
		extra = append(extra, wr)
	}
	return extra
}

func namedFrom(c route.Candidate, role, profile string) Worker {
	w := named(c.Provider, role, profile)
	w.Why = quotaWhy(c.Quota)
	return w
}

func named(provider, role, profile string) Worker {
	if provider == "" {
		return Worker{Role: role}
	}
	w := Worker{Role: role, Provider: provider, Profile: profile, Via: ViaDispatch}
	id, err := ProviderID(provider)
	if err == nil {
		w.CAOProvider = id
		w.Via = ViaCAO
		return w
	}
	return w
}

func quotaWhy(s quota.Snapshot) string {
	if lab := strings.TrimSpace(s.Label); lab != "" {
		return lab
	}
	return strings.TrimSpace(quota.FormatLabel(s))
}

func pickRole(ctx context.Context, reg *defaults.Registry, role, supervisor string, allowOllama, excludeSupervisor bool) (route.Candidate, error) {
	var exclude []string
	if excludeSupervisor {
		exclude = []string{supervisor}
	}
	dec, err := route.Resolve(ctx, reg, route.Request{
		Role:           role,
		Exclude:        exclude,
		RequireHealthy: true,
	})
	if err != nil {
		return route.Candidate{}, err
	}
	name := PickRoutedPeer(dec.Ranked, supervisor, allowOllama, excludeSupervisor)
	for _, c := range dec.Ranked {
		if strings.EqualFold(strings.TrimSpace(c.Provider), name) {
			return c, nil
		}
	}
	return route.Candidate{Provider: name}, nil
}

// PickCAOPeer returns the first ranked provider CAO 2.4.1 can launch in tmux.
func PickCAOPeer(ranked []route.Candidate, supervisor string) string {
	sup := strings.ToLower(strings.TrimSpace(supervisor))
	for _, c := range ranked {
		p := strings.ToLower(strings.TrimSpace(c.Provider))
		if p == "" || p == sup {
			continue
		}
		if _, err := ProviderID(p); err != nil {
			continue
		}
		if quotaExhausted(c.Quota) {
			continue
		}
		return p
	}
	return ""
}

// PickRoutedPeer returns the first ranked healthy peer for this role.
// Grok is eligible via dispatch. Ollama is only eligible when allowOllama is set (tiny).
// When excludeSupervisor is false, a second instance of the supervisor CLI is allowed.
func PickRoutedPeer(ranked []route.Candidate, supervisor string, allowOllama, excludeSupervisor bool) string {
	sup := strings.ToLower(strings.TrimSpace(supervisor))
	if allowOllama {
		for _, c := range ranked {
			p := strings.ToLower(strings.TrimSpace(c.Provider))
			if p == "ollama" && !quotaExhausted(c.Quota) {
				if excludeSupervisor && p == sup {
					continue
				}
				return p
			}
		}
	}
	for _, c := range ranked {
		p := strings.ToLower(strings.TrimSpace(c.Provider))
		if p == "" || p == "ollama" {
			continue
		}
		if excludeSupervisor && p == sup {
			continue
		}
		if quotaExhausted(c.Quota) {
			continue
		}
		if p == "grok" {
			return p
		}
		if _, err := ProviderID(p); err == nil {
			return p
		}
	}
	return ""
}

func quotaExhausted(s quota.Snapshot) bool {
	return s.RemainingPct != nil && *s.RemainingPct <= 0
}

func (w Worker) DispatchCmd(workDir string) string {
	if w.Provider == "" || w.Via != ViaDispatch {
		return ""
	}
	role := w.Role
	if role == "" {
		role = "implement"
	}
	cmd := fmt.Sprintf("agentpick dispatch --role %s --prefer %s -p \"<self-contained task>\"", role, w.Provider)
	if d := strings.TrimSpace(workDir); d != "" {
		cmd = fmt.Sprintf("agentpick dispatch --role %s --prefer %s --dir %s -p \"<self-contained task>\"", role, w.Provider, d)
	}
	return cmd
}

func allWorkers(w Workers) []Worker {
	out := []Worker{w.Implement, w.Review, w.Tiny}
	return append(out, w.Extra...)
}

func (w Workers) Summary() string {
	var parts []string
	for _, wr := range allWorkers(w) {
		if wr.Provider == "" {
			continue
		}
		via := wr.Via
		if via == "" {
			via = ViaCAO
		}
		label := wr.Provider
		if wr.Profile != "" && via == ViaCAO {
			label = fmt.Sprintf("%s (%s)", wr.Provider, wr.Profile)
		}
		line := fmt.Sprintf("%s=%s via %s", wr.Role, label, via)
		if wr.Why != "" {
			line += " [" + wr.Why + "]"
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return "no extra workers (supervisor does the work)"
	}
	return strings.Join(parts, ", ")
}

func extraNames(w Workers) string {
	var names []string
	for _, wr := range w.Extra {
		if wr.Provider != "" {
			names = append(names, wr.Provider)
		}
	}
	return strings.Join(names, ",")
}
