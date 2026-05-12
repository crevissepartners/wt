package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/crevissepartners/wt/internal/runner"
)

func TestWorktreeRemoveDeletesSymlinkButPreservesSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges differ on windows")
	}

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init", "-b", "main")
	runGit(t, repoRoot, "config", "user.name", "wt-test")
	runGit(t, repoRoot, "config", "user.email", "wt-test@example.com")
	writeFile(t, filepath.Join(repoRoot, "README.md"), []byte("main\n"), 0o644)
	writeFile(t, filepath.Join(repoRoot, ".env.local"), []byte("primary\n"), 0o600)
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")
	runGit(t, repoRoot, "branch", "feature/symlink")

	linkedRoot := filepath.Join(t.TempDir(), "feature-symlink")
	runGit(t, repoRoot, "worktree", "add", linkedRoot, "feature/symlink")

	target := filepath.Join(linkedRoot, ".env.local")
	rel, err := filepath.Rel(filepath.Dir(target), filepath.Join(repoRoot, ".env.local"))
	if err != nil {
		t.Fatalf("filepath.Rel() error = %v", err)
	}
	if err := os.Symlink(rel, target); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}

	if err := WorktreeRemove(context.Background(), runner.OSRunner{}, repoRoot, linkedRoot, true); err != nil {
		t.Fatalf("WorktreeRemove() error = %v", err)
	}

	if _, err := os.Stat(linkedRoot); !os.IsNotExist(err) {
		t.Fatalf("worktree path still exists, stat err = %v", err)
	}
	sourceData, err := os.ReadFile(filepath.Join(repoRoot, ".env.local"))
	if err != nil {
		t.Fatalf("os.ReadFile(source) error = %v", err)
	}
	if strings.TrimSpace(string(sourceData)) != "primary" {
		t.Fatalf("source data = %q, want primary", sourceData)
	}
}
