package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// All ordinary product launches cross this seam, including tmux shell commands.
// Native absolute system inputs have no fixture override, so model their absence
// with ENOENT without consulting or modifying system policy.
func (fx fixture) productProfile(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Fatal("fixture containment requires macOS sandbox-exec")
	}
	path := filepath.Join(fx.temporary, "product-launch.sb")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Keep the profile stable across refreshes and fixture immutability checks.
	profile := fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(allow file-write* (literal "/dev/null") (literal "/dev/tty"))
(deny file-read* (with errno ENOENT) (subpath "/etc/codex") (subpath "/private/etc/codex"))
(deny file-read-data (require-not (require-any
 (literal "/") (literal %s) (literal %s)
 (subpath %s) (subpath "/System") (subpath "/usr/lib") (subpath "/dev"))))
(deny file-read-data (subpath %s))
(deny file-read* (with errno ENOENT) (subpath %s))
(deny process-exec (require-not (require-any (literal %s) (literal %s))))
`, sandboxString(canonicalPath(t, skillglassBinary)), sandboxString(canonicalPath(t, isolationProbeBinary)), sandboxString(canonicalPath(t, fx.base)),
		sandboxString(filepath.Join(canonicalPath(t, fx.base), "excluded", "live-home")),
		sandboxString(filepath.Join(canonicalPath(t, fx.base), "excluded", "system")),
		sandboxString(canonicalPath(t, skillglassBinary)), sandboxString(canonicalPath(t, isolationProbeBinary)))
	writeFile(t, path, []byte(profile), 0600)
	return path
}

func (fx fixture) productCommand(t *testing.T, ctx context.Context, args ...string) *exec.Cmd {
	t.Helper()
	return fx.containedCommand(t, ctx, skillglassBinary, args...)
}

func (fx fixture) containedCommand(t *testing.T, ctx context.Context, executable string, args ...string) *exec.Cmd {
	t.Helper()
	argv := append([]string{"-f", fx.productProfile(t), executable}, args...)
	return exec.CommandContext(ctx, "/usr/bin/sandbox-exec", argv...)
}

func (fx fixture) productShell(t *testing.T) string {
	t.Helper()
	return fx.containedShell(t, skillglassBinary)
}

func (fx fixture) containedShell(t *testing.T, executable string) string {
	t.Helper()
	return shellQuote("/usr/bin/sandbox-exec") + " -f " + shellQuote(fx.productProfile(t)) + " " + shellQuote(executable)
}
