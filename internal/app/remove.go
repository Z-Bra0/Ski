package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
)

// Remove deletes a skill from the active manifest, lockfile, and target symlinks.
// The store cache entry is left intact for potential reuse.
func (s Service) Remove(name string) error {
	manifestPath := s.manifestPath()
	originalManifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}

	effectiveTargets := effectiveTargetsForSkill(doc, ms)

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	if lockEntry, ok := findLockSkill(lf.Skills, name); ok {
		effectiveTargets = unionStrings(effectiveTargets, lockEntry.Targets)
	}

	changes, err := s.planRemoveTargetChanges(effectiveTargets, name)
	if err != nil {
		return fmt.Errorf("remove symlinks: %w", err)
	}

	applied := make([]updateTargetChange, 0, len(changes))
	for _, change := range changes {
		if err := s.applyUpdateTargetChange(name, change); err != nil {
			rollbackApplied := append(append([]updateTargetChange(nil), applied...), change)
			rollbackErr := s.rollbackRemove(name, rollbackApplied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return fmt.Errorf("remove symlinks: %w (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("remove symlinks: %w", err)
		}
		applied = append(applied, change)
	}

	doc.Skills = removeByName(doc.Skills, name, func(skill manifest.Skill) string { return skill.Name })
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		rollbackErr := s.rollbackRemove(name, applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		if rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", manifestPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	lf.Skills = removeByName(lf.Skills, name, func(skill lockfile.Skill) string { return skill.Name })
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		rollbackErr := s.rollbackRemove(name, applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		if rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", lockPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	return nil
}

func (s Service) planRemoveTargetChanges(targets []string, name string) ([]updateTargetChange, error) {
	changes := make([]updateTargetChange, 0, len(targets))
	for _, targetName := range targets {
		dir, err := s.skillDir(targetName)
		if err != nil {
			return nil, err
		}

		linkPath := filepath.Join(dir, name)
		previousPath, err := readExistingSymlink(linkPath)
		if err != nil {
			return nil, err
		}
		if previousPath == "" {
			continue
		}

		changes = append(changes, updateTargetChange{
			Target:       targetName,
			PreviousPath: previousPath,
		})
	}

	return changes, nil
}

func (s Service) rollbackRemove(name string, applied []updateTargetChange, manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		change := applied[i]
		if change.PreviousPath == "" {
			continue
		}
		if err := s.linkAll([]string{change.Target}, name, change.PreviousPath); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if err := restoreProjectFiles(manifestPath, manifestData, lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}
