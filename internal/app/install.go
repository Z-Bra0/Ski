package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
)

type plannedInstall struct {
	Name    string
	Changes []updateTargetChange
}

// Install reads the active manifest and lockfile, fetches all skills into the
// store, verifies integrity, and links them to configured targets.
// Returns the number of skills processed.
func (s Service) Install() (int, error) {
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
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
		if _, err := s.preflightInstallLinks(effectiveTargets, mSkill.Name, stored.Path); err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		changes, err := s.planUpdateTargetChanges(mSkill, effectiveTargets, lockedEntry, hasLock, stored.Path)
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

	linked := make([]plannedInstall, 0, len(plans))
	for _, plan := range plans {
		appliedCount := 0
		for _, change := range plan.Changes {
			if err := s.applyUpdateTargetChange(plan.Name, change); err != nil {
				rollbackPlans := append([]plannedInstall(nil), linked...)
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
			appliedCount++
		}
		plan.Changes = plan.Changes[:appliedCount]
		linked = append(linked, plan)
	}

	return len(plans), nil
}

func (s Service) preflightInstallLinks(targets []string, name, storePath string) ([]string, error) {
	seen := make(map[string]string, len(targets))
	targetsToCreate := make([]string, 0, len(targets))
	for _, targetName := range targets {
		dir, err := s.skillDir(targetName)
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[dir]; ok {
			return nil, fmt.Errorf("targets %q and %q resolve to the same directory %s", previous, targetName, dir)
		}
		seen[dir] = targetName

		linkPath := filepath.Join(dir, name)
		info, err := os.Lstat(linkPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return nil, fmt.Errorf("%s already exists and is not a symlink", linkPath)
			}
			current, err := os.Readlink(linkPath)
			if err != nil {
				return nil, fmt.Errorf("readlink %s: %w", linkPath, err)
			}
			if current != storePath {
				return nil, fmt.Errorf("%s already links to %s", linkPath, current)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("lstat %s: %w", linkPath, err)
		}
		targetsToCreate = append(targetsToCreate, targetName)
	}
	return targetsToCreate, nil
}

func (s Service) rollbackInstall(linked []plannedInstall, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(linked) - 1; i >= 0; i-- {
		for j := len(linked[i].Changes) - 1; j >= 0; j-- {
			change := linked[i].Changes[j]
			if change.PreviousPath == change.DesiredPath {
				continue
			}
			if change.DesiredPath != "" {
				if err := s.unlinkAll([]string{change.Target}, linked[i].Name); err != nil {
					rollbackErr = errors.Join(rollbackErr, err)
					continue
				}
			}
			if change.PreviousPath != "" {
				if err := s.linkAll([]string{change.Target}, linked[i].Name, change.PreviousPath); err != nil {
					rollbackErr = errors.Join(rollbackErr, err)
				}
			}
		}
	}
	if err := restoreLockfile(lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}
