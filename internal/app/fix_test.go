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
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL,
			UpstreamSkill: "repo-map",
		}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 1 || findings[0].Kind != FindingKindMissingLockEntry {
		t.Fatalf("findings = %#v, want missing lock entry", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 1 || !results[0].Fixed {
		t.Fatalf("results = %#v, want fixed missing lock entry", results)
	}

	if _, err := lockfile.ReadFile(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	assertInstalledSkillDir(t, filepath.Join(projectDir, ".claude", "skills", "repo-map"))

	remaining, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() after Fix error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining findings = %#v, want none", remaining)
	}
}

func TestFixRepairsMissingTargetInstall(t *testing.T) {
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}

	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 1 || findings[0].Kind != FindingKindMissingTargetInstall {
		t.Fatalf("findings = %#v, want missing target install", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 1 || !results[0].Fixed {
		t.Fatalf("results = %#v, want fixed missing target install", results)
	}
	assertInstalledSkillDir(t, targetPath)
}

func TestFixRepairsStoreIntegrity(t *testing.T) {
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

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	if err := os.WriteFile(filepath.Join(storePath, "SKILL.md"), []byte("---\nname: repo-map\ndescription: tampered\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 2 || findings[0].Kind != FindingKindStoreIntegrity || findings[1].Kind != FindingKindDriftedTarget {
		t.Fatalf("findings = %#v, want store integrity + drifted target", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 2 || !results[0].Fixed || results[1].Err != nil {
		t.Fatalf("results = %#v, want repaired store integrity without follow-on errors", results)
	}

	remaining, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() after Fix error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining findings = %#v, want none", remaining)
	}
}

func TestFixRepairsInvalidStoreSnapshot(t *testing.T) {
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

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	if err := os.WriteFile(filepath.Join(storePath, "SKILL.md"), []byte("---\nname: repo-map\ndescription: [unterminated\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 1 || findings[0].Kind != FindingKindStoreInvalid {
		t.Fatalf("findings = %#v, want invalid store snapshot", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 1 || !results[0].Fixed {
		t.Fatalf("results = %#v, want repaired invalid store snapshot", results)
	}

	remaining, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() after Fix error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining findings = %#v, want none", remaining)
	}
}

func TestFixSkipsLegacySymlink(t *testing.T) {
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}
	manualDir := t.TempDir()
	if err := os.Symlink(manualDir, targetPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 1 || findings[0].Kind != FindingKindLegacySymlink {
		t.Fatalf("findings = %#v, want legacy symlink", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 1 || results[0].Fixed || results[0].Note != "manual intervention required" {
		t.Fatalf("results = %#v, want skipped legacy symlink", results)
	}

	remaining, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() after Fix error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Kind != FindingKindLegacySymlink {
		t.Fatalf("remaining findings = %#v, want legacy symlink", remaining)
	}
}

func TestFixKeepsTargetsMismatchVisibleWhenUnexpectedTargetRemovalFails(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	setupSvc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	if _, _, err := setupSvc.AddSelected("git:"+repo.URL, nil, "", false, nil); err != nil {
		t.Fatalf("AddSelected() error = %v", err)
	}

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repo.URL,
			UpstreamSkill: "repo-map",
		}},
	}); err != nil {
		t.Fatalf("WriteFile(updated manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		removeAllFn: func(targets []string, name string) error {
			if len(targets) == 1 && targets[0] == "codex" {
				return fmt.Errorf("forced remove failure for %s", name)
			}
			return nil
		},
	}

	findings, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %#v, want targets mismatch + unexpected target", findings)
	}

	results, err := svc.Fix(findings)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want two results", results)
	}
	if results[0].Finding.Kind != FindingKindTargetsMismatch || !strings.Contains(results[0].Note, "unchanged") {
		t.Fatalf("targets mismatch result = %#v, want unchanged note", results[0])
	}
	if results[1].Finding.Kind != FindingKindUnexpectedTarget || results[1].Err == nil {
		t.Fatalf("unexpected target result = %#v, want removal error", results[1])
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || len(lf.Skills[0].Targets) != 2 || lf.Skills[0].Targets[1] != "codex" {
		t.Fatalf("lockfile targets = %#v, want stale codex target preserved", lf.Skills)
	}

	remaining, err := svc.Doctor()
	if err != nil {
		t.Fatalf("Doctor() after Fix error = %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining findings = %#v, want targets mismatch + unexpected target still visible", remaining)
	}
}
