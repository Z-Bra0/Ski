package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	FileName       = "ski.lock.json"
	GlobalFileName = "global.lock.json"
	CurrentVersion = 1
)

type Lockfile struct {
	Version int     `json:"version"`
	Skills  []Skill `json:"skills"`
}

type Skill struct {
	Name          string   `json:"name"`
	Source        string   `json:"source"`
	UpstreamSkill string   `json:"upstream_skill,omitempty"`
	Version       string   `json:"version,omitempty"`
	Commit        string   `json:"commit"`
	Integrity     string   `json:"integrity"`
	Targets       []string `json:"targets,omitempty"`
}

func Default() Lockfile {
	return Lockfile{
		Version: CurrentVersion,
		Skills:  []Skill{},
	}
}

func ReadFile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	if err := lf.Validate(); err != nil {
		return nil, err
	}
	return &lf, nil
}

func WriteFile(path string, lf Lockfile) error {
	if err := lf.Validate(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (lf Lockfile) Validate() error {
	if lf.Version != CurrentVersion {
		return fmt.Errorf("unsupported lockfile version %d", lf.Version)
	}

	names := make(map[string]struct{}, len(lf.Skills))
	for i, skill := range lf.Skills {
		if skill.Name == "" {
			return fmt.Errorf("skill %d: name is required", i)
		}
		if skill.Source == "" {
			return fmt.Errorf("skill %q: source is required", skill.Name)
		}
		if skill.Commit == "" {
			return fmt.Errorf("skill %q: commit is required", skill.Name)
		}
		if skill.Integrity == "" {
			return fmt.Errorf("skill %q: integrity is required", skill.Name)
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
