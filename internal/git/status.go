package git

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/crevissepartners/wt/internal/runner"
)

// Dirty kind tokens reported by WorktreeStatus.
const (
	DirtyKindStaged     = "staged"
	DirtyKindModified   = "modified"
	DirtyKindUntracked  = "untracked"
	DirtyKindConflicted = "conflicted"
)

// WorktreeStatus is the uncommitted-work summary of a single worktree.
//
// Scope matches `git worktree remove`: staged changes, unstaged tracked
// changes, merge conflicts, and untracked files count as dirty. Ignored files
// do not, and stash entries do not (they live in the shared object store and
// survive worktree removal).
type WorktreeStatus struct {
	Dirty bool
	Kinds []string
}

// WorktreeDirtyStatus reports uncommitted work inside worktreePath.
func WorktreeDirtyStatus(ctx context.Context, r runner.Runner, worktreePath string) (WorktreeStatus, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return WorktreeStatus{}, fmt.Errorf("empty worktree path")
	}

	res, err := r.Run(ctx, worktreePath, "git", "--no-optional-locks", "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return WorktreeStatus{}, fmt.Errorf("git status --porcelain: %s", commandError(res, err))
	}
	return parseWorktreeStatus(string(res.Stdout)), nil
}

func parseWorktreeStatus(out string) WorktreeStatus {
	staged := false
	modified := false
	untracked := false
	conflicted := false

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}

		x := line[0]
		y := line[1]
		switch {
		case x == '?' && y == '?':
			untracked = true
		case x == '!' && y == '!':
			// ignored entry; only present with --ignored, never counted as dirty
		case isUnmergedStatus(x, y):
			conflicted = true
		default:
			if x != ' ' {
				staged = true
			}
			if y != ' ' {
				modified = true
			}
		}
	}

	kinds := make([]string, 0, 4)
	if staged {
		kinds = append(kinds, DirtyKindStaged)
	}
	if modified {
		kinds = append(kinds, DirtyKindModified)
	}
	if untracked {
		kinds = append(kinds, DirtyKindUntracked)
	}
	if conflicted {
		kinds = append(kinds, DirtyKindConflicted)
	}

	if len(kinds) == 0 {
		return WorktreeStatus{}
	}
	return WorktreeStatus{Dirty: true, Kinds: kinds}
}

// isUnmergedStatus reports whether the porcelain XY pair is a merge conflict.
func isUnmergedStatus(x byte, y byte) bool {
	switch string([]byte{x, y}) {
	case "DD", "AU", "UD", "UA", "DU", "AA", "UU":
		return true
	}
	return false
}
