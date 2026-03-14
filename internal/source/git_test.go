package source

import (
	"strings"
	"testing"
)

func TestParseGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantURL string
		wantRef string
		wantErr string
	}{
		{
			name:    "https with ref",
			raw:     "git:https://github.com/acme/repo-map.git@v1.0.0",
			wantURL: "https://github.com/acme/repo-map.git",
			wantRef: "v1.0.0",
		},
		{
			name:    "https without ref",
			raw:     "git:https://github.com/acme/repo-map.git",
			wantURL: "https://github.com/acme/repo-map.git",
		},
		{
			name:    "scp style",
			raw:     "git:git@github.com:acme/repo-map.git@a1b2c3d",
			wantURL: "git@github.com:acme/repo-map.git",
			wantRef: "a1b2c3d",
		},
		{
			name:    "ssh scheme userinfo without ref",
			raw:     "git:ssh://git@github.com/acme/repo-map.git",
			wantURL: "ssh://git@github.com/acme/repo-map.git",
		},
		{
			name:    "missing prefix",
			raw:     "github:acme/repo-map",
			wantErr: "expected git:<url>[@ref]",
		},
		{
			name:    "missing url",
			raw:     "git:",
			wantErr: "missing url",
		},
		{
			name:    "empty ref",
			raw:     "git:https://github.com/acme/repo-map.git@",
			wantErr: "empty ref",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseGit(tc.raw)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("ParseGit() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseGit() error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGit() error = %v", err)
			}
			if got.URL != tc.wantURL || got.Ref != tc.wantRef {
				t.Fatalf("ParseGit() = %#v, want URL=%q Ref=%q", got, tc.wantURL, tc.wantRef)
			}
		})
	}
}

func TestGitDeriveName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  Git
		want    string
		wantErr string
	}{
		{
			name:   "https url",
			source: Git{URL: "https://github.com/acme/repo-map.git"},
			want:   "repo-map",
		},
		{
			name:   "scp style url",
			source: Git{URL: "git@github.com:acme/repo-map.git"},
			want:   "repo-map",
		},
		{
			name:   "local path",
			source: Git{URL: "../skills/repo-map"},
			want:   "repo-map",
		},
		{
			name:    "missing segment",
			source:  Git{URL: "https://github.com/"},
			wantErr: "missing repository name",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.source.DeriveName()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("DeriveName() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("DeriveName() error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DeriveName() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("DeriveName() = %q, want %q", got, tc.want)
			}
		})
	}
}
