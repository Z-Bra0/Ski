package source

import (
	"strings"
	"testing"
)

func TestParseGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		raw        string
		wantURL    string
		wantRef    string
		wantSkills []string
		wantErr    string
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
				name:       "with skill selectors",
				raw:        "git:https://github.com/acme/repo-map.git@v1.0.0##beta-skill,alpha-skill",
				wantURL:    "https://github.com/acme/repo-map.git",
				wantRef:    "v1.0.0",
				wantSkills: []string{"alpha-skill", "beta-skill"},
			},
			{
				name:    "single hash stays in url path",
				raw:     "git:/tmp/skill#pack",
				wantURL: "/tmp/skill#pack",
			},
		{
			name:       "url fragment plus selectors",
			raw:        "git:https://example.com/repo#fragment.git##alpha-skill",
			wantURL:    "https://example.com/repo#fragment.git",
			wantSkills: []string{"alpha-skill"},
		},
		{
			name:    "escaped double hash stays in local path",
			raw:     `git:/tmp/example/repo\#\#pack`,
			wantURL: "/tmp/example/repo##pack",
		},
		{
			name:    "escaped at sign stays in local path",
			raw:     `git:/tmp/example/repo\@pack`,
			wantURL: "/tmp/example/repo@pack",
		},
		{
			name:    "missing prefix",
			raw:     "github:acme/repo-map",
			wantErr: "expected git:<url>[@ref][##skill[,skill...]]",
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
			{
				name:    "empty selector",
				raw:     "git:https://github.com/acme/repo-map.git##",
				wantErr: "empty skill selector",
			},
			{
				name:    "invalid selector",
				raw:     "git:https://github.com/acme/repo-map.git##bad_name",
				wantErr: "invalid skill selector",
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
			if got.URL != tc.wantURL || got.Ref != tc.wantRef || !equalStrings(got.Skills, tc.wantSkills) {
				t.Fatalf("ParseGit() = %#v, want URL=%q Ref=%q Skills=%#v", got, tc.wantURL, tc.wantRef, tc.wantSkills)
			}
		})
	}
}

func TestGitStringIncludesSortedSelectors(t *testing.T) {
	t.Parallel()

	got := (Git{
		URL:    "https://github.com/acme/repo-map.git",
		Ref:    "v1.0.0",
		Skills: []string{"beta-skill", "alpha-skill"},
	}).String()

	want := "git:https://github.com/acme/repo-map.git@v1.0.0##alpha-skill,beta-skill"
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestGitStringEscapesLiteralSeparators(t *testing.T) {
	t.Parallel()

	got := (Git{
		URL: "git@github.com:acme/repo##pack.git",
		Ref: "release#1",
	}).String()

	want := `git:git\@github.com:acme/repo\#\#pack.git@release\#1`
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestGitRoundTripWithEscapedURLSeparators(t *testing.T) {
	t.Parallel()

	original := Git{
		URL:    "/tmp/example/repo##pack",
		Ref:    "release@2026#1",
		Skills: []string{"beta-skill", "alpha-skill"},
	}

	parsed, err := ParseGit(original.String())
	if err != nil {
		t.Fatalf("ParseGit(String()) error = %v", err)
	}
	if parsed.URL != original.URL || parsed.Ref != original.Ref || !equalStrings(parsed.Skills, []string{"alpha-skill", "beta-skill"}) {
		t.Fatalf("ParseGit(String()) = %#v, want %#v", parsed, original.WithSkills([]string{"alpha-skill", "beta-skill"}))
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

func TestIsCommitRef(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "", want: false},
		{ref: "main", want: false},
		{ref: "v1.0.0", want: false},
		{ref: "abc123", want: false},
		{ref: "abc1234", want: true},
		{ref: "ABC1234DEF", want: true},
		{ref: "a1b2c3d4e5f6a7b8c9d0", want: true},
		{ref: "0123456789012345678901234567890123456789", want: true},
		{ref: "01234567890123456789012345678901234567890", want: false},
		{ref: "abc123z", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()
			if got := IsCommitRef(tc.ref); got != tc.want {
				t.Fatalf("IsCommitRef(%q) = %v, want %v", tc.ref, got, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
