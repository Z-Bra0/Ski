package target_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Z-Bra0/Ski/internal/target"
)

// claudeLink returns the expected symlink path for the "claude" target.
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

// TestLinkCreatesSymlink verifies that Link creates the target directory and
// a symlink pointing at storePath.
func TestLinkCreatesSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.Link(root, "claude", "my-skill", store); err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	got, err := os.Readlink(claudeLink(root, "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

func TestLinkSupportsCustomRelativeTarget(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")
	targetDir := "dir:./agent-skills/claude"

	if err := target.Link(root, targetDir, "my-skill", store); err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	got, err := os.Readlink(customLink(root, filepath.Clean("./agent-skills/claude"), "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

func TestLinkSupportsCustomTargetViaInScopeSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")
	if err := os.MkdirAll(filepath.Join(root, "managed"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "managed"), filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	if err := target.Link(root, "dir:linked/skills", "my-skill", store); err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	got, err := os.Readlink(filepath.Join(root, "managed", "skills", "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

// TestLinkIsIdempotent verifies that calling Link twice with the same arguments
// succeeds without error.
func TestLinkIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	for i := range 2 {
		if err := target.Link(root, "claude", "my-skill", store); err != nil {
			t.Fatalf("Link() #%d error = %v", i+1, err)
		}
	}
}

// TestLinkRejectsConflictingSymlink verifies that Link errors when the link
// already points somewhere else.
func TestLinkRejectsConflictingSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store1 := filepath.Join(t.TempDir(), "v1")
	store2 := filepath.Join(t.TempDir(), "v2")

	if err := target.Link(root, "claude", "my-skill", store1); err != nil {
		t.Fatalf("Link(store1) error = %v", err)
	}
	err := target.Link(root, "claude", "my-skill", store2)
	if err == nil {
		t.Fatal("Link(store2) error = nil, want conflict error")
	}
	if !strings.Contains(err.Error(), "already links to") {
		t.Fatalf("Link() error = %v, want 'already links to'", err)
	}
}

// TestLinkRejectsNonSymlink verifies that Link errors when a regular file
// already occupies the link path.
func TestLinkRejectsNonSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// Place a regular file where the symlink should go.
	if err := os.WriteFile(filepath.Join(dir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := target.Link(root, "claude", "my-skill", "/some/store")
	if err == nil {
		t.Fatal("Link() error = nil, want not-a-symlink error")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("Link() error = %v, want 'not a symlink'", err)
	}
}

// TestLinkRejectsUnsupportedTarget verifies that Link errors on unknown targets.
func TestLinkRejectsUnsupportedTarget(t *testing.T) {
	t.Parallel()

	err := target.Link(t.TempDir(), "unknown-agent", "my-skill", "/store")
	if err == nil {
		t.Fatal("Link() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Link() error = %v, want 'unsupported target'", err)
	}
}

func TestLinkRejectsAbsoluteCustomTarget(t *testing.T) {
	t.Parallel()

	err := target.Link(t.TempDir(), "dir:/tmp/agent-skills", "my-skill", "/store")
	if err == nil {
		t.Fatal("Link() error = nil, want project-relative error")
	}
	if !strings.Contains(err.Error(), "project-relative") {
		t.Fatalf("Link() error = %v, want project-relative error", err)
	}
}

func TestLinkRejectsParentEscapingCustomTarget(t *testing.T) {
	t.Parallel()

	err := target.Link(t.TempDir(), "dir:../agent-skills", "my-skill", "/store")
	if err == nil {
		t.Fatal("Link() error = nil, want project-root error")
	}
	if !strings.Contains(err.Error(), "within the project root") {
		t.Fatalf("Link() error = %v, want project-root error", err)
	}
}

func TestLinkRejectsProjectRootCustomTarget(t *testing.T) {
	t.Parallel()

	for _, customTarget := range []string{"dir:.", "dir:./"} {
		err := target.Link(t.TempDir(), customTarget, "my-skill", "/store")
		if err == nil {
			t.Fatalf("Link(%q) error = nil, want project-root error", customTarget)
		}
		if !strings.Contains(err.Error(), "would install skills into the project root") {
			t.Fatalf("Link(%q) error = %v, want explicit project-root error", customTarget, err)
		}
	}
}

func TestLinkGlobalRejectsHomeRootCustomTarget(t *testing.T) {
	t.Parallel()

	for _, customTarget := range []string{"dir:.", "dir:./"} {
		err := target.LinkGlobal(t.TempDir(), customTarget, "my-skill", "/store")
		if err == nil {
			t.Fatalf("LinkGlobal(%q) error = nil, want home-root error", customTarget)
		}
		if !strings.Contains(err.Error(), "would install skills into the user home directory") {
			t.Fatalf("LinkGlobal(%q) error = %v, want explicit home-root error", customTarget, err)
		}
	}
}

func TestLinkRejectsBareRelativePathWithoutPrefix(t *testing.T) {
	t.Parallel()

	err := target.Link(t.TempDir(), "./agent-skills/claude", "my-skill", "/store")
	if err == nil {
		t.Fatal("Link() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Link() error = %v, want unsupported target error", err)
	}
}

func TestLinkRejectsCustomTargetWithSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.Link(root, "dir:linked/skills", "my-skill", "/store")
	if err == nil {
		t.Fatal("Link() error = nil, want symlink traversal error")
	}
	if !strings.Contains(err.Error(), "escapes the managed scope via symlink traversal") {
		t.Fatalf("Link() error = %v, want symlink traversal error", err)
	}
}

func TestLinkGlobalCreatesHomeSymlink(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.LinkGlobal(homeDir, "claude", "my-skill", store); err != nil {
		t.Fatalf("LinkGlobal() error = %v", err)
	}

	got, err := os.Readlink(filepath.Join(homeDir, ".claude", "skills", "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

func TestLinkGlobalSupportsHomeRelativeCustomTarget(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.LinkGlobal(homeDir, "dir:agent-skills/claude", "my-skill", store); err != nil {
		t.Fatalf("LinkGlobal() error = %v", err)
	}

	got, err := os.Readlink(filepath.Join(homeDir, "agent-skills", "claude", "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

func TestLinkGlobalSupportsTildeExpansion(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.LinkGlobal(homeDir, "dir:~/agent-skills/claude", "my-skill", store); err != nil {
		t.Fatalf("LinkGlobal() error = %v", err)
	}

	got, err := os.Readlink(filepath.Join(homeDir, "agent-skills", "claude", "my-skill"))
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != store {
		t.Fatalf("symlink target = %q, want %q", got, store)
	}
}

func TestLinkGlobalRejectsCustomTargetWithSymlinkEscape(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(homeDir, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	err := target.LinkGlobal(homeDir, "dir:linked/skills", "my-skill", "/store")
	if err == nil {
		t.Fatal("LinkGlobal() error = nil, want symlink traversal error")
	}
	if !strings.Contains(err.Error(), "escapes the managed scope via symlink traversal") {
		t.Fatalf("LinkGlobal() error = %v, want symlink traversal error", err)
	}
}

// TestUnlinkRemovesSymlink verifies that Unlink removes an existing symlink.
func TestUnlinkRemovesSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.Link(root, "claude", "my-skill", store); err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if err := target.Unlink(root, "claude", "my-skill"); err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if _, err := os.Lstat(claudeLink(root, "my-skill")); !os.IsNotExist(err) {
		t.Fatalf("symlink still exists after Unlink")
	}
}

// TestUnlinkIsIdempotent verifies that Unlink on a missing path succeeds.
func TestUnlinkIsIdempotent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	// No symlink created — must not error.
	if err := target.Unlink(root, "claude", "my-skill"); err != nil {
		t.Fatalf("Unlink() on missing path error = %v", err)
	}
}

// TestUnlinkRejectsNonSymlink verifies that Unlink errors rather than deleting
// a regular file that occupies the link path.
func TestUnlinkRejectsNonSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := target.Unlink(root, "claude", "my-skill")
	if err == nil {
		t.Fatal("Unlink() error = nil, want not-a-symlink error")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("Unlink() error = %v, want 'not a symlink'", err)
	}

	// File must be untouched.
	if _, err := os.Stat(filepath.Join(dir, "my-skill")); err != nil {
		t.Fatalf("file removed unexpectedly: %v", err)
	}
}

// TestUnlinkRejectsUnsupportedTarget verifies that Unlink errors on unknown targets.
func TestUnlinkRejectsUnsupportedTarget(t *testing.T) {
	t.Parallel()

	err := target.Unlink(t.TempDir(), "unknown-agent", "my-skill")
	if err == nil {
		t.Fatal("Unlink() error = nil, want unsupported target error")
	}
	if !strings.Contains(err.Error(), "unsupported target") {
		t.Fatalf("Unlink() error = %v, want 'unsupported target'", err)
	}
}

// TestUnlinkAllRemovesAcrossTargets verifies that UnlinkAll removes symlinks
// from every listed target.
func TestUnlinkAllRemovesAcrossTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	if err := target.LinkAll(root, []string{"claude", "codex"}, "my-skill", store); err != nil {
		t.Fatalf("LinkAll() error = %v", err)
	}
	if err := target.UnlinkAll(root, []string{"claude", "codex"}, "my-skill"); err != nil {
		t.Fatalf("UnlinkAll() error = %v", err)
	}

	for _, link := range []string{claudeLink(root, "my-skill"), codexLink(root, "my-skill")} {
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Fatalf("symlink %s still exists after UnlinkAll", link)
		}
	}
}

// TestUnlinkAllStopsOnFirstError verifies that UnlinkAll aborts on the first
// failure and does not silently skip errors.
func TestUnlinkAllStopsOnFirstError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store := filepath.Join(t.TempDir(), "my-skill")

	// Create a codex symlink but put a regular file in the claude slot.
	if err := target.Link(root, "codex", "my-skill", store); err != nil {
		t.Fatalf("Link(codex) error = %v", err)
	}
	claudeDir := filepath.Join(root, ".claude", "skills")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "my-skill"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// claude comes first alphabetically in this list — it should fail.
	err := target.UnlinkAll(root, []string{"claude", "codex"}, "my-skill")
	if err == nil {
		t.Fatal("UnlinkAll() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Fatalf("UnlinkAll() error = %v, want 'not a symlink'", err)
	}

	// Codex symlink must still be intact since we stopped after the claude error.
	if _, err := os.Lstat(codexLink(root, "my-skill")); err != nil {
		t.Fatalf("codex symlink removed despite error: %v", err)
	}
}
