# Skillglass

A clear view into your local agent skills.

Skillglass is a Go terminal tool for browsing, reading, comparing, and controlling local agent skills. Control support varies by application and scope.

## Status

The production inventory, identity, comparison, and application discovery slices are implemented. Skillglass discovers explicit custom and project roots, groups references only when they resolve to the same proven source object, reports collection problems without discarding healthy sources, and exposes the same immutable snapshot through the CLI and keyboard browser. Codex, Claude Code, Pi, OpenCode, CodeAF, Gemini CLI, and Hermes integrations add native scope and local configured-availability explanations. Validated global Codex, global/project Claude Code, active-profile global Hermes and global/project Pi controls use source-bound preview and explicit confirmation. Other write controls remain unsupported. Pi cached settings require reload; running-session execution remains unobserved. Generic explicit-root inventory keeps unknown ownership and availability.

The project was planned as **Skill Viewer**. Existing Linear titles and historical planning documents retain that name; **Skillglass** is the product and repository name.

## Planned capabilities

- Extend version-specific availability evidence and remaining validation coverage.
- Extend validated enable/disable controls to remaining agents, and add permanent deletion of explicitly selected content after confirmation.
- Work offline, with the first release targeting macOS.

Native integrations cover Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF. Capabilities must be validated per application, version, and scope. CodeAF browsing with explicit persistent-control limitations is an accepted first-release exception.

## Start here

- [Product principles](PRODUCT.md)
- [Terminal design](DESIGN.md)
- [Specification](docs/specs/skill-viewer-v1.md)
- [Published ticket breakdown](docs/plans/skill-viewer-v1-ticket-breakdown.md)
- [Coding workflow](docs/agents/coding-workflow.md)
- [Parallel agentic loop](docs/agents/agentic-loop.md)
- [Domain glossary](GLOSSARY.md)
- [Agent instructions](AGENTS.md)
- [ALE-23 verification](docs/verification/ale-23.md)
- [ALE-24 verification](docs/verification/ale-24.md)
- [ALE-25 verification](docs/verification/ale-25.md)
- [ALE-26 Codex verification](docs/verification/ale-26.md)
- [ALE-27 global Codex control verification](docs/verification/ale-27.md)
- [ALE-30 Claude Code control verification](docs/verification/ale-30.md)
- [ALE-29 Claude Code verification](docs/verification/ale-29.md)
- [ALE-34 Pi control verification](docs/verification/ale-34.md)
- [ALE-43 verification](docs/verification/ale-43.md)

[Linear](https://linear.app/alex-dancer/project/skill-viewer-58706f175bed) holds live ticket requirements, status, and dependencies. The local copies are references.

## Build and use

Build on macOS with Go 1.27.1. Download the pinned dependencies once with network access; the product runs offline:

```sh
/opt/homebrew/bin/go mod download
/opt/homebrew/bin/go build -mod=readonly -trimpath -o bin/skillglass ./cmd/skillglass
```

Generic inventory operations require an explicit discovery root; help does not. Select native discovery with `--agent codex|claude-code|pi|opencode|codeaf|gemini|hermes`. The version-specific scope and evidence contracts are documented for [Codex](docs/verification/ale-26.md), [Claude Code](docs/verification/ale-29.md), [Pi](docs/verification/ale-33.md), [OpenCode](docs/verification/ale-35.md), [CodeAF](docs/verification/ale-39.md), [Gemini CLI](docs/verification/ale-37.md), and [Hermes](docs/verification/ale-31.md). Repeat `--root` for custom locations. Use `--project-root` for roots associated with a selected `--project`; using a project root without `--project` is a usage error. Without `--agent`, project context never selects a root implicitly. Explicit roots containing `SKILL.md` select that bundle; otherwise only direct child bundle candidates are inspected, not nested collections. Explicit directory and manifest symlinks may resolve outside the root, but their siblings are not scanned.

```sh
bin/skillglass list --agent claude-code --project /path/to/repository --availability explicit-only --json
bin/skillglass explain --agent claude-code --project /path/to/repository SOURCE_ID
bin/skillglass browse --agent claude-code --project /path/to/repository
bin/skillglass list --agent codex --project /path/to/repository --availability disabled --json
bin/skillglass explain --agent codex --project /path/to/repository SOURCE_ID
bin/skillglass browse --agent codex --project /path/to/repository
bin/skillglass list --root /path/to/skills
bin/skillglass list --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills --context project --problem duplicate-name --search testing --json
bin/skillglass problems --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills --search broken --json
bin/skillglass read --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills SOURCE_ID
bin/skillglass compare --root /path/to/custom --json LEFT_SOURCE_ID RIGHT_SOURCE_ID
bin/skillglass browse --root /path/to/custom --project /path/to/project --project-root /path/to/project-skills --theme auto
```

To change one source's global Codex choice, obtain its ID from `list`, inspect a preview, then repeat the exact request with its confirmation token:

```sh
bin/skillglass disable --agent codex --scope global --json SOURCE_ID
bin/skillglass disable --agent codex --scope global --confirm c1:PREVIEW_TOKEN SOURCE_ID
bin/skillglass enable --agent codex --scope global SOURCE_ID
```

Preview changes no settings. Add `--project /path/to/repository` to inspect the effective result in that project while explicitly saving a global choice. See [global Codex control behavior and limits](docs/verification/ale-27.md) for contextual blockers, persistence outcomes and unsupported scopes.

In Codex browser source detail, press `D` to disable or `e` to enable, then `g` to select global scope. Press `p` instead to inspect the versioned unsupported project-control explanation, or `i` to request removal of a project override and inheritance. Both project requests refuse without changing settings on the validated Codex version. Inspect the paged preview. Press `y` to apply a ready global choice, or Enter or Escape to cancel. Long paths remain accessible with arrows, Page Up/Down and Home/End; on the scope selection page, `g` selects global scope rather than scrolling.

For a unique ordinary Claude Code source, `--agent claude-code` supports global and exact selected-project enable/disable plus project inheritance:

```sh
bin/skillglass disable --agent claude-code --scope global --json SOURCE_ID
bin/skillglass enable --agent claude-code --scope project --project /path/to/repository SOURCE_ID
bin/skillglass inherit --agent claude-code --scope project --project /path/to/repository SOURCE_ID
```

Repeat the exact preview request with `--confirm cl1:PREVIEW_TOKEN` to apply. In the Claude Code browser, `D`/`e` opens scope selection, `g`/`p` selects global/project, and `i` removes the project override. Inspect the preview, then `y` confirms; Enter or Escape cancels. Inheritance can succeed while the inherited result remains disabled. Higher-priority project/local choices refuse broader on/off changes. Controls require the command name to match every native discovery directory name; renamed skills, plugins and independent command-name or directory-selector collisions cannot be toggled as isolated ordinary skills. [Claude control evidence and limits](docs/verification/ale-30.md) records versions, filesystem guarantees and session loading limits.

Hermes controls use `--agent hermes --scope global` with the same preview and `--confirm` token workflow. Global selects the active profile shown in the preview. In the Hermes reader, `D` and `e` open the same scope/confirmation pages. Only that profile's `skills.disabled` membership changes; platform denies and other profiles remain intact. Project enable/disable and `inherit` are unsupported on Hermes 0.20.0 and never fall back to global. An enable saved while platform policy still denies use reports the unmet effective result and exits nonzero. Running-session adoption and reload/restart requirements remain unknown. See [Hermes control validation and limits](docs/verification/ale-32.md).

Pi 1.1.0 ordinary sources support global enable/disable and trusted selected-project overrides through exact lexical resource settings:

```sh
bin/skillglass disable --agent pi --scope global --json SOURCE_ID
bin/skillglass enable --agent pi --scope project --project /path/to/repository SOURCE_ID
bin/skillglass inherit --agent pi --scope project --project /path/to/repository SOURCE_ID
```

Inspect the preview and repeat that exact request with `--confirm pi1:PREVIEW_TOKEN` to apply. In the Pi browser, `D`/`e` opens scope selection, `g`/`p` selects global/project, `i` selects project inheritance, `y` confirms and Enter or Escape cancels. A project override of inherited content uses an exact instruction-file registration plus its exact choice; inherit explicitly removes that native pair and preserves lone user resource registrations. Equivalent relative, tilde and directory registrations of the selected inherited source make new reversible overrides ambiguous. A pair mixed with retained selected-source discovery refuses inheritance without deleting user resources. Existing roots, exclusions, package settings, skill contents and other applications remain intact. Managed/package controls, ambiguous resources and unverified scopes are unsupported, with no broader fallback. Capability reporting describes native writer eligibility; explicit previews additionally verify effective results and independent-source effects. A higher-priority choice or independent name winner can refuse the request. Existing cached Pi settings require reload; fresh sessions load saved choices. Interactive `/reload` adoption and model execution remain unverified. See [Pi control evidence and limits](docs/verification/ale-34.md).

`bin/skillglass inherit --agent codex --scope project --project /path/to/repository SOURCE_ID` requests removal of the selected source's project override and restoration of applicable inheritance. Project enable, disable and inheritance requests return `unsupported` without a confirmation token or filesystem changes on Codex 0.155.1. They never fall back to a global change. Global `inherit` is also unsupported because there is no project override to remove at that scope. See [project control evidence and limits](docs/verification/ale-28.md) for the native limitation and the distinction between local configuration inference and session enforcement.

`list` and `problems` accept `--source-root`, `--context all|project|custom|global|system|managed`, `--availability STATE`, and `--problem CODE`; `browse` accepts the same filters for its initial view, with search entered interactively. Use `bin/skillglass help` for supported availability values; the default is all. All supplied filters combine with AND. The source-root value must be one of the selected normalized absolute roots and never expands discovery. Search is case-insensitive. `list --search` matches source names and descriptions. `problems --search` matches problem codes, summaries, details, reported diagnostic paths, and all associated original discovery and requested-root paths. Context and source-root must match the same associated reference or root, but search may match any known provenance of that eligible problem. Availability filters match a source's overall configured state, not an individual reference's participation. Source-associated problems require a related source with that state and a reference matching the context/root filters; diagnostics without an associated source remain visible under availability filtering. Filters change the view of the selected snapshot, not its completeness or exit status. Each CLI invocation performs a fresh scan.

Use a source ID from `list` as the final `read` or `explain` argument, after all scope and JSON flags. Both commands have no view filters and return the same source detail, including every known discovery reference and availability explanation; JSON identifies the requested command in `operation`. Supporting resources are inventoried recursively inside each bundle without following resource symlinks or opening resource contents. Browsing and reading never execute bundled instructions or scripts. Read-only inspection changes neither sources nor agent settings; browser writes require the explicit control confirmation described above. Human output escapes terminal controls; JSON preserves the text for programmatic use.

Compare two IDs from `list`, with all flags before the IDs. Like `read`, `compare` accepts scope and JSON flags but no view filters. It reads current raw instructions and supporting resources, shows text differences and added/removed/changed entries, identifies binary changes without printing their bytes, and includes file-kind and mode changes. Selecting the same ID twice explains shared references to one source. Equal independent copies retain distinct identities. JSON includes `report.relation`, `report.outcome`, `report.coverage`, sorted `report.changes`, coverage `report.issues`, and both reference inventories. Completed equality or differences exit 0; incomplete comparison exits 1. Unrelated discovery-scope problems remain separate from selected-pair coverage.

Comparison never writes, executes bundled content, repairs links or consolidates copies. Incomplete coverage produces an indeterminate outcome rather than an identical claim. A clearly labeled truncated diff only limits presentation, not byte comparison. See [comparison behavior and limits](docs/verification/ale-25.md#behavior-and-limits) for link handling, read and presentation bounds, and the best-effort consistency policy.

Directory symlinks that reach the same proven directory object produce one source with several references. Equal names, equal bytes, and a shared `SKILL.md` target do not prove shared bundle identity, so those directories remain independently selectable. Every reference retains its discovery context, original discovery path, resolution status, and root; successfully resolved references also carry the resolved path and source ID. Broken and cyclic references stay available without a fabricated source. Filtering shows a source when at least one reference matches while detail keeps its complete reference inventory. JSON exposes the active `filters`, root contexts in `scope.roots`, reference contexts and `matches_filters`, and each problem's related root, reference, and source IDs.

The documented problem codes are `root-missing`, `root-unreadable`, `broken-reference`, `symlink-cycle`, `unreadable-manifest`, `malformed-manifest`, `metadata-non-finite-float`, `manifest-too-large`, `manifest-invalid-encoding`, `unreadable-resource`, `resource-limit`, `identity-uncertain`, `duplicate-name`, `shared-manifest`, and `native-input`. Duplicate names, shared manifests, and non-finite metadata are informational and do not make otherwise complete coverage incomplete. External resource links remain visible through their resource path, target, and `link_scope: external`; they are not traversed. Failures that prevent selected content from being discovered or read preserve useful partial output, mark coverage incomplete, and return status 1.

At 96 columns and wider, the browser keeps a 32 to 36 column source list beside a live rendered-instruction preview. Moving focus updates the preview; `Enter` opens the full-width reader. Below 96 columns, Sources uses the full width and `Enter` opens the focused reader. The reader starts on Instructions. Press `d` for exact Details, `i` to return to Instructions, and `v` to switch between rendered and escaped raw instructions. Details retains full source IDs, paths, identity, ownership and availability evidence, every reference, diagnostics, raw metadata, and supporting resources. Compact labels distinguish independent sources before Details, including same-name sources under different roots. See [source identity presentation](DESIGN.md#source-inventory-and-identity) for hint and truncation rules, and [responsive layout](DESIGN.md#responsive-layout) for compact filter indicators.

In Sources, press `m` to mark the focused source as the left side, navigate or search for a partner, then press `v` to compare. The same marked source can also be selected to inspect shared-source semantics. Comparison runs asynchronously and shows pair identity, coverage and paged differences; scroll with `j`/`k`, `PgUp`/`PgDn`, and `g`/`G`, and press `Esc` to return. Press `r` in Comparison to refresh the inventory and return to Sources, then `v` to compare again. `Esc` in Sources clears the mark; refresh retains it only while the same proven source object remains reachable.

For a selected native agent, press `a` to cycle configured availability and `c` to cycle global/system/managed as well as the ordinary reference contexts. Press `c` to cycle all, project, and custom reference contexts in generic inventory; `o` to cycle selected roots; `f` to cycle problem codes; and `r` to refresh explicitly. The browser has no watcher. Refresh observes external edits and retains a selected source only while the same proven source object remains reachable. A rename or one retargeted alias can retain selection when another reference still reaches that object. Replacement, loss, uncertain identity, or retargeting its final reference clears selection visibly. A selected source hidden by filters remains explicitly identified as hidden and is never rebound to a visible row.

Move with the arrow keys or `j`/`k`, press `/` to search the current Sources or Problems view, `Enter` to keep a search or open the focused item, and `Tab` to switch views. While editing search, `Esc` cancels and restores the pre-edit query; the left arrow leaves the editor and query unchanged. Outside search, `Esc` or the left arrow returns from a reader, Scope, or Help. Press `q` to quit outside search. Search supports bracketed paste as query data and `Ctrl+U` to clear. Readers, Scope, and Help also support `PgUp`/`PgDn`, `Home`/`End`, and `g`/`G` for top and end. Press `?` for complete scrollable help.

Refresh also clears a selected problem when it is no longer reported in the snapshot, not merely hidden by filters. Its open detail returns to Problems; if Scope is open, its return target changes instead. Disappearance feedback appears in the active Problems context without replacing source-selection feedback in Sources, source readers, or their Scope view.

`browse --theme auto` uses `COLORFGBG` to choose light when it reports a light background and otherwise uses dark. Choose `dark`, `light`, or `none` for deterministic output. Theme values must use these exact spellings. A non-empty `NO_COLOR` forces no-color output. Theme selection never writes settings or reads ambient Glamour style files. Rendered Markdown and Details use the [browser visual system](DESIGN.md#visual-system). Raw and Markdown-decoded terminal controls are shown as inert escape notation, and renderer-created terminal links are removed while their visible text and URL remain.

Press `s` from either inventory view or source/problem detail to inspect the full selected project, every discovery root, and active filters. The scope view scrolls with the arrow keys or `j`/`k`, `PgUp`/`PgDn`, and `g`/`G`; press `Esc` or the left arrow to return. While search input is active, `a`, `c`, `o`, `f`, `r`, and `s` remain ordinary query text. Long searches retain the complete query for filtering and show its current tail on one row with a leading ellipsis.

Generic inventory and Codex snapshots require YAML mapping frontmatter with a non-empty string `name` and an optional string `description`. Each other native integration documents its parsing and eligibility exceptions in the linked verification record, including Claude Code's name fallback and Pi's standalone Markdown resources. Mapping keys must decode to unique UTF-8 strings within each mapping; non-string keys and invalid decoded strings are rejected. Ordinary aliases and merge overrides remain supported. A malformed readable UTF-8 manifest stays selectable with exact `manifest.raw_text` and diagnostics, without fabricated valid metadata. Manifest reads are limited to 4 MiB and resource inventories to 10,000 entries; exceeding either bound reports incomplete coverage rather than silent truncation.

JSON output preserves YAML non-finite floats with an explicit, portable representation instead of failing serialization:

```json
{"kind":"non-finite-float","value":"nan"}
{"kind":"non-finite-float","value":"positive-infinity"}
{"kind":"non-finite-float","value":"negative-infinity"}
```

Each affected manifest also reports one `metadata-non-finite-float` diagnostic. The diagnostic does not limit discovery coverage, so an otherwise complete request still exits with status 0.

A manifest that is not valid UTF-8 reports `manifest-invalid-encoding`, remains selectable with its paths and resources, and makes coverage incomplete with exit status 1. JSON preserves the complete file as standard padded base64 in `manifest.raw_bytes_base64` instead of replacing invalid bytes. Content-derived text and metadata fields are omitted for that manifest. Human output and browser detail show a recovery hint directing you to `list` or `read` with `--json`; they do not display an altered body. Valid UTF-8 output is unchanged, including a literal U+FFFD character.

Exit status 0 means the requested result is complete, including an empty collection and help. Status 1 means incomplete coverage, an operational failure, an unknown selection, or an unsuccessful control request; useful partial output is preserved when available. See [control outcomes](docs/verification/ale-27.md#persistence-and-outcomes) for persistence and effective-result distinctions. On normal quit, the browser returns 1 if its current snapshot has incomplete coverage or its last confirmed control result was unsuccessful. Browsing, refresh and cancellation do not clear that control failure; a subsequent successfully confirmed change does. Status 2 reports command usage errors.

## Development workflow

Follow the Matt Pocock skill sequence in the [coding workflow](docs/agents/coding-workflow.md). Implement authorized work, verify the actual result, and review Standards and Spec independently. For parallel tickets and ticket parts, delegated decisions, and automatic no-mistakes fixes, follow the [agentic-loop operating guide](docs/agents/agentic-loop.md), including its approval boundaries.

The active Codex chat drives the loop through native agents. The scheduler persists leases and evidence across interruptions. It does not launch models or keep running after the coordinating chat ends.

For scheduler checks, executable acceptance, terminal evidence, static checks, and containment limits, follow the [ALE-24 verification guide](docs/verification/ale-24.md#reproduce-the-acceptance-evidence).

Skill links in the workflow point to the maintainer's installed local skills; those external skill files are not bundled in this repository.

## Planning records

The `.scratch/skill-viewer/` documents preserve earlier discovery and decision history. Their historical open decisions and status text are not the current implementation queue; the published Linear specification and tickets take precedence.

Downloaded toolchains under `.tools/` and local review artifacts under `.lavish/` are excluded from version control. Links to those artifacts in historical notes refer to the original local workspace.
