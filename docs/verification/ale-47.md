# Approved collection execution

The public workflow is `review prepare`, `review select`, explicit approval of
that exact `selection_id`, and `review apply --selection FILE`. Proposal and
selection remain read-only. Apply accepts only the complete approved contract-1
artifact and checks the whole sequence before writes. See the
[contract](../review-contract.md) for receipts, freshness and partial outcomes.

`tests/acceptance/ale47_review_apply_test.go` exercises the built executable in
disposable homes and projects under the shared containment policy. It covers:

- A chosen subset across Codex and Claude Code, omitted recommendations,
  protected independent same-name copies, no-op and empty selections.
- Two sources sharing one native settings file for each of Codex, Claude Code,
  Hermes, Pi, OpenCode and Gemini CLI, matching approved final byte hashes.
- Whole-selection zero-write refusal for absent approval, altered effects,
  external settings, same-byte file replacement, changed evidence, source
  identity and shared references; contradictory recommendations and a mixed
  supported/unsupported project proposal.
- A sandbox-denied later application write, retaining earlier commits, marking
  later items not attempted and returning final configured observations.
- Genuine external edits during a batch. The fixture controller holds the second
  native directory lock, waits for the first actual commit, edits configuration,
  source identity, supporting evidence or a reference, then releases the lock.
  There is no production failure switch. The native freshness check refuses
  before another availability write.
- A native configuration directory created during the first change, followed by
  another approved change in that file, with incomplete evidence disclosed.
- Project-only execution and human receipts with terminal-control escaping.

Run these cases alongside the proposal and grouped-control regressions with
`SKILLGLASS_TEST_RUN='TestALE(40|46|47)' tests/acceptance/run.sh ABSOLUTE_EVIDENCE`.
Use external temporary directories and the offline environment from the
[production verification guide](production-verification.md). Supporting Go
packages must use the proved contained launcher documented there. No installed
agent session is invoked and persisted configuration does not prove adoption.

`tools/review_scenario.py create --execution` prepares a separately sealed writer
surface for an actual companion walkthrough. The fresh evaluator receives the
emitted companion and neutral synthetic request, supplies its own assessment,
waits for a concrete subset choice and explicit approval, then applies and
reinspects. Keep controller smokes separate from actual model transcripts.
Deterministic tests and instruction checks do not prove agent behavior. The
coordinator reserves and records the actual model walkthrough separately.
