# Offline Mac delivery verification

The [README](../../README.md#build-and-use) owns local Mac build, installation, update, and runtime instructions. This record owns delivered-artifact verification and its limits. The cached build uses `-mod=readonly -trimpath` with Go module network access disabled. This is a local unsigned build; public distribution, signing, notarization, and release hosting are unverified.

## Delivered artifact evidence

The acceptance runner accepts an absolute executable through `SKILLGLASS_BIN`. Without it, the runner retains its offline build behavior. TestMain copies supplied bytes to an external temporary directory and verifies equality. `artifact-identity.json` identifies both the delivered and staged files. Finalized product launches and contained tmux starts check the stable staged product path and digest independently of the mutable test dispatch target. Ordinary tmux utility queries do not check the product digest. The tmux payload launcher independently checks the finalized dispatched executable digest immediately before recording and launching its payload. Probe controls retain their own executable identity; they never substitute probe bytes for product identity. Invocation receipts retain actual executable paths, artifact hashes, environments, policies, and completions. The original installation pathname is not executed after staging.

Two fresh builds with the same source state, cached dependencies, flags, and Go toolchain are compared byte for byte. This establishes repeatability only within that tested environment. The coordinator's run report owns exact paths, digests, check results, terminal review, and remaining gate status. A pending review or suite is not delivery certification.

## Journey coverage

`TestALE42DeliveredCLIJourney` and `TestALE42DeliveredBrowserJourney` use independent disposable collections with one fixture shape. Public executable output and filesystem effects are the oracle. Native source IDs resolve canonical paths. The fixture contains one long shared source discovered by all seven adapters, independent same-name copies, managed plugin content, project/global precedence, unrelated settings, inert bundled code, external resources, activity logs, and credential canaries.

The CLI journey inventories, reads, and explains the shared source for all seven applications, compares independent instructions and resources, inspects project precedence, refuses managed control, applies and removes a supported project override, refuses Codex project scope without fallback, refuses a seven-member aggregate, then previews and applies a fresh six-writer selection. Reinspection retains Gemini's `ask` state after enable and CodeAF's independent state. Incomplete requested coverage refuses; incomplete unrequested coverage remains visible for an explicit subset. Deletion requires the exact target token and preserves independent copies and external targets.

The real-terminal journey runs at 120 by 30 and 48 by 28 cells. Current-grid guards drive search, comparison, instructions, Details, project disable/inherit, mixed aggregate refusal, fresh six-writer selection, managed refusal, deletion cancellation, exact typed confirmation, and observed exit. CLI reinspection checks browser writes. Captures retain long paths and meaningful transitions; frame existence alone is not visual approval. The run report records inspected frames after the last change.

Earlier ticket tests remain intact. The full acceptance suite must run against the same supplied artifact, including shared deletion, adapter controls, independent copies, stale previews, ownership limits, and isolation controls.

## Containment and support limits

Product payloads retain the existing credential-free environment whitelist, disposable paths, exact executable allowlists, denied runtime network, modeled absence of machine native inputs, denied outside content reads, and bounded mutation writes. Read-only launches deny writes. Separate controls verify those restrictions and reject invalid policies. Go module network settings are not runtime network isolation.

The integrated fixture adds stricter read denial for owned representative log and credential files. Before each denial, a contained probe establishes that the same owned file is readable under the base policy; under the stricter policy the read fails while an ordinary allowed read succeeds. Core journeys then succeed with all canaries denied, no canary text in captured output, and no bundled execution marker. This establishes non-ingestion for the tested paths and journeys, not universal filesystem tracing. A browser authorized for explicit mutations retains the bounded writer policy while its read/compare phases are checked for zero filesystem changes.

Fixture output does not establish current native-session enforcement. The pinned native records remain authoritative and retain their documented gaps:

| Application | Recorded support |
| --- | --- |
| Codex | [0.155.1 global controls](ale-27.md); project controls unsupported and inference-session project enforcement unverified. |
| Claude Code | [2.1.282 global/project controls](ale-30.md); ownership and activation restrictions remain visible. |
| Hermes | [0.20.0 active-profile global controls](ale-32.md); project controls unsupported; historical startup/catalog evidence has [explicit limits](ale-31.md). |
| Pi | [Version and global/project controls](ale-34.md); package/source provenance and reload limits remain scope-specific. |
| OpenCode | [Version and global/project controls](ale-36.md); native permission distinctions remain visible. |
| Gemini CLI | [0.63.0 global/project controls](ale-38.md); unioned denies and activation consent remain distinct. |
| CodeAF | [v0.7.1 with installed dirty revision](ale-39.md); browsing and precedence only, persistent per-skill control unsupported. |

The inherited [ALE-28 native resolver correction](../research/ale-28-native-fix-2026-10-18.md) records a custom Linux arm64 Codex 0.162.1 patch and its measured fresh-thread adoption. Stock 0.162.1 ignored project rules in that comparison. The custom build does not run natively on macOS, and its version string cannot enable Skillglass project controls. The stock Skillglass project refusal remains required until a supported distribution/platform and production writer are validated. ALE-42 does not rerun that native matrix.

Running-session adoption remains unknown for these fixture journeys. These journeys do not install or launch native agents, read live credentials, or modify installed skills.

## Reproduce the artifact check

Follow the [combined-suite prerequisites and containment procedure](ale-24.md#reproduce-the-acceptance-evidence). After dependency setup, build twice into fresh destinations and supply one artifact to the full acceptance runner:

```sh
export TMPDIR="$(mktemp -d /private/tmp/ale42-check.XXXXXX)"
export GOTMPDIR="$TMPDIR/go"
mkdir "$GOTMPDIR"
./tools/build-local.sh "$TMPDIR/install-a"
./tools/build-local.sh "$TMPDIR/install-b"
cmp "$TMPDIR/install-a/skillglass" "$TMPDIR/install-b/skillglass"
unset SKILLGLASS_TEST_RUN
SKILLGLASS_BIN="$TMPDIR/install-a/skillglass" ./tests/acceptance/run.sh /ABSOLUTE/FRESH/EVIDENCE
```

Then run the [repository checks](ale-24.md#reproduce-the-acceptance-evidence) with the separately proved contained collection-test launcher and inspect the terminal artifacts. Acceptance runs retain separate controller and product-payload receipts. Failed, skipped, blocked, or unrun checks belong in the run report.
