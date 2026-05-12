package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/crevissepartners/wt/internal/runner"
	"github.com/crevissepartners/wt/internal/tui/picker"
	"github.com/crevissepartners/wt/internal/worktree"
)

func TestCleanup_PreviewMixed(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	removePath := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}

	currentPath := filepath.Join(t.TempDir(), "current")
	if err := os.MkdirAll(filepath.Join(currentPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create current worktree: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove

worktree `+currentPath+`
HEAD 2222222222222222222222222222222222222222
branch refs/heads/current
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/current", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "would-prune  /repo/.wt/prunable  (prunable)  [gitdir file points to non-existent location]") {
		t.Fatalf("stdout = %q, want prune preview", out)
	}
	if !strings.Contains(out, "would-remove  "+removePath+"  (feature-remove)  [merged:main]") {
		t.Fatalf("stdout = %q, want remove preview", out)
	}
	if !strings.Contains(out, "skip  /repo  (main)  [current]") {
		t.Fatalf("stdout = %q, want current skip", out)
	}
	if !strings.Contains(out, "skip  "+currentPath+"  (current)  [not-recommended]") {
		t.Fatalf("stdout = %q, want non-recommended skip", out)
	}
}

func TestCleanup_ApplyMixed(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	removePath := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}

	porcelainBefore := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	porcelainAfterPrune := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--apply"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelainBefore), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "prune", "--expire", "now"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelainAfterPrune), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "remove", "--force", removePath},
					res:     runner.Result{ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "pruned  /repo/.wt/prunable  (prunable)") {
		t.Fatalf("stdout = %q, want pruned line", out)
	}
	if !strings.Contains(out, "removed  "+removePath+"  (feature-remove)") {
		t.Fatalf("stdout = %q, want removed line", out)
	}
}

func TestCleanup_JSONIncludesMergedReasons(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	removePath := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--json"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@github.com:crevissepartners/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    ghBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    ghBin,
					args:    []string{"pr", "list", "--state", "merged", "--head", "main", "--json", "number,title,url", "--limit", "1", "--base", "main"},
					res:     runner.Result{Stdout: []byte(`[]`), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    ghBin,
					args:    []string{"pr", "list", "--state", "merged", "--head", "feature-remove", "--json", "number,title,url", "--limit", "1", "--base", "main"},
					res:     runner.Result{Stdout: []byte(`[{"number":42,"title":"feature remove","url":"https://github.com/crevissepartners/wt/pull/42"}]`), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	gotObjects := decodeJSONObjects(t, stdout.Bytes())
	if len(gotObjects) != 2 {
		t.Fatalf("len(json) = %d, want 2", len(gotObjects))
	}
	for _, key := range []string{"pathExists", "dotGitExists", "valid", "mergedIntoBase", "baseRef"} {
		if _, ok := gotObjects[1][key]; !ok {
			t.Fatalf("expected verify field %q to be present: %#v", key, gotObjects[1])
		}
	}
	if gotObjects[1]["hostingProvider"] != "github" {
		t.Fatalf("hostingProvider = %#v, want github", gotObjects[1]["hostingProvider"])
	}
	if gotObjects[1]["hostingKind"] != "pr" {
		t.Fatalf("hostingKind = %#v, want pr", gotObjects[1]["hostingKind"])
	}
	if gotObjects[1]["hostingChangeNumber"] != float64(42) {
		t.Fatalf("hostingChangeNumber = %#v, want 42", gotObjects[1]["hostingChangeNumber"])
	}
	if gotObjects[1]["hostingChangeTitle"] != "feature remove" {
		t.Fatalf("hostingChangeTitle = %#v, want feature remove", gotObjects[1]["hostingChangeTitle"])
	}
	if gotObjects[1]["hostingChangeUrl"] != "https://github.com/crevissepartners/wt/pull/42" {
		t.Fatalf("hostingChangeUrl = %#v, want PR URL", gotObjects[1]["hostingChangeUrl"])
	}

	var got []cleanupItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[1].RecommendedAction != "remove" || got[1].Action != "would-remove" {
		t.Fatalf("unexpected cleanup item: %#v", got[1])
	}
	if got[1].MergedIntoBase == nil || *got[1].MergedIntoBase {
		t.Fatalf("mergedIntoBase = %#v, want false", got[1].MergedIntoBase)
	}
	if got[1].MergedViaHosting == nil || !*got[1].MergedViaHosting {
		t.Fatalf("mergedViaHosting = %#v, want true", got[1].MergedViaHosting)
	}
	if got[1].Reason != "merged-hosting:github#42" {
		t.Fatalf("reason = %q, want hosting merge reason", got[1].Reason)
	}
}

func TestCleanup_JSONHostingCacheReusesDuplicateBranchLookup(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	removePath1 := filepath.Join(t.TempDir(), "feature-x-1")
	removePath2 := filepath.Join(t.TempDir(), "feature-x-2")
	for _, path := range []string{removePath1, removePath2} {
		if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatalf("failed to create remove worktree: %v", err)
		}
	}

	porcelain := strings.TrimSpace(`
worktree `+removePath1+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-x

worktree `+removePath2+`
HEAD 2222222222222222222222222222222222222222
branch refs/heads/feature-x
`) + "\n"

	r := &fakeRunner{
		t: t,
		calls: []fakeCall{
			{
				workDir: cwd,
				name:    "git",
				args:    []string{"rev-parse", "--show-toplevel"},
				res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"worktree", "list", "--porcelain"},
				res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
				res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"remote", "get-url", "origin"},
				res:     runner.Result{Stdout: []byte("git@github.com:crevissepartners/wt.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "origin/main"},
				res:     runner.Result{ExitCode: 1},
				err:     assertErr("exit 1"),
			},
			{
				workDir: repo,
				name:    ghBin,
				args:    []string{"auth", "status"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: repo,
				name:    ghBin,
				args:    []string{"pr", "list", "--state", "merged", "--head", "feature-x", "--json", "number,title,url", "--limit", "1", "--base", "main"},
				res:     runner.Result{Stdout: []byte(`[{"number":42,"title":"feature x","url":"https://github.com/crevissepartners/wt/pull/42"}]`), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "origin/main"},
				res:     runner.Result{ExitCode: 1},
				err:     assertErr("exit 1"),
			},
		},
	}

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--json"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: r,
		Cwd:    cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if r.i != len(r.calls) {
		t.Fatalf("runner consumed %d calls, want %d", r.i, len(r.calls))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 2 {
		t.Fatalf("len(json) = %d, want 2", len(got))
	}
	for i := range got {
		if got[i]["mergedViaHosting"] != true {
			t.Fatalf("json[%d].mergedViaHosting = %#v, want true", i, got[i]["mergedViaHosting"])
		}
		if got[i]["hostingChangeNumber"] != float64(42) {
			t.Fatalf("json[%d].hostingChangeNumber = %#v, want 42", i, got[i]["hostingChangeNumber"])
		}
		if got[i]["hostingChangeUrl"] != "https://github.com/crevissepartners/wt/pull/42" {
			t.Fatalf("json[%d].hostingChangeUrl = %#v, want PR URL", i, got[i]["hostingChangeUrl"])
		}
	}
}

func TestCleanup_JSONHostingUnavailableIncludesReason(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+wtPath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-x
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--json"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@github.com:crevissepartners/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    ghBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "note: hosting verify skipped") {
		t.Fatalf("stderr = %q, want hosting verify note", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 2 {
		t.Fatalf("len(json) = %d, want 2", len(got))
	}
	if got[1]["hostingProvider"] != "github" {
		t.Fatalf("hostingProvider = %#v, want github", got[1]["hostingProvider"])
	}
	if got[1]["hostingKind"] != "pr" {
		t.Fatalf("hostingKind = %#v, want pr", got[1]["hostingKind"])
	}
	if got[1]["hostingReason"] != "gh-auth-unavailable" {
		t.Fatalf("hostingReason = %#v, want gh-auth-unavailable", got[1]["hostingReason"])
	}
	if got[1]["mergedViaHosting"] != nil {
		t.Fatalf("mergedViaHosting = %#v, want nil", got[1]["mergedViaHosting"])
	}
	for _, key := range []string{"pathExists", "dotGitExists", "valid", "mergedIntoBase", "baseRef"} {
		if _, ok := got[1][key]; !ok {
			t.Fatalf("expected verify field %q to be present: %#v", key, got[1])
		}
	}
}

func TestCleanup_JSONApplyPreservesActionSemantics(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	removePath := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}

	porcelainBefore := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	porcelainAfterPrune := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--json", "--apply"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelainBefore), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "prune", "--expire", "now"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelainAfterPrune), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "remove", "--force", removePath},
					res:     runner.Result{ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got []cleanupItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].Action != actionSkip || got[0].Applied || got[0].Removed {
		t.Fatalf("main candidate = %#v, want skip/not-applied/not-removed", got[0])
	}
	if got[1].Action != actionPruned || !got[1].Applied || !got[1].Removed {
		t.Fatalf("prunable candidate = %#v, want pruned/applied/removed", got[1])
	}
	if got[2].Action != actionRemoved || !got[2].Applied || !got[2].Removed {
		t.Fatalf("remove candidate = %#v, want removed/applied/removed", got[2])
	}
}

func TestCleanup_SkipExceptions(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	detachedPath := filepath.Join(t.TempDir(), "detached")
	if err := os.MkdirAll(filepath.Join(detachedPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create detached worktree: %v", err)
	}

	lockedPath := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(filepath.Join(lockedPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create locked worktree: %v", err)
	}

	missingGitPath := filepath.Join(t.TempDir(), "missing-git")
	if err := os.MkdirAll(missingGitPath, 0o755); err != nil {
		t.Fatalf("failed to create missing-git worktree: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+detachedPath+`
HEAD 1111111111111111111111111111111111111111
detached

worktree `+lockedPath+`
HEAD 2222222222222222222222222222222222222222
branch refs/heads/locked
locked manual

worktree `+missingGitPath+`
HEAD 3333333333333333333333333333333333333333
branch refs/heads/missing-git
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/locked", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/missing-git", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"skip  /repo  (main)  [current]",
		"skip  " + detachedPath + "  [detached]",
		"skip  " + lockedPath + "  (locked)  [locked]",
		"skip  " + missingGitPath + "  (missing-git)  [missing-git]",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	}
}

func TestCleanup_SkipPrimary(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo/.wt/current"

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/current
HEAD 1111111111111111111111111111111111111111
branch refs/heads/current
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte("/repo/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/current", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd: cwd,
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "skip  /repo  (main)  [primary]") {
		t.Fatalf("stdout = %q, want primary skip", out)
	}
	if !strings.Contains(out, "skip  /repo/.wt/current  (current)  [current]") {
		t.Fatalf("stdout = %q, want current skip", out)
	}
}

func TestCleanupTUI_RequiresTTY(t *testing.T) {
	t.Parallel()

	root := newRootCmd()
	root.SetArgs([]string{"cleanup", "--tui"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		CanUseTUI: func() bool { return false },
	}))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %#v, want exitError code 2", err)
	}
	if err.Error() != "wt cleanup: --tui requires a TTY on stdin and stderr" {
		t.Fatalf("err = %q, want TTY guidance", err.Error())
	}
}

func TestCleanupTUI_RejectsJSON(t *testing.T) {
	t.Parallel()

	root := newRootCmd()
	root.SetArgs([]string{"cleanup", "--tui", "--json"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{}))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("err = %#v, want exitError code 2", err)
	}
	if err.Error() != "wt cleanup: --tui cannot be combined with --json" {
		t.Fatalf("err = %q, want json/tui guidance", err.Error())
	}
}

func TestCleanupTUI_PreviewSelection(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	removePath := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--tui"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
			},
		},
		Cwd:       cwd,
		CanUseTUI: func() bool { return true },
		ReviewCleanup: func(_ *cobra.Command, candidates []cleanupCandidate, apply bool) ([]cleanupCandidate, error) {
			if apply {
				t.Fatal("apply = true, want false")
			}
			if len(candidates) != 2 {
				t.Fatalf("len(candidates) = %d, want 2", len(candidates))
			}
			if candidates[0].Signals.RecommendedAction != "prune" || candidates[1].Signals.RecommendedAction != "remove" {
				t.Fatalf("unexpected candidates: %#v", candidates)
			}
			return candidates[1:], nil
		},
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "would-remove  "+removePath+"  (feature-remove)") {
		t.Fatalf("stdout = %q, want selected remove preview", out)
	}
	if strings.Contains(out, "would-prune") {
		t.Fatalf("stdout = %q, want prune excluded by selection", out)
	}
	if strings.Contains(out, "skip  /repo  (main)") {
		t.Fatalf("stdout = %q, want non-selected entries excluded", out)
	}
}

func TestCleanupTUI_ApplyConfirm(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"
	removePath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}
	before := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-x
`) + "\n"
	after := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+removePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-x
`) + "\n"

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader("yes\n"))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"cleanup", "--tui", "--apply"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(before), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "prune", "--expire", "now"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(after), ExitCode: 0},
				},
			},
		},
		Cwd:       cwd,
		CanUseTUI: func() bool { return true },
		ReviewCleanup: func(_ *cobra.Command, candidates []cleanupCandidate, apply bool) ([]cleanupCandidate, error) {
			if !apply {
				t.Fatal("apply = false, want true")
			}
			if len(candidates) != 2 {
				t.Fatalf("len(candidates) = %d, want 2", len(candidates))
			}
			return candidates[:1], nil
		},
	}))

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "Apply cleanup to 1 selected candidate? [y/N]") {
		t.Fatalf("stderr = %q, want confirmation prompt", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "pruned  /repo/.wt/prunable  (prunable)") {
		t.Fatalf("stdout = %q, want selected prune applied", out)
	}
	if strings.Contains(out, "feature-x") {
		t.Fatalf("stdout = %q, want unselected candidate excluded", out)
	}
}

func TestCleanupTUI_ConfirmCancelAborts(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"
	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location
`) + "\n"

	root := newRootCmd()
	root.SetIn(strings.NewReader("n\n"))
	root.SetArgs([]string{"cleanup", "--tui", "--apply"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd:       cwd,
		CanUseTUI: func() bool { return true },
		ReviewCleanup: func(_ *cobra.Command, candidates []cleanupCandidate, _ bool) ([]cleanupCandidate, error) {
			return candidates, nil
		},
	}))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("err = %#v, want exitError code 1", err)
	}
	if err.Error() != "wt cleanup: aborted" {
		t.Fatalf("err = %q, want aborted", err.Error())
	}
}

func TestCleanupTUI_ReviewCancelReturnsExit130(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"
	porcelain := strings.TrimSpace(`
worktree /repo/.wt/prunable
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/prunable
prunable gitdir file points to non-existent location
`) + "\n"

	root := newRootCmd()
	root.SetArgs([]string{"cleanup", "--tui"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/prunable", "origin/main"},
					res:     runner.Result{ExitCode: 1},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd:       cwd,
		CanUseTUI: func() bool { return true },
		ReviewCleanup: func(_ *cobra.Command, _ []cleanupCandidate, _ bool) ([]cleanupCandidate, error) {
			return nil, picker.ErrCancelled
		},
	}))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 130 {
		t.Fatalf("err = %#v, want exitError code 130", err)
	}
	if err.Error() != "wt cleanup: review cancelled" {
		t.Fatalf("err = %q, want review cancelled", err.Error())
	}
}

func TestBuildCleanupReviewPickerItems(t *testing.T) {
	t.Parallel()

	candidates := []cleanupCandidate{
		{
			Worktree: worktree.Worktree{
				Path:   "/repo/.wt/feature-x",
				Branch: "refs/heads/feature-x",
			},
			Signals: listSignals{RecommendedAction: "remove"},
			Reason:  "merged:main",
		},
	}
	selected := map[string]struct{}{
		"/repo/.wt/feature-x": {},
	}

	items := buildCleanupReviewPickerItems(candidates, selected, true)
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].ID != cleanupReviewDoneID {
		t.Fatalf("items[0].ID = %q, want done item", items[0].ID)
	}
	if !strings.Contains(items[0].Meta, "selected 1/1") {
		t.Fatalf("items[0].Meta = %q, want selection summary", items[0].Meta)
	}
	if items[1].Label != "[x] feature-x" {
		t.Fatalf("items[1].Label = %q, want selected label", items[1].Label)
	}
	if !strings.Contains(items[1].Meta, "action:remove") || !strings.Contains(items[1].Meta, "merged:main") {
		t.Fatalf("items[1].Meta = %q, want action/reason metadata", items[1].Meta)
	}
}

func TestCleanup_ApplyRemovePermissionDeniedIncludesDiagnosticTarget(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"
	target := filepath.Join(t.TempDir(), "feature-remove")
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create remove worktree: %v", err)
	}
	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main

worktree `+target+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-remove
`) + "\n"

	root := newRootCmd()
	root.SetArgs([]string{"cleanup", "--apply"})
	root.SetContext(context.WithValue(context.Background(), depsKey{}, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"},
					res:     runner.Result{Stdout: []byte("refs/remotes/origin/main\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "origin/main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{ExitCode: 2},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/main", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "origin/main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "remove", "--force", target},
					res:     runner.Result{ExitCode: 1, Stderr: []byte("permission denied")},
					err:     assertErr("exit 1"),
				},
			},
		},
		Cwd: cwd,
	}))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %q, want permission denied", err.Error())
	}
	if !strings.Contains(err.Error(), "target="+target) {
		t.Fatalf("err = %q, want target diagnostic", err.Error())
	}
}

type staticErr string

func (e staticErr) Error() string { return string(e) }

func assertErr(msg string) error { return staticErr(msg) }
