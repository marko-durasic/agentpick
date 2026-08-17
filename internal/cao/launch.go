package cao

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Options controls a CAO session started by agentpick.
type Options struct {
	Provider    string // agentpick name (cursor, claude, …)
	WorkDir     string
	BriefPath   string
	Workers     Workers
	DryRun      bool
	SkipServer  bool // tests / when server is already known up
	SkipInstall bool
}

// Plan is the argv to exec after cao-server is up.
type Plan struct {
	ServerArgv  []string
	LaunchArgv  []string
	Host        string
	Port        int
	SessionName string
}

// ResolveHostPort returns loopback CAO bind settings.
func ResolveHostPort() (host string, port int, err error) {
	host = strings.TrimSpace(os.Getenv("AGENTPICK_CAO_HOST"))
	if host == "" {
		host = DefaultHost
	}
	if !isLoopbackHost(host) {
		return "", 0, fmt.Errorf("CAO host must be loopback (127.0.0.1/localhost), got %q", host)
	}
	port = DefaultPort
	if v := strings.TrimSpace(os.Getenv("AGENTPICK_CAO_PORT")); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 || n > 65535 {
			return "", 0, fmt.Errorf("invalid AGENTPICK_CAO_PORT %q", v)
		}
		port = n
	}
	if port == oauthPort || port == headroomPort {
		return "", 0, fmt.Errorf("CAO port %d is reserved (8787 Cursor OAuth, 8788 Headroom); use %d", port, DefaultPort)
	}
	return host, port, nil
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Resolve builds cao-server + cao launch argv. Never includes --yolo.
func Resolve(opt Options) (Plan, error) {
	host, port, err := ResolveHostPort()
	if err != nil {
		return Plan{}, err
	}
	caoID, err := ProviderID(opt.Provider)
	if err != nil {
		return Plan{}, err
	}
	caoBin, err := exec.LookPath("cao")
	if err != nil {
		return Plan{}, fmt.Errorf("cao not on PATH; install pinned cli-agent-orchestrator==%s", PinVersion)
	}
	serverBin, err := exec.LookPath("cao-server")
	if err != nil {
		return Plan{}, fmt.Errorf("cao-server not on PATH; install pinned cli-agent-orchestrator==%s", PinVersion)
	}
	wd := strings.TrimSpace(opt.WorkDir)
	if wd == "" {
		choice, werr := ResolveWorkDir("")
		if werr != nil {
			return Plan{}, werr
		}
		wd = choice.Path
	}
	session := sessionName()
	serverArgv := []string{serverBin, "--host", host, "--port", strconv.Itoa(port), "--terminal", "tmux"}
	launchArgv := []string{
		caoBin, "launch",
		"--agents", SupervisorProfile,
		"--session-name", session,
		"--provider", caoID,
		"--working-directory", wd,
		"--env", "AGENTPICK_ORCHESTRATOR=1",
		"--env", "AGENTPICK_ORCHESTRATOR_PROVIDER=" + opt.Provider,
	}
	if p := strings.TrimSpace(opt.BriefPath); p != "" {
		launchArgv = append(launchArgv, "--env", "AGENTPICK_ORCHESTRATOR_BRIEF="+p)
	}
	if opt.Workers.Implement.Provider != "" {
		launchArgv = append(launchArgv, "--env", "AGENTPICK_WORKER_IMPLEMENT="+opt.Workers.Implement.Provider)
	}
	if opt.Workers.Review.Provider != "" {
		launchArgv = append(launchArgv, "--env", "AGENTPICK_WORKER_REVIEW="+opt.Workers.Review.Provider)
	}
	if fleet := extraNames(opt.Workers); fleet != "" {
		launchArgv = append(launchArgv, "--env", "AGENTPICK_WORKER_FLEET="+fleet)
	}
	launchArgv = append(launchArgv, "Workers in this session are the full healthy fleet (agentpick_dev / agentpick_review plus extra panes like agentpick_agy). Divide and conquer: send_message the best worker by role and leftover usage, including Grok via the dispatch command when listed. Workers send_message you back. Do not do specialist work yourself. Slash commands like /start work. Do not ask me to run agentpick route. Wait for my task.")
	for _, a := range launchArgv {
		if a == "--yolo" || strings.HasPrefix(a, "--yolo=") {
			return Plan{}, fmt.Errorf("internal error: --yolo must never be passed")
		}
	}
	return Plan{ServerArgv: serverArgv, LaunchArgv: launchArgv, Host: host, Port: port, SessionName: session}, nil
}

// Available reports whether CAO + tmux are on PATH.
func Available() bool {
	if _, err := exec.LookPath("cao"); err != nil {
		return false
	}
	if _, err := exec.LookPath("cao-server"); err != nil {
		return false
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return false
	}
	return true
}

// CheckPin fails if `cao --version` is not the dogfood PyPI pin.
func CheckPin() error {
	out, err := exec.Command("cao", "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("cao --version: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), PinVersion) {
		return fmt.Errorf("cao version %q (want %s). Reinstall: uv tool install cli-agent-orchestrator==%s — do not `cao update` to @main", strings.TrimSpace(string(out)), PinVersion, PinVersion)
	}
	return nil
}

// MissingHint explains how to install the dogfood stack.
func MissingHint() string {
	return fmt.Sprintf("CAO dogfood needs tmux, uv, and pinned cli-agent-orchestrator==%s (cao-server --host 127.0.0.1 --port %d). Single-CLI: agentpick --no-cao, or agentpick <provider>.", PinVersion, DefaultPort)
}

// Run ensures cao-server then execs cao launch (or prints argv on DryRun).
func Run(ctx context.Context, opt Options) error {
	plan, err := Resolve(opt)
	if err != nil {
		return err
	}
	if err := CheckPin(); err != nil {
		if opt.DryRun {
			fmt.Fprintf(os.Stderr, "agentpick: %v\n", err)
		} else {
			return err
		}
	}
	if opt.DryRun {
		fmt.Printf("dry-run: %s\n", shellJoin(plan.ServerArgv))
		fmt.Printf("dry-run: %s\n", shellJoin(plan.LaunchArgv))
		if s := opt.Workers.Summary(); s != "" {
			fmt.Printf("dry-run workers: %s\n", s)
		}
		for _, wr := range caoAssignable(opt.Workers) {
			fmt.Printf("dry-run spawn: %s\n", spawnTerminalURL(loopbackBase(plan.Host, plan.Port), plan.SessionName, wr.Profile, wr.CAOProvider, opt.WorkDir))
		}
		return nil
	}
	if !opt.SkipInstall {
		if err := InstallSessionProfiles(opt.Provider, opt.Workers, opt.WorkDir); err != nil {
			return err
		}
	}
	if !opt.SkipServer {
		if err := EnsureServer(ctx, plan); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "agentpick: CAO session %s — http://%s:%d (new agentpick = new session; AGENTPICK_CAO_SESSION to reuse)\n", plan.SessionName, plan.Host, plan.Port)
	spawnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		if err := SpawnSessionWorkers(spawnCtx, plan, opt); err != nil && spawnCtx.Err() == nil {
			fmt.Fprintf(os.Stderr, "agentpick: spawn workers: %v\n", err)
		}
	}()
	cmd := exec.CommandContext(ctx, plan.LaunchArgv[0], plan.LaunchArgv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func shellJoin(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if strings.ContainsAny(a, " \t\n\"'") {
			out[i] = strconv.Quote(a)
			continue
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}
