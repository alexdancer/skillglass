package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alexdancer/skillglass/internal/companion"
	"github.com/alexdancer/skillglass/internal/setup"
)

func runSetup(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var apps, sharedApps stringList
	flags.Var(&apps, "app", "explicitly selected installation target; repeat")
	flags.Var(&sharedApps, "allow-shared-app", "explicitly acknowledge indirect discovery by this application; repeat; installs no separate target")
	selectedCompanion := flags.String("companion", companion.Name, "select the exact bundled companion to preview or install")
	install := flags.Bool("install", false, "install only the bundled companion for the explicit applications")
	check := flags.Bool("check", false, "check PATH executable compatibility without installing")
	machine := flags.Bool("json", false, "emit one JSON setup report")
	fail := func(err error, code int) int {
		if errors.Is(err, flag.ErrHelp) {
			if *machine || setupJSONRequested(args) {
				return writeReviewJSON(map[string]any{"schema_version": 1, "operation": "setup-help", "help": usage}, stdout, stderr)
			}
			io.WriteString(stdout, usage)
			return 0
		}
		if *machine || setupJSONRequested(args) {
			returnCode := writeReviewJSON(map[string]any{"schema_version": 1, "operation": "setup", "state": "refused", "exit_code": code, "failure": map[string]string{"message": err.Error(), "action": "Use setup --app APP ... for a read-only preview, then repeat the explicit applications with --install. Use setup --check for executable discovery."}, "help": usage}, stdout, stderr)
			if returnCode != 0 {
				return returnCode
			}
		} else if code == 0 {
			io.WriteString(stdout, usage)
		} else {
			writeError(stderr, err)
		}
		return code
	}
	if err := flags.Parse(args); err != nil {
		return fail(err, 2)
	}
	if flags.NArg() != 0 {
		return fail(errors.New("setup accepts flags only; arbitrary content and destination paths are unsupported"), 2)
	}
	if *check {
		if len(apps) > 0 || len(sharedApps) > 0 || *install {
			return fail(errors.New("--check is a separate read-only executable check; omit --app, --allow-shared-app and --install"), 2)
		}
		result := setup.CheckExecutableFor(context.Background(), *selectedCompanion)
		code := 0
		if result.State != "compatible" {
			code = 1
		}
		if *machine {
			if c := writeReviewJSON(map[string]any{"schema_version": 1, "operation": "setup-check", "state": result.State, "exit_code": code, "executable": result}, stdout, stderr); c != 0 {
				return c
			}
		} else {
			fmt.Fprintf(stdout, "Executable: %s | %s\n%s\n%s\n", result.State, safeInline(result.Path), safeInline(result.Reason), safeInline(result.Action))
		}
		return code
	}
	report, err := setup.SetupNamed(context.Background(), apps, sharedApps, *install, *selectedCompanion)
	if err != nil {
		return fail(err, 2)
	}
	if *machine {
		if code := writeReviewJSON(report, stdout, stderr); code != 0 {
			return code
		}
		return report.ExitCode
	}
	fmt.Fprintf(stdout, "Companion setup %s | %s %s | user level\nInstallation targets: %v\nAcknowledged shared consumers: %v\nExecutable: %s | %s\n", report.State, report.Name, report.Version, report.Applications, report.AllowedSharedApps, report.Executable.State, safeInline(report.Executable.Path))
	for _, r := range report.Results {
		fmt.Fprintf(stdout, "\n%s: %s\nDestination: %s\nValidated native version: %s\nAffected applications: %v\n%s\nRecovery: %s\n", r.Application, r.State, safeInline(r.Destination), safeInline(r.ValidatedVersion), r.Consumers, safeInline(r.Reason), safeInline(r.Action))
	}
	for _, r := range report.Results {
		if r.Audit != nil {
			for _, warning := range r.Audit.Warnings {
				fmt.Fprintf(stdout, "Warning: %s\n", safeInline(warning))
			}
			for _, e := range r.Audit.ConsumerEvidence {
				fmt.Fprintf(stdout, "Shared discovery %s: %s | %s\n", e.Application, safeInline(e.State), safeInline(e.Reason))
			}
		}
	}
	for _, note := range report.Notes {
		fmt.Fprintln(stdout, note)
	}
	return report.ExitCode
}

// Setup has no positional input or arbitrary path operands. Preserve requested
// JSON for usage errors even when an unsupported flag ends parsing early.
func setupJSONRequested(args []string) bool {
	machine := false
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			break
		}
		key, value, has := strings.Cut(args[i], "=")
		switch key {
		case "--app", "-app", "--allow-shared-app", "-allow-shared-app":
			if !has {
				i++
			}
		case "--json", "-json":
			machine = true
			if has {
				if parsed, err := strconv.ParseBool(value); err == nil {
					machine = parsed
				}
			}
		}
	}
	return machine
}
