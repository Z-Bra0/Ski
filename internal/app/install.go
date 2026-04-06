package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/store"
)

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
	lf, err := parseOrDefaultLockfile(originalLockData, hadLockfile)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	lockByName := make(map[string]lockfile.Skill, len(lf.Skills))
	for _, ls := range lf.Skills {
		lockByName[ls.Name] = ls
	}

	nextLock := cloneLockfile(*lf)
	plans := make([]plannedTargetChanges, 0, len(doc.Skills))
	for _, mSkill := range doc.Skills {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockedEntry, hasLock := lockByName[mSkill.Name]
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
		} else if !skillEnabled(mSkill) {
			// No lock entry for a disabled skill. Use the effective targets as the
			// previous set so that any stale target directories from a prior install
			// are detected and removed.
			previousTargets = effectiveTargets
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
		plans = append(plans, plannedTargetChanges{
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

	applied := make([]plannedTargetChanges, 0, len(plans))
	for _, plan := range plans {
		appliedPlan, failure := s.applyTargetChangePlan(plan, applied, func(applied []plannedTargetChanges) error {
			return s.rollbackInstall(applied, lockPath, originalLockData, hadLockfile)
		})
		if failure != nil {
			return 0, formatTargetChangeFailure(fmt.Sprintf("skill %q: ", failure.Name), failure)
		}
		applied = append(applied, appliedPlan)
	}

	cleanupTargetChangePlanBackups(applied)
	return len(plans), nil
}

func (s Service) rollbackInstall(applied []plannedTargetChanges, lockPath string, lockData []byte, hadLockfile bool) error {
	rollbackErr := s.rollbackTargetChangePlans(applied)
	cleanupTargetChangePlanBackups(applied)
	if err := restoreLockfile(lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}
