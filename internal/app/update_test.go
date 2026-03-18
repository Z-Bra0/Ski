package app

import (
	"testing"

	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/testutil"
)

func TestResolveUpdateCommitTreatsMissingPinnedCommitAsPinned(t *testing.T) {
	t.Parallel()

	repo := testutil.NewPlainRepo(t, "repo-map")

	commit, pinned, err := resolveUpdateCommit(repo.Path, source.Git{
		URL: repo.URL,
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

	repo := testutil.NewPlainRepo(t, "repo-map")

	commit, pinned, err := resolveUpdateCommit(repo.Path, source.Git{
		URL: repo.URL,
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
