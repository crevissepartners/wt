package git

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/crevissepartners/wt/internal/runner"
)

func TestParseWorktreeStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		out  string
		want WorktreeStatus
	}{
		{
			name: "clean",
			out:  "",
			want: WorktreeStatus{},
		},
		{
			name: "tracked modification only",
			out:  " M a\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindModified}},
		},
		{
			name: "staged only",
			out:  "M  a\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindStaged}},
		},
		{
			name: "staged add only",
			out:  "A  new.txt\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindStaged}},
		},
		{
			name: "untracked only",
			out:  "?? new.txt\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindUntracked}},
		},
		{
			name: "staged and modified in one entry",
			out:  "MM a\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindStaged, DirtyKindModified}},
		},
		{
			name: "combination across entries",
			out:  "M  a\n D b\n?? c\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindStaged, DirtyKindModified, DirtyKindUntracked}},
		},
		{
			name: "merge conflict",
			out:  "UU a\n",
			want: WorktreeStatus{Dirty: true, Kinds: []string{DirtyKindConflicted}},
		},
		{
			name: "ignored entries are not dirty",
			out:  "!! .cache/\n",
			want: WorktreeStatus{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseWorktreeStatus(tt.out)
			if got.Dirty != tt.want.Dirty || !reflect.DeepEqual(got.Kinds, tt.want.Kinds) {
				t.Fatalf("parseWorktreeStatus(%q) = %+v, want %+v", tt.out, got, tt.want)
			}
		})
	}
}

func TestWorktreeDirtyStatus_RealRepo(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(t *testing.T, worktreePath string)
		wantDirty bool
		wantKinds []string
	}{
		{
			name:      "clean",
			mutate:    func(*testing.T, string) {},
			wantDirty: false,
		},
		{
			name: "tracked modification",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, "README.md"), []byte("changed\n"), 0o644)
			},
			wantDirty: true,
			wantKinds: []string{DirtyKindModified},
		},
		{
			name: "staged change",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, "README.md"), []byte("changed\n"), 0o644)
				runGit(t, worktreePath, "add", "README.md")
			},
			wantDirty: true,
			wantKinds: []string{DirtyKindStaged},
		},
		{
			name: "untracked file",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, "scratch.txt"), []byte("wip\n"), 0o644)
			},
			wantDirty: true,
			wantKinds: []string{DirtyKindUntracked},
		},
		{
			name: "staged plus untracked",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, "README.md"), []byte("changed\n"), 0o644)
				runGit(t, worktreePath, "add", "README.md")
				writeFile(t, filepath.Join(worktreePath, "scratch.txt"), []byte("wip\n"), 0o644)
			},
			wantDirty: true,
			wantKinds: []string{DirtyKindStaged, DirtyKindUntracked},
		},
		{
			name: "ignored file stays clean",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, ".cache", "build.out"), []byte("artifact\n"), 0o644)
			},
			wantDirty: false,
		},
		{
			name: "stash entry stays clean",
			mutate: func(t *testing.T, worktreePath string) {
				writeFile(t, filepath.Join(worktreePath, "README.md"), []byte("changed\n"), 0o644)
				runGit(t, worktreePath, "stash", "push", "-m", "wip")
			},
			wantDirty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worktreePath := newDirtyProbeWorktree(t)
			tt.mutate(t, worktreePath)

			got, err := WorktreeDirtyStatus(context.Background(), runner.OSRunner{}, worktreePath)
			if err != nil {
				t.Fatalf("WorktreeDirtyStatus() error = %v", err)
			}
			if got.Dirty != tt.wantDirty {
				t.Fatalf("Dirty = %v, want %v (kinds=%v)", got.Dirty, tt.wantDirty, got.Kinds)
			}
			if !reflect.DeepEqual(got.Kinds, tt.wantKinds) {
				t.Fatalf("Kinds = %v, want %v", got.Kinds, tt.wantKinds)
			}
		})
	}
}

func TestWorktreeDirtyStatus_EmptyPath(t *testing.T) {
	t.Parallel()

	if _, err := WorktreeDirtyStatus(context.Background(), runner.OSRunner{}, "  "); err == nil {
		t.Fatal("WorktreeDirtyStatus() error = nil, want error for empty path")
	}
}

func TestWorktreeDirtyStatus_MissingPathFails(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := WorktreeDirtyStatus(context.Background(), runner.OSRunner{}, missing); err == nil {
		t.Fatal("WorktreeDirtyStatus() error = nil, want error for a missing path")
	}
}

// newDirtyProbeWorktree builds a repo with one linked worktree and returns the
// linked worktree path.
func newDirtyProbeWorktree(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init", "-b", "main")
	runGit(t, repoRoot, "config", "user.name", "wt-test")
	runGit(t, repoRoot, "config", "user.email", "wt-test@example.com")
	runGit(t, repoRoot, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(repoRoot, "README.md"), []byte("main\n"), 0o644)
	writeFile(t, filepath.Join(repoRoot, ".gitignore"), []byte(".cache/\n"), 0o644)
	runGit(t, repoRoot, "add", "README.md", ".gitignore")
	runGit(t, repoRoot, "commit", "-m", "init")
	runGit(t, repoRoot, "branch", "feature/dirty-probe")

	linkedRoot := filepath.Join(t.TempDir(), "feature-dirty-probe")
	runGit(t, repoRoot, "worktree", "add", linkedRoot, "feature/dirty-probe")
	t.Cleanup(func() {
		_ = os.RemoveAll(linkedRoot)
	})
	return linkedRoot
}
