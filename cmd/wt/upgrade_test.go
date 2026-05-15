package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crevissepartners/wt/internal/runner"
)

func TestUpgrade_DryRun(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/repo/bin/wt", nil
		},
		ResolveLatestVersion: func(_ context.Context, workDir string, modulePath string) (string, error) {
			if workDir != "/repo" {
				t.Fatalf("workDir = %q, want /repo", workDir)
			}
			if modulePath != "github.com/crevissepartners/wt" {
				t.Fatalf("modulePath = %q, want github.com/crevissepartners/wt", modulePath)
			}
			return "v0.10.2", nil
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"upgrade", "--dry-run"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "go install github.com/crevissepartners/wt/cmd/wt@v0.10.2") {
		t.Fatalf("stderr = %q, want resolved version install command", stderr.String())
	}
}

func TestUpgrade_InvalidVersionAtPrefix(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/repo/bin/wt", nil
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade", "--version", "@latest"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 || !strings.Contains(err.Error(), "must not include '@'") || !strings.Contains(err.Error(), "@latest") {
		t.Fatalf("err = %#v, want usage error for invalid version", err)
	}
}

func TestUpgrade_UsesCurrentBinaryDir(t *testing.T) {
	t.Parallel()

	var gotWorkDir, gotInstallDir, gotPackageRef string
	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/repo/bin/wt", nil
		},
		InstallWithGo: func(_ context.Context, workDir string, installDir string, packageRef string) (runner.Result, error) {
			gotWorkDir = workDir
			gotInstallDir = installDir
			gotPackageRef = packageRef
			return runner.Result{ExitCode: 0}, nil
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"upgrade", "--version", "v0.10.2"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if gotWorkDir != "/repo" {
		t.Fatalf("workDir = %q, want /repo", gotWorkDir)
	}
	if gotPackageRef != "github.com/crevissepartners/wt/cmd/wt@v0.10.2" {
		t.Fatalf("packageRef = %q, want github.com/crevissepartners/wt/cmd/wt@v0.10.2", gotPackageRef)
	}
	if gotInstallDir == "" || filepath.IsAbs(gotInstallDir) == false {
		t.Fatalf("installDir = %q, want absolute path", gotInstallDir)
	}
	if !strings.Contains(stdout.String(), "upgraded: github.com/crevissepartners/wt/cmd/wt@v0.10.2") {
		t.Fatalf("stdout = %q, want upgraded message", stdout.String())
	}
}

func TestUpgrade_InstallerError(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/repo/bin/wt", nil
		},
		ResolveLatestVersion: func(_ context.Context, workDir string, modulePath string) (string, error) {
			return "v0.10.2", nil
		},
		InstallWithGo: func(_ context.Context, workDir string, installDir string, packageRef string) (runner.Result, error) {
			return runner.Result{Stderr: []byte("download failed\n"), ExitCode: 1}, fmt.Errorf("exit status 1")
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "go install failed: download failed") {
		t.Fatalf("err = %q, want installer stderr message", err.Error())
	}
}

func TestUpgrade_ResolveLatestVersionError(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/repo/bin/wt", nil
		},
		ResolveLatestVersion: func(_ context.Context, workDir string, modulePath string) (string, error) {
			return "", fmt.Errorf("no released versions found")
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to resolve latest release version") {
		t.Fatalf("err = %q, want resolver failure message", err.Error())
	}
}

func TestUpgrade_RefusesExecPathInWindowsApps(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			// Simulate the post-botched-upgrade state: a real wt.exe binary
			// sitting in the Windows Terminal App Execution Alias slot.
			return `/home/me/AppData/Local/Microsoft/WindowsApps/wt.exe`, nil
		},
		LookPath: func(file string) (string, error) {
			return `/home/me/AppData/Local/Microsoft/WindowsApps/wt.exe`, nil
		},
		InstallWithGo: func(_ context.Context, _ string, _ string, _ string) (runner.Result, error) {
			t.Fatal("InstallWithGo should not be invoked when execPath is under WindowsApps")
			return runner.Result{}, nil
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %#v, want usage error", err)
	}
	if !strings.Contains(err.Error(), "refusing to upgrade binary under WindowsApps") {
		t.Fatalf("err = %q, want WindowsApps refuse message", err.Error())
	}
}

func TestUpgrade_RejectsWindowsExeBinary(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/wt.exe", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/usr/local/bin/wt.exe", nil
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %#v, want usage error", err)
	}
	// The "refuse non-installed binary" guard must trigger even when basename
	// is `wt.exe`, not just `wt`.
	if !strings.Contains(err.Error(), "refusing to upgrade non-installed binary") {
		t.Fatalf("err = %q, want non-installed binary message for wt.exe", err.Error())
	}
}

func TestUpgrade_FiltersWindowsAppsAlias(t *testing.T) {
	t.Parallel()

	var gotInstallDir string
	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/home/me/go/bin/wt-tool", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			// Simulate Windows lookPath resolving the Terminal App Execution Alias
			// (the bug we are guarding against: a WindowsApps entry should NOT be
			// treated as the install target).
			return `/home/me/AppData/Local/Microsoft/WindowsApps/wt.exe`, nil
		},
		InstallWithGo: func(_ context.Context, workDir string, installDir string, packageRef string) (runner.Result, error) {
			gotInstallDir = installDir
			return runner.Result{ExitCode: 0}, nil
		},
		ResolveLatestVersion: func(_ context.Context, _ string, _ string) (string, error) {
			return "v0.10.2", nil
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	wantInstallDir := filepath.Clean("/home/me/go/bin")
	if filepath.Clean(gotInstallDir) != wantInstallDir {
		t.Fatalf("installDir = %q, want %q (must ignore WindowsApps lookPath result)", gotInstallDir, wantInstallDir)
	}
}

func TestUpgrade_RejectsNonInstalledBinary(t *testing.T) {
	t.Parallel()

	d := &deps{
		Cwd: "/repo",
		Executable: func() (string, error) {
			return "/repo/wt", nil
		},
		LookPath: func(file string) (string, error) {
			if file != "wt" {
				t.Fatalf("file = %q, want wt", file)
			}
			return "/usr/local/bin/wt", nil
		},
	}

	root := newRootCmd()
	root.SetArgs([]string{"upgrade"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %#v, want usage error", err)
	}
	if !strings.Contains(err.Error(), "refusing to upgrade non-installed binary") {
		t.Fatalf("err = %q, want non-installed binary message", err.Error())
	}
	if !strings.Contains(err.Error(), "rerun with 'wt upgrade'") {
		t.Fatalf("err = %q, want rerun guidance", err.Error())
	}
}
