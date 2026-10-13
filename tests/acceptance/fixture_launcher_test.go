package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// All ordinary CLI, direct PTY and tmux journeys execute this same launcher.
func fixtureLauncher(executable, directory string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("fixture sandbox requires macOS")
	}
	executable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	wrapper := filepath.Join(directory, "contained-"+filepath.Base(executable))
	script := "#!/bin/sh\nset -eu\n: \"${SKILLGLASS_FIXTURE_PROFILE:?fixture policy required}\"\nexec /usr/bin/sandbox-exec -f \"$SKILLGLASS_FIXTURE_PROFILE\" " + shellQuote(executable) + " \"$@\"\n"
	return wrapper, os.WriteFile(wrapper, []byte(script), 0700)
}

func fixtureProfile(base, executable string) string {
	var ancestors []string
	for parent := filepath.Dir(base); ; parent = filepath.Dir(parent) {
		ancestors = append(ancestors, "(literal "+sandboxString(parent)+")")
		if parent == string(filepath.Separator) {
			break
		}
	}
	return fmt.Sprintf(`(version 1)
(allow default)
(deny network*)
(deny file-write*)
(deny process-exec (require-not (literal %s)))
(deny file-read-data (require-not (require-any
 %s (subpath %s) (literal %s) (subpath "/System") (subpath "/usr/lib") (subpath "/bin") (subpath "/usr/bin") (subpath "/dev"))))
(deny file-read* (subpath "/Users"))
(deny file-read* (subpath "/private/etc/codex") (with errno 2))
(deny file-read* (subpath "/etc/codex") (with errno 2))
(deny file-read* (subpath "/Library/Application Support/opencode") (with errno 2))
(deny file-read* (subpath "/Library/Managed Preferences") (with errno 2))
(deny file-read* (subpath %s) (with errno 2))
`, sandboxString(executable), strings.Join(ancestors, " "), sandboxString(base), sandboxString(executable), sandboxString(filepath.Join(base, "excluded", "modeled-system")))
}
