package source

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

const gitPrefix = "git:"

type Git struct {
	URL string
	Ref string
}

func ParseGit(raw string) (Git, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, gitPrefix) {
		return Git{}, fmt.Errorf("unsupported source %q: expected git:<url>[@ref]", raw)
	}

	spec := strings.TrimSpace(strings.TrimPrefix(raw, gitPrefix))
	if spec == "" {
		return Git{}, fmt.Errorf("invalid git source %q: missing url", raw)
	}
	if strings.ContainsAny(spec, " \t\r\n") {
		return Git{}, fmt.Errorf("invalid git source %q: whitespace is not allowed", raw)
	}

	gitURL, ref := splitGitSpec(spec)
	if gitURL == "" {
		return Git{}, fmt.Errorf("invalid git source %q: missing url", raw)
	}
	if ref == "" && strings.HasSuffix(spec, "@") {
		return Git{}, fmt.Errorf("invalid git source %q: empty ref", raw)
	}

	return Git{URL: gitURL, Ref: ref}, nil
}

func (g Git) String() string {
	if g.Ref == "" {
		return gitPrefix + g.URL
	}
	return gitPrefix + g.URL + "@" + g.Ref
}

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
	at := strings.LastIndex(spec, "@")
	if at <= 0 || at == len(spec)-1 {
		return spec, ""
	}

	suffix := spec[at+1:]
	if strings.Contains(suffix, "/") {
		return spec, ""
	}

	return spec[:at], suffix
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
