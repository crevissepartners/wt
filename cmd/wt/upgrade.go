package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/buildinfo"
)

type upgradeOpts struct {
	Version string
	DryRun  bool
}

func newUpgradeCmd() *cobra.Command {
	var opts upgradeOpts

	cmd := &cobra.Command{
		Use:           "upgrade",
		Short:         "Upgrade wt to a released version",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := getDeps(cmd)
			if err != nil {
				return err
			}

			executable := d.Executable
			if executable == nil {
				executable = os.Executable
			}
			execPath, err := executable()
			if err != nil {
				return fmt.Errorf("wt upgrade: failed to resolve current executable path: %w", err)
			}
			execPath = filepath.Clean(execPath)
			// The Windows Terminal App Execution Alias lives in WindowsApps and
			// is never a valid install target. If the running binary itself sits
			// there (e.g. after a prior botched upgrade overwrote the sentinel),
			// refuse rather than letting `go install` clobber that slot again.
			if isWindowsAppsAlias(execPath) {
				return usageError(fmt.Errorf("wt upgrade: refusing to upgrade binary under WindowsApps: %s; this is the Windows Terminal App Execution Alias slot, not a real install location. Reinstall wt elsewhere (e.g. `go install github.com/crevissepartners/wt/cmd/wt@latest`) and re-run upgrade", execPath))
			}
			installDir := filepath.Dir(execPath)
			lookPath := d.LookPath
			if lookPath == nil {
				lookPath = exec.LookPath
			}
			if wtPath, lookErr := lookPath("wt"); lookErr == nil && strings.TrimSpace(wtPath) != "" {
				wtPath = filepath.Clean(wtPath)
				// Same filter on the PATH-resolved candidate: if `wt` resolves to
				// the WindowsApps alias, treat it as "no installed wt on PATH"
				// and keep installDir at the running binary's own directory.
				if !isWindowsAppsAlias(wtPath) {
					if isWtExecutable(execPath) && execPath != wtPath {
						return usageError(fmt.Errorf("wt upgrade: refusing to upgrade non-installed binary: current=%s, path-wt=%s; rerun with 'wt upgrade'", execPath, wtPath))
					}
					if !isWtExecutable(execPath) {
						installDir = filepath.Dir(wtPath)
					}
				}
			}

			version := strings.TrimSpace(opts.Version)
			if version == "" {
				version = "latest"
			}
			if strings.HasPrefix(version, "@") {
				return usageError(fmt.Errorf("wt upgrade: --version must not include '@': %s", version))
			}
			if version == "latest" {
				resolver := d.ResolveLatestVersion
				if resolver == nil {
					resolver = resolveLatestVersion
				}
				resolvedVersion, resolveErr := resolver(cmd.Context(), d.Cwd, buildinfo.ModulePath)
				if resolveErr != nil {
					return fmt.Errorf("wt upgrade: failed to resolve latest release version: %w", resolveErr)
				}
				version = resolvedVersion
			}

			packageRef := fmt.Sprintf("%s/cmd/wt@%s", buildinfo.ModulePath, version)
			if opts.DryRun {
				fmt.Fprintf(cmd.ErrOrStderr(), "dry-run: GOBIN=%s go install %s\n", installDir, packageRef)
				return nil
			}

			installer := d.InstallWithGo
			if installer == nil {
				installer = installWithGo
			}

			res, err := installer(cmd.Context(), d.Cwd, installDir, packageRef)
			if err != nil {
				msg := strings.TrimSpace(string(res.Stderr))
				if msg == "" {
					msg = err.Error()
				}
				return fmt.Errorf("wt upgrade: go install failed: %s", msg)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "upgraded: %s\n", packageRef)
			fmt.Fprintf(cmd.OutOrStdout(), "install-dir: %s\n", installDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&opts.Version, "version", "latest", "release version to install (e.g. v0.10.2 or latest)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "print install command without executing it")
	return cmd
}

// isWindowsAppsAlias reports whether p points into the Microsoft Windows
// App Execution Alias directory. Those entries are special reparse points
// that cannot be used as a go install target, so we ignore them when
// resolving the upgrade install directory.
func isWindowsAppsAlias(p string) bool {
	lower := strings.ToLower(p)
	return strings.Contains(lower, `\windowsapps\`) || strings.Contains(lower, "/windowsapps/")
}

// isWtExecutable reports whether p looks like an installed wt binary by
// basename. Matches both POSIX (`wt`) and Windows (`wt.exe`) so guards work
// on both. Used to detect whether the current process is a real installed
// wt vs. a side build (e.g. `./wt-tool`).
func isWtExecutable(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	return base == "wt" || base == "wt.exe"
}
