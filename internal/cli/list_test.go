package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
)

func TestListGlobalShowsHomeScopedSkills(t *testing.T) {
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
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	addCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	addCmd.SetArgs([]string{"add", "-g", "git:" + repoPath + "@v1.0.0"})
	if err := addCmd.Execute(); err != nil {
		t.Fatalf("add Execute() error = %v", err)
	}

	var stdout bytes.Buffer
	listCmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return projectDir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &stdout,
		Stderr:     &bytes.Buffer{},
	})
	listCmd.SetArgs([]string{"list", "-g"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list Execute() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "#") {
		t.Fatalf("stdout = %q, want index column", out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "SOURCE") || !strings.Contains(out, "UPSTREAM") || !strings.Contains(out, "COMMIT") || !strings.Contains(out, "TARGETS") {
		t.Fatalf("stdout = %q, want table header", out)
	}
	if !strings.Contains(out, "1  repo-map") && !strings.Contains(out, "1\trepo-map") {
		t.Fatalf("stdout = %q, want numbered repo-map row", out)
	}
	if !strings.Contains(out, "repo-map") {
		t.Fatalf("stdout = %q, want repo-map row", out)
	}
	if !strings.Contains(out, "git:"+repoPath+"@v1.0.0") {
		t.Fatalf("stdout = %q, want canonical source", out)
	}
	if !strings.Contains(out, commit[:7]) {
		t.Fatalf("stdout = %q, want commit %q", out, commit[:7])
	}
	if !strings.Contains(out, "claude") {
		t.Fatalf("stdout = %q, want target column", out)
	}
}
