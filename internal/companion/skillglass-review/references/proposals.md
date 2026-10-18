# Review contract 1

`review prepare --draft FILE --json` reads an agent-authored JSON draft. All fields below are required except `project`, which is required only for project scope. `goal` accepts `broad`, `lean` or specific nonempty user preferences. Empty arrays explicitly express no keep-enabled constraints, recommendations, unchanged entries or unresolved questions.

```json
{
  "schema_version": 1,
  "context": {
    "applications": ["codex", "claude-code"],
    "scope": "global",
    "goal": "lean",
    "keep_enabled": []
  },
  "sources": [
    {
      "source_id": "EXACT_SOURCE_ID",
      "assessment": "How this source fits the user's goal, with observed evidence.",
      "supporting_paths": ["references/guide.md"],
      "limitations": []
    }
  ],
  "changes": [
    {
      "id": "change-1",
      "source_id": "EXACT_SOURCE_ID",
      "application": "codex",
      "action": "disable",
      "reason": "Evidence and goal supporting this recommendation."
    }
  ],
  "unchanged": [],
  "unresolved": ["Questions or evidence gaps that remain, if any."]
}
```

A project context also includes `"project": "/absolute/selected/project"` and `"scope": "project"`. Global context omits `project`. `keep_enabled` contains exact source IDs, protecting them for every selected application. `sources` assesses each distinct reviewed source once; cite exact bundle-relative supporting paths. Instructions are always read and pinned. The CLI includes actual read coverage, representations, diagnostics and content hashes. It cannot verify the agent's assessment or prove actual usage. Each unchanged entry has `source_id`, `application` and `reason`. Unaccounted source/application pairs remain explicitly unreviewed.

`changes` array order is execution order. IDs are unique opaque labels chosen by the author. Every change selects exactly one source, application and enable/disable action within the single context scope. Repeated or contradictory source/application changes, disabling keep-enabled sources, unknown identities and contradictory unchanged entries refuse before writes. A blocked native preview remains visible and cannot be approved. Revise recommendations explicitly if a supported subset is desired.

The proposal contains the normalized draft, selected-application discovery snapshots, reviewed sources/evidence, unchanged/unresolved summaries, unreviewed pairs and ordered native effects. Each effect has exact configuration destination, setting changes, no-write state and configuration byte hashes before/after preceding proposed effects. Baseline native preview observations are retained separately. Shared consumers include unselected applications as bounded context; they do not expand selected actions. No native single-command confirmation tokens are exposed. The proposal ID seals all context, evidence, order and effects. It is an integrity identifier, not proof of human consent or durable filesystem freshness.

Save the complete JSON stdout as `proposal.json`. Select the user's chosen IDs:

```sh
skillglass review select --proposal proposal.json --choose change-1 --json
```

Selection rechecks the complete original proposal against live observations, then calculates the chosen effects in proposal order. Save/show the complete selection preview. Its `omitted` IDs are explicitly unapproved. It has a separate `selection_id`, since removing an earlier recommendation can change later before/after effects.

After explicit user approval of that exact selection preview, repeat the same command with `--approve SELECTION_ID`. The returned `state: approved` records the caller's attestation of user approval; the CLI cannot authenticate who supplied it. Empty selection approves no actions. Neither prepare nor select writes source content or settings. Availability execution is a separate command in a later compatible build, which must validate the exact approved selection, preflight the whole selected batch and preserve freshness. Proposal/selection hashes are not native `--confirm` tokens.

## Failures, coverage and exit statuses

Unreadable input JSON files and malformed, duplicate-key, unknown-field, unsupported-version, altered or stale artifacts refuse safely. Missing or unreadable supporting evidence instead remains visible in the proposal's evidence diagnostics and incomplete selected-evidence coverage; it does not by itself block preparation. Discovery and selected evidence coverage remain separate; successful preparation is not a claim of complete review. Commands return one JSON object with `--json`, including failures.

- `0`: proposal prepared, subset previewed or approval recorded. Inspect coverage and limitations even on success.
- `1`: preparation returned a blocked native proposal, or selection/approval was refused, including altered or stale proposals, invalid chosen IDs and mismatched approval IDs.
- `2`: invalid usage, an unreadable or invalid draft/proposal JSON file, or a draft validation or preparation error.
