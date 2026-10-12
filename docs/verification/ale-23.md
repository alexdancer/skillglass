# ALE-23 verification record

ALE-23 adds the first production `skillglass` executable. Its immutable collection snapshot is shared by both terminal interfaces. The [README](../../README.md#build-and-use) owns current commands, discovery scope, browser controls, manifest representations, and exit semantics; this document owns verification procedures and containment limits.

## Corrections made from executable evidence

- The first red journey failed with exit 127 because no production executable existed. The implemented vertical slice made the same public `list --root ... --json` journey pass.
- An unreadable empty root initially fabricated an unnamed source because an `Lstat` permission error was treated as proof that `SKILL.md` existed. Discovery now reports the root as incomplete and does not invent a bundle.
- The first browser frame repeated the selected-source header. The duplicate row was removed.
- The original end-of-document calculation could leave the last wrapped resource field beyond the reachable viewport. Detail pagination now reserves the actual wrapped footer height, and `G` exposes the complete last resource.
- Key help originally split words at narrow widths. Help wraps whole key-action groups at separators.
- A readable manifest inside a mode `0111` bundle was retained, but failure to enumerate its resources was initially missed. Resource discovery now handles a root `WalkDir` error before skipping the bundle directory, reports `unreadable-resource`, preserves the useful source, and exits with status 1 because coverage is incomplete.
- A directly selected bundle with directory mode `0444` could be listed but not traversed. Its failed manifest-presence probe was discarded, fabricating complete empty coverage. Successful partial enumeration now retains the same `unreadable-manifest` presence diagnostic as child candidates; a failed root listing still produces only `root-unreadable`.
- Invalid UTF-8 in instructions, frontmatter, or an unsplittable manifest was silently replaced in exported text. `TestManifestRawEncodingPreservesExactBytes` and `TestBrowserInvalidManifestEncodingHasRecoveryHint` now cover the README's lossless representation and recovery contract, with valid UTF-8, literal U+FFFD, and empty-file controls.
- YAML non-finite floats initially made JSON encoding fail after discovery. `TestNonFiniteMetadataHasTaggedJSONRepresentation` now checks the README's portable tags and diagnostic contract, nested values, raw text, and healthy siblings.
- Nested non-string mapping keys could prevent the entire JSON inventory from rendering, while top-level keys could be coerced to strings. `TestUnsupportedMetadataPreservesPartialInventory` and `TestStringKeyedMetadataKeepsNestedValues` cover rejection and supported structured-metadata controls under the README's manifest contract.
- YAML binary and alias keys could overwrite metadata, and decoded binary strings could be silently altered by JSON. `TestMetadataRepresentabilityPreservesPartialInventory` checks collisions and invalid strings, including overridden merge values; `TestMetadataRepresentabilityKeepsValidStringsAndMergeOverrides` supplies positive controls.
- Bracketed paste was ignored by the browser. `TestBrowserBracketedPasteIsSearchData` now covers filtering, pasted controls, keep/cancel/clear transitions, and ignored paste outside search.
- Overlapping roots could lose cached source diagnostics in whichever root was inspected second. Each root now counts unique diagnostics from its own references and sources, including reused sources, while global source diagnostics remain deduplicated. Informational warnings do not limit coverage.
- Long project paths and many discovery roots could consume the entire browser viewport. `TestBrowserManyRootsHasScrollableScopeAt60Columns` checks bounded headers, reachable full scope, and restoration of the inventory and scrolled detail position.
- A long search query could still wrap across the whole browser viewport. `TestBrowserLongSearchUsesSingleRowTailViewport` checks the README's search-tail behavior, full-query filtering, and editing across a resize.

## Reproduce the acceptance evidence

Use macOS, the Go toolchain and dependency setup in the [README](../../README.md#build-and-use), and `tmux` on PATH. The containment checks also require macOS `sandbox-exec`. Python scheduler checks require Python 3.10 or later; shell lint requires `shellcheck` on PATH.

The harness builds with the checked-in module graph while Go network access is disabled, creates fresh HOME, XDG, project, and collection fixtures, and then tests the public executable. Supply a fresh absolute evidence directory for every run so captures and receipts cannot mix with an earlier run:

```sh
evidence="$(pwd)/.agent-loop/ale-23-evidence-$(date +%Y%m%d-%H%M%S)"
test ! -e "$evidence" && ./tests/acceptance/run.sh "$evidence"
```

Inspect the emitted raw terminal output, keys, dimensions, and tmux cell captures, not just the test summary.

It covers complete, empty, missing, unreadable, mixed, malformed, hostile-control, invalid-encoding, metadata-representability, non-finite metadata, overlapping-root, unlistable bundle resources, and renamed-directory cases. It checks exit codes 0, 1, and 2; exact JSON data; safe human rendering; full instructions and nested resources; fixture immutability; no script execution; and nonblocking FIFO discovery.

The browser coverage includes a public PTY journey and real tmux cell captures at 48, 60, 80, 96, and 120 columns by 30 rows. The journey searches, selects, opens, scrolls to the end, checks the complete final resource and footer, and exits with `q`. A separate 60 by 30 journey uses a long project and eight long roots to prove the bounded normal header, full scope top and end, search handling of `s`, and restoration of both inventory and detail. The single-root journey also exercises a live resize from 80 by 30 to 63 by 20 and back. A focused search journey covers a complete 1,800-character query, full-query filtering, wide Unicode, backspace, keep/cancel/clear transitions, and a 60 by 30 to 48 by 20 resize while the body, status, and controls remain visible.

Run the repository checks separately after the final changes:

```sh
/opt/homebrew/bin/gofmt -l cmd internal tests/acceptance
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go vet ./...
env GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go test -count=1 -mod=readonly ./...
python3 -m unittest discover -s tests/agent_loop -v
shellcheck tests/acceptance/run.sh
sh -n tests/acceptance/run.sh
git diff --check
```

The formatting check must print no files; use `gofmt -w` on any reported files and rerun. These are local checks, not a claim of green CI. The earlier repair verification passed executable acceptance, Go tests, vet, formatting, and all 20 scheduler tests; subsequent changes require fresh evidence.

## Runtime containment boundary

The Darwin acceptance harness runs the public CLI and browser under `sandbox-exec`. The profile denies network access, file writes, unrelated file-content reads, and child execution. It permits content reads only for the exact executable, the fresh fixture, `/System`, `/usr/lib`, `/dev`, and the literal `/` needed during process startup. Process execution is limited to the exact tested binary path.

Positive controls prove that the excluded canary, write target, loopback listener, and `/usr/bin/true` work without the profile. Inside the profile, allowed fixture content remains readable while the excluded content read, write, loopback connection, and child execution are denied. The fixture tree is byte-for-byte unchanged afterward.

This is content isolation rather than a complete macOS capability sandbox. The profile starts from `allow default`, so ambient filesystem metadata reads and other non-content operations remain allowed. Tightening those metadata operations caused the dynamic loader or process startup to fail before application code ran, so the acceptance claim is limited to the controls above.
