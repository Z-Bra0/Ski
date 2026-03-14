package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

const FileName = "SKILL.md"

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type Metadata struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	License      string            `yaml:"license,omitempty"`
	Compatibility string           `yaml:"compatibility,omitempty"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
	AllowedTools string            `yaml:"allowed-tools,omitempty"`
}

func ValidateDir(dir string, expectedName string) (*Metadata, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("invalid skill: missing %s", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	frontmatter, err := extractFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("invalid skill %s: %w", path, err)
	}

	var meta Metadata
	dec := yaml.NewDecoder(bytes.NewReader(frontmatter))
	dec.KnownFields(true)
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("invalid skill %s: parse YAML frontmatter: %w", path, err)
	}

	if err := meta.Validate(expectedName); err != nil {
		return nil, fmt.Errorf("invalid skill %s: %w", path, err)
	}

	return &meta, nil
}

func (m Metadata) Validate(expectedName string) error {
	if len(m.Name) == 0 {
		return fmt.Errorf("name is required")
	}
	if len(m.Name) > 64 {
		return fmt.Errorf("name must be 1-64 characters")
	}
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("name must use lowercase letters, numbers, and single hyphens only")
	}
	if expectedName != "" && m.Name != expectedName {
		return fmt.Errorf("name %q must match installed directory name %q", m.Name, expectedName)
	}

	description := strings.TrimSpace(m.Description)
	if len(description) == 0 {
		return fmt.Errorf("description is required")
	}
	if len(description) > 1024 {
		return fmt.Errorf("description must be 1-1024 characters")
	}

	if m.Compatibility != "" {
		compatibility := strings.TrimSpace(m.Compatibility)
		if len(compatibility) == 0 || len(compatibility) > 500 {
			return fmt.Errorf("compatibility must be 1-500 characters when provided")
		}
	}

	if m.AllowedTools != "" && len(strings.TrimSpace(m.AllowedTools)) == 0 {
		return fmt.Errorf("allowed-tools must not be empty when provided")
	}

	return nil
}

func extractFrontmatter(data []byte) ([]byte, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}

	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
		} else {
			return nil, fmt.Errorf("missing closing frontmatter delimiter")
		}
	}

	frontmatter := rest[:end]
	if strings.TrimSpace(frontmatter) == "" {
		return nil, fmt.Errorf("frontmatter is empty")
	}
	return []byte(frontmatter), nil
}
