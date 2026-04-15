package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
	"github.com/Z-Bra0/Ski/internal/target"
)

const (
	FindingKindMissingLockEntry     = "missing_lock_entry"
	FindingKindOrphanedLockEntry    = "orphaned_lock_entry"
	FindingKindSourceMismatch       = "source_mismatch"
	FindingKindUpstreamMismatch     = "upstream_mismatch"
	FindingKindTargetsMismatch      = "targets_mismatch"
	FindingKindStoreMissing         = "store_missing"
	FindingKindStoreInvalid         = "store_invalid"
	FindingKindStoreSymlink         = "store_symlink"
	FindingKindStoreIntegrity       = "store_integrity"
	FindingKindMissingTargetInstall = "missing_target_install"
	FindingKindDriftedTarget        = "drifted_target"
	FindingKindUnexpectedTarget     = "unexpected_target"
	FindingKindUnexpectedEntryType  = "unexpected_entry_type"
	FindingKindUnmanagedTarget      = "unmanaged_target"
)

// DoctorFinding describes one inconsistency found by Service.Doctor.
type DoctorFinding struct {
	Kind       string
	Skill      string
	Message    string
	TargetName string
	StorePath  string
}

// String formats a DoctorFinding for CLI display.
func (f DoctorFinding) String() string {
	if f.Skill == "" {
		if f.TargetName != "" {
			return fmt.Sprintf("[%s] %s", f.TargetName, f.Message)
		}
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Skill, f.Message)
}

// Doctor checks for active-scope inconsistencies across the manifest,
// lockfile, store, and installed target directories.
func (s Service) Doctor() ([]DoctorFinding, error) {
	doc, lf, err := s.loadProjectState()
	if err != nil {
		return nil, err
	}

	lockByName := make(map[string]lockfile.Skill, len(lf.Skills))
	for _, skill := range lf.Skills {
		lockByName[skill.Name] = skill
	}

	findings := make([]DoctorFinding, 0)
	manifestNames := make(map[string]struct{}, len(doc.Skills))
	for _, skill := range doc.Skills {
		manifestNames[skill.Name] = struct{}{}

		locked, ok := lockByName[skill.Name]
		if !ok {
			findings = append(findings, DoctorFinding{
				Kind:    FindingKindMissingLockEntry,
				Skill:   skill.Name,
				Message: "missing lockfile entry",
			})
			continue
		}

		findings = append(findings, s.doctorSkillFindings(doc, skill, locked)...)
	}

	for _, skill := range lf.Skills {
		if _, ok := manifestNames[skill.Name]; ok {
			continue
		}
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindOrphanedLockEntry,
			Skill:   skill.Name,
			Message: "lockfile entry exists but skill is not declared in ski.toml",
		})
	}

	findings = append(findings, s.doctorUnmanagedLocalTargets(doc, manifestNames, lockByName)...)

	return findings, nil
}

func (s Service) doctorSkillFindings(doc *manifest.Manifest, skill manifest.Skill, locked lockfile.Skill) []DoctorFinding {
	findings := make([]DoctorFinding, 0)
	expectedTargets := effectiveTargetsForSkill(doc, skill)
	installTargets := installTargetsForSkill(doc, skill)
	targetsToInspect := unionStrings(expectedTargets, locked.Targets)

	manifestSource, manifestUpstream, err := canonicalSkillIdentity(skill.Source, skill.UpstreamSkill)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindSourceMismatch,
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}
	lockSource, lockUpstream, err := canonicalSkillIdentity(locked.Source, locked.UpstreamSkill)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindSourceMismatch,
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}

	if lockSource != manifestSource {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindSourceMismatch,
			Skill:   skill.Name,
			Message: fmt.Sprintf("lockfile source %q does not match manifest source %q", locked.Source, skill.Source),
		})
	}
	if lockUpstream != manifestUpstream {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindUpstreamMismatch,
			Skill:   skill.Name,
			Message: fmt.Sprintf("lockfile upstream skill %q does not match manifest upstream skill %q", locked.UpstreamSkill, skill.UpstreamSkill),
		})
	}
	if !sameStrings(expectedTargets, locked.Targets) {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindTargetsMismatch,
			Skill:   skill.Name,
			Message: fmt.Sprintf("lockfile targets %v do not match manifest targets %v", locked.Targets, expectedTargets),
		})
	}

	src, err := s.loadSkillSourceForScope(locked.Source, locked.UpstreamSkill)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Kind:    FindingKindSourceMismatch,
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}
	stored, err := store.FindGit(s.HomeDir, src, locked.Commit, skill.Name)
	if err != nil {
		findings = append(findings, classifyStoreFinding(skill.Name, err)...)
		return findings
	}
	storePath := stored.Path

	if stored.Integrity != locked.Integrity {
		findings = append(findings, DoctorFinding{
			Kind:      FindingKindStoreIntegrity,
			Skill:     skill.Name,
			Message:   fmt.Sprintf("integrity mismatch: got %s, want %s", stored.Integrity, locked.Integrity),
			StorePath: storePath,
		})
	}

	for _, targetName := range targetsToInspect {
		shouldExist := slices.Contains(installTargets, targetName)
		findings = append(findings, s.doctorTargetFindings(skill.Name, targetName, storePath, shouldExist)...)
	}

	return findings
}

func classifyStoreFinding(skillName string, err error) []DoctorFinding {
	kind := FindingKindStoreInvalid
	storePath := ""
	if errors.Is(err, os.ErrNotExist) {
		kind = FindingKindStoreMissing
	}
	var symlinkErr store.SnapshotSymlinkError
	if errors.As(err, &symlinkErr) {
		kind = FindingKindStoreSymlink
		storePath = symlinkErr.Root
	}
	return []DoctorFinding{{
		Kind:      kind,
		Skill:     skillName,
		Message:   err.Error(),
		StorePath: storePath,
	}}
}

func (s Service) doctorUnmanagedLocalTargets(doc *manifest.Manifest, manifestNames map[string]struct{}, lockByName map[string]lockfile.Skill) []DoctorFinding {
	if s.Global {
		return nil
	}

	targetNames := append([]string(nil), target.BuiltInNames()...)
	targetNames = unionStrings(targetNames, doc.Targets)
	for _, skill := range doc.Skills {
		targetNames = unionStrings(targetNames, skill.Targets)
	}
	for _, locked := range lockByName {
		targetNames = unionStrings(targetNames, locked.Targets)
	}

	findings := make([]DoctorFinding, 0)
	seenDirs := make(map[string]struct{}, len(targetNames))
	for _, targetName := range targetNames {
		dir, err := s.skillDir(targetName)
		if err != nil {
			continue
		}
		if _, ok := seenDirs[dir]; ok {
			continue
		}
		seenDirs[dir] = struct{}{}

		entries, err := os.ReadDir(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			findings = append(findings, DoctorFinding{
				Kind:       FindingKindUnexpectedEntryType,
				Message:    fmt.Sprintf("read %s: %v", dir, err),
				TargetName: targetName,
			})
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillName := entry.Name()
			if _, ok := manifestNames[skillName]; ok {
				continue
			}
			if _, ok := lockByName[skillName]; ok {
				continue
			}
			findings = append(findings, DoctorFinding{
				Kind:       FindingKindUnmanagedTarget,
				Skill:      skillName,
				TargetName: targetName,
				Message:    fmt.Sprintf("unmanaged %s target at %s", targetName, filepath.Join(dir, skillName)),
			})
		}
	}

	return findings
}

func (s Service) doctorTargetFindings(skillName, targetName, storePath string, shouldExist bool) []DoctorFinding {
	expectedPath := ""
	if shouldExist {
		expectedPath = storePath
	}
	inspection, err := s.inspectTarget(targetName, skillName, expectedPath)
	if err != nil {
		return []DoctorFinding{{
			Kind:       FindingKindUnexpectedEntryType,
			Skill:      skillName,
			Message:    err.Error(),
			TargetName: targetName,
			StorePath:  storePath,
		}}
	}

	switch inspection.Status {
	case targetStatusMissing:
		if !shouldExist {
			return nil
		}
		return []DoctorFinding{{
			Kind:       FindingKindMissingTargetInstall,
			Skill:      skillName,
			Message:    fmt.Sprintf("missing %s target at %s", targetName, inspection.Path),
			TargetName: targetName,
			StorePath:  storePath,
		}}
	case targetStatusInstalled:
		if shouldExist {
			return nil
		}
		return []DoctorFinding{{
			Kind:       FindingKindUnexpectedTarget,
			Skill:      skillName,
			Message:    fmt.Sprintf("unexpected %s target at %s", targetName, inspection.Path),
			TargetName: targetName,
			StorePath:  storePath,
		}}
	case targetStatusDrifted:
		if !shouldExist {
			return []DoctorFinding{{
				Kind:       FindingKindUnexpectedTarget,
				Skill:      skillName,
				Message:    fmt.Sprintf("unexpected %s target at %s", targetName, inspection.Path),
				TargetName: targetName,
				StorePath:  storePath,
			}}
		}
		return []DoctorFinding{{
			Kind:       FindingKindDriftedTarget,
			Skill:      skillName,
			Message:    driftedTargetError(inspection.Path).Error(),
			TargetName: targetName,
			StorePath:  storePath,
		}}
	default:
		message := fmt.Sprintf("%s is not a managed skill directory", inspection.Path)
		if !shouldExist {
			message = fmt.Sprintf("unexpected %s entry at %s", targetName, inspection.Path)
		}
		return []DoctorFinding{{
			Kind:       FindingKindUnexpectedEntryType,
			Skill:      skillName,
			Message:    message,
			TargetName: targetName,
			StorePath:  storePath,
		}}
	}
}
