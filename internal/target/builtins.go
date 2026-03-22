package target

import (
	"path/filepath"
	"strings"
)

type Builtin struct {
	Name       string
	ProjectDir string
	GlobalDir  string
}

type builtinSpec struct {
	Name       string
	ProjectDir string
	GlobalDir  string
}

var builtinSpecs = []builtinSpec{
	{Name: "claude", ProjectDir: ".claude/skills", GlobalDir: ".claude/skills"},
	{Name: "codex", ProjectDir: ".codex/skills", GlobalDir: ".codex/skills"},
	{Name: "cursor", ProjectDir: ".cursor/skills", GlobalDir: ".cursor/skills"},
	{Name: "openclaw", ProjectDir: ".openclaw/skills", GlobalDir: ".openclaw/skills"},
	{Name: "opencode", ProjectDir: ".opencode/skills", GlobalDir: ".config/opencode/skills"},
	{Name: "goose", ProjectDir: ".goose/skills", GlobalDir: ".config/goose/skills"},
	{Name: "agents", ProjectDir: ".agents/skills", GlobalDir: ".config/agents/skills"},
}

var builtins = makeBuiltins(builtinSpecs)

var builtinsByName = func() map[string]Builtin {
	out := make(map[string]Builtin, len(builtins))
	for _, builtin := range builtins {
		out[builtin.Name] = builtin
	}
	return out
}()

func makeBuiltins(specs []builtinSpec) []Builtin {
	out := make([]Builtin, 0, len(specs))
	for _, spec := range specs {
		out = append(out, Builtin{
			Name:       spec.Name,
			ProjectDir: normalizePathSpec(spec.ProjectDir),
			GlobalDir:  normalizePathSpec(spec.GlobalDir),
		})
	}
	return out
}

func normalizePathSpec(path string) string {
	parts := strings.Split(path, "/")
	return filepath.Join(parts...)
}
