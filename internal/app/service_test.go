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
		linkAllFn: func(targets []string, name, storePath string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("forced link failure for %s", name)
			}
			return target.LinkAll(projectDir, targets, name, storePath)
		},
		unlinkAllFn: func(targets []string, name string) error {
			return target.UnlinkAll(projectDir, targets, name)
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
		t.Fatalf("alpha link stat error = %v, want not exist", err)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "beta-skill")); !os.IsNotExist(err) {
		t.Fatalf("beta link stat error = %v, want not exist", err)
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

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", repo.Commit)
	codexLink := filepath.Join(projectDir, ".codex", "skills", "repo-map")
	targetPath, err := os.Readlink(codexLink)
	if err != nil {
		t.Fatalf("Readlink(codex) error = %v", err)
	}
	if targetPath != storePath {
		t.Fatalf("codex symlink target = %q, want %q", targetPath, storePath)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("claude link stat error = %v, want not exist", err)
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
		linkAllFn: func(targets []string, name, storePath string) error {
			callCount++
			if callCount == 2 {
				return fmt.Errorf("forced install link failure for %s", name)
			}
			return target.LinkAll(projectDir, targets, name, storePath)
		},
		unlinkAllFn: func(targets []string, name string) error {
			return target.UnlinkAll(projectDir, targets, name)
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
		t.Fatalf("alpha link stat error = %v, want not exist", err)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "beta-skill")); !os.IsNotExist(err) {
		t.Fatalf("beta link stat error = %v, want not exist", err)
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
	svc.linkAllFn = func(targets []string, name, storePath string) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("forced update link failure for %s", name)
		}
		return target.LinkAll(projectDir, targets, name, storePath)
	}
	svc.unlinkAllFn = func(targets []string, name string) error {
		return target.UnlinkAll(projectDir, targets, name)
	}

	updates, err := svc.Update("")
	if err == nil {
		t.Fatal("Update() error = nil, want forced link failure")
	}
	if updates != nil {
		t.Fatalf("Update() updates = %#v, want nil on rollback", updates)
	}
	if !strings.Contains(err.Error(), "forced update link failure for beta-skill") {
		t.Fatalf("Update() error = %v, want forced update link failure", err)
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
		linkPath := filepath.Join(projectDir, ".claude", "skills", skillName)
		targetPath, err := os.Readlink(linkPath)
		if err != nil {
			t.Fatalf("Readlink(%s) error = %v", skillName, err)
		}
		wantStore := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", originalCommit, "skills", skillName)
		if targetPath != wantStore {
			t.Fatalf("%s symlink target = %q, want %q", skillName, targetPath, wantStore)
		}
	}
}

func TestRemoveRollsBackAfterUnlinkFailure(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const (
		skillName = "repo-map"
		source    = "git:https://example.com/repo-map.git"
		commit    = "abc1234abc1234abc1234abc1234abc1234abc123"
		storePath = "/tmp/fake-store-path"
	)

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	originalManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills: []manifest.Skill{
			{Name: skillName, Source: source},
		},
	}
	if err := manifest.WriteFile(manifestPath, originalManifest); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	originalLock := lockfile.Lockfile{
		Version: 1,
		Skills: []lockfile.Skill{
			{
				Name:      skillName,
				Source:    source,
				Commit:    commit,
				Integrity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Targets:   []string{"claude", "codex"},
			},
		},
	}
	if err := lockfile.WriteFile(lockfile.Path(projectDir), originalLock); err != nil {
		t.Fatalf("WriteFile(lockfile) error = %v", err)
	}

	for _, targetName := range []string{"claude", "codex"} {
		if err := target.LinkAll(projectDir, []string{targetName}, skillName, storePath); err != nil {
			t.Fatalf("LinkAll(%s) error = %v", targetName, err)
		}
	}

	svc := Service{
		ProjectDir: projectDir,
		HomeDir:    homeDir,
		linkAllFn: func(targets []string, name, storePath string) error {
			return target.LinkAll(projectDir, targets, name, storePath)
		},
		unlinkAllFn: func(targets []string, name string) error {
			if err := target.UnlinkAll(projectDir, []string{"claude"}, name); err != nil {
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
		linkPath := filepath.Join(projectDir, "."+targetName, "skills", skillName)
		targetPath, err := os.Readlink(linkPath)
		if err != nil {
			t.Fatalf("Readlink(%s) error = %v", targetName, err)
		}
		if targetPath != storePath {
			t.Fatalf("%s symlink target = %q, want %q", targetName, targetPath, storePath)
		}
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
