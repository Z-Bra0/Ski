package target

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Z-Bra0/Ski/internal/fsutil"
)

const customDirPrefix = "dir:"

// MaterializeAll copies a skill into every target directory in project scope.
func MaterializeAll(projectRoot string, targets []string, name, storePath string) error {
	return linkAll(projectRoot, false, targets, name, storePath)
}

// MaterializeAllGlobal copies a skill into every target directory in global scope.
func MaterializeAllGlobal(homeDir string, targets []string, name, storePath string) error {
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

// Materialize copies a skill into one project-scoped target directory.
func Materialize(projectRoot, target, name, storePath string) error {
	dir, err := SkillDir(projectRoot, target)
	if err != nil {
		return err
	}
	return linkDir(dir, name, storePath)
}

// MaterializeGlobal copies a skill into one global-scoped target directory.
func MaterializeGlobal(homeDir, target, name, storePath string) error {
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

	entryPath := filepath.Join(dir, name)
	info, err := os.Lstat(entryPath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s already exists and is not a directory", entryPath)
		}
		return fmt.Errorf("%s already exists", entryPath)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("lstat %s: %w", entryPath, err)
	}

	return installDir(dir, name, storePath)
}

// Replace swaps the installed target entry for one project-scoped target directory.
func Replace(projectRoot, target, name, storePath string) error {
	dir, err := SkillDir(projectRoot, target)
	if err != nil {
		return err
	}
	return replaceDir(dir, name, storePath)
}

// ReplaceGlobal swaps the installed target entry for one global-scoped target directory.
func ReplaceGlobal(homeDir, target, name, storePath string) error {
	dir, err := GlobalSkillDir(homeDir, target)
	if err != nil {
		return err
	}
	return replaceDir(dir, name, storePath)
}

func replaceDir(dir, name, storePath string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	entryPath := filepath.Join(dir, name)
	info, err := os.Lstat(entryPath)
	if errors.Is(err, os.ErrNotExist) {
		return installDir(dir, name, storePath)
	}
	if err != nil {
		return fmt.Errorf("lstat %s: %w", entryPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s already exists and is not a directory", entryPath)
	}

	stagePath, err := stageCopy(dir, name, storePath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagePath)

	backupPath, err := reserveBackupPath(dir, name)
	if err != nil {
		return err
	}
	if err := os.Rename(entryPath, backupPath); err != nil {
		return fmt.Errorf("rename %s to backup: %w", entryPath, err)
	}
	if err := os.Rename(stagePath, entryPath); err != nil {
		restoreErr := os.Rename(backupPath, entryPath)
		if restoreErr != nil {
			return fmt.Errorf("finalize %s: %w (restore failed: %v)", entryPath, err, restoreErr)
		}
		return fmt.Errorf("finalize %s: %w", entryPath, err)
	}
	if err := os.RemoveAll(backupPath); err != nil {
		return fmt.Errorf("remove backup %s: %w", backupPath, err)
	}
	return nil
}

// RemoveAll removes a skill entry from every project-scoped target directory.
func RemoveAll(projectRoot string, targets []string, name string) error {
	return unlinkAll(projectRoot, false, targets, name)
}

// RemoveAllGlobal removes a skill entry from every global-scoped target directory.
func RemoveAllGlobal(homeDir string, targets []string, name string) error {
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

// Remove removes a skill entry from one project-scoped target directory.
func Remove(projectRoot, target, name string) error {
	dir, err := SkillDir(projectRoot, target)
	if err != nil {
		return err
	}
	return unlinkDir(dir, name)
}

// RemoveGlobal removes a skill entry from one global-scoped target directory.
func RemoveGlobal(homeDir, target, name string) error {
	dir, err := GlobalSkillDir(homeDir, target)
	if err != nil {
		return err
	}
	return unlinkDir(dir, name)
}

func unlinkDir(dir, name string) error {
	entryPath := filepath.Join(dir, name)
	info, err := os.Lstat(entryPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lstat %s: %w", entryPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory; remove it manually", entryPath)
	}
	if err := os.RemoveAll(entryPath); err != nil {
		return fmt.Errorf("remove %s: %w", entryPath, err)
	}
	return nil
}

func installDir(dir, name, storePath string) error {
	stagePath, err := stageCopy(dir, name, storePath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagePath)

	entryPath := filepath.Join(dir, name)
	if err := os.Rename(stagePath, entryPath); err != nil {
		return fmt.Errorf("finalize %s: %w", entryPath, err)
	}
	return nil
}

func stageCopy(dir, name, storePath string) (string, error) {
	stagePath, err := os.MkdirTemp(dir, "."+name+"-staged-")
	if err != nil {
		return "", fmt.Errorf("create staging dir for %s: %w", filepath.Join(dir, name), err)
	}
	if err := os.Remove(stagePath); err != nil {
		return "", fmt.Errorf("prepare staging path %s: %w", stagePath, err)
	}
	if err := fsutil.CopyTree(storePath, stagePath); err != nil {
		return "", fmt.Errorf("copy %s to staging dir: %w", storePath, err)
	}
	return stagePath, nil
}

func reserveBackupPath(dir, name string) (string, error) {
	backupPath, err := os.MkdirTemp(dir, "."+name+"-backup-")
	if err != nil {
		return "", fmt.Errorf("create backup path for %s: %w", filepath.Join(dir, name), err)
	}
	if err := os.Remove(backupPath); err != nil {
		return "", fmt.Errorf("prepare backup path %s: %w", backupPath, err)
	}
	return backupPath, nil
}

// SkillDir resolves a target name to its project-scoped directory.
func SkillDir(projectRoot, target string) (string, error) {
	return skillDir(projectRoot, false, target)
}

// GlobalSkillDir resolves a target name to its global-scoped directory.
func GlobalSkillDir(homeDir, target string) (string, error) {
	return skillDir(homeDir, true, target)
}

func IsBuiltIn(targetName string) bool {
	_, ok := builtinsByName[targetName]
	return ok
}

func BuiltInNames() []string {
	names := make([]string, 0, len(builtins))
	for _, builtin := range builtins {
		names = append(names, builtin.Name)
	}
	return names
}

func skillDir(baseDir string, global bool, target string) (string, error) {
	builtin, ok := builtinsByName[target]
	if ok {
		rel := builtin.ProjectDir
		if global {
			rel = builtin.GlobalDir
		}
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
	if rel == "." {
		return "", fmt.Errorf("custom target %q would install skills into the %s; use a subdirectory", target, scopeRootLabel(global))
	}
	parentPrefix := ".." + string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, parentPrefix) {
		return "", fmt.Errorf("target %q must resolve to a subdirectory within the %s", target, scopeRootLabel(global))
	}

	return resolveScopedDir(baseDir, clean, target)
}

func scopeRootLabel(global bool) string {
	if global {
		return "user home directory"
	}
	return "project root"
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
		return "", fmt.Errorf("custom target %q would install skills into the managed scope root; use a subdirectory", target)
	}

	// Walk each existing component with Lstat/EvalSymlinks so an in-scope lexical
	// path cannot escape the managed base through a symlink hop.
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
