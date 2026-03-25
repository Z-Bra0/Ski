package app

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/store"
)

const (
	FindingKindMissingLockEntry     = "missing_lock_entry"
	FindingKindOrphanedLockEntry    = "orphaned_lock_entry"
	FindingKindSourceMismatch       = "source_mismatch"
	FindingKindUpstreamMismatch     = "upstream_mismatch"
	FindingKindTargetsMismatch      = "targets_mismatch"
	FindingKindStoreMissing         = "store_missing"
	FindingKindStoreInvalid         = "store_invalid"
	FindingKindStoreIntegrity       = "store_integrity"
	FindingKindMissingTargetInstall = "missing_target_install"
	FindingKindDriftedTarget        = "drifted_target"
	FindingKindUnexpectedTarget     = "unexpected_target"
	FindingKindLegacySymlink        = "legacy_symlink"
	FindingKindUnexpectedEntryType  = "unexpected_entry_type"
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

	return findings, nil
}

func (s Service) doctorSkillFindings(doc *manifest.Manifest, skill manifest.Skill, locked lockfile.Skill) []DoctorFinding {
	findings := make([]DoctorFinding, 0)
	expectedTargets := effectiveTargetsForSkill(doc, skill)
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
		shouldExist := slices.Contains(expectedTargets, targetName)
		findings = append(findings, s.doctorTargetFindings(skill.Name, targetName, storePath, shouldExist)...)
	}

	return findings
}

func classifyStoreFinding(skillName string, err error) []DoctorFinding {
	kind := FindingKindStoreInvalid
	if errors.Is(err, os.ErrNotExist) {
		kind = FindingKindStoreMissing
	}
	return []DoctorFinding{{
		Kind:    kind,
		Skill:   skillName,
		Message: err.Error(),
	}}
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
	case targetStatusLegacySymlink:
		return []DoctorFinding{{
			Kind:       FindingKindLegacySymlink,
			Skill:      skillName,
			Message:    legacySymlinkInstallError(inspection.Path).Error(),
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
