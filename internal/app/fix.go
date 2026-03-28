package app

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
)

// FixResult reports whether one doctor finding was repaired.
type FixResult struct {
	Finding DoctorFinding
	Fixed   bool
	Err     error
	Note    string
}

type fixSkillGroup struct {
	name           string
	findingIndexes []int
}

type fixSkillState struct {
	manifestSkill    manifest.Skill
	hasManifest      bool
	lockEntry        lockfile.Skill
	hasLock          bool
	effectiveTargets []string
	storePath        string
}

// Fix repairs all safe doctor findings best-effort and returns one result per
// input finding. It writes the updated lockfile once at the end.
func (s Service) Fix(findings []DoctorFinding) ([]FixResult, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	lockPath := s.lockPath()
	results := make([]FixResult, len(findings))
	for i, finding := range findings {
		results[i] = FixResult{Finding: finding}
	}

	nextLock := cloneLockfile(*lf)
	groups := groupFixFindings(findings)
	lockChanged := false
	for _, group := range groups {
		changed, err := s.fixSkillGroup(doc, &nextLock, findings, results, group)
		if err != nil {
			return results, err
		}
		lockChanged = lockChanged || changed
	}

	if !lockChanged {
		return results, nil
	}
	if err := ensureParentDir(lockPath); err != nil {
		return results, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		return results, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return results, nil
}

func groupFixFindings(findings []DoctorFinding) []fixSkillGroup {
	order := make([]fixSkillGroup, 0)
	indexBySkill := make(map[string]int)
	for i, finding := range findings {
		idx, ok := indexBySkill[finding.Skill]
		if !ok {
			indexBySkill[finding.Skill] = len(order)
			order = append(order, fixSkillGroup{name: finding.Skill, findingIndexes: []int{i}})
			continue
		}
		order[idx].findingIndexes = append(order[idx].findingIndexes, i)
	}
	return order
}

func (s Service) fixSkillGroup(doc *manifest.Manifest, nextLock *lockfile.Lockfile, findings []DoctorFinding, results []FixResult, group fixSkillGroup) (bool, error) {
	state := buildFixSkillState(doc, nextLock, group.name)
	lockChanged := false

	if state.hasManifest {
		changed, err := s.repairLockEntry(state, nextLock, findings, results, group.findingIndexes)
		if err != nil {
			return lockChanged, err
		}
		lockChanged = lockChanged || changed
		state = buildFixSkillState(doc, nextLock, group.name)
	} else {
		changed := s.repairOrphanedLockEntry(state, nextLock, findings, results, group.findingIndexes)
		lockChanged = lockChanged || changed
		return lockChanged, nil
	}

	state = buildFixSkillState(doc, nextLock, group.name)
	if state.hasLock {
		changed, err := s.repairStoreAndTargets(state, nextLock, findings, results, group.findingIndexes)
		if err != nil {
			return lockChanged, err
		}
		lockChanged = lockChanged || changed
	}

	return lockChanged, nil
}

func buildFixSkillState(doc *manifest.Manifest, lf *lockfile.Lockfile, skillName string) fixSkillState {
	state := fixSkillState{}
	if skill, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == skillName }); ok {
		state.manifestSkill = skill
		state.hasManifest = true
		state.effectiveTargets = effectiveTargetsForSkill(doc, skill)
	}
	if lockEntry, ok := findLockSkill(lf.Skills, skillName); ok {
		state.lockEntry = lockEntry
		state.hasLock = true
	}
	return state
}

func (s Service) repairOrphanedLockEntry(state fixSkillState, nextLock *lockfile.Lockfile, findings []DoctorFinding, results []FixResult, indexes []int) bool {
	changed := false
	for _, idx := range indexes {
		if findings[idx].Kind != FindingKindOrphanedLockEntry {
			continue
		}
		if state.hasLock {
			for _, targetName := range state.lockEntry.Targets {
				if err := s.removeAll([]string{targetName}, findings[idx].Skill); err != nil {
					results[idx].Err = err
					results[idx].Note = "failed to remove orphaned target installs; lockfile entry unchanged"
					continue
				}
			}
			if results[idx].Err != nil {
				continue
			}
		}
		nextLock.Skills = removeByName(nextLock.Skills, findings[idx].Skill, func(skill lockfile.Skill) string { return skill.Name })
		results[idx].Fixed = true
		results[idx].Note = "removed orphaned lockfile entry and target installs"
		changed = true
	}
	return changed
}

func (s Service) repairLockEntry(state fixSkillState, nextLock *lockfile.Lockfile, findings []DoctorFinding, results []FixResult, indexes []int) (bool, error) {
	lockChanged := false

	if hasFinding(findings, indexes, FindingKindMissingLockEntry) {
		stored, err := s.ensureManifestSkillStored(state.manifestSkill)
		if err != nil {
			for _, idx := range indexes {
				if findings[idx].Kind == FindingKindMissingLockEntry {
					results[idx].Err = err
				}
			}
			return lockChanged, nil
		}
		lockEntry, err := buildLockSkill(state.manifestSkill, stored, state.effectiveTargets)
		if err != nil {
			for _, idx := range indexes {
				if findings[idx].Kind == FindingKindMissingLockEntry {
					results[idx].Err = err
				}
			}
			return lockChanged, nil
		}
		upsertLockSkill(nextLock, lockEntry)
		for _, idx := range indexes {
			if findings[idx].Kind == FindingKindMissingLockEntry {
				results[idx].Fixed = true
				results[idx].Note = "recreated lockfile entry"
			}
		}
		return true, nil
	}

	if !state.hasLock {
		return false, nil
	}

	lockEntry := state.lockEntry
	changed := false
	for _, idx := range indexes {
		switch findings[idx].Kind {
		case FindingKindSourceMismatch, FindingKindUpstreamMismatch:
			sourceValue, upstreamSkill, err := canonicalSkillIdentity(state.manifestSkill.Source, state.manifestSkill.UpstreamSkill)
			if err != nil {
				results[idx].Err = err
				continue
			}
			if lockEntry.Source != sourceValue || lockEntry.UpstreamSkill != upstreamSkill {
				lockEntry.Source = sourceValue
				lockEntry.UpstreamSkill = upstreamSkill
				changed = true
			}
			results[idx].Fixed = true
			results[idx].Note = "updated lockfile source identity"
		}
	}
	if changed {
		upsertLockSkill(nextLock, lockEntry)
		lockChanged = true
	}

	return lockChanged, nil
}

func (s Service) repairStoreAndTargets(state fixSkillState, nextLock *lockfile.Lockfile, findings []DoctorFinding, results []FixResult, indexes []int) (bool, error) {
	lockChanged := false
	lockEntry := state.lockEntry

	needsRefresh := hasAnyFinding(findings, indexes,
		FindingKindStoreMissing,
		FindingKindStoreInvalid,
		FindingKindStoreIntegrity,
	)

	var stored store.Result
	var err error
	if needsRefresh {
		stored, err = s.refreshLockedSkill(lockEntry, state.manifestSkill.Name)
		if err != nil {
			for _, idx := range indexes {
				switch findings[idx].Kind {
				case FindingKindStoreMissing, FindingKindStoreInvalid, FindingKindStoreIntegrity:
					results[idx].Err = err
				}
			}
			return lockChanged, nil
		}
		lockEntry.Commit = stored.Commit
		lockEntry.Integrity = stored.Integrity
		upsertLockSkill(nextLock, lockEntry)
		lockChanged = true
		for _, idx := range indexes {
			switch findings[idx].Kind {
			case FindingKindStoreMissing:
				results[idx].Fixed = true
				results[idx].Note = "refetched missing store snapshot"
			case FindingKindStoreInvalid:
				results[idx].Fixed = true
				results[idx].Note = "refreshed invalid store snapshot"
			case FindingKindStoreIntegrity:
				results[idx].Fixed = true
				results[idx].Note = "refreshed store snapshot and updated integrity"
			}
		}
	} else {
		stored, err = s.findLockedSkill(lockEntry, state.manifestSkill.Name)
		if err != nil {
			for _, idx := range indexes {
				switch findings[idx].Kind {
				case FindingKindMissingTargetInstall, FindingKindDriftedTarget, FindingKindUnexpectedTarget:
					results[idx].Err = err
				}
			}
			return lockChanged, nil
		}
	}
	state.storePath = stored.Path

	targets := collectTargetNames(state.effectiveTargets, findings, indexes)
	for _, targetName := range targets {
		if err := s.repairTarget(state, findings, results, indexes, targetName); err != nil {
			return lockChanged, err
		}
	}

	if hasFinding(findings, indexes, FindingKindTargetsMismatch) {
		if canFinalizeTargetsMismatch(findings, results, indexes) {
			lockEntry.Targets = append([]string(nil), state.effectiveTargets...)
			upsertLockSkill(nextLock, lockEntry)
			lockChanged = true
			for _, idx := range indexes {
				if findings[idx].Kind != FindingKindTargetsMismatch {
					continue
				}
				results[idx].Fixed = true
				results[idx].Note = "updated lockfile targets"
			}
		} else {
			for _, idx := range indexes {
				if findings[idx].Kind != FindingKindTargetsMismatch {
					continue
				}
				results[idx].Note = "target cleanup did not complete; lockfile targets unchanged"
			}
		}
	}

	return lockChanged, nil
}

func collectTargetNames(expectedTargets []string, findings []DoctorFinding, indexes []int) []string {
	targets := append([]string(nil), expectedTargets...)
	for _, idx := range indexes {
		targetName := findings[idx].TargetName
		if targetName == "" {
			continue
		}
		if slices.Contains(targets, targetName) {
			continue
		}
		targets = append(targets, targetName)
	}
	return targets
}

func (s Service) repairTarget(state fixSkillState, findings []DoctorFinding, results []FixResult, indexes []int, targetName string) error {
	shouldExist := slices.Contains(state.effectiveTargets, targetName)
	expectedPath := ""
	if shouldExist {
		expectedPath = state.storePath
	}
	inspection, err := s.inspectTarget(targetName, state.manifestSkill.Name, expectedPath)
	if err != nil {
		markTargetFindingError(results, findings, indexes, targetName, err)
		return nil
	}

	switch inspection.Status {
	case targetStatusMissing:
		if !shouldExist {
			return nil
		}
		if err := s.materializeAll([]string{targetName}, state.manifestSkill.Name, state.storePath); err != nil {
			markTargetFindingError(results, findings, indexes, targetName, err)
			return nil
		}
		markTargetFindingFixed(results, findings, indexes, targetName, FindingKindMissingTargetInstall, fmt.Sprintf("materialized %s target", targetName))
	case targetStatusInstalled:
		if shouldExist {
			return nil
		}
		if err := s.removeAll([]string{targetName}, state.manifestSkill.Name); err != nil {
			markTargetFindingError(results, findings, indexes, targetName, err)
			return nil
		}
		markTargetFindingFixed(results, findings, indexes, targetName, FindingKindUnexpectedTarget, fmt.Sprintf("removed unexpected %s target", targetName))
	case targetStatusDrifted:
		if !shouldExist {
			if err := s.removeAll([]string{targetName}, state.manifestSkill.Name); err != nil {
				markTargetFindingError(results, findings, indexes, targetName, err)
				return nil
			}
			markTargetFindingFixed(results, findings, indexes, targetName, FindingKindUnexpectedTarget, fmt.Sprintf("removed unexpected %s target", targetName))
			return nil
		}
		if err := s.replaceTarget(targetName, state.manifestSkill.Name, state.storePath); err != nil {
			markTargetFindingError(results, findings, indexes, targetName, err)
			return nil
		}
		markTargetFindingFixed(results, findings, indexes, targetName, FindingKindDriftedTarget, fmt.Sprintf("replaced drifted %s target", targetName))
	default:
		markTargetFindingNote(results, findings, indexes, targetName, FindingKindUnexpectedEntryType, "manual intervention required")
	}

	return nil
}

func markTargetFindingFixed(results []FixResult, findings []DoctorFinding, indexes []int, targetName, kind, note string) {
	for _, idx := range indexes {
		if findings[idx].TargetName != targetName || findings[idx].Kind != kind {
			continue
		}
		results[idx].Fixed = true
		results[idx].Note = note
	}
}

func markTargetFindingError(results []FixResult, findings []DoctorFinding, indexes []int, targetName string, err error) {
	for _, idx := range indexes {
		if findings[idx].TargetName != targetName {
			continue
		}
		switch findings[idx].Kind {
		case FindingKindMissingTargetInstall, FindingKindDriftedTarget, FindingKindUnexpectedTarget:
			results[idx].Err = err
		}
	}
}

func markTargetFindingNote(results []FixResult, findings []DoctorFinding, indexes []int, targetName, kind, note string) {
	for _, idx := range indexes {
		if findings[idx].TargetName != targetName || findings[idx].Kind != kind {
			continue
		}
		results[idx].Note = note
	}
}

func canFinalizeTargetsMismatch(findings []DoctorFinding, results []FixResult, indexes []int) bool {
	for _, idx := range indexes {
		switch findings[idx].Kind {
		case FindingKindMissingTargetInstall, FindingKindDriftedTarget, FindingKindUnexpectedTarget, FindingKindUnexpectedEntryType:
			if findings[idx].TargetName == "" {
				continue
			}
			if results[idx].Err != nil {
				return false
			}
			if !results[idx].Fixed {
				return false
			}
		}
	}
	return true
}

func hasFinding(findings []DoctorFinding, indexes []int, kind string) bool {
	for _, idx := range indexes {
		if findings[idx].Kind == kind {
			return true
		}
	}
	return false
}

func hasAnyFinding(findings []DoctorFinding, indexes []int, kinds ...string) bool {
	for _, kind := range kinds {
		if hasFinding(findings, indexes, kind) {
			return true
		}
	}
	return false
}

func (s Service) ensureManifestSkillStored(skill manifest.Skill) (store.Result, error) {
	src, err := s.loadSkillSourceForScope(skill.Source, skill.UpstreamSkill)
	if err != nil {
		return store.Result{}, err
	}
	return store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, skill.Name)
}

func (s Service) findLockedSkill(lockEntry lockfile.Skill, skillName string) (store.Result, error) {
	src, err := s.loadSkillSourceForScope(lockEntry.Source, lockEntry.UpstreamSkill)
	if err != nil {
		return store.Result{}, err
	}
	return store.FindGit(s.HomeDir, src, lockEntry.Commit, skillName)
}

func (s Service) refreshLockedSkill(lockEntry lockfile.Skill, skillName string) (store.Result, error) {
	src, err := s.loadSkillSourceForScope(lockEntry.Source, lockEntry.UpstreamSkill)
	if err != nil {
		return store.Result{}, err
	}
	src.Ref = lockEntry.Commit
	repo, err := store.RefreshGit(s.sourceResolveDir(), s.HomeDir, src)
	if err != nil {
		return store.Result{}, err
	}
	selected, err := store.FindGit(s.HomeDir, src, repo.Commit, skillName)
	if err != nil {
		return store.Result{}, err
	}
	return selected, nil
}
