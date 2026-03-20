package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
)

func TestInitCreatesManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &stdout,
		Stderr: &stderr,
	})
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	path := filepath.Join(dir, manifest.FileName)
	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	if got, want := *doc, manifest.Default(); !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %#v, want %#v", got, want)
	}

	if got := stdout.String(); !strings.Contains(got, path) {
		t.Fatalf("stdout = %q, want path %q", got, path)
	}
}

func TestInitFailsWhenManifestExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want already exists error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Execute() error = %v, want already exists", err)
	}
}

func TestInitDoesNotPromptWhenManifestExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, manifest.FileName)
	if err := manifest.WriteFile(path, manifest.Default()); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		IsTTY:  func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			t.Fatal("PromptMultiSelect should not be called when manifest already exists")
			return nil, nil
		},
		PromptInput: func(req TextPromptRequest) (string, error) {
			t.Fatal("PromptInput should not be called when manifest already exists")
			return "", nil
		},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want already exists error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Execute() error = %v, want already exists", err)
	}
}

func TestInitCreatesGlobalManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init", "-g"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	path := manifest.GlobalPath(homeDir)
	doc, err := manifest.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if got, want := *doc, manifest.Default(); !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest = %#v, want %#v", got, want)
	}
}

func TestInitPromptsForTargetsOnTTY(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var stdout bytes.Buffer

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
		IsTTY:  func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			if req.Title != "Select targets" {
				t.Fatalf("prompt title = %q, want target selection title", req.Title)
			}
			if req.Height != 10 {
				t.Fatalf("prompt height = %d, want 10", req.Height)
			}
			if !strings.Contains(req.Description, "space to toggle multiple items") {
				t.Fatalf("prompt description = %q, want multi-select hint", req.Description)
			}
			wantOptions := []string{"all", "claude", "codex", "cursor", "openclaw"}
			if !reflect.DeepEqual(req.Options, wantOptions) {
				t.Fatalf("prompt options = %#v, want %#v", req.Options, wantOptions)
			}
			return []string{"claude", "codex"}, nil
		},
	})
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(dir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills:  []manifest.Skill{},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}
}

func TestInitPromptAllExpandsBuiltInTargets(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		IsTTY:  func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			return []string{"all"}, nil
		},
	})
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(dir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "codex", "cursor", "openclaw"},
		Skills:  []manifest.Skill{},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}
}

func TestInitWithTargetFlagsSkipsPrompt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		IsTTY:  func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			t.Fatal("PromptMultiSelect should not be called when --target is provided")
			return nil, nil
		},
	})
	cmd.SetArgs([]string{"init", "--target", "claude", "--target", "dir:./agent-skills/claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(filepath.Join(dir, manifest.FileName))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "dir:./agent-skills/claude"},
		Skills:  []manifest.Skill{},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}
}

func TestInitRejectsTargetsThatResolveToSameDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init", "--target", "claude", "--target", "dir:./.claude/skills"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want duplicate-directory error")
	}
	if !strings.Contains(err.Error(), `targets "claude" and "dir:./.claude/skills" resolve to the same directory`) {
		t.Fatalf("Execute() error = %v, want duplicate-directory error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, manifest.FileName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat error = %v, want not exist", statErr)
	}
}

func TestInitGlobalRejectsTargetsThatResolveToSameDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init", "-g", "--target", "claude", "--target", "dir:~/.claude/skills"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want duplicate-directory error")
	}
	if !strings.Contains(err.Error(), `targets "claude" and "dir:~/.claude/skills" resolve to the same directory`) {
		t.Fatalf("Execute() error = %v, want duplicate-directory error", err)
	}
	if _, statErr := os.Stat(manifest.GlobalPath(homeDir)); !os.IsNotExist(statErr) {
		t.Fatalf("global manifest stat error = %v, want not exist", statErr)
	}
}
