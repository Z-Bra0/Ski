package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/store"
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
func (s Service) AddSelected(rawSource string, selectedSkills []string, nameOverride string) ([]string, error) {
	path := s.manifestPath()
	originalManifestData, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found; %s", path, s.initHint())
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	doc, err := manifest.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	src, err := s.prepareAddSource(rawSource)
	if err != nil {
		return nil, err
	}

	if len(src.Skills) > 0 && len(selectedSkills) > 0 && !sameStrings(src.Skills, selectedSkills) {
		return nil, fmt.Errorf("selected skills %v do not match source selectors %v", selectedSkills, src.Skills)
	}

	discovered, err := store.DiscoverGit(s.sourceResolveDir(), s.HomeDir, src)
	if err != nil {
		return nil, err
	}

	requestedSkills := append([]string(nil), selectedSkills...)
	if len(requestedSkills) == 0 {
		requestedSkills = append(requestedSkills, src.Skills...)
	}
	requestedSkills, err = resolveRequestedSkills(discovered.Skills, requestedSkills)
	if err != nil {
		return nil, err
	}

	if nameOverride != "" && len(requestedSkills) != 1 {
		return nil, fmt.Errorf("name override can only be used when adding one skill")
	}

	lockPath := s.lockPath()
	originalLockData, hadLockfile, err := readOptionalFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", lockPath, err)
	}
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", lockPath, err)
	}

	baseSource := src.WithoutSkills()
	effectiveTargets := append([]string(nil), doc.Targets...)
	nextDoc := cloneManifest(*doc)
	nextLock := cloneLockfile(*lf)
	added := make([]string, 0, len(requestedSkills))
	planned := make([]plannedAdd, 0, len(requestedSkills))
	for _, selectedSkillName := range requestedSkills {
		localName := selectedSkillName
		if nameOverride != "" {
			localName = nameOverride
		}
		canonical := baseSource.String()

		if existing, ok := findSkill(nextDoc.Skills, func(skill manifest.Skill) bool { return skill.Name == localName }); ok {
			return nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
		}
		if existing, ok, err := findSkillByIdentity(nextDoc.Skills, canonical, selectedSkillName); err != nil {
			return nil, err
		} else if ok {
			return nil, fmt.Errorf("source %q with upstream skill %q already exists as skill %q", canonical, selectedSkillName, existing.Name)
		}

		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
		if err != nil {
			return nil, err
		}

		if err := s.preflightAddLinks(effectiveTargets, localName); err != nil {
			return nil, err
		}

		lockEntry := lockfile.Skill{
			Name:          localName,
			Source:        canonical,
			UpstreamSkill: selectedSkillName,
			Commit:        stored.Commit,
			Integrity:     stored.Integrity,
			Targets:       effectiveTargets,
		}
		manifestEntry := manifest.Skill{
			Name:          localName,
			Source:        canonical,
			UpstreamSkill: selectedSkillName,
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
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, nextLock); err != nil {
		return nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	if err := ensureParentDir(path); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := manifest.WriteFile(path, nextDoc); err != nil {
		if restoreErr := restoreProjectFiles(path, originalManifestData, lockPath, originalLockData, hadLockfile); restoreErr != nil {
			return nil, fmt.Errorf("write %s: %w (rollback failed: %v)", path, err, restoreErr)
		}
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	linked := make([]plannedAdd, 0, len(planned))
	for _, plan := range planned {
		if err := s.linkAll(plan.Targets, plan.Name, plan.StorePath); err != nil {
			rollbackErr := s.rollbackAddSelected(linked, path, originalManifestData, lockPath, originalLockData, hadLockfile)
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
			}
			return nil, err
		}
		linked = append(linked, plan)
	}

	return added, nil
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

func (s Service) preflightAddLinks(targets []string, name string) error {
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
		if _, err := os.Lstat(linkPath); err == nil {
			return fmt.Errorf("%s already exists", linkPath)
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
