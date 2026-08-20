package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/marko-durasic/agentpick/internal/defaults"
	"github.com/marko-durasic/agentpick/internal/dispatch"
	"github.com/marko-durasic/agentpick/internal/history"
	"github.com/marko-durasic/agentpick/internal/quota"
	"github.com/marko-durasic/agentpick/internal/route"
	"github.com/spf13/cobra"
)

func newRouteCmd() *cobra.Command {
	var role string
	var exclude []string
	var prefer []string
	var asJSON bool
	var requireHealthy bool
	var skipQuota bool
	var taskClass string
	var lane string

	cmd := &cobra.Command{
		Use:   "route",
		Short: "Rank providers for a role (quota-aware orchestration)",
		Long: `Pick the best coding-agent CLI for a task role without launching it.

Roles: implement, review, plan, tiny, debug, orchestrator
Aliases: independent_review → review, idea_proposal → plan, tiny_task → tiny

Examples:
  agentpick route --role review
  agentpick route --role review --exclude cursor
  agentpick route --role implement --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := defaults.Load()
			if err != nil {
				return err
			}
			if strings.TrimSpace(role) == "" {
				return fmt.Errorf("--role is required (implement|review|plan|tiny|debug)")
			}
			ctx := context.Background()
			dec, err := route.Resolve(ctx, reg, route.Request{
				Role:           role,
				Exclude:        exclude,
				Prefer:         prefer,
				RequireHealthy: requireHealthy,
				SkipQuota:      skipQuota,
				TaskClass:      taskClass,
				Lane:           lane,
			})
			if err != nil {
				return err
			}
			_ = history.Append(dec)
			history.MaybeFeedRankings(dec)
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(dec)
			}
			printRouteHuman(cmd.OutOrStdout(), dec)
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "Task role (implement|review|plan|tiny|debug)")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Provider(s) to skip (e.g. cursor for author exclusion)")
	cmd.Flags().StringArrayVar(&prefer, "prefer", nil, "Override provider order")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Machine-readable JSON decision")
	cmd.Flags().BoolVar(&requireHealthy, "require-healthy", true, "Skip providers whose binary is missing")
	cmd.Flags().BoolVar(&skipQuota, "skip-quota", false, "Skip live quota probes")
	cmd.Flags().StringVar(&taskClass, "task-class", "", "Optional task class hint (#750)")
	cmd.Flags().StringVar(&lane, "lane", "", "Optional business lane (revenue|product|...)")
	return cmd
}

func newDispatchCmd(noHeadroom *bool) *cobra.Command {
	var role string
	var exclude []string
	var prefer []string
	var prompt string
	var promptFile string
	var workDir string
	var timeout string
	var dryRun bool
	var requireHealthy bool
	var taskClass string
	var lane string

	cmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Route and run a headless peer invoke",
		Long: `Route to the best provider for a role, then execute with allowlisted argv.

Examples:
  agentpick dispatch --role review --exclude cursor -p "review this diff"
  agentpick dispatch --role plan --prompt-file brief.md --dry-run
  agentpick dispatch --role implement --dir ./repo -p "fix the failing test"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := defaults.Load()
			if err != nil {
				return err
			}
			if strings.TrimSpace(role) == "" {
				return fmt.Errorf("--role is required")
			}
			if strings.TrimSpace(prompt) == "" && strings.TrimSpace(promptFile) == "" {
				return fmt.Errorf("provide -p/--prompt or --prompt-file")
			}
			dur := 8 * time.Minute
			if strings.TrimSpace(timeout) != "" {
				parsed, err := time.ParseDuration(timeout)
				if err != nil {
					return fmt.Errorf("invalid --timeout: %w", err)
				}
				dur = parsed
			}
			ctx := context.Background()
			res, err := dispatch.Run(ctx, reg, dispatch.Options{
				Role:           role,
				Exclude:        exclude,
				Prefer:         prefer,
				Prompt:         prompt,
				PromptFile:     promptFile,
				WorkDir:        workDir,
				Timeout:        dur,
				DryRun:         dryRun,
				NoHeadroom:     *noHeadroom,
				RequireHealthy: requireHealthy,
				TaskClass:      taskClass,
				Lane:           lane,
				RecordHistory:  true,
			})
			if err != nil {
				return err
			}
			if res.Err != nil && !dryRun {
				fmt.Fprintf(cmd.ErrOrStderr(), "dispatch: provider=%s exit=%d: %v\n", res.Provider, res.ExitCode, res.Err)
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "route=%s reason=%s\n", res.Decision.Provider, res.Decision.Reason)
				fmt.Fprintf(cmd.OutOrStdout(), "argv: %s\n", res.Output)
				return nil
			}
			if res.Output != "" {
				fmt.Fprint(cmd.OutOrStdout(), res.Output)
			}
			if res.Err != nil {
				return res.Err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&role, "role", "", "Task role")
	cmd.Flags().StringArrayVar(&exclude, "exclude", nil, "Providers to skip")
	cmd.Flags().StringArrayVar(&prefer, "prefer", nil, "Override provider order")
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Prompt text")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "Read prompt from file")
	cmd.Flags().StringVar(&workDir, "dir", "", "Working directory for the peer CLI")
	cmd.Flags().StringVar(&timeout, "timeout", "8m", "Max run time")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print resolved argv without executing")
	cmd.Flags().BoolVar(&requireHealthy, "require-healthy", true, "Skip missing binaries")
	cmd.Flags().StringVar(&taskClass, "task-class", "", "Optional task class (#750)")
	cmd.Flags().StringVar(&lane, "lane", "", "Optional business lane")
	return cmd
}

func printRouteHuman(out interface{ Write([]byte) (int, error) }, dec route.Decision) {
	fmt.Fprintf(out, "role=%s action=%s provider=%s\n", dec.Role, dec.Action, dec.Provider)
	fmt.Fprintf(out, "reason: %s\n", dec.Reason)
	if len(dec.Ranked) > 0 {
		fmt.Fprintf(out, "\nranked:\n")
		for i, c := range dec.Ranked {
			q := quota.FormatLabel(c.Quota)
			fmt.Fprintf(out, "  %d. %-8s score=%.1f model-rank=%d quota=%s %s\n",
				i+1, c.Provider, c.Score, c.Priority, q, c.Reason)
		}
	}
}
