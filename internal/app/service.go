package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Z-Bra0/Ski/internal/fsutil"
	"github.com/Z-Bra0/Ski/internal/lockfile"
	"github.com/Z-Bra0/Ski/internal/manifest"
	"github.com/Z-Bra0/Ski/internal/source"
	"github.com/Z-Bra0/Ski/internal/store"
	"github.com/Z-Bra0/Ski/internal/target"
)

// Service orchestrates ski operations for a single project or the global scope.
type Service struct {
	ProjectDir string
	HomeDir    string
	Global     bool

	// Test hooks — nil in production; set in tests to inject failures.
	materializeAllFn func(targets []string, name, storePath string) error
	removeAllFn      func(targets []string, name string) error
	replaceTargetFn  func(target, name, storePath string) error
}

// MultiSkillSelectionError reports that a repository contains multiple skills and
// the caller must choose one or more of them explicitly.
type MultiSkillSelectionError struct {
	Skills []string
}

func (e MultiSkillSelectionError) Error() string {
	return fmt.Sprintf("multiple skills found in repository: %s", strings.Join(e.Skills, ", "))
}

func (s Service) materializeAll(targets []string, name, storePath string) error {
	if s.materializeAllFn != nil {
		return s.materializeAllFn(targets, name, storePath)
	}
	if s.Global {
		return target.MaterializeAllGlobal(s.HomeDir, targets, name, storePath)
	}
	return target.MaterializeAll(s.ProjectDir, targets, name, storePath)
}

func (s Service) removeAll(targets []string, name string) error {
	if s.removeAllFn != nil {
		return s.removeAllFn(targets, name)
	}
	if s.Global {
		return target.RemoveAllGlobal(s.HomeDir, targets, name)
	}
	return target.RemoveAll(s.ProjectDir, targets, name)
}

func (s Service) backupTarget(targetName, skillName string) (string, error) {
	dir, err := s.skillDir(targetName)
	if err != nil {
		return "", err
	}
	entryPath := filepath.Join(dir, skillName)
	if _, err := os.Lstat(entryPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("lstat %s: %w", entryPath, err)
	}
	backupPath, err := os.MkdirTemp(dir, "."+skillName+"-txbackup-")
	if err != nil {
		return "", fmt.Errorf("create backup dir for %s: %w", entryPath, err)
	}
	if err := os.Remove(backupPath); err != nil {
		return "", fmt.Errorf("prepare backup path %s: %w", backupPath, err)
	}
	if err := fsutil.CopyTree(entryPath, backupPath); err != nil {
		return "", fmt.Errorf("backup %s: %w", entryPath, err)
	}
	return backupPath, nil
}

func (s Service) replaceTarget(targetName, name, storePath string) error {
	if s.replaceTargetFn != nil {
		return s.replaceTargetFn(targetName, name, storePath)
	}
	if s.Global {
		return target.ReplaceGlobal(s.HomeDir, targetName, name, storePath)
	}
	return target.Replace(s.ProjectDir, targetName, name, storePath)
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

func (s Service) readManifest(path string) (*manifest.Manifest, error) {
	doc, err := manifest.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := s.validateManifestTargets(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s Service) loadSourceForScope(rawSource string) (source.Git, error) {
	return source.ParseGit(rawSource)
}

// CheckInitAvailable reports whether the active-scope manifest can be created.
func (s Service) CheckInitAvailable() error {
	path := s.manifestPath()
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return nil
}

// Init creates a new default manifest in the active scope.
func (s Service) Init() (string, error) {
	return s.InitWithTargets(nil)
}

// InitWithTargets creates a new manifest in the active scope with the provided targets.
func (s Service) InitWithTargets(targets []string) (string, error) {
	path := s.manifestPath()
	if err := s.CheckInitAvailable(); err != nil {
		return "", err
	}

	doc := manifest.Default()
	doc.Targets = append([]string(nil), targets...)
	if err := s.validateManifestTargets(&doc); err != nil {
		return "", err
	}
	if err := ensureParentDir(path); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := manifest.WriteFile(path, doc); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func (s Service) loadProjectState() (*manifest.Manifest, *lockfile.Lockfile, error) {
	manifestPath := s.manifestPath()
	doc, err := s.readManifest(manifestPath)
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

func (s Service) validateManifestTargets(doc *manifest.Manifest) error {
	if err := s.validateTargetSet(doc.Targets, "manifest targets"); err != nil {
		return err
	}
	for _, skill := range doc.Skills {
		if err := s.validateTargetSet(skill.Targets, fmt.Sprintf("skill %q targets", skill.Name)); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) validateTargetSet(targets []string, context string) error {
	seenNames := make(map[string]struct{}, len(targets))
	seenDirs := make(map[string]string, len(targets))
	for _, rawTarget := range targets {
		targetName := strings.TrimSpace(rawTarget)
		if targetName == "" {
			return fmt.Errorf("%s: target names must not be empty", context)
		}
		if targetName != rawTarget {
			return fmt.Errorf("%s: target %q must not include leading or trailing whitespace", context, rawTarget)
		}
		if _, ok := seenNames[targetName]; ok {
			return fmt.Errorf("%s: duplicate target %q", context, targetName)
		}
		seenNames[targetName] = struct{}{}

		dir, err := s.skillDir(targetName)
		if err != nil {
			return fmt.Errorf("%s: %w", context, err)
		}
		if previous, ok := seenDirs[dir]; ok {
			return fmt.Errorf("%s: targets %q and %q resolve to the same directory %s", context, previous, targetName, dir)
		}
		seenDirs[dir] = targetName
	}
	return nil
}

func findSkill(skills []manifest.Skill, match func(manifest.Skill) bool) (manifest.Skill, bool) {
	for _, skill := range skills {
		if match(skill) {
			return skill, true
		}
	}
	return manifest.Skill{}, false
}

func findLockSkill(skills []lockfile.Skill, name string) (lockfile.Skill, bool) {
	for _, skill := range skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return lockfile.Skill{}, false
}

func parseOrDefaultLockfile(data []byte, existed bool) (*lockfile.Lockfile, error) {
	if !existed {
		doc := lockfile.Default()
		return &doc, nil
	}
	return lockfile.Parse(data)
}

func readOrDefaultLockfile(path string) (*lockfile.Lockfile, error) {
	data, existed, err := readOptionalFile(path)
	if err != nil {
		return nil, err
	}
	return parseOrDefaultLockfile(data, existed)
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
			Name:          skill.Name,
			Source:        skill.Source,
			UpstreamSkill: skill.UpstreamSkill,
			Version:       skill.Version,
			Enabled:       cloneBoolPtr(skill.Enabled),
			Targets:       append([]string(nil), skill.Targets...),
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
			Name:          skill.Name,
			Source:        skill.Source,
			UpstreamSkill: skill.UpstreamSkill,
			Version:       skill.Version,
			Commit:        skill.Commit,
			Integrity:     skill.Integrity,
			Targets:       append([]string(nil), skill.Targets...),
		}
	}
	return clone
}

func buildLockSkill(skill manifest.Skill, stored store.Result, effectiveTargets []string) (lockfile.Skill, error) {
	lockEntry := lockfile.Skill{
		Name:      skill.Name,
		Version:   skill.Version,
		Commit:    stored.Commit,
		Integrity: stored.Integrity,
		Targets:   append([]string(nil), effectiveTargets...),
	}
	var err error
	lockEntry.Source, lockEntry.UpstreamSkill, err = canonicalSkillIdentity(skill.Source, skill.UpstreamSkill)
	if err != nil {
		return lockfile.Skill{}, err
	}
	return lockEntry, nil
}

func restoreProjectFiles(manifestPath string, manifestData []byte, lockPath string, lockData []byte, hadLockfile bool) error {
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("restore %s: %w", manifestPath, err)
	}
	return restoreLockfile(lockPath, lockData, hadLockfile)
}

func restoreLockfile(lockPath string, lockData []byte, hadLockfile bool) error {
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

func removeByName[T any](skills []T, name string, getName func(T) string) []T {
	out := make([]T, 0, len(skills))
	for _, skill := range skills {
		if getName(skill) != name {
			out = append(out, skill)
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))
	result := append([]string(nil), a...)
	for _, item := range a {
		seen[item] = struct{}{}
	}
	for _, item := range b {
		if _, ok := seen[item]; ok {
			continue
		}
		result = append(result, item)
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

func installTargetsForSkill(doc *manifest.Manifest, skill manifest.Skill) []string {
	if !skillEnabled(skill) {
		return nil
	}
	return effectiveTargetsForSkill(doc, skill)
}

func skillEnabled(skill manifest.Skill) bool {
	return skill.Enabled == nil || *skill.Enabled
}

func setSkillEnabled(skill *manifest.Skill, enabled bool) {
	if enabled {
		skill.Enabled = nil
		return
	}
	skill.Enabled = boolPtr(false)
}

func boolPtr(v bool) *bool {
	return &v
}

func cloneBoolPtr(v *bool) *bool {
	if v == nil {
		return nil
	}
	return boolPtr(*v)
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

func differenceStrings(a, b []string) []string {
	out := make([]string, 0, len(a))
	seen := make(map[string]struct{}, len(b))
	for _, item := range b {
		seen[item] = struct{}{}
	}
	for _, item := range a {
		if _, ok := seen[item]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func intersectStrings(a, b []string) []string {
	out := make([]string, 0, len(a))
	seen := make(map[string]struct{}, len(b))
	for _, item := range b {
		seen[item] = struct{}{}
	}
	for _, item := range a {
		if _, ok := seen[item]; !ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func skillTargetsOverride(defaultTargets, effectiveTargets []string) []string {
	if sameStrings(defaultTargets, effectiveTargets) {
		return nil
	}
	return append([]string(nil), effectiveTargets...)
}

func selectSkills(doc *manifest.Manifest, name string, manifestPath string) ([]manifest.Skill, error) {
	if name == "" {
		return doc.Skills, nil
	}

	skill, ok := findSkill(doc.Skills, func(skill manifest.Skill) bool { return skill.Name == name })
	if !ok {
		return nil, fmt.Errorf("skill %q not found in %s", name, manifestPath)
	}
	return []manifest.Skill{skill}, nil
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
