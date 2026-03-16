package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ski/internal/lockfile"
	"ski/internal/manifest"
)

func TestInstallFromLockfile(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	// Write manifest with the skill
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{{Name: "repo-map", Source: "git:" + repoPath + "@v1.0.0"}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	// Run ski add first to populate store and produce lockfile
	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "git:" + repoPath + "@v1.0.0"})
	// Reset manifest to not have the skill so add can proceed
	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest reset) error = %v", err)
	}
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	// Remove symlinks to simulate a fresh clone
	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.Remove(linkPath); err != nil {
		t.Fatalf("Remove(symlink) error = %v", err)
	}

	// Now run ski install
	var stdout bytes.Buffer
	installCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	installCmd.SetArgs([]string{"install"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v", err)
	}

	// Symlink should be restored
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if targetPath != storePath {
		t.Fatalf("symlink target = %q, want %q", targetPath, storePath)
	}

	if got := stdout.String(); !strings.Contains(got, "installed 1 skills") {
		t.Fatalf("stdout = %q, want installed confirmation", got)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	// Add the skill
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

	// Run install twice — should succeed both times
	for i := range 2 {
		var stdout bytes.Buffer
		installCmd := NewRootCmd(Options{
			Getwd:      func() (string, error) { return projectDir, nil },
			GetHomeDir: func() (string, error) { return homeDir, nil },
			Stdout:     &stdout,
			Stderr:     &bytes.Buffer{},
		})
		installCmd.SetArgs([]string{"install"})
		if err := installCmd.Execute(); err != nil {
			t.Fatalf("install #%d Execute() error = %v", i+1, err)
		}
		if got := stdout.String(); !strings.Contains(got, "installed 1 skills") {
			t.Fatalf("install #%d stdout = %q, want installed confirmation", i+1, got)
		}
	}
}

func TestInstallWithoutLockfileGeneratesOne(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{{Name: "repo-map", Source: "git:" + repoPath + "@v1.0.0"}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	// No lockfile — install should fetch and create one
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

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != commit {
		t.Fatalf("lockfile = %#v, want commit %q", lf.Skills, commit)
	}
	if !reflect.DeepEqual(lf.Skills[0].Targets, []string{"claude"}) {
		t.Fatalf("lockfile targets = %#v, want [claude]", lf.Skills[0].Targets)
	}
}

func TestInstallUsesPerSkillTargetOverrides(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:    "repo-map",
				Source:  "git:" + repoPath + "@v1.0.0",
				Targets: []string{"codex"},
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

	codexLink := filepath.Join(projectDir, ".codex", "skills", "repo-map")
	targetPath, err := os.Readlink(codexLink)
	if err != nil {
		t.Fatalf("Readlink(codex) error = %v", err)
	}
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if targetPath != wantStore {
		t.Fatalf("codex symlink target = %q, want %q", targetPath, wantStore)
	}

	claudeLink := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if _, err := os.Lstat(claudeLink); !os.IsNotExist(err) {
		t.Fatalf("claude link exists = %v, want missing", err)
	}

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if !reflect.DeepEqual(lf.Skills[0].Targets, []string{"codex"}) {
		t.Fatalf("lockfile targets = %#v, want [codex]", lf.Skills[0].Targets)
	}
}

func TestInstallFailsWithoutManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"install"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run `ski init` first") {
		t.Fatalf("Execute() error = %v, want init guidance", err)
	}
}

func TestInstallEmptyManifest(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Default()); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"install"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "installed 0 skills") {
		t.Fatalf("stdout = %q, want 0 skills", got)
	}
}
