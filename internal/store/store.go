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

	"github.com/Z-Bra0/Ski/internal/skill"
	"github.com/Z-Bra0/Ski/internal/source"
)

var renameDir = os.Rename

const maxSkillDiscoveryDepth = 3

// Result describes one selected skill inside a stored repository snapshot.
type Result struct {
	Commit    string
	Integrity string
	Path      string
}

// RepoResult describes a discovered repository snapshot and its skills.
type RepoResult struct {
	Commit        string
	Root          string
	Skills        []DiscoveredSkill
	InvalidSkills []InvalidSkill
}

// DiscoveredSkill identifies one skill directory inside a repository snapshot.
type DiscoveredSkill struct {
	Name         string
	RelativePath string
	Path         string
}

// InvalidSkill describes a discovered SKILL.md that exists but could not be parsed or validated.
type InvalidSkill struct {
	CandidateName string
	Path          string
	Err           error
}

type discoveredSkillNameResult struct {
	Name       string
	Found      bool
	InvalidErr error
}

// DiscoverGit fetches or loads a git repository snapshot from the shared store.
func DiscoverGit(projectRoot, homeDir string, spec source.Git) (RepoResult, error) {
	storeKey, err := spec.DeriveName()
	if err != nil {
		return RepoResult{}, err
	}

	// Reuse an existing stored snapshot when we can cheaply resolve the commit.
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

	skills, invalidSkills, err := discoverSkills(checkoutDir)
	if err != nil {
		return RepoResult{}, err
	}
	if len(skills) == 0 {
		if len(invalidSkills) > 0 {
			rewritten, err := storeInvalidGitSnapshot(checkoutDir, storePath, invalidSkills)
			if err != nil {
				return RepoResult{}, err
			}
			return RepoResult{}, rewritten[0].Err
		}
		return RepoResult{}, fmt.Errorf("no skills found in repository")
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return RepoResult{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(storePath), err)
	}
	// The store keeps plain snapshots, not working clones with git metadata.
	if err := os.RemoveAll(filepath.Join(checkoutDir, ".git")); err != nil {
		return RepoResult{}, fmt.Errorf("remove git metadata: %w", err)
	}
	if err := moveDirIntoStore(checkoutDir, storePath); err != nil {
		return RepoResult{}, fmt.Errorf("move checkout into store: %w", err)
	}

	return buildRepoResult(
		storePath,
		commit,
		skills,
		rewriteInvalidSkillPaths(invalidSkills, checkoutDir, storePath),
	), nil
}

func storeInvalidGitSnapshot(checkoutDir, storePath string, invalidSkills []InvalidSkill) ([]InvalidSkill, error) {
	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(storePath), err)
	}
	if err := os.RemoveAll(filepath.Join(checkoutDir, ".git")); err != nil {
		return nil, fmt.Errorf("remove git metadata: %w", err)
	}
	if err := moveDirIntoStore(checkoutDir, storePath); err != nil {
		return nil, fmt.Errorf("move checkout into store: %w", err)
	}
	return rewriteInvalidSkillPaths(invalidSkills, checkoutDir, storePath), nil
}

// EnsureGit ensures a selected skill is present in the store and returns its path.
func EnsureGit(projectRoot, homeDir string, spec source.Git, expectedName string) (Result, error) {
	result, _, err := EnsureGitWithWarnings(projectRoot, homeDir, spec, expectedName)
	return result, err
}

// EnsureGitWithWarnings ensures a selected skill is present in the store and
// returns strict-spec warnings for the selected skill only.
func EnsureGitWithWarnings(projectRoot, homeDir string, spec source.Git, expectedName string) (Result, []skill.ValidationWarning, error) {
	repo, err := DiscoverGit(projectRoot, homeDir, spec)
	if err != nil {
		return Result{}, nil, err
	}

	selected, err := resolveSelectedSkill(repo, spec, expectedName)
	if err != nil {
		return Result{}, nil, err
	}
	if _, warnings, err := skill.ValidateDirWithWarnings(selected.Path, selected.Name); err != nil {
		return Result{}, nil, err
	} else {
		integrity, err := HashDir(repo.Root)
		if err != nil {
			return Result{}, nil, fmt.Errorf("hash %s: %w", repo.Root, err)
		}

		return Result{Commit: repo.Commit, Integrity: integrity, Path: selected.Path}, warnings, nil
	}
}

// FindGit locates an already-stored git snapshot at a specific commit.
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

	selected, err := resolveSelectedSkill(repo, spec, expectedName)
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

	skills, invalidSkills, err := discoverSkills(storePath)
	if err != nil {
		return RepoResult{}, err
	}
	if len(skills) == 0 {
		if len(invalidSkills) > 0 {
			return RepoResult{}, invalidSkills[0].Err
		}
		return RepoResult{}, fmt.Errorf("no skills found in store snapshot %s", storePath)
	}
	return buildRepoResult(storePath, commit, skills, invalidSkills), nil
}

func buildRepoResult(root, commit string, skills []DiscoveredSkill, invalidSkills []InvalidSkill) RepoResult {
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
	return RepoResult{
		Commit:        commit,
		Root:          root,
		Skills:        out,
		InvalidSkills: append([]InvalidSkill(nil), invalidSkills...),
	}
}

func rewriteInvalidSkillPaths(invalidSkills []InvalidSkill, fromRoot, toRoot string) []InvalidSkill {
	if len(invalidSkills) == 0 || fromRoot == toRoot {
		return append([]InvalidSkill(nil), invalidSkills...)
	}

	out := make([]InvalidSkill, 0, len(invalidSkills))
	for _, invalid := range invalidSkills {
		rewritten := invalid
		oldPath := invalid.Path
		if rel, err := filepath.Rel(fromRoot, invalid.Path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			rewritten.Path = filepath.Join(toRoot, rel)
		}
		if oldPath != rewritten.Path && invalid.Err != nil {
			rewritten.Err = fmt.Errorf(strings.ReplaceAll(invalid.Err.Error(), oldPath, rewritten.Path))
		}
		out = append(out, rewritten)
	}
	return out
}

func discoverSkills(root string) ([]DiscoveredSkill, []InvalidSkill, error) {
	skills := make([]DiscoveredSkill, 0)
	invalidSkills := make([]InvalidSkill, 0)
	seen := make(map[string]string)

	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		result, err := loadDiscoveredSkillName(dir)
		if err != nil {
			return err
		} else if result.InvalidErr != nil {
			candidateName := result.Name
			if candidateName == "" {
				candidateName = fallbackCandidateSkillName(root, dir)
			}
			invalidSkills = append(invalidSkills, InvalidSkill{
				CandidateName: candidateName,
				Path:          filepath.Join(dir, skill.FileName),
				Err:           result.InvalidErr,
			})
		} else if result.Found {
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return fmt.Errorf("derive relative path for %s: %w", dir, err)
			}
			if previous, ok := seen[result.Name]; ok {
				return fmt.Errorf("duplicate skill name %q found at %s and %s", result.Name, previous, rel)
			}
			seen[result.Name] = rel
			skills = append(skills, DiscoveredSkill{
				Name:         result.Name,
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
		return nil, nil, err
	}

	slices.SortFunc(skills, func(a, b DiscoveredSkill) int {
		return strings.Compare(a.Name, b.Name)
	})
	slices.SortFunc(invalidSkills, func(a, b InvalidSkill) int {
		return strings.Compare(a.Path, b.Path)
	})
	return skills, invalidSkills, nil
}

func loadDiscoveredSkillName(dir string) (discoveredSkillNameResult, error) {
	path := filepath.Join(dir, skill.FileName)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return discoveredSkillNameResult{}, nil
		}
		return discoveredSkillNameResult{}, fmt.Errorf("stat %s: %w", path, err)
	}

	name, err := skill.DiscoverName(dir)
	if err != nil {
		return discoveredSkillNameResult{
			Name:       skill.DiscoverCandidateName(dir),
			InvalidErr: err,
		}, nil
	}
	return discoveredSkillNameResult{
		Name:  name,
		Found: true,
	}, nil
}

func fallbackCandidateSkillName(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err == nil && rel != "." {
		return filepath.Base(dir)
	}
	return filepath.Base(root)
}

func matchInvalidSelectedSkill(repo RepoResult, spec source.Git, expectedName string) error {
	requested := expectedName
	if len(spec.Skills) == 1 {
		requested = spec.Skills[0]
	}
	if requested == "" {
		return nil
	}
	for _, invalid := range repo.InvalidSkills {
		if invalid.CandidateName == requested {
			return invalid.Err
		}
	}
	return nil
}

func resolveSelectedSkill(repo RepoResult, spec source.Git, expectedName string) (DiscoveredSkill, error) {
	selected, err := resolveDiscoveredSkill(repo.Skills, spec, expectedName)
	if err != nil {
		if invalidErr := matchInvalidSelectedSkill(repo, spec, expectedName); invalidErr != nil {
			return DiscoveredSkill{}, invalidErr
		}
		return DiscoveredSkill{}, err
	}
	return selected, nil
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

	// Cross-device renames fail with EXDEV, so stage a full copy on the target
	// filesystem and then do the final rename there.
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

// HashDir returns the canonical SHA-256 hash for a stored snapshot directory.
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
