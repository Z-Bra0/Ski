package cli

import (
	"bytes"
	"strings"
	"testing"

	"ski/internal/buildinfo"
)

func TestVersionPrintsBuildVersion(t *testing.T) {
	t.Parallel()

	original := buildinfo.Version
	buildinfo.Version = "0.1.0-test"
	t.Cleanup(func() {
		buildinfo.Version = original
	})

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := strings.TrimSpace(stdout.String()); got != "0.1.0-test" {
		t.Fatalf("stdout = %q, want 0.1.0-test", got)
	}
}
