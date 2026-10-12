# ALE-24 verification record

ALE-24 adds reference-aware source identity, custom and project provenance, searchable problems, AND-combined view filters, and explicit browser refresh. The [README](../../README.md#build-and-use) owns the current commands, problem codes, JSON fields, browser keys, selection rules, and exit semantics. This document records the public evidence used to verify them.

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

`TestALE24FreshInvocationRetainsObjectIdentityAcrossReferenceChanges` proves that a new CLI invocation sees source insertion, manifest edits, broken references, and alias retargeting. A surviving alias retains the original source ID.

`TestALE24BrowserModesAndReferencesAtRequiredWidths` drives real tmux panes at 48, 60, 80, 96, and 120 columns. It inspects Sources and Problems headings, complete active filter fields, both custom and project references through scrolling, body space, controls, and safe cell widths.

`TestALE24BrowserRefreshSelectionAndProblemPaste` selects a source by its public ID, then applies external rename, row insertion, manifest edit, one-alias retarget, final-alias retarget, and equal-byte path replacement changes. Refresh retains the same proven object through rename and a surviving alias, retains it while filters hide it, and visibly clears it after loss or replacement. The same journey searches Problems with bracketed-pasted shortcut letters and opens the related problem detail.

The full ALE-23 suite runs unchanged. It continues to cover JSON-safe metadata, decoded-key collisions, lossless invalid UTF-8 bytes, root manifest-presence permission failures, overlapping-root problem coverage, bracketed paste, long single-row search tails, scrollable scope, strict viewport restoration, hostile terminal data, and source/settings immutability.

## Reproduce the acceptance evidence

Use macOS, Go 1.27.1, and `tmux` on PATH. Dependency downloads must already be present because the run disables Go network access. Supply a fresh absolute evidence directory so terminal captures and command receipts cannot mix with an earlier run:

```sh
evidence="$(pwd)/.agent-loop/ale-24-evidence-$(date +%Y%m%d-%H%M%S)"
test ! -e "$evidence" && ./tests/acceptance/run.sh "$evidence"
```

Inspect `cases.json`, raw PTY bytes, driven keys, pane dimensions, and current tmux cells. The acceptance driver builds the public executable and launches it with synthetic HOME, XDG, project, temporary, and current directories. The product is denied network access, file writes, unrelated file-content reads, and child execution. Fixture manifests prove that commands and browser journeys do not change sources or settings.

Run repository checks after the last source or documentation change:

```sh
/opt/homebrew/bin/gofmt -l cmd internal tests/acceptance
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go vet ./...
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go test -count=1 -mod=readonly ./...
python3 -m unittest discover -s tests/agent_loop -v
shellcheck tests/acceptance/run.sh
sh -n tests/acceptance/run.sh
git diff --check
```

## Evidence limits

Source identity is proven with the macOS directory object's device, inode, and birth-time information. When that evidence cannot be read, Skillglass keeps the occurrence separate, reports `identity-uncertain`, and does not retain its selection across refresh.

The inherited `sandbox-exec` profile verifies content isolation rather than every metadata operation. It starts from `allow default` because denying ambient metadata reads prevented the dynamic loader or process startup from reaching application code. The evidence therefore proves the five explicit controls above, not a complete macOS capability sandbox.

Permission fixtures verify that the test process itself receives a permission error before accepting an unreadable-path assertion. Environments that bypass POSIX permissions cannot claim that coverage from a mode change alone.
