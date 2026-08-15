package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/runner"
)

type fakeCall struct {
	workDir string
	name    string
	args    []string
	res     runner.Result
	err     error
}

type fakeRunner struct {
	t     *testing.T
	calls []fakeCall
	i     int
}

func (f *fakeRunner) Run(_ context.Context, workDir string, name string, args ...string) (runner.Result, error) {
	f.t.Helper()

	if f.i >= len(f.calls) {
		f.t.Fatalf("unexpected command: dir=%q name=%q args=%q", workDir, name, args)
	}
	want := f.calls[f.i]
	f.i++

	if workDir != want.workDir || name != want.name || !reflect.DeepEqual(args, want.args) {
		f.t.Fatalf("command mismatch:\n  got:  dir=%q name=%q args=%q\n  want: dir=%q name=%q args=%q",
			workDir, name, args,
			want.workDir, want.name, want.args,
		)
	}

	return want.res, want.err
}

// cleanDirtyStatus is the default dirty probe stub: every worktree is clean.
// Tests that exercise the dirty guard use dirtyStatusByPath or failingDirtyStatus
// instead. Stubbing here keeps the strict fakeRunner call sequences focused on
// the commands each test actually asserts on.
func cleanDirtyStatus(_ context.Context, _ runner.Runner, _ string) (git.WorktreeStatus, error) {
	return git.WorktreeStatus{}, nil
}

// dirtyStatusByPath reports the given status for the listed worktree paths and
// clean for everything else.
func dirtyStatusByPath(byPath map[string]git.WorktreeStatus) func(context.Context, runner.Runner, string) (git.WorktreeStatus, error) {
	return func(_ context.Context, _ runner.Runner, worktreePath string) (git.WorktreeStatus, error) {
		if status, ok := byPath[worktreePath]; ok {
			return status, nil
		}
		return git.WorktreeStatus{}, nil
	}
}

// failingDirtyStatus simulates a probe that cannot determine the worktree state.
func failingDirtyStatus(_ context.Context, _ runner.Runner, _ string) (git.WorktreeStatus, error) {
	return git.WorktreeStatus{}, errors.New("git status --porcelain: boom")
}

func writeExecutableStub(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
