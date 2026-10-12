# Agentic loop design

## Work plan

- [x] Read the Principles section of the poteto-mode skill.
- [x] Phase A: Frame.
- [x] Phase B: Design the workflow.
- [x] Phase C: Run the loop.
- [x] Phase D: Keep the audit trail.
- [ ] Phase E: Verify and hand back.

The deliverable is a reusable local coding loop for Skillglass. It schedules independent tickets and ticket parts, delegates implementation and judgment, and drives no-mistakes fixes without routine human decisions. Setting up the loop does not start the application backlog.

The completion predicate is an executable coordination mechanism with behavior tests, a documented native-agent execution contract, and verified no-mistakes command compatibility. A live model run is separate evidence from deterministic scheduling tests.

## Throughput checkpoint

- Blocking first steps. Resolve the commit boundary and pin a small execution contract before implementation.
- Independent workstreams. Compare two design candidates in parallel, then separate implementation, behavioral verification, and operating instructions.
- Shared mutable state. Each writer gets an isolated worktree. One coordinator owns dispatch and integration. The no-mistakes executor owns its active validation worktree.
- Smallest safe decomposition. Parallelize independently testable ticket parts with explicit contracts. Keep coupled edits with one owner. Count nested agents and validation agents against capacity.

## Grounding

At framing, the repository contained planning documents and a one-ticket coding workflow, with no application code or scheduler. The initial no-mistakes v1.79.0 inspection found a runnable Pi backend and a stopped daemon before pipeline initialization. These are historical observations, not current runtime state. Use the [operating guide](../agents/agentic-loop.md#drive-no-mistakes-without-routine-questions) for current CLI compatibility and custody checks.

The [operating guide](../agents/agentic-loop.md) owns the loop's authorization and delegation boundaries.

Codex exposes native collaboration tools. The pstack compatibility mapping forbids copying Cursor runtime commands or model names into Codex. Cursor-only helper tools are not available here.

## Selected design

Two independent design agents converged on a SQLite scheduler driven by a native Codex coordinator. A read-only judge scored both designs 8 of 10. Candidate A supplied the smaller interface. Candidate B contributed explicit capacity reservation and stale-attempt rejection. The comparison did not use different model families.

The alternative was a standalone daemon launching coding CLIs. That would need a separate authenticated worker protocol and supervision system. The selected version uses the native tools already exposed in Codex and states its lifetime limit. It resumes across chats, but it does not keep executing when the coordinator ends.

The [implementation contract](agentic-loop-contract.md) defines the data shape and public CLI. Code and tests were produced by separate agents in separate temporary directories. The coordinator inspected both before copying them into the repository. A review added exclusive active native-agent binding so one delegate cannot own two jobs at once.

## Verification record

The executable behavior suite passed 20 tests. It covers competing processes, capacity, resource exclusion, idempotent claims and results, stale leases, acceptance dependencies, immutable evidence, and authorization. See the [README](../../README.md#development-workflow) for the repeatable check command. Native Spec review found that the example needed an explicit repair writer between decision and validation. The revised graph gives that job exclusive ownership through fix, test, and renewed review.

The [implementation contract](agentic-loop-contract.md) owns scheduler guarantees; the [operating guide](../agents/agentic-loop.md#accept-results-and-resume-safely) owns coordinator acceptance checks. Review and live pipeline results are recorded in the local `.agent-loop/` decision trail.

The Cursor-only `control-cli`, `deslop`, and registered Comment Sicko runtime are unavailable under the installed cross-agent mapping. Verification uses the real Python CLI, independent native review, and no-mistakes. Those unavailable skill runs are not claimed as completed.
