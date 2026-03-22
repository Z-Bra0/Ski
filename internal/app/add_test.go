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
