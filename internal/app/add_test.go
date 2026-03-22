package app

import (
	"errors"
	"strings"
	"testing"

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
