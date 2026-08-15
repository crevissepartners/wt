package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/buildinfo"
	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/runner"
	"github.com/crevissepartners/wt/internal/worktree"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	rootCmd := newRootCmd()
	rootCmd.SetArgs(args)
	rootCmd.SetContext(ctx)

	err := rootCmd.Execute()
	if err == nil {
		return 0
	}

	var exitErr *exitError
	if errors.As(err, &exitErr) {
		if exitErr.Err != nil {
			fmt.Fprintln(os.Stderr, exitErr.Err)
		}
		return exitErr.Code
	}

	fmt.Fprintln(os.Stderr, err)
	return 1
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "wt",
		Short:         "git worktree helper (WIP)",
		Version:       buildinfo.EffectiveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return ensureDeps(cmd)
		},
	}
	rootCmd.SetVersionTemplate("wt {{.Version}}\n")

	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newPathCmd())
	rootCmd.AddCommand(newRepoRootCmd())
	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newCleanupCmd())
	rootCmd.AddCommand(newPruneCmd())
	rootCmd.AddCommand(newRemoveCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	return rootCmd
}

type depsKey struct{}

type deps struct {
	Runner               runner.Runner
	Cwd                  string
	Executable           func() (string, error)
	IsInteractive        func() bool
	CanUseTUI            func() bool
	PickWorktree         func(cmd *cobra.Command, wts []worktree.Worktree, initialFilter string) (worktree.Worktree, error)
	PreviewPrune         func(cmd *cobra.Command, items []pruneCandidate, apply bool) error
	ConfirmPrune         func(in io.Reader, out io.Writer, count int) (bool, error)
	ReviewCleanup        func(cmd *cobra.Command, candidates []cleanupCandidate, apply bool) ([]cleanupCandidate, error)
	ConfirmCleanup       func(in io.Reader, out io.Writer, count int) (bool, error)
	InstallWithGo        func(ctx context.Context, workDir string, installDir string, packageRef string) (runner.Result, error)
	ResolveLatestVersion func(ctx context.Context, workDir string, modulePath string) (string, error)
	DirtyStatus          func(ctx context.Context, r runner.Runner, worktreePath string) (git.WorktreeStatus, error)
	LookPath             func(file string) (string, error)
	ReadFile             func(path string) ([]byte, error)
	FileExists           func(path string) bool
	Getenv               func(key string) string
}

func ensureDeps(cmd *cobra.Command) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Value(depsKey{}).(*deps); ok {
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	d := &deps{
		Runner:               runner.OSRunner{Env: os.Environ()},
		Cwd:                  cwd,
		Executable:           os.Executable,
		IsInteractive:        stdinIsTTY,
		CanUseTUI:            stdioCanUseTUI,
		PickWorktree:         pickWorktreeWithTUI,
		PreviewPrune:         previewPruneWithTUI,
		ConfirmPrune:         confirmPrune,
		ReviewCleanup:        reviewCleanupWithTUI,
		ConfirmCleanup:       confirmCleanup,
		InstallWithGo:        installWithGo,
		ResolveLatestVersion: resolveLatestVersion,
		DirtyStatus:          git.WorktreeDirtyStatus,
		LookPath:             exec.LookPath,
		ReadFile:             os.ReadFile,
		FileExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
		Getenv: os.Getenv,
	}

	cmd.SetContext(context.WithValue(ctx, depsKey{}, d))
	return nil
}

func getDeps(cmd *cobra.Command) (*deps, error) {
	ctx := cmd.Context()
	if ctx == nil {
		return nil, fmt.Errorf("missing context")
	}
	d, ok := ctx.Value(depsKey{}).(*deps)
	if !ok || d == nil {
		return nil, fmt.Errorf("missing dependencies (internal error)")
	}
	return d, nil
}

type exitError struct {
	Code int
	Err  error
}

func (e *exitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func usageError(err error) error {
	return &exitError{Code: 2, Err: err}
}
