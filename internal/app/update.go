package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"ski/internal/lockfile"
	"ski/internal/source"
	"ski/internal/store"
)

type UpdateInfo struct {
	Name          string
	CurrentCommit string
	LatestCommit  string
}

func (s Service) CheckUpdates(name string) ([]UpdateInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	selected, err := selectSkills(doc, name)
	if err != nil {
		return nil, err
	}

	updates := make([]UpdateInfo, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSourceForScope(mSkill.Source)
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

	selected, err := selectSkills(doc, name)
	if err != nil {
		return nil, err
	}

	updates := make([]UpdateInfo, 0, len(selected))
	for _, mSkill := range selected {
		src, err := s.loadSourceForScope(mSkill.Source)
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
		if hasLock {
			if err := s.unlinkAll(unionStrings(targets, locked.Targets), mSkill.Name); err != nil {
				return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
			}
		}
		if err := s.linkAll(targets, mSkill.Name, stored.Path); err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		upsertLockSkill(lf, lockfile.Skill{
			Name:      mSkill.Name,
			Source:    mSkill.Source,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   targets,
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

	lockPath := s.lockPath()
	if err := ensureParentDir(lockPath); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return updates, nil
}

func resolveUpdateCommit(projectDir string, src source.Git) (string, bool, error) {
	commit, err := source.ResolveGit(projectDir, src)
	if err == nil {
		return commit, false, nil
	}
	if src.Ref != "" && source.IsCommitRef(src.Ref) && strings.Contains(err.Error(), "no matching revision found") {
		return "", true, nil
	}
	return "", false, err
}
