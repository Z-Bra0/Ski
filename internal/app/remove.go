package app

import (
	"errors"
	"fmt"
	"os"

	"ski/internal/lockfile"
	"ski/internal/manifest"
)

// Remove deletes a skill from the active manifest, lockfile, and target symlinks.
// The store cache entry is left intact for potential reuse.
func (s Service) Remove(name string) error {
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}

	effectiveTargets := effectiveTargetsForSkill(doc, ms)

	lockPath := s.lockPath()
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	if lock, ok := findLockSkill(lf.Skills, name); ok {
		effectiveTargets = unionStrings(effectiveTargets, lock.Targets)
	}

	doc.Skills = removeByName(doc.Skills, name, func(skill manifest.Skill) string { return skill.Name })
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	lf.Skills = removeByName(lf.Skills, name, func(skill lockfile.Skill) string { return skill.Name })
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	if err := s.unlinkAll(effectiveTargets, name); err != nil {
		return fmt.Errorf("remove symlinks: %w", err)
	}

	return nil
}
