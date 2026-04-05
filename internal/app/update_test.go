package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/testutil"
)

func TestResolveUpdateInfoTreatsMissingPinnedCommitAsPinned(t *testing.T) {
	t.Parallel()

	repo := testutil.NewPlainRepo(t, "repo-map")

	info, pinned, err := resolveUpdateInfo(repo.Path, source.Git{
		URL: repo.URL,
		Ref: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})
	if err != nil {
		t.Fatalf("resolveUpdateInfo() error = %v", err)
	}
	if info.Commit != "" {
		t.Fatalf("resolveUpdateInfo().Commit = %q, want empty", info.Commit)
	}
	if !pinned {
		t.Fatal("resolveUpdateInfo() pinned = false, want true")
	}
}

func TestResolveUpdateInfoReturnsErrorForMissingBranch(t *testing.T) {
	t.Parallel()

	repo := testutil.NewPlainRepo(t, "repo-map")

	info, pinned, err := resolveUpdateInfo(repo.Path, source.Git{
		URL: repo.URL,
		Ref: "missing-branch",
	})
	if err == nil {
		t.Fatal("resolveUpdateInfo() error = nil, want missing-branch error")
	}
	if info.Commit != "" {
		t.Fatalf("resolveUpdateInfo().Commit = %q, want empty", info.Commit)
	}
	if pinned {
		t.Fatal("resolveUpdateInfo() pinned = true, want false")
	}
}

func TestResolveUpdateInfoFallsBackWhenMetadataLookupFails(t *testing.T) {
	t.Parallel()

	originalResolveGitCommit := resolveGitCommit
	originalResolveGitInfo := resolveGitInfo
	t.Cleanup(func() {
		resolveGitCommit = originalResolveGitCommit
		resolveGitInfo = originalResolveGitInfo
	})

	resolveGitCommit = func(projectDir string, src source.Git) (string, error) {
		return "abc1234abc1234abc1234abc1234abc1234abc1", nil
	}
	resolveGitInfo = func(projectDir string, src source.Git) (source.ResolveInfo, error) {
		return source.ResolveInfo{}, fmt.Errorf("metadata lookup failed")
	}

	info, pinned, err := resolveUpdateInfo(t.TempDir(), source.Git{
		URL: "https://example.com/repo-map.git",
	})
	if err != nil {
		t.Fatalf("resolveUpdateInfo() error = %v", err)
	}
	if pinned {
		t.Fatal("resolveUpdateInfo() pinned = true, want false")
	}
	if info.Commit != "abc1234abc1234abc1234abc1234abc1234abc1" {
		t.Fatalf("resolveUpdateInfo().Commit = %q, want fallback commit", info.Commit)
	}
	if info.Tracking != "HEAD" {
		t.Fatalf("resolveUpdateInfo().Tracking = %q, want HEAD", info.Tracking)
	}
	if info.LatestAt != "" {
		t.Fatalf("resolveUpdateInfo().LatestAt = %q, want empty", info.LatestAt)
	}
}

func TestCheckUpdatesReturnsTrackingAndLatestDate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	if _, _, err := svc.AddSelected("git:"+repo.URL, nil, "", false, nil); err != nil {
		t.Fatalf("AddSelected() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo.Path, "update-marker.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	testutil.RunGit(t, repo.Path, "add", ".")
	testutil.RunGit(t, repo.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "v2")

	updates, err := svc.CheckUpdates("")
	if err != nil {
		t.Fatalf("CheckUpdates() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("CheckUpdates() = %#v, want one update", updates)
	}

	wantTracking := strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "symbolic-ref", "--short", "HEAD"))
	if updates[0].Tracking != wantTracking {
		t.Fatalf("updates[0].Tracking = %q, want %q", updates[0].Tracking, wantTracking)
	}
	wantDate := strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "show", "-s", "--format=%cs", "HEAD"))
	if updates[0].LatestAt != wantDate {
		t.Fatalf("updates[0].LatestAt = %q, want %q", updates[0].LatestAt, wantDate)
	}
	if updates[0].CurrentCommit != repo.Commit {
		t.Fatalf("updates[0].CurrentCommit = %q, want %q", updates[0].CurrentCommit, repo.Commit)
	}
}

func TestCheckUpdatesWithoutLockfileLeavesCurrentCommitEmpty(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL + "@v1.0.0",
			UpstreamSkill: "repo-map",
		}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	updates, err := svc.CheckUpdates("")
	if err != nil {
		t.Fatalf("CheckUpdates() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("CheckUpdates() = %#v, want one update", updates)
	}
	if updates[0].CurrentCommit != "" {
		t.Fatalf("updates[0].CurrentCommit = %q, want empty without lockfile", updates[0].CurrentCommit)
	}
	if updates[0].Tracking != "v1.0.0" {
		t.Fatalf("updates[0].Tracking = %q, want v1.0.0", updates[0].Tracking)
	}
}
