package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/testutil"
)

func TestFixRepairsMissingLockEntry(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir, homeDir := newFixPaths(t)
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL,
			UpstreamSkill: "repo-map",
		}},
	})

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	findings := requireDoctorKinds(t, svc, FindingKindMissingLockEntry)
	results := requireFixKinds(t, svc, findings, FindingKindMissingLockEntry)
	if !requireResultByKind(t, results, FindingKindMissingLockEntry).Fixed {
		t.Fatalf("results = %#v, want missing lock entry fixed", results)
	}

	if _, err := lockfile.ReadFile(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	assertInstalledSkillDir(t, filepath.Join(projectDir, ".claude", "skills", "repo-map"))
	requireNoDoctorFindings(t, svc)
}

func TestFixRepairsMissingTargetInstall(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindMissingTargetInstall)
	results := requireFixKinds(t, svc, findings, FindingKindMissingTargetInstall)
	if !requireResultByKind(t, results, FindingKindMissingTargetInstall).Fixed {
		t.Fatalf("results = %#v, want missing target install fixed", results)
	}

	assertInstalledSkillDir(t, targetPath)
}

func TestFixRepairsStoreIntegrity(t *testing.T) {
	t.Parallel()

	svc, repo, _, homeDir := setupInstalledSkillFixture(t, []string{"claude"})
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	if err := os.WriteFile(filepath.Join(storePath, "SKILL.md"), []byte("---\nname: repo-map\ndescription: tampered\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindStoreIntegrity, FindingKindDriftedTarget)
	results := requireFixKinds(t, svc, findings, FindingKindStoreIntegrity, FindingKindDriftedTarget)

	storeResult := requireResultByKind(t, results, FindingKindStoreIntegrity)
	if !storeResult.Fixed {
		t.Fatalf("store integrity result = %#v, want fixed", storeResult)
	}
	driftedResult := requireResultByKind(t, results, FindingKindDriftedTarget)
	if driftedResult.Err != nil {
		t.Fatalf("drifted target result = %#v, want no error", driftedResult)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixRepairsInvalidStoreSnapshot(t *testing.T) {
	t.Parallel()

	svc, repo, _, homeDir := setupInstalledSkillFixture(t, []string{"claude"})
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	if err := os.WriteFile(filepath.Join(storePath, "SKILL.md"), []byte("---\nname: repo-map\ndescription: [unterminated\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindStoreInvalid)
	results := requireFixKinds(t, svc, findings, FindingKindStoreInvalid)
	if !requireResultByKind(t, results, FindingKindStoreInvalid).Fixed {
		t.Fatalf("results = %#v, want invalid store snapshot fixed", results)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixSkipsLegacySymlink(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}
	manualDir := t.TempDir()
	if err := os.Symlink(manualDir, targetPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindLegacySymlink)
	results := requireFixKinds(t, svc, findings, FindingKindLegacySymlink)
	result := requireResultByKind(t, results, FindingKindLegacySymlink)
	if result.Fixed || result.Note != "manual intervention required" {
		t.Fatalf("result = %#v, want manual intervention note", result)
	}

	requireDoctorKinds(t, svc, FindingKindLegacySymlink)
}

func TestFixKeepsTargetsMismatchVisibleWhenUnexpectedTargetRemovalFails(t *testing.T) {
	t.Parallel()

	svc, repo, projectDir, homeDir := setupInstalledSkillFixture(t, []string{"claude", "codex"})
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL,
			UpstreamSkill: "repo-map",
		}},
	})

	svc = Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		removeAllFn: func(targets []string, name string) error {
			if len(targets) == 1 && targets[0] == "codex" {
				return fmt.Errorf("forced remove failure for %s", name)
			}
			return nil
		},
	}

	findings := requireDoctorKinds(t, svc, FindingKindTargetsMismatch, FindingKindUnexpectedTarget)
	results := requireFixKinds(t, svc, findings, FindingKindTargetsMismatch, FindingKindUnexpectedTarget)

	targetsMismatch := requireResultByKind(t, results, FindingKindTargetsMismatch)
	if !strings.Contains(targetsMismatch.Note, "unchanged") {
		t.Fatalf("targets mismatch result = %#v, want unchanged note", targetsMismatch)
	}
	unexpectedTarget := requireResultByKind(t, results, FindingKindUnexpectedTarget)
	if unexpectedTarget.Err == nil {
		t.Fatalf("unexpected target result = %#v, want removal error", unexpectedTarget)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || len(lf.Skills[0].Targets) != 2 || lf.Skills[0].Targets[1] != "codex" {
		t.Fatalf("lockfile targets = %#v, want stale codex target preserved", lf.Skills)
	}

	requireDoctorKinds(t, svc, FindingKindTargetsMismatch, FindingKindUnexpectedTarget)
}

func TestFixRepairsOrphanedLockEntry(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	})

	findings := requireDoctorKinds(t, svc, FindingKindOrphanedLockEntry)
	results := requireFixKinds(t, svc, findings, FindingKindOrphanedLockEntry)
	if !requireResultByKind(t, results, FindingKindOrphanedLockEntry).Fixed {
		t.Fatalf("results = %#v, want orphaned lock entry fixed", results)
	}

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("orphaned target stat error = %v, want not exist", err)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 0 {
		t.Fatalf("lockfile skills = %#v, want empty after orphan removal", lf.Skills)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixKeepsOrphanedLockEntryVisibleWhenTargetRemovalFails(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, homeDir := setupInstalledSkillFixture(t, []string{"claude"})
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	})

	svc = Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		removeAllFn: func(targets []string, name string) error {
			return fmt.Errorf("forced orphan cleanup failure for %s", name)
		},
	}

	findings := requireDoctorKinds(t, svc, FindingKindOrphanedLockEntry)
	results := requireFixKinds(t, svc, findings, FindingKindOrphanedLockEntry)
	result := requireResultByKind(t, results, FindingKindOrphanedLockEntry)
	if result.Fixed {
		t.Fatalf("result = %#v, want orphaned lock entry left unfixed", result)
	}
	if result.Err == nil {
		t.Fatalf("result = %#v, want orphaned cleanup error", result)
	}
	if !strings.Contains(result.Note, "unchanged") {
		t.Fatalf("result = %#v, want unchanged note", result)
	}

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	assertInstalledSkillDir(t, targetPath)

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Name != "repo-map" {
		t.Fatalf("lockfile skills = %#v, want orphaned entry preserved", lf.Skills)
	}

	requireDoctorKinds(t, svc, FindingKindOrphanedLockEntry)
}

func TestFixRepairsUpstreamMismatch(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lf.Skills[0].UpstreamSkill = ""
	if err := lockfile.WriteFile(lockfile.Path(projectDir), *lf); err != nil {
		t.Fatalf("WriteFile(lockfile) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindUpstreamMismatch)
	results := requireFixKinds(t, svc, findings, FindingKindUpstreamMismatch)
	if !requireResultByKind(t, results, FindingKindUpstreamMismatch).Fixed {
		t.Fatalf("results = %#v, want upstream mismatch fixed", results)
	}

	lf, err = lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) after fix error = %v", err)
	}
	if lf.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("upstream_skill = %q, want repo-map", lf.Skills[0].UpstreamSkill)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixRepairsStoreMissing(t *testing.T) {
	t.Parallel()

	svc, repo, _, homeDir := setupInstalledSkillFixture(t, []string{"claude"})
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	if err := os.RemoveAll(storePath); err != nil {
		t.Fatalf("RemoveAll(store) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindStoreMissing)
	results := requireFixKinds(t, svc, findings, FindingKindStoreMissing)
	if !requireResultByKind(t, results, FindingKindStoreMissing).Fixed {
		t.Fatalf("results = %#v, want store missing fixed", results)
	}

	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("Stat(store) after fix error = %v, want store restored", err)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixRepairsDriftedTargetInIsolation(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.WriteFile(filepath.Join(targetPath, "SKILL.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatalf("WriteFile(target SKILL.md) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindDriftedTarget)
	results := requireFixKinds(t, svc, findings, FindingKindDriftedTarget)
	if !requireResultByKind(t, results, FindingKindDriftedTarget).Fixed {
		t.Fatalf("results = %#v, want drifted target fixed", results)
	}

	assertInstalledSkillDir(t, targetPath)
	requireNoDoctorFindings(t, svc)
}

func TestFixRepairsUnexpectedTarget(t *testing.T) {
	t.Parallel()

	svc, repo, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude", "codex"})
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL,
			UpstreamSkill: "repo-map",
		}},
	})

	findings := requireDoctorKinds(t, svc, FindingKindTargetsMismatch, FindingKindUnexpectedTarget)
	results := requireFixKinds(t, svc, findings, FindingKindTargetsMismatch, FindingKindUnexpectedTarget)

	if !requireResultByKind(t, results, FindingKindTargetsMismatch).Fixed {
		t.Fatalf("results = %#v, want targets mismatch fixed", results)
	}
	if !requireResultByKind(t, results, FindingKindUnexpectedTarget).Fixed {
		t.Fatalf("results = %#v, want unexpected target fixed", results)
	}

	codexPath := filepath.Join(projectDir, ".codex", "skills", "repo-map")
	if _, err := os.Stat(codexPath); !os.IsNotExist(err) {
		t.Fatalf("codex target still exists at %s, want removed", codexPath)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || len(lf.Skills[0].Targets) != 1 || lf.Skills[0].Targets[0] != "claude" {
		t.Fatalf("lockfile targets = %v, want [claude]", lf.Skills[0].Targets)
	}

	requireNoDoctorFindings(t, svc)
}

func TestFixSkipsUnexpectedEntryType(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(target as file) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindUnexpectedEntryType)
	results := requireFixKinds(t, svc, findings, FindingKindUnexpectedEntryType)
	result := requireResultByKind(t, results, FindingKindUnexpectedEntryType)
	if result.Fixed || result.Note != "manual intervention required" {
		t.Fatalf("result = %#v, want manual intervention note", result)
	}

	requireDoctorKinds(t, svc, FindingKindUnexpectedEntryType)
}

func TestFixIsIdempotent(t *testing.T) {
	t.Parallel()

	svc, _, projectDir, _ := setupInstalledSkillFixture(t, []string{"claude"})
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}

	findings := requireDoctorKinds(t, svc, FindingKindMissingTargetInstall)
	if _, err := svc.Fix(findings); err != nil {
		t.Fatalf("Fix() error = %v", err)
	}

	requireNoDoctorFindings(t, svc)
}

func newFixPaths(t testing.TB) (string, string) {
	t.Helper()
	return t.TempDir(), t.TempDir()
}

func setupInstalledSkillFixture(t *testing.T, targets []string) (Service, testutil.Repo, string, string) {
	t.Helper()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir, homeDir := newFixPaths(t)
	writeManifestDoc(t, projectDir, manifest.Manifest{
		Version: 1,
		Targets: append([]string(nil), targets...),
		Skills:  []manifest.Skill{},
	})

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	if _, _, err := svc.AddSelected("git:"+repo.URL, nil, "", false, nil); err != nil {
		t.Fatalf("AddSelected() error = %v", err)
	}

	return svc, repo, projectDir, homeDir
}

func writeManifestDoc(t testing.TB, projectDir string, doc manifest.Manifest) {
	t.Helper()
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), doc); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
}

func requireDoctorKinds(t testing.TB, svc Service, wantKinds ...string) []DoctorFinding {
	t.Helper()
	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	assertKindMultiset(t, findingKinds(findings), wantKinds, "findings")
	return findings
}

func requireNoDoctorFindings(t testing.TB, svc Service) {
	t.Helper()
	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("remaining findings = %#v, want none", findings)
	}
}

func requireFixKinds(t testing.TB, svc Service, findings []DoctorFinding, wantKinds ...string) []FixResult {
	t.Helper()
	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	assertKindMultiset(t, resultKinds(results), wantKinds, "results")
	return results
}

func requireResultByKind(t testing.TB, results []FixResult, kind string) FixResult {
	t.Helper()
	for _, result := range results {
		if result.Finding.Kind == kind {
			return result
		}
	}
	t.Fatalf("results = %#v, missing kind %q", results, kind)
	return FixResult{}
}

func findingKinds(findings []DoctorFinding) []string {
	kinds := make([]string, 0, len(findings))
	for _, finding := range findings {
		kinds = append(kinds, finding.Kind)
	}
	return kinds
}

func resultKinds(results []FixResult) []string {
	kinds := make([]string, 0, len(results))
	for _, result := range results {
		kinds = append(kinds, result.Finding.Kind)
	}
	return kinds
}

func assertKindMultiset(t testing.TB, gotKinds, wantKinds []string, label string) {
	t.Helper()
	gotCounts := countKinds(gotKinds)
	wantCounts := countKinds(wantKinds)

	if len(gotKinds) != len(wantKinds) {
		t.Fatalf("%s kinds = %v, want %v", label, gotKinds, wantKinds)
	}
	if len(gotCounts) != len(wantCounts) {
		t.Fatalf("%s kinds = %v, want %v", label, gotKinds, wantKinds)
	}
	for kind, want := range wantCounts {
		if gotCounts[kind] != want {
			t.Fatalf("%s kinds = %v, want %v", label, gotKinds, wantKinds)
		}
	}
}

func countKinds(kinds []string) map[string]int {
	counts := make(map[string]int, len(kinds))
	for _, kind := range kinds {
		counts[kind]++
	}
	return counts
}
