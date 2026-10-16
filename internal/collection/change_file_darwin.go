//go:build darwin

package collection

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type changeIdentity struct {
	Path                string
	Device              int32
	Inode               uint64
	BirthSec, BirthNano int64
	Mode                uint32
	UID, GID            uint32
}
type changeFact struct {
	Path                  string
	Resolved              string
	Exists                bool
	Directory             bool
	Identity              changeIdentity
	Links                 uint16
	Flags                 uint32
	ChangeSec, ChangeNano int64
	Symlink               string
	Chain                 []changeIdentity
	Attributes            map[string][]byte
	Bytes                 []byte
}
type changeDestination struct {
	Path            string
	Parent          changeFact
	Config          changeFact
	CreateDirectory string
}

func statIdentity(path string, stat *syscall.Stat_t) changeIdentity {
	return changeIdentity{path, stat.Dev, stat.Ino, stat.Birthtimespec.Sec, stat.Birthtimespec.Nsec, uint32(stat.Mode), stat.Uid, stat.Gid}
}
func observeChangePath(path string) (changeFact, error) {
	fact := changeFact{Path: filepath.Clean(path)}
	// Resolution of every existing ancestor is part of consent, including directory aliases.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err == nil {
			stat := info.Sys().(*syscall.Stat_t)
			fact.Chain = append(fact.Chain, statIdentity(parent, stat))
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, e := filepath.EvalSymlinks(parent)
				if e != nil {
					return fact, e
				}
				token, e := os.Stat(resolved)
				if e != nil {
					return fact, e
				}
				fact.Chain = append(fact.Chain, statIdentity(resolved, token.Sys().(*syscall.Stat_t)))
			}
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fact, err
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fact, nil
	}
	if err != nil {
		return fact, err
	}
	fact.Exists = true
	stat := info.Sys().(*syscall.Stat_t)
	fact.Identity = statIdentity(path, stat)
	if !info.IsDir() {
		fact.Links = stat.Nlink
	}
	fact.Flags = stat.Flags
	fact.Directory = info.IsDir()
	if !fact.Directory {
		fact.ChangeSec = stat.Ctimespec.Sec
		fact.ChangeNano = stat.Ctimespec.Nsec
	}
	fact.Resolved, err = filepath.EvalSymlinks(path)
	if err != nil {
		return fact, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		fact.Symlink, err = os.Readlink(path)
		if err != nil {
			return fact, err
		}
		return fact, nil
	}
	if info.Mode().IsRegular() {
		fact.Attributes, err = changeAttributes(path)
		if err != nil {
			return fact, err
		}
		fact.Bytes, err = readChangeRegular(path, fact.Identity)
		if err != nil {
			return fact, err
		}
		after, e := os.Lstat(path)
		if e != nil || statIdentity(path, after.Sys().(*syscall.Stat_t)) != fact.Identity || after.Sys().(*syscall.Stat_t).Ctimespec != stat.Ctimespec {
			return fact, errors.New("file changed while being observed")
		}
	}
	return fact, nil
}
func inspectChangeDestination(codexHome string) (changeDestination, error) {
	return inspectChangeDestinationPath(filepath.Join(codexHome, "config.toml"))
}
func inspectChangeDestinationPath(path string) (changeDestination, error) {
	codexHome := filepath.Dir(path)
	result := changeDestination{Path: path}
	root, err := observeChangePath(codexHome)
	if err != nil {
		return result, errors.New("configuration directory cannot be resolved safely")
	}
	if !root.Exists {
		parent, err := observeChangePath(filepath.Dir(codexHome))
		if err != nil || !parent.Directory {
			return result, errors.New("missing configuration directory requires an existing immediate parent; recursive creation is unsupported")
		}
		result.Parent = parent
		result.CreateDirectory = codexHome
	} else {
		resolved, err := filepath.EvalSymlinks(codexHome)
		if err != nil {
			return result, err
		}
		parent, err := observeChangePath(resolved)
		if err != nil || !parent.Directory {
			return result, errors.New("configuration directory must resolve to a stable directory")
		}
		result.Parent = parent
	}
	if result.Parent.Identity.UID != uint32(os.Geteuid()) || result.Parent.Identity.Mode&0022 != 0 || result.Parent.Flags != 0 {
		return result, errors.New("configuration parent ownership, permissions or file flags are unsafe")
	}
	if !changeACLAbsent(result.Parent.Resolved) {
		return result, errors.New("configuration parent ACL metadata cannot be preserved safely")
	}
	result.Config, err = observeChangePath(result.Path)
	if err != nil {
		return result, errors.New("configuration is unreadable or has unsafe links")
	}
	if result.Config.Exists {
		fact := result.Config
		if fact.Symlink != "" || fact.Directory || fact.Identity.Mode&unix.S_IFMT != unix.S_IFREG || fact.Links != 1 {
			return result, errors.New("configuration must be a regular single-link file; config symlinks and hardlinks are refused")
		}
		if fact.Identity.UID != uint32(os.Geteuid()) || fact.Flags != 0 || fact.Identity.Mode&07000 != 0 {
			return result, errors.New("configuration ownership or file flags cannot be preserved safely")
		}
		if !changeACLAbsent(result.Path) {
			return result, errors.New("configuration ACL metadata cannot be preserved by this writer")
		}
	}
	return result, nil
}
func (p *changePlan) checkpoint(name string) error {
	if p.fault != nil {
		return p.fault(name)
	}
	return nil
}
func lockChangeDirectory(ctx context.Context, fd int) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("configuration directory is busy; request a fresh preview")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func sameChangeFacts(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return bytes.Equal(a, b)
}
func (p *changePlan) apply(ctx context.Context) (out ChangeOutcome) {
	out = ChangeOutcome{State: "failed", Reason: "configuration write failed", Intent: p.intent, WriteState: "none", Persistence: "not-written", Achievement: "unknown", Reload: changeReload()}
	if p.intent.Application == "claude-code" {
		out.Reload = SessionExplanation{State: "unknown", Evidence: claudeEvidence(EvidenceUnknown, "session-reload", "", "", "a fresh Claude Code launch is a conservative way to load the saved choice; existing session adoption was not verified; whether restart is necessary is unknown")}
	}
	dest := p.destination
	parentFD, err := unix.Open(dest.Parent.Resolved, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		out.Reason = "open configuration parent failed"
		return
	}
	defer unix.Close(parentFD)
	if !changeFDMatches(parentFD, dest.Parent.Identity) {
		out.State = "refused"
		out.Reason = "configuration parent descriptor identity changed"
		return
	}
	if err = lockChangeDirectory(ctx, parentFD); err != nil {
		out.Reason = err.Error()
		return
	}
	defer unix.Flock(parentFD, unix.LOCK_UN)
	if err = p.recheck(ctx); err != nil {
		out.State = "refused"
		out.Reason = err.Error()
		return
	}
	if bytes.Equal(dest.Config.Bytes, p.postimage) {
		out.State = "applied"
		out.Reason = "choice already saved; no rewrite needed"
		return p.inspectResult(out, dest.Parent.Identity)
	}
	writeFD := parentFD
	created := changeFact{}
	tempName := ""
	tempIdentity := changeIdentity{}
	defer func() {
		if tempName != "" {
			path := filepath.Join(dest.Parent.Resolved, tempName)
			if dest.CreateDirectory != "" {
				path = filepath.Join(dest.CreateDirectory, tempName)
			}
			fact, e := observeChangePath(path)
			if e == nil && fact.Exists && fact.Identity == tempIdentity {
				e = p.checkpoint("cleanup")
				if e == nil {
					e = unix.Unlinkat(writeFD, tempName, 0)
				}
			}
			if e != nil || (fact.Exists && fact.Identity != tempIdentity) {
				out.State = "failed"
				out.Artifacts = append(out.Artifacts, path)
				out.Reason += "; temporary artifact cleanup failed"
			}
		}
		if created.Exists && out.WriteState == "none" {
			fact, e := observeChangePath(created.Path)
			if e == nil && fact.Identity == created.Identity {
				e = unix.Unlinkat(parentFD, filepath.Base(created.Path), unix.AT_REMOVEDIR)
			}
			if e != nil || fact.Identity != created.Identity {
				out.State = "failed"
				out.Artifacts = append(out.Artifacts, created.Path)
				out.Reason += "; created directory cleanup failed"
			}
		}
		if writeFD != parentFD {
			_ = unix.Close(writeFD)
		}
	}()
	if dest.CreateDirectory != "" {
		if err = unix.Mkdirat(parentFD, filepath.Base(dest.CreateDirectory), 0700); err != nil {
			out.Reason = "exclusive configuration directory creation failed"
			return
		}
		created, err = observeChangePath(dest.CreateDirectory)
		if err != nil {
			out.Reason = "new directory identity could not be established"
			out.Artifacts = append(out.Artifacts, dest.CreateDirectory)
			return
		}
		writeFD, err = unix.Openat(parentFD, filepath.Base(dest.CreateDirectory), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			out.Reason = "open newly created configuration directory failed"
			return
		}
	}
	random := make([]byte, 12)
	if _, err = rand.Read(random); err != nil {
		out.Reason = "temporary name generation failed"
		return
	}
	tempName = ".skillglass-" + hex.EncodeToString(random) + ".tmp"
	if dest.Config.Exists {
		err = unix.Clonefileat(writeFD, filepath.Base(dest.Path), writeFD, tempName, unix.CLONE_NOFOLLOW)
		if err != nil {
			out.Reason = "same-directory metadata-preserving clone failed"
			return
		}
	}
	openFlags := unix.O_WRONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if !dest.Config.Exists {
		openFlags |= unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(writeFD, tempName, openFlags, 0600)
	if err != nil {
		out.Reason = "same-directory temporary open failed"
		return
	}
	path := filepath.Join(dest.Parent.Resolved, tempName)
	if created.Exists {
		path = filepath.Join(created.Path, tempName)
	}
	file := os.NewFile(uintptr(fd), path)
	temp, err := observeChangePath(path)
	if err != nil {
		file.Close()
		out.Reason = "temporary identity unavailable"
		return
	}
	tempIdentity = temp.Identity
	if dest.Config.Exists {
		if temp.Identity.Mode != dest.Config.Identity.Mode || temp.Identity.UID != dest.Config.Identity.UID || temp.Identity.GID != dest.Config.Identity.GID || !sameChangeFacts(temp.Attributes, dest.Config.Attributes) {
			file.Close()
			out.Reason = "temporary clone did not preserve file metadata"
			return
		}
	}
	if err = file.Truncate(0); err != nil {
		file.Close()
		out.Reason = "temporary resize failed"
		return
	}
	if err = p.checkpoint("write"); err == nil {
		_, err = file.Write(p.postimage)
	}
	if err == nil {
		err = p.checkpoint("file-sync")
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		out.Reason = "temporary write or sync failed"
		return
	}
	stagedFact, err := observeChangePath(path)
	if err != nil || (dest.Config.Exists && !sameChangeFacts(stagedFact.Attributes, dest.Config.Attributes)) {
		out.Reason = "temporary metadata verification failed"
		return
	}
	staged, err := nativeBytes(path)
	if err != nil || !bytes.Equal(staged, p.postimage) {
		out.Reason = "temporary content verification failed"
		return
	}
	if err = p.checkpoint("before-recheck"); err == nil {
		if created.Exists {
			err = p.recheckCreated(ctx, created, tempName)
		} else {
			err = p.recheck(ctx)
		}
	}
	if err != nil {
		out.State = "refused"
		out.Reason = err.Error()
		return
	}
	if !changeFDMatches(writeFD, func() changeIdentity {
		if created.Exists {
			return created.Identity
		}
		return dest.Parent.Identity
	}()) {
		out.State = "refused"
		out.Reason = "configuration parent descriptor changed before commit"
		return
	}
	if err = verifyHeldConfig(writeFD, dest.Config); err != nil {
		out.State = "refused"
		out.Reason = "configuration entry changed before commit"
		return
	}
	if err = ctx.Err(); err != nil {
		out.State = "refused"
		out.Reason = "operation interrupted before commit"
		return
	}
	if err = p.checkpoint("rename"); err == nil {
		flags := uint32(0)
		if !dest.Config.Exists {
			flags = unix.RENAME_EXCL
		}
		err = unix.RenameatxNp(writeFD, tempName, writeFD, filepath.Base(dest.Path), flags)
	}
	if err != nil {
		out.Reason = "atomic configuration installation failed"
		return
	}
	tempName = ""
	out.WriteState = "committed"
	out.Persistence = "unverified"
	out.State = "applied"
	out.Reason = "choice saved"
	syncErr := p.checkpoint("directory-sync")
	if syncErr == nil {
		syncErr = unix.Fsync(writeFD)
	}
	if syncErr == nil && created.Exists {
		syncErr = unix.Fsync(parentFD)
	}
	if err = p.checkpoint("postread"); err != nil {
		out.State = "failed"
		out.Reason = "configuration committed; persisted verification failed"
		return
	}
	committedParent := dest.Parent.Identity
	if created.Exists {
		committedParent = created.Identity
	}
	out = p.inspectResult(out, committedParent)
	if syncErr != nil {
		out.State = "failed"
		out.Reason += "; directory durability could not be verified"
	}
	if out.Persistence != "verified" {
		out.State = "failed"
		out.Reason += "; persisted bytes no longer match the confirmed change"
	}
	return
}
func (p *changePlan) recheckCreated(ctx context.Context, created changeFact, tempName string) error {
	current, err := observeChangePath(created.Path)
	if err != nil || current.Identity != created.Identity {
		return errors.New("created configuration directory changed before commit")
	}
	entries, err := os.ReadDir(created.Path)
	if err != nil || len(entries) != 1 || entries[0].Name() != tempName {
		return errors.New("unexpected entry appeared in new configuration directory")
	}
	next := (&Snapshot{request: p.request}).PreviewChange(ctx, p.intent)
	if next.plan == nil {
		return errors.New("preview is stale after directory creation")
	}
	dependencies := next.plan.dependencies
	for i := range dependencies {
		chain := dependencies[i].Chain[:0]
		for _, item := range dependencies[i].Chain {
			if item.Path != created.Path {
				chain = append(chain, item)
			}
		}
		dependencies[i].Chain = chain
	}
	if !bytes.Equal(next.plan.snapshotFacts, p.snapshotFacts) || !sameChangeFacts(dependencies, p.dependencies) || !bytes.Equal(next.plan.postimage, p.postimage) {
		return errors.New("preview is stale; dependencies changed during directory creation")
	}
	return nil
}

func changeAttributes(path string) (map[string][]byte, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if size > 64*1024 {
		return nil, errors.New("extended attributes exceed bounded metadata limit")
	}
	if size == 0 {
		return nil, nil
	}
	names := make([]byte, size)
	n, err := unix.Listxattr(path, names)
	if err != nil {
		return nil, err
	}
	attributes := map[string][]byte{}
	total := 0
	for _, name := range strings.Split(string(names[:n]), "\x00") {
		if name == "" {
			continue
		}
		size, err := unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, err
		}
		total += size
		if total > 64*1024 {
			return nil, errors.New("extended attributes exceed bounded metadata limit")
		}
		value := make([]byte, size)
		n, err := unix.Getxattr(path, name, value)
		if err != nil {
			return nil, err
		}
		attributes[name] = value[:n]
	}
	return attributes, nil
}

func changeFDMatches(fd int, identity changeIdentity) bool {
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil {
		return false
	}
	return stat.Dev == identity.Device && stat.Ino == identity.Inode && stat.Btim.Sec == identity.BirthSec && stat.Btim.Nsec == identity.BirthNano && uint32(stat.Mode) == identity.Mode && stat.Uid == identity.UID && stat.Gid == identity.GID
}

func readChangeRegular(path string, expected changeIdentity) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || statIdentity(path, info.Sys().(*syscall.Stat_t)) != expected {
		return nil, errors.New("input identity changed during open")
	}
	if info.Size() > maxManifestBytes {
		return nil, errors.New("input exceeds bounded read limit")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if len(raw) > int(maxManifestBytes) {
		return nil, errors.New("input exceeds bounded read limit")
	}
	return raw, err
}
func verifyHeldConfig(parentFD int, expected changeFact) error {
	fd, err := unix.Openat(parentFD, filepath.Base(expected.Path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if !expected.Exists {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if err == nil {
			unix.Close(fd)
		}
		return errors.New("absent config appeared")
	}
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), expected.Path)
	defer file.Close()
	if !changeFDMatches(fd, expected.Identity) {
		return errors.New("config entry object changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxManifestBytes+1))
	if err != nil || !bytes.Equal(raw, expected.Bytes) {
		return errors.New("config entry bytes changed")
	}
	return nil
}

// Darwin getattrlist returns an empty extended-security reference when no ACL
// is attached. A twelve-byte buffer fits only that empty result; real ACLs or
// unsupported/unreadable metadata fail closed. This reads metadata directly so
// read-only inventory capability checks do not launch a subprocess.
func changeACLAbsent(path string) bool {
	pointer, err := syscall.BytePtrFromString(path)
	if err != nil {
		return false
	}
	attributes := struct {
		Count, Reserved                       uint16
		Common, Volume, Directory, File, Fork uint32
	}{Count: 5, Common: 0x00400000}
	var result [12]byte
	_, _, errno := syscall.Syscall6(syscall.SYS_GETATTRLIST, uintptr(unsafe.Pointer(pointer)), uintptr(unsafe.Pointer(&attributes)), uintptr(unsafe.Pointer(&result[0])), uintptr(len(result)), 0, 0)
	return errno == 0 && binary.LittleEndian.Uint32(result[:4]) == 12 && binary.LittleEndian.Uint32(result[8:]) == 0
}
