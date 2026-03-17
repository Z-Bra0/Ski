package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	Global     bool

	// Test hooks — nil in production; set in tests to inject failures.
	linkAllFn   func(targets []string, name, storePath string) error
	unlinkAllFn func(targets []string, name string) error
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

type plannedAdd struct {
	Name      string
	Source    string
	Targets   []string
	StorePath string
	Lock      lockfile.Skill
	Manifest  manifest.Skill
}

type MultiSkillSelectionError struct {
	Skills []string
}

func (s Service) linkAll(targets []string, name, storePath string) error {
	if s.linkAllFn != nil {
		return s.linkAllFn(targets, name, storePath)
	}
	if s.Global {
		return target.LinkAllGlobal(s.HomeDir, targets, name, storePath)
	}
	return target.LinkAll(s.ProjectDir, targets, name, storePath)
}

func (s Service) unlinkAll(targets []string, name string) error {
	if s.unlinkAllFn != nil {
		return s.unlinkAllFn(targets, name)
	}
	if s.Global {
		return target.UnlinkAllGlobal(s.HomeDir, targets, name)
	}
	return target.UnlinkAll(s.ProjectDir, targets, name)
}

func (f DoctorFinding) String() string {
	if f.Skill == "" {
		return f.Message
	}
	return fmt.Sprintf("%s: %s", f.Skill, f.Message)
}

func (e MultiSkillSelectionError) Error() string {
	return fmt.Sprintf("multiple skills found in repository: %s", strings.Join(e.Skills, ", "))
}

func (s Service) manifestPath() string {
	if s.Global {
		return manifest.GlobalPath(s.HomeDir)
	}
	return filepath.Join(s.ProjectDir, manifest.FileName)
}

func (s Service) lockPath() string {
	if s.Global {
		return lockfile.GlobalPath(s.HomeDir)
	}
	return lockfile.Path(s.ProjectDir)
}

func (s Service) initHint() string {
	if s.Global {
		return "run `ski init -g` first"
	}
	return "run `ski init` first"
}

func (s Service) sourceResolveDir() string {
	if s.Global {
		return s.HomeDir
	}
	return s.ProjectDir
}

func (s Service) skillDir(targetName string) (string, error) {
	if s.Global {
		return target.GlobalSkillDir(s.HomeDir, targetName)
	}
	return target.SkillDir(s.ProjectDir, targetName)
}

func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func (s Service) prepareAddSource(rawSource string) (source.Git, error) {
	src, err := source.ParseGit(rawSource)
	if err != nil {
		return source.Git{}, err
	}
	if !s.Global {
		return src, nil
	}

	return s.canonicalizeGlobalAddSource(src)
}

func (s Service) canonicalizeGlobalAddSource(src source.Git) (source.Git, error) {
	if !src.IsLocalPath() {
		return src, nil
	}

	url, err := canonicalizeGlobalLocalPath(src.URL, s.ProjectDir, s.HomeDir, true)
	if err != nil {
		return source.Git{}, fmt.Errorf("global git source %q: %w", src.URL, err)
	}
	src.URL = url
	return src, nil
}

func (s Service) loadSourceForScope(rawSource string) (source.Git, error) {
	src, err := source.ParseGit(rawSource)
	if err != nil {
		return source.Git{}, err
	}
	if !s.Global || !src.IsLocalPath() {
		return src, nil
	}

	url, err := canonicalizeGlobalLocalPath(src.URL, s.ProjectDir, s.HomeDir, false)
	if err != nil {
		return source.Git{}, err
	}
	src.URL = url
	return src, nil
}

func canonicalizeGlobalLocalPath(raw string, cwd string, homeDir string, allowRelative bool) (string, error) {
	switch {
	case raw == "~":
		return homeDir, nil
	case strings.HasPrefix(raw, "~/"):
		return filepath.Join(homeDir, raw[2:]), nil
	case strings.HasPrefix(raw, "~\\"):
		return filepath.Join(homeDir, raw[2:]), nil
	case filepath.IsAbs(raw):
		return filepath.Clean(raw), nil
	case allowRelative:
		return filepath.Abs(filepath.Join(cwd, raw))
	default:
		return "", fmt.Errorf("relative local git source %q is not allowed in global scope; use an absolute path", raw)
	}
}

// Init creates a new ski.toml in the project directory.
// Returns the path of the created manifest.
func (s Service) Init() (string, error) {
	path := s.manifestPath()
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}

	doc := manifest.Default()
	if err := ensureParentDir(path); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// Add parses a git source, fetches it into the store, links to targets,
// and writes both ski.toml and ski.lock.json.
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

	if nameOverride != "" {
		if len(requestedSkills) != 1 {
			return nil, fmt.Errorf("name override can only be used when adding one skill")
		}
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
		canonical := baseSource.WithSkills([]string{selectedSkillName}).String()

		if existing, ok := findSkill(nextDoc.Skills, func(s manifest.Skill) bool { return s.Name == localName }); ok {
			return nil, fmt.Errorf("skill name %q already exists for source %q", localName, existing.Source)
		}
		if existing, ok := findSkill(nextDoc.Skills, func(s manifest.Skill) bool { return s.Source == canonical }); ok {
			return nil, fmt.Errorf("source %q already exists as skill %q", canonical, existing.Name)
		}

		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, baseSource.WithSkills([]string{selectedSkillName}), selectedSkillName)
		if err != nil {
			return nil, err
		}

		if err := s.preflightAddLinks(effectiveTargets, localName); err != nil {
			return nil, err
		}

		lockEntry := lockfile.Skill{
			Name:      localName,
			Source:    canonical,
			Commit:    stored.Commit,
			Integrity: stored.Integrity,
			Targets:   effectiveTargets,
		}
		manifestEntry := manifest.Skill{
			Name:   localName,
			Source: canonical,
		}
		upsertLockSkill(&nextLock, lockEntry)
		nextDoc.Skills = append(nextDoc.Skills, manifestEntry)

		planned = append(planned, plannedAdd{
			Name:      localName,
			Source:    canonical,
			Targets:   append([]string(nil), effectiveTargets...),
			StorePath: stored.Path,
			Lock:      lockEntry,
			Manifest:  manifestEntry,
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

func findSkill(skills []manifest.Skill, match func(manifest.Skill) bool) (manifest.Skill, bool) {
	for _, skill := range skills {
		if match(skill) {
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

func readOptionalFile(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
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

func cloneManifest(doc manifest.Manifest) manifest.Manifest {
	clone := manifest.Manifest{
		Version: doc.Version,
		Targets: append([]string(nil), doc.Targets...),
		Skills:  make([]manifest.Skill, len(doc.Skills)),
	}
	for i, skill := range doc.Skills {
		clone.Skills[i] = manifest.Skill{
			Name:    skill.Name,
			Source:  skill.Source,
			Version: skill.Version,
			Targets: append([]string(nil), skill.Targets...),
		}
	}
	return clone
}

func cloneLockfile(doc lockfile.Lockfile) lockfile.Lockfile {
	clone := lockfile.Lockfile{
		Version: doc.Version,
		Skills:  make([]lockfile.Skill, len(doc.Skills)),
	}
	for i, skill := range doc.Skills {
		clone.Skills[i] = lockfile.Skill{
			Name:      skill.Name,
			Source:    skill.Source,
			Version:   skill.Version,
			Commit:    skill.Commit,
			Integrity: skill.Integrity,
			Targets:   append([]string(nil), skill.Targets...),
		}
	}
	return clone
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

func restoreProjectFiles(manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("restore %s: %w", manifestPath, err)
	}
	if hadLockfile {
		if err := os.WriteFile(lockPath, lockData, 0o644); err != nil {
			return fmt.Errorf("restore %s: %w", lockPath, err)
		}
		return nil
	}
	if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", lockPath, err)
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

// Install reads ski.toml and ski.lock.json, fetches all skills into the store,
// verifies integrity against the lockfile, and links them to configured targets.
// Returns the number of skills processed.
func (s Service) Install() (int, error) {
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return 0, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := s.lockPath()
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", lockPath, err)
	}

	count := 0
	for _, mSkill := range doc.Skills {
		src, err := s.loadSourceForScope(mSkill.Source)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		// Pin to locked commit for reproducibility.
		lockedEntry, hasLock := findLockSkill(lf.Skills, mSkill.Name)
		if hasLock {
			src.Ref = lockedEntry.Commit
		}

		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return count, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		if hasLock && stored.Integrity != lockedEntry.Integrity {
			return count, fmt.Errorf("skill %q: integrity mismatch: got %s, want %s",
				mSkill.Name, stored.Integrity, lockedEntry.Integrity)
		}

		effectiveTargets := effectiveTargetsForSkill(doc, mSkill)
		if err := s.linkAll(effectiveTargets, mSkill.Name, stored.Path); err != nil {
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

	if err := ensureParentDir(lockPath); err != nil {
		return count, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return count, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return count, nil
}

// Remove deletes a skill from ski.toml, ski.lock.json, and all target symlinks.
// The store cache entry is left intact for potential reuse.
func (s Service) Remove(name string) error {
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}

	ms, ok := findSkill(doc.Skills, func(s manifest.Skill) bool { return s.Name == name })
	if !ok {
		return fmt.Errorf("skill %q not found in %s", name, s.manifestPath())
	}

	// Union manifest targets with lock targets so we clean up even if targets changed since install.
	effectiveTargets := effectiveTargetsForSkill(doc, ms)

	// Union with lock targets so we clean up even if targets changed since install.
	lockPath := s.lockPath()
	lf, err := readOrDefaultLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", lockPath, err)
	}
	if lock, ok := findLockSkill(lf.Skills, name); ok {
		effectiveTargets = unionStrings(effectiveTargets, lock.Targets)
	}

	// Write metadata first so that if either write fails the symlinks still
	// exist and the project remains in a consistent, retryable state.
	doc.Skills = removeByName(doc.Skills, name, func(s manifest.Skill) string { return s.Name })
	if err := manifest.WriteFile(manifestPath, *doc); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}

	lf.Skills = removeByName(lf.Skills, name, func(s lockfile.Skill) string { return s.Name })
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	// Unlink last: if this fails, metadata is already clean and the orphaned
	// symlink points to still-valid store data — user can remove it manually
	// or re-run ski install to reconcile.
	if err := s.unlinkAll(effectiveTargets, name); err != nil {
		return fmt.Errorf("remove symlinks: %w", err)
	}

	return nil
}

func removeByName[T any](skills []T, name string, getName func(T) string) []T {
	out := make([]T, 0, len(skills))
	for _, s := range skills {
		if getName(s) != name {
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

func resolveRequestedSkills(discovered []store.DiscoveredSkill, requested []string) ([]string, error) {
	if len(discovered) == 0 {
		return nil, fmt.Errorf("no skills found in repository")
	}

	available := make(map[string]struct{}, len(discovered))
	availableNames := make([]string, 0, len(discovered))
	for _, skill := range discovered {
		available[skill.Name] = struct{}{}
		availableNames = append(availableNames, skill.Name)
	}

	if len(requested) == 0 {
		if len(discovered) == 1 {
			return []string{discovered[0].Name}, nil
		}
		return nil, MultiSkillSelectionError{Skills: availableNames}
	}

	seen := make(map[string]struct{}, len(requested))
	selected := make([]string, 0, len(requested))
	for _, name := range requested {
		if _, ok := available[name]; !ok {
			return nil, fmt.Errorf("skill %q not found in repository (available: %s)", name, strings.Join(availableNames, ", "))
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate selected skill %q", name)
		}
		seen[name] = struct{}{}
		selected = append(selected, name)
	}

	slices.Sort(selected)
	return selected, nil
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
		src, err := s.loadSourceForScope(mSkill.Source)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.sourceResolveDir(), src)
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
		src, err := s.loadSourceForScope(mSkill.Source)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}
		latestCommit, pinned, err := resolveUpdateCommit(s.sourceResolveDir(), src)
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
		stored, err := store.EnsureGit(s.sourceResolveDir(), s.HomeDir, src, mSkill.Name)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
		}

		targets := effectiveTargetsForSkill(doc, mSkill)
		if hasLock {
			if err := s.unlinkAll(unionStrings(targets, locked.Targets), mSkill.Name); err != nil {
				return nil, fmt.Errorf("skill %q: %w", mSkill.Name, err)
			}
		}
		if err := s.linkAll(targets, mSkill.Name, stored.Path); err != nil {
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

	lockPath := s.lockPath()
	if err := ensureParentDir(lockPath); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(lockPath), err)
	}
	if err := lockfile.WriteFile(lockPath, *lf); err != nil {
		return nil, fmt.Errorf("write %s: %w", lockPath, err)
	}

	return updates, nil
}

func (s Service) loadProjectState() (*manifest.Manifest, *lockfile.Lockfile, error) {
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return nil, nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := s.lockPath()
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

	skill, ok := findSkill(doc.Skills, func(s manifest.Skill) bool { return s.Name == name })
	if !ok {
		return nil, fmt.Errorf("skill %q not found in manifest", name)
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
	manifestPath := s.manifestPath()
	doc, err := manifest.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s not found; %s", manifestPath, s.initHint())
		}
		return nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}

	lockPath := s.lockPath()
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
