package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type consolidationSelection struct {
	SchemaVersion int      `json:"schema_version"`
	Applications  []string `json:"applications"`
	Roots         []string `json:"roots,omitempty"`
	Library       string   `json:"library"`
}

// LastConsolidationSelection reads only the remembered selection. It never
// creates the state directory or writes a default selection.
func LastConsolidationSelection() (apps, roots []string, library string, found bool, err error) {
	path, err := consolidationSelectionPath()
	if err != nil {
		return nil, nil, "", false, err
	}
	raw, found, err := readPrivateSelection(path)
	if err != nil || !found {
		return nil, nil, "", found, err
	}
	var value consolidationSelection
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, nil, "", false, fmt.Errorf("read remembered consolidation selection: %w", err)
	}
	if value.SchemaVersion != ConsolidationSchemaVersion || len(value.Applications) == 0 || value.Library == "" {
		return nil, nil, "", false, errors.New("remembered consolidation selection has an unsupported or incomplete schema")
	}
	return append([]string(nil), value.Applications...), append([]string(nil), value.Roots...), value.Library, true, nil
}

// CurrentConsolidationSelectionID pins the last applied selection as evidence.
// The stable absent marker lets an explicit-scope preview detect a concurrent
// first write before it replaces the remembered selection after apply.
func CurrentConsolidationSelectionID() (string, error) {
	apps, roots, library, found, err := LastConsolidationSelection()
	if err != nil {
		return "", err
	}
	if !found {
		return "selection1:none", nil
	}
	return rememberedSelectionID(apps, roots, library), nil
}

func RememberConsolidationSelection(request ConsolidationRequest) error {
	path, err := consolidationSelectionPath()
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := ensureConsolidationDirectory(directory, true); err != nil {
		return fmt.Errorf("create consolidation selection directory: %w", err)
	}
	value := consolidationSelection{SchemaVersion: ConsolidationSchemaVersion, Applications: append([]string(nil), request.Applications...), Roots: append([]string(nil), request.Roots...), Library: request.Library}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return writePrivateAtomic(path, encoded)
}

func consolidationSelectionPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base != "" && !filepath.IsAbs(base) {
		return "", errors.New("XDG_CONFIG_HOME must be absolute for consolidation selection state")
	}
	if base == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return "", errors.New("HOME is required for consolidation selection state")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "skillglass", "consolidation.json"), nil
}

func createPrivateDirectory(path string) error {
	return ensureConsolidationDirectory(path, true)
}

func writePrivateAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := createPrivateDirectory(dir); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return errors.New("private state target is an alias or not a regular file")
		}
		var stat unix.Stat_t
		if err := unix.Lstat(path, &stat); err != nil || stat.Uid != uint32(os.Geteuid()) {
			return errors.New("private state target is not owned by the effective user")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".consolidation-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	parent, err := openAnchored(dir, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	return unix.Fsync(int(parent.Fd()))
}

func readPrivateSelection(path string) ([]byte, bool, error) {
	if !filepath.IsAbs(path) {
		return nil, false, errors.New("remembered selection path is not absolute")
	}
	parent, err := openAnchored(filepath.Dir(path), true)
	if errors.Is(err, unix.ENOENT) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer parent.Close()
	var parentStat unix.Stat_t
	if err := unix.Fstat(int(parent.Fd()), &parentStat); err != nil {
		return nil, false, err
	}
	if parentStat.Uid != uint32(os.Geteuid()) || parentStat.Mode&0o077 != 0 {
		return nil, false, errors.New("remembered selection directory is not private to the effective user")
	}
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), filepath.Base(path), &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG || before.Uid != uint32(os.Geteuid()) || before.Mode&0o077 != 0 || before.Size > 64<<10 {
		return nil, false, errors.New("remembered selection is not a private bounded regular file")
	}
	fd, err := unix.Openat(int(parent.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, false, err
	}
	if opened.Dev != before.Dev || opened.Ino != before.Ino || opened.Uid != uint32(os.Geteuid()) || opened.Mode&0o077 != 0 || opened.Size > 64<<10 {
		return nil, false, errors.New("remembered selection changed during inspection")
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64<<10+1))
	if err != nil {
		return nil, false, err
	}
	if len(raw) > 64<<10 {
		return nil, false, errors.New("remembered consolidation selection exceeds 64 KiB")
	}
	return raw, true, nil
}

func consolidationStateOutsideRoots(request ConsolidationRequest, apps []ConsolidationApplication) []string {
	path, err := consolidationSelectionPath()
	if err != nil {
		return []string{err.Error()}
	}
	var reasons []string
	if resolved, resolveErr := setupProspectivePath(path); resolveErr != nil || filepath.Clean(resolved) != filepath.Clean(path) {
		reasons = append(reasons, "remembered selection state path or an ancestor is an alias or unresolved")
	}
	for _, app := range apps {
		for _, root := range app.Scope.Roots {
			if setupPathsOverlap(path, root.Requested) != deleteUnrelated {
				reasons = append(reasons, "remembered selection state overlaps "+app.Application+" discovery root "+root.Requested)
			}
		}
	}
	for _, root := range request.Roots {
		if setupPathsOverlap(path, root) != deleteUnrelated {
			reasons = append(reasons, "remembered selection state overlaps explicit discovery root "+root)
		}
	}
	return uniqueStrings(reasons)
}

func normalizeSelectionFrom(value string) string {
	if value == "remembered" || strings.EqualFold(value, "remembered") {
		return "remembered"
	}
	return "arguments"
}
