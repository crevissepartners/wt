package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/worktree"
)

type removeResult struct {
	Path       string   `json:"path"`
	Branch     string   `json:"branch,omitempty"`
	Action     string   `json:"action"`
	Applied    bool     `json:"applied"`
	Removed    bool     `json:"removed"`
	Dirty      *bool    `json:"dirty"`
	DirtyKinds []string `json:"dirtyKinds,omitempty"`
}

func newRemoveCmd() *cobra.Command {
	var dryRun bool
	var force bool
	var forceDirty bool
	var jsonOut bool
	var tui bool

	cmd := &cobra.Command{
		Use:   "remove [query]",
		Short: "Remove a selected worktree",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completePathQuery(cmd, args, toComplete)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = strings.TrimSpace(args[0])
			}
			if query == "" && !tui {
				return usageError(fmt.Errorf("wt remove: query is required unless --tui is set"))
			}

			d, err := getDeps(cmd)
			if err != nil {
				return err
			}
			if tui && (d.CanUseTUI == nil || !d.CanUseTUI()) {
				return usageError(fmt.Errorf("wt remove: --tui requires a TTY on stdin and stderr"))
			}

			interactive := d.IsInteractive != nil && d.IsInteractive()
			if !dryRun && !force && !interactive {
				return usageError(fmt.Errorf("wt remove: requires --dry-run or --force"))
			}

			ctx := cmd.Context()
			repoRoot, err := git.RepoRoot(ctx, d.Runner, d.Cwd)
			if err != nil {
				return err
			}
			primaryRoot, err := git.PrimaryWorktreeRoot(ctx, d.Runner, repoRoot)
			if err != nil {
				return err
			}

			wts, err := git.WorktreeList(ctx, d.Runner, repoRoot)
			if err != nil {
				return err
			}

			chosen, err := selectRemoveTarget(cmd, d, wts, query, tui)
			if err != nil {
				return err
			}
			if err := validateRemoveTarget(repoRoot, primaryRoot, chosen); err != nil {
				return err
			}

			dirty := resolveWorktreeDirty(ctx, d, nil, chosen)
			result := removeResult{
				Path:       chosen.Path,
				Branch:     strings.TrimPrefix(chosen.Branch, "refs/heads/"),
				Action:     actionWouldRemove,
				Applied:    false,
				Removed:    false,
				Dirty:      dirty.JSONValue(),
				DirtyKinds: dirty.KindList(),
			}

			if dirty.BlocksRemoval() && !forceDirty {
				if !dryRun {
					return usageError(fmt.Errorf("wt remove: %s; use --force-dirty to remove anyway: %s", dirtyRefusalDetail(dirty), chosen.Path))
				}
				result.Action = actionSkip
				fmt.Fprintf(cmd.ErrOrStderr(), "note: %s; use --force-dirty to remove anyway\n", dirtyRefusalDetail(dirty))
				return writeRemoveResult(cmd, jsonOut, result)
			}

			if !dryRun && !force {
				confirmed, err := confirmRemove(cmd.InOrStdin(), cmd.ErrOrStderr(), result.Path, result.Branch)
				if err != nil {
					return err
				}
				if !confirmed {
					return &exitError{Code: 1, Err: fmt.Errorf("wt remove: aborted")}
				}
			}

			if !dryRun && (force || interactive) {
				if err := git.WorktreeRemove(ctx, d.Runner, repoRoot, chosen.Path, true); err != nil {
					return err
				}
				result.Action = actionRemoved
				result.Applied = true
				result.Removed = true
			}

			return writeRemoveResult(cmd, jsonOut, result)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview removal without changing anything")
	cmd.Flags().BoolVar(&force, "force", false, "actually remove the selected worktree")
	cmd.Flags().BoolVar(&forceDirty, "force-dirty", false, "allow removing a worktree that has uncommitted changes")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&tui, "tui", false, "use TUI selection when query is omitted or ambiguous")
	return cmd
}

func writeRemoveResult(cmd *cobra.Command, jsonOut bool, result removeResult) error {
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	line := fmt.Sprintf("%s  %s", result.Action, result.Path)
	if result.Branch != "" {
		line += fmt.Sprintf("  (%s)", result.Branch)
	}
	fmt.Fprintln(cmd.OutOrStdout(), line)
	return nil
}

// dirtyRefusalDetail explains why a remove target is not safe to delete.
func dirtyRefusalDetail(dirty dirtyState) string {
	if summary := dirty.KindSummary(); summary != "" {
		return fmt.Sprintf("worktree has uncommitted changes (%s)", summary)
	}
	return "cannot determine worktree status"
}

func selectRemoveTarget(cmd *cobra.Command, d *deps, wts []worktree.Worktree, query string, tui bool) (worktree.Worktree, error) {
	if !tui {
		return resolveMatchedWorktree("wt remove", wts, query)
	}

	matches := matchWorktrees(wts, query)
	switch {
	case query == "":
		return selectWorktreeWithTUI(cmd, d, "wt remove", wts, "")
	case len(matches) == 0:
		return worktree.Worktree{}, &exitError{Code: 1, Err: fmt.Errorf("wt remove: no matches for %q", query)}
	case len(matches) == 1:
		return matches[0], nil
	default:
		return selectWorktreeWithTUI(cmd, d, "wt remove", matches, query)
	}
}

func validateRemoveTarget(repoRoot string, primaryRoot string, chosen worktree.Worktree) error {
	if chosen.Path == repoRoot {
		return usageError(fmt.Errorf("wt remove: cannot remove current worktree: %s", chosen.Path))
	}
	if chosen.Path == primaryRoot {
		return usageError(fmt.Errorf("wt remove: cannot remove primary worktree: %s", chosen.Path))
	}
	if chosen.Prunable {
		return usageError(fmt.Errorf("wt remove: target is prunable; use 'wt prune --apply': %s", chosen.Path))
	}
	return nil
}

func confirmRemove(in io.Reader, out io.Writer, path string, branch string) (bool, error) {
	prompt := fmt.Sprintf("Remove worktree %s", path)
	if branch != "" {
		prompt += fmt.Sprintf(" (%s)", branch)
	}
	prompt += "? [y/N] "

	if _, err := fmt.Fprint(out, prompt); err != nil {
		return false, err
	}

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("wt remove: failed to read confirmation: %w", err)
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
