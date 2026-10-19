package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/alexdancer/skillglass/internal/collection"
)

// Run preserves human output and gives inspection commands a single JSON result,
// including errors that occur before a request or snapshot can be constructed.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "review" {
		return runReview(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "companion" {
		return runCompanion(args[1:], stdout, stderr)
	}
	machine := inspectionJSONRequested(args)
	operation := ""
	if len(args) > 0 {
		operation = args[0]
	}
	inspection := operation == "list" || operation == "problems" || operation == "read" || operation == "explain" || operation == "read-resource" || operation == "compare" || operation == "help" || operation == "--help" || operation == "-h"
	// Unknown commands in machine mode also need parseable usage failures.
	if !machine || (!inspection && (operation == "enable" || operation == "disable" || operation == "inherit" || operation == "delete" || operation == "browse")) {
		return run(args, stdin, stdout, stderr)
	}
	var output, diagnostics bytes.Buffer
	code := run(args, stdin, &output, &diagnostics)
	envelope := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		envelope = map[string]any{"operation": operation, "scope": nil, "help": usage}
		if code != 0 {
			envelope["error"] = strings.TrimSpace(diagnostics.String())
		}
	}
	envelope["schema_version"] = 1
	envelope["request"] = map[string]any{"arguments": args[1:]}
	envelope["exit_code"] = code
	discovery, selected := "not-started", "not-applicable"
	if scope, ok := envelope["scope"].(map[string]any); ok {
		discovery, _ = scope["completeness"].(string)
	}
	if discovery != "not-started" && (operation == "read" || operation == "explain") {
		selected = "failed"
		if source, ok := envelope["source"].(map[string]any); ok {
			manifest, _ := source["manifest"].(map[string]any)
			if manifest["status"] != "unreadable" {
				selected = "complete"
			}
		}
	}
	if resource, ok := envelope["resource_read"].(map[string]any); ok {
		selected, _ = resource["coverage"].(string)
	}
	if discovery != "not-started" && operation == "compare" {
		selected = "failed"
	}
	if report, ok := envelope["report"].(map[string]any); ok {
		selected, _ = report["coverage"].(string)
	}
	envelope["coverage"] = map[string]string{"discovery": discovery, "selected": selected}
	outcome := "success"
	if discovery == "incomplete" {
		outcome = "partial"
	}
	if code != 0 {
		outcome = "partial"
		if code == 2 || discovery == "not-started" || selected == "failed" || selected == "incomplete" {
			outcome = "failed"
		}
		message, _ := envelope["error"].(string)
		if message == "" {
			message = strings.TrimSpace(diagnostics.String())
		}
		errorCode, action := "incomplete-discovery", "Inspect scope and problems; preserve useful results without claiming complete coverage."
		if code == 2 {
			errorCode, action = "usage-error", "Correct the arguments using help and retry."
		} else if selected == "failed" {
			errorCode, action = "selected-source-unreadable", "Rediscover exact IDs in the same scope and inspect source diagnostics."
		} else if selected == "incomplete" {
			errorCode, action = "selected-read-incomplete", "Inspect resource_read.diagnostics or report.issues and disclose missing evidence."
		}
		if message == "" {
			message = "Inspection did not complete; inspect coverage and diagnostics."
		}
		envelope["failure"] = map[string]string{"code": errorCode, "message": message, "action": action}
	}
	envelope["outcome"] = outcome
	raw, err := encodeJSON(envelope)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if _, err := stdout.Write(raw); err != nil {
		writeError(stderr, err)
		return 1
	}
	return code
}

func writeInspectionFailure(operation string, snapshot *collection.Snapshot, failure error, code int, stdout, stderr io.Writer) int {
	raw, err := encodeJSON(map[string]any{
		"operation": operation,
		"scope":     snapshot.Scope(),
		"problems":  snapshot.Problems(),
		"error":     failure.Error(),
		"help":      usage,
	})
	if err == nil {
		_, err = stdout.Write(raw)
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	return code
}

func inspectionJSONRequested(args []string) bool {
	machine := false
	for i := 0; i < len(args); i++ {
		if i == 0 && !strings.HasPrefix(args[i], "-") {
			continue
		}
		// Match flag.FlagSet's boundary: after a positional operand or --,
		// filenames must not select or override the output mode.
		if args[i] == "--" || args[i] == "-" || !strings.HasPrefix(args[i], "-") {
			break
		}
		name, value, hasValue := strings.Cut(args[i], "=")
		switch name {
		case "--json", "-json":
			machine = true
			if hasValue {
				parsed, err := strconv.ParseBool(value)
				if err == nil {
					machine = parsed
				}
			}
		case "--root", "--project-root", "--project", "--agent", "--search", "--source-root", "--context", "--problem", "--availability",
			"-root", "-project-root", "-project", "-agent", "-search", "-source-root", "-context", "-problem", "-availability":
			if !hasValue {
				i++
			}
		}
	}
	return machine
}

func runResource(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("read-resource", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, false, true)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		return 2
	}
	if flags.NArg() != 2 {
		writeError(stderr, errors.New("read-resource requires SOURCE_ID RELATIVE_PATH after flags"))
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	id := collection.SourceID(flags.Arg(0))
	result := snapshot.ReadResource(context.Background(), id, flags.Arg(1))
	if options.jsonOutput {
		source, found := snapshot.Source(id)
		var selected *collection.Source
		if found {
			selected = &source
		}
		raw, err := encodeJSON(struct {
			Operation    string                  `json:"operation"`
			Scope        collection.Scope        `json:"scope"`
			Source       *collection.Source      `json:"source,omitempty"`
			References   []collection.Reference  `json:"references"`
			Problems     []collection.Problem    `json:"problems"`
			ResourceRead collection.ResourceRead `json:"resource_read"`
		}{"read-resource", snapshot.Scope(), selected, snapshot.References(id), snapshot.Problems(), result})
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		if _, err := stdout.Write(raw); err != nil {
			writeError(stderr, err)
			return 1
		}
	} else {
		var output strings.Builder
		fmt.Fprintf(&output, "Skillglass resource\nSource: %s\nPath: %s\nRead: %s | coverage: %s | representation: %s\n", safeInline(string(id)), safeInline(result.Path), result.State, result.Coverage, result.Representation)
		renderScope(&output, snapshot.Scope())
		renderProblems(&output, snapshot.Problems())
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(&output, "%s: %s\n%s\n", safeInline(diagnostic.Code), safeInline(diagnostic.Message), safeInline(diagnostic.Action))
		}
		if result.Text != nil {
			output.WriteString("\n" + safeBlock(*result.Text) + "\n")
		}
		if result.Representation == "base64" {
			fmt.Fprintf(&output, "\nBinary (%d bytes); exact bytes are in resource_read.bytes_base64 with --json.\n", result.BytesRead)
		}
		if _, err := io.WriteString(stdout, output.String()); err != nil {
			writeError(stderr, err)
			return 1
		}
	}
	if result.Coverage != collection.Complete || !snapshot.Complete() {
		return 1
	}
	return 0
}
