package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/store"
)

type plannedInstall struct {
	Name    string
	Changes []updateTargetChange
}

// Install reads the active manifest and lockfile, fetches all skills into the
// store, verifies integrity, and installs them to configured targets.
// Returns the number of skills processed.
func (s Service) Install() (int, error) {
	manifestPath := s.manifestPath()
	doc, err := s.readManifest(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return 0, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	nextLock := cloneLockfile(*lf)
	plans := make([]plannedInstall, 0, len(doc.Skills))
	for _, mSkill := range doc.Skills {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockedEntry, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock {
			src.Ref = lockedEntry.Commit
		}

		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		if hasLock && stored.Integrity != lockedEntry.Integrity {
			return 0, fmt.Errorf("skill %q: integrity mismatch: got %s, want %s",
				mSkill.Name, stored.Integrity, lockedEntry.Integrity)
		}

		effectiveTargets := effectiveTargetsForSkill(doc, mSkill)
		desiredTargets := installTargetsForSkill(doc, mSkill)
		previousTargets := []string(nil)
		previousPath := ""
		if hasLock {
			previousTargets = append(previousTargets, lockedEntry.Targets...)
			previousPath = stored.Path
		}
		changes, err := s.planUpdateTargetChanges(mSkill.Name, desiredTargets, previousTargets, previousPath, stored.Path)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockEntry := lockfile.Skill{
			Name:      mSkill.Name,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Version:   mSkill.Version,
			Targets:   effectiveTargets,
		}
		lockEntry.Source, lockEntry.UpstreamSkill, err = canonicalSkillIdentity(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		upsertLockSkill(&nextLock, lockEntry)
		plans = append(plans, plannedInstall{
			Name:    mSkill.Name,
			Changes: changes,
		})
	}

	if err := ensureParentDir(lockPath); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		return 0, fmt.Errorf("write %s: %w", lockPath, err)
	}

	applied := make([]plannedInstall, 0, len(plans))
	for _, plan := range plans {
		appliedCount := 0
		for i := range plan.Changes {
			backupPath, err := s.applyUpdateTargetChange(plan.Name, plan.Changes[i])
			if err != nil {
				rollbackPlans := append([]plannedInstall(nil), applied...)
				if appliedCount > 0 {
					rollbackPlans = append(rollbackPlans, plannedInstall{
						Name:    plan.Name,
						Changes: append([]updateTargetChange(nil), plan.Changes[:appliedCount]...),
					})
				}
				rollbackErr := s.rollbackInstall(rollbackPlans, lockPath, originalLockData, hadLockfile)
				if rollbackErr != nil {
					return 0, fmt.Errorf("skill %q: %w (rollback failed: %v)", plan.Name, err, rollbackErr)
				}
				return 0, fmt.Errorf("skill %q: %w", plan.Name, err)
			}
			plan.Changes[i].BackupPath = backupPath
			appliedCount++
		}
		plan.Changes = plan.Changes[:appliedCount]
		applied = append(applied, plan)
	}

	cleanupInstallBackups(applied)
	return len(plans), nil
}

func (s Service) rollbackInstall(applied []plannedInstall, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		for j := len(applied[i].Changes) - 1; j >= 0; j-- {
			change := applied[i].Changes[j]
			if change.PreviousPath == change.DesiredPath && change.BackupPath == "" {
				continue
			}
			if _, err := s.applyUpdateTargetChange(applied[i].Name, reverseUpdateTargetChange(change)); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
	}
	cleanupInstallBackups(applied)
	if err := restoreLockfile(lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}

func cleanupInstallBackups(plans []plannedInstall) {
	for _, plan := range plans {
		for _, change := range plan.Changes {
			if change.BackupPath != "" {
				os.RemoveAll(change.BackupPath)
			}
		}
	}
}
