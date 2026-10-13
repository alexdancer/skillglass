# ALE-25 comparison verification

Compare selected independent bundles through the shared `Snapshot.Compare` operation, `skillglass compare [SCOPE] [--json] LEFT_SOURCE_ID RIGHT_SOURCE_ID`, or browser `m` followed by navigation and `v`. Source IDs remain directory identities. Same-source selections explain shared references; equal independent bytes never combine IDs.

## Behavior and limits

The comparison reads fresh raw manifests and regular supporting files, including malformed readable manifests. Sorted changes include added/removed resources, exact byte differences, kind and permission changes, and empty directories. Binary classification uses invalid UTF-8, NUL and unsafe controls; binary bytes never become diff text. Supporting links are compared as link entries without reading target bytes. Missing/cyclic targets, external targets, unreadable entries and special files make coverage incomplete. A linked manifest is explicitly read with target consistency evidence; an external linked manifest remains incomplete even if its observed bytes match. Source provenance and reference inventories remain in the CLI JSON envelope.

Limits: 10,000 entries and 64 MiB observed regular-file bytes per side, 8 MiB per file, 1,000 diff lines per changed file and 4,096 bytes per diff line. A read limit makes the outcome indeterminate. Diff presentation truncation does not affect completed exact byte comparison. Link text is bounded by the filesystem's link length. Regular reads use nonblocking, no-follow descriptor-relative traversal anchored from `/`; FIFOs, sockets and devices are never read or executed. Access times are excluded from consistency stamps because reading may update them. Directory membership, object identity, modes, sizes, modification/change times and byte hashes are observed across two passes, with per-open and per-read checks. This is best-effort stable observation, not an atomic snapshot; changes that restore all observable evidence between checks can evade detection.

Comparison exit status is 0 for completed equality or divergence, 1 for incomplete comparison or missing selectors, and 2 for syntax errors. Discovery scope coverage is shown independently in JSON and does not turn unrelated root failures into pair failures. Browser `c` continues filtering contexts. Marked source identity survives a refresh only while the same proven source remains reachable. Comparison completion carries a generation token, and a canceled or superseded request cannot replace the current view.

## Reproduce

```sh
GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/... -count=1
GOPROXY=off GOSUMDB=off go vet -mod=readonly ./...
./tests/acceptance/run.sh /tmp/ale25-evidence
python3 -m unittest discover -s tests/agent_loop -p 'test_*.py'
```

`tests/acceptance/run.sh` builds the actual executable and isolation probe, records the binary digest and candidate head, runs the complete inherited and new acceptance suite, and saves public command transcripts, terminal cells and unchanged-tree evidence. Tests require macOS, Go 1.27.1, tmux, and cached pinned dependencies. All product runs use the established isolated temporary home/project/environment harness and denied network policy.

## Acceptance evidence

- Public executable tests cover equal independent copies, divergent instructions, added/removed text, binary differences, mode/kind changes, empty directories, malformed raw manifests, shared references, independently shared manifests and divergent support.
- Incomplete coverage tests exercise external/broken/cyclic links, unreadable files, FIFOs, oversized resources and linked manifests. Output tests exercise escaped filenames, unknown selections, exact final-newline differences, bounded diff presentation and independent discovery-scope coverage.
- Before/after tree manifests include all fixtures, external targets and unrelated settings. Bundled executable scripts would create a sentinel if run; the sentinel remains absent.
- Real tmux tests at 48, 60, 80 and 120 columns select same-name independent rows, show marked source path and ID, compare content edited after discovery, scroll full differences, cancel, replace the selected directories outside scope, reject stale identity, and refresh the mark away. Captured cells enforce width and height constraints.
- Private deterministic read-observer tests induce equal-size rewrites, rename/add/remove, manifest link replacement, and selected-parent link substitution at a controlled traversal point. This narrow private seam covers filesystem timing that cannot be reliably induced via public executable commands. There is no public test flag or environment backdoor.

The implementation candidate and exact verification receipts are recorded by the agent loop under `.agent-loop/ale-25/build.md` and its unique `build-evidence/` directory. Independent Standards and Spec reviews and the no-mistakes publication pipeline are owned by the coordinator and are not claimed by this build record.
