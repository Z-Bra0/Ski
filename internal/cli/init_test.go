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
