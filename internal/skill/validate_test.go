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
	if got, want := meta.AllowedTools.Strings(), []string{"Bash(git:*) Read"}; !slicesEqual(got, want) {
		t.Fatalf("meta.AllowedTools = %#v, want %#v", got, want)
	}
}

func TestValidateDirIgnoresUnknownFrontmatterFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
version: 1.1.0
description: Builds a repository map.
custom-field: keep-ignored
---
`)

	meta, err := ValidateDir(dir, "repo-map")
	if err != nil {
		t.Fatalf("ValidateDir() error = %v", err)
	}
	if meta.Name != "repo-map" {
		t.Fatalf("meta.Name = %q, want repo-map", meta.Name)
	}
}

func TestValidateDirAcceptsAllowedToolsList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
description: Builds a repository map.
allowed-tools:
  - Bash
  - Read
  - AskUserQuestion
---
`)

	meta, err := ValidateDir(dir, "repo-map")
	if err != nil {
		t.Fatalf("ValidateDir() error = %v", err)
	}
	if got, want := meta.AllowedTools.Strings(), []string{"Bash", "Read", "AskUserQuestion"}; !slicesEqual(got, want) {
		t.Fatalf("meta.AllowedTools = %#v, want %#v", got, want)
	}
}

func TestDiscoverNameAllowsOtherwiseInvalidSkillMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
version: 1.1.0
---
`)

	name, err := DiscoverName(dir)
	if err != nil {
		t.Fatalf("DiscoverName() error = %v", err)
	}
	if name != "repo-map" {
		t.Fatalf("DiscoverName() = %q, want repo-map", name)
	}
}

func TestDiscoverCandidateNameRecoversNameFromMalformedFrontmatter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
description: [unterminated
---
`)

	if got := DiscoverCandidateName(dir); got != "repo-map" {
		t.Fatalf("DiscoverCandidateName() = %q, want repo-map", got)
	}
}

func TestValidateDirWithWarningsReportsStrictSpecMismatches(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
version: 1.1.0
description: `+strings.Repeat("x", 1025)+`
allowed-tools:
  - Bash
  - Read
---
`)

	meta, warnings, err := ValidateDirWithWarnings(dir, "repo-map")
	if err != nil {
		t.Fatalf("ValidateDirWithWarnings() error = %v", err)
	}
	if meta.Name != "repo-map" {
		t.Fatalf("meta.Name = %q, want repo-map", meta.Name)
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %#v, want 3 warnings", warnings)
	}
	assertWarningContains(t, warnings, `skill "repo-map" (`+filepath.Join(dir, FileName)+`): unknown frontmatter field "version"`)
	assertWarningContains(t, warnings, `skill "repo-map" (`+filepath.Join(dir, FileName)+`): description exceeds the Agent Skills spec limit of 1024 characters`)
	assertWarningContains(t, warnings, `skill "repo-map" (`+filepath.Join(dir, FileName)+`): allowed-tools should use the Agent Skills space-delimited string form`)
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

func TestValidateDirRejectsNonStringAllowedToolsEntry(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSkillFile(t, dir, `---
name: repo-map
description: Builds a repository map.
allowed-tools:
  - Bash
  - foo: bar
---
`)

	_, err := ValidateDir(dir, "repo-map")
	if err == nil {
		t.Fatal("ValidateDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "allowed-tools entries must be strings") {
		t.Fatalf("ValidateDir() error = %v, want allowed-tools type error", err)
	}
}

func writeSkillFile(t *testing.T, dir string, content string) {
	t.Helper()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertWarningContains(t *testing.T, warnings []ValidationWarning, want string) {
	t.Helper()

	for _, warning := range warnings {
		got := warningString(warning)
		if strings.Contains(got, want) {
			return
		}
	}
	t.Fatalf("warnings = %#v, want %q", warnings, want)
}

func warningString(warning ValidationWarning) string {
	return `skill "` + warning.Name + `" (` + warning.Path + `): ` + warning.Message
}
