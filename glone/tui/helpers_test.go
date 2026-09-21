package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsRepoCloned(t *testing.T) {
	cloneDir := t.TempDir()

	if err := os.Mkdir(filepath.Join(cloneDir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isRepoCloned(cloneDir, "empty") {
		t.Fatal("plain directory reported as cloned")
	}

	if err := os.MkdirAll(filepath.Join(cloneDir, "repository", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !isRepoCloned(cloneDir, "repository") {
		t.Fatal("repository with .git directory not reported as cloned")
	}

	worktree := filepath.Join(cloneDir, "worktree")
	if err := os.Mkdir(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: /tmp/shared/.git/worktrees/worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isRepoCloned(cloneDir, "worktree") {
		t.Fatal("worktree with .git file not reported as cloned")
	}
}

func TestSplitProgressLines(t *testing.T) {
	advance, token, err := splitProgressLines([]byte("Receiving objects: 42%\rnext"), false)
	if err != nil {
		t.Fatal(err)
	}
	if advance != len("Receiving objects: 42%\r") {
		t.Fatalf("advance = %d", advance)
	}
	if string(token) != "Receiving objects: 42%\r" {
		t.Fatalf("token = %q", token)
	}
}
