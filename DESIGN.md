# Design

## Chosen direction

The production browser uses **Quiet split**, the approved ALE-43 direction. It keeps the collection and the focused instructions in one visual context at wide widths, then reduces to one focused pane when space is limited. The design uses minimal dividers, blue navigation, lavender content accents, quiet secondary text, and a visible selected-row surface.

The collection snapshot and stable `SourceID` remain the domain authority. Browser state is limited to mode, focus, selected IDs, source tab, rendered or raw instruction view, filters, scroll positions, feedback, terminal size, and presentation theme.

## Information hierarchy

Every frame has three regions:

1. **Chrome** shows `Skillglass`, coverage, Sources and Problems, root count, active filters, and focused or selected identity.
2. **Content** shows the current inventory, reader, problem, scope, or help view.
3. **Footer** shows the shortest useful controls for that context, with `? help` and `q quit` retained in ordinary views at narrow widths.

Help and Scope keep the underlying inventory or reader's mode and focused or selected identity in chrome, including hidden selections and Help opened from Scope.

Search sits directly above the active inventory. Source rows show name, description, and compact provenance. Focus uses both a `›` marker and a selected surface, so no state depends on color alone.

## Responsive layout

### 96 columns and wider

Sources use a split layout. The left list is 32 to 36 columns wide. The remaining width is a live preview of the focused source's rendered Instructions. Moving focus updates the preview without changing source selection. `Enter` opens the full-width reader for sustained reading.

Problems use the full content width because their summary and path need horizontal space.

### Below 96 columns

Sources and Problems use the full width as inventories. `Enter` opens a full-width source or problem reader. `Esc` or the left arrow returns to the previous inventory. The same reader is used at 48, 60, and 80 columns, so resize does not change the selected identity or reading mode.

Chrome normally uses three lines at wide widths and four below 96; Sources adds a line when a retained selection is hidden by filters. Active filters reserve space for every dimension and truncate values individually. Compact summaries use `s:` for search, `c:` for context, `r:` for root, and `p:` for problem. Full project, root, source, and reference paths remain reachable in Scope or Details.

## Source inventory and identity

A source row uses four terminal rows: name, description, provenance, and spacing. The focused source stays within the visible row window.

Compact provenance combines reference contexts, root labels, and a reference count when more than one reference reaches the source. Equal root basenames receive stable scope ordinals such as `#1 skills` and `#2 skills`. Independent sources with the same display name receive a globally unique prefix of the stable source ID, such as `review [k4mz]`, even when their full provenance differs because compact provenance can truncate away that difference. Prefixes use at least four characters and extend only as needed for uniqueness across the snapshot. The hint appears in the list, Focused or Selected line, and reader title. A retained selection hidden by filters always uses that globally unique prefix, even for a unique name, because its compact line may omit provenance. Width-constrained labels preserve the hidden marker and identity hint, reserving bounded space for compact provenance in Focused and Selected chrome and truncating the human name to the remaining width. Details always shows the complete source ID and paths.

This display rule never groups or re-identifies sources. It only derives labels from the authoritative snapshot.

## Source reader

The source reader opens on **Instructions**. Its fixed header contains the visible source label, up to two description lines, the Instructions and Details tabs, and one divider. The remaining viewport belongs to content.

Rendered Instructions support headings, emphasis, links, blockquotes, nested lists, tables, inline code, and fenced code through Glamour. `v` switches between Rendered and Raw instructions. Raw preserves the exact instruction text while escaping terminal controls for display.

Details contains, in order:

- complete source ID and source path;
- identity method and certainty;
- ownership and availability state with reasons;
- manifest status and link paths when applicable;
- every reference ID, context, root ID, discovery path, resolved path, status, and filter match;
- source diagnostics;
- raw frontmatter and malformed raw manifest text when applicable;
- the full supporting-resource inventory.

`d` opens Details and `i` returns to Instructions. Arrow keys, `j`/`k`, `PgUp`/`PgDn`, `Home`/`End`, and `g`/`G` scroll the active reader.

## Problems, scope, and help

Problems show a text severity label, problem code, summary, and the distinguishing end of long paths. Problem detail exposes the exact problem ID, code, path, summary, detail, coverage effect, and related source, reference, and root IDs.

Scope shows the selected project, active filters, every root's state and context, counts, IDs, requested path, and resolved path. It is scrollable and returns to the prior inventory or reader, subject to [refresh reconciliation](#empty-error-and-refresh-states).

Help is a scrollable view. It documents movement, open and return, Sources and Problems switching, search, clear, source tabs, raw view, every filter, refresh, Scope, top and end, help, quit, theme selection, and `NO_COLOR`. Its scroll position resets when opened and remains valid across resize.

## Interaction protocol

| Key | Action |
|---|---|
| `↑` `↓` or `j` `k` | Move focus or scroll |
| `Enter` | Keep search or open the focused item |
| `Esc` | Cancel active search and restore its pre-edit query; otherwise return from a reader, Scope, or Help |
| `←` | Return from a reader, Scope, or Help; remain in active search without changing its query |
| `Tab` | Switch Sources and Problems |
| `/` | Edit search for the active inventory |
| `Ctrl+U` | Clear search while editing |
| `d` / `i` | Show source Details or Instructions |
| `v` | Toggle rendered and raw instructions |
| `c` | Cycle all, project, and custom contexts |
| `o` | Cycle selected roots |
| `f` | Cycle documented problem codes |
| `r` | Refresh the collection explicitly |
| `s` | Inspect full scope and filters |
| `PgUp` / `PgDn` | Move one page in a reader, Scope, or Help |
| `Home` / `End` | Move to top or end in a reader, Scope, or Help |
| `g` / `G` | Move to top or end |
| `?` | Toggle complete help |
| `q` | Quit outside search |

While search input is active, filter and navigation letters are ordinary query text. `Esc` cancels the edit and restores the query that was active before `/`; `←` does not cancel or edit search. Search keeps the complete query for filtering and shows its visible tail with a leading ellipsis when necessary.

## Visual system

Dark and light themes share semantic roles:

- **Navigation accent:** product name, active mode, active tab, and focus.
- **Content accent:** Markdown headings, links, and inline code, plus Details labels, section titles, and YAML mapping keys.
- **Body:** primary reading text.
- **Muted:** descriptions, provenance, dividers, inactive tabs, and help text.
- **Caution:** incomplete coverage.
- **Selected surface:** the complete focused inventory row.

Dark uses navigation accent `#93C5FD`, content accent `#C4B5FD`, body `#DCE5EF`, muted `#A8B3C3`, code background `#342B47`, and selected background `#20425B`. Light uses navigation accent `#0369A1`, content accent `#6D28D9`, body `#1E293B`, muted `#5A6574`, code background `#EFE8FF`, and selected background `#DBEAFE`.

The Markdown renderer uses the content accent for headings, links, and inline code. Details styles parser-identified YAML mapping keys over the preserved raw text. Values, block-scalar bodies, and malformed raw metadata stay neutral. The renderer does not load ambient Glamour configuration or a separate syntax-color theme. No-color mode emits structural text without SGR styling.

`browse --theme auto|dark|light|none` selects presentation deterministically. `NO_COLOR` overrides dark or light to none. Auto selects light only from a light `COLORFGBG` background and otherwise uses dark. Theme selection reads environment values but writes no settings.

## Terminal safety

All untrusted manifest, path, problem, and resource text passes through terminal-control escaping before display. Markdown numeric entities that decode to C0 or C1 controls are converted to visible escape notation before parsing. The final rendered stream admits only renderer-owned SGR sequences. OSC links are removed while their visible text and URL remain, and any other introduced terminal control is rendered inert.

If Markdown rendering fails, the reader shows escaped raw text with a visible fallback message. Exact JSON remains available through `read --json` and `list --json`.

## Empty, error, and refresh states

An empty selected scope says that no sources were found. A filtered empty state says there are no matching sources or problems. Coverage stays visible as complete or incomplete in both states.

The [README's refresh rules](README.md#build-and-use) own source and problem selection reconciliation, reader and Scope return targets, and context-specific feedback. Filtering alone does not clear a retained selection.

The browser has no loading spinner because discovery completes before the interactive program starts and refresh is explicit. Operational discovery failures remain represented as collection problems and incomplete coverage.
