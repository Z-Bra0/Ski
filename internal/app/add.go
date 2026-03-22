package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/skill"
	"github.com/Z-Bra0/Ski/internal/store"
)

type plannedAdd struct {
	Name      string
	Targets   []string
	StorePath string
}

// Add parses a git source, fetches it into the store, links to targets,
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
	doc, err := s.readManifest(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	src, err := s.prepareAddSource(rawSource)
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

	requestedSkills, err := resolveSkillSelection(discovered, selectedSkills, addAll, nameOverride)
	if err != nil {
		return nil, nil, err
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := readOrDefaultLockfile(lockPath)
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
			sameIdentity, err := sameSkillIdentity(existing.Source, existing.UpstreamSkill, canonical, selectedSkillName)
			if err != nil {
				return nil, nil, fmt.Errorf("skill %q: %w", existing.Name, err)
			}
			if !sameIdentity {
				return nil, nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
			}
			if len(targetOverride) == 0 {
				return nil, nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
			}

			currentTargets := effectiveTargetsForSkill(&nextDoc, existing)
			mergedTargets := unionStrings(currentTargets, targetOverride)
			targetsToLink := differenceStrings(mergedTargets, currentTargets)

			var (
				stored        store.Result
				skillWarnings []skill.ValidationWarning
			)
			if locked, ok := findLockSkill(nextLock.Skills, localName); ok {
				stored, err = store.FindGit(s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), locked.Commit, selectedSkillName)
				if err != nil {
					return nil, nil, err
				}
				lockEntry := locked
				lockEntry.Source = canonical
				lockEntry.UpstreamSkill = selectedSkillName
				lockEntry.Version = existing.Version
				lockEntry.Targets = mergedTargets
				upsertLockSkill(&nextLock, lockEntry)
			} else {
				stored, skillWarnings, err = store.EnsureGitWithWarnings(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
				if err != nil {
					return nil, nil, err
				}
				lockEntry := lockfile.Skill{
					Name:          localName,
					Source:        canonical,
					UpstreamSkill: selectedSkillName,
					Version:       existing.Version,
					Commit:        stored.Commit,
					Integrity:     stored.Integrity,
					Targets:       mergedTargets,
				}
				upsertLockSkill(&nextLock, lockEntry)
			}
			warnings = append(warnings, skillWarnings...)

			if err := s.preflightAddLinks(mergedTargets, localName, stored.Path); err != nil {
				return nil, nil, err
			}

			for i := range nextDoc.Skills {
				if nextDoc.Skills[i].Name != localName {
					continue
				}
				nextDoc.Skills[i].Source = canonical
				nextDoc.Skills[i].UpstreamSkill = selectedSkillName
				nextDoc.Skills[i].Targets = skillTargetsOverride(nextDoc.Targets, mergedTargets)
				break
			}

			planned = append(planned, plannedAdd{
				Name:      localName,
				Targets:   append([]string(nil), targetsToLink...),
				StorePath: stored.Path,
			})
			added = append(added, localName)
			continue
		}
		if existing, ok, err := findSkillByIdentity(nextDoc.Skills, canonical, selectedSkillName); err != nil {
			return nil, nil, err
		} else if ok {
			return nil, nil, fmt.Errorf("source %q with upstream skill %q already exists as skill %q", canonical, selectedSkillName, existing.Name)
		}

		stored, skillWarnings, err := store.EnsureGitWithWarnings(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
		if err != nil {
			return nil, nil, err
		}
		warnings = append(warnings, skillWarnings...)

		if err := s.preflightAddLinks(effectiveTargets, localName, stored.Path); err != nil {
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
			Targets:   append([]string(nil), effectiveTargets...),
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

func findSkillByIdentity(skills []manifest.Skill, sourceValue, upstreamSkill string) (manifest.Skill, bool, error) {
	for _, skill := range skills {
		same, err := sameSkillIdentity(skill.Source, skill.UpstreamSkill, sourceValue, upstreamSkill)
		if err != nil {
			return manifest.Skill{}, false, fmt.Errorf("skill %q: %w", skill.Name, err)
		}
		if same {
			return skill, true, nil
		}
	}
	return manifest.Skill{}, false, nil
}

func (s Service) preflightAddLinks(targets []string, name, storePath string) error {
	seen := make(map[string]string, len(targets))
	for _, targetName := range targets {
		dir, err := s.skillDir(targetName)
		if err != nil {
			return err
		}
		if previous, ok := seen[dir]; ok {
			return fmt.Errorf("targets %q and %q resolve to the same directory %s", previous, targetName, dir)
		}
		seen[dir] = targetName
		linkPath := filepath.Join(dir, name)
		info, err := os.Lstat(linkPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("%s already exists and is not a symlink", linkPath)
			}
			current, err := os.Readlink(linkPath)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", linkPath, err)
			}
			// Treat an existing matching link as already reconciled so add can
			// fill in the remaining targets and persist manifest/lock state.
			if current == storePath {
				continue
			}
			return fmt.Errorf("%s already links to %s", linkPath, current)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("lstat %s: %w", linkPath, err)
		}
	}
	return nil
}

// commitAddPlans writes the updated lockfile and manifest to disk, then
// creates symlinks for each planned skill. On any failure it rolls back all
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

	linked := make([]plannedAdd, 0, len(planned))
	for _, plan := range planned {
		if err := s.linkAll(plan.Targets, plan.Name, plan.StorePath); err != nil {
			rollbackErr := s.rollbackAddSelected(linked, manifestPath, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
			return err
		}
		linked = append(linked, plan)
	}
	return nil
}

func (s Service) rollbackAddSelected(linked []plannedAdd, manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	var rollbackErr error
	for i := len(linked) - 1; i >= 0; i-- {
		if err := s.unlinkAll(linked[i].Targets, linked[i].Name); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if err := restoreProjectFiles(manifestPath, manifestData, lockPath, lockData, hadLockfile); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	return rollbackErr
}
