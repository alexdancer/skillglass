package collection

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const compareFileLimit = 8 << 20
const compareByteLimit = 64 << 20
const compareEntryLimit = 10000
const compareDiffLimit = 1000

type ComparisonRelation string
type ComparisonOutcome string

const (
	RelationIndependent  ComparisonRelation = "independent-copies"
	RelationSameSource   ComparisonRelation = "same-source"
	RelationUnknown      ComparisonRelation = "unknown"
	OutcomeIdentical     ComparisonOutcome  = "identical"
	OutcomeDifferent     ComparisonOutcome  = "different"
	OutcomeIndeterminate ComparisonOutcome  = "indeterminate"
)

type ComparisonIssue struct {
	Side   string `json:"side"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type BundleChange struct {
	LeftLink      string   `json:"left_link,omitempty"`
	RightLink     string   `json:"right_link,omitempty"`
	LeftKind      string   `json:"left_kind,omitempty"`
	RightKind     string   `json:"right_kind,omitempty"`
	Path          string   `json:"path"`
	Status        string   `json:"status"`
	Kind          string   `json:"kind"`
	LeftMode      string   `json:"left_mode,omitempty"`
	RightMode     string   `json:"right_mode,omitempty"`
	LeftSize      int64    `json:"left_size"`
	RightSize     int64    `json:"right_size"`
	Diff          []string `json:"diff,omitempty"`
	DiffTruncated bool     `json:"diff_truncated,omitempty"`
}
type Comparison struct {
	Left      SourceID           `json:"left_id"`
	Right     SourceID           `json:"right_id"`
	LeftPath  string             `json:"left_path"`
	RightPath string             `json:"right_path"`
	Relation  ComparisonRelation `json:"relation"`
	Outcome   ComparisonOutcome  `json:"outcome"`
	Coverage  Completeness       `json:"coverage"`
	Changes   []BundleChange     `json:"changes"`
	Issues    []ComparisonIssue  `json:"issues"`
}
type bundleEntry struct {
	Target    unix.Stat_t
	Stamp     unix.Stat_t
	Link      string
	Hash      [32]byte
	Data      []byte
	Known     bool
	TypeKnown bool
}
type bundleObservation struct {
	proven      bool
	entries     map[string]bundleEntry
	directories map[string]bool
	issues      []ComparisonIssue
	bytes       int64
	side        string
	ctx         context.Context
	observer    func(string, string)
}

// Compare observes current bundle contents. It never executes content or follows supporting links.
// Stability checks are best effort, not an atomic filesystem snapshot.
func (snapshot *Snapshot) Compare(ctx context.Context, left, right SourceID) (Comparison, error) {
	return snapshot.compare(ctx, left, right, nil)
}
func (snapshot *Snapshot) compare(ctx context.Context, left, right SourceID, observer func(string, string)) (Comparison, error) {
	r := Comparison{Left: left, Right: right, Relation: "independent-copies", Outcome: "identical", Coverage: Complete, Changes: []BundleChange{}, Issues: []ComparisonIssue{}}
	a, ok := snapshot.Source(left)
	if !ok {
		return r, fmt.Errorf("left source %s is not in selected scope", left)
	}
	b, ok := snapshot.Source(right)
	if !ok {
		return r, fmt.Errorf("right source %s is not in selected scope", right)
	}
	r.LeftPath = a.Directory
	r.RightPath = b.Directory
	if left == right {
		r.Relation = "same-source"
	}
	first := make([]bundleObservation, 2)
	for i, s := range []Source{a, b} {
		side := []string{"left", "right"}[i]
		first[i] = observeBundle(ctx, s, side, observer)
		r.Issues = append(r.Issues, first[i].issues...)
		if !first[i].proven {
			r.Relation = RelationUnknown
		}
	}
	paths := map[string]bool{}
	for _, o := range first {
		for p := range o.entries {
			paths[p] = true
		}
	}
	ordered := make([]string, 0, len(paths))
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	for _, p := range ordered {
		x, xok := first[0].entries[p]
		y, yok := first[1].entries[p]
		if xok && yok && x.Known && y.Known && x.Stamp.Mode == y.Stamp.Mode && x.Hash == y.Hash && x.Link == y.Link && x.Target.Mode == y.Target.Mode {
			continue
		}
		c := BundleChange{LeftLink: x.Link, RightLink: y.Link, LeftKind: entryKind(x.Stamp.Mode), RightKind: entryKind(y.Stamp.Mode), Path: p, Status: "changed", Kind: "metadata", LeftMode: fmt.Sprintf("%#o", x.Stamp.Mode), RightMode: fmt.Sprintf("%#o", y.Stamp.Mode), LeftSize: x.Stamp.Size, RightSize: y.Stamp.Size}
		if !xok {
			c.LeftKind = ""
			c.Status = "added"
		}
		if !yok {
			c.RightKind = ""
			c.Status = "removed"
		}
		if (!xok && !first[0].absenceKnown(p)) || (!yok && !first[1].absenceKnown(p)) {
			c.Status = "unknown"
			c.Kind = "unknown"
		} else if (!xok || x.Known) && (!yok || y.Known) && (x.Stamp.Mode&unix.S_IFMT == unix.S_IFREG || y.Stamp.Mode&unix.S_IFMT == unix.S_IFREG || (p == "SKILL.md" && (x.Target.Mode&unix.S_IFMT == unix.S_IFREG || y.Target.Mode&unix.S_IFMT == unix.S_IFREG))) {
			if safeText(x.Data) && safeText(y.Data) {
				c.Kind = "text"
				c.Diff, c.DiffTruncated = boundedDiff(p, x.Data, y.Data)
			} else {
				c.Kind = "binary"
			}
		} else if (xok && !x.Known) || (yok && !y.Known) {
			c.Kind = "unknown"
		}
		r.Changes = append(r.Changes, c)
	}
	// Re-read both inventories after all initial reads, detecting membership and byte changes.
	for i, s := range []Source{a, b} {
		last := observeBundle(ctx, s, []string{"left", "right"}[i], nil)
		r.Issues = append(r.Issues, last.issues...)
		if !last.proven {
			r.Relation = RelationUnknown
		}
		if !sameObservation(first[i], last) {
			r.Issues = append(r.Issues, ComparisonIssue{[]string{"left", "right"}[i], ".", "source changed during comparison"})
		}
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if len(r.Changes) > 0 {
		r.Outcome = "different"
	}

	seen := map[ComparisonIssue]bool{}
	issues := r.Issues[:0]
	for _, issue := range r.Issues {
		if !seen[issue] {
			seen[issue] = true
			issues = append(issues, issue)
		}
	}
	r.Issues = issues
	if len(r.Issues) > 0 {
		r.Coverage = Incomplete
		r.Outcome = "indeterminate"
	}
	return r, nil
}
func sameObservation(a, b bundleObservation) bool {
	if len(a.entries) != len(b.entries) || !reflect.DeepEqual(a.directories, b.directories) {
		return false
	}
	for p, x := range a.entries {
		y, ok := b.entries[p]
		if !ok || !sameStamp(x.Stamp, y.Stamp) || x.Hash != y.Hash || x.Link != y.Link || !sameStamp(x.Target, y.Target) || x.Known != y.Known || x.TypeKnown != y.TypeKnown {
			return false
		}
	}
	return true
}
func (o bundleObservation) absenceKnown(p string) bool {
	for p != "." {
		parent := filepath.Dir(p)
		if o.directories[parent] {
			return true
		}
		if entry, exists := o.entries[parent]; exists {
			// A stable non-directory entry cannot contain descendants. Byte
			// readability is independent of this observed type evidence, while
			// untraversed links and directories leave membership unknown.
			kind := entry.Stamp.Mode & unix.S_IFMT
			return entry.TypeKnown && kind != unix.S_IFDIR && kind != unix.S_IFLNK
		}
		p = parent
	}
	return false
}
func (o *bundleObservation) issue(p string, err error) {
	o.issues = append(o.issues, ComparisonIssue{o.side, p, err.Error()})
}
func observeBundle(ctx context.Context, s Source, side string, observer func(string, string)) bundleObservation {
	o := bundleObservation{entries: map[string]bundleEntry{}, directories: map[string]bool{}, side: side, ctx: ctx, observer: observer}
	token, err := directoryObjectToken(s.Directory)
	if err != nil || s.Identity.Certainty != "proven" || sourceID(token) != s.ID {
		reason := "selected directory identity is no longer proven"
		if err != nil {
			reason += ": " + err.Error()
		}
		o.issue(".", fmt.Errorf("%s", reason))
		return o
	}
	f, err := openAnchored(s.Directory, true)
	if err != nil {
		o.issue(".", err)
		return o
	}
	defer f.Close()
	var st unix.Stat_t
	if err = unix.Fstat(int(f.Fd()), &st); err != nil {
		o.issue(".", err)
		return o
	}
	if sourceID(objectToken{uint64(st.Dev), st.Ino, st.Btim.Sec, st.Btim.Nsec}) != s.ID {
		o.issue(".", fmt.Errorf("directory replaced while opening"))
		return o
	}
	o.proven = true
	o.entries["."] = bundleEntry{Stamp: st, Known: true, TypeKnown: true}
	o.walk(f, "", s.Directory)
	token, err = directoryObjectToken(s.Directory)
	if err != nil || sourceID(token) != s.ID {
		o.proven = false
		o.issue(".", fmt.Errorf("selected directory changed during comparison"))
	}
	manifest, ok := o.entries["SKILL.md"]
	if !ok {
		if o.absenceKnown("SKILL.md") {
			o.issue("SKILL.md", fmt.Errorf("manifest missing"))
		} else {
			o.issue("SKILL.md", fmt.Errorf("manifest could not be observed"))
		}
	} else if !manifest.Known || (manifest.Stamp.Mode&unix.S_IFMT != unix.S_IFREG && (manifest.Stamp.Mode&unix.S_IFMT != unix.S_IFLNK || manifest.Target.Mode&unix.S_IFMT != unix.S_IFREG)) {
		manifest.Known = false
		o.entries["SKILL.md"] = manifest
		o.issue("SKILL.md", fmt.Errorf("manifest is not a readable regular file"))
	}
	return o
}

// Anchor every path segment, including the source's ancestors, without following links.
func openAnchored(path string, directory bool) (*os.File, error) {
	f, err := os.Open("/")
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	for i, p := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		if i < len(parts)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		fd, e := unix.Openat(int(f.Fd()), p, flags, 0)
		f.Close()
		if e != nil {
			return nil, e
		}
		f = os.NewFile(uintptr(fd), p)
	}
	return f, nil
}
func (o *bundleObservation) walk(dir *os.File, prefix, root string) {
	if o.ctx.Err() != nil {
		return
	}
	var before unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &before); err != nil {
		o.issue(prefix, err)
		return
	}
	remaining := max(1, compareEntryLimit-len(o.entries))
	names, err := dir.Readdirnames(remaining + 1)
	if err != nil && err != io.EOF {
		o.issue(prefix, err)
		return
	}
	complete := len(names) <= remaining
	if !complete {
		o.issue(prefix, fmt.Errorf("entry limit %d reached; directory inventory truncated", compareEntryLimit))
		names = names[:remaining]
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(prefix, name)
		if len(o.entries) >= compareEntryLimit {
			o.issue(p, fmt.Errorf("entry limit %d reached", compareEntryLimit))
			complete = false
			break
		}
		var st unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			o.issue(p, err)
			complete = false
			continue
		}
		e := bundleEntry{Stamp: st, Known: true, TypeKnown: true}
		o.entries[p] = e
		if o.observer != nil {
			o.observer(o.side, p)
		}
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFLNK:
			buf := make([]byte, 4096)
			n, err := unix.Readlinkat(int(dir.Fd()), name, buf)
			if err != nil {
				o.issue(p, err)
				e.Known = false
			} else {
				e.Link = string(buf[:n])
				target := filepath.Join(root, prefix, e.Link)
				if filepath.IsAbs(e.Link) {
					target = e.Link
				}
				resolved, err := filepath.EvalSymlinks(target)
				if err != nil {
					o.issue(p, fmt.Errorf("missing or cyclic link target: %w", err))
				} else {
					rel, _ := filepath.Rel(root, resolved)
					external := rel == ".." || strings.HasPrefix(rel, "../")
					if external {
						o.issue(p, fmt.Errorf("external link target excluded: %s", resolved))
					}
					if p == "SKILL.md" {
						f, err := openAnchored(resolved, false)
						if err != nil {
							o.issue(p, err)
							e.Known = false
						} else {
							var targetStamp unix.Stat_t
							_ = unix.Fstat(int(f.Fd()), &targetStamp)
							e.Target = targetStamp
							if targetStamp.Mode&unix.S_IFMT != unix.S_IFREG {
								o.issue(p, fmt.Errorf("linked manifest is not regular"))
								e.Known = false
							} else {
								e.Data, e.Known = o.read(f, p, targetStamp)
								e.Hash = sha256.Sum256(e.Data)
							}
							f.Close()
						}
					}
				}
			}
		case unix.S_IFREG, unix.S_IFDIR:
			flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
			if st.Mode&unix.S_IFMT == unix.S_IFDIR {
				flags |= unix.O_DIRECTORY
			}
			fd, err := unix.Openat(int(dir.Fd()), name, flags, 0)
			if err != nil {
				o.issue(p, err)
				e.Known = false
			} else {
				f := os.NewFile(uintptr(fd), p)
				var actual unix.Stat_t
				_ = unix.Fstat(fd, &actual)
				if !sameStamp(st, actual) {
					o.issue(p, fmt.Errorf("entry changed before open"))
					e.Known = false
					e.TypeKnown = false
				} else if st.Mode&unix.S_IFMT == unix.S_IFDIR {
					o.walk(f, p, root)
				} else {
					e.Data, e.Known = o.read(f, p, st)
					e.Hash = sha256.Sum256(e.Data)
				}
				if err := unix.Fstat(fd, &actual); err != nil || !sameStamp(st, actual) {
					o.issue(p, fmt.Errorf("entry descriptor changed during read"))
					e.Known = false
					e.TypeKnown = false
				}
				f.Close()
			}
		default:
			o.issue(p, fmt.Errorf("special file excluded"))
			e.Known = false
		}
		var after unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil || !sameStamp(st, after) {
			o.issue(p, fmt.Errorf("entry changed during read"))
			e.Known = false
			e.TypeKnown = false
		}
		o.entries[p] = e
	}
	var after unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &after); err != nil || !sameStamp(before, after) {
		o.issue(prefix, fmt.Errorf("directory changed during traversal"))
		complete = false
	}
	if complete {
		o.directories[filepath.Clean(prefix)] = true
	}
}
func (o *bundleObservation) read(f *os.File, p string, st unix.Stat_t) ([]byte, bool) {
	if st.Mode&0444 == 0 {
		o.issue(p, fmt.Errorf("file has no read permissions"))
		return nil, false
	}
	if st.Size > compareFileLimit || o.bytes+st.Size > compareByteLimit {
		o.issue(p, fmt.Errorf("read limit: 8 MiB per file, 64 MiB per side"))
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(f, compareFileLimit+1))
	if o.observer != nil {
		o.observer(o.side, p+":read")
	}
	o.bytes += int64(len(data))
	var after unix.Stat_t
	_ = unix.Fstat(int(f.Fd()), &after)
	if err != nil || len(data) > compareFileLimit || !sameStamp(st, after) {
		o.issue(p, fmt.Errorf("read failed or file changed: %v", err))
		return nil, false
	}
	return data, true
}
func safeText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func boundedDiff(p string, a, b []byte) ([]string, bool) {
	if bytes.Equal(a, b) {
		return nil, false
	}
	x, y := strings.SplitAfter(string(a), "\n"), strings.SplitAfter(string(b), "\n")
	if len(x) > 0 && x[len(x)-1] == "" {
		x = x[:len(x)-1]
	}
	if len(y) > 0 && y[len(y)-1] == "" {
		y = y[:len(y)-1]
	}
	prefix := 0
	for prefix < len(x) && prefix < len(y) && x[prefix] == y[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(x)-prefix && suffix < len(y)-prefix && x[len(x)-1-suffix] == y[len(y)-1-suffix] {
		suffix++
	}
	start := max(0, prefix-3)
	xe, ye := min(len(x), len(x)-suffix+3), min(len(y), len(y)-suffix+3)
	lines := []string{"--- left/" + p, "+++ right/" + p, fmt.Sprintf("@@ -%d,%d +%d,%d @@", start+1, xe-start, start+1, ye-start)}
	truncated := false
	add := func(marker, line string) {
		if len(lines) >= compareDiffLimit {
			truncated = true
			return
		}
		hasNewline := strings.HasSuffix(line, "\n")
		if len(line) > 4096 {
			limit := 4096
			for limit > 0 && !utf8.ValidString(line[:limit]) {
				limit--
			}
			line = line[:limit] + " [line truncated]"
			truncated = true
		}
		lines = append(lines, marker+strings.TrimSuffix(line, "\n"))
		if !hasNewline {
			lines = append(lines, "\\ No newline at end of file")
		}
	}
	for _, line := range x[start:prefix] {
		add(" ", line)
	}
	for _, line := range x[prefix : len(x)-suffix] {
		add("-", line)
	}
	for _, line := range y[prefix : len(y)-suffix] {
		add("+", line)
	}
	for _, line := range x[len(x)-suffix : xe] {
		add(" ", line)
	}
	return lines, truncated
}

func sameStamp(a, b unix.Stat_t) bool {
	a.Atim = unix.Timespec{}
	b.Atim = unix.Timespec{}
	return reflect.DeepEqual(a, b)
}

func entryKind(mode uint16) string {
	switch mode & unix.S_IFMT {
	case unix.S_IFREG:
		return "file"
	case unix.S_IFDIR:
		return "directory"
	case unix.S_IFLNK:
		return "symlink"
	default:
		return "special"
	}
}
