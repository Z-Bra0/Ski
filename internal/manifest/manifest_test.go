package manifest

import (
	"reflect"
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestMarshalParseRoundTrip(t *testing.T) {
	t.Parallel()

	original := Manifest{
		Version: 1,
		Targets: []string{"claude", "codex"},
		Skills: []Skill{
			{
				Name:          "repo-map",
				Source:        "github:acme/repo-map@v1.0.0",
				UpstreamSkill: "repo-map",
				Version:       "0.3.1",
			},
			{
				Name:          "audit-solidity",
				Source:        "git:https://github.com/org/audit-solidity.git",
				UpstreamSkill: "audit-solidity",
				Enabled:       boolPtr(false),
				Targets:       []string{"claude"},
			},
		},
	}

	data, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !reflect.DeepEqual(*parsed, original) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", *parsed, original)
	}
}

func TestMarshalDefaultManifest(t *testing.T) {
	t.Parallel()

	data, err := Marshal(Default())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "version = 1") {
		t.Fatalf("Marshal() = %q, want version", got)
	}
	if !strings.Contains(got, "targets = []") {
		t.Fatalf("Marshal() = %q, want empty targets", got)
	}
	if strings.Contains(got, "skill = []") {
		t.Fatalf("Marshal() = %q, should not emit empty skill array", got)
	}
}

func TestParseRealTOMLFeatures(t *testing.T) {
	t.Parallel()

	data := []byte(`
version = 1
targets = [
  "codex",
] # trailing comma is valid TOML

[[skill]]
name = 'repo "#1" map'
source = 'git:https://example.com/repo#fragment.git'
upstream_skill = 'repo-fragment'
`)

	doc, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	want := Manifest{
		Version: 1,
		Targets: []string{"codex"},
		Skills: []Skill{
			{
				Name:          `repo "#1" map`,
				Source:        "git:https://example.com/repo#fragment.git",
				UpstreamSkill: "repo-fragment",
			},
		},
	}

	if !reflect.DeepEqual(*doc, want) {
		t.Fatalf("Parse() = %#v, want %#v", *doc, want)
	}
}

func TestParseRejectsInvalidManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name: "unsupported version",
			data: `
version = 2
`,
			wantErr: "unsupported manifest version 2",
		},
		{
			name: "unknown root key",
			data: `
version = 1
unknown = "value"
`,
			wantErr: "strict mode",
		},
		{
			name: "missing skill name",
			data: `
version = 1

[[skill]]
source = "github:acme/repo-map"
`,
			wantErr: "skill 0: name is required",
		},
		{
			name: "missing skill source",
			data: `
version = 1

[[skill]]
name = "repo-map"
`,
			wantErr: `skill "repo-map": source is required`,
		},
		{
			name: "duplicate skill names",
			data: `
version = 1

[[skill]]
name = "repo-map"
source = "github:acme/repo-map"

[[skill]]
name = "repo-map"
source = "github:acme/other-repo"
`,
			wantErr: `duplicate skill name "repo-map"`,
		},
		{
			name: "unknown skill key",
			data: `
version = 1

[[skill]]
name = "repo-map"
source = "github:acme/repo-map"
unknown = "value"
`,
			wantErr: "strict mode",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(tc.data))
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("Parse() error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestMarshalParseDisabledSkill(t *testing.T) {
	t.Parallel()

	doc := Manifest{
		Version: 1,
		Targets: []string{"claude"},
		Skills: []Skill{{
			Name:    "repo-map",
			Source:  "git:https://github.com/org/repo-map.git",
			Enabled: boolPtr(false),
		}},
	}

	data, err := Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "enabled = false") {
		t.Fatalf("Marshal() = %q, want enabled = false", string(data))
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if parsed.Skills[0].Enabled == nil || *parsed.Skills[0].Enabled {
		t.Fatalf("Parse() enabled = %#v, want disabled", parsed.Skills[0].Enabled)
	}
}
