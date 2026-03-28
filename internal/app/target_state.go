package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Z-Bra0/Ski/internal/fsutil"
)

const (
	targetStatusMissing          = "missing"
	targetStatusInstalled        = "installed"
	targetStatusDrifted          = "drifted"
	targetStatusUnexpectedEntry  = "unexpected entry"
	targetStatusStoreUnavailable = "store unavailable"
)

type targetInspection struct {
	Path   string
	Status string
}

func (s Service) inspectTarget(targetName, skillName, expectedPath string) (targetInspection, error) {
	dir, err := s.skillDir(targetName)
	if err != nil {
		return targetInspection{}, err
	}

	entryPath := filepath.Join(dir, skillName)
	info, err := os.Lstat(entryPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return targetInspection{Path: entryPath, Status: targetStatusMissing}, nil
	case err != nil:
		return targetInspection{}, fmt.Errorf("lstat %s: %w", entryPath, err)
	case !info.IsDir():
		return targetInspection{Path: entryPath, Status: targetStatusUnexpectedEntry}, nil
	case expectedPath == "":
		return targetInspection{Path: entryPath, Status: targetStatusInstalled}, nil
	}

	same, err := sameDirContents(entryPath, expectedPath)
	if err != nil {
		return targetInspection{}, fmt.Errorf("compare %s to %s: %w", entryPath, expectedPath, err)
	}
	if same {
		return targetInspection{Path: entryPath, Status: targetStatusInstalled}, nil
	}
	return targetInspection{Path: entryPath, Status: targetStatusDrifted}, nil
}

func sameDirContents(left, right string) (bool, error) {
	leftHash, err := fsutil.HashDir(left)
	if err != nil {
		return false, err
	}
	rightHash, err := fsutil.HashDir(right)
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func driftedTargetError(path string) error {
	return fmt.Errorf("target %s was modified and no longer matches the locked skill contents", path)
}

func unexpectedTargetEntryError(path string) error {
	return fmt.Errorf("%s already exists and is not a managed skill directory", path)
}
