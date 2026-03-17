package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/store"
)

type DoctorFinding struct {
	Skill   string
	Message string
}

func (f DoctorFinding) String() string {
	if f.Skill == "" {
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Skill, f.Message)
}

// Doctor checks for active-scope inconsistencies across the manifest,
// lockfile, store, and linked target directories.
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

	if locked.Source != skill.Source {
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: fmt.Sprintf("lockfile source %q does not match manifest source %q", locked.Source, skill.Source),
		})
	}
	if !sameStrings(expectedTargets, locked.Targets) {
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: fmt.Sprintf("lockfile targets %v do not match manifest targets %v", locked.Targets, expectedTargets),
		})
	}

	src, err := s.loadSourceForScope(locked.Source)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}
	stored, err := store.FindGit(s.HomeDir, src, locked.Commit, skill.Name)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}
	storePath := stored.Path

	info, err := os.Stat(storePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: fmt.Sprintf("store path %s is missing", storePath),
		})
	case err != nil:
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: fmt.Sprintf("stat %s: %v", storePath, err),
		})
	case !info.IsDir():
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: fmt.Sprintf("store path %s is not a directory", storePath),
		})
	default:
		if stored.Integrity != locked.Integrity {
			findings = append(findings, DoctorFinding{
				Skill:   skill.Name,
				Message: fmt.Sprintf("integrity mismatch: got %s, want %s", stored.Integrity, locked.Integrity),
			})
		}
	}

	for _, targetName := range targetsToInspect {
		shouldExist := slices.Contains(expectedTargets, targetName)
		findings = append(findings, s.doctorTargetFindings(skill.Name, targetName, storePath, shouldExist)...)
	}

	return findings
}

func (s Service) doctorTargetFindings(skillName, targetName, storePath string, shouldExist bool) []DoctorFinding {
	dir, err := s.skillDir(targetName)
	if err != nil {
		return []DoctorFinding{{
			Skill:   skillName,
			Message: err.Error(),
		}}
	}

	linkPath := filepath.Join(dir, skillName)
	info, err := os.Lstat(linkPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !shouldExist {
			return nil
		}
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("missing %s symlink at %s", targetName, linkPath),
		}}
	case err != nil:
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("lstat %s: %v", linkPath, err),
		}}
	case info.Mode()&os.ModeSymlink == 0:
		if !shouldExist {
			return []DoctorFinding{{
				Skill:   skillName,
				Message: fmt.Sprintf("unexpected %s entry at %s is not a symlink", targetName, linkPath),
			}}
		}
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("%s is not a symlink", linkPath),
		}}
	}

	current, err := os.Readlink(linkPath)
	if err != nil {
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("readlink %s: %v", linkPath, err),
		}}
	}
	if !shouldExist {
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("unexpected %s symlink at %s points to %s", targetName, linkPath, current),
		}}
	}
	if current != storePath {
		return []DoctorFinding{{
			Skill:   skillName,
			Message: fmt.Sprintf("%s symlink points to %s, want %s", targetName, current, storePath),
		}}
	}

	return nil
}
