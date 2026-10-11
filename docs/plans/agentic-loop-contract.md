# Agentic loop implementation contract

This contract defines the first local scheduler. It does not launch models, run Git mutations, or interpret no-mistakes TOON. The native coordinator performs those actions under the operating instructions. The scheduler makes dispatch, resource ownership, capacity, and restart behavior testable.

## Public interface

Use Python 3.10 or later and the standard library. The entry point is `tools/agent_loop.py`. Every command accepts `--db PATH`. Output is one JSON object. Errors use exit 2 and a JSON error on stdout. Successful commands use exit 0.

- `init --plan PATH [--allow-local-commits]`. Create a database from a validated immutable plan. Repeating the exact normalized plan and authorization is a no-op. Refuse a different plan or authorization at the same database.
- `claim --owner NAME`. Atomically reserve one runnable job. Repeating an owner returns its existing active lease. If nothing fits, return `assignment: null` and status. A lease has a random token and increasing job attempt number. Record reservation before native launch.
- `bind --job ID --token TOKEN --agent ID`. Record the actual native agent ID. Repeating the same binding is a no-op. Refuse replacement of a bound agent or binding one native identity to two active jobs. Reuse after termination is allowed.
- `finish --job ID --token TOKEN --evidence PATH`. Accept a structured result only for the active lease. Store the exact evidence bytes and SHA-256 in SQLite. Repeating identical evidence is a no-op. Reject a changed duplicate or stale token.
- `recover --job ID --token TOKEN --owner-stopped --reason TEXT`. Mark an abandoned reservation blocked after the coordinator has checked native and pipeline ownership. Require the explicit flag and nonempty reason. Never expire leases automatically.
- `retry --job ID`. Return a blocked job to pending. A passed job cannot be retried. The next claim increments its attempt and gets a new token.
- `status`. Return the immutable run policy, current assignments, ready jobs, blocked jobs, dependency waits, and capacity use. No mutations.
- `evidence --job ID`. Return the stored evidence document and digest for the latest accepted result, including a blocked result. Never reread the source file as the authoritative evidence.

`--db` is a global argument before the subcommand. Never read an absent database by creating an empty file. Use explicit SQLite transactions for all mutations. Multiple processes claiming against one database must not duplicate work or overbook capacity. Store an append-only events table in the same transactions. Set a bounded busy timeout.

## Plan shape

The plan is JSON with exactly these fields. Reject unknown fields and wrong types, including booleans used as integers.

```json
{
  "version": 1,
  "run_id": "example",
  "intent": "The full authorized objective and constraints.",
  "base_sha": "e1af9474d6ca77e7139928865a16d32b96113954",
  "capacity": 3,
  "jobs": [
    {
      "id": "ticket-a-build",
      "ticket": "EXAMPLE-A",
      "role": "work",
      "depends_on": [],
      "slots": 1,
      "resources": {"worktree:/absolute/isolated/path": "write"},
      "instructions": "Implement the authorized contract and report evidence.",
      "context": ["docs/agents/agentic-loop.md"]
    }
  ]
}
```

Roles are `work`, `review`, `decision`, `gate`, and `accept`. IDs and resource keys are nonempty strings. Resource identity is exact-string identity. The coordinator must canonicalize real paths and use the same key for the same resource. Modes are `read` and `write`. Read/read can overlap. Every other same-key combination conflicts. A job's slots must fit the positive capacity. Gate jobs require local-commit authorization and cannot claim without it. Every job has at least one resource. The graph must have unique IDs, known dependencies, no duplicate dependencies, and no cycles. Preserve plan order as the deterministic dispatch preference.

Only `accept` jobs may satisfy a dependency from a different ticket. This forces an explicit ticket acceptance checkpoint rather than treating a partial implementation as a ticket's completion. Acceptance must depend on at least one same-ticket job. `passed` means that the coordinator accepted the job's evidence. It does not mean a Git merge, ticket closure, or remote publication occurred.

Authorization is persisted separately from the plan. The only supported authorization flag enables local commits. Merge, push, PR publication, and ticket closure are always false in this version. Instructions are context, not authorization.

## Evidence shape

The evidence JSON contains exactly `job`, `token`, `attempt`, `base_sha`, `head_sha`, `verdict`, `summary`, and `checks`. Reject unknown fields and wrong types. `base_sha` must equal the pinned run base. Both SHAs are 40 lowercase hexadecimal characters. The scheduler validates their form, not Git ancestry or review quality. The coordinator must check actual Git identity and inspect artifacts before submitting results.

Verdicts are `passed` and `blocked`. A passed result requires at least one check, all with exit code 0. A check has exactly `command`, `exit_code`, and `evidence`, with nonempty command and evidence strings. A blocked result may have no checks or failed checks. A summary is nonempty. The job, token, and attempt must match the active assignment.

Persist evidence up to 1 MiB. Resolve a file once and retain the bytes. A subsequent external edit must not change the accepted record. Keep previous attempts and events for auditing. The CLI must not execute any plan instruction or evidence command string.

## Behavioral verification

Exercise the executable through subprocesses and temporary databases. Cover malformed plans, cycles, missing dependencies, cross-ticket acceptance, capacity, conflicting writers, compatible readers, duplicate owners, competing claim processes, duplicate binding, immutable evidence, changed duplicates, stale tokens after recovery and retry, no lease expiry, missing database behavior, and the local-commit gate boundary. Tests assert emitted JSON and resulting scheduling behavior, not source strings.

The initial acceptance proof uses fixtures and no live models. A native agent smoke run and a real no-mistakes pipeline are separate checks. Never describe those as passed based on scheduler tests.
