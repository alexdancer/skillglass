# Companion and collection review contract 1

The executable bundles `skillglass-review` 1.0.0 in `internal/companion`. Run
`skillglass companion --json` for its version, complete files and exact SHA-256
hashes, or `companion --file SKILL.md` for instructions. The bundle has no runtime
repository path dependency. Displaying it performs no installation. Explicit user-level
setup is documented in [companion setup](companion-setup.md); reads and review
commands never install the companion. The
[portable proposal reference](../internal/companion/skillglass-review/references/proposals.md)
owns the authoring format and approval steps. The [direct command reference](../internal/companion/skillglass-review/references/direct.md)
teaches ordinary reads and confirmed single-source controls.

`review prepare --draft FILE` is a read-only observation. An external agent
supplies assessment, explicit applications, global or one exact project, review
goal, keep-enabled source IDs, ordered recommendations, unchanged entries and
unresolved questions. The CLI supplies selected-application inventories, proven
source identity, exact cited instruction/resource byte hashes, per-read
limitations, bounded shared consumers and native previews. It assesses no quality
and observes no actual usage. Unaccounted source/application pairs stay unreviewed.
Discovery coverage and selected evidence coverage remain separate.

Each native preview is retained as `changes[].baseline` with its confirmation
removed. `observation_id` is a SHA-256 of the native freshness seal, so it cannot
be passed to native `--confirm`. The original validated recipe also provides a
pure settings transform. Those transforms compute `effects` in proposal order,
using the accumulated bytes for a shared settings file. Each effect retains its
intent, exact destination, per-setting before/after values, complete config byte
hashes and existence states. No destinations are written. Aliased destinations
and cross-application input interference refuse rather than guessing their
combined meaning. This introduces no native capability beyond existing previews.

`proposal_id` hashes the complete proposal, including normalized draft, exact
source/application/project context, selected evidence, discovery, consumer
provenance, order, baseline observations and prospective effects. Artifact
integrity is not proof of human consent. A blocked recommendation prevents
selection; explicitly revise the draft if the user wants a supported subset.

`review select --proposal FILE --choose ID ...` re-observes and compares the
entire proposal before previewing the chosen subset. It preserves proposal order,
recalculates settings effects with omitted recommendations removed, and seals
that result as `selection_id`. The output explicitly lists `chosen` and `omitted`.
An empty selection contains zero effects. After showing the exact selection and
receiving explicit user approval, the caller repeats it with `--approve
SELECTION_ID`. The CLI records the caller's attestation as `state: approved`.
Selection never changes availability, including when approval is recorded.

`review apply --selection FILE` consumes only an integrity-valid `state: approved`
selection. It reconstructs the entire original proposal and selected effects,
including omitted recommendations, before any availability write. A blocked or
stale selection refuses as a whole; a supported subset needs an explicit revised
proposal and approval. An empty selection applies no changes.

Execution guards the original native read set, reviewed evidence and structural
inventories for every catalog consumer. Each next native preview must match the
approved ordered effect exactly. It retains the native writer's immediate
rechecks under its held directory lock and before commit. Expected progression
advances configuration facts only to the exact staged file object installed by a
successful native write, with verified bytes and metadata. Exclusive native
directory creation may add only that held directory's identity to ancestor
chains. Every other file, identity, source, reference, evidence observation and
new native input remains subject to freshness checks. Independent stale native
tokens are never replayed and unrelated same-byte replacements still refuse.

The first unexpected non-success stops application. Completed writes remain;
later items are `not-attempted`. Machine receipts retain each source, application,
scope/project, native write state, persistence, configured achievement, and
reload/session limits. `final_configured` and `final_observation` separately
report final reinspection of successful, failed and unapplied items. A successful
native no-write result is `no-op`. Omitted recommendations stay unapproved.
There is no cross-file atomicity or automatic rollback. A continuation requires
fresh inspection and renewed approval. Exit 1 reports any unmet or unknown
request, including final reinspection failure; exit 2 reports invalid input.

`collection.ApplyReview` composes the established native controls; it adds no
application or scope capability. `PrepareReview`, `SelectReview`, `ApproveReview`,
`ValidateReviewProposal` and `ValidateReviewSelection` retain contract 1.

JSON files use schema version 1, strict known fields and unique object keys.
Malformed, altered, incompatible, ambiguous and stale requests refuse. The
[proposal reference](../internal/companion/skillglass-review/references/proposals.md#failures-coverage-and-exit-statuses)
owns failure, coverage and exit-status semantics. Human output identifies sources,
effects, unchanged and unresolved summaries, and coverage. It escapes terminal
controls.

## Representative external-agent evaluations

`tools/review_scenario.py` provides a disposable CLI surface, read-only by default. It runs no
model. Create a fresh session for each independent evaluator with a built artifact
and the repository's built isolation probe:

```sh
python3 tools/review_scenario.py create --binary /absolute/skillglass --probe /absolute/isolationprobe --output /absolute/new-evidence-directory --case global-lean
```

Cases are `global-lean`, `global-broad`, `project-broad`, `missing-inputs` and
`incomplete-evidence`. The provider stages exact executable bytes outside the
fixture, creates synthetic homes/project/configuration, proves the shared
reader/writer/outside/symlink/network/execution controls, and records sealed
profiles and launch receipts. The synthetic PATH exposes the exact immutable staged
`skillglass` executable for the companion's startup check. The surface accepts
only `setup --check`, `setup --check --json` and `setup --json --check`; setup
previews, installation and all other setup forms remain denied, including in
execution sessions. Startup checks and other read-only product operations use the
reader profile, with native system inputs controlled absent. No installed native
agent is invoked. `session.json` and `prompt.txt` are evaluator inputs. `run` records raw output,
exit status and before/after manifests; `put` accepts JSON only into a fixture
artifact filename; `status` compares source/settings state with the initial tree.
For an execution walkthrough, add `--execution` to `create`. That session permits
only `review apply` through its separately sealed writer profile. Prepare/select
and ordinary reads retain the reader profile and equal before/after requirement;
direct native confirmation stays outside this surface. Apply may change only the
fixture's explicit Codex/Claude settings paths. All other content remains guarded.
The result records actual `no_write` and both manifests. `status` reports the
initial/current manifests; an explicitly enabled execution session may differ.
Use distinct sessions for controller smokes and actual model evaluations.
The controller and an independent model evaluator need separately reserved slots.

Give a fresh evaluator the emitted companion, prompt, and tool surface without
expected recommendations. Retain its actual questions, responses and tool choices
with model/version, synthetic follow-up/approval messages, receipts and final
filesystem results. Evaluate whether it asks only missing questions, treats broad
and lean goals differently when warranted, includes disabled sources, protects
keep-enabled identities, preserves shared contexts and independent same-name
copies, discloses incomplete evidence, and excludes omitted recommendations.
The subset approval follow-up must identify a concrete chosen subset from its
actual proposal. These are behavioral evaluations; deterministic acceptance or
searches through instruction strings cannot substitute for them.

## Installed-companion delivery evaluations

For the delivered Mac journey, use `create --installed --execution` as documented
in [delivered workflow verification](verification/ale-49.md). This is explicit
controller setup for Codex and Claude Code with separate OpenCode/CodeAF shared
acknowledgement before model dispatch. It preserves exact installed bundle files
and setup manifests separately from model calls. `context.txt` contains those
actual installed bytes and the tool surface; `request.txt` contains the neutral
synthetic request. `--case missing-inputs --incomplete-evidence` independently
combines missing user inputs and evidence limitations. No installation command
is added to the evaluator's narrow `run` surface. Every installed companion copy
remains a distinct inventory source; synthetic keep-enabled inputs should
protect both copies when their continued availability is required.
