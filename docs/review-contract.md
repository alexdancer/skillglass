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

`collection.PrepareReview`, `SelectReview`, `ApproveReview`,
`ValidateReviewProposal` and `ValidateReviewSelection` are the shared Go interface
for the execution slice. A future executor must accept only an approved,
integrity-valid selection; preflight every chosen effect before writes; compare
original observations with live state; and track only expected preceding approved
writes. Selection effects do not authorize omitted recommendations. Independent
preview tokens are insufficient for shared-settings execution. A later
implementation may reuse the same native recipes and in-memory progression but
must retain external-edit, identity, discovery and effect freshness checks.
The current build has no collection execution command.

JSON files use schema version 1, strict known fields and unique object keys.
Malformed, altered, incompatible, ambiguous and stale requests refuse. The
[proposal reference](../internal/companion/skillglass-review/references/proposals.md#failures-coverage-and-exit-statuses)
owns failure, coverage and exit-status semantics. Human output identifies sources,
effects, unchanged and unresolved summaries, and coverage. It escapes terminal
controls.

## Representative external-agent evaluations

`tools/review_scenario.py` provides a disposable read-only CLI surface. It runs no
model. Create a fresh session for each independent evaluator with a built artifact
and the repository's built isolation probe:

```sh
python3 tools/review_scenario.py create --binary /absolute/skillglass --probe /absolute/isolationprobe --output /absolute/new-evidence-directory --case global-lean
```

Cases are `global-lean`, `global-broad`, `project-broad`, `missing-inputs` and
`incomplete-evidence`. The provider stages exact executable bytes outside the
fixture, creates synthetic homes/project/configuration, proves the shared
reader/writer/outside/symlink/network/execution controls, and records sealed
profiles and launch receipts. Product operations use the reader profile, with
native system inputs controlled absent. No installed native agent is invoked.
`session.json` and `prompt.txt` are evaluator inputs. `run` records raw output,
exit status and before/after manifests; `put` accepts JSON only into a fixture
artifact filename; `status` compares source/settings state with the initial tree.
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
