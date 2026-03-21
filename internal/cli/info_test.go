package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
)

func TestInfoShowsDetailedSkillState(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
				Version:       "1.2.3",
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

	var stdout bytes.Buffer
	infoCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	infoCmd.SetArgs([]string{"info", "repo-map"})
	if err := infoCmd.Execute(); err != nil {
		t.Fatalf("info Execute() error = %v", err)
	}

	out := stdout.String()
	assertContains(t, out, "name: repo-map")
	assertContains(t, out, "source: git:"+repoPath+"@v1.0.0")
	assertContains(t, out, "upstream: repo-map")
	assertContains(t, out, "version: 1.2.3")
	assertContains(t, out, "commit: "+commit)
	assertContains(t, out, "integrity: sha256:")
	assertContains(t, out, "store path: "+filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit))
	assertContains(t, out, "targets: claude")
	assertContains(t, out, "target claude: linked")
}

func TestInfoReportsDriftedTarget(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
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

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.Remove(linkPath); err != nil {
		t.Fatalf("Remove(link) error = %v", err)
	}
	makeSymlink(t, linkPath, fakeStorePath(homeDir, "repo-map", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"))

	var stdout bytes.Buffer
	infoCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	infoCmd.SetArgs([]string{"info", "repo-map"})
	if err := infoCmd.Execute(); err != nil {
		t.Fatalf("info Execute() error = %v", err)
	}

	out := stdout.String()
	assertContains(t, out, "target claude: drifted")
	assertContains(t, out, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
}

func TestInfoErrorsForUnknownSkill(t *testing.T) {
	t.Parallel()

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
	cmd.SetArgs([]string{"info", "repo-map"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `skill "repo-map" not found in `) {
		t.Fatalf("Execute() error = %v, want missing skill error", err)
	}
}

func TestInfoReportsMissingStoreSnapshot(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
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

	storeRoot := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if err := os.RemoveAll(storeRoot); err != nil {
		t.Fatalf("RemoveAll(storeRoot) error = %v", err)
	}

	var stdout bytes.Buffer
	infoCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	infoCmd.SetArgs([]string{"info", "repo-map"})
	if err := infoCmd.Execute(); err != nil {
		t.Fatalf("info Execute() error = %v", err)
	}

	out := stdout.String()
	assertContains(t, out, "store error:")
	assertContains(t, out, "store unavailable")
}

func TestInfoReportsMalformedStoredSelectedSkill(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
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

	storeRoot := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if err := os.WriteFile(filepath.Join(storeRoot, "SKILL.md"), []byte(`---
name: repo-map
description: [unterminated
---
`), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}

	var stdout bytes.Buffer
	infoCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	infoCmd.SetArgs([]string{"info", "repo-map"})
	if err := infoCmd.Execute(); err != nil {
		t.Fatalf("info Execute() error = %v", err)
	}

	out := stdout.String()
	assertContains(t, out, "store error:")
	assertContains(t, out, "parse YAML frontmatter")
	assertContains(t, out, "target claude: store unavailable")
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
}
