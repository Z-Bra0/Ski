package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDirAcceptsValidSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
description: Builds a repository map. Use when the user asks for codebase structure or dependency summaries.
compatibility: Requires git
metadata:
  author: acme
allowed-tools: Bash(git:*) Read
---

# Repo Map
`)

	meta, err := ValidateDir(dir, "repo-map")
	if err != nil {
		t.Fatalf("ValidateDir() error = %v", err)
	}
	if meta.Name != "repo-map" {
		t.Fatalf("meta.Name = %q, want repo-map", meta.Name)
	}
}

func TestValidateDirRejectsMissingSkillFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := ValidateDir(dir, "repo-map")
	if err == nil {
		t.Fatal("ValidateDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("ValidateDir() error = %v, want missing file error", err)
	}
}

func TestValidateDirRejectsNameMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: other-skill
description: Does a thing. Use when the user asks for that thing.
---
`)

	_, err := ValidateDir(dir, "repo-map")
	if err == nil {
		t.Fatal("ValidateDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "must match installed directory name") {
		t.Fatalf("ValidateDir() error = %v, want name mismatch error", err)
	}
}

func writeSkillFile(t *testing.T, dir string, content string) {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
