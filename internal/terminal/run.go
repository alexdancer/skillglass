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
  skillglass list [SCOPE] [FILTERS] [--search TEXT] [--json]
  skillglass problems [SCOPE] [FILTERS] [--search TEXT] [--json]
  skillglass read [SCOPE] [--json] SOURCE_ID
  skillglass explain [SCOPE] [--json] SOURCE_ID
  skillglass compare [SCOPE] [--json] LEFT_SOURCE_ID RIGHT_SOURCE_ID
  skillglass browse [SCOPE]

Scope:
  --agent APP          codex or claude-code native scope and configured availability
  --root ROOT          custom discovery root; repeatable
  --project PROJECT    selected project context
  --project-root ROOT  discovery root associated with --project; repeatable

Filters:
  --source-root ROOT   one selected normalized root
  --context VALUE      all, project, custom, global, system, or managed (default all)
  --problem CODE       one documented problem code
  --availability STATE all, available, disabled, explicit-only, ask, shadowed, unavailable, unknown

Without --agent, at least one explicit --root or --project-root is required. The selected agent adds native roots for the selected project only.
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

type commonOptions struct {
	agent        string
	availability string
	roots        stringList
	projectRoots stringList
	project      optionalString
	sourceRoot   string
	context      string
	problem      string
	jsonOutput   bool
}

func (options *commonOptions) bind(flags *flag.FlagSet, includeFilters, includeJSON bool) {
	flags.StringVar(&options.agent, "agent", "", "selected agent application (codex or claude-code)")
	flags.Var(&options.roots, "root", "explicit custom discovery root; repeatable")
	flags.Var(&options.projectRoots, "project-root", "explicit project-associated discovery root; repeatable")
	flags.Var(&options.project, "project", "selected project context")
	if includeFilters {
		flags.StringVar(&options.availability, "availability", "", "filter by configured availability")
		flags.StringVar(&options.sourceRoot, "source-root", "", "filter to one selected root")
		flags.StringVar(&options.context, "context", "all", "filter by all, project, or custom context")
		flags.StringVar(&options.problem, "problem", "", "filter by a documented problem code")
	}
	if includeJSON {
		flags.BoolVar(&options.jsonOutput, "json", false, "emit machine-readable JSON")
	}
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
	case "problems":
		return runProblems(args[1:], stdout, stderr)
	case "read", "explain":
		return runSource(args[0], args[1:], stdout, stderr)
	case "compare":
		return runCompare(args[1:], stdout, stderr)
	case "browse":
		return runBrowse(args[1:], stdin, stdout, stderr)
	default:
		writeError(stderr, fmt.Errorf("unknown subcommand %q", args[0]))
		io.WriteString(stderr, usage)
		return 2
	}
}

func runList(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, true, true)
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
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	filters, err := optionFilters(snapshot, options, *search)
	if err != nil {
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}

	var output []byte
	if options.jsonOutput {
		output, err = encodeListJSON(snapshot, filters)
	} else {
		output = []byte(renderList(snapshot, filters))
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

func runProblems(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("problems", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, true, true)
	search := flags.String("search", "", "search problem codes, messages, and discovery paths")
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
		writeError(stderr, errors.New("problems does not accept positional arguments"))
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	filters, err := optionFilters(snapshot, options, *search)
	if err != nil {
		writeError(stderr, err)
		io.WriteString(stderr, usage)
		return 2
	}
	var output []byte
	if options.jsonOutput {
		output, err = encodeProblemsJSON(snapshot, filters)
	} else {
		output = []byte(renderProblemList(snapshot, filters))
	}
	if err != nil {
		writeError(stderr, fmt.Errorf("render problems: %w", err))
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

func runSource(operation string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var options commonOptions
	options.bind(flags, false, true)
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
		writeError(stderr, fmt.Errorf("%s requires exactly one SOURCE_ID after the flags", operation))
		io.WriteString(stderr, usage)
		return 2
	}
	snapshot, code := buildSnapshot(options, stderr)
	if snapshot == nil {
		return code
	}
	id := collection.SourceID(flags.Arg(0))

	var output []byte
	var found bool
	var err error
	if options.jsonOutput {
		output, err = encodeSourceJSON(snapshot, id, operation)
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

func buildSnapshot(options commonOptions, stderr io.Writer) (*collection.Snapshot, int) {
	var projectValue *string
	if options.project.set {
		projectValue = &options.project.value
	}
	request, err := collection.NewRequestWithAgent(options.roots, options.projectRoots, projectValue, options.agent)
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
