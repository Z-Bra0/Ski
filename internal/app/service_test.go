package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/target"
)

func TestAddSelectedRollsBackAfterLinkFailure(t *testing.T) {
	t.Parallel()

	repoPath := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
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
	_, err := svc.AddSelected("git:"+repoPath, []string{"alpha-skill", "beta-skill"}, "")
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

func TestInstallRollsBackAfterLinkFailure(t *testing.T) {
	t.Parallel()

	repoPath := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
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
			{Name: "alpha-skill", Source: "git:" + repoPath + "##alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repoPath + "##beta-skill"},
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

	repoPath := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
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
			{Name: "alpha-skill", Source: "git:" + repoPath + "##alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repoPath + "##beta-skill"},
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

	if err := os.WriteFile(filepath.Join(repoPath, "update-marker.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "update")

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

type multiSkillSpec struct {
	Path string
	Name string
}

func createMultiSkillRepo(t *testing.T, repoName string, specs []multiSkillSpec) string {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	for _, spec := range specs {
		writeSkillDir(t, filepath.Join(repoPath, spec.Path), spec.Name)
	}

	runGitTest(t, root, "init", repoPath)
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	return repoPath
}

func writeSkillDir(t *testing.T, dir string, skillName string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	skillDoc := `---
name: ` + skillName + `
description: Test skill for rollback behavior.
---

# ` + skillName + `
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillDoc), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools", "helper.sh"), []byte("echo helper\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(helper.sh) error = %v", err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}
