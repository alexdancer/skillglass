# ALE-24 verification record

ALE-24 adds reference-aware source identity, custom and project provenance, searchable problems, AND-combined view filters, and explicit browser refresh. The [README](../../README.md#build-and-use) owns the current commands, problem codes, JSON fields, browser keys, selection rules, and exit semantics. This document owns current combined-suite verification procedures and ALE-24 coverage; the [ALE-23 record](ale-23.md#runtime-containment-boundary) owns inherited containment limits.

## Corrections made from executable evidence

- The accepted ALE-23 executable rejected `--project-root`, which reproduced the missing public capability before implementation.
- Darwin can report a symlink cycle as a bare `EvalSymlinks: too many links` error. Cycle classification now recognizes typed `ELOOP` or that exact unwrapped sentinel instead of scanning arbitrary path text. Missing targets containing `too many links` or `too many levels of symbolic links` remain `broken-reference` problems.
- An early query projection retained references only through resolved sources. Unresolved broken and cyclic references therefore disappeared while problems still named their IDs. Views now retain references selected through either visible sources or visible problem relationships, including their original path, status, filter match, and diagnostic.
- A fixed filter row allowed a long selected root to hide context and problem values. The browser now renders complete bounded filter fields, combines them only when they fit, and puts the full root path in the scrollable scope view.
- Adding selection identity to the header initially reduced the 80 by 24 detail body enough to hide the inherited final-instruction PTY evidence. Selection and filter fields now share rows only when complete, preserving the body, controls, selector, and filter dimensions.
- Terminal delta rendering could emit only the changed suffix of `Resources:` after an end jump. The end action requests a complete repaint, retaining the raw PTY proof as well as the current-cell proof.

## Public behavior covered

`TestALE24GroupsOnlyProvenDirectoryReferences` uses two explicit roots and proves the identity boundary in one public JSON result. Two directory aliases become one source with two references. Independent byte-for-byte copies, divergent same-name copies, and directories that share only a manifest target remain separate. Reading each manifest-only alias keeps its own supporting resources.

`TestALE24FiltersUseReferenceProvenanceWithANDSemantics` covers custom and project roots, selected-root filtering, context filtering, problem filtering, search, invalid inputs, and the same normalized root supplied through both contexts. It verifies that filters narrow matching references without manufacturing ownership or availability evidence.

`TestALE24ProblemsRemainSearchableWithUsablePartialInventory` combines healthy sources with missing roots, broken references, directory and entry cycles, unreadable and malformed manifests, and external resource links. Each reported problem retains a path and reason, every reported reference ID resolves to preserved reference data, and unresolved references remain source-free. Missing targets whose names contain cycle-like error phrases stay distinct from real cycles. Healthy source detail remains readable while the complete selected scope returns status 1.

`TestRootCyclesRemainClassifiedThroughAncestors` covers direct and ancestor root cycles, candidate and manifest cycles, and non-cycle controls. It checks original root diagnostic paths, root state, filtered JSON and human output, healthy partial inventory, exit status, and fixture immutability.

`TestALE24FreshInvocationRetainsObjectIdentityAcrossReferenceChanges` proves that a new CLI invocation sees source insertion, manifest edits, broken references, and alias retargeting. A surviving alias retains the original source ID.

`TestALE24BrowserModesAndReferencesAtRequiredWidths` drives real tmux panes at 48, 60, 80, 96, and 120 columns. It inspects Sources and Problems headings, complete active filter fields, both custom and project references through scrolling, body space, controls, and safe cell widths.

`TestALE24BrowserRefreshSelectionAndProblemPaste` selects a source by its public ID, then applies external rename, row insertion, manifest edit, one-alias retarget, final-alias retarget, and equal-byte path replacement changes. Refresh retains the same proven object through rename and a surviving alias, retains it while filters hide it, and visibly clears it after loss or replacement. The same journey searches Problems with bracketed-pasted shortcut letters and opens the related problem detail.

`TestReviewProblemSearchUsesKnownProvenance` exercises the README's search contract with code, summary, detail, resolved-path, alias, requested-root, and case-folding queries; filter conjunctions; another associated reference's search text; and root-only failures. `TestReviewBrowserProblemSearchFindsOriginalAlias` covers an uppercase original-alias query in a real terminal.

The full inherited ALE-23 coverage continues to run; see the [ALE-23 record](ale-23.md#reproduce-the-acceptance-evidence) for its journeys. Terminal action synchronization was strengthened rather than left unchanged: [the shared action driver](../../tests/acceptance/terminal_action_test.go) captures pre-action cells and dimensions, rejects an already-satisfied postcondition, and waits for changed cells with the semantic postcondition on the current grid. Resize also requires the target dimensions. Ignored paste uses subsequent observable actions as barriers. The strict end-frame scope-restoration assertion remains in `TestBrowserManyRootsHasScrollableScopeAt60Columns`.

`TestBrowserTmuxCellsAtRequiredWidths` compares description search with public CLI JSON and uses the guarded current-cell boundary to require the completed Search field, editing state, expected source, and `Focus: 1/1`, with the nonmatching row absent. Opening detail verifies that source's public selector. `TestBrowserPTYJourney` retains the minimal source/detail/end/no-match journey and terminal restoration; its raw waits inspect bytes after action-specific checkpoints. A description literal already displayed in inventory is not evidence that search completed, and incremental redraws need not re-emit the full Search prefix.

Historical ALE-23 hostile inventory and purported detail captures were byte-identical and do not prove the intermediate detail transition. Keep that limitation with the historical evidence rather than relabeling those frames. Current hostile-detail capture uses the guarded action boundary in `TestBrowserHostileTerminalDataUsesSafeCells`.

## Reproduce the acceptance evidence

Use macOS, the [README's Go toolchain and dependency setup](../../README.md#build-and-use), and `tmux` on PATH. Containment checks require macOS `sandbox-exec`; scheduler checks require Python 3.10 or later; shell lint requires `shellcheck` on PATH. Dependency downloads must already be present because the run disables Go network access. Use a dedicated temporary directory outside every Git checkout for each worker's `TMPDIR` and `GOTMPDIR`: placing temporary homes or projects under a checkout can make ancestor discovery inherit its real Git boundary. Supply a fresh absolute evidence directory so terminal captures and command receipts cannot mix with an earlier run:

```sh
export TMPDIR="$(mktemp -d /private/tmp/ale-acceptance.XXXXXX)"
export GOTMPDIR="$TMPDIR/go"
mkdir -p "$GOTMPDIR"
evidence="$(pwd)/.agent-loop/ale-24-evidence-$(date +%Y%m%d-%H%M%S)"
test ! -e "$evidence" && ./tests/acceptance/run.sh "$evidence"
```

Inspect `cases.json`, raw PTY bytes, driven keys, pane dimensions, and current tmux cells. The acceptance driver builds the public executable and launches it with synthetic HOME, XDG, project, temporary, and current directories. Inspect fixture before/after manifests alongside command and browser receipts for source/settings immutability. The [inherited containment boundary](ale-23.md#runtime-containment-boundary) defines runtime restrictions, positive controls, and their limits.

Run repository checks after the last source or documentation change:

```sh
/opt/homebrew/bin/gofmt -l cmd internal tests/acceptance
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go vet ./...
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go test -count=1 -mod=readonly ./...
python3 -m unittest discover -s tests/agent_loop -v
python3 -m unittest discover -s tests/native_verification -v
python3 -m py_compile tools/claude_containment.py tools/verify_claude.py tools/verify_claude_plugin_manifest.py tools/verify_codeaf.py tests/native_verification/test_verify_claude.py
shellcheck tests/acceptance/run.sh
sh -n tests/acceptance/run.sh
git diff --check
```

The formatting check must print no files; use `gofmt -w` on any reported files and rerun. These are local checks, not a claim of green CI. Earlier verification does not cover later changes.

## Evidence limits

Recorded Darwin validation consistently supplied directory object metadata. The `identity-uncertain` fallback in [discovery](../../internal/collection/discover.go) could not be induced on that filesystem and is not claimed as runtime-tested coverage. The [README](../../README.md#build-and-use) owns identity and refresh-selection semantics.

Permission fixtures verify that the test process itself receives a permission error before accepting an unreadable-path assertion. Environments that bypass POSIX permissions cannot claim that coverage from a mode change alone.
