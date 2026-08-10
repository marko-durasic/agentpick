package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/marko-durasic/agentpick/internal/defaults"
)

// DefaultHeadroomPort is the DuReef shared Headroom proxy port.
// Cursor mcp login OAuth owns :8787 — Headroom must never latch there.
const DefaultHeadroomPort = 8788

// OAuthCallbackPort is Cursor's hardcoded mcp login callback (do not use for Headroom).
const OAuthCallbackPort = 8787

// Options controls how a provider is launched.
type Options struct {
	NoHeadroom    bool
	NoTokensave   bool
	DryRun        bool
	ExtraArgs     []string
	HeadroomPort  int // 0 = resolve from env / default
}

// Plan is the resolved argv + env for a launch.
type Plan struct {
	Argv            []string
	Env             []string
	UsedHeadroom    bool
	HeadroomPort    int
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
	port := ResolveHeadroomPort(opt.HeadroomPort)

	if wantWrap && headroomOK {
		argv := BuildHeadroomArgv(headroomPath, p.HeadroomWrap, port, p.HeadroomFlags, p.Passthrough, extra)
		return Plan{Argv: argv, Env: env, UsedHeadroom: true, HeadroomPort: port}, nil
	}

	binPath, err := lookPath(p.Binary)
	if err != nil {
		return Plan{}, fmt.Errorf("%s not found on PATH (install the CLI, or add it to PATH)", p.Binary)
	}

	argv := []string{binPath}
	argv = append(argv, p.Passthrough...)
	argv = append(argv, extra...)

	plan := Plan{Argv: argv, Env: env, UsedHeadroom: false, HeadroomPort: port}
	switch {
	case p.HeadroomWrap == "":
		// Provider has no Headroom integration (e.g. agy, grok native).
	case opt.NoHeadroom:
		plan.HeadroomSkipped = "forced by --no-headroom"
	case !headroomOK:
		plan.HeadroomSkipped = "headroom not on PATH; launching native CLI"
	}
	return plan, nil
}

// ResolveHeadroomPort picks the shared Headroom proxy port.
// Order: explicit > DUREEF_HEADROOM_PORT > HEADROOM_PORT > DefaultHeadroomPort (8788).
// A resolved :8787 is remapped to :8788 so Cursor OAuth callback is never stolen.
func ResolveHeadroomPort(explicit int) int {
	port := explicit
	if port <= 0 {
		port = envPort("DUREEF_HEADROOM_PORT")
	}
	if port <= 0 {
		port = envPort("HEADROOM_PORT")
	}
	if port <= 0 {
		port = DefaultHeadroomPort
	}
	if port == OAuthCallbackPort {
		// Never latch Headroom onto Cursor mcp login OAuth callback.
		return DefaultHeadroomPort
	}
	return port
}

// BuildHeadroomArgv builds:
//
//	headroom wrap <tool> --port <N> [headroom_flags…] -- [passthrough…] [extra…]
//
// Always uses long-form --port (never bare -p) so wrap tools whose short -p
// means something else (or that pass -p through) cannot mis-route the proxy.
func BuildHeadroomArgv(headroomPath, tool string, port int, headroomFlags, passthrough, extra []string) []string {
	if port <= 0 {
		port = DefaultHeadroomPort
	}
	if port == OAuthCallbackPort {
		port = DefaultHeadroomPort
	}
	argv := []string{headroomPath, "wrap", tool, "--port", strconv.Itoa(port)}
	argv = append(argv, sanitizeHeadroomFlags(headroomFlags)...)
	argv = append(argv, "--")
	argv = append(argv, passthrough...)
	argv = append(argv, extra...)
	return argv
}

// sanitizeHeadroomFlags drops bare -p / --port pairs from YAML flags so the
// programmatically injected --port is the single source of truth.
func sanitizeHeadroomFlags(flags []string) []string {
	if len(flags) == 0 {
		return nil
	}
	out := make([]string, 0, len(flags))
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		switch {
		case f == "-p" || f == "--port":
			// Skip flag and its value if present as a separate arg.
			if i+1 < len(flags) && !strings.HasPrefix(flags[i+1], "-") {
				i++
			}
			continue
		case strings.HasPrefix(f, "-p=") || strings.HasPrefix(f, "--port="):
			continue
		default:
			out = append(out, f)
		}
	}
	return out
}

func envPort(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return 0
	}
	return n
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
