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

The repository contains planning documents and a one-ticket coding workflow. It has no application code or existing scheduler. The installed no-mistakes v1.79.0 supports `axi run --yes`, `--base-branch`, bounded waits, and explicit step skips. It validates committed feature-branch history. Its doctor reports a runnable Pi agent and a stopped daemon. This repository has not been initialized for no-mistakes.

The user authorized local commits for the loop after confirming the no-mistakes prerequisite. The new request also authorizes unattended ordinary review decisions and fixes. Publication, merge, and closure remain separate.

Codex exposes native collaboration tools. The pstack compatibility mapping forbids copying Cursor runtime commands or model names into Codex. Cursor-only helper tools are not available here.

## Selected design

Two independent design agents converged on a SQLite scheduler driven by a native Codex coordinator. A read-only judge scored both designs 8 of 10. Candidate A supplied the smaller interface. Candidate B contributed explicit capacity reservation and stale-attempt rejection. The comparison did not use different model families.

The alternative was a standalone daemon launching coding CLIs. That would need a separate authenticated worker protocol and supervision system. The selected version uses the native tools already exposed in Codex and states its lifetime limit. It resumes across chats, but it does not keep executing when the coordinator ends.

The [implementation contract](agentic-loop-contract.md) defines the data shape and public CLI. Code and tests were produced by separate agents in separate temporary directories. The coordinator inspected both before copying them into the repository. A review added exclusive active native-agent binding so one delegate cannot own two jobs at once.

## Verification record

The executable behavior suite passed 19 tests. It covers competing processes, capacity, resource exclusion, idempotent claims and results, stale leases, acceptance dependencies, immutable evidence, and authorization. Run `python3 -m unittest discover -s tests/agent_loop -v` to repeat it.

The scheduler does not validate the truth of an agent's evidence, Git ancestry, filesystem isolation, or no-mistakes outcomes. The coordinator owns those checks. The operating guide distinguishes them from scheduler guarantees. Review and live pipeline results are recorded in the local `.agent-loop/` decision trail.

The Cursor-only `control-cli`, `deslop`, and registered Comment Sicko runtime are unavailable under the installed cross-agent mapping. Verification uses the real Python CLI, independent native review, and no-mistakes. Those unavailable skill runs are not claimed as completed.
