package app

import (
	"os"
	"path/filepath"
	"testing"

	"ski/internal/source"
)

func TestResolveUpdateCommitTreatsMissingPinnedCommitAsPinned(t *testing.T) {
	t.Parallel()

	repoPath := createPlainGitRepo(t, "repo-map")

	commit, pinned, err := resolveUpdateCommit(repoPath, source.Git{
		URL: repoPath,
		Ref: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})
	if err != nil {
		t.Fatalf("resolveUpdateCommit() error = %v", err)
	}
	if commit != "" {
		t.Fatalf("resolveUpdateCommit() commit = %q, want empty", commit)
	}
	if !pinned {
		t.Fatal("resolveUpdateCommit() pinned = false, want true")
	}
}

func TestResolveUpdateCommitReturnsErrorForMissingBranch(t *testing.T) {
	t.Parallel()

	repoPath := createPlainGitRepo(t, "repo-map")

	commit, pinned, err := resolveUpdateCommit(repoPath, source.Git{
		URL: repoPath,
		Ref: "missing-branch",
	})
	if err == nil {
		t.Fatal("resolveUpdateCommit() error = nil, want missing-branch error")
	}
	if commit != "" {
		t.Fatalf("resolveUpdateCommit() commit = %q, want empty", commit)
	}
	if pinned {
		t.Fatal("resolveUpdateCommit() pinned = true, want false")
	}
}

func createPlainGitRepo(t *testing.T, repoName string) string {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}

	runGitTest(t, root, "init", repoPath)
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	return repoPath
}
