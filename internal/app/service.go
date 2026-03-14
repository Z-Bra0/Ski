package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/source"
	"ski/internal/store"
	"ski/internal/target"
)

// Service orchestrates ski operations for a single project.
type Service struct {
	ProjectDir string
	HomeDir    string
}

// Init creates a new ski.toml in the project directory.
// Returns the path of the created manifest.
func (s Service) Init() (string, error) {
	path := filepath.Join(s.ProjectDir, manifest.FileName)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	doc := manifest.Default()
	if err := manifest.WriteFile(path, doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// Add parses a git source, fetches it into the store, links to targets,
// and writes both ski.toml and ski.lock.json.
// Returns the skill name that was added.
func (s Service) Add(rawSource string, nameOverride string) (string, error) {
	path := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s not found; run `ski init` first", path)
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	src, err := source.ParseGit(rawSource)
	if err != nil {
		return "", err
	}

	skillName := nameOverride
	if skillName == "" {
		skillName, err = src.DeriveName()
		if err != nil {
			return "", err
		}
	}

	if existing, ok := findSkillByName(doc.Skills, skillName); ok {
		if nameOverride == "" {
			return "", fmt.Errorf("derived skill name %q already exists for source %q; rerun with --name", skillName, existing.Source)
		}
		return "", fmt.Errorf("skill name %q already exists", skillName)
	}

	canonical := src.String()
	if existing, ok := findSkillBySource(doc.Skills, canonical); ok {
		return "", fmt.Errorf("source %q already exists as skill %q", canonical, existing.Name)
	}

	stored, err := store.EnsureGit(s.ProjectDir, s.HomeDir, src, skillName)
	if err != nil {
		return "", err
	}

	effectiveTargets := append([]string(nil), doc.Targets...)
	if err := target.LinkAll(s.ProjectDir, effectiveTargets, skillName, stored.Path); err != nil {
		return "", err
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", lockPath, err)
	}
	upsertLockSkill(lf, lockfile.Skill{
		Name:      skillName,
		Source:    canonical,
		Commit:    stored.Commit,
		Integrity: stored.Integrity,
		Targets:   effectiveTargets,
	})
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return "", fmt.Errorf("write %s: %w", lockPath, err)
	}

	doc.Skills = append(doc.Skills, manifest.Skill{
		Name:   skillName,
		Source: canonical,
	})
	if err := manifest.WriteFile(path, *doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return skillName, nil
}

func findSkillByName(skills []manifest.Skill, name string) (manifest.Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return manifest.Skill{}, false
}

func findSkillBySource(skills []manifest.Skill, source string) (manifest.Skill, bool) {
	for _, skill := range skills {
		if skill.Source == source {
			return skill, true
		}
	}
	return manifest.Skill{}, false
}

func readOrDefaultLockfile(path string) (*lockfile.Lockfile, error) {
	lf, err := lockfile.ReadFile(path)
	if err == nil {
		return lf, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		doc := lockfile.Default()
		return &doc, nil
	}
	return nil, err
}

func upsertLockSkill(lf *lockfile.Lockfile, skill lockfile.Skill) {
	for i, existing := range lf.Skills {
		if existing.Name == skill.Name {
			lf.Skills[i] = skill
			return
		}
	}
	lf.Skills = append(lf.Skills, skill)
}

// Install reads ski.toml and ski.lock.json, fetches all skills into the store,
// verifies integrity against the lockfile, and links them to configured targets.
// Returns the number of skills processed.
func (s Service) Install() (int, error) {
	manifestPath := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%s not found; run `ski init` first", manifestPath)
		}
		return 0, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	effectiveTargets := append([]string(nil), doc.Targets...)
	count := 0
	for _, mSkill := range doc.Skills {
		src, err := source.ParseGit(mSkill.Source)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		// Pin to locked commit for reproducibility.
		lockedEntry, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock {
			src.Ref = lockedEntry.Commit
		}

		stored, err := store.EnsureGit(s.ProjectDir, s.HomeDir, src, mSkill.Name)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		if hasLock && stored.Integrity != lockedEntry.Integrity {
			return count, fmt.Errorf("skill %q: integrity mismatch: got %s, want %s",
				mSkill.Name, stored.Integrity, lockedEntry.Integrity)
		}

		if err := target.LinkAll(s.ProjectDir, effectiveTargets, mSkill.Name, stored.Path); err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		upsertLockSkill(lf, lockfile.Skill{
			Name:      mSkill.Name,
			Source:    mSkill.Source,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   effectiveTargets,
		})
		count++
	}

	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return count, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return count, nil
}

func findLockSkill(skills []lockfile.Skill, name string) (lockfile.Skill, bool) {
	for _, s := range skills {
		if s.Name == name {
			return s, true
		}
	}
	return lockfile.Skill{}, false
}
