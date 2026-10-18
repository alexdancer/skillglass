//go:build darwin

package setup

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexdancer/skillglass/internal/companion"
	"golang.org/x/sys/unix"
)

// Every directory hop is descriptor-relative and refuses symlinks. Publication
// uses RENAME_EXCL, preserving any competing/customized destination.
func installSetup(home, destination string) error {
	if err := safeSetupPath(home, destination); err != nil {
		return err
	}
	rel, err := filepath.Rel(home, filepath.Dir(destination))
	if err != nil {
		return err
	}
	fd, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { unix.Close(fd) }()
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if errors.Is(e, unix.ENOENT) {
			if e = unix.Mkdirat(fd, part, 0700); e != nil && !errors.Is(e, unix.EEXIST) {
				return e
			}
			next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
		if e != nil {
			return e
		}
		unix.Close(fd)
		fd = next
	}
	var random [12]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	stage := "." + companion.Name + "-setup-" + hex.EncodeToString(random[:])
	if err = unix.Mkdirat(fd, stage, 0700); err != nil {
		return err
	}
	directories := bundleDirectories()
	stageFD, err := unix.Openat(fd, stage, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Unlinkat(fd, stage, unix.AT_REMOVEDIR)
		return err
	}
	defer unix.Close(stageFD)
	published := false
	// Cleanup touches only this unpublished, exclusively created staging tree.
	defer func() {
		if published {
			return
		}
		for _, file := range companion.Bundled().Files {
			unix.Unlinkat(stageFD, file.Path, 0)
		}
		for i := len(directories) - 1; i >= 0; i-- {
			unix.Unlinkat(stageFD, directories[i], unix.AT_REMOVEDIR)
		}
		unix.Unlinkat(fd, stage, unix.AT_REMOVEDIR)
	}()
	for _, directory := range directories {
		if err = unix.Mkdirat(stageFD, directory, 0700); err != nil {
			return err
		}
	}
	for _, file := range companion.Bundled().Files {
		child, e := unix.Openat(stageFD, file.Path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if e != nil {
			return e
		}
		output := os.NewFile(uintptr(child), file.Path)
		_, err = output.WriteString(file.Content)
		if err == nil {
			err = output.Sync()
		}
		closeErr := output.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err = unix.Fsync(stageFD); err != nil {
		return err
	}
	err = unix.RenameatxNp(fd, stage, fd, filepath.Base(destination), unix.RENAME_EXCL)
	published = err == nil
	if err == nil {
		err = unix.Fsync(fd)
	}
	return err
}

func bundleDirectories() []string {
	unique := map[string]bool{}
	for _, file := range companion.Bundled().Files {
		for parent := filepath.Dir(file.Path); parent != "."; parent = filepath.Dir(parent) {
			unique[parent] = true
		}
	}
	result := make([]string, 0, len(unique))
	for directory := range unique {
		result = append(result, directory)
	}
	sort.Strings(result)
	return result
}
