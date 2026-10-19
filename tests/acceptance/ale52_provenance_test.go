//go:build darwin

package acceptance_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestALE52MacOSReplacementMetadata(t *testing.T) {
	fx := piFixture(t)
	root := filepath.Join(fx.home, ".pi/agent/skills")
	target := codexSkill(t, root, "target", "target")
	sibling := codexSkill(t, root, "sibling", "sibling")
	config := filepath.Join(fx.home, ".pi/agent/settings.json")
	if err := os.Chmod(config, 0640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(config, "org.skillglass.fixture", []byte("preserve-exactly"), 0); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	original := before.Sys().(*syscall.Stat_t)
	provenance := make([]byte, 256)
	n, tagErr := unix.Getxattr(config, "com.apple.provenance", provenance)
	if tagErr == nil {
		t.Logf("naturally generated provenance present (%d bytes); never written by fixture", n)
	} else if tagErr != unix.ENOATTR {
		t.Fatal(tagErr)
	}
	originalProvenance := append([]byte(nil), provenance[:max(n, 0)]...)
	sourceBytes, siblingBytes := readBytes(t, target), readBytes(t, sibling)
	id := ale50PiSource(t, fx, target, 0).ID
	result := changeCommand(t, fx, "", "disable", "--agent", "pi", "--scope", "global", "--project", fx.project, "--json", id)
	assertExit(t, result, 0)
	if !strings.Contains(string(result.stdout), "com.apple.provenance") {
		t.Fatal("preview does not explain OS-managed replacement metadata", string(result.stdout))
	}
	human := changeCommand(t, fx, "", "disable", "--agent", "pi", "--scope", "global", "--project", fx.project, id)
	assertExit(t, human, 0)
	if !bytes.Contains(human.stdout, []byte("com.apple.provenance")) {
		t.Fatal("human preview omits replacement policy", string(human.stdout))
	}
	var preview changeReceipt
	decodeJSON(t, result.stdout, &preview)
	out := piChange(t, fx, "global", "disable", id, preview.Confirmation, 0)
	assertSavedChange(t, out)
	n, tagErr = unix.Getxattr(config, "com.apple.provenance", provenance)
	if tagErr != nil && tagErr != unix.ENOATTR {
		t.Fatal(tagErr)
	}
	if !bytes.Equal(originalProvenance, provenance[:max(n, 0)]) && !strings.Contains(out.Reason, "com.apple.provenance differed") {
		t.Fatal("changed tag absent from committed receipt", out)
	}
	after, err := os.Stat(config)
	if err != nil {
		t.Fatal(err)
	}
	saved := after.Sys().(*syscall.Stat_t)
	if saved.Mode != original.Mode || saved.Uid != original.Uid || saved.Gid != original.Gid || saved.Flags != original.Flags || saved.Nlink != original.Nlink {
		t.Fatal("replacement changed protected metadata")
	}
	value := make([]byte, 64)
	n, err = unix.Getxattr(config, "org.skillglass.fixture", value)
	if err != nil || string(value[:n]) != "preserve-exactly" {
		t.Fatal("ordinary xattr changed", err)
	}
	if !bytes.Equal(sourceBytes, readBytes(t, target)) || !bytes.Equal(siblingBytes, readBytes(t, sibling)) {
		t.Fatal("source content changed")
	}
	entries, err := os.ReadDir(filepath.Dir(config))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".skillglass-") {
			t.Fatal("temporary replacement leaked")
		}
	}
}

func TestALE52OriginalMetadataFreshnessRefuses(t *testing.T) {
	for _, mutate := range []string{"xattr-added", "xattr-changed", "mode", "hardlink", "symlink", "flags"} {
		t.Run(mutate, func(t *testing.T) {
			fx := piFixture(t)
			target := codexSkill(t, filepath.Join(fx.home, ".pi/agent/skills"), "target", "target")
			config := filepath.Join(fx.home, ".pi/agent/settings.json")
			if mutate == "xattr-changed" {
				if err := unix.Setxattr(config, "org.skillglass.fixture", []byte("before"), 0); err != nil {
					t.Fatal(err)
				}
			}
			id := ale50PiSource(t, fx, target, 0).ID
			preview := piChange(t, fx, "global", "disable", id, "", 0)
			originalBytes := readBytes(t, config)
			var err error
			switch mutate {
			case "xattr-added", "xattr-changed":
				err = unix.Setxattr(config, "org.skillglass.fixture", []byte("external-change"), 0)
			case "mode":
				err = os.Chmod(config, 0640)
			case "hardlink":
				err = os.Link(config, config+".independent-link")
			case "symlink":
				err = os.Rename(config, config+".external")
				if err == nil {
					err = os.Symlink(config+".external", config)
				}
			case "flags":
				err = unix.Chflags(config, unix.UF_NODUMP)
				t.Cleanup(func() { _ = unix.Chflags(config, 0) })
			}
			if err != nil {
				t.Fatal(err)
			}
			out := piChange(t, fx, "global", "disable", id, preview.Confirmation, 1)
			if out.State == "applied" || (out.WriteState != "none" && out.WriteState != "") {
				t.Fatal("stale metadata wrote", out)
			}
			if !bytes.Equal(originalBytes, readBytes(t, config)) {
				t.Fatal("external state overwritten")
			}
		})
	}
}
