package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
