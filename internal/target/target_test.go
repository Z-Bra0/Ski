package target_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ski/internal/target"
)

// claudeLink returns the expected symlink path for the "claude" target.
func claudeLink(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".claude", "skills", name)
}

func codexLink(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".codex", "skills", name)
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
