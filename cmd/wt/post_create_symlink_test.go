package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/crevissepartners/wt/internal/runner"
)

func TestParsePostCreateConfigSymlink(t *testing.T) {
	t.Parallel()

	cfg, err := parsePostCreateConfig("/repo/.wt/config.toml", []byte(`
[[hooks.postCreate.symlinks]]
source = ".env.local"

[[hooks.postCreate.symlinks]]
source = 'local/certs'
target = ".config/certs"
onMissingSource = "fail"
onExistingTarget = "skip"
`))
	if err != nil {
		t.Fatalf("parsePostCreateConfig() error = %v", err)
	}
	if len(cfg.Symlinks) != 2 {
		t.Fatalf("len(Symlinks) = %d, want 2", len(cfg.Symlinks))
	}
	if cfg.Symlinks[0].Source != ".env.local" || cfg.Symlinks[0].Target != ".env.local" {
		t.Fatalf("first symlink = %#v, want source and default target .env.local", cfg.Symlinks[0])
	}
	if cfg.Symlinks[1].Source != filepath.Join("local", "certs") ||
		cfg.Symlinks[1].Target != filepath.Join(".config", "certs") ||
		cfg.Symlinks[1].OnMissingSource != missingSourceFail ||
		cfg.Symlinks[1].OnExistingTarget != existingTargetSkip {
		t.Fatalf("second symlink = %#v, want normalized explicit values", cfg.Symlinks[1])
	}
}

func TestParsePostCreateConfigRejectsEscapingPath(t *testing.T) {
	t.Parallel()

	_, err := parsePostCreateConfig("/repo/.wt/config.toml", []byte(`
[[hooks.postCreate.symlinks]]
source = "../secret"
target = ".env.local"
`))
	if err == nil {
		t.Fatal("parsePostCreateConfig() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "path must be relative") {
		t.Fatalf("err = %q, want relative path error", err)
	}
}

func TestApplyPostCreateSymlinksCreatesRelativeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on windows")
	}

	primary := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "feature-x")
	source := filepath.Join(primary, ".env.local")
	if err := os.WriteFile(source, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg := postCreateConfig{Symlinks: []postCreateSymlink{{
		Source:           ".env.local",
		Target:           filepath.Join("config", ".env.local"),
		OnMissingSource:  missingSourceSkip,
		OnExistingTarget: existingTargetFail,
	}}}
	if err := applyPostCreateSymlinks(primary, worktree, cfg, false); err != nil {
		t.Fatalf("applyPostCreateSymlinks() error = %v", err)
	}

	target := filepath.Join(worktree, "config", ".env.local")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("os.Lstat(target) error = %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	linkValue, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("os.Readlink() error = %v", err)
	}
	if filepath.IsAbs(linkValue) {
		t.Fatalf("link value = %q, want relative symlink", linkValue)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkValue))
	if resolved != source {
		t.Fatalf("resolved link = %q, want %q", resolved, source)
	}
}

func TestApplyPostCreateSymlinksFailsOnExistingDifferentTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on windows")
	}

	primary := t.TempDir()
	worktree := t.TempDir()
	if err := os.WriteFile(filepath.Join(primary, ".env.local"), []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(source) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".env.local"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(target) error = %v", err)
	}

	cfg := postCreateConfig{Symlinks: []postCreateSymlink{{
		Source:           ".env.local",
		Target:           ".env.local",
		OnMissingSource:  missingSourceSkip,
		OnExistingTarget: existingTargetFail,
	}}}
	err := applyPostCreateSymlinks(primary, worktree, cfg, false)
	if err == nil {
		t.Fatal("applyPostCreateSymlinks() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "target already exists") {
		t.Fatalf("err = %q, want target exists error", err)
	}
}

func TestApplyPostCreateSymlinksSkipsMissingSourceByDefault(t *testing.T) {
	t.Parallel()

	primary := t.TempDir()
	worktree := t.TempDir()
	cfg := postCreateConfig{Symlinks: []postCreateSymlink{{
		Source:           ".env.local",
		Target:           ".env.local",
		OnMissingSource:  missingSourceSkip,
		OnExistingTarget: existingTargetFail,
	}}}
	if err := applyPostCreateSymlinks(primary, worktree, cfg, false); err != nil {
		t.Fatalf("applyPostCreateSymlinks() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(worktree, ".env.local")); !os.IsNotExist(err) {
		t.Fatalf("target stat err = %v, want not exist", err)
	}
}

func TestCreateAppliesPostCreateSymlinkConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on windows")
	}

	const cwd = "/cwd"
	primary := t.TempDir()
	if err := os.MkdirAll(filepath.Join(primary, ".wt"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(.wt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".env.local"), []byte("primary\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(source) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".wt", "config.toml"), []byte(`
[[hooks.postCreate.symlinks]]
source = ".env.local"
target = ".env.local"
`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(config) error = %v", err)
	}

	targetPath := filepath.Join(primary, ".wt", "feature-x")
	porcelain := strings.TrimSpace(`
worktree `+primary+`
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main
`) + "\n"

	r := &fakeRunner{
		t: t,
		calls: []fakeCall{
			{
				workDir: cwd,
				name:    "git",
				args:    []string{"rev-parse", "--show-toplevel"},
				res:     runner.Result{Stdout: []byte(primary + "\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(filepath.Join(primary, ".git") + "\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"worktree", "list", "--porcelain"},
				res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"config", "--local", "--get", "wt.root"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "refs/heads/feature-x^{commit}"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
				res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "refs/remotes/origin/feature-x^{commit}"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"worktree", "add", "-b", "feature-x", targetPath, "origin/main"},
				res:     runner.Result{ExitCode: 0},
			},
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"create", "feature-x"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{Runner: r, Cwd: cwd}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if stdout.String() != targetPath+"\n" {
		t.Fatalf("stdout = %q, want only path", stdout.String())
	}

	target := filepath.Join(targetPath, ".env.local")
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target symlink info = %v, err = %v; want symlink", info, err)
	}
}

func TestPathCreateAppliesPostCreateSymlinkConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on windows")
	}

	const cwd = "/cwd"
	primary := t.TempDir()
	if err := os.MkdirAll(filepath.Join(primary, ".wt"), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(.wt) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".env.local"), []byte("primary\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(source) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, ".wt", "config.toml"), []byte(`
[[hooks.postCreate.symlinks]]
source = ".env.local"
target = ".env.local"
`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(config) error = %v", err)
	}

	targetPath := filepath.Join(primary, ".wt", "feature-x")
	porcelain := strings.TrimSpace(`
worktree `+primary+`
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main
`) + "\n"

	r := &fakeRunner{
		t: t,
		calls: []fakeCall{
			{
				workDir: cwd,
				name:    "git",
				args:    []string{"rev-parse", "--show-toplevel"},
				res:     runner.Result{Stdout: []byte(primary + "\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"worktree", "list", "--porcelain"},
				res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(filepath.Join(primary, ".git") + "\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"config", "--local", "--get", "wt.root"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "refs/heads/feature-x^{commit}"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
				res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "refs/remotes/origin/feature-x^{commit}"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: primary,
				name:    "git",
				args:    []string{"worktree", "add", "-b", "feature-x", targetPath, "origin/main"},
				res:     runner.Result{ExitCode: 0},
			},
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"path", "feature-x", "--create"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{Runner: r, Cwd: cwd}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if stdout.String() != targetPath+"\n" {
		t.Fatalf("stdout = %q, want only path", stdout.String())
	}

	target := filepath.Join(targetPath, ".env.local")
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target symlink info = %v, err = %v; want symlink", info, err)
	}
}
