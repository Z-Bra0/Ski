package store

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Z-Bra0/Ski/internal/fsutil"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/testutil"
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
	if err := os.WriteFile(filepath.Join(src, "helper.md"), []byte("helper notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(helper.md) error = %v", err)
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

	helperData, err := os.ReadFile(filepath.Join(dst, "helper.md"))
	if err != nil {
		t.Fatalf("ReadFile(helper.md) error = %v", err)
	}
	if string(helperData) != "helper notes\n" {
		t.Fatalf("helper.md = %q, want preserved content", string(helperData))
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

func TestFindGitReportsMalformedStoredSelectedSkill(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(stored.Path, "SKILL.md"), []byte(`---
name: alpha-skill
description: [unterminated
---
`), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	_, err = FindGit(homeDir, spec, stored.Commit, "alpha-skill")
	if err == nil {
		t.Fatal("FindGit() error = nil, want malformed selected skill error")
	}
	if strings.Contains(err.Error(), `skill "alpha-skill" not found in repository`) {
		t.Fatalf("FindGit() error = %v, want malformed skill error instead of not found", err)
	}
	if !strings.Contains(err.Error(), "parse YAML frontmatter") {
		t.Fatalf("FindGit() error = %v, want YAML parse error", err)
	}
}

func TestDiscoverGitRewritesInvalidOnlyRepoErrorsToStorePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := filepath.Join(root, "skill-pack")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "SKILL.md"), []byte(`---
name: repo-map
description: [unterminated
---
`), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	runGitTest(t, root, "init", repoPath)
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "broken skill")

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{URL: repoPath}

	_, err := DiscoverGit(projectDir, homeDir, spec)
	if err == nil {
		t.Fatal("DiscoverGit() error = nil, want malformed skill error")
	}
	if strings.Contains(err.Error(), "/checkout/SKILL.md") {
		t.Fatalf("DiscoverGit() error = %v, want rewritten store path instead of temp checkout path", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(homeDir, ".ski", "store", "git", "skill-pack")) {
		t.Fatalf("DiscoverGit() error = %v, want stable store path", err)
	}
}

func TestDiscoverGitRejectsSymlinkBeforePersistingSnapshot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := filepath.Join(root, "skill-pack")	
	writeSkillDir(t, filepath.Join(repoPath, "skills", "alpha-skill"), "alpha-skill")
	if err := os.Symlink("skills/alpha-skill/SKILL.md", filepath.Join(repoPath, "symlinked-skill")); err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("symlink not permitted on this filesystem: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}
	runGitTest(t, root, "init", repoPath)
	runGitTest(t, repoPath, "add", ".")
	runGitTest(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "add symlink")

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{URL: repoPath}

	_, err := DiscoverGit(projectDir, homeDir, spec)
	if err == nil {
		t.Fatal("DiscoverGit() error = nil, want symlink rejection")
	}
	if !errors.Is(err, fsutil.ErrSymlinkNotPermitted) {
		t.Fatalf("DiscoverGit() error = %v, want ErrSymlinkNotPermitted", err)
	}

	commit := strings.TrimSpace(gitOutputTest(t, repoPath, "rev-parse", "HEAD"))
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", commit)
	if _, statErr := os.Stat(storePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("store path %s should not exist after symlink rejection, stat err = %v", storePath, statErr)
	}
}

func TestFindGitReturnsSnapshotSymlinkErrorWithStorePath(t *testing.T) {
	t.Parallel()

	repoPath := createMultiSkillRepoWithSharedFile(t)
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{URL: repoPath, Skills: []string{"alpha-skill"}}

	stored, err := EnsureGit(projectDir, homeDir, spec, "alpha-skill")
	if err != nil {
		t.Fatalf("EnsureGit() error = %v", err)
	}

	poisoned := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", stored.Commit, "poison-link")
	if err := os.Symlink("shared.txt", poisoned); err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("symlink not permitted on this filesystem: %v", err)
		}
		t.Fatalf("Symlink() error = %v", err)
	}

	_, err = FindGit(homeDir, spec, stored.Commit, "alpha-skill")
	if err == nil {
		t.Fatal("FindGit() error = nil, want SnapshotSymlinkError")
	}
	var symlinkErr SnapshotSymlinkError
	if !errors.As(err, &symlinkErr) {
		t.Fatalf("FindGit() error = %v, want SnapshotSymlinkError", err)
	}
	if symlinkErr.Root != filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", stored.Commit) {
		t.Fatalf("SnapshotSymlinkError.Root = %q", symlinkErr.Root)
	}
	if !errors.Is(err, fsutil.ErrSymlinkNotPermitted) {
		t.Fatalf("FindGit() error = %v, want ErrSymlinkNotPermitted", err)
	}
}

func TestFindGitFallsBackToLegacyStoreKey(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "acme/repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{URL: repo.URL}

	stored, err := EnsureGit(projectDir, homeDir, spec, "repo-map")
	if err != nil {
		t.Fatalf("EnsureGit() error = %v", err)
	}

	primaryKey, err := spec.DeriveName()
	if err != nil {
		t.Fatalf("DeriveName() error = %v", err)
	}
	legacyKey, err := spec.DeriveLegacyName()
	if err != nil {
		t.Fatalf("DeriveLegacyName() error = %v", err)
	}
	if primaryKey == legacyKey {
		t.Fatalf("test setup: primary and legacy keys are equal: %q", primaryKey)
	}

	primaryPath := filepath.Join(homeDir, ".ski", "store", "git", primaryKey, stored.Commit)
	legacyPath := filepath.Join(homeDir, ".ski", "store", "git", legacyKey, stored.Commit)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(legacy parent) error = %v", err)
	}
	if err := os.Rename(primaryPath, legacyPath); err != nil {
		t.Fatalf("Rename(primary->legacy) error = %v", err)
	}

	reloaded, err := FindGit(homeDir, spec, stored.Commit, "repo-map")
	if err != nil {
		t.Fatalf("FindGit() error = %v", err)
	}
	if !strings.HasPrefix(reloaded.Path, legacyPath) {
		t.Fatalf("FindGit() path = %q, want legacy-key path under %q", reloaded.Path, legacyPath)
	}
}

func TestRefreshGitPreservesExistingSnapshotWhenReplaceFails(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	spec := source.Git{URL: repo.URL}

	stored, err := EnsureGit(projectDir, homeDir, spec, "repo-map")
	if err != nil {
		t.Fatalf("EnsureGit() error = %v", err)
	}

	markerPath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", stored.Commit, "local-cache.txt")
	if err := os.WriteFile(markerPath, []byte("preserve me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(local-cache.txt) error = %v", err)
	}

	origRename := renameDir
	t.Cleanup(func() {
		renameDir = origRename
	})

	replaceAttempt := 0
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", stored.Commit)
	renameDir = func(oldpath, newpath string) error {
		if newpath == storePath {
			replaceAttempt++
			if replaceAttempt == 1 {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EPERM}
			}
		}
		return origRename(oldpath, newpath)
	}

	_, err = RefreshGit(projectDir, homeDir, spec)
	if err == nil {
		t.Fatal("RefreshGit() error = nil, want replace failure")
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("ReadFile(local-cache.txt) error = %v", err)
	}
	if string(data) != "preserve me\n" {
		t.Fatalf("local-cache.txt = %q, want preserved marker", string(data))
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

func gitOutputTest(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output)
}
