package target

import (
	"fmt"
	"os"
	"path/filepath"
)

var dirs = map[string]string{
	"claude":   filepath.Join(".claude", "skills"),
	"codex":    filepath.Join(".codex", "skills"),
	"cursor":   filepath.Join(".cursor", "skills"),
	"openclaw": filepath.Join(".openclaw", "skills"),
}

func LinkAll(projectRoot string, targets []string, name, storePath string) error {
	for _, target := range targets {
		if err := Link(projectRoot, target, name, storePath); err != nil {
			return err
		}
	}
	return nil
}

func Link(projectRoot, target, name, storePath string) error {
	dir, err := SkillDir(projectRoot, target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	linkPath := filepath.Join(dir, name)
	info, err := os.Lstat(linkPath)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s already exists and is not a symlink", linkPath)
		}
		current, err := os.Readlink(linkPath)
		if err != nil {
			return fmt.Errorf("readlink %s: %w", linkPath, err)
		}
		if current == storePath {
			return nil
		}
		return fmt.Errorf("%s already links to %s", linkPath, current)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", linkPath, err)
	}

	if err := os.Symlink(storePath, linkPath); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", linkPath, storePath, err)
	}
	return nil
}

func SkillDir(projectRoot, target string) (string, error) {
	rel, ok := dirs[target]
	if !ok {
		return "", fmt.Errorf("unsupported target %q", target)
	}
	return filepath.Join(projectRoot, rel), nil
}
