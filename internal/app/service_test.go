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

	origLinkAll := linkAll
	origUnlinkAll := unlinkAll
	t.Cleanup(func() {
		linkAll = origLinkAll
		unlinkAll = origUnlinkAll
	})

	callCount := 0
	linkAll = func(projectRoot string, targets []string, name, storePath string) error {
		callCount++
		if callCount == 2 {
			return fmt.Errorf("forced link failure for %s", name)
		}
		return target.LinkAll(projectRoot, targets, name, storePath)
	}
	unlinkAll = target.UnlinkAll

	svc := Service{ProjectDir: projectDir, HomeDir: homeDir}
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
