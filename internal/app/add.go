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
	Name          string
	Source        string
	UpstreamSkill string
	Targets       []string
	StorePath     string
	Lock          lockfile.Skill
	Manifest      manifest.Skill
}

// Add parses a git source, fetches it into the store, links to targets,
// and writes both the manifest and lockfile.
// Returns the skill names that were added.
func (s Service) AddSelected(rawSource string, selectedSkills []string, nameOverride string, addAll bool) ([]string, []skill.ValidationWarning, error) {
	path := s.manifestPath()
	originalManifestData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("%s not found; %s", path, s.initHint())
		}
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := manifest.ReadFile(path)
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

	requestedSkills := append([]string(nil), selectedSkills...)
	if len(requestedSkills) == 0 {
		requestedSkills = append(requestedSkills, src.Skills...)
	}
	explicitSelection := len(requestedSkills) > 0
	if !explicitSelection && !addAll {
		selectable := make([]string, 0, len(discovered.Skills))
		for _, discoveredSkill := range discovered.Skills {
			if _, _, err := skill.ValidateDirWithWarnings(discoveredSkill.Path, discoveredSkill.Name); err == nil {
				selectable = append(selectable, discoveredSkill.Name)
			}
		}
		if len(selectable) > 1 {
			return nil, nil, MultiSkillSelectionError{Skills: selectable}
		}
	}
	if (!explicitSelection || addAll) && len(discovered.InvalidSkills) > 0 {
		return nil, nil, discovered.InvalidSkills[0].Err
	}
	if !explicitSelection || addAll {
		for _, discoveredSkill := range discovered.Skills {
			if _, _, err := skill.ValidateDirWithWarnings(discoveredSkill.Path, discoveredSkill.Name); err != nil {
				return nil, nil, err
			}
		}
	}
	if explicitSelection {
		for _, requestedSkill := range requestedSkills {
			for _, invalid := range discovered.InvalidSkills {
				if invalid.CandidateName == requestedSkill {
					return nil, nil, invalid.Err
				}
			}
		}
	}
	requestedSkills, err = resolveRequestedSkills(discovered.Skills, requestedSkills)
	if err != nil {
		return nil, nil, err
	}

	if nameOverride != "" && len(requestedSkills) != 1 {
		return nil, nil, fmt.Errorf("name override can only be used when adding one skill")
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
			return nil, nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
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
			Name:          localName,
			Source:        canonical,
			UpstreamSkill: selectedSkillName,
			Targets:       append([]string(nil), effectiveTargets...),
			StorePath:     stored.Path,
			Lock:          lockEntry,
			Manifest:      manifestEntry,
		})
		added = append(added, localName)
	}

	if err := ensureParentDir(lockPath); err != nil {
		return nil, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		return nil, nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	if err := ensureParentDir(path); err != nil {
		return nil, nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := manifest.WriteFile(path, nextDoc); err != nil {
		if restoreErr := restoreProjectFiles(path, originalManifestData, lockPath, originalLockData, hadLockfile); restoreErr != nil {
			return nil, nil, fmt.Errorf("write %s: %w (rollback failed: %v)", path, err, restoreErr)
		}
		return nil, nil, fmt.Errorf("write %s: %w", path, err)
	}

	linked := make([]plannedAdd, 0, len(planned))
	for _, plan := range planned {
		if err := s.linkAll(plan.Targets, plan.Name, plan.StorePath); err != nil {
			rollbackErr := s.rollbackAddSelected(linked, path, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return nil, nil, fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
			return nil, nil, err
		}
		linked = append(linked, plan)
	}

	return added, append([]skill.ValidationWarning(nil), warnings...), nil
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
