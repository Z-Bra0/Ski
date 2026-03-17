package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/source"
	"ski/internal/store"
)

type UpdateInfo struct {
	Name          string
	CurrentCommit string
	LatestCommit  string
}

type plannedUpdate struct {
	Name    string
	Changes []updateTargetChange
}

type updateTargetChange struct {
	Target       string
	PreviousPath string
	DesiredPath  string
}

func (s Service) CheckUpdates(name string) ([]UpdateInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	selected, err := selectSkills(doc, name, s.manifestPath())
	if err != nil {
		return nil, err
	}

	updates := make([]UpdateInfo, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.sourceResolveDir(), src)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		if pinned {
			continue
		}

		currentCommit := ""
		if locked, ok := findLockSkill(lf.Skills, mSkill.Name); ok {
			currentCommit = locked.Commit
		}

		if currentCommit == latestCommit {
			continue
		}

		updates = append(updates, UpdateInfo{
			Name:          mSkill.Name,
			CurrentCommit: currentCommit,
			LatestCommit:  latestCommit,
		})
	}

	return updates, nil
}

func (s Service) Update(name string) ([]UpdateInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", lockPath, err)
	}

	selected, err := selectSkills(doc, name, s.manifestPath())
	if err != nil {
		return nil, err
	}

	nextLock := cloneLockfile(*lf)
	updates := make([]UpdateInfo, 0, len(selected))
	plans := make([]plannedUpdate, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.sourceResolveDir(), src)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		if pinned {
			continue
		}

		locked, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock && locked.Commit == latestCommit {
			continue
		}

		src.Ref = latestCommit
		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		targets := effectiveTargetsForSkill(doc, mSkill)
		changes, err := s.planUpdateTargetChanges(mSkill, targets, locked, hasLock, stored.Path)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockEntry := lockfile.Skill{
			Name:      mSkill.Name,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   targets,
		}
		lockEntry.Source, lockEntry.UpstreamSkill, err = canonicalSkillIdentity(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		upsertLockSkill(&nextLock, lockEntry)
		plans = append(plans, plannedUpdate{
			Name:    mSkill.Name,
			Changes: changes,
		})
		updates = append(updates, UpdateInfo{
			Name:          mSkill.Name,
			CurrentCommit: locked.Commit,
			LatestCommit:  stored.Commit,
		})
	}

	if len(updates) == 0 {
		return updates, nil
	}

	if err := ensureParentDir(lockPath); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		return nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	applied := make([]plannedUpdate, 0, len(plans))
	for _, plan := range plans {
		appliedCount := 0
		for _, change := range plan.Changes {
			if err := s.applyUpdateTargetChange(plan.Name, change); err != nil {
				rollbackPlans := append([]plannedUpdate(nil), applied...)
				if appliedCount > 0 {
					rollbackPlans = append(rollbackPlans, plannedUpdate{
						Name:    plan.Name,
						Changes: append([]updateTargetChange(nil), plan.Changes[:appliedCount]...),
					})
				}
				rollbackErr := s.rollbackUpdate(rollbackPlans, lockPath, originalLockData, hadLockfile)
				if rollbackErr != nil {
					return nil, fmt.Errorf("skill %q: %w (rollback failed: %v)", plan.Name, err, rollbackErr)
				}
				return nil, fmt.Errorf("skill %q: %w", plan.Name, err)
			}
			appliedCount++
		}
		plan.Changes = plan.Changes[:appliedCount]
		applied = append(applied, plan)
	}

	return updates, nil
}

func (s Service) planUpdateTargetChanges(skill manifest.Skill, targets []string, locked lockfile.Skill, hasLock bool, desiredPath string) ([]updateTargetChange, error) {
	targetsToInspect := append([]string(nil), targets...)
	if hasLock {
		targetsToInspect = unionStrings(targetsToInspect, locked.Targets)
	}

	changes := make([]updateTargetChange, 0, len(targetsToInspect))
	for _, targetName := range targetsToInspect {
		dir, err := s.skillDir(targetName)
		if err != nil {
			return nil, err
		}

		linkPath := filepath.Join(dir, skill.Name)
		previousPath, err := readExistingSymlink(linkPath)
		if err != nil {
			return nil, err
		}

		nextPath := ""
		if slices.Contains(targets, targetName) {
			nextPath = desiredPath
		}
		if previousPath == nextPath {
			continue
		}

		changes = append(changes, updateTargetChange{
			Target:       targetName,
			PreviousPath: previousPath,
			DesiredPath:  nextPath,
		})
	}

	return changes, nil
}

func readExistingSymlink(linkPath string) (string, error) {
	info, err := os.Lstat(linkPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("lstat %s: %w", linkPath, err)
	case info.Mode()&os.ModeSymlink == 0:
		return "", fmt.Errorf("%s already exists and is not a symlink", linkPath)
	}

	current, err := os.Readlink(linkPath)
	if err != nil {
		return "", fmt.Errorf("readlink %s: %w", linkPath, err)
	}
	return current, nil
}

func (s Service) applyUpdateTargetChange(name string, change updateTargetChange) error {
	if change.PreviousPath == change.DesiredPath {
		return nil
	}

	if change.PreviousPath != "" {
		if err := s.unlinkAll([]string{change.Target}, name); err != nil {
			return err
		}
	}
	if change.DesiredPath == "" {
		return nil
	}

	if err := s.linkAll([]string{change.Target}, name, change.DesiredPath); err != nil {
		if change.PreviousPath == "" {
			return err
		}
		restoreErr := s.linkAll([]string{change.Target}, name, change.PreviousPath)
		if restoreErr != nil {
			return fmt.Errorf("%w (restore failed: %v)", err, restoreErr)
		}
		return err
	}
	return nil
}

func (s Service) rollbackUpdate(applied []plannedUpdate, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		for j := len(applied[i].Changes) - 1; j >= 0; j-- {
			change := applied[i].Changes[j]
			if change.PreviousPath == change.DesiredPath {
				continue
			}
			if change.DesiredPath != "" {
				if err := s.unlinkAll([]string{change.Target}, applied[i].Name); err != nil {
					rollbackErr = errors.Join(rollbackErr, err)
					continue
				}
			}
			if change.PreviousPath != "" {
				if err := s.linkAll([]string{change.Target}, applied[i].Name, change.PreviousPath); err != nil {
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

func resolveUpdateCommit(projectDir string, src source.Git) (string, bool, error) {
	commit, err := source.ResolveGit(projectDir, src)
	if err == nil {
		return commit, false, nil
	}
	if src.Ref != "" && source.IsCommitRef(src.Ref) && source.IsNoMatchingRevision(err) {
		return "", true, nil
	}
	return "", false, err
}
