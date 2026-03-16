package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ski/internal/lockfile"
	"ski/internal/manifest"
	"ski/internal/source"
	"ski/internal/store"
	"ski/internal/target"
)

// Service orchestrates ski operations for a single project.
type Service struct {
	ProjectDir string
	HomeDir    string
}

type UpdateInfo struct {
	Name          string
	CurrentCommit string
	LatestCommit  string
}

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

// Init creates a new ski.toml in the project directory.
// Returns the path of the created manifest.
func (s Service) Init() (string, error) {
	path := filepath.Join(s.ProjectDir, manifest.FileName)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	doc := manifest.Default()
	if err := manifest.WriteFile(path, doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// Add parses a git source, fetches it into the store, links to targets,
// and writes both ski.toml and ski.lock.json.
// Returns the skill name that was added.
func (s Service) Add(rawSource string, nameOverride string) (string, error) {
	path := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s not found; run `ski init` first", path)
		}
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	src, err := source.ParseGit(rawSource)
	if err != nil {
		return "", err
	}

	skillName := nameOverride
	if skillName == "" {
		skillName, err = src.DeriveName()
		if err != nil {
			return "", err
		}
	}

	if existing, ok := findSkillByName(doc.Skills, skillName); ok {
		if nameOverride == "" {
			return "", fmt.Errorf("derived skill name %q already exists for source %q; rerun with --name", skillName, existing.Source)
		}
		return "", fmt.Errorf("skill name %q already exists", skillName)
	}

	canonical := src.String()
	if existing, ok := findSkillBySource(doc.Skills, canonical); ok {
		return "", fmt.Errorf("source %q already exists as skill %q", canonical, existing.Name)
	}

	stored, err := store.EnsureGit(s.ProjectDir, s.HomeDir, src, skillName)
	if err != nil {
		return "", err
	}

	effectiveTargets := append([]string(nil), doc.Targets...)
	if err := target.LinkAll(s.ProjectDir, effectiveTargets, skillName, stored.Path); err != nil {
		return "", err
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", lockPath, err)
	}
	upsertLockSkill(lf, lockfile.Skill{
		Name:      skillName,
		Source:    canonical,
		Commit:    stored.Commit,
		Integrity: stored.Integrity,
		Targets:   effectiveTargets,
	})
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return "", fmt.Errorf("write %s: %w", lockPath, err)
	}

	doc.Skills = append(doc.Skills, manifest.Skill{
		Name:   skillName,
		Source: canonical,
	})
	if err := manifest.WriteFile(path, *doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	return skillName, nil
}

func findSkillByName(skills []manifest.Skill, name string) (manifest.Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return manifest.Skill{}, false
}

func findSkillBySource(skills []manifest.Skill, source string) (manifest.Skill, bool) {
	for _, skill := range skills {
		if skill.Source == source {
			return skill, true
		}
	}
	return manifest.Skill{}, false
}

func readOrDefaultLockfile(path string) (*lockfile.Lockfile, error) {
	lf, err := lockfile.ReadFile(path)
	if err == nil {
		return lf, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		doc := lockfile.Default()
		return &doc, nil
	}
	return nil, err
}

func upsertLockSkill(lf *lockfile.Lockfile, skill lockfile.Skill) {
	for i, existing := range lf.Skills {
		if existing.Name == skill.Name {
			lf.Skills[i] = skill
			return
		}
	}
	lf.Skills = append(lf.Skills, skill)
}

// Install reads ski.toml and ski.lock.json, fetches all skills into the store,
// verifies integrity against the lockfile, and links them to configured targets.
// Returns the number of skills processed.
func (s Service) Install() (int, error) {
	manifestPath := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%s not found; run `ski init` first", manifestPath)
		}
		return 0, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	count := 0
	for _, mSkill := range doc.Skills {
		src, err := source.ParseGit(mSkill.Source)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		// Pin to locked commit for reproducibility.
		lockedEntry, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock {
			src.Ref = lockedEntry.Commit
		}

		stored, err := store.EnsureGit(s.ProjectDir, s.HomeDir, src, mSkill.Name)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		if hasLock && stored.Integrity != lockedEntry.Integrity {
			return count, fmt.Errorf("skill %q: integrity mismatch: got %s, want %s",
				mSkill.Name, stored.Integrity, lockedEntry.Integrity)
		}

		effectiveTargets := effectiveTargetsForSkill(doc, mSkill)
		if err := target.LinkAll(s.ProjectDir, effectiveTargets, mSkill.Name, stored.Path); err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		upsertLockSkill(lf, lockfile.Skill{
			Name:      mSkill.Name,
			Source:    mSkill.Source,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   effectiveTargets,
		})
		count++
	}

	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return count, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return count, nil
}

// Remove deletes a skill from ski.toml, ski.lock.json, and all target symlinks.
// The store cache entry is left intact for potential reuse.
func (s Service) Remove(name string) error {
	manifestPath := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; run `ski init` first", manifestPath)
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkillByName(doc.Skills, name)
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, manifest.FileName)
	}

	// Effective manifest targets for this skill.
	effectiveTargets := append([]string(nil), doc.Targets...)
	if len(ms.Targets) > 0 {
		effectiveTargets = ms.Targets
	}

	// Union with lock targets so we clean up even if targets changed since install.
	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	if lock, ok := findLockSkill(lf.Skills, name); ok {
		effectiveTargets = unionStrings(effectiveTargets, lock.Targets)
	}

	// Write metadata first so that if either write fails the symlinks still
	// exist and the project remains in a consistent, retryable state.
	doc.Skills = removeManifestSkill(doc.Skills, name)
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	lf.Skills = removeLockSkill(lf.Skills, name)
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	// Unlink last: if this fails, metadata is already clean and the orphaned
	// symlink points to still-valid store data — user can remove it manually
	// or re-run ski install to reconcile.
	if err := target.UnlinkAll(s.ProjectDir, effectiveTargets, name); err != nil {
		return fmt.Errorf("remove symlinks: %w", err)
	}

	return nil
}

func removeManifestSkill(skills []manifest.Skill, name string) []manifest.Skill {
	out := make([]manifest.Skill, 0, len(skills))
	for _, s := range skills {
		if s.Name != name {
			out = append(out, s)
		}
	}
	return out
}

func removeLockSkill(skills []lockfile.Skill, name string) []lockfile.Skill {
	out := make([]lockfile.Skill, 0, len(skills))
	for _, s := range skills {
		if s.Name != name {
			out = append(out, s)
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))
	result := append([]string(nil), a...)
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			result = append(result, s)
		}
	}
	return result
}

func effectiveTargetsForSkill(doc *manifest.Manifest, skill manifest.Skill) []string {
	targets := append([]string(nil), doc.Targets...)
	if len(skill.Targets) > 0 {
		targets = append([]string(nil), skill.Targets...)
	}
	return targets
}

func findLockSkill(skills []lockfile.Skill, name string) (lockfile.Skill, bool) {
	for _, s := range skills {
		if s.Name == name {
			return s, true
		}
	}
	return lockfile.Skill{}, false
}

// Doctor checks for project-state inconsistencies across the manifest,
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

	storePath, err := lockedStorePath(s.HomeDir, locked)
	if err != nil {
		findings = append(findings, DoctorFinding{
			Skill:   skill.Name,
			Message: err.Error(),
		})
		return findings
	}

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
		integrity, err := store.HashDir(storePath)
		if err != nil {
			findings = append(findings, DoctorFinding{
				Skill:   skill.Name,
				Message: fmt.Sprintf("hash %s: %v", storePath, err),
			})
		} else if integrity != locked.Integrity {
			findings = append(findings, DoctorFinding{
				Skill:   skill.Name,
				Message: fmt.Sprintf("integrity mismatch: got %s, want %s", integrity, locked.Integrity),
			})
		}
	}

	for _, targetName := range targetsToInspect {
		shouldExist := containsString(expectedTargets, targetName)
		findings = append(findings, s.doctorTargetFindings(skill.Name, targetName, storePath, shouldExist)...)
	}

	return findings
}

func (s Service) doctorTargetFindings(skillName, targetName, storePath string, shouldExist bool) []DoctorFinding {
	dir, err := target.SkillDir(s.ProjectDir, targetName)
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

func lockedStorePath(homeDir string, locked lockfile.Skill) (string, error) {
	src, err := source.ParseGit(locked.Source)
	if err != nil {
		return "", fmt.Errorf("invalid lockfile source %q: %w", locked.Source, err)
	}
	storeKey, err := src.DeriveName()
	if err != nil {
		return "", fmt.Errorf("derive store key from %q: %w", locked.Source, err)
	}
	return filepath.Join(homeDir, ".ski", "store", "git", storeKey, locked.Commit), nil
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, item := range a {
		counts[item]++
	}
	for _, item := range b {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
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
		src, err := source.ParseGit(mSkill.Source)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.ProjectDir, src)
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
		src, err := source.ParseGit(mSkill.Source)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.ProjectDir, src)
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
		stored, err := store.EnsureGit(s.ProjectDir, s.HomeDir, src, mSkill.Name)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		targets := effectiveTargetsForSkill(doc, mSkill)
		if hasLock {
			if err := target.UnlinkAll(s.ProjectDir, unionStrings(targets, locked.Targets), mSkill.Name); err != nil {
				return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
			}
		}
		if err := target.LinkAll(s.ProjectDir, targets, mSkill.Name, stored.Path); err != nil {
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

	lockPath := lockfile.Path(s.ProjectDir)
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return updates, nil
}

func (s Service) loadProjectState() (*manifest.Manifest, *lockfile.Lockfile, error) {
	manifestPath := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("%s not found; run `ski init` first", manifestPath)
		}
		return nil, nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", lockPath, err)
	}

	return doc, lf, nil
}

func selectSkills(doc *manifest.Manifest, name string) ([]manifest.Skill, error) {
	if name == "" {
		return doc.Skills, nil
	}

	skill, ok := findSkillByName(doc.Skills, name)
	if !ok {
		return nil, fmt.Errorf("skill %q not found in %s", name, manifest.FileName)
	}
	return []manifest.Skill{skill}, nil
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

// SkillInfo holds display data for a single installed skill.
type SkillInfo struct {
	Name    string
	Source  string
	Commit  string   // short SHA from lockfile; empty if not yet locked
	Targets []string // effective targets
}

// List returns the skills declared in ski.toml, enriched with lock data.
func (s Service) List() ([]SkillInfo, error) {
	manifestPath := filepath.Join(s.ProjectDir, manifest.FileName)
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found; run `ski init` first", manifestPath)
		}
		return nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := lockfile.Path(s.ProjectDir)
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", lockPath, err)
	}

	infos := make([]SkillInfo, 0, len(doc.Skills))
	for _, ms := range doc.Skills {
		info := SkillInfo{
			Name:    ms.Name,
			Source:  ms.Source,
			Targets: doc.Targets,
		}
		if len(ms.Targets) > 0 {
			info.Targets = ms.Targets
		}
		if lock, ok := findLockSkill(lf.Skills, ms.Name); ok {
			if len(lock.Commit) >= 7 {
				info.Commit = lock.Commit[:7]
			} else {
				info.Commit = lock.Commit
			}
		}
		infos = append(infos, info)
	}
	return infos, nil
}
