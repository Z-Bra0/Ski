package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"ski/internal/skill"
	"ski/internal/source"
)

var renameDir = os.Rename

type Result struct {
	Commit    string
	Integrity string
	Path      string
}

func EnsureGit(projectRoot, homeDir string, spec source.Git, expectedName string) (Result, error) {
	storeKey, err := spec.DeriveName()
	if err != nil {
		return Result{}, err
	}

	tmpRoot, err := os.MkdirTemp("", "ski-git-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	checkoutDir := filepath.Join(tmpRoot, "checkout")
	if err := runGit(projectRoot, "clone", "--quiet", spec.URL, checkoutDir); err != nil {
		return Result{}, fmt.Errorf("clone %q: %w", spec.URL, err)
	}

	if spec.Ref != "" {
		if err := runGit(projectRoot, "-C", checkoutDir, "-c", "advice.detachedHead=false", "checkout", "--quiet", spec.Ref); err != nil {
			return Result{}, fmt.Errorf("checkout %q: %w", spec.Ref, err)
		}
	}

	commit, err := gitOutput(projectRoot, "-C", checkoutDir, "rev-parse", "HEAD")
	if err != nil {
		return Result{}, fmt.Errorf("resolve commit: %w", err)
	}

	storePath := filepath.Join(homeDir, ".ski", "store", "git", storeKey, commit)
	if _, err := os.Stat(storePath); err == nil {
		if _, err := skill.ValidateDir(storePath, expectedName); err != nil {
			return Result{}, err
		}
		integrity, err := HashDir(storePath)
		if err != nil {
			return Result{}, fmt.Errorf("hash %s: %w", storePath, err)
		}
		return Result{Commit: commit, Integrity: integrity, Path: storePath}, nil
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat %s: %w", storePath, err)
	}

	if _, err := skill.ValidateDir(checkoutDir, expectedName); err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(filepath.Dir(storePath), 0o755); err != nil {
		return Result{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(storePath), err)
	}
	if err := os.RemoveAll(filepath.Join(checkoutDir, ".git")); err != nil {
		return Result{}, fmt.Errorf("remove git metadata: %w", err)
	}
	if err := moveDirIntoStore(checkoutDir, storePath); err != nil {
		return Result{}, fmt.Errorf("move checkout into store: %w", err)
	}

	integrity, err := HashDir(storePath)
	if err != nil {
		return Result{}, fmt.Errorf("hash %s: %w", storePath, err)
	}

	return Result{Commit: commit, Integrity: integrity, Path: storePath}, nil
}

func moveDirIntoStore(src, dst string) error {
	if err := renameDir(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	stageRoot, err := os.MkdirTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-tmp-")
	if err != nil {
		return fmt.Errorf("create store staging dir: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	stagePath := filepath.Join(stageRoot, filepath.Base(dst))
	if err := copyTree(src, stagePath); err != nil {
		return fmt.Errorf("copy checkout into store staging dir: %w", err)
	}
	if err := renameDir(stagePath, dst); err != nil {
		return fmt.Errorf("finalize staged store dir: %w", err)
	}
	return nil
}

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
			target, err := os.Readlink(childAbs)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(w, "symlink\x00"+childRel+"\x00"+target+"\x00"); err != nil {
				return err
			}
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

func copyTree(src, dst string) error {
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

		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyTree(srcPath, dstPath); err != nil {
				return err
			}
			if err := os.Chmod(dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		default:
			if err := copyFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
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

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
