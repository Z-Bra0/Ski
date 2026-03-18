package source

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
)

const gitPrefix = "git:"
const skillSelectorSeparator = "##"

var commitRefPattern = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
var skillSelectorPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// NoMatchingRevisionError reports that a requested git ref resolved to no revision.
type NoMatchingRevisionError struct {
	Ref string
}

// Error implements the error interface.
func (e NoMatchingRevisionError) Error() string {
	return fmt.Sprintf("resolve %q: no matching revision found", e.Ref)
}

// IsNoMatchingRevision reports whether err is a NoMatchingRevisionError.
func IsNoMatchingRevision(err error) bool {
	var target NoMatchingRevisionError
	return errors.As(err, &target)
}

// Git identifies a git repository plus an optional ref and upstream skill selectors.
type Git struct {
	URL    string
	Ref    string
	Skills []string
}

// ParseGit parses a canonical git: source or a bare remote git URL.
func ParseGit(raw string) (Git, error) {
	raw = strings.TrimSpace(raw)
	spec, ok := normalizeGitInput(raw)
	if !ok {
		return Git{}, fmt.Errorf("unsupported source %q: expected a remote git source like git:https://host/repo.git or https://host/repo.git", raw)
	}
	if spec == "" {
		return Git{}, fmt.Errorf("invalid git source %q: missing url", raw)
	}
	if strings.ContainsAny(spec, " \t\r\n") {
		return Git{}, fmt.Errorf("invalid git source %q: whitespace is not allowed", raw)
	}

	spec, selectors, hasSelector := splitSkillSelectors(spec)
	if hasSelector && selectors == "" {
		return Git{}, fmt.Errorf("invalid git source %q: empty skill selector", raw)
	}

	gitURL, ref := splitGitSpec(spec)
	if gitURL == "" {
		return Git{}, fmt.Errorf("invalid git source %q: missing url", raw)
	}
	if ref == "" && strings.HasSuffix(spec, "@") {
		return Git{}, fmt.Errorf("invalid git source %q: empty ref", raw)
	}

	gitURL = unescapeGitComponent(gitURL)
	ref = unescapeGitComponent(ref)
	if !isSupportedRemoteGitURL(gitURL) {
		return Git{}, fmt.Errorf("unsupported source %q: local filesystem git sources are not supported", raw)
	}

	skills, err := parseSkillSelectors(selectors)
	if err != nil {
		return Git{}, fmt.Errorf("invalid git source %q: %w", raw, err)
	}

	return Git{URL: gitURL, Ref: ref, Skills: skills}, nil
}

func normalizeGitInput(raw string) (string, bool) {
	switch {
	case strings.HasPrefix(raw, "file://"):
		return raw, true
	case isBareRemoteGitURL(raw):
		return raw, true
	case strings.HasPrefix(raw, gitPrefix):
		return strings.TrimSpace(strings.TrimPrefix(raw, gitPrefix)), true
	default:
		return "", false
	}
}

func isBareRemoteGitURL(raw string) bool {
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(raw, prefix) {
			return true
		}
	}
	return false
}

func isSupportedRemoteGitURL(raw string) bool {
	if isBareRemoteGitURL(raw) {
		return true
	}
	if strings.Contains(raw, "://") {
		return false
	}
	hostSep := strings.Index(raw, ":")
	return hostSep >= 0 && !strings.Contains(raw[:hostSep], "/")
}

// String returns the canonical git: representation for the source.
func (g Git) String() string {
	var b strings.Builder
	b.WriteString(gitPrefix)
	b.WriteString(escapeGitComponent(g.URL))
	if g.Ref == "" {
		if len(g.Skills) == 0 {
			return b.String()
		}
		b.WriteString(skillSelectorSeparator)
		b.WriteString(strings.Join(normalizeSkillSelectors(g.Skills), ","))
		return b.String()
	}
	b.WriteString("@")
	b.WriteString(escapeGitComponent(g.Ref))
	if len(g.Skills) > 0 {
		b.WriteString(skillSelectorSeparator)
		b.WriteString(strings.Join(normalizeSkillSelectors(g.Skills), ","))
	}
	return b.String()
}

// DeriveName returns the repository-derived store key for the source.
func (g Git) DeriveName() (string, error) {
	p := strings.TrimSuffix(g.pathForName(), "/")
	if p == "" {
		return "", fmt.Errorf("derive name from %q: missing repository name", g.URL)
	}

	name := path.Base(p)
	name = strings.TrimSuffix(name, ".git")
	if name == "" || name == "." || name == "/" {
		return "", fmt.Errorf("derive name from %q: missing repository name", g.URL)
	}
	return name, nil
}

func splitGitSpec(spec string) (gitURL string, ref string) {
	at := lastUnescapedIndex(spec, "@")
	if at <= 0 || at == len(spec)-1 {
		return spec, ""
	}

	suffix := spec[at+1:]
	if strings.Contains(suffix, "/") {
		return spec, ""
	}

	return spec[:at], suffix
}

func splitSkillSelectors(spec string) (base string, selectors string, hasSelector bool) {
	idx := lastUnescapedIndex(spec, skillSelectorSeparator)
	if idx < 0 {
		return spec, "", false
	}
	return spec[:idx], spec[idx+len(skillSelectorSeparator):], true
}

func (g Git) pathForName() string {
	if strings.Contains(g.URL, "://") {
		if parsed, err := url.Parse(g.URL); err == nil && parsed.Path != "" {
			return parsed.Path
		}
	}

	if hostSep := strings.Index(g.URL, ":"); hostSep >= 0 && !strings.Contains(g.URL[:hostSep], "/") {
		return g.URL[hostSep+1:]
	}

	return g.URL
}

// IsRemote reports whether the source URL refers to a remote git endpoint.
func (g Git) IsRemote() bool {
	return isSupportedRemoteGitURL(g.URL)
}

// ResolveGit resolves the source ref to a concrete commit SHA.
func ResolveGit(dir string, spec Git) (string, error) {
	patterns := []string{"HEAD"}
	if spec.Ref != "" {
		patterns = []string{spec.Ref + "^{}", spec.Ref}
	}

	cmd := exec.Command("git", append([]string{"ls-remote", spec.URL}, patterns...)...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 1 && fields[0] != "" {
			return fields[0], nil
		}
	}

	refLabel := "HEAD"
	if spec.Ref != "" {
		refLabel = spec.Ref
	}
	return "", NoMatchingRevisionError{Ref: refLabel}
}

// IsCommitRef reports whether ref looks like a raw commit SHA.
func IsCommitRef(ref string) bool {
	return commitRefPattern.MatchString(ref)
}

// WithSkills returns a copy of g with normalized upstream skill selectors.
func (g Git) WithSkills(skills []string) Git {
	g.Skills = normalizeSkillSelectors(skills)
	return g
}

// WithoutSkills returns a copy of g with any upstream skill selectors removed.
func (g Git) WithoutSkills() Git {
	g.Skills = nil
	return g
}

func parseSkillSelectors(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	skills := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("empty skill selector")
		}
		if !skillSelectorPattern.MatchString(name) {
			return nil, fmt.Errorf("invalid skill selector %q", name)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate skill selector %q", name)
		}
		seen[name] = struct{}{}
		skills = append(skills, name)
	}
	return normalizeSkillSelectors(skills), nil
}

func normalizeSkillSelectors(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}
	out := append([]string(nil), skills...)
	slices.Sort(out)
	return out
}

func lastUnescapedIndex(s string, sep string) int {
	if len(sep) == 0 || len(s) < len(sep) {
		return -1
	}

	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] != sep {
			continue
		}
		escaped := false
		for j := 0; j < len(sep); j++ {
			if isEscapedAt(s, i+j) {
				escaped = true
				break
			}
		}
		if !escaped {
			return i
		}
	}

	return -1
}

func isEscapedAt(s string, idx int) bool {
	backslashes := 0
	for i := idx - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func escapeGitComponent(raw string) string {
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, ch := range raw {
		switch ch {
		case '\\', '@', '#':
			b.WriteByte('\\')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func unescapeGitComponent(raw string) string {
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			switch raw[i+1] {
			case '\\', '@', '#':
				b.WriteByte(raw[i+1])
				i++
				continue
			}
		}
		b.WriteByte(raw[i])
	}
	return b.String()
}
