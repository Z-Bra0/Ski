package cli

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ski/internal/manifest"
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
			return []string{"claude", "codex"}, nil
		},
		PromptInput: func(req TextPromptRequest) (string, error) {
			if req.Title != "Custom target directories" {
				t.Fatalf("prompt title = %q, want custom target title", req.Title)
			}
			return "./agent-skills/claude", nil
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
		Targets: []string{"claude", "codex", "dir:./agent-skills/claude"},
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
		PromptInput: func(req TextPromptRequest) (string, error) {
			t.Fatal("PromptInput should not be called when --target is provided")
			return "", nil
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

func TestInitGlobalPromptSupportsCustomHomeRelativeDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	homeDir := t.TempDir()

	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return homeDir, nil },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTTY:      func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			return []string{"claude"}, nil
		},
		PromptInput: func(req TextPromptRequest) (string, error) {
			return "~/agent-skills/claude", nil
		},
	})
	cmd.SetArgs([]string{"init", "-g"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	doc, err := manifest.ReadFile(manifest.GlobalPath(homeDir))
	if err != nil {
		t.Fatalf("ReadFile(global manifest) error = %v", err)
	}
	want := manifest.Manifest{
		Version: 1,
		Targets: []string{"claude", "dir:~/agent-skills/claude"},
		Skills:  []manifest.Skill{},
	}
	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("manifest = %#v, want %#v", *doc, want)
	}
}
