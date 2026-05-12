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
)

func decodeJSONObjects(t *testing.T, data []byte) []map[string]any {
	t.Helper()

	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("stdout is not valid json: %v\nstdout=%q", err, string(data))
	}
	return got
}

func newListCmdWithDeps(t *testing.T, d *deps) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	cmd := newListCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetContext(context.WithValue(context.Background(), depsKey{}, d))
	return cmd, &stdout, &stderr
}

func TestList_Porcelain(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res: runner.Result{
						Stdout:   []byte(repo + "\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res: runner.Result{
						Stdout:   []byte(porcelain),
						ExitCode: 0,
					},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--porcelain"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if stdout.String() != porcelain {
		t.Fatalf("stdout = %q, want porcelain output", stdout.String())
	}
}

func TestList_JSON(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	porcelain := strings.TrimSpace(`
worktree /repo
HEAD 0123456789abcdef0123456789abcdef01234567
branch refs/heads/main
locked reason: manually locked
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res: runner.Result{
						Stdout:   []byte(repo + "\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res: runner.Result{
						Stdout:   []byte(porcelain),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res: runner.Result{
						Stdout:   []byte(repo + "/.git\n"),
						ExitCode: 0,
					},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var got []jsonWorktree
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid json: %v\nstdout=%q", err, stdout.String())
	}
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	if got[0].Path != "/repo" || got[0].Branch != "refs/heads/main" || got[0].Locked != true {
		t.Fatalf("unexpected json: %#v", got[0])
	}
	if !got[0].Current || !got[0].Primary || got[0].RecommendedAction != "none" || got[0].SafeToRemove {
		t.Fatalf("unexpected derived signals: %#v", got[0])
	}
	if got[0].LockReason == "" {
		t.Fatalf("expected lockReason to be present")
	}
}

func TestList_FlagsMutualExclusion(t *testing.T) {
	t.Parallel()

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{t: t},
		Cwd:    "/cwd",
	})
	cmd.SetArgs([]string{"--json", "--porcelain"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want non-nil")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error = %#v, want exitError code 2", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty (cobra prints errors in main)", stderr.String())
	}
}

func TestList_VerifyHostingRejectsPorcelain(t *testing.T) {
	t.Parallel()

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{t: t},
		Cwd:    "/cwd",
	})
	cmd.SetArgs([]string{"--porcelain", "--verify-hosting"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want non-nil")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error = %#v, want exitError code 2", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestList_FiltersRejectPorcelain(t *testing.T) {
	t.Parallel()

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{t: t},
		Cwd:    "/cwd",
	})
	cmd.SetArgs([]string{"--porcelain", "--stale"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want non-nil")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error = %#v, want exitError code 2", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestList_FiltersRejectInvalidRecommended(t *testing.T) {
	t.Parallel()

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{t: t},
		Cwd:    "/cwd",
	})
	cmd.SetArgs([]string{"--recommended", "bad-value"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want non-nil")
	}
	var ee *exitError
	if !errors.As(err, &ee) || ee.Code != 2 {
		t.Fatalf("error = %#v, want exitError code 2", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestList_Verify_Merged(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res: runner.Result{
						Stdout:   []byte(repo + "\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res: runner.Result{
						Stdout:   []byte(porcelain),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res: runner.Result{
						Stdout:   []byte(repo + "/.git\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res: runner.Result{
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "main"},
					res: runner.Result{
						ExitCode: 0,
					},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--verify", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "merged") {
		t.Fatalf("stdout = %q, want merged marker", stdout.String())
	}
	if !strings.Contains(stdout.String(), "safe-remove") || !strings.Contains(stdout.String(), "recommend:remove") {
		t.Fatalf("stdout = %q, want remove recommendation markers", stdout.String())
	}
}

func TestList_Verify_JSONIncludesFields(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res: runner.Result{
						Stdout:   []byte(repo + "\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res: runner.Result{
						Stdout:   []byte(porcelain),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res: runner.Result{
						Stdout:   []byte(repo + "/.git\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res: runner.Result{
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "main"},
					res: runner.Result{
						ExitCode: 0,
					},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	for _, key := range []string{"pathExists", "dotGitExists", "valid", "mergedIntoBase", "baseRef"} {
		if _, ok := got[0][key]; !ok {
			t.Fatalf("expected verify field %q to be present: %#v", key, got[0])
		}
	}
	if got[0]["baseRef"] != "main" {
		t.Fatalf("baseRef = %#v, want main", got[0]["baseRef"])
	}
	if got[0]["mergedIntoBase"] != true {
		t.Fatalf("mergedIntoBase = %#v, want true", got[0]["mergedIntoBase"])
	}
	if got[0]["recommendedAction"] != "remove" || got[0]["safeToRemove"] != true || got[0]["stale"] != false {
		t.Fatalf("unexpected derived fields: %#v", got[0])
	}
}

func TestList_VerifyHosting_GitHubMergedPR(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
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
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-x", "main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
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
					res:     runner.Result{Stdout: []byte(`[{"number":1,"title":"merged feature","url":"https://github.com/crevissepartners/wt/pull/1"}]`), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--verify", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "merged-hosting:github") {
		t.Fatalf("stdout = %q, want merged-hosting:github marker", stdout.String())
	}
}

func TestList_VerifyHosting_JSONIncludesChangeMetadataOnly(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
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
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	if _, ok := got[0]["mergedIntoBase"]; ok {
		t.Fatalf("mergedIntoBase should be absent when only --verify-hosting is used: %#v", got[0])
	}
	if got[0]["mergedViaHosting"] != true {
		t.Fatalf("mergedViaHosting = %#v, want true", got[0]["mergedViaHosting"])
	}
	if got[0]["hostingChangeNumber"] != float64(42) {
		t.Fatalf("hostingChangeNumber = %#v, want 42", got[0]["hostingChangeNumber"])
	}
	if got[0]["hostingChangeTitle"] != "feature x" {
		t.Fatalf("hostingChangeTitle = %#v, want feature x", got[0]["hostingChangeTitle"])
	}
	if got[0]["hostingChangeURL"] != "https://github.com/crevissepartners/wt/pull/42" {
		t.Fatalf("hostingChangeURL = %#v, want PR URL", got[0]["hostingChangeURL"])
	}
	if got[0]["hostingChangeUrl"] != got[0]["hostingChangeURL"] {
		t.Fatalf("hostingChangeUrl = %#v, want legacy alias to match hostingChangeURL", got[0]["hostingChangeUrl"])
	}
	if got[0]["recommendedAction"] != "remove" || got[0]["safeToRemove"] != true || got[0]["stale"] != false {
		t.Fatalf("unexpected derived fields: %#v", got[0])
	}
}

func TestList_VerifyHosting_JSONUnavailable(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
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
					name:    ghBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	if got[0]["hostingProvider"] != "github" {
		t.Fatalf("hostingProvider = %#v, want github", got[0]["hostingProvider"])
	}
	if got[0]["hostingKind"] != "pr" {
		t.Fatalf("hostingKind = %#v, want pr", got[0]["hostingKind"])
	}
	if got[0]["mergedViaHosting"] != nil {
		t.Fatalf("mergedViaHosting = %#v, want nil", got[0]["mergedViaHosting"])
	}
	if got[0]["hostingReason"] != "gh-auth-unavailable" {
		t.Fatalf("hostingReason = %#v, want gh-auth-unavailable", got[0]["hostingReason"])
	}
	if _, ok := got[0]["mergedIntoBase"]; ok {
		t.Fatalf("mergedIntoBase should be absent when only --verify-hosting is used: %#v", got[0])
	}
}

func TestList_VerifyHosting_TextShowsNoteWhenGHUnavailable(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	t.Setenv("WT_GH_BIN", "")
	t.Setenv("PATH", "")

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@github.com:crevissepartners/wt.git\n"), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "note: hosting verify skipped") {
		t.Fatalf("stderr = %q, want note", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatalf("stdout = empty, want list output")
	}
}

func TestList_VerifyHosting_TextReusesPrecomputedInfoForNoteAndRows(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
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
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
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
				name:    ghBin,
				args:    []string{"auth", "status"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: repo,
				name:    ghBin,
				args:    []string{"pr", "list", "--state", "merged", "--head", "feature-x", "--json", "number,title,url", "--limit", "1", "--base", "main"},
				res:     runner.Result{ExitCode: 1},
				err:     errors.New("exit 1"),
			},
		},
	}
	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: r,
		Cwd:    cwd,
	})

	cmd.SetArgs([]string{"--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if r.i != len(r.calls) {
		t.Fatalf("runner consumed %d calls, want %d", r.i, len(r.calls))
	}
	if !strings.Contains(stderr.String(), "note: hosting verify skipped (gh PR query failed)") {
		t.Fatalf("stderr = %q, want gh PR query failure note", stderr.String())
	}
	if !strings.Contains(stdout.String(), wtPath) {
		t.Fatalf("stdout = %q, want worktree row", stdout.String())
	}
}

func TestList_VerifyHosting_SameBranchUsesOneGitHubLookup(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const ghBin = "/mock/bin/gh"
	t.Setenv("WT_GH_BIN", ghBin)

	wtPath1 := filepath.Join(t.TempDir(), "feature-x-1")
	wtPath2 := filepath.Join(t.TempDir(), "feature-x-2")
	for _, path := range []string{wtPath1, wtPath2} {
		if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatalf("failed to create .git dir: %v", err)
		}
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath1+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-x

worktree `+wtPath2+`
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
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
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
		},
	}
	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: r,
		Cwd:    cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
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
		if got[i]["hostingChangeURL"] != "https://github.com/crevissepartners/wt/pull/42" {
			t.Fatalf("json[%d].hostingChangeURL = %#v, want PR URL", i, got[i]["hostingChangeURL"])
		}
		if got[i]["hostingChangeUrl"] != got[i]["hostingChangeURL"] {
			t.Fatalf("json[%d].hostingChangeUrl = %#v, want legacy alias", i, got[i]["hostingChangeUrl"])
		}
	}
}

func TestList_VerifyHosting_JSONGitLabMergedMR(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const glabBin = "/mock/bin/glab"
	t.Setenv("WT_GLAB_BIN", glabBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"api", "projects/:fullpath/merge_requests?state=merged&source_branch=feature-x&per_page=1&order_by=updated_at&sort=desc&target_branch=main"},
					res:     runner.Result{Stdout: []byte(`[{"iid":17,"title":"feature x","web_url":"https://gitlab.com/team/wt/-/merge_requests/17","merged_at":"2026-03-05T00:00:00Z"}]`), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if got[0]["hostingProvider"] != "gitlab" {
		t.Fatalf("hostingProvider = %#v, want gitlab", got[0]["hostingProvider"])
	}
	if got[0]["hostingKind"] != "mr" {
		t.Fatalf("hostingKind = %#v, want mr", got[0]["hostingKind"])
	}
	if got[0]["mergedViaHosting"] != true {
		t.Fatalf("mergedViaHosting = %#v, want true", got[0]["mergedViaHosting"])
	}
	if got[0]["hostingChangeNumber"] != float64(17) {
		t.Fatalf("hostingChangeNumber = %#v, want 17", got[0]["hostingChangeNumber"])
	}
	if got[0]["hostingChangeTitle"] != "feature x" {
		t.Fatalf("hostingChangeTitle = %#v, want feature x", got[0]["hostingChangeTitle"])
	}
	if got[0]["hostingChangeURL"] != "https://gitlab.com/team/wt/-/merge_requests/17" {
		t.Fatalf("hostingChangeURL = %#v, want MR URL", got[0]["hostingChangeURL"])
	}
	if got[0]["hostingChangeUrl"] != got[0]["hostingChangeURL"] {
		t.Fatalf("hostingChangeUrl = %#v, want legacy alias to match hostingChangeURL", got[0]["hostingChangeUrl"])
	}
}

func TestList_VerifyHosting_GitLabAuthRunsOnceForMultipleBranches(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const glabBin = "/mock/bin/glab"
	t.Setenv("WT_GLAB_BIN", glabBin)

	wtPath1 := filepath.Join(t.TempDir(), "feature-a")
	wtPath2 := filepath.Join(t.TempDir(), "feature-b")
	for _, path := range []string{wtPath1, wtPath2} {
		if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatalf("failed to create .git dir: %v", err)
		}
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath1+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/feature-a

worktree `+wtPath2+`
HEAD 2222222222222222222222222222222222222222
branch refs/heads/feature-b
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
				args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
				res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: repo,
				name:    "git",
				args:    []string{"remote", "get-url", "origin"},
				res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    glabBin,
				args:    []string{"auth", "status"},
				res:     runner.Result{ExitCode: 0},
			},
			{
				workDir: repo,
				name:    glabBin,
				args:    []string{"api", "projects/:fullpath/merge_requests?state=merged&source_branch=feature-a&per_page=1&order_by=updated_at&sort=desc&target_branch=main"},
				res:     runner.Result{Stdout: []byte(`[]`), ExitCode: 0},
			},
			{
				workDir: repo,
				name:    glabBin,
				args:    []string{"api", "projects/:fullpath/merge_requests?state=merged&source_branch=feature-b&per_page=1&order_by=updated_at&sort=desc&target_branch=main"},
				res:     runner.Result{Stdout: []byte(`[]`), ExitCode: 0},
			},
		},
	}
	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: r,
		Cwd:    cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
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
		if got[i]["hostingProvider"] != "gitlab" || got[i]["hostingKind"] != "mr" {
			t.Fatalf("json[%d] hosting fields = %#v", i, got[i])
		}
		if got[i]["mergedViaHosting"] != false {
			t.Fatalf("json[%d].mergedViaHosting = %#v, want false", i, got[i]["mergedViaHosting"])
		}
	}
}

func TestList_VerifyHosting_JSONGitLabUnavailable(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	t.Setenv("WT_GLAB_BIN", "")
	t.Setenv("PATH", "")

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if got[0]["hostingProvider"] != "gitlab" {
		t.Fatalf("hostingProvider = %#v, want gitlab", got[0]["hostingProvider"])
	}
	if got[0]["hostingKind"] != "mr" {
		t.Fatalf("hostingKind = %#v, want mr", got[0]["hostingKind"])
	}
	if got[0]["mergedViaHosting"] != nil {
		t.Fatalf("mergedViaHosting = %#v, want nil", got[0]["mergedViaHosting"])
	}
	if got[0]["hostingReason"] != "glab-auth-unavailable" {
		t.Fatalf("hostingReason = %#v, want glab-auth-unavailable", got[0]["hostingReason"])
	}
}

func TestList_VerifyHosting_JSONGitLabUnauthenticated(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const glabBin = "/mock/bin/glab"
	t.Setenv("WT_GLAB_BIN", glabBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if got[0]["hostingReason"] != "glab-auth-unavailable" {
		t.Fatalf("hostingReason = %#v, want glab-auth-unavailable", got[0]["hostingReason"])
	}
}

func TestList_VerifyHosting_JSONGitLabQueryFailureDegrades(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const glabBin = "/mock/bin/glab"
	t.Setenv("WT_GLAB_BIN", glabBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"api", "projects/:fullpath/merge_requests?state=merged&source_branch=feature-x&per_page=1&order_by=updated_at&sort=desc&target_branch=main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if got[0]["mergedViaHosting"] != nil {
		t.Fatalf("mergedViaHosting = %#v, want nil", got[0]["mergedViaHosting"])
	}
	if got[0]["hostingReason"] != "glab-mr-query-failed" {
		t.Fatalf("hostingReason = %#v, want glab-mr-query-failed", got[0]["hostingReason"])
	}
}

func TestList_VerifyHosting_TextShowsNoteWhenGitLabQueryFails(t *testing.T) {
	const cwd = "/cwd"
	const repo = "/repo"
	const glabBin = "/mock/bin/glab"
	t.Setenv("WT_GLAB_BIN", glabBin)

	wtPath := filepath.Join(t.TempDir(), "feature-x")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"remote", "get-url", "origin"},
					res:     runner.Result{Stdout: []byte("git@gitlab.com:team/wt.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"auth", "status"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    glabBin,
					args:    []string{"api", "projects/:fullpath/merge_requests?state=merged&source_branch=feature-x&per_page=1&order_by=updated_at&sort=desc&target_branch=main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--verify-hosting", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(stderr.String(), "note: hosting verify skipped (glab MR query failed)") {
		t.Fatalf("stderr = %q, want glab MR query failure note", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatalf("stdout = empty, want list output")
	}
}

func TestList_Verify_JSONDetachedUsesNullMergedIntoBase(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := filepath.Join(t.TempDir(), "detached")
	if err := os.MkdirAll(filepath.Join(wtPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
detached
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{
			t: t,
			calls: []fakeCall{
				{
					workDir: cwd,
					name:    "git",
					args:    []string{"rev-parse", "--show-toplevel"},
					res: runner.Result{
						Stdout:   []byte(repo + "\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"worktree", "list", "--porcelain"},
					res: runner.Result{
						Stdout:   []byte(porcelain),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res: runner.Result{
						Stdout:   []byte(repo + "/.git\n"),
						ExitCode: 0,
					},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res: runner.Result{
						ExitCode: 0,
					},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--verify", "--base", "main"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	for _, key := range []string{"pathExists", "dotGitExists", "valid", "mergedIntoBase", "baseRef"} {
		if _, ok := got[0][key]; !ok {
			t.Fatalf("expected verify field %q to be present: %#v", key, got[0])
		}
	}
	if got[0]["mergedIntoBase"] != nil {
		t.Fatalf("mergedIntoBase = %#v, want null", got[0]["mergedIntoBase"])
	}
	if got[0]["baseRef"] != "main" {
		t.Fatalf("baseRef = %#v, want main", got[0]["baseRef"])
	}
	if got[0]["detached"] != true {
		t.Fatalf("detached = %#v, want true", got[0]["detached"])
	}
	if got[0]["recommendedAction"] != "none" || got[0]["safeToRemove"] != false {
		t.Fatalf("unexpected derived fields: %#v", got[0])
	}
}

func TestList_JSONPrunableSignals(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	porcelain := strings.TrimSpace(`
worktree /repo/.wt/feature-x
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-x
prunable gitdir file points to non-existent location
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if got[0]["stale"] != true || got[0]["recommendedAction"] != "prune" || got[0]["safeToRemove"] != false {
		t.Fatalf("unexpected derived fields: %#v", got[0])
	}
}

func TestList_FilterStaleJSON(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	livePath := filepath.Join(t.TempDir(), "live")
	if err := os.MkdirAll(filepath.Join(livePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	missingPath := filepath.Join(t.TempDir(), "missing")

	porcelain := strings.TrimSpace(`
worktree `+livePath+`
HEAD 1111111111111111111111111111111111111111
branch refs/heads/live
worktree `+missingPath+`
HEAD 2222222222222222222222222222222222222222
branch refs/heads/missing
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--json", "--stale"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := decodeJSONObjects(t, stdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	if got[0]["path"] != missingPath {
		t.Fatalf("path = %#v, want %q", got[0]["path"], missingPath)
	}
	if got[0]["stale"] != true {
		t.Fatalf("stale = %#v, want true", got[0]["stale"])
	}
}

func TestList_FilterSafeToRemoveConsistency(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	safePath := filepath.Join(t.TempDir(), "safe")
	if err := os.MkdirAll(filepath.Join(safePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	keepPath := filepath.Join(t.TempDir(), "keep")
	if err := os.MkdirAll(filepath.Join(keepPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+safePath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-safe
worktree `+keepPath+`
HEAD fedcbafedcbafedcbafedcbafedcbafedcbafedc
branch refs/heads/feature-keep
`) + "\n"

	textCmd, textStdout, textStderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-safe", "main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-keep", "main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	textCmd.SetArgs([]string{"--verify", "--base", "main", "--safe-to-remove"})
	if err := textCmd.Execute(); err != nil {
		t.Fatalf("text Execute() error = %v", err)
	}
	if textStderr.Len() != 0 {
		t.Fatalf("text stderr = %q, want empty", textStderr.String())
	}
	if !strings.Contains(textStdout.String(), safePath) {
		t.Fatalf("text stdout = %q, want safe path", textStdout.String())
	}
	if strings.Contains(textStdout.String(), keepPath) {
		t.Fatalf("text stdout = %q, keep path must be filtered out", textStdout.String())
	}

	jsonCmd, jsonStdout, jsonStderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-safe", "main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-keep", "main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	jsonCmd.SetArgs([]string{"--json", "--verify", "--base", "main", "--safe-to-remove"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("json Execute() error = %v", err)
	}
	if jsonStderr.Len() != 0 {
		t.Fatalf("json stderr = %q, want empty", jsonStderr.String())
	}
	got := decodeJSONObjects(t, jsonStdout.Bytes())
	if len(got) != 1 {
		t.Fatalf("len(json) = %d, want 1", len(got))
	}
	if got[0]["path"] != safePath {
		t.Fatalf("json path = %#v, want %q", got[0]["path"], safePath)
	}
	if got[0]["safeToRemove"] != true {
		t.Fatalf("json safeToRemove = %#v, want true", got[0]["safeToRemove"])
	}
}

func TestList_FilterRecommendedRemove(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	removePath := filepath.Join(t.TempDir(), "remove")
	if err := os.MkdirAll(filepath.Join(removePath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}
	keepPath := filepath.Join(t.TempDir(), "keep")
	if err := os.MkdirAll(filepath.Join(keepPath, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	porcelain := strings.TrimSpace(`
worktree `+removePath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/feature-remove
worktree `+keepPath+`
HEAD fedcbafedcbafedcbafedcbafedcbafedcbafedc
branch refs/heads/feature-keep
`) + "\n"

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
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
					args:    []string{"rev-parse", "--path-format=absolute", "--git-common-dir"},
					res:     runner.Result{Stdout: []byte(repo + "/.git\n"), ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-remove", "main"},
					res:     runner.Result{ExitCode: 0},
				},
				{
					workDir: repo,
					name:    "git",
					args:    []string{"merge-base", "--is-ancestor", "refs/heads/feature-keep", "main"},
					res:     runner.Result{ExitCode: 1},
					err:     errors.New("exit 1"),
				},
			},
		},
		Cwd: cwd,
	})

	cmd.SetArgs([]string{"--verify", "--base", "main", "--recommended", "remove"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), removePath) {
		t.Fatalf("stdout = %q, want remove path", stdout.String())
	}
	if strings.Contains(stdout.String(), keepPath) {
		t.Fatalf("stdout = %q, keep path must be filtered out", stdout.String())
	}
}
