package target

import (
	"errors"
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
	return linkAll(projectRoot, false, targets, name, storePath)
}

func LinkAllGlobal(homeDir string, targets []string, name, storePath string) error {
	return linkAll(homeDir, true, targets, name, storePath)
}

func linkAll(baseDir string, global bool, targets []string, name, storePath string) error {
	dirs, err := resolveTargetDirs(baseDir, global, targets)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := linkDir(dir, name, storePath); err != nil {
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
	return linkDir(dir, name, storePath)
}

func LinkGlobal(homeDir, target, name, storePath string) error {
	dir, err := GlobalSkillDir(homeDir, target)
	if err != nil {
		return err
	}
	return linkDir(dir, name, storePath)
}

func linkDir(dir, name, storePath string) error {
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
	return unlinkAll(projectRoot, false, targets, name)
}

func UnlinkAllGlobal(homeDir string, targets []string, name string) error {
	return unlinkAll(homeDir, true, targets, name)
}

func unlinkAll(baseDir string, global bool, targets []string, name string) error {
	dirs, err := resolveTargetDirs(baseDir, global, targets)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := unlinkDir(dir, name); err != nil {
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
	return unlinkDir(dir, name)
}

func UnlinkGlobal(homeDir, target, name string) error {
	dir, err := GlobalSkillDir(homeDir, target)
	if err != nil {
		return err
	}
	return unlinkDir(dir, name)
}

func unlinkDir(dir, name string) error {
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
	return skillDir(projectRoot, false, target)
}

func GlobalSkillDir(homeDir, target string) (string, error) {
	return skillDir(homeDir, true, target)
}

func skillDir(baseDir string, global bool, target string) (string, error) {
	rel, ok := dirs[target]
	if ok {
		return resolveScopedDir(baseDir, filepath.Join(baseDir, rel), target)
	}

	return customSkillDir(baseDir, global, target)
}

func customSkillDir(baseDir string, global bool, target string) (string, error) {
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

	if strings.HasPrefix(path, "~") {
		if !global {
			return "", fmt.Errorf("custom target %q may use ~ only in global scope", target)
		}
		path = expandHome(path, baseDir)
	} else {
		if filepath.IsAbs(path) {
			return "", fmt.Errorf("custom target %q must be project-relative", target)
		}
		path = filepath.Join(baseDir, path)
	}

	clean := filepath.Clean(path)
	rel, err := filepath.Rel(baseDir, clean)
	if err != nil {
		return "", fmt.Errorf("resolve custom target %q: %w", target, err)
	}
	parentPrefix := ".." + string(filepath.Separator)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, parentPrefix) {
		return "", fmt.Errorf("target %q must resolve to a subdirectory within the project root", target)
	}

	return resolveScopedDir(baseDir, clean, target)
}

func resolveTargetDirs(baseDir string, global bool, targets []string) ([]string, error) {
	seen := make(map[string]string, len(targets))
	dirsOut := make([]string, 0, len(targets))
	for _, targetName := range targets {
		dir, err := skillDir(baseDir, global, targetName)
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[dir]; ok {
			return nil, fmt.Errorf("targets %q and %q resolve to the same directory %s", previous, targetName, dir)
		}
		seen[dir] = targetName
		dirsOut = append(dirsOut, dir)
	}
	return dirsOut, nil
}

func expandHome(path string, homeDir string) string {
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	if strings.HasPrefix(path, "~\\") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func resolveScopedDir(baseDir, path, target string) (string, error) {
	baseReal, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve scope root %s: %w", baseDir, err)
	}

	path = filepath.Clean(path)
	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return "", fmt.Errorf("resolve custom target %q: %w", target, err)
	}
	if rel == "." {
		return "", fmt.Errorf("custom target %q must resolve to a subdirectory within the project root", target)
	}

	parts := strings.Split(rel, string(filepath.Separator))
	currentLex := baseDir
	currentReal := baseReal
	for i, part := range parts {
		nextLex := filepath.Join(currentLex, part)
		info, err := os.Lstat(nextLex)
		if errors.Is(err, os.ErrNotExist) {
			currentReal = filepath.Join(currentReal, filepath.Join(parts[i:]...))
			break
		}
		if err != nil {
			return "", fmt.Errorf("lstat %s: %w", nextLex, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(nextLex)
			if err != nil {
				return "", fmt.Errorf("resolve symlink %s: %w", nextLex, err)
			}
			if err := ensureWithinScope(baseReal, resolved, target); err != nil {
				return "", err
			}
			currentReal = resolved
			currentLex = nextLex
			continue
		}

		currentReal = filepath.Join(currentReal, part)
		currentLex = nextLex
	}

	if err := ensureWithinScope(baseReal, currentReal, target); err != nil {
		return "", err
	}
	return currentReal, nil
}

func ensureWithinScope(baseReal, path, target string) error {
	rel, err := filepath.Rel(baseReal, path)
	if err != nil {
		return fmt.Errorf("resolve target %q within scope: %w", target, err)
	}
	parentPrefix := ".." + string(filepath.Separator)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, parentPrefix) {
		return fmt.Errorf("target %q escapes the managed scope via symlink traversal", target)
	}
	return nil
}
