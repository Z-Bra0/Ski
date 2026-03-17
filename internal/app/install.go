package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/store"
)

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
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	count := 0
	for _, mSkill := range doc.Skills {
		src, err := s.loadSourceForScope(mSkill.Source)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockedEntry, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock {
			src.Ref = lockedEntry.Commit
		}

		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		if hasLock && stored.Integrity != lockedEntry.Integrity {
			return count, fmt.Errorf("skill %q: integrity mismatch: got %s, want %s",
				mSkill.Name, stored.Integrity, lockedEntry.Integrity)
		}

		effectiveTargets := effectiveTargetsForSkill(doc, mSkill)
		if err := s.linkAll(effectiveTargets, mSkill.Name, stored.Path); err != nil {
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

	if err := ensureParentDir(lockPath); err != nil {
		return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return count, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return count, nil
}
