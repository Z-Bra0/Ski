package app

import (
	"errors"
	"fmt"
	"os"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/store"
)

// Disable marks a declared skill as disabled and removes its installed targets.
func (s Service) Disable(name string) error {
	manifestPath := s.manifestPath()
	originalManifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	doc, err := manifest.Parse(originalManifestData)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	if err := s.validateManifestTargets(doc); err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}
	if !skillEnabled(ms) {
		return fmt.Errorf("skill %q is already disabled", name)
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := parseOrDefaultLockfile(originalLockData, hadLockfile)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}

	effectiveTargets := effectiveTargetsForSkill(doc, ms)
	lockEntry, hasLock := findLockSkill(lf.Skills, name)
	if hasLock {
		effectiveTargets = unionStrings(effectiveTargets, lockEntry.Targets)
	}
	previousStorePath, err := s.ensureLockedSkillStorePath(lockEntry, hasLock, name)
	if err != nil {
		return err
	}

	changes, err := s.planRemoveTargetChanges(effectiveTargets, name, previousStorePath)
	if err != nil {
		return fmt.Errorf("disable targets: %w", err)
	}

	applied := make([]updateTargetChange, 0, len(changes))
	for i := range changes {
		backupPath, err := s.applyUpdateTargetChange(name, changes[i])
		if err != nil {
			rollbackApplied := append(append([]updateTargetChange(nil), applied...), changes[i])
			rollbackErr := s.rollbackRemove(name, rollbackApplied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return fmt.Errorf("disable targets: %w (rollback failed: %v)", err, rollbackErr)
			}
			return fmt.Errorf("disable targets: %w", err)
		}
		changes[i].BackupPath = backupPath
		applied = append(applied, changes[i])
	}

	for i := range doc.Skills {
		if doc.Skills[i].Name != name {
			continue
		}
		setSkillEnabled(&doc.Skills[i], false)
		break
	}
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		rollbackErr := s.rollbackRemove(name, applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		if rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", manifestPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	cleanupTargetChangeBackups(applied)
	return nil
}

// Enable marks a declared disabled skill as enabled and restores its targets.
func (s Service) Enable(name string) error {
	manifestPath := s.manifestPath()
	originalManifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	doc, err := manifest.Parse(originalManifestData)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	if err := s.validateManifestTargets(doc); err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}
	if skillEnabled(ms) {
		return fmt.Errorf("skill %q is already enabled", name)
	}

	lockPath := s.lockPath()
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	lockEntry, ok := findLockSkill(lf.Skills, name)
	if !ok {
		return fmt.Errorf("skill %q has no lockfile entry to enable from", name)
	}

	stored, err := s.ensureLockedSkillStored(lockEntry, name)
	if err != nil {
		return fmt.Errorf("skill %q: %w", name, err)
	}
	installTargets, err := s.preflightAddTargets(effectiveTargetsForSkill(doc, ms), name, stored.Path)
	if err != nil {
		return fmt.Errorf("enable targets: %w", err)
	}

	for i := range doc.Skills {
		if doc.Skills[i].Name != name {
			continue
		}
		setSkillEnabled(&doc.Skills[i], true)
		break
	}
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	if err := s.materializeAll(installTargets, name, stored.Path); err != nil {
		rollbackErr := s.removeAll(installTargets, name)
		restoreErr := os.WriteFile(manifestPath, originalManifestData, 0o644)
		if rollbackErr != nil || restoreErr != nil {
			return fmt.Errorf("enable targets: %w (rollback remove failed: %v, restore failed: %v)", err, rollbackErr, restoreErr)
		}
		return fmt.Errorf("enable targets: %w", err)
	}

	return nil
}

func (s Service) ensureLockedSkillStored(lockEntry lockfile.Skill, skillName string) (store.Result, error) {
	src, err := s.loadSkillSourceForScope(lockEntry.Source, lockEntry.UpstreamSkill)
	if err != nil {
		return store.Result{}, err
	}
	src.Ref = lockEntry.Commit
	stored, err := s.ensureStoredForLock(src, lockEntry, skillName)
	if err != nil {
		return store.Result{}, err
	}
	return stored, nil
}

func (s Service) ensureLockedSkillStorePath(lockEntry lockfile.Skill, hasLock bool, skillName string) (string, error) {
	if !hasLock {
		return "", nil
	}
	stored, err := s.ensureLockedSkillStored(lockEntry, skillName)
	if err != nil {
		return "", err
	}
	return stored.Path, nil
}

func (s Service) ensureStoredForLock(src source.Git, lockEntry lockfile.Skill, skillName string) (store.Result, error) {
	stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, skillName)
	if err != nil {
		return store.Result{}, err
	}
	if stored.Integrity != lockEntry.Integrity {
		return store.Result{}, fmt.Errorf("integrity mismatch: got %s, want %s", stored.Integrity, lockEntry.Integrity)
	}
	return stored, nil
}
