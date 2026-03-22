package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/target"
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
			wantOptions := append([]string{"all"}, target.BuiltInNames()...)
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
		Targets: target.BuiltInNames(),
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

func TestInitRejectsInvalidTargetSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  string
		wantErr string
	}{
		{
			name:    "missing custom dir path",
			target:  "dir:",
			wantErr: `custom target "dir:": missing directory path`,
		},
		{
			name:    "escape outside project",
			target:  "dir:../escape",
			wantErr: `target "dir:../escape" must resolve to a subdirectory within the project root`,
		},
		{
			name:    "absolute path",
			target:  "dir:/abs/path",
			wantErr: `custom target "dir:/abs/path" must be project-relative`,
		},
		{
			name:    "home expansion outside global scope",
			target:  "dir:~/skills",
			wantErr: `custom target "dir:~/skills" may use ~ only in global scope`,
		},
		{
			name:    "unsupported target",
			target:  "unsupported-target",
			wantErr: `unsupported target "unsupported-target"`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cmd := NewRootCmd(Options{
				Getwd:  func() (string, error) { return dir, nil },
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			})
			cmd.SetArgs([]string{"init", "--target", tc.target})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, tc.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(dir, manifest.FileName)); !os.IsNotExist(statErr) {
				t.Fatalf("manifest stat error = %v, want not exist", statErr)
			}
		})
	}
}

func TestInitFailsWhenWorkingDirectoryResolutionFails(t *testing.T) {
	t.Parallel()

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return "", errBoom("cwd") },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want cwd error")
	}
	if !strings.Contains(err.Error(), "resolve working directory: cwd") {
		t.Fatalf("Execute() error = %v, want cwd resolution error", err)
	}
}

func TestInitFailsWhenHomeDirectoryResolutionFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := NewRootCmd(Options{
		Getwd:      func() (string, error) { return dir, nil },
		GetHomeDir: func() (string, error) { return "", errBoom("home") },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want home error")
	}
	if !strings.Contains(err.Error(), "resolve home directory: home") {
		t.Fatalf("Execute() error = %v, want home resolution error", err)
	}
}

func TestInitFailsWhenManifestStatHitsUnexpectedError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	baseFile := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(baseFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(baseFile) error = %v", err)
	}

	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return filepath.Join(baseFile, "child"), nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want stat error")
	}
	if !strings.Contains(err.Error(), "stat ") {
		t.Fatalf("Execute() error = %v, want stat error", err)
	}
}

func TestInitDoesNotCreateManifestWhenPromptFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cmd := NewRootCmd(Options{
		Getwd:  func() (string, error) { return dir, nil },
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		IsTTY:  func() bool { return true },
		PromptMultiSelect: func(req MultiSelectRequest) ([]string, error) {
			return nil, errBoom("prompt canceled")
		},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want prompt error")
	}
	if !strings.Contains(err.Error(), "prompt canceled") {
		t.Fatalf("Execute() error = %v, want prompt error", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, manifest.FileName)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest stat error = %v, want not exist", statErr)
	}
}

func errBoom(label string) error {
	return errors.New(label)
}
