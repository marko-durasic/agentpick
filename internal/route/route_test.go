package route

import (
	"context"
	"testing"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/quota"
)

func TestNormalizeRole(t *testing.T) {
	if got := NormalizeRole("independent_review"); got != "review" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeRole("IMPLEMENT"); got != "implement" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveReviewExcludesCursor(t *testing.T) {
	reg, err := defaults.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ctx := context.Background()
	dec, err := Resolve(ctx, reg, Request{
		Role:           "review",
		Exclude:        []string{"cursor", "cursor-agent"},
		RequireHealthy: false,
		SkipQuota:      true,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Action != "use" {
		t.Fatalf("action=%s reason=%s", dec.Action, dec.Reason)
	}
	if dec.Provider == "cursor" {
		t.Fatalf("expected non-cursor, got %s", dec.Provider)
	}
	// First in review order after exclude should be codex or claude
	if dec.Provider != "codex" && dec.Provider != "claude" {
		t.Fatalf("unexpected provider %s", dec.Provider)
	}
}

func TestResolveImplementPrefersCursorInOrder(t *testing.T) {
	reg, err := defaults.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dec, err := Resolve(context.Background(), reg, Request{
		Role:           "implement",
		RequireHealthy: false,
		SkipQuota:      true,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "cursor" {
		t.Fatalf("want cursor first in implement order, got %s", dec.Provider)
	}
}

func TestScoreCandidateQuotaTieBreak(t *testing.T) {
	high := quota.Snapshot{RemainingPct: ptrFloat(80), Window: "week"}
	low := quota.Snapshot{RemainingPct: ptrFloat(20), Window: "week"}
	sHigh := scoreCandidate(2, high)
	sLow := scoreCandidate(2, low)
	if sHigh <= sLow {
		t.Fatalf("higher quota should score higher: %v vs %v", sHigh, sLow)
	}
}

func TestResolveUsesPrefetchedFleetQuota(t *testing.T) {
	reg, err := defaults.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dec, err := Resolve(context.Background(), reg, Request{
		Role: "implement",
		Quota: map[string]quota.Snapshot{
			"cursor": {Provider: "cursor", RemainingPct: ptrFloat(0)},
			"claude": {Provider: "claude", RemainingPct: ptrFloat(100)},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if dec.Provider != "claude" {
		t.Fatalf("prefetched quota should balance toward claude, got %s (%s)", dec.Provider, dec.Reason)
	}
}

func TestResolveMarksHardStartupFailureUnhealthy(t *testing.T) {
	reg := &defaults.Registry{
		RoleOrders: map[string][]string{"implement": {"broken"}},
		Providers: map[string]defaults.Provider{
			"broken": {Binary: "sh", Roles: []string{"implement"}},
		},
	}
	dec, err := Resolve(context.Background(), reg, Request{
		Role: "implement",
		Quota: map[string]quota.Snapshot{
			"broken": {Provider: "broken", UnavailableReason: "BYOK mode without model"},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Ranked) != 1 || dec.Ranked[0].Healthy {
		t.Fatalf("hard startup failure should be unhealthy: %+v", dec.Ranked)
	}
}

func TestResolveMarksCopilotMissingModelUnhealthy(t *testing.T) {
	reg, err := defaults.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dec, err := Resolve(context.Background(), reg, Request{
		Role:   "implement",
		Prefer: []string{"copilot"},
		Quota: map[string]quota.Snapshot{
			"copilot": {Provider: "copilot", Source: "unknown", UnavailableReason: "copilot in BYOK mode without model (set COPILOT_MODEL)"},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(dec.Ranked) != 1 || dec.Ranked[0].Healthy {
		t.Fatalf("copilot startup refusal should be unhealthy: %+v", dec.Ranked)
	}
}

func ptrFloat(f float64) *float64 { return &f }
