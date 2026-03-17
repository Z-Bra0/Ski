package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ski/internal/lockfile"
	"ski/internal/manifest"
)

// makeSymlink creates parent dirs and a symlink at linkPath -> target.
func makeSymlink(t *testing.T, linkPath, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("Symlink(%s -> %s) error = %v", linkPath, target, err)
	}
}

// fakeStorePath returns a non-existent but syntactically valid store path.
func fakeStorePath(homeDir, skillName, commit string) string {
	return filepath.Join(homeDir, ".ski", "store", "git", skillName, commit)
}

func removeCmd(t *testing.T, projectDir, homeDir string, args ...string) error {
	t.Helper()
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs(append([]string{"remove"}, args...))
	return cmd.Execute()
}

// writeFakeLockfile writes a lockfile with one skill entry using provided values.
func writeFakeLockfile(t *testing.T, projectDir, name, source, commit string, targets []string) {
	t.Helper()
	lf := lockfile.Lockfile{
		Version: 1,
		Skills: []lockfile.Skill{
			{
				Name:      name,
				Source:    source,
				Commit:    commit,
				Integrity: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Targets:   targets,
			},
		},
	}
	if err := lockfile.WriteFile(lockfile.Path(projectDir), lf); err != nil {
		t.Fatalf("WriteFile(lockfile) error = %v", err)
	}
}

func TestRemoveDeletesSymlinkAndMetadata(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Add the skill so the store, symlink, manifest, and lockfile are all real.
	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	addCmd.SetArgs([]string{"add", "git:" + repoPath})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if _, err := os.Lstat(linkPath); err != nil {
		t.Fatalf("symlink missing before remove: %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"remove", "repo-map"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Symlink must be gone.
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("symlink still exists after remove")
	}

	// Manifest must be empty.
	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 0 {
		t.Fatalf("manifest skills = %v, want empty", doc.Skills)
	}

	// Lockfile must be empty.
	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 0 {
		t.Fatalf("lockfile skills = %v, want empty", lf.Skills)
	}

	if got := stdout.String(); !strings.Contains(got, `removed skill "repo-map"`) {
		t.Fatalf("stdout = %q, want remove confirmation", got)
	}
}

func TestRemovePreservesStoreEntry(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
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

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)

	if err := removeCmd(t, projectDir, homeDir, "repo-map"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Store directory must survive remove.
	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("store entry missing after remove: %v", err)
	}
}

func TestRemoveWithPerSkillTargetOverride(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const (
		skillName = "repo-map"
		source    = "git:https://example.com/repo-map.git"
		commit    = "abc1234abc1234abc1234abc1234abc1234abc123"
	)

	// Manifest: global target is "claude" but this skill overrides to "codex".
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: skillName, Source: source, Targets: []string{"codex"}},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	writeFakeLockfile(t, projectDir, skillName, source, commit, []string{"codex"})

	// Place the symlink where the skill-level target says it should be.
	codexLink := filepath.Join(projectDir, ".codex", "skills", skillName)
	makeSymlink(t, codexLink, fakeStorePath(homeDir, skillName, commit))

	if err := removeCmd(t, projectDir, homeDir, skillName); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Per-skill target symlink must be gone.
	if _, err := os.Lstat(codexLink); !os.IsNotExist(err) {
		t.Fatalf("codex symlink still exists after remove")
	}

	// Global target symlink was never created, so there is nothing to assert
	// about it — but the remove must not have errored.
}

func TestRemoveCleansStaleTargetsFromLockfile(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	const (
		skillName = "repo-map"
		source    = "git:https://example.com/repo-map.git"
		commit    = "abc1234abc1234abc1234abc1234abc1234abc123"
	)

	// Manifest now targets "claude", but the lockfile still records "codex"
	// from a previous install before the user changed targets.
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{{Name: skillName, Source: source}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	writeFakeLockfile(t, projectDir, skillName, source, commit, []string{"codex"})

	storePath := fakeStorePath(homeDir, skillName, commit)
	claudeLink := filepath.Join(projectDir, ".claude", "skills", skillName)
	codexLink := filepath.Join(projectDir, ".codex", "skills", skillName)
	makeSymlink(t, claudeLink, storePath)
	makeSymlink(t, codexLink, storePath)

	if err := removeCmd(t, projectDir, homeDir, skillName); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Both the current manifest target and the stale lock target must be gone.
	if _, err := os.Lstat(claudeLink); !os.IsNotExist(err) {
		t.Fatalf("claude symlink still exists after remove")
	}
	if _, err := os.Lstat(codexLink); !os.IsNotExist(err) {
		t.Fatalf("stale codex symlink still exists after remove")
	}
}

func TestRemoveDeletesCustomTargetSymlink(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	customTarget := "dir:./agent-skills/claude"
	const (
		skillName = "repo-map"
		source    = "git:https://example.com/repo-map.git"
		commit    = "abc1234abc1234abc1234abc1234abc1234abc123"
	)

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{customTarget},
		Skills: []manifest.Skill{
			{Name: skillName, Source: source},
		},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	writeFakeLockfile(t, projectDir, skillName, source, commit, []string{customTarget})

	linkPath := filepath.Join(projectDir, filepath.Clean("./agent-skills/claude"), skillName)
	makeSymlink(t, linkPath, fakeStorePath(homeDir, skillName, commit))

	if err := removeCmd(t, projectDir, homeDir, skillName); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Fatalf("custom target symlink still exists after remove")
	}
}

func TestRemoveGlobalDeletesHomeSymlinkAndMetadata(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
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

	removeCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	removeCmd.SetArgs([]string{"remove", "-g", "repo-map"})
	if err := removeCmd.Execute(); err != nil {
		t.Fatalf("remove Execute() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(homeDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("global claude symlink still exists after remove")
	}

	doc, err := manifest.ReadFile(globalManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 0 {
		t.Fatalf("manifest skills = %v, want empty", doc.Skills)
	}

	lf, err := lockfile.ReadFile(lockfile.GlobalPath(homeDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 0 {
		t.Fatalf("lockfile skills = %v, want empty", lf.Skills)
	}
}

func TestRemoveFailsWithoutManifest(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	err := removeCmd(t, projectDir, homeDir, "repo-map")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "ski.toml not found") {
		t.Fatalf("Execute() error = %v, want ski.toml not found", err)
	}
}

func TestRemoveFailsIfSkillNotFound(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Default()); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	err := removeCmd(t, projectDir, homeDir, "nonexistent")
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `skill "nonexistent" not found`) {
		t.Fatalf("Execute() error = %v, want skill not found", err)
	}
}
