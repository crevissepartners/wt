package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/worktree"
)

type createOpts struct {
	Path   string
	Root   string
	From   string
	DryRun bool
}

func newCreateCmd() *cobra.Command {
	var opts createOpts

	cmd := &cobra.Command{
		Use:           "create <branch>",
		Short:         "Create a new worktree for a branch",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := strings.TrimSpace(args[0])
			if branch == "" {
				return usageError(fmt.Errorf("wt create: branch cannot be empty"))
			}

			d, err := getDeps(cmd)
			if err != nil {
				return err
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

			path, err := createWorktree(ctx, d, repoRoot, primaryRoot, branch, opts)
			if err != nil {
				return err
			}

			// stdout must be path-only to allow scripting/cd chaining.
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Path, "path", "", "worktree path")
	cmd.Flags().StringVar(&opts.Root, "root", "", "worktree root for default path resolution")
	cmd.Flags().StringVar(&opts.From, "from", "", "start point for new branch (default: origin/HEAD or main)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print what would be executed (no changes)")
	return cmd
}

func createWorktree(ctx context.Context, d *deps, repoRoot string, primaryRoot string, branch string, opts createOpts) (string, error) {
	wts, err := git.WorktreeList(ctx, d.Runner, repoRoot)
	if err != nil {
		return "", err
	}
	return createWorktreeFromList(ctx, d, repoRoot, primaryRoot, "wt create", branch, opts, wts)
}

func createWorktreeFromList(ctx context.Context, d *deps, repoRoot string, primaryRoot string, commandName string, branch string, opts createOpts, wts []worktree.Worktree) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", usageError(fmt.Errorf("wt create: branch cannot be empty"))
	}
	if err := ensureCreateQuerySafe(commandName, branch, branch, wts); err != nil {
		return "", err
	}

	targetPath, err := resolveCreateTargetPath(ctx, d, repoRoot, primaryRoot, branch, opts)
	if err != nil {
		return "", err
	}
	if err := preflightCreateTargetPath(commandName, targetPath); err != nil {
		return "", err
	}
	for _, wt := range wts {
		if wt.Branch == "refs/heads/"+branch && wt.Path != "" {
			return wt.Path, nil
		}
	}

	postCreate, err := loadPostCreateConfig(primaryRoot, d.ReadFile)
	if err != nil {
		return "", err
	}

	// If the local branch exists, we just add a worktree for it.
	localExists, err := git.RefExists(ctx, d.Runner, repoRoot, "refs/heads/"+branch)
	if err != nil {
		return "", err
	}

	if localExists {
		if opts.DryRun {
			fmt.Fprintf(os.Stderr, "dry-run: git worktree add %s %s\n", targetPath, branch)
			if err := applyPostCreateSymlinks(primaryRoot, targetPath, postCreate, true); err != nil {
				return "", err
			}
			return targetPath, nil
		}
		if err := git.WorktreeAddExistingBranch(ctx, d.Runner, repoRoot, targetPath, branch); err != nil {
			return "", err
		}
		if err := applyPostCreateSymlinks(primaryRoot, targetPath, postCreate, false); err != nil {
			return "", err
		}
		return targetPath, nil
	}

	from := strings.TrimSpace(opts.From)
	if from == "" {
		from = git.DefaultBaseRef(ctx, d.Runner, repoRoot)
	}

	// If user explicitly provided --from, it takes precedence.
	startPoint := from
	if strings.TrimSpace(opts.From) == "" {
		remoteExists, err := git.RefExists(ctx, d.Runner, repoRoot, "refs/remotes/origin/"+branch)
		if err != nil {
			return "", err
		}
		if remoteExists {
			startPoint = "origin/" + branch
		}
	}

	exists, err := git.RefExists(ctx, d.Runner, repoRoot, startPoint)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", usageError(fmt.Errorf("wt create: start point does not exist: %s", startPoint))
	}

	if opts.DryRun {
		fmt.Fprintf(os.Stderr, "dry-run: git worktree add -b %s %s %s\n", branch, targetPath, startPoint)
		if err := applyPostCreateSymlinks(primaryRoot, targetPath, postCreate, true); err != nil {
			return "", err
		}
		return targetPath, nil
	}

	if err := git.WorktreeAddNewBranch(ctx, d.Runner, repoRoot, targetPath, branch, startPoint); err != nil {
		return "", err
	}
	if err := applyPostCreateSymlinks(primaryRoot, targetPath, postCreate, false); err != nil {
		return "", err
	}
	return targetPath, nil
}

func safeBranchPathPart(branch string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("wt create: branch cannot be empty")
	}

	p := filepath.Clean(filepath.FromSlash(branch))
	if p == "." || p == ".." || strings.HasPrefix(p, ".."+string(os.PathSeparator)) || filepath.IsAbs(p) {
		return "", fmt.Errorf("wt create: unsupported branch for default path: %q", branch)
	}
	return p, nil
}

func normalizeCreateBranch(query string, from string) string {
	branch := strings.TrimSpace(query)
	if strings.TrimSpace(from) != "" {
		return branch
	}

	if after, ok := strings.CutPrefix(branch, "origin/"); ok && strings.TrimSpace(after) != "" {
		return after
	}
	return branch
}

func ensureCreateQuerySafe(commandName string, query string, branch string, wts []worktree.Worktree) error {
	conflict, ok := findCreatePrunableConflict(query, branch, wts)
	if !ok {
		return nil
	}

	return &exitError{
		Code: 1,
		Err:  fmt.Errorf("%s: registered worktree entry is prunable; run 'wt prune --apply' first: %s (%s)", commandName, conflict.Path, displayBranch(conflict)),
	}
}

func findCreatePrunableConflict(query string, branch string, wts []worktree.Worktree) (worktree.Worktree, bool) {
	branch = strings.TrimSpace(branch)
	query = strings.TrimSpace(query)
	branchRef := ""
	if branch != "" {
		branchRef = "refs/heads/" + branch
	}

	for _, wt := range wts {
		if !wt.Prunable {
			continue
		}
		if branchRef != "" && wt.Branch == branchRef {
			return wt, true
		}
	}

	if query == "" {
		return worktree.Worktree{}, false
	}

	for _, wt := range wts {
		if !wt.Prunable {
			continue
		}
		if len(matchWorktrees([]worktree.Worktree{wt}, query)) > 0 {
			return wt, true
		}
	}

	return worktree.Worktree{}, false
}
