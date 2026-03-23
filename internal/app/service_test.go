package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/target"
	"github.com/Z-Bra0/Ski/internal/testutil"
)

func TestAddSelectedRollsBackAfterLinkFailure(t *testing.T) {
	t.Parallel()

	repo := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	originalManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}
	if err := manifest.WriteFile(manifestPath, originalManifest); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	callCount := 0
	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		materializeAllFn: func(targets []string, name, storePath string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("forced link failure for %s", name)
			}
			return target.MaterializeAll(projectDir, targets, name, storePath)
		},
		removeAllFn: func(targets []string, name string) error {
			return target.RemoveAll(projectDir, targets, name)
		},
	}
	_, _, err := svc.AddSelected("git:"+repo.URL, []string{"alpha-skill", "beta-skill"}, "", false, nil)
	if err == nil {
		t.Fatal("AddSelected() error = nil, want forced link failure")
	}
	if !strings.Contains(err.Error(), "forced link failure for beta-skill") {
		t.Fatalf("AddSelected() error = %v, want forced link failure", err)
	}

	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 0 {
		t.Fatalf("manifest skills = %#v, want empty after rollback", doc.Skills)
	}

	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "alpha-skill")); !os.IsNotExist(err) {
		t.Fatalf("alpha target stat error = %v, want not exist", err)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "beta-skill")); !os.IsNotExist(err) {
		t.Fatalf("beta target stat error = %v, want not exist", err)
	}

	if _, err := os.Stat(lockfile.Path(projectDir)); !os.IsNotExist(err) {
		t.Fatalf("lockfile stat error = %v, want not exist", err)
	}
}

func TestAddSelectedUsesSkillLevelTargetOverride(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	added, warnings, err := svc.AddSelected("git:"+repo.URL, nil, "", false, []string{"codex"})
	if err != nil {
		t.Fatalf("AddSelected() error = %v", err)
	}
	if got, want := added, []string{"repo-map"}; !sameStrings(got, want) {
		t.Fatalf("added = %#v, want %#v", got, want)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repo.URL,
				UpstreamSkill: "repo-map",
				Targets:       []string{"codex"},
			},
		},
	}
	if !reflect.DeepEqual(*doc, wantManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, wantManifest)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 {
		t.Fatalf("lockfile skills = %#v, want one entry", lf.Skills)
	}
	if got, want := lf.Skills[0].Targets, []string{"codex"}; !sameStrings(got, want) {
		t.Fatalf("lockfile targets = %#v, want %#v", got, want)
	}

	assertInstalledSkillDir(t, filepath.Join(projectDir, ".codex", "skills", "repo-map"))
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("claude target stat error = %v, want not exist", err)
	}
}

func TestAddSelectedHonorsLegacySourceSelectorsWithoutExplicitSelection(t *testing.T) {
	t.Parallel()

	repo := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	added, warnings, err := svc.AddSelected("git:"+repo.URL+"##beta-skill", nil, "", false, nil)
	if err != nil {
		t.Fatalf("AddSelected() error = %v", err)
	}
	if got, want := added, []string{"beta-skill"}; !sameStrings(got, want) {
		t.Fatalf("added = %#v, want %#v", got, want)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "beta-skill",
				Source:        "git:" + repo.URL,
				UpstreamSkill: "beta-skill",
			},
		},
	}
	if !reflect.DeepEqual(*doc, wantManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, wantManifest)
	}
}

func TestInitWithTargetsWritesSelectedTargets(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	path, err := svc.InitWithTargets([]string{"claude", "codex"})
	if err != nil {
		t.Fatalf("InitWithTargets() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if got, want := doc.Targets, []string{"claude", "codex"}; !sameStrings(got, want) {
		t.Fatalf("manifest targets = %#v, want %#v", got, want)
	}
}

func TestListRejectsInvalidManifestTargets(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"bad-target"},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	_, err := svc.List()
	if err == nil {
		t.Fatal("List() error = nil, want invalid manifest targets error")
	}
	if !strings.Contains(err.Error(), `manifest targets: unsupported target "bad-target"`) {
		t.Fatalf("List() error = %v, want invalid target error", err)
	}
}

func TestAddSelectedRejectsManifestTargetDirectoryConflicts(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "dir:./.claude/skills"},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	_, _, err := svc.AddSelected("git:https://example.com/skills.git", nil, "", false, nil)
	if err == nil {
		t.Fatal("AddSelected() error = nil, want manifest target conflict error")
	}
	if !strings.Contains(err.Error(), `manifest targets: targets "claude" and "dir:./.claude/skills" resolve to the same directory`) {
		t.Fatalf("AddSelected() error = %v, want target conflict error", err)
	}
}

func TestListRejectsInvalidGlobalManifestTargets(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	globalManifestPath := manifest.GlobalPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(globalManifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(global manifest dir) error = %v", err)
	}
	if err := manifest.WriteFile(globalManifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "dir:~/.claude/skills"},
	}); err != nil {
		t.Fatalf("WriteFile(global manifest) error = %v", err)
	}

	svc := Service{
		HomeDir: homeDir,
		Global:  true,
	}

	_, err := svc.List()
	if err == nil {
		t.Fatal("List() error = nil, want invalid global manifest targets error")
	}
	if !strings.Contains(err.Error(), `manifest targets: targets "claude" and "dir:~/.claude/skills" resolve to the same directory`) {
		t.Fatalf("List() error = %v, want global target conflict error", err)
	}
}

func TestListRejectsManifestTargetsWithWhitespace(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{" claude "},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	_, err := svc.List()
	if err == nil {
		t.Fatal("List() error = nil, want whitespace target error")
	}
	if !strings.Contains(err.Error(), `manifest targets: target " claude " must not include leading or trailing whitespace`) {
		t.Fatalf("List() error = %v, want whitespace target error", err)
	}
}

func TestListRejectsSkillLevelTargetsWithWhitespace(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:    "repo-map",
				Source:  "git:https://example.com/repo-map.git",
				Targets: []string{" codex "},
			},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}

	_, err := svc.List()
	if err == nil {
		t.Fatal("List() error = nil, want whitespace skill target error")
	}
	if !strings.Contains(err.Error(), `skill "repo-map" targets: target " codex " must not include leading or trailing whitespace`) {
		t.Fatalf("List() error = %v, want whitespace skill target error", err)
	}
}

func TestInstallRollsBackAfterLinkFailure(t *testing.T) {
	t.Parallel()

	repo := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "alpha-skill", Source: "git:" + repo.URL + "##alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repo.URL + "##beta-skill"},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	callCount := 0
	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		materializeAllFn: func(targets []string, name, storePath string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("forced install link failure for %s", name)
			}
			return target.MaterializeAll(projectDir, targets, name, storePath)
		},
		removeAllFn: func(targets []string, name string) error {
			return target.RemoveAll(projectDir, targets, name)
		},
	}

	count, err := svc.Install()
	if err == nil {
		t.Fatal("Install() error = nil, want forced link failure")
	}
	if count != 0 {
		t.Fatalf("Install() count = %d, want 0 after rollback", count)
	}
	if !strings.Contains(err.Error(), "forced install link failure for beta-skill") {
		t.Fatalf("Install() error = %v, want forced install link failure", err)
	}

	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "alpha-skill")); !os.IsNotExist(err) {
		t.Fatalf("alpha target stat error = %v, want not exist", err)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "beta-skill")); !os.IsNotExist(err) {
		t.Fatalf("beta target stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(lockfile.Path(projectDir)); !os.IsNotExist(err) {
		t.Fatalf("lockfile stat error = %v, want not exist", err)
	}
}

func TestUpdateRollsBackAfterLinkFailure(t *testing.T) {
	t.Parallel()

	repo := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "alpha-skill", Source: "git:" + repo.URL + "##alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repo.URL + "##beta-skill"},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}
	if _, err := svc.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	originalLock, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(originalLock.Skills) != 2 {
		t.Fatalf("original lockfile skills = %#v, want 2", originalLock.Skills)
	}
	originalCommit := originalLock.Skills[0].Commit

	if err := os.WriteFile(filepath.Join(repo.Path, "update-marker.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	testutil.RunGit(t, repo.Path, "add", ".")
	testutil.RunGit(t, repo.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "update")

	callCount := 0
	svc.materializeAllFn = func(targets []string, name, storePath string) error {
		return target.MaterializeAll(projectDir, targets, name, storePath)
	}
	svc.removeAllFn = func(targets []string, name string) error {
		return target.RemoveAll(projectDir, targets, name)
	}
	svc.replaceTargetFn = func(targetName, name, storePath string) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("forced update replace failure for %s", name)
		}
		return target.Replace(projectDir, targetName, name, storePath)
	}

	updates, err := svc.Update("")
	if err == nil {
		t.Fatal("Update() error = nil, want forced link failure")
	}
	if updates != nil {
		t.Fatalf("Update() updates = %#v, want nil on rollback", updates)
	}
	if !strings.Contains(err.Error(), "forced update replace failure for beta-skill") {
		t.Fatalf("Update() error = %v, want forced update replace failure", err)
	}

	restoredLock, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile restored) error = %v", err)
	}
	for _, skill := range restoredLock.Skills {
		if skill.Commit != originalCommit {
			t.Fatalf("restored lock skill = %#v, want original commit %q", skill, originalCommit)
		}
	}

	for _, skillName := range []string{"alpha-skill", "beta-skill"} {
		assertInstalledSkillDir(t, filepath.Join(projectDir, ".claude", "skills", skillName))
	}
}

func TestRemoveRollsBackAfterUnlinkFailure(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const skillName = "repo-map"

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	setupSvc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
	}
	if _, _, err := setupSvc.AddSelected("git:"+repo.URL, nil, "", false, nil); err != nil {
		t.Fatalf("AddSelected() setup error = %v", err)
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		materializeAllFn: func(targets []string, name, storePath string) error {
			return target.MaterializeAll(projectDir, targets, name, storePath)
		},
		removeAllFn: func(targets []string, name string) error {
			if err := target.RemoveAll(projectDir, []string{"claude"}, name); err != nil {
				return err
			}
			return fmt.Errorf("forced remove unlink failure for %s", name)
		},
	}

	err := svc.Remove(skillName, nil)
	if err == nil {
		t.Fatal("Remove() error = nil, want forced unlink failure")
	}
	if !strings.Contains(err.Error(), "forced remove unlink failure for repo-map") {
		t.Fatalf("Remove() error = %v, want forced unlink failure", err)
	}

	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 1 || doc.Skills[0].Name != skillName {
		t.Fatalf("manifest skills = %#v, want original skill after rollback", doc.Skills)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Name != skillName {
		t.Fatalf("lockfile skills = %#v, want original skill after rollback", lf.Skills)
	}

	for _, targetName := range []string{"claude", "codex"} {
		assertInstalledSkillDir(t, filepath.Join(projectDir, "."+targetName, "skills", skillName))
	}
}

func TestRemoveWithoutLockfileSucceeds(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const skillName = "repo-map"

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", skillName)
	assertInstalledSkillDir(t, targetPath)

	if err := os.Remove(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("Remove(lockfile) error = %v", err)
	}

	if err := svc.Remove(skillName, nil); err != nil {
		t.Fatalf("Remove() without lockfile error = %v", err)
	}

	if _, err := os.Lstat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target still exists after Remove without lockfile")
	}
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 0 {
		t.Fatalf("manifest skills = %#v, want empty after remove", doc.Skills)
	}
}

func TestUpdateWithoutLockfileReplaces(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const skillName = "repo-map"

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", skillName)
	assertInstalledSkillDir(t, targetPath)

	// Add a new commit to the repo so update has something to fetch.
	if err := os.WriteFile(filepath.Join(repo.Path, "update-marker.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	testutil.RunGit(t, repo.Path, "add", ".")
	testutil.RunGit(t, repo.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "v2")

	if err := os.Remove(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("Remove(lockfile) error = %v", err)
	}

	updates, err := svc.Update("")
	if err != nil {
		t.Fatalf("Update() without lockfile error = %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("Update() without lockfile returned no updates, want at least one")
	}

	assertInstalledSkillDir(t, targetPath)
	// The update-marker file must be present in the new copy.
	if _, err := os.Stat(filepath.Join(targetPath, "update-marker.txt")); err != nil {
		t.Fatalf("update-marker.txt missing after update without lockfile: %v", err)
	}
}

type multiSkillSpec struct {
	Path string
	Name string
}

func createMultiSkillRepo(t *testing.T, repoName string, specs []multiSkillSpec) testutil.Repo {
	t.Helper()

	repoSpecs := make([]testutil.SkillSpec, 0, len(specs))
	for _, spec := range specs {
		repoSpecs = append(repoSpecs, testutil.SkillSpec{Path: spec.Path, Name: spec.Name})
	}
	return testutil.NewMultiSkillRepo(t, repoName, repoSpecs)
}

func assertInstalledSkillDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		t.Fatalf("Stat(%s/SKILL.md) error = %v", path, err)
	}
}

// TestUpdateForceReplaceRollbackRestoresOriginal verifies that when an update
// force-replaces a target (no lockfile) and a later step fails, the original
// target content is restored from the backup.
func TestUpdateForceReplaceRollbackRestoresOriginal(t *testing.T) {
	t.Parallel()

	repo := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "alpha-skill", Source: "git:" + repo.URL + "##alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repo.URL + "##beta-skill"},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
	if _, err := svc.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// Write a marker into the installed alpha target so we can verify restoration.
	alphaTarget := filepath.Join(projectDir, ".claude", "skills", "alpha-skill")
	markerPath := filepath.Join(alphaTarget, "original-marker.txt")
	if err := os.WriteFile(markerPath, []byte("original\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}

	// Push a new commit so update has something to fetch.
	if err := os.WriteFile(filepath.Join(repo.Path, "update-marker.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	testutil.RunGit(t, repo.Path, "add", ".")
	testutil.RunGit(t, repo.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "v2")

	// Delete the lockfile to trigger force-replace on update.
	if err := os.Remove(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("Remove(lockfile) error = %v", err)
	}

	// Force a failure on the second replace (beta-skill).
	callCount := 0
	svc.materializeAllFn = func(targets []string, name, storePath string) error {
		return target.MaterializeAll(projectDir, targets, name, storePath)
	}
	svc.removeAllFn = func(targets []string, name string) error {
		return target.RemoveAll(projectDir, targets, name)
	}
	svc.replaceTargetFn = func(targetName, name, storePath string) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("forced replace failure for %s", name)
		}
		return target.Replace(projectDir, targetName, name, storePath)
	}

	_, err := svc.Update("")
	if err == nil {
		t.Fatal("Update() error = nil, want forced replace failure")
	}
	if !strings.Contains(err.Error(), "forced replace failure") {
		t.Fatalf("Update() error = %v, want forced replace failure", err)
	}

	// The alpha target must be restored with its original content, including
	// the marker file that was added after initial install.
	assertInstalledSkillDir(t, alphaTarget)
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("original-marker.txt missing after rollback — original content was not restored: %v", err)
	}
}

// TestRemoveForceRemoveRollbackRestoresTarget verifies that when a lockfile-less
// remove deletes a target and a later write fails, the deleted target is
// restored from the backup.
func TestRemoveForceRemoveRollbackRestoresTarget(t *testing.T) {
	// Cannot be parallel: temporarily makes a file read-only.

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const skillName = "repo-map"

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", skillName)
	assertInstalledSkillDir(t, targetPath)

	// Delete the lockfile to trigger force-remove path.
	if err := os.Remove(lockfile.Path(projectDir)); err != nil {
		t.Fatalf("Remove(lockfile) error = %v", err)
	}

	// Make the manifest file read-only so the manifest write fails after
	// the target has already been deleted.
	if err := os.Chmod(manifestPath, 0o444); err != nil {
		t.Fatalf("Chmod(manifest) error = %v", err)
	}
	t.Cleanup(func() { os.Chmod(manifestPath, 0o644) })

	err := svc.Remove(skillName, nil)
	if err == nil {
		t.Fatal("Remove() error = nil, want manifest write failure")
	}

	// Restore manifest permissions so we can inspect the state.
	if err := os.Chmod(manifestPath, 0o644); err != nil {
		t.Fatalf("Chmod restore error = %v", err)
	}

	// The target must be restored — the backup should have been used.
	assertInstalledSkillDir(t, targetPath)
}
