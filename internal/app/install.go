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
	Name            string
	Targets         []string
	StorePath       string
	Lock            lockfile.Skill
	TargetsToCreate []string
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
		targetsToCreate, err := s.preflightInstallLinks(effectiveTargets, mSkill.Name, stored.Path)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockEntry := lockfile.Skill{
			Name:      mSkill.Name,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   effectiveTargets,
		}
		lockEntry.Source, lockEntry.UpstreamSkill, err = canonicalSkillIdentity(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return 0, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		upsertLockSkill(&nextLock, lockEntry)
		plans = append(plans, plannedInstall{
			Name:            mSkill.Name,
			Targets:         effectiveTargets,
			StorePath:       stored.Path,
			Lock:            lockEntry,
			TargetsToCreate: targetsToCreate,
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
		createdTargets := make([]string, 0, len(plan.TargetsToCreate))
		for _, targetName := range plan.TargetsToCreate {
			if err := s.linkAll([]string{targetName}, plan.Name, plan.StorePath); err != nil {
				plan.TargetsToCreate = createdTargets
				rollbackErr := s.rollbackInstall(linked, lockPath, originalLockData, hadLockfile)
				if rollbackErr != nil {
					return 0, fmt.Errorf("skill %q: %w (rollback failed: %v)", plan.Name, err, rollbackErr)
				}
				return 0, fmt.Errorf("skill %q: %w", plan.Name, err)
			}
			createdTargets = append(createdTargets, targetName)
		}
		plan.TargetsToCreate = createdTargets
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
		if len(linked[i].TargetsToCreate) == 0 {
			continue
		}
		if err := s.unlinkAll(linked[i].TargetsToCreate, linked[i].Name); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if err := restoreLockfile(lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}
