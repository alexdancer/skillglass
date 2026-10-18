# Inspection JSON contract, version 1

The ordinary CLI supports direct use by agents without the companion skill. Run
`skillglass help` for human help or `skillglass help --json` for machine help.
All options precede positional source IDs and resource paths. The first positional
operand or `--` ends option parsing; resource names such as `--json=false` are
literal paths and cannot change the selected output mode.

```sh
skillglass list --agent codex --project /path/to/project --json
skillglass read --agent codex --project /path/to/project --json SOURCE_ID
skillglass read-resource --agent codex --project /path/to/project --json SOURCE_ID references/nested/guide.md
skillglass explain --agent codex --project /path/to/project --json SOURCE_ID
```

Use the same explicit scope for discovery and subsequent reads. `list` includes
disabled sources by default. Copy exact `sources[].id` values, never select by
name. `source_path`, `identity`, `reference_ids`, and `references` preserve
provenance. The [README](../README.md#build-and-use) owns source identity and
discovery rules. If a selected source is absent or replaced, rediscover it,
review the replacement, and use its current ID.

## Compatibility

`list`, `problems`, `read`, `explain`, `read-resource`, `compare`, and machine help
return one UTF-8 JSON object on stdout when `--json` is selected, including usage
errors and failures before discovery. Human diagnostic text is not mixed into
machine stdout or stderr. An output-stream failure itself can prevent delivery
and is reported on stderr. Unknown commands with `--json` also return a JSON
usage error. Native mutation commands retain their existing preview/result
contracts; this document does not version their responses.

Each inspection object includes:

| Field | Meaning |
| --- | --- |
| `schema_version` | Integer `1`. Incompatible shape/meaning changes require another version. |
| `operation` | Requested inspection command. |
| `request.arguments` | Original arguments after the command, preserving the requested selection even before scope normalization succeeds. |
| `scope` | Normalized selected project, application and discovered roots; `null` if discovery did not start. |
| `outcome` | `success`, `partial`, or `failed`. |
| `exit_code` | Same status as the process. |
| `coverage.discovery` | `complete`, `incomplete`, or `not-started`, independently of filters and selected-file success. |
| `coverage.selected` | `complete`, `incomplete`, `failed`, or `not-applicable`. Covers only selected instructions/resource/comparison. |
| `failure` | On nonzero status: stable `code`, diagnostic `message`, and actionable `action`. |

Existing operation fields retain their shapes: list/problem `sources`,
`references`, `problems`, `filters`; read/explain `source`, `references`,
`problems` and legacy string `error` when selected source lookup fails; compare
`report` and left/right references. Resource reads add `resource_read` and retain
selected source/provenance and collection problems. `help` contains human usage
text in a JSON string for help and early usage errors.

Consumers must ignore unknown object fields, tolerate absent optional values
and `null` or empty inventory arrays, and treat unknown diagnostic/state values
as unsupported rather than successful evidence. Field order and whitespace are
not contractual. Metadata values preserve their existing JSON representations;
integers are not rounded by the inspection envelope. Source IDs are opaque and
must not be parsed. This is a local, fresh observation per invocation, not an
atomic filesystem snapshot or a durable approval token.

## Exit status and coverage

- `0`: requested inspection completed. Discovery is complete for list, problems,
  read, explain and read-resource. Compare retains its existing selected-pair
  status: it can return `0` with `outcome: partial` and incomplete unrelated
  discovery. Help succeeds without discovery.
- `1`: operational or coverage limitation. Useful partial results can still exist.
- `2`: invalid command, selection argument count, flag, filter, or scope request.

For example, a healthy selected file plus an unrelated missing discovery root
returns `1`, `outcome: partial`, `coverage.discovery: incomplete`, and
`coverage.selected: complete`, retaining the file text. A missing/stale source,
unreadable instructions, excluded resource or incomplete comparison returns
`outcome: failed`; inspect selection diagnostics. `list` and `problems` can return
useful sources with incomplete discovery. Neither filtered output nor status `0`
proves that an agent has read every resource, reviewed quality, or observed usage
in a running agent session. A malformed manifest may retain complete raw
instruction evidence while parsed metadata or overall discovery remains
incomplete. Inspect `manifest.status` and `problems` as well as coverage.

## Supporting evidence

`read` and `explain` reuse instruction reads, recursive resource metadata and
native configured-availability explanations. `availability.capabilities` and related
capability evidence identify supported applications/scopes and session limits;
use the ordinary enable/disable preview for exact supported effects. Availability
inference is separate from actual use by running agents.

`read-resource SOURCE_ID RELATIVE_PATH` explicitly reads one regular file in a
proven bundle directory. Copy a normalized, slash-separated `resources[].path`
from the inventory, or a path below an internal directory link. Absolute request
paths, `..`, redundant components, and `SKILL.md` refuse; use `read` for the
instructions. Standalone instruction sources have no implied sibling bundle and
refuse supporting-resource reads. Resource metadata is recursively inventoried
up to the existing 10,000-entry limit, without opening contents or traversing
links. An explicit known path may still be requested when inventory is incomplete;
that read cannot make discovery complete.

`resource_read` includes source ID, requested path, state (`read` or `failed`),
coverage, representation, size, bytes read, limit, resource metadata when present,
and structured diagnostics with code/message/action. It reads at most 1 MiB
(1,048,576 bytes), with a one-byte sentinel for detecting growth. Oversized,
interrupted, inaccessible, unstable, directory and special-file reads return no
partial content and representation `none`. Diagnostic codes identify the limit
or failure and explain the next action. Observed bytes are withheld on failed
reads; `bytes_read` is an observation count, not available evidence.

Complete regular-file bytes use `representation: utf-8` and `text` when valid
UTF-8 with no control characters other than tab/newline/carriage return. Other
bytes use `representation: base64` and exact `bytes_base64`. Binary bytes can be
retrieved without claiming their meaning was inspected. Human output escapes
terminal controls and preserves text lines, and directs binary consumers to JSON.

Resource links are resolved explicitly with a limit of 40 hops, using held
directory descriptors and no implicit operating-system symlink traversal.
Relative and absolute links provably within the canonical selected bundle are
supported. External link targets, including indirect escape chains, are not
opened. Broken, cyclic and unreadable links report coverage limitations. The
inventory retains raw `link_target` and lexical `link_scope`; link-scope metadata
alone cannot prove that an indirect link chain stays inside the bundle. Link
chains through external directory aliases, even if they eventually return to the
bundle, are excluded. File and traversed path observations are checked for
concurrent changes; this is best-effort stability evidence, not atomic isolation.

No inspection command executes instructions or scripts, installs skills, updates
caches, changes availability, or writes source contents/settings. Supporting
resource restrictions do not change existing native discovery or manifest-link
semantics. Disclose external, binary, bounded, inaccessible and unsupported
resources as limitations in a review rather than inventing review coverage.
