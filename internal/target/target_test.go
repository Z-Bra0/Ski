package target_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/target"
)

func claudeLink(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".claude", "skills", name)
}

func codexLink(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".codex", "skills", name)
}

func customLink(projectRoot, rel, name string) string {
	return filepath.Join(projectRoot, rel, name)
}

func TestBuiltInTargetDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	homeDir := t.TempDir()
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks(root) error = %v", err)
	}
	homeReal, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(homeDir) error = %v", err)
	}

	tests := []struct {
		name       string
		projectDir string
		globalDir  string
	}{
		{name: "claude", projectDir: filepath.Join(rootReal, ".claude", "skills"), globalDir: filepath.Join(homeReal, ".claude", "skills")},
		{name: "codex", projectDir: filepath.Join(rootReal, ".codex", "skills"), globalDir: filepath.Join(homeReal, ".codex", "skills")},
		{name: "cursor", projectDir: filepath.Join(rootReal, ".cursor", "skills"), globalDir: filepath.Join(homeReal, ".cursor", "skills")},
		{name: "copilot", projectDir: filepath.Join(rootReal, ".github", "skills"), globalDir: filepath.Join(homeReal, ".github", "skills")},
		{name: "windsurf", projectDir: filepath.Join(rootReal, ".windsurf", "skills"), globalDir: filepath.Join(homeReal, ".codeium", "windsurf", "skills")},
		{name: "gemini", projectDir: filepath.Join(rootReal, ".gemini", "skills"), globalDir: filepath.Join(homeReal, ".gemini", "skills")},
		{name: "antigravity", projectDir: filepath.Join(rootReal, ".agent", "skills"), globalDir: filepath.Join(homeReal, ".gemini", "antigravity", "skills")},
		{name: "openclaw", projectDir: filepath.Join(rootReal, ".openclaw", "skills"), globalDir: filepath.Join(homeReal, ".openclaw", "skills")},
		{name: "opencode", projectDir: filepath.Join(rootReal, ".opencode", "skills"), globalDir: filepath.Join(homeReal, ".config", "opencode", "skills")},
		{name: "goose", projectDir: filepath.Join(rootReal, ".goose", "skills"), globalDir: filepath.Join(homeReal, ".config", "goose", "skills")},
		{name: "agents", projectDir: filepath.Join(rootReal, ".agents", "skills"), globalDir: filepath.Join(homeReal, ".config", "agents", "skills")},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotProject, err := target.SkillDir(root, tc.name)
			if err != nil {
				t.Fatalf("SkillDir(%q) error = %v", tc.name, err)
			}
			if gotProject != tc.projectDir {
				t.Fatalf("SkillDir(%q) = %q, want %q", tc.name, gotProject, tc.projectDir)
			}

			gotGlobal, err := target.GlobalSkillDir(homeDir, tc.name)
			if err != nil {
				t.Fatalf("GlobalSkillDir(%q) error = %v", tc.name, err)
			}
			if gotGlobal != tc.globalDir {
				t.Fatalf("GlobalSkillDir(%q) = %q, want %q", tc.name, gotGlobal, tc.globalDir)
			}
		})
	}
}

func TestMaterializeCreatesInstalledCopy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.Materialize(root, "claude", "my-skill", store); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}

	assertInstalledSkill(t, claudeLink(root, "my-skill"), "alpha")
}

func TestMaterializeSupportsCustomRelativeTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.Materialize(root, "dir:./agent-skills/claude", "my-skill", store); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}

	assertInstalledSkill(t, customLink(root, filepath.Clean("./agent-skills/claude"), "my-skill"), "alpha")
}

func TestMaterializeSupportsCustomTargetViaInScopeSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")
	if err := os.MkdirAll(filepath.Join(root, "managed"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "managed"), filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := target.Materialize(root, "dir:linked/skills", "my-skill", store); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}

	assertInstalledSkill(t, filepath.Join(root, "managed", "skills", "my-skill"), "alpha")
}

func TestMaterializeRejectsExistingInstalledDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store1 := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")
	store2 := writeStoreSkill(t, t.TempDir(), "my-skill", "beta")

	if err := target.Materialize(root, "claude", "my-skill", store1); err != nil {
		t.Fatalf("Materialize(store1) error = %v", err)
	}
	err := target.Materialize(root, "claude", "my-skill", store2)
	if err == nil {
		t.Fatal("Materialize(store2) error = nil, want conflict error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Materialize() error = %v, want existing-directory error", err)
	}
}

func TestMaterializeRejectsNonDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := target.Materialize(root, "claude", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want non-directory error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Materialize() error = %v, want non-directory error", err)
	}
}

func TestMaterializeRejectsLegacySymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink("/tmp/old-skill", filepath.Join(dir, "my-skill")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.Materialize(root, "claude", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want legacy symlink error")
	}
	if !strings.Contains(err.Error(), "legacy symlink install") {
		t.Fatalf("Materialize() error = %v, want legacy symlink error", err)
	}
}

func TestMaterializeRejectsUnsupportedTarget(t *testing.T) {
	t.Parallel()

	err := target.Materialize(t.TempDir(), "unknown-agent", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Materialize() error = %v, want 'unsupported target'", err)
	}
}

func TestMaterializeRejectsAbsoluteCustomTarget(t *testing.T) {
	t.Parallel()

	err := target.Materialize(t.TempDir(), "dir:/tmp/agent-skills", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want project-relative error")
	}
	if !strings.Contains(err.Error(), "project-relative") {
		t.Fatalf("Materialize() error = %v, want project-relative error", err)
	}
}

func TestMaterializeRejectsParentEscapingCustomTarget(t *testing.T) {
	t.Parallel()

	err := target.Materialize(t.TempDir(), "dir:../agent-skills", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want project-root error")
	}
	if !strings.Contains(err.Error(), "within the project root") {
		t.Fatalf("Materialize() error = %v, want project-root error", err)
	}
}

func TestMaterializeRejectsProjectRootCustomTarget(t *testing.T) {
	t.Parallel()

	for _, customTarget := range []string{"dir:.", "dir:./"} {
		err := target.Materialize(t.TempDir(), customTarget, "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
		if err == nil {
			t.Fatalf("Materialize(%q) error = nil, want project-root error", customTarget)
		}
		if !strings.Contains(err.Error(), "would install skills into the project root") {
			t.Fatalf("Materialize(%q) error = %v, want explicit project-root error", customTarget, err)
		}
	}
}

func TestMaterializeGlobalRejectsHomeRootCustomTarget(t *testing.T) {
	t.Parallel()

	for _, customTarget := range []string{"dir:.", "dir:./"} {
		err := target.MaterializeGlobal(t.TempDir(), customTarget, "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
		if err == nil {
			t.Fatalf("MaterializeGlobal(%q) error = nil, want home-root error", customTarget)
		}
		if !strings.Contains(err.Error(), "would install skills into the user home directory") {
			t.Fatalf("MaterializeGlobal(%q) error = %v, want explicit home-root error", customTarget, err)
		}
	}
}

func TestMaterializeRejectsBareRelativePathWithoutPrefix(t *testing.T) {
	t.Parallel()

	err := target.Materialize(t.TempDir(), "./agent-skills/claude", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Materialize() error = %v, want unsupported target error", err)
	}
}

func TestMaterializeRejectsCustomTargetWithSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.Materialize(root, "dir:linked/skills", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Materialize() error = nil, want symlink traversal error")
	}
	if !strings.Contains(err.Error(), "escapes the managed scope via symlink traversal") {
		t.Fatalf("Materialize() error = %v, want symlink traversal error", err)
	}
}

func TestMaterializeGlobalCreatesInstalledCopy(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.MaterializeGlobal(homeDir, "claude", "my-skill", store); err != nil {
		t.Fatalf("MaterializeGlobal() error = %v", err)
	}

	assertInstalledSkill(t, filepath.Join(homeDir, ".claude", "skills", "my-skill"), "alpha")
}

func TestMaterializeGlobalSupportsHomeRelativeCustomTarget(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.MaterializeGlobal(homeDir, "dir:agent-skills/claude", "my-skill", store); err != nil {
		t.Fatalf("MaterializeGlobal() error = %v", err)
	}

	assertInstalledSkill(t, filepath.Join(homeDir, "agent-skills", "claude", "my-skill"), "alpha")
}

func TestMaterializeGlobalSupportsTildeExpansion(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.MaterializeGlobal(homeDir, "dir:~/agent-skills/claude", "my-skill", store); err != nil {
		t.Fatalf("MaterializeGlobal() error = %v", err)
	}

	assertInstalledSkill(t, filepath.Join(homeDir, "agent-skills", "claude", "my-skill"), "alpha")
}

func TestMaterializeGlobalRejectsCustomTargetWithSymlinkEscape(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(homeDir, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.MaterializeGlobal(homeDir, "dir:linked/skills", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("MaterializeGlobal() error = nil, want symlink traversal error")
	}
	if !strings.Contains(err.Error(), "escapes the managed scope via symlink traversal") {
		t.Fatalf("MaterializeGlobal() error = %v, want symlink traversal error", err)
	}
}

func TestReplaceSwapsInstalledDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store1 := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")
	store2 := writeStoreSkill(t, t.TempDir(), "my-skill", "beta")

	if err := target.Materialize(root, "claude", "my-skill", store1); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if err := target.Replace(root, "claude", "my-skill", store2); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}

	assertInstalledSkill(t, claudeLink(root, "my-skill"), "beta")
}

func TestReplaceRejectsLegacySymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink("/tmp/old-skill", filepath.Join(dir, "my-skill")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.Replace(root, "claude", "my-skill", writeStoreSkill(t, t.TempDir(), "my-skill", "alpha"))
	if err == nil {
		t.Fatal("Replace() error = nil, want legacy symlink error")
	}
	if !strings.Contains(err.Error(), "legacy symlink install") {
		t.Fatalf("Replace() error = %v, want legacy symlink error", err)
	}
}

func TestRemoveRemovesInstalledDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.Materialize(root, "claude", "my-skill", store); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if err := target.Remove(root, "claude", "my-skill"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Lstat(claudeLink(root, "my-skill")); !os.IsNotExist(err) {
		t.Fatalf("target still exists after Remove")
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := target.Remove(root, "claude", "my-skill"); err != nil {
		t.Fatalf("Remove() on missing path error = %v", err)
	}
}

func TestRemoveRejectsNonDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := target.Remove(root, "claude", "my-skill")
	if err == nil {
		t.Fatal("Remove() error = nil, want non-directory error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Remove() error = %v, want non-directory error", err)
	}
}

func TestRemoveRejectsLegacySymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink("/tmp/old-skill", filepath.Join(dir, "my-skill")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.Remove(root, "claude", "my-skill")
	if err == nil {
		t.Fatal("Remove() error = nil, want legacy symlink error")
	}
	if !strings.Contains(err.Error(), "legacy symlink install") {
		t.Fatalf("Remove() error = %v, want legacy symlink error", err)
	}
}

func TestRemoveRejectsUnsupportedTarget(t *testing.T) {
	t.Parallel()

	err := target.Remove(t.TempDir(), "unknown-agent", "my-skill")
	if err == nil {
		t.Fatal("Remove() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Remove() error = %v, want 'unsupported target'", err)
	}
}

func TestRemoveAllRemovesAcrossTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.MaterializeAll(root, []string{"claude", "codex"}, "my-skill", store); err != nil {
		t.Fatalf("MaterializeAll() error = %v", err)
	}
	if err := target.RemoveAll(root, []string{"claude", "codex"}, "my-skill"); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	for _, path := range []string{claudeLink(root, "my-skill"), codexLink(root, "my-skill")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("target %s still exists after RemoveAll", path)
		}
	}
}

func TestRemoveAllStopsOnFirstError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := writeStoreSkill(t, t.TempDir(), "my-skill", "alpha")

	if err := target.Materialize(root, "codex", "my-skill", store); err != nil {
		t.Fatalf("Materialize(codex) error = %v", err)
	}
	claudeDir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := target.RemoveAll(root, []string{"claude", "codex"}, "my-skill")
	if err == nil {
		t.Fatal("RemoveAll() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("RemoveAll() error = %v, want non-directory error", err)
	}

	assertInstalledSkill(t, codexLink(root, "my-skill"), "alpha")
}

func writeStoreSkill(t *testing.T, root, skillName, marker string) string {
	t.Helper()

	path := filepath.Join(root, skillName)
	if err := os.MkdirAll(filepath.Join(path, "tools"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	skillDoc := "---\nname: " + skillName + "\ndescription: " + marker + "\n---\n\n# " + skillName + "\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(skillDoc), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "tools", "helper.txt"), []byte(marker), 0o644); err != nil {
		t.Fatalf("WriteFile(helper.txt) error = %v", err)
	}
	return path
}

func assertInstalledSkill(t *testing.T, path, marker string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
	data, err := os.ReadFile(filepath.Join(path, "tools", "helper.txt"))
	if err != nil {
		t.Fatalf("ReadFile(helper.txt) error = %v", err)
	}
	if string(data) != marker {
		t.Fatalf("helper.txt = %q, want %q", string(data), marker)
	}
}
