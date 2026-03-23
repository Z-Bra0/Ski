package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
)

func TestUpdateAdvancesLockfileAndInstalledTarget(t *testing.T) {
	t.Parallel()

	repoPath, oldCommit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "git:" + repoPath})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	newCommit := advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")

	var stdout bytes.Buffer
	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if got := lf.Skills[0].Commit; got != newCommit {
		t.Fatalf("lockfile commit = %q, want %q", got, newCommit)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", newCommit)
	assertInstalledSkillMatchesStore(t, linkPath, wantStore)
	if strings.Contains(stdout.String(), oldCommit[:7]) == false || !strings.Contains(stdout.String(), "updated 1 skills") {
		t.Fatalf("stdout = %q, want update summary", stdout.String())
	}
}

func TestUpdatePreservesInformationalVersionInLockfile(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:    "repo-map",
				Source:  "git:" + repoPath + "@v1.0.0",
				Version: "1.2.3",
			},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	installCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	installCmd.SetArgs([]string{"install"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v", err)
	}

	advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")

	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lock, ok := findLockSkillForTest(lf.Skills, "repo-map")
	if !ok {
		t.Fatal("lockfile missing repo-map entry")
	}
	if lock.Version != "1.2.3" {
		t.Fatalf("lockfile version = %q, want 1.2.3", lock.Version)
	}
}

func TestUpdateGlobalAdvancesHomeLockfileAndInstalledTarget(t *testing.T) {
	t.Parallel()

	repoPath, oldCommit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	globalManifestPath := manifest.GlobalPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(globalManifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := manifest.WriteFile(globalManifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "-g", "git:" + repoPath})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	newCommit := advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")

	var stdout bytes.Buffer
	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update", "-g"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(lockfile.GlobalPath(homeDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if got := lf.Skills[0].Commit; got != newCommit {
		t.Fatalf("lockfile commit = %q, want %q", got, newCommit)
	}

	linkPath := filepath.Join(homeDir, ".claude", "skills", "repo-map")
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", newCommit)
	assertInstalledSkillMatchesStore(t, linkPath, wantStore)
	if !strings.Contains(stdout.String(), oldCommit[:7]) || !strings.Contains(stdout.String(), "updated 1 skills") {
		t.Fatalf("stdout = %q, want update summary", stdout.String())
	}
}

func TestUpdateCheckReportsWithoutMutating(t *testing.T) {
	t.Parallel()

	repoPath, oldCommit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "git:" + repoPath})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	newCommit := advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")

	var stdout bytes.Buffer
	checkCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	checkCmd.SetArgs([]string{"update", "--check"})
	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("update --check Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if got := lf.Skills[0].Commit; got != oldCommit {
		t.Fatalf("lockfile commit = %q, want %q", got, oldCommit)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", oldCommit)
	assertInstalledSkillMatchesStore(t, linkPath, wantStore)
	if !strings.Contains(stdout.String(), newCommit[:7]) || !strings.Contains(stdout.String(), "1 skills can be updated") {
		t.Fatalf("stdout = %q, want check summary", stdout.String())
	}
}

func TestUpdateSpecificSkillOnly(t *testing.T) {
	t.Parallel()

	repoA, oldCommitA := createGitRepo(t, "repo-map", "repo-map")
	repoB, oldCommitB := createGitRepo(t, "audit-skill", "audit-skill")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoA})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add repoA Execute() error = %v", err)
	}
	cmd.SetArgs([]string{"add", "git:" + repoB})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add repoB Execute() error = %v", err)
	}

	newCommitA := advanceGitRepo(t, repoPathForURL(t, repoA), "repo-map", "second")

	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update", "repo-map"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lockA, ok := findLockSkillForTest(lf.Skills, "repo-map")
	if !ok || lockA.Commit != newCommitA {
		t.Fatalf("repo-map lock = %#v, want commit %q", lockA, newCommitA)
	}
	lockB, ok := findLockSkillForTest(lf.Skills, "audit-skill")
	if !ok || lockB.Commit != oldCommitB {
		t.Fatalf("audit-skill lock = %#v, want unchanged commit %q", lockB, oldCommitB)
	}
	if oldCommitA == newCommitA {
		t.Fatal("repo-map commit did not change in test setup")
	}
}

func TestUpdateCheckAcceptsSkillReference(t *testing.T) {
	t.Parallel()

	repoA, oldCommitA := createGitRepo(t, "repo-map", "repo-map")
	repoB, _ := createGitRepo(t, "audit-skill", "audit-skill")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "git:" + repoA})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add repoA Execute() error = %v", err)
	}
	addCmd.SetArgs([]string{"add", "git:" + repoB})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add repoB Execute() error = %v", err)
	}

	newCommitA := advanceGitRepo(t, repoPathForURL(t, repoA), "repo-map", "second")

	var stdout bytes.Buffer
	checkCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	checkCmd.SetArgs([]string{"update", "@1", "--check"})
	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("update --check Execute() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "repo-map "+oldCommitA[:7]+" -> "+newCommitA[:7]) {
		t.Fatalf("stdout = %q, want repo-map update line", stdout.String())
	}
	if strings.Contains(stdout.String(), "audit-skill") {
		t.Fatalf("stdout = %q, want no audit-skill update line", stdout.String())
	}
}

func TestUpdateGlobalMissingSkillIncludesGlobalManifestPath(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	globalManifestPath := manifest.GlobalPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(globalManifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := manifest.WriteFile(globalManifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update", "-g", "missing-skill"})

	err := updateCmd.Execute()
	if err == nil {
		t.Fatal("update Execute() error = nil, want missing skill error")
	}
	if !strings.Contains(err.Error(), globalManifestPath) {
		t.Fatalf("update error = %q, want manifest path %q", err.Error(), globalManifestPath)
	}
}

func TestUpdateFailsIfSkillNotFound(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Default()); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"update", "missing"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `skill "missing" not found`) {
		t.Fatalf("Execute() error = %v, want skill not found", err)
	}
}

func TestUpdateSkipsCommitPinnedSkill(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "repo-map", Source: "git:" + repoPath + "@" + commit},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	installCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	installCmd.SetArgs([]string{"install"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v", err)
	}

	advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")

	var checkOut bytes.Buffer
	checkCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &checkOut,
		Stderr:     &bytes.Buffer{},
	})
	checkCmd.SetArgs([]string{"update", "--check"})
	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("update --check Execute() error = %v", err)
	}
	if !strings.Contains(checkOut.String(), "all skills up to date") {
		t.Fatalf("stdout = %q, want pinned skill to be skipped", checkOut.String())
	}

	var updateOut bytes.Buffer
	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &updateOut,
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}
	if !strings.Contains(updateOut.String(), "all skills up to date") {
		t.Fatalf("stdout = %q, want pinned skill to be skipped", updateOut.String())
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lock, ok := findLockSkillForTest(lf.Skills, "repo-map")
	if !ok || lock.Commit != commit {
		t.Fatalf("lock = %#v, want unchanged commit %q", lock, commit)
	}
}

func TestUpdateTracksHexTagRefs(t *testing.T) {
	t.Parallel()

	repoPath, initialCommit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const hexTag = "deadbeef"

	runGit(t, repoPathForURL(t, repoPath), "tag", hexTag)

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "repo-map", Source: "git:" + repoPath + "@" + hexTag},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	installCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	installCmd.SetArgs([]string{"install"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v", err)
	}

	newCommit := advanceGitRepo(t, repoPathForURL(t, repoPath), "repo-map", "second")
	runGit(t, repoPathForURL(t, repoPath), "tag", "-f", hexTag)

	var checkOut bytes.Buffer
	checkCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &checkOut,
		Stderr:     &bytes.Buffer{},
	})
	checkCmd.SetArgs([]string{"update", "--check"})
	if err := checkCmd.Execute(); err != nil {
		t.Fatalf("update --check Execute() error = %v", err)
	}
	if !strings.Contains(checkOut.String(), initialCommit[:7]) || !strings.Contains(checkOut.String(), newCommit[:7]) {
		t.Fatalf("stdout = %q, want hex tag update summary", checkOut.String())
	}

	var updateOut bytes.Buffer
	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &updateOut,
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}
	if !strings.Contains(updateOut.String(), "updated 1 skills") {
		t.Fatalf("stdout = %q, want update summary", updateOut.String())
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lock, ok := findLockSkillForTest(lf.Skills, "repo-map")
	if !ok || lock.Commit != newCommit {
		t.Fatalf("lock = %#v, want updated commit %q", lock, newCommit)
	}
}

func advanceGitRepo(t *testing.T, repoPath, skillName, message string) string {
	t.Helper()

	content := `---
name: ` + skillName + `
description: Builds a repository map. Updated.
---

# ` + skillName + `
`
	if err := os.WriteFile(filepath.Join(repoPath, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", message)
	return strings.TrimSpace(runGitOutput(t, repoPath, "rev-parse", "HEAD"))
}

func findLockSkillForTest(skills []lockfile.Skill, name string) (lockfile.Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return lockfile.Skill{}, false
}
