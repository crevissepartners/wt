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

	"github.com/crevissepartners/wt/internal/git"
	"github.com/crevissepartners/wt/internal/runner"
)

// newLiveWorktreePath creates a directory that looks like a live linked
// worktree (path + .git present) so the dirty probe is actually consulted.
// listJSONKeysBefore is the `wt list --json` key set as of v0.10.10, before the
// dirty guard landed. Existing consumers read these keys.
var listJSONKeysBefore = []string{
	"path", "head", "branch",
	"detached", "locked", "prunable", "current", "primary", "stale",
	"recommendedAction", "safeToRemove",
}

// listJSONVerifyKeysBefore is the extra `--verify` key set as of v0.10.10.
var listJSONVerifyKeysBefore = []string{
	"pathExists", "dotGitExists", "valid", "mergedIntoBase", "baseRef",
}

// TestList_JSONKeysStayBackwardCompatible pins that the dirty fields are purely
// additive: every key an existing consumer reads is still present, and the only
// new keys are `dirty` and `dirtyKinds`.
func TestList_JSONKeysStayBackwardCompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		calls    func(cwd string, repo string, porcelain string) []fakeCall
		status   git.WorktreeStatus
		wantKeys []string
		wantNew  []string
	}{
		{
			name:     "plain clean",
			args:     []string{"--json"},
			calls:    listCallsNoVerify,
			status:   git.WorktreeStatus{},
			wantKeys: listJSONKeysBefore,
			wantNew:  []string{"dirty"},
		},
		{
			name: "verify dirty",
			args: []string{"--verify", "--base", "main", "--json"},
			calls: func(cwd string, repo string, porcelain string) []fakeCall {
				return listCallsVerifyMerged(cwd, repo, porcelain, "feature-x")
			},
			status:   git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindModified}},
			wantKeys: append(append([]string{}, listJSONKeysBefore...), listJSONVerifyKeysBefore...),
			wantNew:  []string{"dirty", "dirtyKinds"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const cwd = "/cwd"
			const repo = "/repo"

			wtPath := newLiveWorktreePath(t, "feature-x")
			porcelain := listPorcelainFor(wtPath, "feature-x")

			cmd, stdout, _ := newListCmdWithDeps(t, &deps{
				Runner: &fakeRunner{t: t, calls: tt.calls(cwd, repo, porcelain)},
				Cwd:    cwd,
				DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
					wtPath: tt.status,
				}),
			})

			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			got := firstJSONObject(t, stdout.Bytes())
			for _, key := range tt.wantKeys {
				if _, ok := got[key]; !ok {
					t.Fatalf("json dropped pre-existing key %q: %#v", key, got)
				}
			}

			known := map[string]struct{}{}
			for _, key := range append(append([]string{}, tt.wantKeys...), tt.wantNew...) {
				known[key] = struct{}{}
			}
			for key := range got {
				if _, ok := known[key]; !ok {
					t.Fatalf("json has unexpected key %q: %#v", key, got)
				}
			}
		})
	}
}

func newLiveWorktreePath(t *testing.T, name string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create worktree dir: %v", err)
	}
	return path
}

func listPorcelainFor(wtPath string, branch string) string {
	return strings.TrimSpace(`
worktree `+wtPath+`
HEAD abcdefabcdefabcdefabcdefabcdefabcdefabcd
branch refs/heads/`+branch+`
`) + "\n"
}

// listCallsNoVerify is the runner sequence for `wt list [--json]`.
func listCallsNoVerify(cwd string, repo string, porcelain string) []fakeCall {
	return []fakeCall{
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
	}
}

// listCallsVerifyMerged is the runner sequence for
// `wt list --verify --base main` where the branch is an ancestor of main.
func listCallsVerifyMerged(cwd string, repo string, porcelain string, branch string) []fakeCall {
	return append(listCallsNoVerify(cwd, repo, porcelain),
		fakeCall{
			workDir: repo,
			name:    "git",
			args:    []string{"rev-parse", "--verify", "--quiet", "main^{commit}"},
			res:     runner.Result{ExitCode: 0},
		},
		fakeCall{
			workDir: repo,
			name:    "git",
			args:    []string{"merge-base", "--is-ancestor", "refs/heads/" + branch, "main"},
			res:     runner.Result{ExitCode: 0},
		},
	)
}

// TestJSONWorktree_DirtyIsPresentInBothMarshalPaths covers the hosting-verify
// branch of jsonWorktree.MarshalJSON, which builds its object as a map rather
// than a struct.
func TestJSONWorktree_DirtyIsPresentInBothMarshalPaths(t *testing.T) {
	t.Parallel()

	dirty := true
	base := jsonWorktree{
		Path:              "/repo/.wt/feature-x",
		Branch:            "refs/heads/feature-x",
		Dirty:             &dirty,
		DirtyKinds:        []string{"modified"},
		RecommendedAction: "none",
	}

	tests := []struct {
		name  string
		input jsonWorktree
	}{
		{name: "struct path", input: base},
		{
			name: "verify path",
			input: func() jsonWorktree {
				out := base
				merged := true
				out.Verify = &jsonVerifyFields{MergedIntoBase: &merged, BaseRef: "origin/main"}
				return out
			}(),
		},
		{
			name: "hosting map path",
			input: func() jsonWorktree {
				out := base
				out.Verify = &jsonVerifyFields{HostingProvider: "github", HostingKind: "pr"}
				return out
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			wantJSONField(t, got, "dirty", true)
			kinds, ok := got["dirtyKinds"].([]any)
			if !ok || len(kinds) != 1 || kinds[0] != "modified" {
				t.Fatalf("json dirtyKinds = %#v, want [modified]", got["dirtyKinds"])
			}
		})
	}
}

// parseListLineMarkers returns the comma-joined `[...]` marker segment of the
// single `wt list` output line. Matching on the marker segment instead of the
// whole line keeps the assertion independent of the temp path text.
func parseListLineMarkers(t *testing.T, out string) string {
	t.Helper()

	line := strings.TrimSpace(out)
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("stdout has %d lines, want 1: %q", strings.Count(line, "\n")+1, out)
	}
	open := strings.LastIndex(line, "[")
	if open < 0 || !strings.HasSuffix(line, "]") {
		return ""
	}
	return line[open+1 : len(line)-1]
}

func firstJSONObject(t *testing.T, data []byte) map[string]any {
	t.Helper()

	objects := decodeJSONObjects(t, data)
	if len(objects) != 1 {
		t.Fatalf("got %d json entries, want 1: %s", len(objects), string(data))
	}
	return objects[0]
}

func wantJSONField(t *testing.T, obj map[string]any, key string, want any) {
	t.Helper()

	got, ok := obj[key]
	if !ok {
		t.Fatalf("json is missing %q: %#v", key, obj)
	}
	if got != want {
		t.Fatalf("json[%q] = %#v, want %#v", key, got, want)
	}
}

// TestList_DirtyWorktreeIsNeverSafeToRemove pins the core contract: a merged
// branch whose worktree still holds uncommitted work is not a remove candidate,
// whatever kind of uncommitted work it is.
func TestList_DirtyWorktreeIsNeverSafeToRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     git.WorktreeStatus
		wantDirty  any
		wantKinds  []any
		wantSafe   bool
		wantAction string
	}{
		{
			name:       "clean",
			status:     git.WorktreeStatus{},
			wantDirty:  false,
			wantSafe:   true,
			wantAction: "remove",
		},
		{
			name:       "tracked modification",
			status:     git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindModified}},
			wantDirty:  true,
			wantKinds:  []any{"modified"},
			wantSafe:   false,
			wantAction: "none",
		},
		{
			name:       "staged",
			status:     git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindStaged}},
			wantDirty:  true,
			wantKinds:  []any{"staged"},
			wantSafe:   false,
			wantAction: "none",
		},
		{
			name:       "untracked only",
			status:     git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindUntracked}},
			wantDirty:  true,
			wantKinds:  []any{"untracked"},
			wantSafe:   false,
			wantAction: "none",
		},
		{
			name: "combination",
			status: git.WorktreeStatus{Dirty: true, Kinds: []string{
				git.DirtyKindStaged, git.DirtyKindModified, git.DirtyKindUntracked,
			}},
			wantDirty:  true,
			wantKinds:  []any{"staged", "modified", "untracked"},
			wantSafe:   false,
			wantAction: "none",
		},
		{
			name:       "merge conflict",
			status:     git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindConflicted}},
			wantDirty:  true,
			wantKinds:  []any{"conflicted"},
			wantSafe:   false,
			wantAction: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const cwd = "/cwd"
			const repo = "/repo"

			wtPath := newLiveWorktreePath(t, "feature-x")
			porcelain := listPorcelainFor(wtPath, "feature-x")

			cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
				Runner: &fakeRunner{t: t, calls: listCallsVerifyMerged(cwd, repo, porcelain, "feature-x")},
				Cwd:    cwd,
				DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
					wtPath: tt.status,
				}),
			})

			cmd.SetArgs([]string{"--verify", "--base", "main", "--json"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}

			got := firstJSONObject(t, stdout.Bytes())
			wantJSONField(t, got, "dirty", tt.wantDirty)
			wantJSONField(t, got, "safeToRemove", tt.wantSafe)
			wantJSONField(t, got, "recommendedAction", tt.wantAction)
			wantJSONField(t, got, "mergedIntoBase", true)

			if tt.wantKinds == nil {
				if _, ok := got["dirtyKinds"]; ok {
					t.Fatalf("json has dirtyKinds for a clean worktree: %#v", got)
				}
			} else {
				kinds, ok := got["dirtyKinds"].([]any)
				if !ok {
					t.Fatalf("json dirtyKinds = %#v, want a list", got["dirtyKinds"])
				}
				if len(kinds) != len(tt.wantKinds) {
					t.Fatalf("json dirtyKinds = %#v, want %#v", kinds, tt.wantKinds)
				}
				for i := range kinds {
					if kinds[i] != tt.wantKinds[i] {
						t.Fatalf("json dirtyKinds = %#v, want %#v", kinds, tt.wantKinds)
					}
				}
			}
		})
	}
}

// TestList_DirtyTextMarkers checks the human-readable markers for the same two
// cases: dirty entries gain a `dirty` marker and lose the remove markers, clean
// entries keep exactly the markers they had before this guard existed.
func TestList_DirtyTextMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      git.WorktreeStatus
		wantMarkers string
	}{
		{
			name:        "clean keeps remove recommendation",
			status:      git.WorktreeStatus{},
			wantMarkers: "merged,safe-remove,recommend:remove",
		},
		{
			name:        "dirty drops remove recommendation",
			status:      git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindModified}},
			wantMarkers: "merged,dirty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const cwd = "/cwd"
			const repo = "/repo"

			wtPath := newLiveWorktreePath(t, "feature-x")
			porcelain := listPorcelainFor(wtPath, "feature-x")

			cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
				Runner: &fakeRunner{t: t, calls: listCallsVerifyMerged(cwd, repo, porcelain, "feature-x")},
				Cwd:    cwd,
				DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
					wtPath: tt.status,
				}),
			})

			cmd.SetArgs([]string{"--verify", "--base", "main"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}

			if got := parseListLineMarkers(t, stdout.String()); got != tt.wantMarkers {
				t.Fatalf("markers = %q, want %q (stdout=%q)", got, tt.wantMarkers, stdout.String())
			}
		})
	}
}

// TestList_DirtyAnswerIsIdenticalWithAndWithoutVerify automates the reported
// reproduction: `--verify` used to flip a dirty worktree from `none` to
// `remove`. Both paths must now agree.
func TestList_DirtyAnswerIsIdenticalWithAndWithoutVerify(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	dirtyStatus := git.WorktreeStatus{Dirty: true, Kinds: []string{git.DirtyKindModified}}

	runList := func(t *testing.T, calls []fakeCall, wtPath string, args ...string) map[string]any {
		t.Helper()

		cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
			Runner: &fakeRunner{t: t, calls: calls},
			Cwd:    cwd,
			DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
				wtPath: dirtyStatus,
			}),
		})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
		return firstJSONObject(t, stdout.Bytes())
	}

	plainPath := newLiveWorktreePath(t, "feature-x")
	plain := runList(t,
		listCallsNoVerify(cwd, repo, listPorcelainFor(plainPath, "feature-x")),
		plainPath,
		"--json",
	)

	verifyPath := newLiveWorktreePath(t, "feature-x")
	verified := runList(t,
		listCallsVerifyMerged(cwd, repo, listPorcelainFor(verifyPath, "feature-x"), "feature-x"),
		verifyPath,
		"--verify", "--base", "main", "--json",
	)

	for _, key := range []string{"dirty", "safeToRemove", "recommendedAction"} {
		if plain[key] != verified[key] {
			t.Fatalf("--verify changed %q: plain=%#v verify=%#v", key, plain[key], verified[key])
		}
	}
	wantJSONField(t, plain, "dirty", true)
	wantJSONField(t, plain, "safeToRemove", false)
	wantJSONField(t, plain, "recommendedAction", "none")
	wantJSONField(t, verified, "mergedIntoBase", true)
}

// TestList_DirtyProbeFailureIsNotSafeToRemove keeps an undetermined status from
// silently reading as clean.
func TestList_DirtyProbeFailureIsNotSafeToRemove(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner:      &fakeRunner{t: t, calls: listCallsVerifyMerged(cwd, repo, porcelain, "feature-x")},
		Cwd:         cwd,
		DirtyStatus: failingDirtyStatus,
	})

	cmd.SetArgs([]string{"--verify", "--base", "main", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := firstJSONObject(t, stdout.Bytes())
	if got["dirty"] != nil {
		t.Fatalf("json dirty = %#v, want null for an undetermined probe", got["dirty"])
	}
	wantJSONField(t, got, "safeToRemove", false)
	wantJSONField(t, got, "recommendedAction", "none")
}

// TestList_DirtyFiltersFollowTheGuard makes sure the derived-signal filters
// consume the same judgement.
func TestList_DirtyFiltersFollowTheGuard(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	cmd, stdout, stderr := newListCmdWithDeps(t, &deps{
		Runner: &fakeRunner{t: t, calls: listCallsVerifyMerged(cwd, repo, porcelain, "feature-x")},
		Cwd:    cwd,
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindUntracked}},
		}),
	})

	cmd.SetArgs([]string{"--verify", "--base", "main", "--json", "--safe-to-remove"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if got := decodeJSONObjects(t, stdout.Bytes()); len(got) != 0 {
		t.Fatalf("--safe-to-remove returned %d dirty entries, want 0: %s", len(got), stdout.String())
	}
}

func runRootCmd(t *testing.T, d *deps, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()

	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	root.SetContext(context.WithValue(context.Background(), depsKey{}, d))
	return &stdout, &stderr, root.Execute()
}

// removeCalls is the runner sequence for `wt remove <query>` up to (but not
// including) the actual removal.
func removeCalls(cwd string, repo string, porcelain string) []fakeCall {
	return []fakeCall{
		{
			workDir: cwd,
			name:    "git",
			args:    []string{"rev-parse", "--show-toplevel"},
			res:     runner.Result{Stdout: []byte(repo + "\n"), ExitCode: 0},
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
			args:    []string{"worktree", "list", "--porcelain"},
			res:     runner.Result{Stdout: []byte(porcelain), ExitCode: 0},
		},
	}
}

func TestRemove_DirtyRefusesWithoutForceDirty(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	// The fake runner has no `worktree remove` entry, so any removal attempt
	// fails the test outright.
	stdout, stderr, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: removeCalls(cwd, repo, porcelain)},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindModified, git.DirtyKindUntracked}},
		}),
	}, "remove", "feature-x", "--force")

	if err == nil {
		t.Fatal("Execute() error = nil, want refusal")
	}
	var exitErr *exitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("Execute() error = %#v, want exitError code 2", err)
	}
	if !strings.Contains(err.Error(), "uncommitted changes (modified,untracked)") {
		t.Fatalf("error = %q, want the dirty kinds in the message", err.Error())
	}
	if !strings.Contains(err.Error(), "--force-dirty") {
		t.Fatalf("error = %q, want the --force-dirty hint", err.Error())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	_ = stderr
}

func TestRemove_DirtyDryRunReportsSkip(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: removeCalls(cwd, repo, porcelain)},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindModified}},
		}),
	}, "remove", "feature-x", "--dry-run")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantLine := "skip  " + wtPath + "  (feature-x)"
	if strings.TrimSpace(stdout.String()) != wantLine {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantLine)
	}
	if !strings.Contains(stderr.String(), "uncommitted changes (modified)") {
		t.Fatalf("stderr = %q, want a dirty note", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--force-dirty") {
		t.Fatalf("stderr = %q, want the --force-dirty hint", stderr.String())
	}
}

func TestRemove_DirtyJSONExposesDirtyFields(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	stdout, _, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: removeCalls(cwd, repo, porcelain)},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindUntracked}},
		}),
	}, "remove", "feature-x", "--dry-run", "--json")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid json: %v\nstdout=%q", err, stdout.String())
	}
	wantJSONField(t, got, "action", actionSkip)
	wantJSONField(t, got, "applied", false)
	wantJSONField(t, got, "removed", false)
	wantJSONField(t, got, "dirty", true)
	kinds, ok := got["dirtyKinds"].([]any)
	if !ok || len(kinds) != 1 || kinds[0] != "untracked" {
		t.Fatalf("json dirtyKinds = %#v, want [untracked]", got["dirtyKinds"])
	}
}

func TestRemove_ForceDirtyRemovesDirtyWorktree(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	calls := append(removeCalls(cwd, repo, porcelain), fakeCall{
		workDir: repo,
		name:    "git",
		args:    []string{"worktree", "remove", "--force", wtPath},
		res:     runner.Result{ExitCode: 0},
	})

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: calls},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindModified}},
		}),
	}, "remove", "feature-x", "--force", "--force-dirty")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	wantLine := "removed  " + wtPath + "  (feature-x)"
	if strings.TrimSpace(stdout.String()) != wantLine {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantLine)
	}
}

func TestRemove_UndeterminedDirtyStatusRefuses(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	_, _, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: removeCalls(cwd, repo, porcelain)},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus:   failingDirtyStatus,
	}, "remove", "feature-x", "--force")

	if err == nil {
		t.Fatal("Execute() error = nil, want refusal")
	}
	if !strings.Contains(err.Error(), "cannot determine worktree status") {
		t.Fatalf("error = %q, want an undetermined-status message", err.Error())
	}
}

// TestRemove_CleanForceIsUnchanged is the no-regression control for the common
// path: a clean worktree still removes with plain --force.
func TestRemove_CleanForceIsUnchanged(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	calls := append(removeCalls(cwd, repo, porcelain), fakeCall{
		workDir: repo,
		name:    "git",
		args:    []string{"worktree", "remove", "--force", wtPath},
		res:     runner.Result{ExitCode: 0},
	})

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner:        &fakeRunner{t: t, calls: calls},
		Cwd:           cwd,
		IsInteractive: func() bool { return false },
		DirtyStatus:   cleanDirtyStatus,
	}, "remove", "feature-x", "--force")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	wantLine := "removed  " + wtPath + "  (feature-x)"
	if strings.TrimSpace(stdout.String()) != wantLine {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantLine)
	}
}

// cleanupCalls is the runner sequence for `wt cleanup` over a single linked
// worktree that is merged into origin/main.
func cleanupCalls(cwd string, repo string, porcelain string, branch string) []fakeCall {
	return []fakeCall{
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
			args:    []string{"merge-base", "--is-ancestor", "refs/heads/" + branch, "origin/main"},
			res:     runner.Result{ExitCode: 0},
		},
	}
}

func TestCleanup_DirtyWorktreeIsSkippedInPreview(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner: &fakeRunner{t: t, calls: cleanupCalls(cwd, repo, porcelain, "feature-x")},
		Cwd:    cwd,
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindModified, git.DirtyKindUntracked}},
		}),
	}, "cleanup")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	wantLine := "skip  " + wtPath + "  (feature-x)  [dirty:modified,untracked]"
	if strings.TrimSpace(stdout.String()) != wantLine {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantLine)
	}
}

// TestCleanup_ApplyLeavesDirtyWorktreeAlone is the strongest guarantee here: the
// fake runner has no `worktree remove` entry, so an attempted removal fails the
// test.
func TestCleanup_ApplyLeavesDirtyWorktreeAlone(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner: &fakeRunner{t: t, calls: cleanupCalls(cwd, repo, porcelain, "feature-x")},
		Cwd:    cwd,
		DirtyStatus: dirtyStatusByPath(map[string]git.WorktreeStatus{
			wtPath: {Dirty: true, Kinds: []string{git.DirtyKindModified}},
		}),
	}, "cleanup", "--apply", "--json")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := firstJSONObject(t, stdout.Bytes())
	wantJSONField(t, got, "action", actionSkip)
	wantJSONField(t, got, "recommendedAction", "none")
	wantJSONField(t, got, "safeToRemove", false)
	wantJSONField(t, got, "applied", false)
	wantJSONField(t, got, "removed", false)
	wantJSONField(t, got, "dirty", true)
	wantJSONField(t, got, "reason", "dirty:modified")
	wantJSONField(t, got, "mergedIntoBase", true)
}

// TestCleanup_CleanApplyIsUnchanged is the no-regression control for cleanup.
func TestCleanup_CleanApplyIsUnchanged(t *testing.T) {
	t.Parallel()

	const cwd = "/cwd"
	const repo = "/repo"

	wtPath := newLiveWorktreePath(t, "feature-x")
	porcelain := listPorcelainFor(wtPath, "feature-x")

	calls := append(cleanupCalls(cwd, repo, porcelain, "feature-x"), fakeCall{
		workDir: repo,
		name:    "git",
		args:    []string{"worktree", "remove", "--force", wtPath},
		res:     runner.Result{ExitCode: 0},
	})

	stdout, stderr, err := runRootCmd(t, &deps{
		Runner:      &fakeRunner{t: t, calls: calls},
		Cwd:         cwd,
		DirtyStatus: cleanDirtyStatus,
	}, "cleanup", "--apply", "--json")

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	got := firstJSONObject(t, stdout.Bytes())
	wantJSONField(t, got, "action", actionRemoved)
	wantJSONField(t, got, "applied", true)
	wantJSONField(t, got, "removed", true)
	wantJSONField(t, got, "safeToRemove", true)
	wantJSONField(t, got, "dirty", false)
}
