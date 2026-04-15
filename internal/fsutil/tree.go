package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrSymlinkNotPermitted is returned by HashDir and CopyTree when a symlink is
// encountered. Skill trees must contain only regular files and directories.
var ErrSymlinkNotPermitted = errors.New("symlink not permitted in skill trees")

// HashDir returns a canonical SHA-256 hash for a directory tree.
func HashDir(root string) (string, error) {
	hasher := sha256.New()
	if err := hashDir(hasher, root, ""); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashDir(w io.Writer, absPath, relPath string) error {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return err
	}

	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	for _, entry := range entries {
		name := entry.Name()
		childAbs := filepath.Join(absPath, name)
		childRel := filepath.ToSlash(filepath.Join(relPath, name))

		info, err := entry.Info()
		if err != nil {
			return err
		}

		switch {
		case entry.IsDir():
			if _, err := io.WriteString(w, "dir\x00"+childRel+"\x00"); err != nil {
				return err
			}
			if err := hashDir(w, childAbs, childRel); err != nil {
				return err
			}
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%q: %w", childAbs, ErrSymlinkNotPermitted)
		default:
			if _, err := io.WriteString(w, "file\x00"+childRel+"\x00"); err != nil {
				return err
			}
			data, err := os.ReadFile(childAbs)
			if err != nil {
				return err
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\x00"); err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyTree copies a directory tree, preserving file permissions.
// Symlinks are rejected: skill trees must contain only regular files and
// directories to prevent symlink-escape attacks.
func CopyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		entryInfo, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case entryInfo.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%q: %w", srcPath, ErrSymlinkNotPermitted)
		case entryInfo.IsDir():
			if err := CopyTree(srcPath, dstPath); err != nil {
				return err
			}
			if err := os.Chmod(dstPath, entryInfo.Mode().Perm()); err != nil {
				return err
			}
		default:
			if err := copyFile(srcPath, dstPath, entryInfo.Mode().Perm()); err != nil {
				return err
			}
		}
	}

	return os.Chmod(dst, info.Mode().Perm())
}

func copyFile(src, dst string, perm os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
