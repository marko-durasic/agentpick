package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/history"
	"github.com/marko-durasic/agentpick/internal/launch"
	"github.com/marko-durasic/agentpick/internal/orchestrate"
	"github.com/marko-durasic/agentpick/internal/quota"
	"github.com/marko-durasic/agentpick/internal/route"
	"github.com/marko-durasic/agentpick/internal/tokensync"
	"github.com/spf13/cobra"
)

// NewRoot builds the agentpick command tree.
func NewRoot() *cobra.Command {
	var noHeadroom bool
	var noTokensave bool
	var dryRun bool

	root := &cobra.Command{
		Use:   "agentpick",
		Short: "Launch coding agents with bang-for-buck defaults",
		Long: `agentpick launches Cursor, Claude, Codex, Grok, Copilot, or Antigravity
with opinionated optimal model/effort settings.

With no arguments, agentpick is an orchestrator picker: it lists installed
CLIs, recommends one from quota + role knowledge, then starts that session
with instructions to delegate specialist work via agentpick dispatch.

When Headroom is installed, eligible providers run through
  headroom wrap <tool> …
so context stays compressed. Use --no-headroom to force the native CLI.

When tokensave is installed, agentpick runs
  tokensave sync
on every indexed project before launch so the code graph stays ready.
Use --no-tokensave to skip.

Global flags may appear before the provider name:
  agentpick --dry-run claude
  agentpick --no-headroom codex "fix tests"
  agentpick --no-tokensave grok`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := defaults.Load()
			if err != nil {
				return err
			}
			name, reason, err := pickOrchestrator(cmd.InOrStdin(), cmd.OutOrStdout(), reg)
			if err != nil {
				return err
			}
			opt := mergeOpts(noHeadroom, noTokensave, dryRun, nil)
			return runOrchestrator(reg, name, reason, opt)
		},
	}

	root.PersistentFlags().BoolVar(&noHeadroom, "no-headroom", false, "skip Headroom wrap; launch the native CLI")
	root.PersistentFlags().BoolVar(&noTokensave, "no-tokensave", false, "skip tokensave sync of indexed projects before launch")
	root.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print the resolved command without executing")

	root.AddCommand(newListCmd())
	root.AddCommand(newRouteCmd())
	root.AddCommand(newDispatchCmd(&noHeadroom))
	root.AddCommand(newProvidersCmd(&noHeadroom, &noTokensave, &dryRun)...)

	return root
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show providers and current optimal defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := defaults.Load()
			if err != nil {
				return err
			}
			hr := launch.HeadroomAvailable()
			ts := tokensync.Available()
			fmt.Fprintf(cmd.OutOrStdout(), "agentpick defaults (v%d, updated %s)\n", reg.Version, reg.Updated)
			fmt.Fprintf(cmd.OutOrStdout(), "headroom on PATH: %v\n", hr)
			fmt.Fprintf(cmd.OutOrStdout(), "tokensave on PATH: %v (preflight sync all projects)\n\n", ts)

			snaps := fetchQuotaFor(reg.Names())
			for _, name := range reg.Names() {
				p := reg.Providers[name]
				avail := "missing"
				if launch.Available(p) {
					avail = "installed"
				}
				wrap := "native only"
				if p.HeadroomWrap != "" {
					wrap = "headroom wrap " + p.HeadroomWrap
					if !hr {
						wrap += " (headroom missing → native fallback)"
					}
				}
				display := p.Display
				if display == "" {
					display = name
				}
				snap := snaps[name]
				fmt.Fprintf(cmd.OutOrStdout(), "  %-8s  %s (%s)\n", name, display, avail)
				fmt.Fprintf(cmd.OutOrStdout(), "            model:  %s\n", p.Summary)
				fmt.Fprintf(cmd.OutOrStdout(), "            launch: %s\n", wrap)
				fmt.Fprintf(cmd.OutOrStdout(), "            quota:  %s\n", quota.FormatLabel(snap))
				fmt.Fprintf(cmd.OutOrStdout(), "            why:    %s\n\n", p.Why)
			}
			fmt.Fprintln(cmd.OutOrStdout(), quota.FormatLegend(snaps))
			return nil
		},
	}
}

func newProvidersCmd(noHeadroom, noTokensave, dryRun *bool) []*cobra.Command {
	reg, err := defaults.Load()
	if err != nil {
		return []*cobra.Command{{
			Use: "error",
			RunE: func(cmd *cobra.Command, args []string) error {
				return err
			},
		}}
	}

	providerSet := map[string]struct{}{}
	for _, name := range reg.Names() {
		providerSet[name] = struct{}{}
	}

	cmds := make([]*cobra.Command, 0, len(reg.Providers))
	for _, name := range reg.Names() {
		name := name
		p := reg.Providers[name]
		display := p.Display
		if display == "" {
			display = name
		}
		c := &cobra.Command{
			Use:                name,
			Short:              fmt.Sprintf("%s — %s", display, p.Summary),
			Long:               fmt.Sprintf("%s\n\nWhy: %s\n\nExtra args are passed through to the agent CLI.", p.Summary, p.Why),
			DisableFlagParsing: true, // pass --effort etc. through untouched
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
					return cmd.Help()
				}
				reg, err := defaults.Load()
				if err != nil {
					return err
				}
				// DisableFlagParsing prevents Cobra from applying root persistent
				// flags that appear before the provider name; re-read from os.Args.
				fromArgsNoHR, fromArgsNoTS, fromArgsDry := globalsFromOSArgs(providerSet)
				extraNoHR, extraNoTS, extraDry, extra := stripGlobalFlags(args)
				opt := mergeOpts(
					*noHeadroom || fromArgsNoHR || extraNoHR,
					*noTokensave || fromArgsNoTS || extraNoTS,
					*dryRun || fromArgsDry || extraDry,
					extra,
				)
				return runProvider(reg, name, opt)
			},
		}
		cmds = append(cmds, c)
	}
	return cmds
}

func mergeOpts(noHeadroom, noTokensave, dryRun bool, extra []string) launch.Options {
	return launch.Options{
		NoHeadroom:  noHeadroom,
		NoTokensave: noTokensave,
		DryRun:      dryRun,
		ExtraArgs:   extra,
	}
}

// globalsFromOSArgs reads agentpick globals that appear before the provider
// subcommand. Needed because DisableFlagParsing skips normal parsing.
func globalsFromOSArgs(providers map[string]struct{}) (noHeadroom, noTokensave, dryRun bool) {
	args := os.Args[1:]
	for _, a := range args {
		if _, ok := providers[a]; ok {
			return noHeadroom, noTokensave, dryRun
		}
		if a == "--" {
			return noHeadroom, noTokensave, dryRun
		}
		switch a {
		case "--no-headroom":
			noHeadroom = true
		case "--no-tokensave":
			noTokensave = true
		case "--dry-run":
			dryRun = true
		}
	}
	return noHeadroom, noTokensave, dryRun
}

// stripGlobalFlags removes agentpick-owned flags that Cobra may forward into
// provider argv when DisableFlagParsing is set.
func stripGlobalFlags(args []string) (noHeadroom, noTokensave, dryRun bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--no-headroom":
			noHeadroom = true
		case "--no-tokensave":
			noTokensave = true
		case "--dry-run":
			dryRun = true
		default:
			rest = append(rest, a)
		}
	}
	return noHeadroom, noTokensave, dryRun, rest
}

func runProvider(reg *defaults.Registry, name string, opt launch.Options) error {
	p, ok := reg.Get(name)
	if !ok {
		return fmt.Errorf("unknown provider %q (try: agentpick list)", name)
	}
	syncRes := tokensync.SyncAll(tokensync.Options{
		NoTokensave: opt.NoTokensave,
		DryRun:      opt.DryRun,
		Out:         os.Stderr,
	})
	if syncRes.Skipped != "" {
		fmt.Fprintf(os.Stderr, "agentpick: %s\n", syncRes.Skipped)
	}
	plan, err := launch.Resolve(p, opt)
	if err != nil {
		return err
	}
	return launch.Exec(plan, opt)
}

func runOrchestrator(reg *defaults.Registry, name, reason string, opt launch.Options) error {
	briefPath, err := orchestrate.WriteBrief(name, reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, "agentpick: orchestrator brief: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "agentpick: orchestrator=%s\n", name)
		fmt.Fprintf(os.Stderr, "agentpick: brief=%s\n", briefPath)
		fmt.Fprintf(os.Stderr, "agentpick: this session will delegate via agentpick dispatch as needed\n")
		opt.ExtraArgs = append(orchestrate.ExtraArgs(name, briefPath), opt.ExtraArgs...)
		opt.ExtraEnv = orchestrate.EnvVars(name, briefPath)
	}
	return runProvider(reg, name, opt)
}

func pickOrchestrator(in io.Reader, out io.Writer, reg *defaults.Registry) (string, string, error) {
	type row struct {
		name string
		p    defaults.Provider
	}
	var rows []row
	for _, name := range reg.Names() {
		p := reg.Providers[name]
		if !launch.Available(p) {
			continue
		}
		rows = append(rows, row{name: name, p: p})
	}
	if len(rows) == 0 {
		return "", "", fmt.Errorf("no supported agent CLIs found on PATH (install cursor-agent, claude, codex, grok, copilot, and/or agy)")
	}

	ctx := context.Background()
	dec, err := route.Resolve(ctx, reg, route.Request{
		Role:           "orchestrator",
		RequireHealthy: true,
	})
	if err != nil {
		return "", "", err
	}
	_ = history.Append(dec)
	history.MaybeFeedRankings(dec)

	recommended := dec.Provider
	reason := dec.Reason
	if recommended == "" {
		recommended = rows[0].name
		reason = "first installed CLI"
	}

	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.name
	}
	snaps := fetchQuotaFor(names)
	defaultIdx := 1
	for i, r := range rows {
		if r.name == recommended {
			defaultIdx = i + 1
			break
		}
	}

	fmt.Fprintln(out, "Select an orchestrator")
	fmt.Fprintln(out, "Enter starts a vibe-coding session that can delegate to other CLIs.")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Recommended: %s\n", recommended)
	if reason != "" {
		fmt.Fprintf(out, "  Why: %s\n", reason)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+quota.FormatPickerHeader())
	fmt.Fprintln(out, "  "+strings.Repeat("─", 78))
	for i, r := range rows {
		snap := snaps[r.name]
		line := quota.FormatPickerRow(i+1, r.name, r.p.Summary, snap)
		if r.name == recommended {
			line += "  ★ recommended orchestrator"
		}
		fmt.Fprintln(out, "  "+line)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, quota.FormatLegend(snaps))
	fmt.Fprintf(out, "Choice [%d]: ", defaultIdx)

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", "", err
		}
		return recommended, reason, nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return recommended, reason, nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(rows) {
		for _, r := range rows {
			if strings.EqualFold(line, r.name) {
				return r.name, "manual pick", nil
			}
		}
		return "", "", fmt.Errorf("invalid choice %q", line)
	}
	picked := rows[n-1].name
	if picked == recommended {
		return picked, reason, nil
	}
	return picked, "manual pick", nil
}

func fetchQuotaFor(names []string) map[string]quota.Snapshot {
	return quota.FetchAll(context.Background(), quota.FetchOptions{Providers: names})
}

// Execute runs the root command.
func Execute() {
	if err := NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "agentpick:", err)
		os.Exit(1)
	}
}
