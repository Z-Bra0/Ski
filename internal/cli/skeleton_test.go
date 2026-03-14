package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpListsSkeletonCommands(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	cmd := NewRootCmd(Options{
		Stdout: &stdout,
		Stderr: &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := stdout.String()
	for _, name := range []string{"install", "remove", "update", "list"} {
		if !strings.Contains(help, name) {
			t.Fatalf("help output missing %q:\n%s", name, help)
		}
	}
}

func TestSkeletonCommandsReturnNotImplemented(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "list", args: []string{"list"}, wantErr: "not implemented: ski list"},
		{name: "remove", args: []string{"remove", "repo-map"}, wantErr: "not implemented: ski remove"},
		{name: "update", args: []string{"update"}, wantErr: "not implemented: ski update"},
		{name: "update check", args: []string{"update", "--check"}, wantErr: "not implemented: ski update --check"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd := NewRootCmd(Options{
				Stdout: &bytes.Buffer{},
				Stderr: &bytes.Buffer{},
			})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
