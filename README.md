# Skillglass

A clear view into your local agent skills.

Skillglass is a Go terminal tool for browsing and reading local agent skills. The planned product will also compare and control skills across agent applications, explain where a skill comes from, and show what a proposed change would affect.

## Status

The production read-only inventory and identity slices are implemented. Skillglass discovers explicit custom and project roots, groups references only when they resolve to the same proven directory object, reports collection problems without discarding healthy sources, and exposes the same immutable snapshot through the CLI and keyboard browser. Ownership and availability remain visibly unknown until later integrations can support evidence-based answers. The repository also contains the approved specification, ticket breakdown, local agent scheduler, and the [ALE-22 terminal prototype](prototypes/ale-22/README.md) that preceded the production implementation.

The project was planned as **Skill Viewer**. Existing Linear titles and historical planning documents retain that name; **Skillglass** is the product and repository name.

## Planned capabilities

- Explain availability by agent application and project, including precedence and unsupported controls.
- Compare independent copies while distinguishing them from shared references.
- Preview and apply validated enable/disable controls, or permanently delete explicitly selected content after confirmation.
- Work offline, with the first release targeting macOS.

Planned integrations cover Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF. Capabilities must be validated per application, version, and scope. CodeAF browsing with explicit persistent-control limitations is an accepted first-release exception.

## Start here

- [Specification](docs/specs/skill-viewer-v1.md)
- [Published ticket breakdown](docs/plans/skill-viewer-v1-ticket-breakdown.md)
- [Coding workflow](docs/agents/coding-workflow.md)
- [Parallel agentic loop](docs/agents/agentic-loop.md)
- [Domain glossary](GLOSSARY.md)
- [Agent instructions](AGENTS.md)
- [ALE-23 verification](docs/verification/ale-23.md)
- [ALE-24 verification](docs/verification/ale-24.md)

[Linear](https://linear.app/alex-dancer/project/skill-viewer-58706f175bed) holds live ticket requirements, status, and dependencies. The local copies are references.

## Build and use

Build on macOS with Go 1.27.1. Download the pinned dependencies once with network access; the product runs offline:

```sh
/opt/homebrew/bin/go mod download
/opt/homebrew/bin/go build -mod=readonly -trimpath -o bin/skillglass ./cmd/skillglass
```

Every operation requires an explicit discovery root; help does not. Repeat `--root` for custom locations. Use `--project-root` for roots associated with a selected `--project`; using a project root without `--project` is a usage error. Project context never selects a root implicitly. A root containing `SKILL.md` selects that bundle; otherwise only direct child bundle candidates are inspected, not nested collections. Explicit directory and manifest symlinks may resolve outside the root, but their siblings are not scanned.

```sh
bin/skillglass list --root /path/to/skills
bin/skillglass list --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills --context project --problem duplicate-name --search testing --json
bin/skillglass problems --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills --search broken --json
bin/skillglass read --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills SOURCE_ID
bin/skillglass browse --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills
```

`list` and `problems` accept `--source-root`, `--context all|project|custom`, and `--problem CODE`; all supplied filters combine with AND. The source-root value must be one of the selected normalized absolute roots and never expands discovery. `list --search` matches source names and descriptions. `problems --search` matches problem codes, messages, and original discovery paths. Filters change the view of the selected snapshot, not its completeness or exit status. Each CLI invocation performs a fresh scan.

Use a source ID from `list` as the final `read` argument, after all scope and JSON flags. `read` deliberately has no view filters and returns the selected source with every known discovery reference. Supporting resources are inventoried recursively inside each bundle without following resource symlinks or opening resource contents. Browsing and reading never execute bundled instructions or scripts or change sources or agent settings. Human output escapes terminal controls; JSON preserves the text for programmatic use.

Directory symlinks that reach the same proven directory object produce one source with several references. Equal names, equal bytes, and a shared `SKILL.md` target do not prove shared bundle identity, so those directories remain independently selectable. Every reference retains its custom or project context, original discovery path, resolution status, and root; successfully resolved references also carry the resolved path and source ID. Broken and cyclic references stay available without a fabricated source. Filtering shows a source when at least one reference matches while detail keeps its complete reference inventory. JSON exposes the active `filters`, root contexts in `scope.roots`, reference contexts and `matches_filters`, and each problem's related root, reference, and source IDs.

The documented problem codes are `root-missing`, `root-unreadable`, `broken-reference`, `symlink-cycle`, `unreadable-manifest`, `malformed-manifest`, `metadata-non-finite-float`, `manifest-too-large`, `manifest-invalid-encoding`, `unreadable-resource`, `resource-limit`, `identity-uncertain`, `duplicate-name`, and `shared-manifest`. Duplicate names, shared manifests, and non-finite metadata are informational and do not make otherwise complete coverage incomplete. External resource links remain visible through their resource path, target, and `link_scope: external`; they are not traversed. Failures that prevent selected content from being discovered or read preserve useful partial output, mark coverage incomplete, and return status 1.

In the browser, move with the arrow keys or `j`/`k`, press `/` to search the current Sources or Problems view, `Enter` to keep a search or open the focused item, `Tab` to switch views, `Esc` or the left arrow to return from detail, and `q` to quit outside search. Search supports bracketed paste as query data and `Ctrl+U` to clear. Source and problem detail scroll with the same movement keys, `PgUp`/`PgDn`, and `g`/`G` for top/end.

Press `c` to cycle all, project, and custom reference contexts; `o` to cycle selected roots; `f` to cycle problem codes; and `r` to refresh explicitly. The browser has no watcher. Refresh observes external edits and retains a selected source only while the same proven directory object remains reachable. A rename or one retargeted alias can retain selection when another reference still reaches that object. Replacement, loss, uncertain identity, or retargeting its final reference clears selection visibly. A selected source hidden by filters remains explicitly identified as hidden and is never rebound to a visible row.

Press `s` from the inventory or source detail to inspect the full selected project and every discovery root. The scope view scrolls with the arrow keys or `j`/`k`, `PgUp`/`PgDn`, and `g`/`G`; press `Esc` or the left arrow to return. While search input is active, `s` remains ordinary query text. Long searches retain the complete query for filtering and show its current tail on one row with a leading ellipsis.

Manifests require YAML mapping frontmatter with a non-empty string `name` and an optional string `description`. Mapping keys must decode to unique UTF-8 strings within each mapping; non-string keys and invalid decoded strings are rejected. Ordinary aliases and merge overrides remain supported. A malformed readable UTF-8 manifest stays selectable with exact `manifest.raw_text` and diagnostics, without fabricated valid metadata. Manifest reads are limited to 4 MiB and resource inventories to 10,000 entries; exceeding either bound reports incomplete coverage rather than silent truncation.

JSON output preserves YAML non-finite floats with an explicit, portable representation instead of failing serialization:

```json
{"kind":"non-finite-float","value":"nan"}
{"kind":"non-finite-float","value":"positive-infinity"}
{"kind":"non-finite-float","value":"negative-infinity"}
```

Each affected manifest also reports one `metadata-non-finite-float` diagnostic. The diagnostic does not limit discovery coverage, so an otherwise complete request still exits with status 0.

A manifest that is not valid UTF-8 reports `manifest-invalid-encoding`, remains selectable with its paths and resources, and makes coverage incomplete with exit status 1. JSON preserves the complete file as standard padded base64 in `manifest.raw_bytes_base64` instead of replacing invalid bytes. Content-derived text and metadata fields are omitted for that manifest. Human output and browser detail show a recovery hint directing you to `list` or `read` with `--json`; they do not display an altered body. Valid UTF-8 output is unchanged, including a literal U+FFFD character.

Exit status 0 means the requested result is complete, including an empty collection and help. Status 1 means incomplete coverage, an operational failure, or an unknown selection; useful partial output is preserved when available. A browser opened with incomplete coverage also returns 1 on normal quit. Status 2 reports command usage errors.

## Development workflow

Follow the Matt Pocock skill sequence in the [coding workflow](docs/agents/coding-workflow.md). Implement authorized work, verify the actual result, and review Standards and Spec independently. For parallel tickets and ticket parts, delegated decisions, and automatic no-mistakes fixes, follow the [agentic-loop operating guide](docs/agents/agentic-loop.md), including its approval boundaries.

The active Codex chat drives the loop through native agents. The scheduler persists leases and evidence across interruptions. It does not launch models or keep running after the coordinating chat ends.

For scheduler checks, executable acceptance, terminal evidence, static checks, and containment limits, follow the [ALE-24 verification guide](docs/verification/ale-24.md#reproduce-the-acceptance-evidence).

Skill links in the workflow point to the maintainer's installed local skills; those external skill files are not bundled in this repository.

## Planning records

The `.scratch/skill-viewer/` documents preserve earlier discovery and decision history. Their historical open decisions and status text are not the current implementation queue; the published Linear specification and tickets take precedence.

Downloaded toolchains under `.tools/` and local review artifacts under `.lavish/` are excluded from version control. Links to those artifacts in historical notes refer to the original local workspace.
