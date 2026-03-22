package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
	"github.com/Z-Bra0/Ski/internal/testutil"
)

// ---------------------------------------------------------------------------
// resolveSkillSelection — selection guard extracted from AddSelected
// ---------------------------------------------------------------------------

// makeDiscovered writes real skill directories and returns a RepoResult whose
// Skills entries have valid on-disk Paths so ValidateDirWithWarnings succeeds.
func makeDiscovered(t *testing.T, names ...string) store.RepoResult {
	t.Helper()
	skills := make([]store.DiscoveredSkill, 0, len(names))
	for _, name := range names {
		dir := t.TempDir()
		testutil.WriteSkillDir(t, dir, name, "Test skill.")
		skills = append(skills, store.DiscoveredSkill{Name: name, Path: dir})
	}
	return store.RepoResult{Skills: skills}
}

func TestResolveSkillSelectionSingleSkillNoRequest(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "repo-map")

	got, err := resolveSkillSelection(discovered, nil, false, "")
	if err != nil {
		t.Fatalf("resolveSkillSelection() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0] != "repo-map" {
		t.Fatalf("resolveSkillSelection() = %v, want [repo-map]", got)
	}
}

func TestResolveSkillSelectionMultiSkillNoRequestReturnsError(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "alpha-skill", "beta-skill")

	_, err := resolveSkillSelection(discovered, nil, false, "")
	if err == nil {
		t.Fatal("resolveSkillSelection() error = nil, want MultiSkillSelectionError")
	}
	var multiErr MultiSkillSelectionError
	if !errors.As(err, &multiErr) {
		t.Fatalf("resolveSkillSelection() error type = %T, want MultiSkillSelectionError", err)
	}
	if !sameStrings(multiErr.Skills, []string{"alpha-skill", "beta-skill"}) {
		t.Fatalf("MultiSkillSelectionError.Skills = %v, want [alpha-skill beta-skill]", multiErr.Skills)
	}
}

func TestResolveSkillSelectionAddAllWithInvalidSkillsReturnsError(t *testing.T) {
	t.Parallel()

	sentinelErr := errors.New("invalid SKILL.md")
	discovered := makeDiscovered(t, "good-skill")
	discovered.InvalidSkills = []store.InvalidSkill{
		{CandidateName: "bad-skill", Err: sentinelErr},
	}

	_, err := resolveSkillSelection(discovered, nil, true, "")
	if err == nil {
		t.Fatal("resolveSkillSelection() error = nil, want invalid skill error")
	}
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("resolveSkillSelection() error = %v, want sentinel", err)
	}
}

func TestResolveSkillSelectionExplicitRequestWithInvalidNameReturnsError(t *testing.T) {
	t.Parallel()

	sentinelErr := errors.New("bad frontmatter")
	discovered := makeDiscovered(t, "good-skill")
	discovered.InvalidSkills = []store.InvalidSkill{
		{CandidateName: "bad-skill", Err: sentinelErr},
	}

	_, err := resolveSkillSelection(discovered, []string{"bad-skill"}, false, "")
	if err == nil {
		t.Fatal("resolveSkillSelection() error = nil, want invalid skill error")
	}
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("resolveSkillSelection() error = %v, want sentinel", err)
	}
}

func TestResolveSkillSelectionExplicitRequestNotInRepoReturnsError(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "good-skill")

	_, err := resolveSkillSelection(discovered, []string{"missing-skill"}, false, "")
	if err == nil {
		t.Fatal("resolveSkillSelection() error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "missing-skill") {
		t.Fatalf("resolveSkillSelection() error = %v, want mention of missing-skill", err)
	}
}

func TestResolveSkillSelectionNameOverrideWithMultipleSkillsReturnsError(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "alpha-skill", "beta-skill")

	_, err := resolveSkillSelection(discovered, []string{"alpha-skill", "beta-skill"}, false, "my-alias")
	if err == nil {
		t.Fatal("resolveSkillSelection() error = nil, want name override error")
	}
	if !strings.Contains(err.Error(), "name override") {
		t.Fatalf("resolveSkillSelection() error = %v, want name override error", err)
	}
}

func TestResolveSkillSelectionNameOverrideWithSingleSkillSucceeds(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "repo-map")

	got, err := resolveSkillSelection(discovered, []string{"repo-map"}, false, "my-alias")
	if err != nil {
		t.Fatalf("resolveSkillSelection() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0] != "repo-map" {
		t.Fatalf("resolveSkillSelection() = %v, want [repo-map]", got)
	}
}

func TestResolveSkillSelectionAddAllNoInvalidSkillsSucceeds(t *testing.T) {
	t.Parallel()

	discovered := makeDiscovered(t, "alpha-skill", "beta-skill")

	got, err := resolveSkillSelection(discovered, nil, true, "")
	if err != nil {
		t.Fatalf("resolveSkillSelection() error = %v, want nil", err)
	}
	if !sameStrings(got, []string{"alpha-skill", "beta-skill"}) {
		t.Fatalf("resolveSkillSelection() = %v, want [alpha-skill beta-skill]", got)
	}
}

// ---------------------------------------------------------------------------
// commitAddPlans — I/O commit phase extracted from AddSelected
// ---------------------------------------------------------------------------

func TestCommitAddPlansLinksAllPlans(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath, lockPath, originalManifestData, originalLockData := prepareCommitState(t, projectDir)

	storePath := t.TempDir()
	plans := []plannedAdd{
		{Name: "alpha-skill", Targets: []string{"claude"}, StorePath: storePath},
	}

	linked := make([]string, 0)
	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		linkAllFn: func(targets []string, name, sp string) error {
			linked = append(linked, name)
			return nil
		},
		unlinkAllFn: func(targets []string, name string) error { return nil },
	}

	err := svc.commitAddPlans(
		plans,
		manifestPath, originalManifestData,
		lockPath, originalLockData, false,
		manifest.Manifest{Version: 1, Targets: []string{"claude"}},
		lockfile.Lockfile{Version: 1},
	)
	if err != nil {
		t.Fatalf("commitAddPlans() error = %v, want nil", err)
	}
	if len(linked) != 1 || linked[0] != "alpha-skill" {
		t.Fatalf("linked = %v, want [alpha-skill]", linked)
	}
}

func TestCommitAddPlansRollsBackOnLinkFailure(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath, lockPath, originalManifestData, originalLockData := prepareCommitState(t, projectDir)

	storePath := t.TempDir()
	plans := []plannedAdd{
		{Name: "alpha-skill", Targets: []string{"claude"}, StorePath: storePath},
		{Name: "beta-skill", Targets: []string{"claude"}, StorePath: storePath},
	}

	callCount := 0
	unlinked := make([]string, 0)
	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		linkAllFn: func(targets []string, name, sp string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("forced link failure for %s", name)
			}
			return nil
		},
		unlinkAllFn: func(targets []string, name string) error {
			unlinked = append(unlinked, name)
			return nil
		},
	}

	err := svc.commitAddPlans(
		plans,
		manifestPath, originalManifestData,
		lockPath, originalLockData, false,
		manifest.Manifest{Version: 1, Targets: []string{"claude"}},
		lockfile.Lockfile{Version: 1},
	)
	if err == nil {
		t.Fatal("commitAddPlans() error = nil, want link failure")
	}
	if !strings.Contains(err.Error(), "forced link failure for beta-skill") {
		t.Fatalf("commitAddPlans() error = %v, want mention of beta-skill", err)
	}
	if len(unlinked) != 1 || unlinked[0] != "alpha-skill" {
		t.Fatalf("unlinked = %v, want [alpha-skill] rolled back", unlinked)
	}
	// manifest and lockfile must be restored
	doc, readErr := manifest.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatalf("ReadFile(manifest) after rollback error = %v", readErr)
	}
	if len(doc.Skills) != 0 {
		t.Fatalf("manifest skills after rollback = %v, want empty", doc.Skills)
	}
	// lockfile was created by commitAddPlans; rollback must remove it
	// (originalLockData was nil → hadLockfile=false → restore deletes it)
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("lockfile should not exist after rollback, stat error = %v", statErr)
	}
}

func TestCommitAddPlansRestoresLockfileWhenManifestWriteFails(t *testing.T) {
	// Cannot be parallel: temporarily makes a directory read-only.

	// Keep the lockfile in one dir and the manifest in a separate dir so we
	// can block only manifest writes without blocking lockfile restoration.
	lockDir := t.TempDir()
	manifestDir := t.TempDir()
	homeDir := t.TempDir()

	lockPath := filepath.Join(lockDir, lockfile.FileName)
	manifestPath := filepath.Join(manifestDir, manifest.FileName)

	// Write a known-good lockfile so there is distinct content to restore.
	goodLock := lockfile.Lockfile{Version: 1}
	if err := lockfile.WriteFile(lockPath, goodLock); err != nil {
		t.Fatalf("WriteFile(lockfile) error = %v", err)
	}
	originalLockData, _ := os.ReadFile(lockPath)

	// Write the manifest.
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{Version: 1, Targets: []string{"claude"}}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	originalManifestData, _ := os.ReadFile(manifestPath)

	// Make the manifest file itself read-only so os.WriteFile fails on it.
	// The lockfile lives in a separate directory and stays writable.
	if err := os.Chmod(manifestPath, 0o444); err != nil {
		t.Fatalf("Chmod(manifestPath) error = %v", err)
	}
	t.Cleanup(func() { os.Chmod(manifestPath, 0o644) })

	// nextLock has different content from the original; after rollback the
	// on-disk lockfile must match originalLockData, not nextLock.
	nextLock := lockfile.Lockfile{
		Version: 1,
		Skills: []lockfile.Skill{{
			Name:          "alpha-skill",
			Source:        "git:https://example.com/skills",
			UpstreamSkill: "alpha-skill",
			Commit:        "abc1234",
			Integrity:     "sha256:aabbcc",
			Targets:       []string{"claude"},
		}},
	}

	svc := Service{
		ProjectDir:  lockDir,
		HomeDir:     homeDir,
		linkAllFn:   func(targets []string, name, sp string) error { return nil },
		unlinkAllFn: func(targets []string, name string) error { return nil },
	}

	err := svc.commitAddPlans(
		[]plannedAdd{{Name: "alpha-skill", Targets: []string{"claude"}, StorePath: t.TempDir()}},
		manifestPath, originalManifestData,
		lockPath, originalLockData, true,
		manifest.Manifest{Version: 1, Targets: []string{"claude"}},
		nextLock,
	)

	// Restore manifest dir permission before any assertions.
	if err2 := os.Chmod(manifestDir, 0o755); err2 != nil {
		t.Fatalf("Chmod restore error = %v", err2)
	}

	if err == nil {
		t.Fatal("commitAddPlans() error = nil, want manifest write failure")
	}

	// The lockfile must be restored to its original content, not left with
	// the updated nextLock content that was written before the manifest failed.
	restoredData, readErr := os.ReadFile(lockPath)
	if readErr != nil {
		t.Fatalf("ReadFile(lockfile) after manifest failure error = %v; lockfile was not restored", readErr)
	}
	if string(restoredData) != string(originalLockData) {
		t.Fatalf("lockfile after manifest failure =\n%s\nwant original:\n%s", restoredData, originalLockData)
	}
}

// prepareCommitState writes a minimal manifest and lockfile and returns their
// paths alongside the original raw bytes for use as rollback data.
func prepareCommitState(t *testing.T, projectDir string) (manifestPath, lockPath string, origManifest, origLock []byte) {
	t.Helper()

	manifestPath = filepath.Join(projectDir, manifest.FileName)
	orig := manifest.Manifest{Version: 1, Targets: []string{"claude"}, Skills: []manifest.Skill{}}
	if err := manifest.WriteFile(manifestPath, orig); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	origManifest, _ = os.ReadFile(manifestPath)

	lockPath = lockfile.Path(projectDir)
	origLock = nil
	return
}
