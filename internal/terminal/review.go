package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/alexdancer/skillglass/internal/collection"
	"github.com/alexdancer/skillglass/internal/companion"
)

func runCompanion(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("companion", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	machine := flags.Bool("json", false, "emit the versioned bundled files and hashes")
	file := flags.String("file", "", "emit one exact bundle-relative file as text")
	if err := flags.Parse(args); err != nil {
		return reviewFailure("companion", err, 2, reviewJSONRequested(args), stdout, stderr)
	}
	if flags.NArg() != 0 {
		return reviewFailure("companion", errors.New("companion accepts no positional arguments"), 2, *machine, stdout, stderr)
	}
	bundle := companion.Bundled()
	if *file != "" {
		for _, entry := range bundle.Files {
			if entry.Path == *file {
				if *machine {
					return writeReviewJSON(map[string]any{"schema_version": 1, "operation": "companion", "name": bundle.Name, "version": bundle.Version, "review_contract_version": bundle.ContractVersion, "file": entry}, stdout, stderr)
				}
				_, err := io.WriteString(stdout, entry.Content)
				if err != nil {
					writeError(stderr, err)
					return 1
				}
				return 0
			}
		}
		return reviewFailure("companion", errors.New("unknown bundled file; use companion --json to list exact paths"), 2, *machine, stdout, stderr)
	}
	if *machine {
		return writeReviewJSON(bundle, stdout, stderr)
	}
	fmt.Fprintf(stdout, "%s %s | review contract %d\n", bundle.Name, bundle.Version, bundle.ContractVersion)
	for _, entry := range bundle.Files {
		fmt.Fprintf(stdout, "%s | sha256 %s\n", entry.Path, entry.SHA256)
	}
	io.WriteString(stdout, "Read with: skillglass companion --file SKILL.md\nBundled files only; no installation or availability writes.\n")
	return 0
}

func runReview(args []string, stdout, stderr io.Writer) int {
	operation := "review"
	if len(args) == 0 {
		return reviewFailure(operation, errors.New("review requires prepare or select"), 2, false, stdout, stderr)
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if reviewJSONRequested(args[1:]) {
			return writeReviewJSON(map[string]any{"schema_version": 1, "operation": "review-help", "help": usage}, stdout, stderr)
		}
		io.WriteString(stdout, usage)
		return 0
	}
	operation = "review-" + args[0]
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	machine := flags.Bool("json", false, "emit one versioned JSON object")
	draftPath, proposalPath := "", ""
	var approval optionalString
	var chosen stringList
	switch args[0] {
	case "prepare":
		flags.StringVar(&draftPath, "draft", "", "agent-authored review draft JSON file")
	case "select":
		flags.StringVar(&proposalPath, "proposal", "", "complete proposal JSON file")
		flags.Var(&chosen, "choose", "chosen change ID; repeat; execution retains proposal order")
		flags.Var(&approval, "approve", "explicit user approval of this exact selection_id")
	default:
		return reviewFailure(operation, errors.New("review requires prepare or select"), 2, reviewJSONRequested(args) || reviewJSONRequested(args[1:]), stdout, stderr)
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			if reviewJSONRequested(args[1:]) {
				return writeReviewJSON(map[string]any{"schema_version": 1, "operation": operation, "help": usage}, stdout, stderr)
			}
			io.WriteString(stdout, usage)
			return 0
		}
		return reviewFailure(operation, err, 2, reviewJSONRequested(args[1:]), stdout, stderr)
	}
	if flags.NArg() != 0 {
		return reviewFailure(operation, errors.New("review accepts flags only; paths use --draft or --proposal"), 2, *machine, stdout, stderr)
	}
	if args[0] == "prepare" {
		var draft collection.ReviewDraft
		if err := readReviewFile(draftPath, &draft); err != nil {
			return reviewFailure(operation, err, 2, *machine, stdout, stderr)
		}
		proposal, err := collection.PrepareReview(context.Background(), draft)
		if err != nil {
			return reviewFailure(operation, err, 2, *machine, stdout, stderr)
		}
		code := 0
		if proposal.State != "prepared" {
			code = 1
		}
		if *machine {
			if status := writeReviewJSON(proposal, stdout, stderr); status != 0 {
				return status
			}
			return code
		}
		renderReviewProposal(stdout, proposal)
		return code
	}
	var proposal collection.ReviewProposal
	if err := readReviewFile(proposalPath, &proposal); err != nil {
		return reviewFailure(operation, err, 2, *machine, stdout, stderr)
	}
	selection, err := collection.SelectReview(context.Background(), proposal, chosen)
	if err != nil {
		return reviewFailure(operation, err, 1, *machine, stdout, stderr)
	}
	if approval.set {
		selection, err = collection.ApproveReview(selection, approval.value)
		if err != nil {
			return reviewFailure(operation, err, 1, *machine, stdout, stderr)
		}
	}
	if *machine {
		return writeReviewJSON(selection, stdout, stderr)
	}
	fmt.Fprintf(stdout, "Selection %s | %s\nChosen in proposal order: %s\nOmitted (unapproved): %s\n", selection.SelectionID, selection.State, displayOptional(strings.Join(selection.Chosen, ", ")), displayOptional(strings.Join(selection.Omitted, ", ")))
	fmt.Fprintf(stdout, "Scope: %s | project: %s\n", selection.Proposal.Draft.Context.Scope, displayOptional(selection.Proposal.Draft.Context.Project))
	chosenChanges := make(map[string]bool, len(selection.Chosen))
	for _, id := range selection.Chosen {
		chosenChanges[id] = true
	}
	for _, change := range selection.Proposal.Changes {
		if !chosenChanges[change.Recommendation.ID] {
			continue
		}
		r := change.Recommendation
		fmt.Fprintf(stdout, "Selected %s: %s %s | scope: %s\nSource ID: %s\nSource: %s\n", safeInline(r.ID), r.Application, r.Action, selection.Proposal.Draft.Context.Scope, safeInline(string(r.Source)), safeInline(change.Baseline.SourcePath))
	}
	renderReviewEffects(stdout, selection.Effects)
	if selection.State == "preview" {
		fmt.Fprintf(stdout, "After explicit user approval, repeat this exact selection with --approve %s\n", selection.SelectionID)
	}
	io.WriteString(stdout, "No source content or settings changed. Collection execution is not included in this build.\n")
	return 0
}

func renderReviewProposal(stdout io.Writer, p collection.ReviewProposal) {
	fmt.Fprintf(stdout, "Proposal %s | %s\nApplications: %s\nScope: %s | project: %s\nGoal: %s\nKeep enabled: %v\n", p.ProposalID, p.State, strings.Join(p.Draft.Context.Applications, ", "), p.Draft.Context.Scope, displayOptional(p.Draft.Context.Project), safeInline(p.Draft.Context.Goal), p.Draft.Context.KeepEnabled)
	for _, change := range p.Changes {
		r := change.Recommendation
		fmt.Fprintf(stdout, "%s. %s %s | %s | %s\nSource: %s\nReason: %s\nNative preview: %s | %s\n", safeInline(r.ID), r.Application, r.Action, p.Draft.Context.Scope, r.Source, safeInline(change.Baseline.SourcePath), safeInline(r.Reason), change.Baseline.State, safeInline(change.Baseline.Reason))
		for _, consumer := range change.Consumers {
			if len(consumer.References) > 0 {
				fmt.Fprintf(stdout, "Shared consumer %s: %d references | discovery %s\n", consumer.Application, len(consumer.References), consumer.Scope.Completeness)
			}
		}
	}
	renderReviewEffects(stdout, p.Effects)
	for _, source := range p.Sources {
		fmt.Fprintf(stdout, "Assessment %s (%s): %s\n", source.Source.ID, strings.Join(source.Applications, ", "), safeInline(source.Assessment))
		for _, e := range source.Evidence {
			fmt.Fprintf(stdout, "Evidence %s: %s | %s | sha256 %s\n", safeInline(e.Path), e.Coverage, e.Representation, e.SHA256)
			for _, d := range e.Diagnostics {
				fmt.Fprintf(stdout, "%s: %s\n", d.Code, safeInline(d.Message))
			}
		}
		for _, l := range source.Limitations {
			fmt.Fprintf(stdout, "Limitation: %s\n", safeInline(l))
		}
	}
	for _, u := range p.Draft.Unchanged {
		fmt.Fprintf(stdout, "Unchanged %s %s: %s\n", u.Application, u.Source, safeInline(u.Reason))
	}
	for _, q := range p.Draft.Unresolved {
		fmt.Fprintf(stdout, "Unresolved: %s\n", safeInline(q))
	}
	fmt.Fprintf(stdout, "Coverage: discovery %s | selected evidence %s | unreviewed source/application pairs %d\n", p.Coverage.Discovery, p.Coverage.SelectedEvidence, len(p.Coverage.Unreviewed))
	for _, pair := range p.Coverage.Unreviewed {
		fmt.Fprintf(stdout, "Unreviewed: %s %s\n", pair.Application, pair.Source)
	}
	for _, l := range p.Limitations {
		fmt.Fprintf(stdout, "%s\n", safeInline(l))
	}
	io.WriteString(stdout, "Use --json to preserve the complete proposal, then review select --proposal FILE --choose ID ... for a subset preview. Omitted recommendations are unapproved.\n")
}
func renderReviewEffects(stdout io.Writer, effects []collection.ReviewEffect) {
	for i, e := range effects {
		fmt.Fprintf(stdout, "Effect %d (%s): %s | %s %s %s\nConfig bytes: %s -> %s | no write needed: %t\n", i+1, safeInline(e.ID), safeInline(e.ConfigPath), e.Intent.Application, e.Intent.Scope, e.Intent.Choice, e.BeforeSHA256, e.AfterSHA256, e.NoWriteNeeded)
		for _, change := range e.Changes {
			fmt.Fprintf(stdout, "Setting %s: %s -> %s\n", safeInline(change.Setting), safeInline(change.Before), safeInline(change.After))
		}
	}
}

const reviewFileLimit = 64 << 20

func readReviewFile(path string, target any) error {
	if path == "" {
		return errors.New("an explicit JSON file path is required")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("read review file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("review input must be a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, reviewFileLimit+1))
	if err != nil {
		return err
	}
	if len(raw) > reviewFileLimit {
		return errors.New("review input exceeds 64 MiB")
	}
	if err := uniqueJSONKeys(json.NewDecoder(bytes.NewReader(raw))); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid review JSON: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("review input must contain exactly one JSON object")
	}
	return nil
}

// Reject duplicate keys at every depth rather than silently choosing a meaning.
func uniqueJSONKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("invalid review JSON: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			text, ok := key.(string)
			if !ok || seen[text] {
				return errors.New("duplicate or invalid JSON object key")
			}
			seen[text] = true
			if err := uniqueJSONKeys(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSONKeys(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}
func reviewJSONRequested(args []string) bool {
	machine := false
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") || args[i] == "--" {
			break
		}
		key, value, has := strings.Cut(args[i], "=")
		if key == "--json" || key == "-json" {
			machine = !has || value != "false"
		} else if (key == "--draft" || key == "--proposal" || key == "--choose" || key == "--approve" || key == "--file") && !has {
			i++
		}
	}
	return machine
}
func reviewFailure(operation string, err error, code int, machine bool, stdout, stderr io.Writer) int {
	if errors.Is(err, flag.ErrHelp) {
		if machine {
			return writeReviewJSON(map[string]any{"schema_version": 1, "operation": operation, "help": usage}, stdout, stderr)
		}
		io.WriteString(stdout, usage)
		return 0
	}
	if machine {
		if result := writeReviewJSON(map[string]any{"schema_version": 1, "operation": operation, "state": "refused", "exit_code": code, "failure": map[string]string{"message": err.Error(), "action": "Inspect the companion contract and help, correct the input or reprepare stale evidence, and obtain approval of the exact selection before availability writes."}, "help": usage}, stdout, stderr); result != 0 {
			return result
		}
	} else {
		writeError(stderr, err)
	}
	return code
}
func writeReviewJSON(value any, stdout, stderr io.Writer) int {
	raw, err := encodeJSON(value)
	if err == nil {
		_, err = stdout.Write(raw)
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	return 0
}
