package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	original := validLockfile()

	if err := WriteFile(path, original); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	parsed, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !reflect.DeepEqual(*parsed, original) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", *parsed, original)
	}
}

func TestWriteFileRejectsInvalidLockfile(t *testing.T) {
	t.Parallel()

	for _, tc := range invalidLockfileCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			lf := validLockfile()
			tc.mutate(&lf)

			err := WriteFile(path, lf)
			if err == nil {
				t.Fatal("WriteFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("WriteFile() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestReadFileRejectsInvalidLockfile(t *testing.T) {
	t.Parallel()

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, FileName)
		payload := []byte(`{
  "version": 1,
  "skills":
}`)
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}

		_, err := ReadFile(path)
		if err == nil {
			t.Fatal("ReadFile() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("ReadFile() error = %q, want substring %q", err.Error(), "invalid character")
		}
	})

	for _, tc := range invalidLockfileCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			payload := mustMarshalLockfile(t, tc.mutate)
			if err := os.WriteFile(path, payload, 0o644); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", path, err)
			}

			_, err := ReadFile(path)
			if err == nil {
				t.Fatal("ReadFile() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ReadFile() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

type invalidLockfileCase struct {
	name    string
	mutate  func(*Lockfile)
	wantErr string
}

func invalidLockfileCases() []invalidLockfileCase {
	return []invalidLockfileCase{
		{
			name: "unsupported version",
			mutate: func(lf *Lockfile) {
				lf.Version = 2
			},
			wantErr: "unsupported lockfile version 2",
		},
		{
			name: "missing commit",
			mutate: func(lf *Lockfile) {
				lf.Skills[0].Commit = ""
			},
			wantErr: `skill "repo-map": commit is required`,
		},
		{
			name: "missing integrity",
			mutate: func(lf *Lockfile) {
				lf.Skills[0].Integrity = ""
			},
			wantErr: `skill "repo-map": integrity is required`,
		},
		{
			name: "duplicate skill names",
			mutate: func(lf *Lockfile) {
				lf.Skills = append(lf.Skills, Skill{
					Name:      "repo-map",
					Source:    "github:acme/other-repo",
					Commit:    "deadbeef",
					Integrity: "sha256:123456",
				})
			},
			wantErr: `duplicate skill name "repo-map"`,
		},
	}
}

func validLockfile() Lockfile {
	return Lockfile{
		Version: 1,
		Skills: []Skill{
			{
				Name:      "repo-map",
				Source:    "github:acme/repo-map@v1.0.0",
				Version:   "0.3.1",
				Commit:    "a1b2c3d4",
				Integrity: "sha256:abcdef",
				Targets:   []string{"codex"},
			},
		},
	}
}

func mustMarshalLockfile(t *testing.T, mutate func(*Lockfile)) []byte {
	t.Helper()

	lf := validLockfile()
	mutate(&lf)

	data, err := json.Marshal(lf)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
