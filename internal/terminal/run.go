package terminal

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/alexdancer/skillglass/internal/collection"
)

const usage = `Usage:
  skillglass list --root ROOT [--root ROOT...] [--project PROJECT] [--search TEXT] [--json]
  skillglass read --root ROOT [--root ROOT...] [--project PROJECT] [--json] SOURCE_ID
  skillglass browse --root ROOT [--root ROOT...] [--project PROJECT]

At least one explicit --root is required. Project context never adds an implicit root.
`

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

type optionalString struct {
	set   bool
	value string
}

func (value *optionalString) String() string {
	return value.value
}

func (value *optionalString) Set(raw string) error {
	value.set = true
	value.value = raw
	return nil
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeError(stderr, errors.New("a subcommand is required"))
		io.WriteString(stderr, usage)
		return 2
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		io.WriteString(stdout, usage)
		return 0
	}
	switch args[0] {
	case "list":
		return runList(args[1:], stdout, stderr)
	case "read":
		return runRead(args[1:], stdout, stderr)
	case "browse":
		return runBrowse(args[1:], stdin, stdout, stderr)
	default:
		writeError(stderr, fmt.Errorf("unknown subcommand %q", args[0]))
		io.WriteString(stderr, usage)
		return 2
	}
}

func runList(args []string, stdout, stderr io.Writer) int {
	flags, roots, project, jsonOutput := commonFlags("list")
	search := flags.String("search", "", "search names and descriptions")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}
	if flags.NArg() != 0 {
		writeError(stderr, errors.New("list does not accept positional arguments"))
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(*roots, *project, stderr)
	if snapshot == nil {
		return code
	}

	var output []byte
	var err error
	if *jsonOutput {
		output, err = encodeListJSON(snapshot, *search)
	} else {
		output = []byte(renderList(snapshot, *search))
	}
	if err != nil {
		writeError(stderr, fmt.Errorf("render list: %w", err))
		return 1
	}
	if _, err := stdout.Write(output); err != nil {
		writeError(stderr, fmt.Errorf("write output: %w", err))
		return 1
	}
	if !snapshot.Complete() {
		return 1
	}
	return 0
}

func runRead(args []string, stdout, stderr io.Writer) int {
	flags, roots, project, jsonOutput := commonFlags("read")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.WriteString(stdout, usage)
			return 0
		}
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}
	if flags.NArg() != 1 {
		writeError(stderr, errors.New("read requires exactly one SOURCE_ID after the flags"))
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(*roots, *project, stderr)
	if snapshot == nil {
		return code
	}
	id := collection.SourceID(flags.Arg(0))

	var output []byte
	var found bool
	var err error
	if *jsonOutput {
		output, err = encodeReadJSON(snapshot, id)
		_, found = snapshot.Source(id)
	} else {
		var rendered string
		rendered, found = renderRead(snapshot, id)
		output = []byte(rendered)
	}
	if err != nil {
		writeError(stderr, fmt.Errorf("render source: %w", err))
		return 1
	}
	if _, err := stdout.Write(output); err != nil {
		writeError(stderr, fmt.Errorf("write output: %w", err))
		return 1
	}
	if !found || !snapshot.Complete() {
		return 1
	}
	return 0
}

func commonFlags(name string) (*flag.FlagSet, *stringList, *optionalString, *bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var roots stringList
	var project optionalString
	flags.Var(&roots, "root", "explicit discovery root; repeatable")
	flags.Var(&project, "project", "selected project context")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	return flags, &roots, &project, jsonOutput
}

func buildSnapshot(roots stringList, project optionalString, stderr io.Writer) (*collection.Snapshot, int) {
	var projectValue *string
	if project.set {
		projectValue = &project.value
	}
	request, err := collection.NewRequest(roots, projectValue)
	if err != nil {
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return nil, 2
	}
	snapshot, err := collection.Build(context.Background(), request)
	if err != nil {
		writeError(stderr, fmt.Errorf("build collection: %w", err))
		return nil, 1
	}
	return snapshot, 0
}

func writeError(writer io.Writer, err error) {
	fmt.Fprintf(writer, "skillglass: %s\n", safeInline(err.Error()))
}
