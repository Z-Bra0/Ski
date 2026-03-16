package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"ski/internal/source"
)

func TestMoveDirIntoStoreFallsBackOnCrossDeviceRename(t *testing.T) {
	t.Parallel()

	srcRoot := t.TempDir()
	dstRoot := t.TempDir()
	src := filepath.Join(srcRoot, "checkout")
	dst := filepath.Join(dstRoot, "store", "git", "repo-map", "commit")

	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("skill"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "helper.sh"), []byte("echo helper\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(helper.sh) error = %v", err)
	}
	if err := os.Symlink("nested/helper.sh", filepath.Join(src, "helper")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll(store parent) error = %v", err)
	}

	origRename := renameDir
	t.Cleanup(func() {
		renameDir = origRename
	})

	callCount := 0
	renameDir = func(oldpath, newpath string) error {
		callCount++
		if callCount == 1 {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
		}
		return origRename(oldpath, newpath)
	}

	if err := moveDirIntoStore(src, dst); err != nil {
		t.Fatalf("moveDirIntoStore() error = %v", err)
	}
	if callCount < 2 {
		t.Fatalf("renameDir call count = %d, want fallback path", callCount)
	}

	data, err := os.ReadFile(filepath.Join(dst, "nested", "helper.sh"))
	if err != nil {
		t.Fatalf("ReadFile(helper.sh) error = %v", err)
	}
	if string(data) != "echo helper\n" {
		t.Fatalf("helper.sh = %q, want preserved content", string(data))
	}

	info, err := os.Stat(filepath.Join(dst, "nested", "helper.sh"))
	if err != nil {
		t.Fatalf("Stat(helper.sh) error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("helper.sh perms = %#o, want 0755", info.Mode().Perm())
	}

	linkTarget, err := os.Readlink(filepath.Join(dst, "helper"))
	if err != nil {
		t.Fatalf("Readlink(helper) error = %v", err)
	}
	if linkTarget != "nested/helper.sh" {
		t.Fatalf("helper symlink target = %q, want nested/helper.sh", linkTarget)
	}
}

func TestFindGitIntegrityCoversWholeRepoSnapshotForSelectedSkill(t *testing.T) {
	t.Parallel()

	repoPath := createMultiSkillRepoWithSharedFile(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{
		URL:    repoPath,
		Skills: []string{"alpha-skill"},
	}

	stored, err := EnsureGit(projectDir, homeDir, spec, "alpha-skill")
	if err != nil {
		t.Fatalf("EnsureGit() error = %v", err)
	}

	storeRoot := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", stored.Commit)
	if err := os.WriteFile(filepath.Join(storeRoot, "shared.txt"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(shared.txt) error = %v", err)
	}

	reloaded, err := FindGit(homeDir, spec, stored.Commit, "alpha-skill")
	if err != nil {
		t.Fatalf("FindGit() error = %v", err)
	}
	if reloaded.Integrity == stored.Integrity {
		t.Fatalf("integrity = %q, want hash to change after shared-file tamper", reloaded.Integrity)
	}
}

func createMultiSkillRepoWithSharedFile(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, "skill-pack")

	writeSkillDir(t, filepath.Join(repoPath, "skills", "alpha-skill"), "alpha-skill")
	writeSkillDir(t, filepath.Join(repoPath, "skills", "beta-skill"), "beta-skill")
	if err := os.WriteFile(filepath.Join(repoPath, "shared.txt"), []byte("shared\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(shared.txt) error = %v", err)
	}

	runGitTest(t, root, "init", repoPath)
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	return repoPath
}

func writeSkillDir(t *testing.T, dir string, name string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	skillDoc := `---
name: ` + name + `
description: Test skill for store integrity coverage.
---

# ` + name + `
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
		t.Fatalf("git %v error = %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
}
