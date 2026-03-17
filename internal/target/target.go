package target

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var dirs = map[string]string{
	"claude":   filepath.Join(".claude", "skills"),
	"codex":    filepath.Join(".codex", "skills"),
	"cursor":   filepath.Join(".cursor", "skills"),
	"openclaw": filepath.Join(".openclaw", "skills"),
}

const customDirPrefix = "dir:"

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

func UnlinkAll(projectRoot string, targets []string, name string) error {
	for _, t := range targets {
		if err := Unlink(projectRoot, t, name); err != nil {
			return err
		}
	}
	return nil
}

func Unlink(projectRoot, target, name string) error {
	dir, err := SkillDir(projectRoot, target)
	if err != nil {
		return err
	}
	linkPath := filepath.Join(dir, name)
	info, err := os.Lstat(linkPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lstat %s: %w", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink; remove it manually", linkPath)
	}
	if err := os.Remove(linkPath); err != nil {
		return fmt.Errorf("remove %s: %w", linkPath, err)
	}
	return nil
}

func SkillDir(projectRoot, target string) (string, error) {
	rel, ok := dirs[target]
	if ok {
		return filepath.Join(projectRoot, rel), nil
	}

	rel, err := customSkillDir(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(projectRoot, rel), nil
}

func customSkillDir(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("target is required")
	}
	if !strings.HasPrefix(target, customDirPrefix) {
		return "", fmt.Errorf("unsupported target %q", target)
	}
	path := strings.TrimSpace(strings.TrimPrefix(target, customDirPrefix))
	if path == "" {
		return "", fmt.Errorf("custom target %q: missing directory path", target)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("custom target %q must be project-relative", target)
	}

	clean := filepath.Clean(path)
	parentPrefix := ".." + string(filepath.Separator)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, parentPrefix) {
		return "", fmt.Errorf("custom target %q must resolve to a subdirectory within the project root", target)
	}

	return clean, nil
}
