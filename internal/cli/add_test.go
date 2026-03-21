package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/skill"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/testutil"
	"github.com/spf13/cobra"
)

var repoPathByURL sync.Map

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

func TestAddAcceptsExistingMatchingLinkAndFillsMissingTargets(t *testing.T) {
	t.Parallel()

	repoPath, commit := createGitRepo(t, "repo-map", "repo-map")
	projectDir := t.TempDir()
	homeDir := t.TempDir()

	path := filepath.Join(projectDir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills:  []manifest.Skill{},
	}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", "repo-map", commit)
	existingLink := filepath.Join(projectDir, ".codex", "skills", "repo-map")
	if err := os.MkdirAll(filepath.Dir(existingLink), 0o755); err != nil {
		t.Fatalf("MkdirAll(existing link dir) error = %v", err)
	}
	if err := os.Symlink(storePath, existingLink); err != nil {
		t.Fatalf("Symlink(existing link) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
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
		Targets: []string{"claude", "codex"},
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

	for _, linkPath := range []string{
		filepath.Join(projectDir, ".claude", "skills", "repo-map"),
		filepath.Join(projectDir, ".codex", "skills", "repo-map"),
	} {
		targetPath, err := os.Readlink(linkPath)
		if err != nil {
			t.Fatalf("Readlink(%q) error = %v", linkPath, err)
		}
		if targetPath != storePath {
			t.Fatalf("symlink target for %s = %q, want %q", linkPath, targetPath, storePath)
		}
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

func TestAddWarnsOnStrictSkillSpecMismatchesButSucceeds(t *testing.T) {
	t.Parallel()

	repo := testutil.NewMultiSkillRepo(t, "gstack", []testutil.SkillSpec{
		{Path: ".", Name: "gstack"},
		{Path: filepath.Join("skills", "office-hours"), Name: "office-hours"},
	})
	repoPathByURL.Store(repo.URL, repo.Path)

	rootSkillPath := filepath.Join(repo.Path, "SKILL.md")
	rootSkillDoc := fmt.Sprintf(`---
name: gstack
version: 1.0.0
description: %s
allowed-tools:
  - Bash
  - Read
---

# gstack
`, strings.Repeat("x", 1025))
	if err := os.WriteFile(rootSkillPath, []byte(rootSkillDoc), 0o644); err != nil {
		t.Fatalf("WriteFile(root SKILL.md) error = %v", err)
	}
	testutil.RunGit(t, repo.Path, "add", ".")
	testutil.RunGit(t, repo.Path, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "relax root skill spec")

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
	var stderr bytes.Buffer
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	cmd.SetArgs([]string{"add", repo.URL, "--skill", "office-hours"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := stdout.String(); !strings.Contains(got, "added office-hours") {
		t.Fatalf("stdout = %q, want add confirmation", got)
	}
	lf, err := lockfile.ReadFile(filepath.Join(projectDir, lockfile.FileName))
	if err != nil {
		t.Fatalf("ReadFile(lockfile) error = %v", err)
	}
	if len(lf.Skills) != 1 {
		t.Fatalf("lockfile skills = %#v, want one entry", lf.Skills)
	}
	storeSkillPath := filepath.Join(homeDir, ".ski", "store", "git", "gstack", lf.Skills[0].Commit, "SKILL.md")
	if strings.Contains(stderr.String(), "/checkout/SKILL.md") {
		t.Fatalf("stderr = %q, want warning paths rewritten away from temp checkout", stderr.String())
	}
	for _, want := range []string{
		`warning: strict Agent Skills mismatches found in 1 skill (3 warnings)`,
		`skill "gstack" (` + storeSkillPath + `)`,
		`- unknown frontmatter field "version" is outside the Agent Skills spec`,
		`- description exceeds the Agent Skills spec limit of 1024 characters`,
		`- allowed-tools should use the Agent Skills space-delimited string form`,
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want substring %q", stderr.String(), want)
		}
	}
}

func TestPrintSkillWarningsGroupsAndSortsOutput(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetErr(&stderr)

	printSkillWarnings(cmd, []skill.ValidationWarning{
		{Name: "beta", Path: "/tmp/beta/SKILL.md", Message: `unknown frontmatter field "version" is outside the Agent Skills spec`},
		{Name: "alpha", Path: "/tmp/alpha/SKILL.md", Message: "allowed-tools should use the Agent Skills space-delimited string form"},
		{Name: "alpha", Path: "/tmp/alpha/SKILL.md", Message: `unknown frontmatter field "version" is outside the Agent Skills spec`},
	})

	got := stderr.String()
	for _, want := range []string{
		`warning: strict Agent Skills mismatches found in 2 skills (3 warnings)`,
		`skill "alpha" (/tmp/alpha/SKILL.md)`,
		`- allowed-tools should use the Agent Skills space-delimited string form`,
		`- unknown frontmatter field "version" is outside the Agent Skills spec`,
		"\n\nskill \"beta\" (/tmp/beta/SKILL.md)\n",
		`- unknown frontmatter field "version" is outside the Agent Skills spec`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want substring %q", got, want)
		}
	}
	if strings.Index(got, `skill "alpha" (/tmp/alpha/SKILL.md)`) > strings.Index(got, `skill "beta" (/tmp/beta/SKILL.md)`) {
		t.Fatalf("stderr = %q, want alpha group before beta group", got)
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

func TestAddSupportsBareRemoteURL(t *testing.T) {
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

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", repoPath})

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

func TestAddRejectsLocalFilesystemSource(t *testing.T) {
	t.Parallel()

	repoPath, _ := createGitRepo(t, "repo-map", "repo-map")
	localRepoPath := repoPathForURL(t, repoPath)
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
	cmd.SetArgs([]string{"add", "-g", "git:" + localRepoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want local-source rejection")
	}
	if !strings.Contains(err.Error(), "local filesystem git sources are not supported") {
		t.Fatalf("Execute() error = %v, want local-source rejection", err)
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
	if !strings.Contains(err.Error(), "expected a remote git source") {
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

	repo := testutil.NewPlainRepo(t, "repo-map")

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"add", "git:" + repo.URL})

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

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return false },
	})
	cmd.SetArgs([]string{"add", repoPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want explicit-selection guidance")
	}
	if !strings.Contains(err.Error(), repoPath) || !strings.Contains(err.Error(), "--skill alpha-skill") || !strings.Contains(err.Error(), "--skill beta-skill") {
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
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			if req.Title != "Select skills to add" {
				t.Fatalf("prompt title = %q, want skill selection title", req.Title)
			}
			if !reflect.DeepEqual(req.Options, []string{"alpha-skill", "beta-skill"}) {
				t.Fatalf("prompt options = %#v, want discovered skills", req.Options)
			}
			return []string{"beta-skill"}, nil
		},
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
	if !strings.Contains(stdout.String(), "added beta-skill") {
		t.Fatalf("stdout = %q, want add confirmation", stdout.String())
	}
}

func createGitRepo(t *testing.T, repoName string, skillName string) (string, string) {
	t.Helper()

	repo := testutil.NewSkillRepo(t, repoName, skillName)
	repoPathByURL.Store(repo.URL, repo.Path)
	return repo.URL, repo.Commit
}

type multiSkillSpec struct {
	Path string
	Name string
}

func createMultiSkillRepo(t *testing.T, repoName string, specs []multiSkillSpec) (string, string) {
	t.Helper()

	repoSpecs := make([]testutil.SkillSpec, 0, len(specs))
	for _, spec := range specs {
		repoSpecs = append(repoSpecs, testutil.SkillSpec{Path: spec.Path, Name: spec.Name})
	}
	repo := testutil.NewMultiSkillRepo(t, repoName, repoSpecs)
	repoPathByURL.Store(repo.URL, repo.Path)
	return repo.URL, repo.Commit
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

func repoPathForURL(t *testing.T, repoURL string) string {
	t.Helper()

	path, ok := repoPathByURL.Load(repoURL)
	if !ok {
		t.Fatalf("repo path not found for url %q", repoURL)
	}
	return path.(string)
}
