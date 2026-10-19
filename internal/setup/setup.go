package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/alexdancer/skillglass/internal/companion"
)

type ExecutableCheck struct {
	State  string `json:"state"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}
type SetupResult struct {
	Application      string                    `json:"application"`
	State            string                    `json:"state"`
	Destination      string                    `json:"destination,omitempty"`
	ValidatedVersion string                    `json:"validated_native_version,omitempty"`
	Consumers        []string                  `json:"affected_applications"`
	Reason           string                    `json:"reason"`
	Action           string                    `json:"action"`
	Audit            *collection.SetupLocation `json:"isolation,omitempty"`
}
type SetupReport struct {
	SchemaVersion     int             `json:"schema_version"`
	Operation         string          `json:"operation"`
	State             string          `json:"state"`
	ExitCode          int             `json:"exit_code"`
	Name              string          `json:"name"`
	Version           string          `json:"version"`
	ContractVersion   int             `json:"review_contract_version"`
	Applications      []string        `json:"applications"`
	AllowedSharedApps []string        `json:"allowed_shared_applications"`
	Install           bool            `json:"install"`
	Executable        ExecutableCheck `json:"executable"`
	Results           []SetupResult   `json:"results"`
	Notes             []string        `json:"notes"`
}

// CheckExecutable uses exactly the command the portable skill invokes. A moved
// build cannot be rescued by an installation-specific absolute-path sidecar.
func CheckExecutable(ctx context.Context) ExecutableCheck {
	out := ExecutableCheck{State: "missing", Reason: "skillglass is not executable on PATH", Action: "Put a compatible Skillglass executable named skillglass on the agent application's PATH, then run skillglass setup --check --json in that environment. If a binary moved, repair PATH; setup does not pin its old pathname."}
	path, err := exec.LookPath("skillglass")
	if err != nil {
		return out
	}
	out.Path, err = filepath.Abs(path)
	if err != nil {
		out.State = "failed"
		out.Reason = err.Error()
		return out
	}
	// The currently executing artifact already owns this exact embedded contract.
	self, e := os.Executable()
	a, ae := os.Stat(self)
	b, be := os.Stat(path)
	if e == nil && ae == nil && be == nil && os.SameFile(a, b) {
		out.State = "compatible"
		out.Reason = "PATH resolves to this executable's exact bundled companion and review contract"
		out.Action = "Use the same PATH in a fresh agent session; running-session discovery is separate."
		return out
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, path, "companion", "--json")
	command.WaitDelay = 250 * time.Millisecond
	var stdout, stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		out.State = "failed"
		out.Reason = "PATH executable could not report its companion contract: " + err.Error() + ": " + strings.TrimSpace(stderr.String())
		return out
	}
	var actual companion.Bundle
	if json.Unmarshal(stdout.Bytes(), &actual) != nil || !reflect.DeepEqual(actual, companion.Bundled()) {
		out.State = "incompatible"
		out.Reason = "PATH executable does not provide the exact bundled companion version and review contract required by this skill"
		return out
	}
	command = exec.CommandContext(bounded, path, "help", "--json")
	command.WaitDelay = 250 * time.Millisecond
	stdout.Reset()
	stderr.Reset()
	command.Stdout = &stdout
	command.Stderr = &stderr
	var capabilities struct {
		SchemaVersion int    `json:"schema_version"`
		Help          string `json:"help"`
	}
	if err := command.Run(); err != nil || json.Unmarshal(stdout.Bytes(), &capabilities) != nil || capabilities.SchemaVersion != 1 || !strings.Contains(capabilities.Help, "review prepare") || !strings.Contains(capabilities.Help, "review select") || !strings.Contains(capabilities.Help, "review apply") || !strings.Contains(capabilities.Help, "read-resource") || !strings.Contains(capabilities.Help, "setup --check") {
		out.State = "incompatible"
		out.Reason = "PATH executable does not advertise the documented setup, resource-reading and review commands"
		return out
	}
	out.State = "compatible"
	out.Reason = "PATH executable reports the exact companion files, version, review contract and documented command capabilities"
	out.Action = "Keep this executable discoverable in the agent application's PATH."
	return out
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, errors.New("executable response exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}

func Setup(ctx context.Context, apps, sharedApps []string, install bool) (SetupReport, error) {
	out := SetupReport{SchemaVersion: 1, Operation: "setup", State: "preview", Name: companion.Name, Version: companion.Version, ContractVersion: companion.ContractVersion, Applications: append([]string{}, apps...), AllowedSharedApps: append([]string{}, sharedApps...), Install: install, Results: []SetupResult{}, Notes: []string{"Only the exact bundled companion is installed. No settings, managed caches or other skills are updated.", "Shared roots require every affected application to be selected with --app or separately acknowledged with --allow-shared-app; uncertain isolation refuses. Shared consent installs no separate target and does not certify that consumer's native discovery.", "Installed files establish persistence only. Use a fresh native session or its documented reload; no skill workload or session restart is performed.", "Failures do not roll back earlier installations. Preserve customized/conflicting copies; compare manually, relocate them yourself if desired, then retry an explicit preview and installation."}}
	selected := map[string]bool{}
	if len(apps) == 0 {
		return out, errors.New("setup requires at least one explicit --app; use --check alone to diagnose executable discovery")
	}
	known := map[string]bool{}
	for _, app := range collection.ApplicationCapabilities() {
		known[app.Application] = true
	}
	for _, app := range apps {
		if !known[app] {
			return out, fmt.Errorf("unknown application %q", app)
		}
		if selected[app] {
			return out, fmt.Errorf("duplicate application %q", app)
		}
		selected[app] = true
	}
	consent := map[string]bool{}
	for _, app := range sharedApps {
		if !known[app] {
			return out, fmt.Errorf("unknown shared application %q", app)
		}
		if selected[app] || consent[app] {
			return out, fmt.Errorf("application %q is repeated or already selected as an installation target", app)
		}
		consent[app] = true
	}
	out.Executable = CheckExecutable(ctx)
	home := os.Getenv("HOME")
	if !filepath.IsAbs(home) {
		return out, errors.New("setup requires an absolute HOME")
	}
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		return out, err
	}
	for _, app := range apps {
		r := SetupResult{Application: app, State: "unsupported", Consumers: []string{}, Action: "Use a supported, validated user-level destination; no generic or project-local fallback is available."}
		root, version := setupRoot(app, home)
		r.ValidatedVersion = version
		if root == "" || runtime.GOOS != "darwin" {
			r.Reason = "companion setup discovery is unverified for this application/platform; native availability controls do not establish setup support"
			out.Results = append(out.Results, r)
			continue
		}
		r.Destination = filepath.Join(root, companion.Name)
		if reason := unverifiedSetupOverride(app, home); reason != "" {
			r.Reason = reason
			r.Action = "Preview and use a validated default native user destination in the matching application environment, or wait for explicit override validation. Setup performs no fallback or settings edits."
			out.Results = append(out.Results, r)
			continue
		}
		r.State = "refused"
		if err := safeSetupPath(home, r.Destination); err != nil {
			r.Reason = err.Error()
			out.Results = append(out.Results, r)
			continue
		}
		state, existingErr := existingSetup(r.Destination)
		if existingErr != nil {
			r.State = "failed"
			r.Reason = existingErr.Error()
			r.Action = "Inspect destination permissions and preserve existing content before retrying."
			out.Results = append(out.Results, r)
			continue
		}
		if state == "conflict" {
			r.Reason = "existing companion differs from the exact bundle, has extra content, or is not a plain directory; preserved without overwrite"
			r.Action = "Compare with skillglass companion --json. Preserve or move your existing copy yourself, then retry setup."
			out.Results = append(out.Results, r)
			continue
		}
		audit := collection.AuditSetupLocation(ctx, r.Destination)
		r.Audit = &audit
		r.Consumers = audit.Consumers
		if len(audit.Refusals) > 0 {
			r.Reason = strings.Join(audit.Refusals, "; ")
			out.Results = append(out.Results, r)
			continue
		}
		unselected := []string{}
		for _, consumer := range r.Consumers {
			if !selected[consumer] && !consent[consumer] {
				unselected = append(unselected, consumer)
			}
		}
		if len(unselected) > 0 {
			r.Reason = "destination has unacknowledged shared consumers: " + strings.Join(unselected, ", ")
			r.Action = "After reviewing shared effects and discovery limits, explicitly acknowledge indirect consumers with --allow-shared-app APP, or select a supported installation target with --app APP. Repeat the same consent with --install; no implicit fallback is used."
			out.Results = append(out.Results, r)
			continue
		}
		if !contains(r.Consumers, app) {
			r.Reason = "validated native discovery does not include the selected destination in this environment"
			out.Results = append(out.Results, r)
			continue
		}
		if out.Executable.State != "compatible" {
			r.Reason = "compatible executable discovery is required: " + out.Executable.Reason
			r.Action = out.Executable.Action
		} else if state == "identical" {
			r.State = "already-present"
			r.Reason = "exact bundled files already present; no writes needed"
			r.Action = "Use a fresh native session or its documented reload with the compatible executable on PATH."
		} else {
			r.State = "ready"
			r.Reason = "validated user-level destination; use the same explicit applications with --install to write"
			r.Action = "Review destinations and shared effects, then repeat these explicit applications and shared-consumer consent with --install."
		}
		out.Results = append(out.Results, r)
	}
	if install {
		for i := range out.Results {
			r := &out.Results[i]
			if r.State != "ready" {
				continue
			}
			// Re-evaluate isolation immediately before every independent installation.
			audit := collection.AuditSetupLocation(ctx, r.Destination)
			if !reflect.DeepEqual(audit, *r.Audit) {
				r.State = "refused"
				r.Reason = "native isolation evidence changed; obtain a fresh setup preview"
				continue
			}
			if err := installSetup(home, r.Destination); err != nil {
				r.State = "failed"
				r.Reason = "installation failed: " + err.Error()
				r.Action = "Correct destination permissions or filesystem failure, preserve existing files, then preview and retry only the desired applications."
				continue
			}
			state, err := existingSetup(r.Destination)
			if err != nil || state != "identical" {
				r.State = "failed"
				r.Reason = "installed bundle verification failed; inspect destination before retrying"
				continue
			}
			r.State = "installed"
			r.Reason = "exact bundled companion installed and byte-verified"
			r.Action = "Invoke skillglass-review in a fresh native session with the verified executable on PATH."
		}
	}
	successful, failed := 0, 0
	for _, r := range out.Results {
		if r.State == "ready" || r.State == "installed" || r.State == "already-present" {
			successful++
		} else {
			failed++
		}
	}
	if failed > 0 {
		out.State = "refused"
		out.ExitCode = 1
		if successful > 0 {
			out.State = "partial"
		}
	} else if install {
		out.State = "complete"
	}
	return out, nil
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func setupRoot(app, home string) (string, string) {
	switch app {
	case "codex":
		base := os.Getenv("CODEX_HOME")
		if base == "" {
			base = filepath.Join(home, ".codex")
		}
		return filepath.Join(base, "skills"), "Codex 0.162.1"
	case "claude-code":
		base := os.Getenv("CLAUDE_CONFIG_DIR")
		if base == "" {
			base = filepath.Join(home, ".claude")
		}
		return filepath.Join(base, "skills"), "Claude Code 2.1.300"
	case "opencode":
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "opencode", "skills"), "OpenCode 1.17.15"
	}
	return "", ""
}
func safeSetupPath(home, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("user-level destination must be absolute")
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return errors.New("destination is outside HOME; custom external user destinations are unverified and refused")
	}
	for p := path; p != home; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination ancestry contains a symlink: %s; unsupported isolation, preserved", p)
		}
		if !info.IsDir() {
			return fmt.Errorf("destination ancestry is not a directory: %s", p)
		}
	}
	return nil
}
func existingSetup(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "conflict", nil
	}
	expected := map[string]string{}
	for _, file := range companion.Bundled().Files {
		expected[file.Path] = file.Content
	}
	count := 0
	entries := 0
	conflict := false
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if current == path {
			return nil
		}
		entries++
		if entries > 10000 {
			return errors.New("existing companion exceeds bounded entry limit; preserved")
		}
		rel, e := filepath.Rel(path, current)
		if e != nil {
			return e
		}
		if entry.IsDir() {
			allowed := false
			for file := range expected {
				if strings.HasPrefix(file, rel+string(filepath.Separator)) {
					allowed = true
				}
			}
			if !allowed {
				conflict = true
				return filepath.SkipDir
			}
			return nil
		}
		want, ok := expected[rel]
		if !ok || !entry.Type().IsRegular() {
			conflict = true
			return nil
		}
		f, e := os.OpenFile(current, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
		if e != nil {
			return e
		}
		opened, e := f.Stat()
		if e != nil || !opened.Mode().IsRegular() {
			f.Close()
			return errors.New("existing companion entry is not a regular file")
		}
		raw, e := io.ReadAll(io.LimitReader(f, int64(len(want))+1))
		f.Close()
		if e != nil {
			return e
		}
		if string(raw) != want {
			conflict = true
		}
		count++
		return nil
	})
	if err != nil {
		return "", err
	}
	if conflict || count != len(expected) {
		return "conflict", nil
	}
	return "identical", nil
}

// Setup advertises the exact destinations independently observed by the native
// probe. Discovery support for an override alone does not validate installation.
func unverifiedSetupOverride(app, home string) string {
	key, expected := "", ""
	switch app {
	case "codex":
		key = "CODEX_HOME"
		expected = filepath.Join(home, ".codex")
	case "claude-code":
		key = "CLAUDE_CONFIG_DIR"
		expected = filepath.Join(home, ".claude")
	case "opencode":
		key = "XDG_CONFIG_HOME"
		expected = filepath.Join(home, ".config")
	}
	if raw := os.Getenv(key); raw != "" && filepath.Clean(raw) != expected {
		return key + " selects a non-default user destination; companion setup at overridden homes is unverified and unsupported"
	}
	return ""
}
