package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/marko-durasic/agentpick/internal/defaults"
)

// Options controls how a provider is launched.
type Options struct {
	NoHeadroom  bool
	NoTokensave bool
	DryRun      bool
	ExtraArgs   []string
}

// Plan is the resolved argv + env for a launch.
type Plan struct {
	Argv          []string
	Env           []string
	UsedHeadroom  bool
	HeadroomSkipped string // reason when wrap was possible but skipped / unavailable
}

// Resolve builds the launch plan for a provider.
func Resolve(p defaults.Provider, opt Options) (Plan, error) {
	if p.Binary == "" {
		return Plan{}, fmt.Errorf("provider has empty binary")
	}

	extra := append([]string{}, opt.ExtraArgs...)
	env := os.Environ()
	for k, v := range p.Env {
		env = setEnv(env, k, v)
	}

	headroomPath, err := lookPath("headroom")
	headroomOK := err == nil
	wantWrap := p.HeadroomWrap != "" && !opt.NoHeadroom

	if wantWrap && headroomOK {
		argv := []string{headroomPath, "wrap", p.HeadroomWrap}
		argv = append(argv, p.HeadroomFlags...)
		argv = append(argv, "--")
		argv = append(argv, p.Passthrough...)
		argv = append(argv, extra...)
		return Plan{Argv: argv, Env: env, UsedHeadroom: true}, nil
	}

	binPath, err := lookPath(p.Binary)
	if err != nil {
		return Plan{}, fmt.Errorf("%s not found on PATH (install the CLI, or add it to PATH)", p.Binary)
	}

	argv := []string{binPath}
	argv = append(argv, p.Passthrough...)
	argv = append(argv, extra...)

	plan := Plan{Argv: argv, Env: env, UsedHeadroom: false}
	switch {
	case p.HeadroomWrap == "":
		// Provider has no Headroom integration (e.g. agy).
	case opt.NoHeadroom:
		plan.HeadroomSkipped = "forced by --no-headroom"
	case !headroomOK:
		plan.HeadroomSkipped = "headroom not on PATH; launching native CLI"
	}
	return plan, nil
}

// Exec runs the plan (replaces the current process when not DryRun).
func Exec(plan Plan, opt Options) error {
	if len(plan.Argv) == 0 {
		return fmt.Errorf("empty argv")
	}
	if plan.HeadroomSkipped != "" {
		fmt.Fprintf(os.Stderr, "agentpick: %s\n", plan.HeadroomSkipped)
	}
	if opt.DryRun {
		fmt.Printf("dry-run: %s\n", shellJoin(plan.Argv))
		for k, v := range envDiff(plan.Env) {
			fmt.Printf("dry-run env: %s=%s\n", k, v)
		}
		return nil
	}
	cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
	cmd.Env = plan.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Available reports whether the provider binary exists on PATH.
func Available(p defaults.Provider) bool {
	_, err := lookPath(p.Binary)
	return err == nil
}

// HeadroomAvailable reports whether headroom is on PATH.
func HeadroomAvailable() bool {
	_, err := lookPath("headroom")
	return err == nil
}

func lookPath(bin string) (string, error) {
	return exec.LookPath(bin)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			out = append(out, prefix+value)
			found = true
			continue
		}
		out = append(out, e)
	}
	if !found {
		out = append(out, prefix+value)
	}
	return out
}

func envDiff(env []string) map[string]string {
	// Surface provider overrides (not the full environ dump).
	interesting := map[string]struct{}{
		"ANTHROPIC_MODEL":                    {},
		"COPILOT_PROVIDER_MODEL_ID":          {},
		"COPILOT_PROVIDER_MAX_PROMPT_TOKENS": {},
		"COPILOT_PROVIDER_MAX_OUTPUT_TOKENS": {},
	}
	out := map[string]string{}
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if _, keep := interesting[k]; keep {
			out[k] = v
		}
	}
	return out
}

func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if strings.ContainsAny(a, " \t\"'") {
			parts[i] = fmt.Sprintf("%q", a)
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}
