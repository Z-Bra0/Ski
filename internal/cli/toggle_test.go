package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/fsutil"
	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
)

func boolPtr(v bool) *bool { return &v }

func TestDisableRemovesInstalledTargetsAndKeepsLockfile(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
		}},
	})

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"disable", "repo-map"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("disable Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if doc.Skills[0].Enabled == nil || *doc.Skills[0].Enabled {
		t.Fatalf("manifest enabled = %#v, want false", doc.Skills[0].Enabled)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("installed target still exists after disable")
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != commit {
		t.Fatalf("lockfile = %#v, want preserved commit %q", lf.Skills, commit)
	}
	if !strings.Contains(stdout.String(), `disabled skill "repo-map"`) {
		t.Fatalf("stdout = %q, want disable confirmation", stdout.String())
	}
}

func TestDisableRemovesLockfileOnlyTargets(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
		}},
	})

	lockPath := lockfile.Path(projectDir)
	lf, err := lockfile.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	lf.Skills[0].Targets = []string{"claude", "codex"}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		t.Fatalf("WriteFile(lockfile) error = %v", err)
	}

	codexPath := filepath.Join(projectDir, ".codex", "skills", "repo-map")
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(codex dir) error = %v", err)
	}
	if err := fsutil.CopyTree(storePath, codexPath); err != nil {
		t.Fatalf("CopyTree(store -> codex) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"disable", "repo-map"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("disable Execute() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(projectDir, ".claude", "skills", "repo-map"),
		codexPath,
	} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("installed target %s still exists after disable", path)
		}
	}
}

func TestEnableAcceptsSkillReferenceAndRestoresTarget(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
			Enabled:       boolPtr(false),
		}},
	})
	if err := os.RemoveAll(filepath.Join(projectDir, ".claude", "skills", "repo-map")); err != nil {
		t.Fatalf("RemoveAll(target) error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"enable", "@1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("enable Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if doc.Skills[0].Enabled != nil {
		t.Fatalf("manifest enabled = %#v, want omitted/nil", doc.Skills[0].Enabled)
	}
	assertInstalledSkillMatchesStore(t,
		filepath.Join(projectDir, ".claude", "skills", "repo-map"),
		filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit),
	)
	if !strings.Contains(stdout.String(), `enabled skill "repo-map"`) {
		t.Fatalf("stdout = %q, want enable confirmation", stdout.String())
	}
}

func TestDisableRejectsAlreadyDisabledSkill(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
			Enabled:       boolPtr(false),
		}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"disable", "repo-map"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("disable Execute() error = nil, want already-disabled error")
	}
	if !strings.Contains(err.Error(), `skill "repo-map" is already disabled`) {
		t.Fatalf("disable Execute() error = %v", err)
	}
}

func TestEnableRejectsInvalidSkillReference(t *testing.T) {
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
	cmd.SetArgs([]string{"enable", "@2"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("enable Execute() error = nil, want out-of-range reference error")
	}
	if !strings.Contains(err.Error(), `skill reference "@2" out of range`) {
		t.Fatalf("enable Execute() error = %v", err)
	}
}

func TestInstallRemovesDisabledSkillTargets(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	doc := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
			Enabled:       boolPtr(false),
		}},
	}
	installManifestForTest(t, projectDir, homeDir, doc)

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(target dir) error = %v", err)
	}
	if err := fsutil.CopyTree(storePath, targetPath); err != nil {
		t.Fatalf("CopyTree(store -> target) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"install"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v", err)
	}

	if _, err := os.Lstat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("disabled target still exists after install")
	}
}

func TestUpdateRefreshesDisabledSkillWithoutReinstalling(t *testing.T) {
	t.Parallel()

	repoPath, oldCommit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath,
			UpstreamSkill: "repo-map",
		}},
	})

	disableCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	disableCmd.SetArgs([]string{"disable", "repo-map"})
	if err := disableCmd.Execute(); err != nil {
		t.Fatalf("disable Execute() error = %v", err)
	}

	repoDir := repoPathForURL(t, repoPath)
	if err := os.WriteFile(filepath.Join(repoDir, "update-marker.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "v2")
	newCommit := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	updateCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	updateCmd.SetArgs([]string{"update"})
	if err := updateCmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("disabled target exists after update")
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != newCommit || lf.Skills[0].Commit == oldCommit {
		t.Fatalf("lockfile = %#v, want updated disabled commit %q", lf.Skills, newCommit)
	}
}

func TestDoctorTreatsInstalledDisabledSkillAsUnexpected(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
			Enabled:       boolPtr(false),
		}},
	}); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	targetPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	writeInstalledSkillAndStore(t, targetPath, storePath, "repo-map")
	writeFakeLockfile(t, projectDir, "repo-map", "git:"+repoPath+"@v1.0.0", commit, []string{"claude"})

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"doctor"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("doctor Execute() error = nil, want unexpected target finding")
	}
	out := stdout.String()
	if !strings.Contains(out, "unexpected claude target") {
		t.Fatalf("stdout = %q, want unexpected target finding", out)
	}
	if strings.Contains(out, "missing claude target") {
		t.Fatalf("stdout = %q, should not report missing target for disabled skill", out)
	}
}

func TestAddUpdatesDisabledSkillInPlaceWithoutReenabling(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	installManifestForTest(t, projectDir, homeDir, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{{
			Name:          "repo-map",
			Source:        "git:" + repoPath + "@v1.0.0",
			UpstreamSkill: "repo-map",
		}},
	})

	disableCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	disableCmd.SetArgs([]string{"disable", "repo-map"})
	if err := disableCmd.Execute(); err != nil {
		t.Fatalf("disable Execute() error = %v", err)
	}

	repoDir := repoPathForURL(t, repoPath)
	if err := os.WriteFile(filepath.Join(repoDir, "update-marker.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(update-marker) error = %v", err)
	}
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "v2")
	newCommit := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "git:" + repoPath + "@" + newCommit})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if doc.Skills[0].Enabled == nil || *doc.Skills[0].Enabled {
		t.Fatalf("manifest enabled = %#v, want still disabled", doc.Skills[0].Enabled)
	}
	if doc.Skills[0].Source != "git:"+repoPath+"@"+newCommit {
		t.Fatalf("manifest source = %q, want updated ref", doc.Skills[0].Source)
	}
	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "repo-map")); !os.IsNotExist(err) {
		t.Fatalf("disabled target exists after add ref update")
	}

	lf, err := lockfile.ReadFile(lockfile.Path(projectDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != newCommit {
		t.Fatalf("lockfile = %#v, want updated commit %q", lf.Skills, newCommit)
	}
}
