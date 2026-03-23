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

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
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
	})

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
	assertContains(t, out, "target claude: installed")
}

func TestInfoAcceptsSkillReference(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	})

	var stdout bytes.Buffer
	infoCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	infoCmd.SetArgs([]string{"info", "@1"})
	if err := infoCmd.Execute(); err != nil {
		t.Fatalf("info Execute() error = %v", err)
	}

	assertContains(t, stdout.String(), "name: repo-map")
}

func TestInfoReportsDriftedTarget(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	})

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.WriteFile(filepath.Join(targetPath, "notes.txt"), []byte("drifted"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
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
	assertContains(t, out, "target claude: drifted")
	assertContains(t, out, filepath.Join(projectDir, ".claude", "skills", "repo-map"))
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

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	})

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

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	})
	overwriteStoredSkillFile(t, homeDir, "repo-map", commit, "SKILL.md", `---
name: repo-map
description: [unterminated
---
`)

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
