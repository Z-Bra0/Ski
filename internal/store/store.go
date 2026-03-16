package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"ski/internal/skill"
	"ski/internal/source"
)

var renameDir = os.Rename

const maxSkillDiscoveryDepth = 3

type Result struct {
	Commit    string
	Integrity string
	Path      string
}

type RepoResult struct {
	Commit string
	Root   string
	Skills []DiscoveredSkill
}

type DiscoveredSkill struct {
	Name         string
	RelativePath string
	Path         string
}

func DiscoverGit(projectRoot, homeDir string, spec source.Git) (RepoResult, error) {
	storeKey, err := spec.DeriveName()
	if err != nil {
		return RepoResult{}, err
	}

	if commit, ok := resolveStoreCommit(projectRoot, spec); ok {
		storePath := filepath.Join(homeDir, ".ski", "store", "git", storeKey, commit)
		repo, err := loadStoredRepo(storePath, commit)
		if err == nil {
			return repo, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return RepoResult{}, err
		}
	}

	tmpRoot, err := os.MkdirTemp("", "ski-git-*")
	if err != nil {
		return RepoResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	checkoutDir := filepath.Join(tmpRoot, "checkout")
	if err := runGit(projectRoot, "clone", "--quiet", spec.URL, checkoutDir); err != nil {
		return RepoResult{}, fmt.Errorf("clone %q: %w", spec.URL, err)
	}

	if spec.Ref != "" {
		if err := runGit(projectRoot, "-C", checkoutDir, "-c", "advice.detachedHead=false", "checkout", "--quiet", spec.Ref); err != nil {
			return RepoResult{}, fmt.Errorf("checkout %q: %w", spec.Ref, err)
		}
	}

	commit, err := gitOutput(projectRoot, "-C", checkoutDir, "rev-parse", "HEAD")
	if err != nil {
		return RepoResult{}, fmt.Errorf("resolve commit: %w", err)
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", storeKey, commit)
	repo, err := loadStoredRepo(storePath, commit)
	if err == nil {
		return repo, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RepoResult{}, err
	}

	skills, err := discoverSkills(checkoutDir)
	if err != nil {
		return RepoResult{}, err
	}
	if len(skills) == 0 {
		return RepoResult{}, fmt.Errorf("no skills found in repository")
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return RepoResult{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(storePath), err)
	}
	if err := os.RemoveAll(filepath.Join(checkoutDir, ".git")); err != nil {
		return RepoResult{}, fmt.Errorf("remove git metadata: %w", err)
	}
	if err := moveDirIntoStore(checkoutDir, storePath); err != nil {
		return RepoResult{}, fmt.Errorf("move checkout into store: %w", err)
	}

	return buildRepoResult(storePath, commit, skills), nil
}

func EnsureGit(projectRoot, homeDir string, spec source.Git, expectedName string) (Result, error) {
	repo, err := DiscoverGit(projectRoot, homeDir, spec)
	if err != nil {
		return Result{}, err
	}

	selected, err := resolveDiscoveredSkill(repo.Skills, spec, expectedName)
	if err != nil {
		return Result{}, err
	}

	integrity, err := HashDir(repo.Root)
	if err != nil {
		return Result{}, fmt.Errorf("hash %s: %w", repo.Root, err)
	}

	return Result{Commit: repo.Commit, Integrity: integrity, Path: selected.Path}, nil
}

func FindGit(homeDir string, spec source.Git, commit string, expectedName string) (Result, error) {
	storeKey, err := spec.DeriveName()
	if err != nil {
		return Result{}, err
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", storeKey, commit)
	repo, err := loadStoredRepo(storePath, commit)
	if err != nil {
		return Result{}, err
	}

	selected, err := resolveDiscoveredSkill(repo.Skills, spec, expectedName)
	if err != nil {
		return Result{}, err
	}

	integrity, err := HashDir(repo.Root)
	if err != nil {
		return Result{}, fmt.Errorf("hash %s: %w", repo.Root, err)
	}

	return Result{Commit: commit, Integrity: integrity, Path: selected.Path}, nil
}

func resolveStoreCommit(projectRoot string, spec source.Git) (string, bool) {
	if spec.Ref != "" && source.IsCommitRef(spec.Ref) {
		return spec.Ref, true
	}

	commit, err := source.ResolveGit(projectRoot, spec.WithoutSkills())
	if err != nil {
		return "", false
	}
	return commit, true
}

func loadStoredRepo(storePath, commit string) (RepoResult, error) {
	info, err := os.Stat(storePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepoResult{}, err
		}
		return RepoResult{}, fmt.Errorf("stat %s: %w", storePath, err)
	}
	if !info.IsDir() {
		return RepoResult{}, fmt.Errorf("store path %s is not a directory", storePath)
	}

	skills, err := discoverSkills(storePath)
	if err != nil {
		return RepoResult{}, err
	}
	if len(skills) == 0 {
		return RepoResult{}, fmt.Errorf("no skills found in store snapshot %s", storePath)
	}
	return buildRepoResult(storePath, commit, skills), nil
}

func buildRepoResult(root, commit string, skills []DiscoveredSkill) RepoResult {
	out := make([]DiscoveredSkill, 0, len(skills))
	for _, discovered := range skills {
		rel := discovered.RelativePath
		if rel == "" {
			rel = "."
		}
		out = append(out, DiscoveredSkill{
			Name:         discovered.Name,
			RelativePath: rel,
			Path:         filepath.Join(root, rel),
		})
	}
	return RepoResult{Commit: commit, Root: root, Skills: out}
}

func discoverSkills(root string) ([]DiscoveredSkill, error) {
	skills := make([]DiscoveredSkill, 0)
	seen := make(map[string]string)

	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		if meta, found, err := loadSkillMetadata(dir); err != nil {
			return err
		} else if found {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return fmt.Errorf("derive relative path for %s: %w", dir, err)
			}
			if previous, ok := seen[meta.Name]; ok {
				return fmt.Errorf("duplicate skill name %q found at %s and %s", meta.Name, previous, rel)
			}
			seen[meta.Name] = rel
			skills = append(skills, DiscoveredSkill{
				Name:         meta.Name,
				RelativePath: rel,
				Path:         dir,
			})
		}

		if depth >= maxSkillDiscoveryDepth {
			return nil
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if shouldSkipDir(entry.Name()) {
				continue
			}
			if err := walk(filepath.Join(dir, entry.Name()), depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walk(root, 0); err != nil {
		return nil, err
	}

	slices.SortFunc(skills, func(a, b DiscoveredSkill) int {
		return strings.Compare(a.Name, b.Name)
	})
	return skills, nil
}

func loadSkillMetadata(dir string) (*skill.Metadata, bool, error) {
	path := filepath.Join(dir, skill.FileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat %s: %w", path, err)
	}

	meta, err := skill.ValidateDir(dir, "")
	if err != nil {
		return nil, false, err
	}
	return meta, true, nil
}

func resolveDiscoveredSkill(skills []DiscoveredSkill, spec source.Git, expectedName string) (DiscoveredSkill, error) {
	if len(spec.Skills) > 1 {
		return DiscoveredSkill{}, fmt.Errorf("source %q selects multiple skills; add one manifest entry per skill", spec.String())
	}

	if len(spec.Skills) == 1 {
		for _, discovered := range skills {
			if discovered.Name == spec.Skills[0] {
				return discovered, nil
			}
		}
		return DiscoveredSkill{}, fmt.Errorf("skill %q not found in repository (available: %s)", spec.Skills[0], strings.Join(discoveredSkillNames(skills), ", "))
	}

	switch len(skills) {
	case 0:
		return DiscoveredSkill{}, fmt.Errorf("no skills found in repository")
	case 1:
		return skills[0], nil
	default:
		for _, discovered := range skills {
			if expectedName != "" && discovered.Name == expectedName {
				return discovered, nil
			}
		}
		return DiscoveredSkill{}, fmt.Errorf("multiple skills found in repository: %s", strings.Join(discoveredSkillNames(skills), ", "))
	}
}

func discoveredSkillNames(skills []DiscoveredSkill) []string {
	names := make([]string, 0, len(skills))
	for _, discovered := range skills {
		names = append(names, discovered.Name)
	}
	return names
}

func shouldSkipDir(name string) bool {
	return name == ".git" || name == "node_modules" || strings.HasPrefix(name, ".")
}

func moveDirIntoStore(src, dst string) error {
	if err := renameDir(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	stageRoot, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-tmp-")
	if err != nil {
		return fmt.Errorf("create store staging dir: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	stagePath := filepath.Join(stageRoot, filepath.Base(dst))
	if err := copyTree(src, stagePath); err != nil {
		return fmt.Errorf("copy checkout into store staging dir: %w", err)
	}
	if err := renameDir(stagePath, dst); err != nil {
		return fmt.Errorf("finalize staged store dir: %w", err)
	}
	return nil
}

func HashDir(root string) (string, error) {
	hasher := sha256.New()
	if err := hashDir(hasher, root, ""); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashDir(w io.Writer, absPath, relPath string) error {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return err
	}

	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	for _, entry := range entries {
		name := entry.Name()
		childAbs := filepath.Join(absPath, name)
		childRel := filepath.ToSlash(filepath.Join(relPath, name))

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			if _, err := io.WriteString(w, "dir\x00"+childRel+"\x00"); err != nil {
				return err
			}
			if err := hashDir(w, childAbs, childRel); err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(childAbs)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(w, "symlink\x00"+childRel+"\x00"+target+"\x00"); err != nil {
				return err
			}
		default:
			if _, err := io.WriteString(w, "file\x00"+childRel+"\x00"); err != nil {
				return err
			}
			data, err := os.ReadFile(childAbs)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\x00"); err != nil {
				return err
			}
		}
	}

	return nil
}

func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyTree(srcPath, dstPath); err != nil {
				return err
			}
			if err := os.Chmod(dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			if err := copyFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}

	return os.Chmod(dst, info.Mode().Perm())
}

func copyFile(src, dst string, perm os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
