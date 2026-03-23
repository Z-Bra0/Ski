package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Z-Bra0/Ski/internal/fsutil"
	"github.com/Z-Bra0/Ski/internal/manifest"
)

func installManifestForTest(t testing.TB, projectDir, homeDir string, doc manifest.Manifest) {
	t.Helper()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), doc); err != nil {
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
}

func overwriteStoredSkillFile(t testing.TB, homeDir, storeKey, commit, relativePath, content string) string {
	t.Helper()

	path := filepath.Join(homeDir, ".ski", "store", "git", storeKey, commit, relativePath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
	return path
}

func assertInstalledSkillMatchesStore(t testing.TB, installedPath, storePath string) {
	t.Helper()

	info, err := os.Stat(installedPath)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", installedPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", installedPath)
	}
	if _, err := os.Stat(filepath.Join(installedPath, "SKILL.md")); err != nil {
		t.Fatalf("Stat(%s/SKILL.md) error = %v", installedPath, err)
	}

	gotHash, err := fsutil.HashDir(installedPath)
	if err != nil {
		t.Fatalf("HashDir(%s) error = %v", installedPath, err)
	}
	wantHash, err := fsutil.HashDir(storePath)
	if err != nil {
		t.Fatalf("HashDir(%s) error = %v", storePath, err)
	}
	if gotHash != wantHash {
		t.Fatalf("installed dir hash = %q, want %q", gotHash, wantHash)
	}
}

func writeSimpleSkillDir(t testing.TB, dir, name string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s/SKILL.md) error = %v", dir, err)
	}
}

func writeInstalledSkillAndStore(t testing.TB, targetPath, storePath, name string) {
	t.Helper()
	writeSimpleSkillDir(t, storePath, name)
	writeSimpleSkillDir(t, targetPath, name)
}
