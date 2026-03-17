package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/source"
)

func TestAddFetchesWritesLockfileAndLinksTargets(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath + "@v1.0.0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	}
	if !reflect.DeepEqual(*doc, wantManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, wantManifest)
	}

	lockPath := filepath.Join(projectDir, lockfile.FileName)
	lf, err := lockfile.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 {
		t.Fatalf("lockfile skills = %#v, want one entry", lf.Skills)
	}
	gotLock := lf.Skills[0]
	if gotLock.Name != "repo-map" || gotLock.Source != "git:"+repoPath+"@v1.0.0" || gotLock.UpstreamSkill != "repo-map" || gotLock.Commit != commit {
		t.Fatalf("lock skill = %#v, want matching name/source/commit", gotLock)
	}
	if gotLock.Integrity == "" || !strings.HasPrefix(gotLock.Integrity, "sha256:") {
		t.Fatalf("lock integrity = %q, want sha256 hash", gotLock.Integrity)
	}
	if !reflect.DeepEqual(gotLock.Targets, []string{"claude"}) {
		t.Fatalf("lock targets = %#v, want [claude]", gotLock.Targets)
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if _, err := os.Stat(filepath.Join(storePath, "SKILL.md")); err != nil {
		t.Fatalf("store SKILL.md error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(storePath, ".git")); !os.IsNotExist(err) {
		t.Fatalf("store .git stat error = %v, want not exist", err)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "repo-map")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if targetPath != storePath {
		t.Fatalf("symlink target = %q, want %q", targetPath, storePath)
	}

	if got := stdout.String(); !strings.Contains(got, "added repo-map") {
		t.Fatalf("stdout = %q, want add confirmation", got)
	}
}

func TestAddSupportsNameOverride(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath, "--name", "custom-name"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(doc.Skills) != 1 || doc.Skills[0].Name != "custom-name" || doc.Skills[0].Source != "git:"+repoPath || doc.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("skills = %#v, want custom-name", doc.Skills)
	}

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Name != "custom-name" || lf.Skills[0].Source != "git:"+repoPath || lf.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("lockfile skills = %#v, want custom-name", lf.Skills)
	}
	if !reflect.DeepEqual(lf.Skills[0].Targets, []string{"claude"}) {
		t.Fatalf("lockfile targets = %#v, want [claude]", lf.Skills[0].Targets)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "custom-name")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if targetPath != wantStore {
		t.Fatalf("symlink target = %q, want %q", targetPath, wantStore)
	}
}

func TestAddSupportsEscapedRepoPathContainingDoubleHash(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "skill##pack", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", source.Git{URL: repoPath}.String()})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	wantSource := source.Git{URL: repoPath}.String()
	if len(doc.Skills) != 1 || doc.Skills[0].Source != wantSource || doc.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("skills = %#v, want source %q with upstream skill repo-map", doc.Skills, wantSource)
	}
}

func TestAddSupportsBareRemoteFileURL(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fileURL := "file://" + repoPath
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", fileURL})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	wantSource := source.Git{URL: fileURL}.String()
	if len(doc.Skills) != 1 || doc.Skills[0].Source != wantSource || doc.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("skills = %#v, want source %q with upstream skill repo-map", doc.Skills, wantSource)
	}
}

func TestAddLinksIntoCustomTargetFolder(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	customTarget := "dir:./agent-skills/claude"

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{customTarget},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if !reflect.DeepEqual(lf.Skills[0].Targets, []string{customTarget}) {
		t.Fatalf("lockfile targets = %#v, want [%q]", lf.Skills[0].Targets, customTarget)
	}

	linkPath := filepath.Join(projectDir, filepath.Clean("./agent-skills/claude"), "repo-map")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if targetPath != wantStore {
		t.Fatalf("symlink target = %q, want %q", targetPath, wantStore)
	}
}

func TestAddGlobalWritesGlobalStateAndLinksToHome(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
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
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "-g", "git:" + repoPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(globalManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 1 || doc.Skills[0].Name != "repo-map" {
		t.Fatalf("manifest skills = %#v, want global repo-map entry", doc.Skills)
	}

	lf, err := lockfile.ReadFile(lockfile.GlobalPath(homeDir))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 || lf.Skills[0].Commit != commit {
		t.Fatalf("lockfile skills = %#v, want one entry with commit %q", lf.Skills, commit)
	}

	linkPath := filepath.Join(homeDir, ".claude", "skills", "repo-map")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	wantStore := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	if targetPath != wantStore {
		t.Fatalf("symlink target = %q, want %q", targetPath, wantStore)
	}
}

func TestAddGlobalCanonicalizesRelativeLocalSource(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := filepath.Join(filepath.Dir(repoPath), "work")
	homeDir := t.TempDir()

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	globalManifestPath := manifest.GlobalPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(globalManifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := manifest.WriteFile(globalManifestPath, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "-g", "git:../repo-map"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(globalManifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantSource := source.Git{URL: repoPath}.String()
	if len(doc.Skills) != 1 || doc.Skills[0].Source != wantSource || doc.Skills[0].UpstreamSkill != "repo-map" {
		t.Fatalf("skills = %#v, want source %q with upstream skill repo-map", doc.Skills, wantSource)
	}
}

func TestAddFailsWithoutManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:https://github.com/acme/repo-map.git"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run `ski init` first") {
		t.Fatalf("Execute() error = %v, want init guidance", err)
	}
}

func TestAddRejectsInvalidSource(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "github:acme/repo-map"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "expected git:<url>[@ref]") || !strings.Contains(err.Error(), "bare remote URL") {
		t.Fatalf("Execute() error = %v, want git source error", err)
	}
}

func TestAddRejectsDuplicateDerivedName(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	doc := manifest.Manifest{
		Version: 1,
		Targets: []string{},
		Skills: []manifest.Skill{
			{
				Name:          "repo-map",
				Source:        "git:/tmp/original-repo-map",
				UpstreamSkill: "repo-map",
			},
		},
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `skill name "repo-map" already exists`) {
		t.Fatalf("Execute() error = %v, want duplicate name error", err)
	}
}

func TestAddRejectsDuplicateSource(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	doc := manifest.Manifest{
		Version: 1,
		Targets: []string{},
		Skills: []manifest.Skill{
			{
				Name:          "existing-skill",
				Source:        "git:" + repoPath + "@v1.0.0",
				UpstreamSkill: "repo-map",
			},
		},
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath + "@v1.0.0##repo-map"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), `source "git:`) || !strings.Contains(err.Error(), "already exists as skill") {
		t.Fatalf("Execute() error = %v, want duplicate source error", err)
	}
}

func TestAddRejectsMixedSkillFlagAndLegacySelector(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath + "##repo-map", "--skill", "repo-map"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want selector conflict")
	}
	if !strings.Contains(err.Error(), "--skill cannot be used with legacy source selectors") {
		t.Fatalf("Execute() error = %v, want selector conflict", err)
	}
}

func TestAddRejectsInvalidSkillRepository(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	homeDir := t.TempDir()
	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	repoRoot := t.TempDir()
	repoPath := filepath.Join(repoRoot, "repo-map")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# not a skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	runGit(t, repoRoot, "init", repoPath)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no skills found in repository") {
		t.Fatalf("Execute() error = %v, want invalid skill error", err)
	}
}

func TestAddMultiSkillRepoWithSkillFlags(t *testing.T) {
	t.Parallel()

	repoPath, commit := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath, "--skill", "beta-skill", "--skill", "alpha-skill"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	wantManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "alpha-skill", Source: "git:" + repoPath, UpstreamSkill: "alpha-skill"},
			{Name: "beta-skill", Source: "git:" + repoPath, UpstreamSkill: "beta-skill"},
		},
	}
	if !reflect.DeepEqual(*doc, wantManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, wantManifest)
	}

	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 2 {
		t.Fatalf("lockfile skills = %#v, want two entries", lf.Skills)
	}

	alphaPath := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", commit, "skills", "alpha-skill")
	betaPath := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", commit, "skills", "beta-skill")
	for name, wantTarget := range map[string]string{
		"alpha-skill": alphaPath,
		"beta-skill":  betaPath,
	} {
		linkPath := filepath.Join(projectDir, ".claude", "skills", name)
		targetPath, err := os.Readlink(linkPath)
		if err != nil {
			t.Fatalf("Readlink(%s) error = %v", name, err)
		}
		if targetPath != wantTarget {
			t.Fatalf("symlink target for %s = %q, want %q", name, targetPath, wantTarget)
		}
	}

	if got := stdout.String(); !strings.Contains(got, "added 2 skills") {
		t.Fatalf("stdout = %q, want multi-add confirmation", got)
	}
}

func TestAddSupportsLegacySourceSelectors(t *testing.T) {
	t.Parallel()

	repoPath, _ := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath + "##beta-skill"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "beta-skill", Source: "git:" + repoPath, UpstreamSkill: "beta-skill"},
		},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}
}

func TestAddMultiSkillRepoWithAll(t *testing.T) {
	t.Parallel()

	repoPath, _ := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath, "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if len(doc.Skills) != 2 {
		t.Fatalf("manifest skills = %#v, want two entries", doc.Skills)
	}
}

func TestAddMultiSkillRepoRollsBackOnSecondSkillLinkConflict(t *testing.T) {
	t.Parallel()

	repoPath, _ := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	manifestPath := filepath.Join(projectDir, manifest.FileName)
	originalManifest := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}
	if err := manifest.WriteFile(manifestPath, originalManifest); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	conflictPath := filepath.Join(projectDir, ".claude", "skills")
	if err := os.MkdirAll(conflictPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(conflictPath, "beta-skill"), []byte("conflict"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath, "--skill", "alpha-skill", "--skill", "beta-skill"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want conflict error")
	}
	if !strings.Contains(err.Error(), "beta-skill") {
		t.Fatalf("Execute() error = %v, want beta conflict details", err)
	}

	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if !reflect.DeepEqual(*doc, originalManifest) {
		t.Fatalf("manifest = %#v, want %#v", *doc, originalManifest)
	}

	if _, err := os.Lstat(filepath.Join(projectDir, ".claude", "skills", "alpha-skill")); !os.IsNotExist(err) {
		t.Fatalf("alpha link stat error = %v, want not exist", err)
	}

	if _, err := os.Stat(filepath.Join(projectDir, lockfile.FileName)); !os.IsNotExist(err) {
		t.Fatalf("lockfile stat error = %v, want not exist", err)
	}

	conflictData, err := os.ReadFile(filepath.Join(conflictPath, "beta-skill"))
	if err != nil {
		t.Fatalf("ReadFile(conflict) error = %v", err)
	}
	if string(conflictData) != "conflict" {
		t.Fatalf("conflict file = %q, want original content", string(conflictData))
	}
}

func TestAddMultiSkillRepoRequiresExplicitSelectionInNonTTY(t *testing.T) {
	t.Parallel()

	repoPath, _ := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want explicit-selection guidance")
	}
	if !strings.Contains(err.Error(), "multiple skills found") || !strings.Contains(err.Error(), "--all") || !strings.Contains(err.Error(), "--skill alpha-skill") || !strings.Contains(err.Error(), "--skill beta-skill") {
		t.Fatalf("Execute() error = %v, want multi-skill guidance", err)
	}
}

func TestAddMultiSkillRepoPreservesBareURLInNonTTYGuidance(t *testing.T) {
	t.Parallel()

	repoPath, _ := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Default()); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fileURL := "file://" + repoPath
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", fileURL})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want explicit-selection guidance")
	}
	if !strings.Contains(err.Error(), fileURL) || !strings.Contains(err.Error(), "--skill alpha-skill") || !strings.Contains(err.Error(), "--skill beta-skill") {
		t.Fatalf("Execute() error = %v, want bare-URL guidance", err)
	}
}

func TestAddMultiSkillRepoPromptsForSelectionOnTTY(t *testing.T) {
	t.Parallel()

	repoPath, commit := createMultiSkillRepo(t, "skill-pack", []multiSkillSpec{
		{Path: filepath.Join("skills", "alpha-skill"), Name: "alpha-skill"},
		{Path: filepath.Join("skills", "beta-skill"), Name: "beta-skill"},
	})
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	if err := manifest.WriteFile(filepath.Join(projectDir, manifest.FileName), manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdin:      strings.NewReader("beta-skill\n"),
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return true },
	})
	cmd.SetArgs([]string{"add", "git:" + repoPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(projectDir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []manifest.Skill{
			{Name: "beta-skill", Source: "git:" + repoPath, UpstreamSkill: "beta-skill"},
		},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}

	linkPath := filepath.Join(projectDir, ".claude", "skills", "beta-skill")
	targetPath, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	wantTarget := filepath.Join(homeDir, ".ski", "store", "git", "skill-pack", commit, "skills", "beta-skill")
	if targetPath != wantTarget {
		t.Fatalf("symlink target = %q, want %q", targetPath, wantTarget)
	}
	if !strings.Contains(stdout.String(), "multiple skills found") {
		t.Fatalf("stdout = %q, want prompt output", stdout.String())
	}
}

func createGitRepo(t *testing.T, repoName string, skillName string) (string, string) {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	writeSkillDir(t, repoPath, skillName)

	runGit(t, root, "init", repoPath)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	runGit(t, repoPath, "tag", "v1.0.0")

	commit := runGitOutput(t, repoPath, "rev-parse", "HEAD")
	return repoPath, strings.TrimSpace(commit)
}

type multiSkillSpec struct {
	Path string
	Name string
}

func createMultiSkillRepo(t *testing.T, repoName string, specs []multiSkillSpec) (string, string) {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	for _, spec := range specs {
		writeSkillDir(t, filepath.Join(repoPath, spec.Path), spec.Name)
	}

	runGit(t, root, "init", repoPath)
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
	runGit(t, repoPath, "tag", "v1.0.0")

	commit := runGitOutput(t, repoPath, "rev-parse", "HEAD")
	return repoPath, strings.TrimSpace(commit)
}

func writeSkillDir(t *testing.T, dir string, skillName string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skillDoc := `---
name: ` + skillName + `
description: Builds a repository map. Use when the user asks for codebase structure or repository summaries.
---

# ` + skillName + `
`
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillDoc), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tools", "helper.sh"), []byte("echo helper\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(helper.sh) error = %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, string(output))
	}
	return string(output)
}
