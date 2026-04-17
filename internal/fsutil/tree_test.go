package fsutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Z-Bra0/Ski/internal/fsutil"
)

// makeTree creates a directory tree under root from a map of
// relative path -> content (empty string = directory, non-empty = file data).
func makeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, data := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if data == "" {
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q): %v", abs, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatalf("MkdirAll(%q): %v", filepath.Dir(abs), err)
			}
			if err := os.WriteFile(abs, []byte(data), 0o644); err != nil {
				t.Fatalf("WriteFile(%q): %v", abs, err)
			}
		}
	}
}

// assertCopyTreeRejectsSymlink runs CopyTree on src and asserts it fails with
// ErrSymlinkNotPermitted.
func assertCopyTreeRejectsSymlink(t *testing.T, src string) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "dst")
	err := fsutil.CopyTree(src, dst)
	if err == nil {
		t.Fatal("CopyTree() error = nil, want ErrSymlinkNotPermitted")
	}
	if !errors.Is(err, fsutil.ErrSymlinkNotPermitted) {
		t.Fatalf("CopyTree() error = %v, want ErrSymlinkNotPermitted", err)
	}
}

func TestCopyTreeRejectsAbsoluteSymlinkTarget(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	makeTree(t, src, map[string]string{"file.txt": "hello"})
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertCopyTreeRejectsSymlink(t, src)
}

func TestCopyTreeRejectsRelativeSymlinkEscapingRoot(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	makeTree(t, src, map[string]string{"file.txt": "hello"})
	if err := os.Symlink("../outside", filepath.Join(src, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertCopyTreeRejectsSymlink(t, src)
}

func TestCopyTreeRejectsSymlinkInSubdir(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "subdir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("../../../outside", filepath.Join(src, "subdir", "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertCopyTreeRejectsSymlink(t, src)
}

func TestCopyTreeRejectsInTreeSymlink(t *testing.T) {
	t.Parallel()
	// Even a symlink pointing at a sibling inside the tree is rejected —
	// skill trees must contain only regular files and directories.
	src := t.TempDir()
	makeTree(t, src, map[string]string{"target.txt": "content"})
	if err := os.Symlink("target.txt", filepath.Join(src, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	assertCopyTreeRejectsSymlink(t, src)
}

func TestCopyTreeCopiesRegularFilesAndDirs(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dst")

	makeTree(t, src, map[string]string{
		"SKILL.md":          "---\nname: example\n---\n",
		"subdir/helper.md":  "helper content",
		"subdir/nested/foo": "deep file",
	})

	if err := fsutil.CopyTree(src, dst); err != nil {
		t.Fatalf("CopyTree() error = %v", err)
	}

	for _, rel := range []string{"SKILL.md", "subdir/helper.md", "subdir/nested/foo"} {
		data, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", rel, err)
		}
		want, _ := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel)))
		if string(data) != string(want) {
			t.Fatalf("file %q: got %q, want %q", rel, data, want)
		}
	}
}

func TestHashDirRejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeTree(t, root, map[string]string{"file.txt": "hello"})
	if err := os.Symlink("file.txt", filepath.Join(root, "link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := fsutil.HashDir(root)
	if err == nil {
		t.Fatal("HashDir() error = nil, want ErrSymlinkNotPermitted")
	}
	if !errors.Is(err, fsutil.ErrSymlinkNotPermitted) {
		t.Fatalf("HashDir() error = %v, want ErrSymlinkNotPermitted", err)
	}
}
