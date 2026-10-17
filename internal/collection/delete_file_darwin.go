//go:build darwin

package collection

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const deleteEntryLimit = 10000
const deleteDepthLimit = 128
const deleteFileLimit int64 = 256 << 20
const deleteByteLimit int64 = 1 << 30

type deleteStamp struct {
	Path                                         string
	Device                                       int32
	Inode                                        uint64
	BirthSec, BirthNano                          int64
	Mode                                         uint16
	UID, GID                                     uint32
	Flags                                        uint32
	Links                                        uint16
	Size                                         int64
	ModifySec, ModifyNano, ChangeSec, ChangeNano int64
}

func deleteStat(path string, st unix.Stat_t) deleteStamp {
	return deleteStamp{path, st.Dev, st.Ino, st.Btim.Sec, st.Btim.Nsec, st.Mode, st.Uid, st.Gid, st.Flags, st.Nlink, st.Size, st.Mtim.Sec, st.Mtim.Nsec, st.Ctim.Sec, st.Ctim.Nsec}
}
func deleteObject(a, b deleteStamp) bool {
	return a.Device == b.Device && a.Inode == b.Inode && a.BirthSec == b.BirthSec && a.BirthNano == b.BirthNano && a.Mode == b.Mode && a.UID == b.UID && a.GID == b.GID && a.Flags == b.Flags
}
func deleteACLAbsent(fd int) bool {
	attributes := struct {
		Count, Reserved                       uint16
		Common, Volume, Directory, File, Fork uint32
	}{Count: 5, Common: 0x00400000}
	var result [12]byte
	_, _, errno := syscall.Syscall6(syscall.SYS_FGETATTRLIST, uintptr(fd), uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)), 0, 0)
	return errno == 0 && binary.LittleEndian.Uint32(result[:4]) == 12 && binary.LittleEndian.Uint32(result[8:]) == 0
}
func deleteOwned(st unix.Stat_t, fd int, symlink bool) error {
	if st.Uid != uint32(os.Geteuid()) {
		return errors.New("ownership is not established for effective UID")
	}
	if !symlink && st.Mode&0022 != 0 {
		return errors.New("group or world writable content is unsafe")
	}
	if st.Flags&(unix.UF_IMMUTABLE|unix.UF_APPEND|unix.SF_IMMUTABLE|unix.SF_APPEND) != 0 {
		return errors.New("immutable or append-only flags prevent safe removal")
	}
	if !deleteACLAbsent(fd) {
		return errors.New("ACL ownership metadata is present or cannot be inspected safely")
	}
	return nil
}
func deleteOpenAt(parent int, name string, kind uint16) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
	if kind == unix.S_IFDIR {
		flags |= unix.O_DIRECTORY
	}
	if kind == unix.S_IFLNK {
		flags = (flags &^ unix.O_NOFOLLOW) | unix.O_SYMLINK
	}
	fd, err := unix.Openat(parent, name, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
func deleteNames(dir *os.File) ([]string, error) {
	fd, err := unix.Openat(int(dir.Fd()), ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), dir.Name())
	defer f.Close()
	names, err := f.Readdirnames(deleteEntryLimit + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(names) > deleteEntryLimit {
		return nil, fmt.Errorf("exact inventory exceeds %d entries", deleteEntryLimit)
	}
	sort.Strings(names)
	return names, nil
}
func inventoryDelete(ctx context.Context, target DeleteTarget) ([]DeleteEntry, []deleteStamp, DeleteOwnership, error) {
	owner := DeleteOwnership{State: "unverified", Reason: "positive local filesystem ownership has not been established", UID: os.Geteuid()}
	var entries []DeleteEntry
	var ancestors []deleteStamp
	fail := func(err error) ([]DeleteEntry, []deleteStamp, DeleteOwnership, error) {
		return entries, ancestors, owner, err
	}
	if target.Path == "/" || !filepath.IsAbs(target.Path) {
		return fail(errors.New("unsafe filesystem root or noncanonical target"))
	}
	private := false
	for path := filepath.Dir(target.Path); ; path = filepath.Dir(path) {
		f, err := openAnchored(path, true)
		if err != nil {
			return fail(fmt.Errorf("ownership ancestor %s: %w", path, err))
		}
		var st unix.Stat_t
		err = unix.Fstat(int(f.Fd()), &st)
		if err == nil && !deleteACLAbsent(int(f.Fd())) {
			err = errors.New("ancestor ACL metadata is present or unverified")
		}
		f.Close()
		if err != nil {
			return fail(err)
		}
		if st.Flags&(unix.UF_IMMUTABLE|unix.UF_APPEND|unix.SF_IMMUTABLE|unix.SF_APPEND) != 0 {
			return fail(errors.New("unsafe immutable or append-only boundary ancestor"))
		}
		stamp := deleteStat(path, st)
		stamp.Size = 0
		stamp.Links = 0
		stamp.ModifySec = 0
		stamp.ModifyNano = 0
		stamp.ChangeSec = 0
		stamp.ChangeNano = 0
		ancestors = append(ancestors, stamp)
		if path == filepath.Dir(target.Path) && st.Uid != uint32(os.Geteuid()) {
			return fail(errors.New("bundle parent must be controlled by effective UID"))
		}
		if st.Uid != 0 && st.Uid != uint32(os.Geteuid()) {
			return fail(errors.New("ancestor is controlled by another principal"))
		}
		if st.Mode&0022 != 0 && !(private && st.Uid == 0 && st.Mode&unix.S_ISVTX != 0) {
			return fail(errors.New("unsafe shared-writable boundary ancestor"))
		}
		if st.Uid == uint32(os.Geteuid()) && st.Mode&0022 == 0 {
			private = true
		}
		if path == "/" {
			break
		}
	}
	parent, err := openAnchored(filepath.Dir(target.Path), true)
	if err != nil {
		return fail(err)
	}
	defer parent.Close()
	var total int64
	var device int32
	var walk func(int, string, string, int) error
	walk = func(parent int, name, relative string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > deleteDepthLimit {
			return fmt.Errorf("exact inventory exceeds depth %d", deleteDepthLimit)
		}
		if len(entries) >= deleteEntryLimit {
			return fmt.Errorf("exact inventory exceeds %d entries", deleteEntryLimit)
		}
		var st unix.Stat_t
		if err := unix.Fstatat(parent, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		kind := st.Mode & unix.S_IFMT
		if kind != unix.S_IFDIR && kind != unix.S_IFREG && kind != unix.S_IFLNK {
			return fmt.Errorf("special entry cannot be deleted safely: %s", relative)
		}
		if relative == "." {
			device = st.Dev
			if len(ancestors) == 0 || ancestors[0].Device != device {
				return errors.New("target crosses a mount or device boundary")
			}
			if target.Kind == "directory" && kind != unix.S_IFDIR || target.Kind == "standalone" && kind != unix.S_IFREG {
				return errors.New("target kind changed")
			}
			if sourceID(objectToken{uint64(st.Dev), st.Ino, st.Btim.Sec, st.Btim.Nsec}) != target.Source {
				return errors.New("target identity changed")
			}
			if target.Kind == "standalone" && (st.Nlink != 1 || !strings.EqualFold(filepath.Ext(target.Path), ".md")) {
				return errors.New("standalone deletion requires a single-link regular Markdown source")
			}
		}
		if st.Dev != device {
			return errors.New("exact inventory crosses a mount or device boundary")
		}
		f, err := deleteOpenAt(parent, name, kind)
		if err != nil {
			return err
		}
		defer f.Close()
		var held unix.Stat_t
		if err := unix.Fstat(int(f.Fd()), &held); err != nil {
			return err
		}
		stamp := deleteStat(relative, st)
		if stamp != deleteStat(relative, held) {
			return errors.New("entry changed during inventory open")
		}
		if err := deleteOwned(st, int(f.Fd()), kind == unix.S_IFLNK); err != nil {
			return fmt.Errorf("%s: %w", relative, err)
		}
		entry := DeleteEntry{Path: relative, Size: st.Size, Links: st.Nlink, stamp: stamp}
		switch kind {
		case unix.S_IFDIR:
			entry.Kind = "directory"
		case unix.S_IFREG:
			entry.Kind = "file"
			if st.Size > deleteFileLimit {
				return fmt.Errorf("%s exceeds 256 MiB file limit", relative)
			}
			total += st.Size
			if total > deleteByteLimit {
				return errors.New("exact inventory exceeds 1 GiB aggregate hashed bytes")
			}
			hash := sha256.New()
			n, e := io.Copy(hash, io.LimitReader(f, deleteFileLimit+1))
			if e != nil {
				return e
			}
			if n != st.Size || n > deleteFileLimit {
				return errors.New("file size changed while hashing")
			}
			entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
		case unix.S_IFLNK:
			entry.Kind = "symlink"
			buf := make([]byte, 4096)
			n, e := unix.Readlinkat(parent, name, buf)
			if e != nil {
				return e
			}
			if n == len(buf) {
				return errors.New("symlink text exceeds inspection limit")
			}
			entry.LinkTarget = string(buf[:n])
		}
		entries = append(entries, entry)
		if kind == unix.S_IFDIR {
			names, err := deleteNames(f)
			if err != nil {
				return err
			}
			if relative != "." {
				for _, child := range names {
					if child == "SKILL.md" {
						return fmt.Errorf("boundary contains another bundle manifest: %s", relative)
					}
				}
			}
			for _, child := range names {
				rel := child
				if relative != "." {
					rel = filepath.Join(relative, child)
				}
				if err := walk(int(f.Fd()), child, rel, depth+1); err != nil {
					return err
				}
			}
		}
		var after unix.Stat_t
		if err := unix.Fstat(int(f.Fd()), &after); err != nil {
			return err
		}
		if stamp != deleteStat(relative, after) {
			return errors.New("entry changed during exact inventory")
		}
		var binding unix.Stat_t
		if err := unix.Fstatat(parent, name, &binding, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if stamp != deleteStat(relative, binding) {
			return errors.New("entry binding changed during exact inventory")
		}
		return nil
	}
	if err := walk(int(parent.Fd()), filepath.Base(target.Path), ".", 0); err != nil {
		return fail(err)
	}
	owner.State = "local-user-controlled"
	owner.Reason = "effective UID owns target and all entries; private boundary parent, trusted ancestors, no writable grants, ACLs, unsafe flags, special entries or crossed devices"
	return entries, ancestors, owner, nil
}

type deleteBinding struct {
	parent *os.File
	name   string
	stamp  deleteStamp
}

func (p *DeletePreview) remove(ctx context.Context, out DeleteOutcome) DeleteOutcome {
	out.State = "failed"
	out.Reason = "removal did not start"
	parent, err := openAnchored(filepath.Dir(p.view.Target.Path), true)
	if err != nil {
		out.Reason = err.Error()
		out.Unverified = append(out.Unverified, ".")
		return out
	}
	defer parent.Close()
	if err = unix.Flock(int(parent.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		out.Reason = "another cooperating writer holds the parent lock"
		return out
	}
	defer unix.Flock(int(parent.Fd()), unix.LOCK_UN)
	expected := map[string]DeleteEntry{}
	for _, e := range p.view.Contents {
		expected[e.Path] = e
	}
	removed := map[string]bool{}
	bindings := []deleteBinding{}
	validate := func() error {
		anchored, e := openAnchored(filepath.Dir(p.view.Target.Path), true)
		if e != nil {
			return e
		}
		defer anchored.Close()
		var st unix.Stat_t
		if e = unix.Fstat(int(anchored.Fd()), &st); e != nil {
			return e
		}
		if len(p.ancestors) == 0 || !deleteObject(p.ancestors[0], deleteStat(p.ancestors[0].Path, st)) {
			return errors.New("canonical parent binding changed")
		}
		for _, ancestor := range p.ancestors {
			held, err := openAnchored(ancestor.Path, true)
			if err != nil {
				return err
			}
			var current unix.Stat_t
			err = unix.Fstat(int(held.Fd()), &current)
			acl := deleteACLAbsent(int(held.Fd()))
			held.Close()
			if err != nil {
				return err
			}
			if !acl || !deleteObject(ancestor, deleteStat(ancestor.Path, current)) {
				return errors.New("ancestor ownership or binding changed")
			}
		}
		for _, b := range bindings {
			if e = unix.Fstatat(int(b.parent.Fd()), b.name, &st, unix.AT_SYMLINK_NOFOLLOW); e != nil {
				return e
			}
			if !deleteObject(b.stamp, deleteStat(b.stamp.Path, st)) {
				return errors.New("planned parent-child binding changed")
			}
			if b.stamp.Mode&unix.S_IFMT == unix.S_IFDIR {
				f, err := deleteOpenAt(int(b.parent.Fd()), b.name, unix.S_IFDIR)
				if err != nil {
					return err
				}
				names, err := deleteNames(f)
				f.Close()
				if err != nil {
					return err
				}
				planned := []string{}
				for path := range expected {
					if path != "." && !removed[path] && filepath.Dir(path) == b.stamp.Path {
						planned = append(planned, filepath.Base(path))
					}
				}
				sort.Strings(planned)
				if !reflect.DeepEqual(names, planned) {
					return errors.New("directory membership changed before removal")
				}
			}
		}
		return nil
	}
	checkpoint := func(phase, path string) error {
		if p.checkpoint != nil {
			return p.checkpoint(phase, path)
		}
		return nil
	}
	var erase func(*os.File, string, string) error
	erase = func(dir *os.File, name, relative string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkpoint("before-validation", relative); err != nil {
			return err
		}
		if err := validate(); err != nil {
			return err
		}
		e := expected[relative]
		var st unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if e.stamp != deleteStat(relative, st) {
			return fmt.Errorf("planned entry changed: %s", relative)
		}
		f, err := deleteOpenAt(int(dir.Fd()), name, st.Mode&unix.S_IFMT)
		if err != nil {
			return err
		}
		defer f.Close()
		var held unix.Stat_t
		if err = unix.Fstat(int(f.Fd()), &held); err != nil {
			return err
		}
		if e.stamp != deleteStat(relative, held) {
			return errors.New("entry changed during removal open")
		}
		if err = deleteOwned(held, int(f.Fd()), e.Kind == "symlink"); err != nil {
			return err
		}
		bindings = append(bindings, deleteBinding{dir, name, e.stamp})
		defer func() { bindings = bindings[:len(bindings)-1] }()
		if e.Kind == "directory" {
			names, err := deleteNames(f)
			if err != nil {
				return err
			}
			planned := []string{}
			for path := range expected {
				if path != "." && filepath.Dir(path) == relative || relative == "." && path != "." && filepath.Dir(path) == "." {
					planned = append(planned, filepath.Base(path))
				}
			}
			sort.Strings(planned)
			if !reflect.DeepEqual(names, planned) {
				return fmt.Errorf("directory membership changed: %s", relative)
			}
			for _, child := range names {
				path := child
				if relative != "." {
					path = filepath.Join(relative, child)
				}
				if err := erase(f, child, path); err != nil {
					return err
				}
			}
			names, err = deleteNames(f)
			if err != nil {
				return err
			}
			if len(names) != 0 {
				return errors.New("new directory membership observed; stopping")
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkpoint("before-unlink-validation", relative); err != nil {
			return err
		}
		if err := validate(); err != nil {
			return err
		}
		if err := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if !deleteObject(e.stamp, deleteStat(relative, st)) || e.Kind != "directory" && e.stamp != deleteStat(relative, st) {
			return fmt.Errorf("planned entry changed before unlink: %s", relative)
		}
		if err := checkpoint("after-unlink-validation", relative); err != nil {
			return err
		}
		flags := 0
		if e.Kind == "directory" {
			flags = unix.AT_REMOVEDIR
		}
		if err := unix.Unlinkat(int(dir.Fd()), name, flags); err != nil {
			return err
		}
		if err := unix.Fstat(int(f.Fd()), &held); err != nil {
			return fmt.Errorf("unlink succeeded but original object cannot be verified: %s", relative)
		}
		// Holding the object distinguishes removal of a substituted name from unlinking the consented object.
		if !deleteUnlinked(f, e.Kind, filepath.Join(p.view.Target.Path, relative), st, held) {
			out.Unverified = append(out.Unverified, relative)
			return fmt.Errorf("unlink succeeded but original object link removal is unverified: %s", relative)
		}
		removed[relative] = true
		out.Removed = append(out.Removed, relative)
		if err := checkpoint("after-unlink", relative); err != nil {
			return err
		}
		return nil
	}
	// Recheck the exact tree after acquiring the cooperative lock and before any syscall can remove content.
	entries, ancestors, _, err := inventoryDelete(ctx, p.view.Target)
	if err == nil && (!reflect.DeepEqual(entries, p.view.Contents) || !reflect.DeepEqual(ancestors, p.ancestors)) {
		err = errors.New("exact target inventory changed before removal")
	}
	if err == nil {
		err = erase(parent, filepath.Base(p.view.Target.Path), ".")
	}
	if err != nil {
		out.Reason = err.Error()
		if len(out.Removed) > 0 || len(out.Unverified) > 0 {
			out.State = "partial"
		}
		if ctx.Err() != nil && len(out.Removed) == 0 {
			out.State = "cancelled"
		}
	}
	p.inspectRemoval(parent, expected, removed, &out)
	if err == nil && len(out.Removed) == len(expected) && len(out.Remaining) == 0 && len(out.Unverified) == 0 && len(out.Replacements) == 0 {
		out.State = "removed"
		out.Reason = "selected content removed and original parent name verified absent; references and settings left untouched"
		out.Verified = true
	}
	if err == nil && !out.Verified {
		out.State = "partial"
		out.Reason = "removal operations finished but final absence or original-object accounting is unverified"
	}
	return out
}
func (p *DeletePreview) inspectRemoval(parent *os.File, expected map[string]DeleteEntry, removed map[string]bool, out *DeleteOutcome) {
	observed := map[string]bool{}
	var inspect func(*os.File, string, string)
	inspect = func(dir *os.File, name, path string) {
		observed[path] = true
		var st unix.Stat_t
		err := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, unix.ENOENT) {
			for candidate := range expected {
				if (candidate == path || deleteContains(path, candidate)) && !removed[candidate] {
					out.Unverified = append(out.Unverified, candidate)
				}
			}
			return
		}
		if err != nil {
			out.Unverified = append(out.Unverified, path)
			return
		}
		e, known := expected[path]
		if !known || !deleteObject(e.stamp, deleteStat(path, st)) {
			out.Replacements = append(out.Replacements, path)
			for candidate := range expected {
				if (candidate == path || deleteContains(path, candidate)) && !removed[candidate] {
					out.Unverified = append(out.Unverified, candidate)
				}
			}
			return
		}
		out.Remaining = append(out.Remaining, path)
		if st.Mode&unix.S_IFMT != unix.S_IFDIR {
			return
		}
		f, err := deleteOpenAt(int(dir.Fd()), name, unix.S_IFDIR)
		if err != nil {
			out.Unverified = append(out.Unverified, path)
			return
		}
		defer f.Close()
		names, err := deleteNames(f)
		if err != nil {
			out.Unverified = append(out.Unverified, path)
			return
		}
		for _, child := range names {
			relative := child
			if path != "." {
				relative = filepath.Join(path, child)
			}
			inspect(f, child, relative)
		}
	}
	inspect(parent, filepath.Base(p.view.Target.Path), ".")
	for path := range expected {
		if !removed[path] && !observed[path] {
			out.Unverified = append(out.Unverified, path)
		}
	}
	out.Remaining = deleteUniquePaths(out.Remaining)
	out.Unverified = deleteUniquePaths(out.Unverified)
	out.Replacements = deleteUniquePaths(out.Replacements)
}

func deleteUnlinked(f *os.File, kind, expectedPath string, before, after unix.Stat_t) bool {
	if kind != "directory" {
		return after.Nlink < before.Nlink
	}
	var path [1024]byte
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), uintptr(unix.F_GETPATH), uintptr(unsafe.Pointer(&path[0])))
	if errno == syscall.ENOENT {
		return true
	}
	if errno != 0 {
		return false
	}
	end := 0
	for end < len(path) && path[end] != 0 {
		end++
	}
	return string(path[:end]) == expectedPath
}

func deleteUniquePaths(paths []string) []string {
	sort.Strings(paths)
	result := paths[:0]
	for _, path := range paths {
		if len(result) == 0 || result[len(result)-1] != path {
			result = append(result, path)
		}
	}
	return result
}
