package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ski/internal/lockfile"
	"ski/internal/manifest"
)

func TestAddFetchesWritesLockfileAndLinksTargets(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath + "@v1.0.0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:" + repoPath + "@v1.0.0",
			},
		},
	}
	if !reflect.DeepEqual(*doc, wantManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, wantManifest)
	}

	lockPath := filepath.Join(projectDir, lockfile.FileName)
	lf, err := lockfile.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 {
		t.Fatalf("lockfile skills = %#v, want one entry", lf.Skills)
	}
	gotLock := lf.Skills[0]
	if gotLock.Name != "repo-map" || gotLock.Source != "git:"+repoPath+"@v1.0.0" || gotLock.Commit != commit {
		t.Fatalf("lock skill = %#v, want matching name/source/commit", gotLock)
	}
	if gotLock.Integrity == "" || !strings.HasPrefix(gotLock.Integrity, "sha256:") {
		t.Fatalf("lock integrity = %q, want sha256 hash", gotLock.Integrity)
	}
	if !reflect.DeepEqual(gotLock.Targets, []string{"claude"}) {
		t.Fatalf("lock targets = %#v, want [claude]", gotLock.Targets)
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if _, err := os.Stat(filepath.Join(storePath, "SKILL.md")); err != nil {
		t.Fatalf("store SKILL.md error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storePath, ".git")); !os.IsNotExist(err) {
		t.Fatalf("store .git stat error = %v, want not exist", err)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if targetPath != storePath {
		t.Fatalf("symlink target = %q, want %q", targetPath, storePath)
	}

	if got := stdout.String(); !strings.Contains(got, "added repo-map") {
		t.Fatalf("stdout = %q, want add confirmation", got)
	}
}

func TestAddSupportsNameOverride(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "custom-name")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath, "--name", "custom-name"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(doc.Skills) != 1 || doc.Skills[0].Name != "custom-name" {
		t.Fatalf("skills = %#v, want custom-name", doc.Skills)
	}

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Name != "custom-name" {
		t.Fatalf("lockfile skills = %#v, want custom-name", lf.Skills)
	}
}

func TestAddFailsWithoutManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:https://github.com/acme/repo-map.git"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run `ski init` first") {
		t.Fatalf("Execute() error = %v, want init guidance", err)
	}
}

func TestAddRejectsInvalidSource(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "github:acme/repo-map"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "expected git:<url>[@ref]") {
		t.Fatalf("Execute() error = %v, want git source error", err)
	}
}

func TestAddRejectsDuplicateDerivedName(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	doc := manifest.Manifest{
		Version: 1,
		Targets: []string{},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:https://github.com/acme/original-repo-map.git",
			},
		},
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:https://github.com/other/repo-map.git"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "rerun with --name") {
		t.Fatalf("Execute() error = %v, want name override guidance", err)
	}
}

func TestAddRejectsDuplicateSource(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	doc := manifest.Manifest{
		Version: 1,
		Targets: []string{},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:https://github.com/acme/repo-map.git@v1.0.0",
			},
		},
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:https://github.com/acme/repo-map.git@v1.0.0", "--name", "other"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "already exists as skill") {
		t.Fatalf("Execute() error = %v, want duplicate source error", err)
	}
}

func TestAddRejectsInvalidSkillRepository(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repoRoot := t.TempDir()
	repoPath := filepath.Join(repoRoot, "repo-map")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# not a skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	runGit(t, repoRoot, "init", repoPath)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "SKILL.md") {
		t.Fatalf("Execute() error = %v, want invalid skill error", err)
	}
}

func createGitRepo(t *testing.T, repoName string, skillName string) (string, string) {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	if err := os.MkdirAll(filepath.Join(repoPath, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skillDoc := `---
name: ` + skillName + `
description: Builds a repository map. Use when the user asks for codebase structure or repository summaries.
---

# ` + skillName + `
`
	if err := os.WriteFile(filepath.Join(repoPath, "SKILL.md"), []byte(skillDoc), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "tools", "helper.sh"), []byte("echo helper\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(helper.sh) error = %v", err)
	}

	runGit(t, root, "init", repoPath)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	runGit(t, repoPath, "tag", "v1.0.0")

	commit := runGitOutput(t, repoPath, "rev-parse", "HEAD")
	return repoPath, strings.TrimSpace(commit)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
	return string(output)
}
