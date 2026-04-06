package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/skill"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/store"
)

type plannedAdd struct {
	Name      string
	Targets   []string
	StorePath string
	Changes   []updateTargetChange
}

// Add parses a git source, fetches it into the store, installs targets,
// and writes both the manifest and lockfile.
// Returns the skill names that were added.
func (s Service) AddSelected(rawSource string, selectedSkills []string, nameOverride string, addAll bool, targetOverride []string) ([]string, []skill.ValidationWarning, error) {
	path := s.manifestPath()
	originalManifestData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("%s not found; %s", path, s.initHint())
		}
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := manifest.Parse(originalManifestData)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := s.validateManifestTargets(doc); err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	src, err := s.loadSourceForScope(rawSource)
	if err != nil {
		return nil, nil, err
	}

	if len(src.Skills) > 0 && len(selectedSkills) > 0 && !sameStrings(src.Skills, selectedSkills) {
		return nil, nil, fmt.Errorf("selected skills %v do not match source selectors %v", selectedSkills, src.Skills)
	}
	discovered, err := store.DiscoverGit(s.sourceResolveDir(), s.HomeDir, src)
	if err != nil {
		return nil, nil, err
	}

	requestedSkills := append([]string(nil), selectedSkills...)
	if len(requestedSkills) == 0 {
		requestedSkills = append(requestedSkills, src.Skills...)
	}

	requestedSkills, err = resolveSkillSelection(discovered, requestedSkills, addAll, nameOverride)
	if err != nil {
		return nil, nil, err
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := parseOrDefaultLockfile(originalLockData, hadLockfile)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", lockPath, err)
	}

	baseSource := src.WithoutSkills()
	effectiveTargets := append([]string(nil), doc.Targets...)
	if len(targetOverride) > 0 {
		effectiveTargets = append([]string(nil), targetOverride...)
	}
	nextDoc := cloneManifest(*doc)
	nextLock := cloneLockfile(*lf)
	added := make([]string, 0, len(requestedSkills))
	planned := make([]plannedAdd, 0, len(requestedSkills))
	warnings := make([]skill.ValidationWarning, 0)
	for _, selectedSkillName := range requestedSkills {
		localName := selectedSkillName
		if nameOverride != "" {
			localName = nameOverride
		}
		canonical := baseSource.String()

		if existing, ok := findSkill(nextDoc.Skills, func(skill manifest.Skill) bool { return skill.Name == localName }); ok {
			sameRepoIdentity, err := sameRepoSkillIdentity(existing.Source, existing.UpstreamSkill, canonical, selectedSkillName)
			if err != nil {
				return nil, nil, fmt.Errorf("skill %q: %w", existing.Name, err)
			}
			if !sameRepoIdentity {
				return nil, nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
			}
			plan, skillWarnings, err := s.planExistingSkillAdd(&nextDoc, &nextLock, existing, canonical, selectedSkillName, baseSource, targetOverride)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, skillWarnings...)
			planned = append(planned, plan)
			added = append(added, existing.Name)
			continue
		}
		if existing, ok, err := findSkillByRepoIdentity(nextDoc.Skills, canonical, selectedSkillName); err != nil {
			return nil, nil, err
		} else if ok {
			if nameOverride != "" && nameOverride != existing.Name {
				return nil, nil, fmt.Errorf("skill %q already exists for source %q; renaming an existing skill via --name is not supported", existing.Name, existing.Source)
			}

			plan, skillWarnings, err := s.planExistingSkillAdd(&nextDoc, &nextLock, existing, canonical, selectedSkillName, baseSource, targetOverride)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, skillWarnings...)
			planned = append(planned, plan)
			added = append(added, existing.Name)
			continue
		}

		stored, skillWarnings, err := store.EnsureGitWithWarnings(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, skillWarnings...)

		installTargets, err := s.preflightAddTargets(effectiveTargets, localName, stored.Path)
		if err != nil {
			return nil, nil, err
		}

		manifestEntry := manifest.Skill{
			Name:          localName,
			Source:        canonical,
			UpstreamSkill: selectedSkillName,
		}
		manifestEntry.Targets = skillTargetsOverride(nextDoc.Targets, effectiveTargets)
		lockEntry := lockfile.Skill{
			Name:          localName,
			Source:        canonical,
			UpstreamSkill: selectedSkillName,
			Version:       manifestEntry.Version,
			Commit:        stored.Commit,
			Integrity:     stored.Integrity,
			Targets:       effectiveTargets,
		}
		upsertLockSkill(&nextLock, lockEntry)
		nextDoc.Skills = append(nextDoc.Skills, manifestEntry)

		planned = append(planned, plannedAdd{
			Name:      localName,
			Targets:   append([]string(nil), installTargets...),
			StorePath: stored.Path,
		})
		added = append(added, localName)
	}

	if err := s.commitAddPlans(
		planned,
		path, originalManifestData,
		lockPath, originalLockData, hadLockfile,
		nextDoc, nextLock,
	); err != nil {
		return nil, nil, err
	}

	return added, append([]skill.ValidationWarning(nil), warnings...), nil
}

// resolveSkillSelection validates the user's skill selection against the
// discovered repository state and returns the final ordered list of upstream
// skill names to add.
func resolveSkillSelection(discovered store.RepoResult, selectedSkills []string, addAll bool, nameOverride string) ([]string, error) {
	requested := append([]string(nil), selectedSkills...)
	explicitSelection := len(requested) > 0

	if explicitSelection {
		for _, req := range requested {
			for _, invalid := range discovered.InvalidSkills {
				if invalid.CandidateName == req {
					return nil, invalid.Err
				}
			}
		}
	} else {
		// Single pass: validate each skill once, recording selectable names and
		// the first validation error. This avoids a second ValidateDirWithWarnings
		// scan that was previously needed for the full-validation step below.
		selectable := make([]string, 0, len(discovered.Skills))
		var firstValidationErr error
		for _, ds := range discovered.Skills {
			if _, _, err := skill.ValidateDirWithWarnings(ds.Path, ds.Name); err != nil {
				if firstValidationErr == nil {
					firstValidationErr = err
				}
			} else {
				selectable = append(selectable, ds.Name)
			}
		}

		if !addAll && len(selectable) > 1 {
			return nil, MultiSkillSelectionError{Skills: selectable}
		}
		// Reject any store-level parse failures before reporting dir errors.
		if len(discovered.InvalidSkills) > 0 {
			return nil, discovered.InvalidSkills[0].Err
		}
		if firstValidationErr != nil {
			return nil, firstValidationErr
		}
		if addAll {
			requested = append(requested, selectable...)
		}
	}

	resolved, err := resolveRequestedSkills(discovered.Skills, requested)
	if err != nil {
		return nil, err
	}

	if nameOverride != "" && len(resolved) != 1 {
		return nil, fmt.Errorf("name override can only be used when adding one skill")
	}

	return resolved, nil
}

func findSkillByRepoIdentity(skills []manifest.Skill, sourceValue, upstreamSkill string) (manifest.Skill, bool, error) {
	for _, skill := range skills {
		same, err := sameRepoSkillIdentity(skill.Source, skill.UpstreamSkill, sourceValue, upstreamSkill)
		if err != nil {
			return manifest.Skill{}, false, fmt.Errorf("skill %q: %w", skill.Name, err)
		}
		if same {
			return skill, true, nil
		}
	}
	return manifest.Skill{}, false, nil
}

func (s Service) planExistingSkillAdd(
	nextDoc *manifest.Manifest,
	nextLock *lockfile.Lockfile,
	existing manifest.Skill,
	canonical string,
	selectedSkillName string,
	baseSource source.Git,
	targetOverride []string,
) (plannedAdd, []skill.ValidationWarning, error) {
	desiredTargets := effectiveTargetsForSkill(nextDoc, existing)
	if len(targetOverride) > 0 {
		desiredTargets = unionStrings(desiredTargets, targetOverride)
	}
	desiredInstallTargets := desiredTargets
	if !skillEnabled(existing) {
		desiredInstallTargets = nil
	}

	locked, hasLock := findLockSkill(nextLock.Skills, existing.Name)
	previousTargets := []string(nil)
	previousStorePath := ""
	if hasLock {
		previousTargets = append(previousTargets, locked.Targets...)
		currentSrc, err := s.loadSkillSourceForScope(locked.Source, locked.UpstreamSkill)
		if err != nil {
			return plannedAdd{}, nil, err
		}
		currentSrc.Ref = locked.Commit
		previousStored, err := store.FindGit(s.HomeDir, currentSrc, locked.Commit, existing.Name)
		if err != nil {
			return plannedAdd{}, nil, err
		}
		previousStorePath = previousStored.Path
	}

	stored, warnings, err := store.EnsureGitWithWarnings(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
	if err != nil {
		return plannedAdd{}, nil, err
	}
	changes, err := s.planUpdateTargetChanges(existing.Name, desiredInstallTargets, previousTargets, previousStorePath, stored.Path)
	if err != nil {
		return plannedAdd{}, nil, err
	}

	updatedEntry := existing
	updatedEntry.Source = canonical
	updatedEntry.UpstreamSkill = selectedSkillName
	updatedEntry.Targets = skillTargetsOverride(nextDoc.Targets, desiredTargets)
	for i := range nextDoc.Skills {
		if nextDoc.Skills[i].Name != existing.Name {
			continue
		}
		nextDoc.Skills[i] = updatedEntry
		break
	}

	lockEntry, err := buildLockSkill(updatedEntry, stored, desiredTargets)
	if err != nil {
		return plannedAdd{}, nil, err
	}
	upsertLockSkill(nextLock, lockEntry)

	return plannedAdd{
		Name:    existing.Name,
		Changes: changes,
	}, warnings, nil
}

func (s Service) preflightAddTargets(targets []string, name, storePath string) ([]string, error) {
	seen := make(map[string]string, len(targets))
	installTargets := make([]string, 0, len(targets))
	for _, targetName := range targets {
		dir, err := s.skillDir(targetName)
		if err != nil {
			return nil, err
		}
		if previous, ok := seen[dir]; ok {
			return nil, fmt.Errorf("targets %q and %q resolve to the same directory %s", previous, targetName, dir)
		}
		seen[dir] = targetName
		inspection, err := s.inspectTarget(targetName, name, storePath)
		if err != nil {
			return nil, err
		}
		switch inspection.Status {
		case targetStatusMissing:
			installTargets = append(installTargets, targetName)
		case targetStatusInstalled:
			continue
		case targetStatusDrifted:
			return nil, driftedTargetError(inspection.Path)
		default:
			return nil, unexpectedTargetEntryError(inspection.Path)
		}
	}
	return installTargets, nil
}

// commitAddPlans writes the updated lockfile and manifest to disk, then
// materializes copied skill folders for each planned skill. On any failure it rolls back all
// on-disk changes made during this call.
func (s Service) commitAddPlans(
	planned []plannedAdd,
	manifestPath string, originalManifestData []byte,
	lockPath string, originalLockData []byte, hadLockfile bool,
	nextDoc manifest.Manifest, nextLock lockfile.Lockfile,
) error {
	if err := ensureParentDir(lockPath); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		if restoreErr := restoreProjectFiles(manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile); restoreErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", lockPath, err, restoreErr)
		}
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	if err := ensureParentDir(manifestPath); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(manifestPath), err)
	}
	if err := manifest.WriteFile(manifestPath, nextDoc); err != nil {
		// Always attempt to restore the lockfile independently — if manifest
		// restoration also fails we still want the lockfile consistent.
		manifestRestoreErr := os.WriteFile(manifestPath, originalManifestData, 0o644)
		lockRestoreErr := restoreLockfile(lockPath, originalLockData, hadLockfile)
		if rollbackErr := errors.Join(manifestRestoreErr, lockRestoreErr); rollbackErr != nil {
			return fmt.Errorf("write %s: %w (rollback failed: %v)", manifestPath, err, rollbackErr)
		}
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	applied := make([]plannedAdd, 0, len(planned))
	for _, plan := range planned {
		if len(plan.Changes) == 0 {
			if err := s.materializeAll(plan.Targets, plan.Name, plan.StorePath); err != nil {
				rollbackErr := s.rollbackAddSelected(applied, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
				if rollbackErr != nil {
					return fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
				}
				return err
			}
			applied = append(applied, plan)
			continue
		}

		priorApplied := applied
		appliedPlan, failure := s.applyTargetChangePlan(plan.targetChangesPlan(), targetChangePlansFromAdd(applied), func(innerApplied []plannedTargetChanges) error {
			// innerApplied contains previously applied non-zero-change plans plus any
			// partially applied changes from the current plan. Zero-change plans
			// (materialized new skills) are not tracked in innerApplied, so we
			// reconstruct the full list by prepending them from priorApplied.
			all := make([]plannedAdd, 0, len(priorApplied)+len(innerApplied))
			for _, p := range priorApplied {
				if len(p.Changes) == 0 {
					all = append(all, p)
				}
			}
			all = append(all, addPlansFromTargetChanges(innerApplied)...)
			return s.rollbackAddSelected(all, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
		})
		if failure != nil {
			if failure.RollbackErr != nil {
				return fmt.Errorf("%w (rollback failed: %v)", failure.Err, failure.RollbackErr)
			}
			return failure.Err
		}
		plan.Changes = appliedPlan.Changes
		applied = append(applied, plan)
	}
	cleanupTargetChangePlanBackups(targetChangePlansFromAdd(applied))
	return nil
}

func (s Service) rollbackAddSelected(applied []plannedAdd, manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(applied) - 1; i >= 0; i-- {
		if len(applied[i].Changes) == 0 {
			if err := s.removeAll(applied[i].Targets, applied[i].Name); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
			continue
		}
		if err := s.rollbackTargetChangePlan(applied[i].targetChangesPlan()); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	cleanupTargetChangePlanBackups(targetChangePlansFromAdd(applied))
	if err := restoreProjectFiles(manifestPath, manifestData, lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}

func (p plannedAdd) targetChangesPlan() plannedTargetChanges {
	return plannedTargetChanges{
		Name:    p.Name,
		Changes: p.Changes,
	}
}

func targetChangePlansFromAdd(plans []plannedAdd) []plannedTargetChanges {
	converted := make([]plannedTargetChanges, 0, len(plans))
	for _, plan := range plans {
		if len(plan.Changes) == 0 {
			continue
		}
		converted = append(converted, plan.targetChangesPlan())
	}
	return converted
}

func addPlansFromTargetChanges(plans []plannedTargetChanges) []plannedAdd {
	converted := make([]plannedAdd, 0, len(plans))
	for _, plan := range plans {
		converted = append(converted, plannedAdd{
			Name:    plan.Name,
			Changes: plan.Changes,
		})
	}
	return converted
}
