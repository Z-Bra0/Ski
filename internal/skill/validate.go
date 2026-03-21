package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

const FileName = "SKILL.md"

var namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var knownFrontmatterFields = map[string]struct{}{
	"name":          {},
	"description":   {},
	"license":       {},
	"compatibility": {},
	"metadata":      {},
	"allowed-tools": {},
}

// Metadata is the validated YAML frontmatter extracted from SKILL.md.
type Metadata struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  AllowedTools      `yaml:"allowed-tools,omitempty"`
}

// AllowedTools accepts either a single string or a list of strings in frontmatter.
type AllowedTools []string

// ValidationWarning reports a non-fatal Agent Skills spec mismatch.
type ValidationWarning struct {
	Name    string
	Path    string
	Message string
}

// UnmarshalYAML decodes allowed-tools from either a scalar string or a string sequence.
func (a *AllowedTools) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		*a = nil
		return nil
	case yaml.ScalarNode:
		text := strings.TrimSpace(value.Value)
		if text == "" {
			*a = nil
			return nil
		}
		*a = AllowedTools{text}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("allowed-tools entries must be strings")
			}
			text := strings.TrimSpace(item.Value)
			if text == "" {
				return fmt.Errorf("allowed-tools entries must not be empty")
			}
			out = append(out, text)
		}
		*a = AllowedTools(out)
		return nil
	default:
		return fmt.Errorf("allowed-tools must be a string or list of strings")
	}
}

// ValidateDir validates the SKILL.md file for dir and returns its metadata.
func ValidateDir(dir string, expectedName string) (*Metadata, error) {
	meta, _, err := ValidateDirWithWarnings(dir, expectedName)
	return meta, err
}

// ValidateDirWithWarnings validates the SKILL.md file for dir, returning
// compatibility errors plus warnings for strict Agent Skills spec mismatches.
func ValidateDirWithWarnings(dir string, expectedName string) (*Metadata, []ValidationWarning, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("invalid skill: missing %s", path)
		}
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	frontmatter, err := extractFrontmatter(data)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid skill %s: %w", path, err)
	}

	root, err := parseFrontmatterNode(frontmatter)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid skill %s: parse YAML frontmatter: %w", path, err)
	}

	var meta Metadata
	if err := root.Decode(&meta); err != nil {
		return nil, nil, fmt.Errorf("invalid skill %s: parse YAML frontmatter: %w", path, err)
	}

	if err := meta.Validate(expectedName); err != nil {
		return nil, nil, fmt.Errorf("invalid skill %s: %w", path, err)
	}

	return &meta, strictWarnings(meta, path, root), nil
}

// Validate checks the metadata against ski's compatibility requirements.
func (m Metadata) Validate(expectedName string) error {
	if len(m.Name) == 0 {
		return fmt.Errorf("name is required")
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

	if m.Compatibility != "" {
		compatibility := strings.TrimSpace(m.Compatibility)
		if len(compatibility) == 0 {
			return fmt.Errorf("compatibility must not be empty when provided")
		}
	}

	for _, tool := range m.AllowedTools {
		if len(strings.TrimSpace(tool)) == 0 {
			return fmt.Errorf("allowed-tools must not contain empty values")
		}
	}

	return nil
}

// Strings returns the normalized allowed-tools entries.
func (a AllowedTools) Strings() []string {
	return append([]string(nil), []string(a)...)
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

func parseFrontmatterNode(frontmatter []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(frontmatter, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0] == nil {
		return nil, fmt.Errorf("frontmatter is empty")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter must be a YAML mapping")
	}
	return root, nil
}

func strictWarnings(meta Metadata, path string, root *yaml.Node) []ValidationWarning {
	warnings := make([]ValidationWarning, 0)
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		value := root.Content[i+1]
		if _, ok := knownFrontmatterFields[key]; !ok {
			warnings = append(warnings, ValidationWarning{
				Name:    meta.Name,
				Path:    path,
				Message: fmt.Sprintf("unknown frontmatter field %q is outside the Agent Skills spec", key),
			})
			continue
		}
		if key == "allowed-tools" && value.Kind == yaml.SequenceNode {
			warnings = append(warnings, ValidationWarning{
				Name:    meta.Name,
				Path:    path,
				Message: "allowed-tools should use the Agent Skills space-delimited string form",
			})
		}
	}

	if len(meta.Name) > 64 {
		warnings = append(warnings, ValidationWarning{
			Name:    meta.Name,
			Path:    path,
			Message: "name exceeds the Agent Skills spec limit of 64 characters",
		})
	}
	if len(strings.TrimSpace(meta.Description)) > 1024 {
		warnings = append(warnings, ValidationWarning{
			Name:    meta.Name,
			Path:    path,
			Message: "description exceeds the Agent Skills spec limit of 1024 characters",
		})
	}
	if compatibility := strings.TrimSpace(meta.Compatibility); compatibility != "" && len(compatibility) > 500 {
		warnings = append(warnings, ValidationWarning{
			Name:    meta.Name,
			Path:    path,
			Message: "compatibility exceeds the Agent Skills spec limit of 500 characters",
		})
	}

	return warnings
}
