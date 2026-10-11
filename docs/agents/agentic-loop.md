# Run the agentic coding loop

Use this workflow when the user asks for a parallel or autonomous coding run. The active Codex chat coordinates agents. `tools/agent_loop.py` persists scheduling state. The loop runs while that coordinator is active and resumes from its database after interruption. It is not a background daemon or a timer.

The user authorized local feature-branch commits and unattended ordinary review decisions on October 11, 2026. Pushes, PR publication, merges, and ticket closure still require authorization. Setting up this workflow does not start ALE-21 or its children.

## Start a run

Use this prompt, replacing the scope with the requested ticket or explicit set.

> Run docs/agents/agentic-loop.md for the selected scope. Use all available safe concurrency across tickets, ticket parts, tests, and reviews. Delegate ordinary implementation and review decisions. Create local feature-branch commits and drive no-mistakes fixes automatically. Stop before push, PR publication, merge, or ticket closure. Keep evidence and restart state. Do not expand the ticket scope.

1. Read [coding-workflow.md](coding-workflow.md), the live Linear tickets, comments, blockers, and assignees. Read the referenced Matt Pocock skills and [poteto-mode](/Users/alex/.agents/skills/poteto-mode/SKILL.md), including its cross-agent mapping. Use native Codex collaboration tools. Do not copy Cursor `Task`, `/loop`, model slugs, or cloud helpers.
2. Pin the starting commit with `git rev-parse HEAD`. The user's delegated decision authority lets the coordinator select this fixed review point without another question. Preserve the full user intent and later decisions in a file. Record the authorized scope, acceptance contracts, base SHA, and stop boundary.
3. Use `implement-spec` to build the live dependency graph. Write a plan following the [scheduler contract](../plans/agentic-loop-contract.md). The fixture under `tools/agent-loop/example-plan.json` is a scheduling demonstration, not the live backlog. Keep real plans, result files, and databases under ignored `.agent-loop/`.
4. Write the throughput checkpoint. Name the blocking setup, independent workstreams, shared state, and smallest safe split. Establish disputed interfaces before dependent implementations. Schedule real dependencies instead of relying on ticket number order.
5. Initialize the plan and local-commit policy. The examples assume the repository root as the working directory.

```sh
mkdir -p .agent-loop
python3 tools/agent_loop.py --db .agent-loop/run.sqlite init --plan .agent-loop/run.json --allow-local-commits
python3 tools/agent_loop.py --db .agent-loop/run.sqlite status
```

The plan is immutable. Repeating initialization with the same plan is safe. A scope change needs a newly identified plan and explicit reconciliation with existing agents and worktrees. Never recreate a database to forget an active writer.

## Dispatch independent work

Keep one coordinator as the only scheduler writer. A worker reports into its own artifact directory. Give every code-writing agent its own branch and worktree. Canonicalize resource paths before constructing the plan. The scheduler compares resource keys exactly and does not enforce filesystem permissions.

The available host capacity includes the coordinator. This session exposes four slots, so at most three native delegates fit. Recompute capacity in later sessions. Count nested agents and reserve the configured peak no-mistakes fan-out before admitting a gate job. Inspect its actual backend configuration rather than guessing. If the peak is unknown, hold that job while independent non-gate work proceeds. Do not promise that the scheduler limits subprocesses launched outside it.

Claim before launch, then bind the returned native agent identity.

```sh
python3 tools/agent_loop.py --db .agent-loop/run.sqlite claim --owner worker-1
python3 tools/agent_loop.py --db .agent-loop/run.sqlite bind --job JOB --token TOKEN --agent NATIVE_AGENT_ID
```

Use the assignment to call `collaboration.spawn_agent` or resume a known idle agent with `followup_task`. Pass pointers to the ticket, full intent, owned paths, exact base, skills, checks, and output directory. Include the commit and publication boundaries. Workers do not spawn extra agents unless the coordinator has reserved their capacity.

Repeat claims for distinct owners until no eligible job fits. Repeating an owner returns its existing lease. Capacity and conflicting read/write resources constrain admission. Independent read-only reviewers can share a frozen candidate. Writers cannot share it with a reviewer or another writer.

Split a ticket across independently verifiable contracts, such as a core behavior and a consumer of an already pinned interface. Give each part an owner and independent artifact. Run ticket-level acceptance after all parts are assembled and checked. Multiple branches needing a merge remain blocked at the merge boundary. Do not cherry-pick or patch-copy around that boundary. A successor may branch from one accepted predecessor's exact head only when all its required inputs are present and that stacked scope is authorized.

Cross-ticket dependencies point to an `accept` job. A part's passing result does not release the next ticket. Inspect the accepted artifact and actual ancestry before dispatching the successor. Linear closure is not required to record local progress, and local progress never silently changes Linear's status to Done.

## Build, review, and delegate decisions

Use the Matt Pocock routing in `coding-workflow.md` for prototype, production, bug diagnosis, module design, and behavior tests. ALE-22 remains a terminal prototype. It does not become production code because it passed a gate.

Use poteto principles deliberately and read each applied leaf skill.

- **Model the Domain** gives the run a task graph and explicit ownership states.
- **Separate Before Serializing Shared State** gives writers separate worktrees and serializes the one coordinator's dispatch ledger.
- **Make Operations Idempotent** keeps leases across restarts and rejects stale results.
- **Build the Lever** supplies the executable scheduler and repeatable checks.
- **Prove It Works** requires actual command results and artifact inspection.
- **Sequence Work into Verifiable Units** checks each candidate before releasing its dependents.

Use `architect` or `design-an-interface` for unresolved design choices. Use `swarm` for coverage partitions. Use `diagnose` and a fresh investigator for repeated failures. Read `principle-attack-the-premise` when two attempted fixes share the same failed assumption. Do not ask the user a question that an experiment can answer.

Freeze the candidate and run Matt Pocock `code-review` Standards and Spec axes in parallel. Apply the full baseline and keep the reports separate. Review comments using `no-comments` where the host supports its reviewer. Unsupported Cursor-only skills are recorded as unavailable, not silently treated as executed.

Give material findings to an independent decision agent with the exact candidate, full intent, and acceptance criteria. It returns `fix`, `dismiss`, or `blocked`, with evidence and the rule it applied. Use `finding-verification` for behavioral defect claims. A smell alone does not justify a rewrite. Delegate reversible choices within scope. The coordinator owns the decision and checks the evidence.

Schedule an explicit `work` job named `repair` after the read-only decision and before the gate. Give it write ownership of the ticket worktree and enough slots for its implementer and renewed Standards/Spec reviews. The example plan reserves three. The decision publishes findings and rationale as an artifact. It never edits the candidate.

Keep the repair lease active through each fix, test, frozen-candidate review, and decision iteration. Do not mark it passed until the current head satisfies the checks. With no accepted findings, verify that the candidate is unchanged and complete the repair job. A review job passing means its review report is accepted; it does not imply that the candidate has no findings. The repair and ticket acceptance checkpoints assess that result. This avoids reopening a passed build or creating unscheduled writers under an immutable plan.

After three iterations with the same failure, assign a fresh diagnostic agent within the reservation and reconsider the failed assumption. Continue when it produces a supported correction. Otherwise record a concrete blocker and preserve the candidate. After a pipeline starts, all fixes go through its AXI controller. Never give a second writer access to an active pipeline candidate.

## Drive no-mistakes without routine questions

Read [no-mistakes](/Users/alex/.agents/skills/no-mistakes/SKILL.md) and the installed CLI help. Commands were checked against v1.79.0. Help from the installed version controls which flags are available.

1. Inspect `git status`. Commit only scoped work on a feature branch. Verify the full commit SHA and clean candidate. Do not commit on the default branch.
2. Run `no-mistakes axi`, then inspect the current branch's active run and `branch_sync`. Initialize with `no-mistakes init` only if needed. Check `no-mistakes doctor` for the configured backend.
3. Start one outer controller per branch. Preserve the full intent verbatim and include the evidence commands. Pass it as one argument, using an argument array when writing software. Never interpolate intent into shell code.

```sh
no-mistakes axi run --yes --intent "FULL USER INTENT AND CONSTRAINTS" --skip=push,pr,ci --wait 30s
```

The explicit skips preserve this loop's publication boundary. `--yes` authorizes ordinary actionable findings, including ordinary `ask-user` findings, to be fixed automatically. It is not permission to change protected policy or broaden scope. A pipeline controller may ask a read-only decision agent to investigate and pass its guidance through `axi respond --action fix --findings ... --instructions ... --yes --wait 30s`.

Read every response. A bounded wait is not failure. Inspect status and reattach to the matching run. Do not start a second run because a process is quiet. Capture the raw output, run ID, submitted head, current head, and full intent digest. If strict receipts are used, reuse the recorded `--launch-nonce` and `--validation-generation` for that attempt according to installed help.

Only the outer AXI controller runs pipeline-control commands. An agent assigned a validation phase performs that phase and returns. `nested_gate_context` means return control immediately. No agent edits an active candidate directly, aborts to bypass a gate, or hand-rebases a live monitor.

Protected-path refusals and Test unvalidated-work refusals remain explicit exceptions under the installed CLI. Preserve the full finding, path, rule, and affected head. Delegate investigation and fix a real validation problem through the documented retry path when allowed. Do not waive a required operator decision. Leave that job blocked and continue independent work.

After a terminal outcome, follow the exact offered `branch_sync` action before another commit. Preserve all pipeline fix commits. Never improvise reset, stash, force, or branch replacement. For recovery that would discard unpublished work, preserve it and obtain the required authorization. Retry a failed terminal run only after correcting the cause and returning custody.

Report `passed`, explicit skips, automatic skips, and `checks-passed` accurately. A local run with push, PR, and CI skipped is local validation only. Empty CI results are not green. This workflow does not poll for a human merge. If a later authorized publication uses a live no-mistakes CI monitor, that monitor owns its conflict recovery.

## Accept results and resume safely

Inspect the actual diff, command output, full head SHA, and acceptance behavior before accepting a worker report. The scheduler stores the coordinator's evidence record; it cannot prove that an LLM judged correctly or that a claimed command ran. Use immutable output artifacts and link them in each evidence check. Do not replace behavior tests with searches through prompts or source code.

Write the evidence JSON from the [contract](../plans/agentic-loop-contract.md), then record it.

```sh
python3 tools/agent_loop.py --db .agent-loop/run.sqlite finish --job JOB --token TOKEN --evidence .agent-loop/results/JOB.json
python3 tools/agent_loop.py --db .agent-loop/run.sqlite evidence --job JOB
```

Drain every delegate result, recompute readiness, and refill available capacity. Keep a decision trail using `show-me-your-work` in `.agent-loop/decisions.tsv`. Log delegated choices, confirmed fixes, rejected findings, pipeline outcomes, and blockers. Include evidence paths and exact heads. Do not log credentials or unrelated transcripts.

On restart, inspect scheduler status, native agent status, Git worktrees, and AXI custody before dispatch. An unbound lease may represent a crash after spawn but before bind. Find the existing agent rather than launching a duplicate. Time alone never releases ownership.

After proving an owner has stopped and no pipeline retains custody, use `recover --owner-stopped --reason ...`, then `retry` if appropriate. The old token cannot complete the new attempt. Do not recover a job merely because its agent is slow.

The run is ready for handoff when every selected ticket has an accepted local candidate or an explicit blocker, and no unaccounted writer remains. Report tested behavior, Standards and Spec outcomes, every pipeline fix, skipped checks, local commit heads, worktree paths, and blocked dependencies. Stop at the authorized boundary. Use the `pr` skill to prepare a local PR body when useful, then publish only after authorization.
