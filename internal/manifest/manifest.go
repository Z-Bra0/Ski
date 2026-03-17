package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	FileName       = "ski.toml"
	GlobalFileName = "global.toml"
	CurrentVersion = 1
)

type Manifest struct {
	Version int      `toml:"version"`
	Targets []string `toml:"targets"`
	Skills  []Skill  `toml:"skill,omitempty"`
}

type Skill struct {
	Name    string   `toml:"name"`
	Source  string   `toml:"source"`
	Version string   `toml:"version,omitempty"`
	Targets []string `toml:"targets,omitempty"`
}

func Default() Manifest {
	return Manifest{
		Version: CurrentVersion,
		Targets: []string{},
		Skills:  []Skill{},
	}
}

func ReadFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func WriteFile(path string, doc Manifest) error {
	data, err := Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Parse(data []byte) (*Manifest, error) {
	doc := Default()

	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}

	normalize(&doc)
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}

func Marshal(doc Manifest) ([]byte, error) {
	normalize(&doc)
	if err := doc.Validate(); err != nil {
		return nil, err
	}

	data, err := toml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (doc Manifest) Validate() error {
	if doc.Version != CurrentVersion {
		return fmt.Errorf("unsupported manifest version %d", doc.Version)
	}

	names := make(map[string]struct{}, len(doc.Skills))
	for i, skill := range doc.Skills {
		if skill.Name == "" {
			return fmt.Errorf("skill %d: name is required", i)
		}
		if skill.Source == "" {
			return fmt.Errorf("skill %q: source is required", skill.Name)
		}
		if _, exists := names[skill.Name]; exists {
			return fmt.Errorf("duplicate skill name %q", skill.Name)
		}
		names[skill.Name] = struct{}{}
	}

	return nil
}

func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

func GlobalPath(homeDir string) string {
	return filepath.Join(homeDir, ".ski", GlobalFileName)
}

func normalize(doc *Manifest) {
	if doc.Targets == nil {
		doc.Targets = []string{}
	}
	if doc.Skills == nil {
		doc.Skills = []Skill{}
	}
}
