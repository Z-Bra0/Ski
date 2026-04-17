package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/store"
)

var resolveGitCommit = source.ResolveGit
var resolveGitInfo = source.ResolveGitInfo

// UpdateInfo reports the current and latest commit for one skill update check.
type UpdateInfo struct {
	Name          string
	Tracking      string
	CurrentCommit string
	LatestCommit  string
	LatestAt      string
}

type updateTargetChange struct {
	Target       string
	PreviousPath string
	DesiredPath  string
	BackupPath   string // temp backup of original content; set after destructive force apply
	ForceRemove  bool   // remove even without a known PreviousPath (no lockfile entry)
	ForceReplace bool   // replace even without a known PreviousPath (directory exists, origin unknown)
}

// CheckUpdates reports which selected skills have newer upstream commits available.
func (s Service) CheckUpdates(name string) ([]UpdateInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	selected, err := selectSkills(doc, name, s.manifestPath())
	if err != nil {
		return nil, err
	}

	lockByName := make(map[string]lockfile.Skill, len(lf.Skills))
	for _, ls := range lf.Skills {
		lockByName[ls.Name] = ls
	}

	updates := make([]UpdateInfo, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		resolved, pinned, err := resolveUpdateInfo(s.sourceResolveDir(), src)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		if pinned {
			continue
		}

		currentCommit := ""
		if locked, ok := lockByName[mSkill.Name]; ok {
			currentCommit = locked.Commit
		}

		if currentCommit == resolved.Commit {
			continue
		}

		updates = append(updates, UpdateInfo{
			Name:          mSkill.Name,
			Tracking:      resolved.Tracking,
			CurrentCommit: currentCommit,
			LatestCommit:  resolved.Commit,
			LatestAt:      resolved.LatestAt,
		})
	}

	return updates, nil
}

// Update resolves and installs newer commits for the selected skills.
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

	lockByName := make(map[string]lockfile.Skill, len(lf.Skills))
	for _, ls := range lf.Skills {
		lockByName[ls.Name] = ls
	}

	nextLock := cloneLockfile(*lf)
	updates := make([]UpdateInfo, 0, len(selected))
	plans := make([]plannedTargetChanges, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSkillSourceForScope(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		resolved, pinned, err := resolveUpdateInfo(s.sourceResolveDir(), src)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		if pinned {
			continue
		}

		locked, hasLock := lockByName[mSkill.Name]
		if hasLock && locked.Commit == resolved.Commit {
			continue
		}

		previousStorePath := ""
		previousTargets := []string(nil)
		if hasLock {
			previousTargets = append(previousTargets, locked.Targets...)
			currentSrc := src
			currentSrc.Ref = locked.Commit
			previousStored, err := store.FindGit(s.HomeDir, currentSrc, locked.Commit, mSkill.Name)
			if err != nil {
				return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
			}
			previousStorePath = previousStored.Path
		}

		src.Ref = resolved.Commit
		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		targets := effectiveTargetsForSkill(doc, mSkill)
		desiredTargets := installTargetsForSkill(doc, mSkill)
		changes, err := s.planUpdateTargetChanges(mSkill.Name, desiredTargets, previousTargets, previousStorePath, stored.Path)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		lockEntry := lockfile.Skill{
			Name:      mSkill.Name,
			Version:   mSkill.Version,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   targets,
		}
		lockEntry.Source, lockEntry.UpstreamSkill, err = canonicalSkillIdentity(mSkill.Source, mSkill.UpstreamSkill)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		upsertLockSkill(&nextLock, lockEntry)
		plans = append(plans, plannedTargetChanges{
			Name:    mSkill.Name,
			Changes: changes,
		})
		updates = append(updates, UpdateInfo{
			Name:          mSkill.Name,
			Tracking:      resolved.Tracking,
			CurrentCommit: locked.Commit,
			LatestCommit:  stored.Commit,
			LatestAt:      resolved.LatestAt,
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

	applied := make([]plannedTargetChanges, 0, len(plans))
	for _, plan := range plans {
		appliedPlan, failure := s.applyTargetChangePlan(plan, applied, func(applied []plannedTargetChanges) error {
			return s.rollbackUpdate(applied, lockPath, originalLockData, hadLockfile)
		})
		if failure != nil {
			return nil, formatTargetChangeFailure(fmt.Sprintf("skill %q: ", failure.Name), failure)
		}
		applied = append(applied, appliedPlan)
	}

	cleanupTargetChangePlanBackups(applied)
	return updates, nil
}

func (s Service) planUpdateTargetChanges(skillName string, desiredTargets []string, previousTargets []string, previousPath, desiredPath string) ([]updateTargetChange, error) {
	targetsToInspect := unionStrings(desiredTargets, previousTargets)

	changes := make([]updateTargetChange, 0, len(targetsToInspect))
	for _, targetName := range targetsToInspect {
		hadPrevious := slices.Contains(previousTargets, targetName)
		expectedCurrentPath := desiredPath
		if hadPrevious {
			expectedCurrentPath = previousPath
		} else if previousPath == "" {
			// No lock entry at all — skip drift check on existing installs.
			expectedCurrentPath = ""
		}

		inspection, err := s.inspectTarget(targetName, skillName, expectedCurrentPath)
		if err != nil {
			return nil, err
		}
		shouldExist := slices.Contains(desiredTargets, targetName)
		nextPath := ""
		currentPath := ""
		if shouldExist {
			nextPath = desiredPath
		}

		switch inspection.Status {
		case targetStatusMissing:
			if !shouldExist {
				continue
			}
		case targetStatusInstalled:
			currentPath = expectedCurrentPath
			if currentPath == nextPath {
				continue
			}
			if currentPath == "" && nextPath != "" {
				// Directory exists but origin unknown (no lockfile) — replace it.
				changes = append(changes, updateTargetChange{
					Target:       targetName,
					DesiredPath:  nextPath,
					ForceReplace: true,
				})
				continue
			}
		case targetStatusDrifted:
			return nil, driftedTargetError(inspection.Path)
		default:
			return nil, unexpectedTargetEntryError(inspection.Path)
		}

		changes = append(changes, updateTargetChange{
			Target:       targetName,
			PreviousPath: currentPath,
			DesiredPath:  nextPath,
		})
	}

	return changes, nil
}

// applyUpdateTargetChange applies a single target change and returns the path
// to a temporary backup of the original content when the change is a forced
// operation (ForceRemove or ForceReplace). Callers must clean up the backup
// on success or use it to restore the original on rollback.
func (s Service) applyUpdateTargetChange(name string, change updateTargetChange) (backupPath string, err error) {
	if change.ForceRemove {
		backupPath, err = s.backupTarget(change.Target, name)
		if err != nil {
			return "", fmt.Errorf("backup before force-remove: %w", err)
		}
		if err := s.removeAll([]string{change.Target}, name); err != nil {
			os.RemoveAll(backupPath)
			return "", err
		}
		return backupPath, nil
	}
	if change.ForceReplace && change.DesiredPath != "" {
		backupPath, err = s.backupTarget(change.Target, name)
		if err != nil {
			return "", fmt.Errorf("backup before force-replace: %w", err)
		}
		if err := s.replaceTarget(change.Target, name, change.DesiredPath); err != nil {
			os.RemoveAll(backupPath)
			return "", err
		}
		return backupPath, nil
	}
	if change.PreviousPath == change.DesiredPath {
		return "", nil
	}

	switch {
	case change.PreviousPath == "" && change.DesiredPath != "":
		return "", s.materializeAll([]string{change.Target}, name, change.DesiredPath)
	case change.PreviousPath != "" && change.DesiredPath == "":
		return "", s.removeAll([]string{change.Target}, name)
	case change.PreviousPath != "" && change.DesiredPath != "":
		return "", s.replaceTarget(change.Target, name, change.DesiredPath)
	default:
		return "", nil
	}
}

func reverseUpdateTargetChange(change updateTargetChange) updateTargetChange {
	if change.BackupPath != "" {
		// A backup of the original was captured before the force operation.
		// Restore from the backup: the current on-disk content is what was
		// written by the forward apply (DesiredPath), and the backup holds
		// what should be put back.
		return updateTargetChange{
			Target:       change.Target,
			PreviousPath: change.DesiredPath,
			DesiredPath:  change.BackupPath,
		}
	}
	return updateTargetChange{
		Target:       change.Target,
		PreviousPath: change.DesiredPath,
		DesiredPath:  change.PreviousPath,
	}
}

func (s Service) rollbackUpdate(applied []plannedTargetChanges, lockPath string, lockData []byte, hadLockfile bool) error {
	rollbackErr := s.rollbackTargetChangePlans(applied)
	cleanupTargetChangePlanBackups(applied)
	if err := restoreLockfile(lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}

func resolveUpdateInfo(projectDir string, src source.Git) (source.ResolveInfo, bool, error) {
	info, err := resolveGitInfo(projectDir, src)
	if err == nil {
		if info.Tracking == "" {
			info.Tracking = fallbackUpdateTracking(src)
		}
		return info, info.Pinned, nil
	}
	if src.Ref != "" && source.IsCommitRef(src.Ref) && source.IsNoMatchingRevision(err) {
		return source.ResolveInfo{}, true, nil
	}

	commit, commitErr := resolveGitCommit(projectDir, src)
	if commitErr != nil {
		return source.ResolveInfo{}, false, commitErr
	}

	return source.ResolveInfo{
		Commit:   commit,
		Tracking: fallbackUpdateTracking(src),
		LatestAt: "",
	}, false, nil
}

func fallbackUpdateTracking(src source.Git) string {
	if src.Ref != "" {
		return src.Ref
	}
	return "HEAD"
}
