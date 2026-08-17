package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/history"
	"github.com/marko-durasic/agentpick/internal/launch"
	"github.com/marko-durasic/agentpick/internal/quota"
	"github.com/marko-durasic/agentpick/internal/route"
)

// Options controls headless dispatch.
type Options struct {
	Role           string
	Exclude        []string
	Prefer         []string
	Prompt         string
	PromptFile     string
	WorkDir        string
	Timeout        time.Duration
	DryRun         bool
	NoHeadroom     bool
	RequireHealthy bool
	TaskClass      string
	Lane           string
	RecordHistory  bool
}

// Result is the outcome of dispatch.
type Result struct {
	Decision route.Decision
	Provider string
	Output   string
	ExitCode int
	Argv     []string
	Err      error
}

const (
	kindReview    = "review"
	kindPropose   = "plan"
	kindTiny      = "tiny"
	kindImplement = "implement"
)

// Run routes then executes the best peer with allowlisted argv.
func Run(ctx context.Context, reg *defaults.Registry, opt Options) (Result, error) {
	prompt, err := resolvePrompt(opt)
	if err != nil {
		return Result{}, err
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 8 * time.Minute
	}

	req := route.Request{
		Role:           opt.Role,
		Exclude:        opt.Exclude,
		Prefer:         opt.Prefer,
		RequireHealthy: opt.RequireHealthy,
		TaskClass:      opt.TaskClass,
		Lane:           opt.Lane,
	}
	dec, err := route.Resolve(ctx, reg, req)
	if err != nil {
		return Result{}, err
	}
	if opt.RecordHistory {
		_ = history.Append(dec)
		history.MaybeFeedRankings(dec)
	}

	if dec.Action != "use" || dec.Provider == "" {
		return Result{Decision: dec, Err: fmt.Errorf("route deferred: %s", dec.Reason)}, nil
	}

	candidates := dec.Ranked
	if len(candidates) == 0 {
		candidates = []route.Candidate{{Provider: dec.Provider, Healthy: true}}
	}

	var last Result
	for _, c := range candidates {
		if opt.RequireHealthy && !c.Healthy {
			continue
		}
		res := invokeOne(ctx, reg, c.Provider, opt.Role, prompt, opt)
		res.Decision = dec
		if res.Err == nil && !quota.IsQuotaExhausted(res.Output) {
			return res, nil
		}
		if quota.IsQuotaExhausted(res.Output) {
			last = res
			continue
		}
		if res.Err == nil {
			return res, nil
		}
		last = res
	}
	if last.Err != nil {
		return last, last.Err
	}
	return last, nil
}

func resolvePrompt(opt Options) (string, error) {
	if strings.TrimSpace(opt.Prompt) != "" {
		return strings.TrimSpace(opt.Prompt), nil
	}
	if strings.TrimSpace(opt.PromptFile) == "" {
		return "", errors.New("prompt or --prompt-file required")
	}
	b, err := os.ReadFile(opt.PromptFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func invokeOne(ctx context.Context, reg *defaults.Registry, provider, role, prompt string, opt Options) Result {
	p, ok := reg.Get(provider)
	if !ok {
		return Result{Provider: provider, Err: fmt.Errorf("unknown provider %s", provider)}
	}
	kind := dispatchKind(role)
	extra, err := providerArgs(provider, kind, prompt)
	if err != nil {
		return Result{Provider: provider, Err: err}
	}
	launchOpt := launch.Options{
		NoHeadroom: opt.NoHeadroom,
		DryRun:     opt.DryRun,
		ExtraArgs:  extra,
	}
	plan, err := launch.Resolve(p, launchOpt)
	if err != nil {
		return Result{Provider: provider, Err: err}
	}
	res := Result{Provider: provider, Argv: plan.Argv}
	if opt.DryRun {
		res.Output = launch.FormatArgv(plan.Argv)
		return res
	}
	runCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, plan.Argv[0], plan.Argv[1:]...)
	cmd.Env = plan.Env
	if wd := strings.TrimSpace(opt.WorkDir); wd != "" {
		cmd.Dir = wd
	}
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	err = cmd.Run()
	res.Output = combined.String()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		}
		res.Err = err
	}
	return res
}

func dispatchKind(role string) string {
	switch route.NormalizeRole(role) {
	case "review":
		return kindReview
	case "plan":
		return kindPropose
	case "tiny":
		return kindTiny
	default:
		return kindImplement
	}
}

// providerArgs — allowlisted argv per DuReef invoke_review / implement_args.
func providerArgs(provider, kind, prompt string) ([]string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("empty prompt")
	}
	switch provider {
	case "codex":
		if kind == kindReview {
			return []string{"exec", "review", "--uncommitted"}, nil
		}
		return []string{"exec", "--sandbox", "read-only", prompt}, nil
	case "claude":
		if kind == kindPropose {
			return []string{"-p", prompt, "--permission-mode", "plan", "--allowedTools", "WebSearch WebFetch", "--strict-mcp-config", "--output-format", "text"}, nil
		}
		if kind == kindImplement {
			return []string{"-p", prompt, "--permission-mode", "acceptEdits", "--allowedTools", "Edit Write Bash Read Glob Grep", "--strict-mcp-config", "--output-format", "text"}, nil
		}
		return []string{"-p", prompt, "--permission-mode", "plan", "--allowedTools", "", "--output-format", "text"}, nil
	case "agy":
		return []string{"-p", prompt, "--mode", "plan", "--output-format", "text"}, nil
	case "cursor":
		if kind == kindImplement {
			return []string{"-p", prompt, "--force", "--trust", "--output-format", "text"}, nil
		}
		return []string{"-p", prompt, "--mode", "plan", "--trust", "--output-format", "text"}, nil
	case "grok":
		return []string{"-p", prompt, "--permission-mode", "plan", "--output-format", "plain"}, nil
	case "copilot":
		if kind == kindImplement {
			return []string{"--allow-all-tools", "-p", prompt}, nil
		}
		return []string{"-p", prompt}, nil
	case "ollama":
		return []string{"run", "qwen3.5:4b", prompt}, nil
	default:
		return nil, fmt.Errorf("provider not eligible for dispatch: %s", provider)
	}
}

// WriteDecisionSidecar writes route_decision.json for audit.
func WriteDecisionSidecar(dir string, dec route.Decision) error {
	if strings.TrimSpace(dir) == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, "route_decision.json")
	type wire struct {
		Role     string `json:"role"`
		Provider string `json:"provider"`
		Reason   string `json:"reason"`
		Action   string `json:"action"`
	}
	b, err := json.Marshal(wire{
		Role:     dec.Role,
		Provider: dec.Provider,
		Reason:   dec.Reason,
		Action:   dec.Action,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
