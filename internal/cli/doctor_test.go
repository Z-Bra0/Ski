package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
)

func TestDoctorReportsHealthyProject(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:    "repo-map",
				Source:  "git:" + repoPath + "@v1.0.0",
				Targets: []string{"codex"},
			},
		},
	})

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor Execute() error = %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "doctor: ok") {
		t.Fatalf("stdout = %q, want healthy doctor output", got)
	}
}

func TestDoctorReportsHealthyGlobalScope(t *testing.T) {
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

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor", "-g"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor Execute() error = %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "doctor: ok") {
		t.Fatalf("stdout = %q, want healthy doctor output", got)
	}
}

func TestDoctorSupportsCustomTargetFolder(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	customTarget := "dir:./agent-skills/claude"

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{customTarget},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:" + repoPath + "@v1.0.0",
			},
		},
	})

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor Execute() error = %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "doctor: ok") {
		t.Fatalf("stdout = %q, want healthy doctor output", got)
	}
}

func TestDoctorReportsIntegrityAndSymlinkProblems(t *testing.T) {
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

	overwriteStoredSkillFile(t, homeDir, "repo-map", commit, "SKILL.md", `---
name: repo-map
description: tampered
---
`)

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.Remove(linkPath); err != nil {
		t.Fatalf("Remove(link) error = %v", err)
	}
	makeSymlink(t, linkPath, fakeStorePath(homeDir, "repo-map", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"))

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})

	err := doctorCmd.Execute()
	if err == nil {
		t.Fatal("doctor Execute() error = nil, want findings")
	}
	if !strings.Contains(err.Error(), "doctor found 2 issues") {
		t.Fatalf("doctor error = %v, want issue summary", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "integrity mismatch") {
		t.Fatalf("stdout = %q, want integrity mismatch", out)
	}
	if !strings.Contains(out, "symlink points to") {
		t.Fatalf("stdout = %q, want symlink mismatch", out)
	}
}

func TestDoctorReportsMalformedStoredSelectedSkill(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:" + repoPath + "@v1.0.0",
			},
		},
	})
	overwriteStoredSkillFile(t, homeDir, "repo-map", commit, "SKILL.md", `---
name: repo-map
description: [unterminated
---
`)

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})

	err := doctorCmd.Execute()
	if err == nil {
		t.Fatal("doctor Execute() error = nil, want findings")
	}
	if !strings.Contains(err.Error(), "doctor found 1 issues") {
		t.Fatalf("doctor error = %v, want issue summary", err)
	}

	out := stdout.String()
	if strings.Contains(out, `skill "repo-map" not found in repository`) {
		t.Fatalf("stdout = %q, want malformed skill error instead of not found", out)
	}
	if !strings.Contains(out, "parse YAML frontmatter") {
		t.Fatalf("stdout = %q, want malformed skill finding", out)
	}
}

func TestDoctorReportsStaleSymlinkFromRemovedTarget(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:" + repoPath + "@v1.0.0",
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

	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:   "repo-map",
				Source: "git:" + repoPath + "@v1.0.0",
			},
		},
	}); err != nil {
		t.Fatalf("WriteFile(updated manifest) error = %v", err)
	}

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})

	err := doctorCmd.Execute()
	if err == nil {
		t.Fatal("doctor Execute() error = nil, want findings")
	}

	out := stdout.String()
	if !strings.Contains(out, "lockfile targets [claude codex] do not match manifest targets [claude]") {
		t.Fatalf("stdout = %q, want target mismatch", out)
	}
	if !strings.Contains(out, "unexpected codex symlink") {
		t.Fatalf("stdout = %q, want stale codex symlink finding", out)
	}
}
