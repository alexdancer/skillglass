package collection

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ResourceReadLimit bounds each explicit read. Oversized files return no content.
const ResourceReadLimit int64 = 1 << 20

type ResourceRead struct {
	SourceID       SourceID             `json:"source_id"`
	Path           string               `json:"path"`
	State          string               `json:"state"`
	Coverage       Completeness         `json:"coverage"`
	Representation string               `json:"representation"`
	Text           *string              `json:"text,omitempty"`
	BytesBase64    string               `json:"bytes_base64,omitempty"`
	Size           int64                `json:"size"`
	BytesRead      int64                `json:"bytes_read"`
	LimitBytes     int64                `json:"limit_bytes"`
	Resource       *Resource            `json:"resource,omitempty"`
	Diagnostics    []ResourceDiagnostic `json:"diagnostics"`
}

type ResourceDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Action  string `json:"action"`
}

// ReadResource reads only the requested regular file inside a proven bundle.
// Every path component is opened relative to a held descriptor. Links are
// resolved explicitly and only within the selected bundle. Stability checks detect
// ordinary concurrent changes, not an atomic snapshot.
func (snapshot *Snapshot) ReadResource(ctx context.Context, id SourceID, path string) ResourceRead {
	result := ResourceRead{SourceID: id, Path: path, State: "failed", Coverage: Incomplete, Representation: "none", LimitBytes: ResourceReadLimit, Diagnostics: []ResourceDiagnostic{}}
	fail := func(code, message, action string) ResourceRead {
		result.Text = nil
		result.BytesBase64 = ""
		result.Representation = "none"
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{code, message, action})
		return result
	}
	source, ok := snapshot.Source(id)
	if !ok {
		return fail("source-not-found", "Selected source is absent or its identity changed.", "Run list with the same scope and select the current exact source ID.")
	}
	if source.Identity.Certainty != "proven" || source.Identity.Method != "darwin-directory-object" {
		return fail("unsupported-source", "Supporting reads require a proven bundle directory; standalone manifests have no implicit sibling bundle.", "Use read for instructions or select a bundle directory from list.")
	}
	if !filepath.IsLocal(path) || filepath.ToSlash(filepath.Clean(path)) != path || path == "." || path == "SKILL.md" {
		return fail("invalid-resource-path", "Select a normalized bundle-relative resource path; SKILL.md is read with read.", "Copy an exact resources[].path from read or list.")
	}
	for _, resource := range source.Resources {
		if resource.Path == path {
			copy := resource
			result.Resource = &copy
			break
		}
	}
	root, err := openAnchored(source.Directory, true)
	if err != nil {
		return fail("source-unreadable", err.Error(), "Check bundle access and rediscover its source ID.")
	}
	defer root.Close()
	var rootStamp unix.Stat_t
	if unix.Fstat(int(root.Fd()), &rootStamp) != nil || sourceID(objectToken{uint64(rootStamp.Dev), rootStamp.Ino, rootStamp.Btim.Sec, rootStamp.Btim.Nsec}) != id {
		return fail("source-changed", "Selected source identity changed before opening.", "Rediscover the source and review it again.")
	}
	type heldEntry struct {
		parent *os.File
		name   string
		stamp  unix.Stat_t
	}
	held := []heldEntry{}
	directories := []*os.File{}
	defer func() {
		for _, directory := range directories {
			directory.Close()
		}
	}()
	parent := root
	parts := strings.Split(path, "/")
	var file *os.File
	var stamp unix.Stat_t
	prefix := []string{}
	links := 0
	for len(parts) > 0 {
		name := parts[0]
		parts = parts[1:]
		if err := ctx.Err(); err != nil {
			return fail("read-interrupted", err.Error(), "Retry the explicit read.")
		}
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			if len(prefix) == 0 {
				return fail("external-resource-link", "Resource link resolution leaves the selected bundle; external target was not opened.", "Disclose external evidence as uninspected.")
			}
			parts = append(append([]string{}, prefix[:len(prefix)-1]...), parts...)
			prefix = nil
			parent = root
			continue
		}
		if err := unix.Fstatat(int(parent.Fd()), name, &stamp, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			code := "resource-unreadable"
			if links > 0 {
				code = "broken-resource-link"
			}
			return fail(code, err.Error(), "Check the path and permissions; rediscover resources if they changed.")
		}
		if stamp.Mode&unix.S_IFMT == unix.S_IFLNK {
			links++
			if links > 40 {
				return fail("resource-link-cycle", "Resource link resolution exceeded 40 links.", "Inspect the cyclic or excessive link chain separately.")
			}
			targetBytes := make([]byte, 4096)
			n, err := unix.Readlinkat(int(parent.Fd()), name, targetBytes)
			if err != nil || n == len(targetBytes) {
				return fail("resource-link-unreadable", "Resource link target could not be read completely.", "Inspect the link metadata separately.")
			}
			target := string(targetBytes[:n])
			held = append(held, heldEntry{parent, name, stamp})
			if filepath.IsAbs(target) {
				// Keep components intact: cleaning b/../file before resolving b could
				// cancel a symlink and return evidence from the wrong file.
				if target == source.Directory {
					target = "."
				} else if strings.HasPrefix(target, source.Directory+"/") {
					target = strings.TrimPrefix(target, source.Directory+"/")
				} else {
					return fail("external-resource-link", fmt.Sprintf("Resource link target %q leaves the selected bundle; it was not opened.", target), "Disclose external evidence as uninspected; no external fallback is performed.")
				}
			} else if len(prefix) > 0 {
				target = strings.Join(prefix, "/") + "/" + target
			}
			parts = append(strings.Split(target, "/"), parts...)
			parent = root
			prefix = nil
			continue
		}
		directory := len(parts) > 0
		if (directory && stamp.Mode&unix.S_IFMT != unix.S_IFDIR) || (!directory && stamp.Mode&unix.S_IFMT != unix.S_IFREG) {
			return fail("unsupported-resource", "Only regular files can be read; directories and special files are excluded.", "Select an exact regular-file path from resources.")
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		if directory {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
		if err != nil {
			return fail("resource-unreadable", err.Error(), "Check resource access; no broader fallback is performed.")
		}
		next := os.NewFile(uintptr(fd), name)
		var actual unix.Stat_t
		if unix.Fstat(fd, &actual) != nil || !sameStamp(stamp, actual) {
			next.Close()
			return fail("resource-changed", "Resource changed while opening.", "Rediscover and repeat the read.")
		}
		held = append(held, heldEntry{parent, name, stamp})
		if directory {
			directories = append(directories, next)
			parent = next
			prefix = append(prefix, name)
		} else {
			file = next
		}
	}
	if file == nil {
		return fail("unsupported-resource", "Resource resolves to a directory rather than a regular file.", "Select a regular-file path.")
	}
	defer file.Close()
	result.Size = stamp.Size
	if stamp.Mode&0444 == 0 {
		return fail("resource-unreadable", "File has no read permissions.", "Check resource permissions.")
	}
	if stamp.Size > ResourceReadLimit {
		return fail("resource-too-large", fmt.Sprintf("File exceeds %d bytes; no partial content returned.", ResourceReadLimit), "Inspect it separately and disclose missing CLI evidence.")
	}
	raw, err := io.ReadAll(io.LimitReader(file, ResourceReadLimit+1))
	result.BytesRead = int64(len(raw))
	if err != nil {
		return fail("resource-unreadable", err.Error(), "Retry after checking resource access; incomplete bytes are withheld.")
	}
	if int64(len(raw)) > ResourceReadLimit {
		return fail("resource-too-large", "Resource grew past the read limit; content withheld.", "Rediscover and inspect it separately.")
	}
	var after unix.Stat_t
	if unix.Fstat(int(file.Fd()), &after) != nil || !sameStamp(stamp, after) || int64(len(raw)) != stamp.Size {
		return fail("resource-changed", "Resource changed during reading; content withheld.", "Rediscover and repeat the read.")
	}
	for _, entry := range held {
		if unix.Fstatat(int(entry.parent.Fd()), entry.name, &after, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameStamp(entry.stamp, after) {
			return fail("resource-changed", "Resource path changed during reading; content withheld.", "Rediscover and repeat the read.")
		}
	}
	currentRoot, err := openAnchored(source.Directory, true)
	if err != nil {
		return fail("source-changed", "Selected source path changed during reading; content withheld.", "Rediscover the source and review it again.")
	}
	defer currentRoot.Close()
	var currentStamp unix.Stat_t
	if unix.Fstat(int(currentRoot.Fd()), &currentStamp) != nil || sourceID(objectToken{uint64(currentStamp.Dev), currentStamp.Ino, currentStamp.Btim.Sec, currentStamp.Btim.Nsec}) != id {
		return fail("source-changed", "Selected bundle identity changed during reading; content withheld.", "Rediscover the source and review it again.")
	}
	if err := ctx.Err(); err != nil {
		return fail("read-interrupted", err.Error(), "Retry the explicit read.")
	}
	result.State = "read"
	result.Coverage = Complete
	if safeText(raw) {
		text := string(raw)
		result.Text = &text
		result.Representation = "utf-8"
	} else {
		result.BytesBase64 = base64.StdEncoding.EncodeToString(raw)
		result.Representation = "base64"
	}
	return result
}
