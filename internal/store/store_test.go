package store

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestMoveDirIntoStoreFallsBackOnCrossDeviceRename(t *testing.T) {
	t.Parallel()

	srcRoot := t.TempDir()
	dstRoot := t.TempDir()
	src := filepath.Join(srcRoot, "checkout")
	dst := filepath.Join(dstRoot, "store", "git", "repo-map", "commit")

	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("skill"), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "helper.sh"), []byte("echo helper\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(helper.sh) error = %v", err)
	}
	if err := os.Symlink("nested/helper.sh", filepath.Join(src, "helper")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("MkdirAll(store parent) error = %v", err)
	}

	origRename := renameDir
	t.Cleanup(func() {
		renameDir = origRename
	})

	callCount := 0
	renameDir = func(oldpath, newpath string) error {
		callCount++
		if callCount == 1 {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
		}
		return origRename(oldpath, newpath)
	}

	if err := moveDirIntoStore(src, dst); err != nil {
		t.Fatalf("moveDirIntoStore() error = %v", err)
	}
	if callCount < 2 {
		t.Fatalf("renameDir call count = %d, want fallback path", callCount)
	}

	data, err := os.ReadFile(filepath.Join(dst, "nested", "helper.sh"))
	if err != nil {
		t.Fatalf("ReadFile(helper.sh) error = %v", err)
	}
	if string(data) != "echo helper\n" {
		t.Fatalf("helper.sh = %q, want preserved content", string(data))
	}

	info, err := os.Stat(filepath.Join(dst, "nested", "helper.sh"))
	if err != nil {
		t.Fatalf("Stat(helper.sh) error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("helper.sh perms = %#o, want 0755", info.Mode().Perm())
	}

	linkTarget, err := os.Readlink(filepath.Join(dst, "helper"))
	if err != nil {
		t.Fatalf("Readlink(helper) error = %v", err)
	}
	if linkTarget != "nested/helper.sh" {
		t.Fatalf("helper symlink target = %q, want nested/helper.sh", linkTarget)
	}
}
