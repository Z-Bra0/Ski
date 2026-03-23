package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
)

// Remove deletes a skill from the active manifest, lockfile, and target installs.
// When targetOverride is non-empty, it removes only those targets from the skill.
// The store cache entry is left intact for potential reuse.
func (s Service) Remove(name string, targetOverride []string) error {
	manifestPath := s.manifestPath()
	originalManifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	doc, err := s.readManifest(manifestPath)
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
	if len(targetOverride) > 0 {
		effectiveTargets = intersectStrings(effectiveTargets, targetOverride)
	}

	previousStorePath := ""
	if lockEntry, ok := findLockSkill(lf.Skills, name); ok {
		src, err := s.loadSkillSourceForScope(lockEntry.Source, lockEntry.UpstreamSkill)
		if err != nil {
			return err
		}
		src.Ref = lockEntry.Commit
		stored, err := store.FindGit(s.HomeDir, src, lockEntry.Commit, name)
		if err != nil {
			return err
		}
		previousStorePath = stored.Path
	}

	changes, err := s.planRemoveTargetChanges(effectiveTargets, name, previousStorePath)
	if err != nil {
		return fmt.Errorf("remove targets: %w", err)
	}

	applied := make([]updateTargetChange, 0, len(changes))
	for i := range changes {
		backupPath, err := s.applyUpdateTargetChange(name, changes[i])
		if err != nil {
			rollbackApplied := append(append([]updateTargetChange(nil), applied...), changes[i])
			rollbackErr := s.rollbackRemove(name, rollbackApplied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return fmt.Errorf("remove targets: %w (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("remove targets: %w", err)
		}
		changes[i].BackupPath = backupPath
		applied = append(applied, changes[i])
	}

	currentTargets := effectiveTargetsForSkill(doc, ms)
	remainingTargets := differenceStrings(currentTargets, targetOverride)
	removeSkill := len(targetOverride) == 0 || len(remainingTargets) == 0

	if removeSkill {
		doc.Skills = removeByName(doc.Skills, name, func(skill manifest.Skill) string { return skill.Name })
	} else {
		for i := range doc.Skills {
			if doc.Skills[i].Name != name {
				continue
			}
			doc.Skills[i].Targets = skillTargetsOverride(doc.Targets, remainingTargets)
			break
		}
	}
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		rollbackErr := s.rollbackRemove(name, applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		if rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", manifestPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	if removeSkill {
		lf.Skills = removeByName(lf.Skills, name, func(skill lockfile.Skill) string { return skill.Name })
	} else {
		for i := range lf.Skills {
			if lf.Skills[i].Name != name {
				continue
			}
			lf.Skills[i].Targets = append([]string(nil), remainingTargets...)
			break
		}
	}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		rollbackErr := s.rollbackRemove(name, applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		if rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", lockPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	cleanupRemoveBackups(applied)
	return nil
}

func (s Service) planRemoveTargetChanges(targets []string, name, previousStorePath string) ([]updateTargetChange, error) {
	changes := make([]updateTargetChange, 0, len(targets))
	for _, targetName := range targets {
		inspection, err := s.inspectTarget(targetName, name, previousStorePath)
		if err != nil {
			return nil, err
		}
		switch inspection.Status {
		case targetStatusMissing:
			continue
		case targetStatusInstalled:
			if previousStorePath == "" {
				// No lockfile entry — allow removal without drift verification.
				// The caller backs up the target before applying this change so
				// that rollback can restore it if a later step fails.
				changes = append(changes, updateTargetChange{
					Target:      targetName,
					ForceRemove: true,
				})
				continue
			}
		case targetStatusLegacySymlink:
			return nil, legacySymlinkInstallError(inspection.Path)
		case targetStatusDrifted:
			return nil, driftedTargetError(inspection.Path)
		case targetStatusUnexpectedEntry:
			return nil, unexpectedTargetEntryError(inspection.Path)
		}

		changes = append(changes, updateTargetChange{
			Target:       targetName,
			PreviousPath: previousStorePath,
		})
	}

	return changes, nil
}

func (s Service) rollbackRemove(name string, applied []updateTargetChange, manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		change := applied[i]
		restorePath := change.PreviousPath
		if restorePath == "" {
			restorePath = change.BackupPath
		}
		if restorePath == "" {
			continue
		}
		if err := s.materializeAll([]string{change.Target}, name, restorePath); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	cleanupRemoveBackups(applied)
	if err := restoreProjectFiles(manifestPath, manifestData, lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}

func cleanupRemoveBackups(changes []updateTargetChange) {
	for _, change := range changes {
		if change.BackupPath != "" {
			os.RemoveAll(change.BackupPath)
		}
	}
}
