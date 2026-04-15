package source

import (
	"errors"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/testutil"
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
			name:    "bare https without prefix",
			raw:     "https://github.com/acme/repo-map.git",
			wantURL: "https://github.com/acme/repo-map.git",
		},
		{
			name:       "bare https with ref and selectors",
			raw:        "https://github.com/acme/repo-map.git@v1.0.0##beta-skill,alpha-skill",
			wantURL:    "https://github.com/acme/repo-map.git",
			wantRef:    "v1.0.0",
			wantSkills: []string{"alpha-skill", "beta-skill"},
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
			name:       "url fragment plus selectors",
			raw:        "git:https://example.com/repo#fragment.git##alpha-skill",
			wantURL:    "https://example.com/repo#fragment.git",
			wantSkills: []string{"alpha-skill"},
		},
		{
			name:    "missing prefix",
			raw:     "github:acme/repo-map",
			wantErr: "expected a remote git source",
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
			name:    "reject local filesystem path",
			raw:     "git:/tmp/skill#pack",
			wantErr: "local filesystem git sources are not supported",
		},
		{
			name:    "reject escaped local path",
			raw:     `git:/tmp/example/repo\#\#pack`,
			wantErr: "local filesystem git sources are not supported",
		},
		{
			name:    "reject file url",
			raw:     "file:///tmp/repo-map",
			wantErr: "local filesystem git sources are not supported",
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
		{
			name:    "reject slash in ref",
			raw:     "git:https://github.com/acme/repo-map.git@release/2026-q1",
			wantErr: "refs containing '/' are unsupported",
		},
		{
			name:    "reject ref starting with dash",
			raw:     "git:https://github.com/acme/repo-map.git@-ccore.sshCommand=malicious",
			wantErr: "refs must not start with '-'",
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

func TestParseGitBareRemoteStringCanonicalizesToGitPrefix(t *testing.T) {
	t.Parallel()

	got, err := ParseGit("https://github.com/acme/repo-map.git@v1.0.0##beta-skill,alpha-skill")
	if err != nil {
		t.Fatalf("ParseGit() error = %v", err)
	}

	want := "git:https://github.com/acme/repo-map.git@v1.0.0##alpha-skill,beta-skill"
	if got.String() != want {
		t.Fatalf("String() = %q, want %q", got.String(), want)
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
		URL:    "git@github.com:acme/repo##pack.git",
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
			want:   "github.com-acme-repo-map",
		},
		{
			name:   "scp style url",
			source: Git{URL: "git@github.com:acme/repo-map.git"},
			want:   "github.com-acme-repo-map",
		},
		{
			name:   "single segment remote path keeps legacy key",
			source: Git{URL: "git://127.0.0.1:9418/repo-map"},
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

func TestGitDeriveNameAvoidsRemoteNamespaceCollisions(t *testing.T) {
	t.Parallel()

	a := Git{URL: "https://github.com/org-a/repo-map.git"}
	b := Git{URL: "https://github.com/org-b/repo-map.git"}

	nameA, err := a.DeriveName()
	if err != nil {
		t.Fatalf("DeriveName(a) error = %v", err)
	}
	nameB, err := b.DeriveName()
	if err != nil {
		t.Fatalf("DeriveName(b) error = %v", err)
	}
	if nameA == nameB {
		t.Fatalf("DeriveName() collision: both resolved to %q", nameA)
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

func TestResolveGitReturnsTypedNoMatchingRevisionError(t *testing.T) {
	t.Parallel()

	repo := testutil.NewPlainRepo(t, "repo-map")

	_, err := ResolveGit(repo.Path, Git{
		URL: repo.URL,
		Ref: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	})
	if err == nil {
		t.Fatal("ResolveGit() error = nil, want missing revision error")
	}

	var typedErr NoMatchingRevisionError
	if !IsNoMatchingRevision(err) || !strings.Contains(err.Error(), "no matching revision found") {
		t.Fatalf("ResolveGit() error = %v, want typed no-matching-revision error", err)
	}
	if !strings.Contains(err.Error(), "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef") {
		t.Fatalf("ResolveGit() error = %v, want missing ref in message", err)
	}
	if !errors.As(err, &typedErr) {
		t.Fatalf("ResolveGit() error = %T, want NoMatchingRevisionError", err)
	}
	if typedErr.Ref != "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
		t.Fatalf("NoMatchingRevisionError.Ref = %q, want pinned ref", typedErr.Ref)
	}
}

func TestResolveGitInfoDefaultBranchReportsTrackingAndDate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")

	info, err := ResolveGitInfo(repo.Path, Git{URL: repo.URL})
	if err != nil {
		t.Fatalf("ResolveGitInfo() error = %v", err)
	}

	wantTracking := strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "symbolic-ref", "--short", "HEAD"))
	if info.Tracking != wantTracking {
		t.Fatalf("ResolveGitInfo().Tracking = %q, want %q", info.Tracking, wantTracking)
	}
	if info.Commit != repo.Commit {
		t.Fatalf("ResolveGitInfo().Commit = %q, want %q", info.Commit, repo.Commit)
	}

	wantDate := strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "show", "-s", "--format=%cs", "HEAD"))
	if info.LatestAt != wantDate {
		t.Fatalf("ResolveGitInfo().LatestAt = %q, want %q", info.LatestAt, wantDate)
	}
}

func TestResolveGitInfoDefaultBranchPreservesSlashInTracking(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")
	testutil.RunGit(t, repo.Path, "branch", "-m", "release/2026-q1")

	info, err := ResolveGitInfo(repo.Path, Git{URL: repo.URL})
	if err != nil {
		t.Fatalf("ResolveGitInfo() error = %v", err)
	}

	if info.Tracking != "release/2026-q1" {
		t.Fatalf("ResolveGitInfo().Tracking = %q, want %q", info.Tracking, "release/2026-q1")
	}
}

func TestResolveGitInfoExplicitRefReportsTrackingAndDate(t *testing.T) {
	t.Parallel()

	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")

	tests := []struct {
		name string
		ref  string
	}{
		{
			name: "branch",
			ref:  strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "symbolic-ref", "--short", "HEAD")),
		},
		{
			name: "tag",
			ref:  "v1.0.0",
		},
		{
			name: "commit",
			ref:  repo.Commit,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			info, err := ResolveGitInfo(repo.Path, Git{
				URL: repo.URL,
				Ref: tc.ref,
			})
			if err != nil {
				t.Fatalf("ResolveGitInfo() error = %v", err)
			}
			if info.Tracking != tc.ref {
				t.Fatalf("ResolveGitInfo().Tracking = %q, want %q", info.Tracking, tc.ref)
			}
			if info.Commit == "" {
				t.Fatal("ResolveGitInfo().Commit = empty, want resolved commit")
			}

			wantDate := strings.TrimSpace(testutil.RunGitOutput(t, repo.Path, "show", "-s", "--format=%cs", info.Commit))
			if info.LatestAt != wantDate {
				t.Fatalf("ResolveGitInfo().LatestAt = %q, want %q", info.LatestAt, wantDate)
			}
		})
	}
}

func TestResolveGitInfoCommitRefSkipsResolveGit(t *testing.T) {
	repo := testutil.NewSkillRepo(t, "repo-map", "repo-map")

	originalResolveGit := resolveGit
	resolveGit = func(dir string, spec Git) (string, error) {
		t.Fatal("resolveGit() should not be called for commit refs")
		return "", nil
	}
	t.Cleanup(func() {
		resolveGit = originalResolveGit
	})

	info, err := ResolveGitInfo(repo.Path, Git{URL: repo.URL, Ref: repo.Commit})
	if err != nil {
		t.Fatalf("ResolveGitInfo() error = %v", err)
	}
	if info.Commit != repo.Commit {
		t.Fatalf("ResolveGitInfo().Commit = %q, want %q", info.Commit, repo.Commit)
	}
	if info.Tracking != repo.Commit {
		t.Fatalf("ResolveGitInfo().Tracking = %q, want %q", info.Tracking, repo.Commit)
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
