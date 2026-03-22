package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
)

// TargetLinkInfo describes one target directory entry for a skill.
type TargetLinkInfo struct {
	Name        string
	Path        string
	Status      string
	CurrentPath string
}

// DetailedSkillInfo reports the resolved state for one declared skill.
type DetailedSkillInfo struct {
	Name          string
	Source        string
	UpstreamSkill string
	Version       string
	Commit        string
	Integrity     string
	StorePath     string
	StoreError    string
	Targets       []TargetLinkInfo
}

// Info returns detailed manifest, lockfile, store, and target-link state for one skill.
func (s Service) Info(name string) (DetailedSkillInfo, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return DetailedSkillInfo{}, err
	}

	skill, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return DetailedSkillInfo{}, fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}

	sourceValue, upstreamSkill, err := canonicalSkillIdentity(skill.Source, skill.UpstreamSkill)
	if err != nil {
		return DetailedSkillInfo{}, err
	}

	info := DetailedSkillInfo{
		Name:          skill.Name,
		Source:        sourceValue,
		UpstreamSkill: upstreamSkill,
		Version:       skill.Version,
	}

	targets := effectiveTargetsForSkill(doc, skill)
	info.Targets = make([]TargetLinkInfo, 0, len(targets))

	lockEntry, hasLock := findLockSkill(lf.Skills, skill.Name)
	if hasLock {
		info.Commit = lockEntry.Commit
		info.Integrity = lockEntry.Integrity

		src, err := s.loadSkillSourceForScope(lockEntry.Source, lockEntry.UpstreamSkill)
		if err != nil {
			return DetailedSkillInfo{}, err
		}
		stored, err := store.FindGit(s.HomeDir, src, lockEntry.Commit, skill.Name)
		if err == nil {
			info.StorePath = stored.Path
			if info.Integrity == "" {
				info.Integrity = stored.Integrity
			}
		} else {
			info.StoreError = err.Error()
		}
	}

	for _, targetName := range targets {
		targetInfo, err := s.inspectTargetLink(doc, targetName, skill.Name, info.StorePath, info.StoreError)
		if err != nil {
			return DetailedSkillInfo{}, err
		}
		info.Targets = append(info.Targets, targetInfo)
	}

	return info, nil
}

func (s Service) inspectTargetLink(doc *manifest.Manifest, targetName, skillName, expectedStorePath, storeError string) (TargetLinkInfo, error) {
	dir, err := s.skillDirForManifest(doc, targetName)
	if err != nil {
		return TargetLinkInfo{}, err
	}

	linkPath := filepath.Join(dir, skillName)
	info := TargetLinkInfo{
		Name: targetName,
		Path: linkPath,
	}

	entry, err := os.Lstat(linkPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if storeError != "" {
			info.Status = "store unavailable"
		} else if expectedStorePath == "" {
			info.Status = "not linked"
		} else {
			info.Status = "missing"
		}
		return info, nil
	case err != nil:
		return TargetLinkInfo{}, fmt.Errorf("lstat %s: %w", linkPath, err)
	case entry.Mode()&os.ModeSymlink == 0:
		info.Status = "not a symlink"
		return info, nil
	}

	currentPath, err := os.Readlink(linkPath)
	if err != nil {
		return TargetLinkInfo{}, fmt.Errorf("readlink %s: %w", linkPath, err)
	}
	info.CurrentPath = currentPath

	switch {
	case storeError != "":
		info.Status = "store unavailable"
	case expectedStorePath == "":
		info.Status = "linked"
	case currentPath == expectedStorePath:
		info.Status = "linked"
	default:
		info.Status = "drifted"
	}

	return info, nil
}
