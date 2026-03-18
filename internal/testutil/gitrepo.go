package testutil

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Repo describes a test git repository and the git:// URL that serves it.
type Repo struct {
	URL    string
	Path   string
	Commit string
}

// SkillSpec identifies one skill directory to create in a repository fixture.
type SkillSpec struct {
	Path string
	Name string
}

// NewSkillRepo creates a single-skill git repository and serves it over git://.
func NewSkillRepo(t testing.TB, repoName, skillName string) Repo {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	WriteSkillDir(t, repoPath, skillName, "Test skill for CLI and service flows.")
	initCommittedRepo(t, root, repoPath)
	RunGit(t, repoPath, "tag", "v1.0.0")

	commit := strings.TrimSpace(RunGitOutput(t, repoPath, "rev-parse", "HEAD"))
	url := startGitDaemon(t, root, repoName)
	return Repo{URL: url, Path: repoPath, Commit: commit}
}

// NewMultiSkillRepo creates a multi-skill git repository and serves it over git://.
func NewMultiSkillRepo(t testing.TB, repoName string, specs []SkillSpec) Repo {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	for _, spec := range specs {
		WriteSkillDir(t, filepath.Join(repoPath, spec.Path), spec.Name, "Test skill for CLI and service flows.")
	}
	initCommittedRepo(t, root, repoPath)
	RunGit(t, repoPath, "tag", "v1.0.0")

	commit := strings.TrimSpace(RunGitOutput(t, repoPath, "rev-parse", "HEAD"))
	url := startGitDaemon(t, root, repoName)
	return Repo{URL: url, Path: repoPath, Commit: commit}
}

// NewPlainRepo creates a non-skill git repository and serves it over git://.
func NewPlainRepo(t testing.TB, repoName string) Repo {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, repoName)
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README.md) error = %v", err)
	}
	initCommittedRepo(t, root, repoPath)

	commit := strings.TrimSpace(RunGitOutput(t, repoPath, "rev-parse", "HEAD"))
	url := startGitDaemon(t, root, repoName)
	return Repo{URL: url, Path: repoPath, Commit: commit}
}

// WriteSkillDir writes a minimal skill directory for test fixtures.
func WriteSkillDir(t testing.TB, dir string, skillName string, description string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Join(dir, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skillDoc := `---
name: ` + skillName + `
description: ` + description + `
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

// RunGit runs a git command in dir and fails the test on error.
func RunGit(t testing.TB, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
}

// RunGitOutput runs a git command in dir and returns stdout.
func RunGitOutput(t testing.TB, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, strings.TrimSpace(string(output)))
	}
	return string(output)
}

func initCommittedRepo(t testing.TB, root, repoPath string) {
	t.Helper()

	RunGit(t, root, "init", repoPath)
	RunGit(t, repoPath, "add", ".")
	RunGit(t, repoPath, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
}

func startGitDaemon(t testing.TB, basePath string, repoName string) string {
	t.Helper()

	port := reservePort(t)
	pidFile := filepath.Join(basePath, fmt.Sprintf(".git-daemon-%d.pid", port))
	cmd := exec.Command(
		"git",
		"daemon",
		"--detach",
		"--reuseaddr",
		"--export-all",
		"--base-path="+basePath,
		"--listen=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--pid-file="+pidFile,
		basePath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start git daemon error = %v\n%s", err, strings.TrimSpace(string(output)))
	}
	t.Cleanup(func() {
		killGitDaemon(pidFile)
	})

	url := fmt.Sprintf("git://127.0.0.1:%d/%s", port, repoName)
	waitForGitDaemon(t, url)
	return url
}

func reservePort(t testing.TB) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port error = %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForGitDaemon(t testing.TB, url string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probe := exec.Command("git", "ls-remote", url, "HEAD")
		if err := probe.Run(); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("git daemon did not become ready for %s", url)
}

func killGitDaemon(pidFile string) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = process.Kill()
	_ = os.Remove(pidFile)
}
