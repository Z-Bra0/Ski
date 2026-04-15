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

func TestDoctorReportsUnmanagedLocalTargetEntry(t *testing.T) {
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

	unmanagedPath := filepath.Join(projectDir, ".claude", "skills", "manual-skill")
	if err := os.MkdirAll(unmanagedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(unmanagedPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(unmanagedPath, "SKILL.md"), []byte("---\nname: manual-skill\ndescription: manual\n---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
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
		t.Fatal("doctor Execute() error = nil, want unmanaged finding")
	}
	if !strings.Contains(err.Error(), "doctor found 1 issues") {
		t.Fatalf("doctor error = %v, want issue summary", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "unmanaged claude target") {
		t.Fatalf("stdout = %q, want unmanaged target finding", out)
	}
}

func TestDoctorFixReportsManualInterventionForUnmanagedLocalTargetEntry(t *testing.T) {
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

	unmanagedPath := filepath.Join(projectDir, ".claude", "skills", "manual-skill")
	if err := os.MkdirAll(unmanagedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(unmanagedPath) error = %v", err)
	}

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor", "--fix"})
	err := doctorCmd.Execute()
	if err == nil {
		t.Fatal("doctor --fix Execute() error = nil, want manual intervention error")
	}

	out := stdout.String()
	if !strings.Contains(out, "unmanaged claude target") {
		t.Fatalf("stdout = %q, want unmanaged target finding", out)
	}
	if !strings.Contains(out, "skipped: manual intervention required") {
		t.Fatalf("stdout = %q, want skipped output", out)
	}
	if !strings.Contains(out, "doctor: fixed 0 issues, 1 require manual intervention") {
		t.Fatalf("stdout = %q, want manual summary", out)
	}
	if _, err := os.Stat(unmanagedPath); err != nil {
		t.Fatalf("Stat(unmanagedPath) error = %v, want entry kept", err)
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

	// Local unmanaged entry should not affect global doctor.
	localUnmanagedPath := filepath.Join(projectDir, ".claude", "skills", "manual-skill")
	if err := os.MkdirAll(localUnmanagedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(localUnmanagedPath) error = %v", err)
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

func TestDoctorReportsUnmanagedEntryInLockOnlyCustomTargetFolder(t *testing.T) {
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

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(manifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile(updated manifest) error = %v", err)
	}

	unmanagedPath := filepath.Join(projectDir, "agent-skills", "claude", "manual-skill")
	if err := os.MkdirAll(unmanagedPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(unmanagedPath) error = %v", err)
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
	if !strings.Contains(out, "unmanaged dir:./agent-skills/claude target") {
		t.Fatalf("stdout = %q, want unmanaged custom target finding", out)
	}
}

func TestDoctorReportsIntegrityAndDriftProblems(t *testing.T) {
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.WriteFile(filepath.Join(targetPath, "notes.txt"), []byte("drifted"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
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
	if !strings.Contains(err.Error(), "doctor found 2 issues") {
		t.Fatalf("doctor error = %v, want issue summary", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "integrity mismatch") {
		t.Fatalf("stdout = %q, want integrity mismatch", out)
	}
	if !strings.Contains(out, "was modified and no longer matches the locked skill contents") {
		t.Fatalf("stdout = %q, want drift finding", out)
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

func TestDoctorReportsStaleTargetFromRemovedTarget(t *testing.T) {
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
	if !strings.Contains(out, "unexpected codex target") {
		t.Fatalf("stdout = %q, want stale codex target finding", out)
	}
}

func TestDoctorFixRepairsMissingTargetInstall(t *testing.T) {
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor", "--fix"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor --fix Execute() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "fixed: materialized claude target") {
		t.Fatalf("stdout = %q, want fixed materialize output", out)
	}
	if !strings.Contains(out, "doctor: fixed 1 issues") {
		t.Fatalf("stdout = %q, want fixed summary", out)
	}

	assertInstalledSkillMatchesStore(t, targetPath, filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit))

	stdout.Reset()
	doctorCmd = NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor"})
	if err := doctorCmd.Execute(); err != nil {
		t.Fatalf("doctor Execute() after fix error = %v", err)
	}
	if !strings.Contains(stdout.String(), "doctor: ok") {
		t.Fatalf("stdout = %q, want doctor ok after fix", stdout.String())
	}
}

func TestDoctorFixReportsManualInterventionForUnexpectedSymlinkEntry(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
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

	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}
	manualDir := t.TempDir()
	if err := os.Symlink(manualDir, targetPath); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	var stdout bytes.Buffer
	doctorCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	doctorCmd.SetArgs([]string{"doctor", "--fix"})
	err := doctorCmd.Execute()
	if err == nil {
		t.Fatal("doctor --fix Execute() error = nil, want manual intervention error")
	}

	out := stdout.String()
	if !strings.Contains(out, "skipped: manual intervention required") {
		t.Fatalf("stdout = %q, want skipped output", out)
	}
	if !strings.Contains(out, "doctor: fixed 0 issues, 1 require manual intervention") {
		t.Fatalf("stdout = %q, want manual summary", out)
	}
}
