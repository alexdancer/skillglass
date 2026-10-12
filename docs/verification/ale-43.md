# ALE-43 verification record

ALE-43 replaces the metadata-first terminal browser with the approved Quiet split design. At wide widths the source inventory stays beside a live rendered-instruction preview. At smaller widths the same source opens in a focused reader. Instructions lead, while exact identity, provenance, metadata, resources, problems, and scope remain reachable.

The [README](../../README.md#build-and-use) owns current commands and keys. [PRODUCT.md](../../PRODUCT.md) records the product boundary and experience principles. [DESIGN.md](../../DESIGN.md) records the chosen visual system, responsive rules, state model, and safety boundary.

## Corrections made from executable evidence

- The accepted ALE-24 executable reproduced the reported hierarchy defect. At 80 by 30, opening an ordinary skill showed its title and description but zero of twelve marked instruction lines in current terminal cells. The focused ALE-43 reader now shows all twelve in the same fixture, exceeding the required eight lines.
- Glamour 2.0.1 decodes numeric HTML entities before terminal rendering. A direct dependency probe showed that `&#27;]52;c;SECRETDATA&#7;` became an active OSC 52 sequence and `&#13;` became a carriage return. The browser now neutralizes encoded controls before Markdown parsing and sanitizes the rendered stream afterward. The hostile text remains visible as inert escape notation. Raw mode retains the original entities.
- Glamour's built-in terminal styles introduced unrelated purple, yellow, green, and red surfaces. Explicit dark, light, and no-color styles now use the browser's single blue accent, body, muted, and quiet code-background roles. Ambient Glamour style files are never loaded.
- A long feedback message could displace both help and quit at 48 columns. Contextual footers now reserve a compact right-hand `? help  q quit` region. Narrow reader and problem footers shorten their left-hand guidance without losing the return or open action.
- Help initially fit its full content only at taller sizes. At 48 by 20, `G` left the screen byte-identical and lower movement, help, quit, theme, and `NO_COLOR` guidance was unreachable. Help now has its own scroll position, top/end and page movement, resize clamping, and a return target.
- Equal-name sources under roots with the same basename produced identical list, Focused, and Selected labels. Root labels now include stable scope ordinals when basenames collide.
- Two independent equal-name sources within the same physical root still produced identical labels after root disambiguation. Source-ID hints repaired that ambiguity; [DESIGN.md](../../DESIGN.md#source-inventory-and-identity) owns the current hint rules, including same-name sources whose full provenance differs.
- Long problem paths showed only their common leading directories in compact rows. Problem inventory now retains the distinguishing path tail while problem detail retains the exact full path.
- Entering search after a filter action retained stale filter feedback in place of search-result navigation status. Entering search now clears transient feedback without changing filters.
- Help handled `q` as an inert key before the global quit action. Global quit now runs from Help and every other view outside active search, where `q` remains query text.
- A retained selection hidden by active filters could disappear from the header while a different visible source held focus. Sources now keep a separate hidden-selection line, and the reader marks a filtered selection directly. At narrow widths the hidden marker precedes a long label so truncation cannot remove the state.
- Committed search remained visible only in the inventory search row. The fixed filter summary now starts with the active search query in source readers and problem detail, while Scope retains the complete exact query.
- Help supported page and Home/End movement but omitted those aliases from its key reference. The complete scrollable Help now lists both `PgUp`/`PgDn` and `Home`/`End`.
- Very long equal names consumed the row and chrome width before an existing stable source-ID hint or root provenance could appear. Source labels now reserve width for the hint and compact provenance, then truncate the human name consistently in inventory rows, Focused and Selected chrome, live preview and reader titles, and hidden-selection fallback.
- Hidden selections with the same name but different root provenance could collapse to the same summary because collision hints were derived only within the visible name-and-location group and the hidden line omitted location. Hidden summaries now reserve the shortest stable source-ID prefix that is unique across the complete snapshot, including at 48 columns.

The hostile source-name, description, and root-basename probe did not reproduce active terminal controls on the accepted candidate. Escaping those display fields remains defense in depth and is not claimed as a reproduced defect.

## Public behavior covered

`TestBrowserTmuxCellsAtRequiredWidths` drives real tmux panes at 48, 60, 80, 96, and 120 columns by 30 rows. It verifies search, current-cell dimensions, split list and live preview at wide widths, focused reading at smaller widths, eight unscrolled instruction markers at 80 by 30, Details, resources, narrow resize, and hidden terminal cursor state.

`TestBrowserHelpIsScrollableAt48x20` opens Help in a 48 by 20 pane, proves the top guidance, uses `G` to expose movement, help, quit, theme, and `NO_COLOR` guidance, and returns to the unchanged Sources view.

`TestBrowserHelpQQuitsOutsideSearch` and `TestBrowserHelpListsPagingAndTopEndAliases` cover the global quit rule and the complete movement reference. `TestBrowserRetainedHiddenSelectionStaysVisible`, `TestBrowserRetainedHiddenSelectionIsLabeledInReader`, and `TestBrowserLongHiddenSelectionMarkerSurvivesNarrowLayout` prove retained state at wide and narrow sizes, including a long selected label. `TestBrowserActiveQueryStaysInChrome` covers source reader and problem detail summaries, including a 48-column reader with simultaneous context and root filters.

`TestBrowserPreservesLongSourceIdentityHints` uses three byte-identical sources with a 128-character shared name under equal-basename roots. It proves distinct list rows, Focused chrome at 120 and 48 columns, live preview and reader titles, Selected chrome, hidden-selection fallback, and fixture immutability.

`TestBrowserHiddenSelectionPreservesDistinctIdentity` selects byte-identical same-name sources under separate equal-basename roots, records their distinct complete IDs in Details, hides each behind the same root filter, and proves distinct retained-selection summaries at 48, 80, and 120 columns without modifying fixtures.

`TestBrowserDisambiguatesEqualSourcesUnderSameBasenameRoots` proves distinct hints and root ordinals at 120 and 48 columns and preserves both in the reader. `TestBrowserDisambiguatesEqualSourcesWithinOneRoot` uses equal names, descriptions, and instructions for two bundle directories inside one root, then proves distinct source-ID hints in inventory, focus, and reader labels. `TestBrowserSameNameAcrossLongRootNames` covers distinct roots whose differing basename tails are lost in compact provenance.

`TestBrowserEveryActiveFilterSurvivesWidthAllocation` checks all active dimensions in Sources, Problems, readers, Scope, and Help across the five required widths with a long query. `TestBrowserDetailsReferenceFilterFlags` checks complete reference inventory and context/root match flags, including hidden selections, and unchanged unfiltered CLI `read` semantics. `TestBrowserRejectsAdditionalThemeSpellings` checks rejection of aliases and case/whitespace variants.

`TestBrowserThemeModesApplyDeterministically` captures ANSI for explicit dark and light themes and proves that explicit none and `NO_COLOR` emit no SGR or OSC sequences. Theme unit tests cover auto detection, invalid values, and `NO_COLOR` precedence.

`TestRenderMarkdownSupportsCommonSkillContent` covers headings, emphasis, links, nested lists, blockquotes, inline code, and fenced code. `TestRenderMarkdownNeutralizesDecodedTerminalControls` covers numeric control entities, raw controls, OSC 8 removal, and retained visible text. Raw browser journeys prove exact hostile entities remain accessible.

The inherited ALE-23 and ALE-24 suites continue to cover collection identity, filters, problem search, refresh reconciliation, scope, long search, invalid encodings, metadata, resource inventory, containment, exit codes, and fixture immutability. Presentation assertions were updated only where ALE-43 intentionally replaced the old header and metadata-first reader. Their source selection, identity, provenance, refresh, and search assertions remain.

## Reproduce the acceptance evidence

Follow the [current combined-suite procedure](ale-24.md#reproduce-the-acceptance-evidence) for prerequisites, a fresh absolute evidence directory, the public executable runner, fixture immutability checks, and repository checks after the last change. The same runner includes all ALE-43 journeys above; the [ALE-23 containment boundary](ale-23.md#runtime-containment-boundary) defines isolation guarantees and limits.

Inspect current terminal cells rather than relying only on raw history. Required production captures include 48, 60, 80, 96, and 120 columns, a 48 by 20 Help journey, dark and light ANSI output, no-color cells, empty and missing roots, long Markdown, same-name independent sources, and hostile content. Prototype images are design references and are not acceptance evidence.

## Evidence limits

Terminal color appearance depends on the user's terminal palette and color support. The acceptance suite proves emitted theme roles and no-color behavior, while visual review of rasterized current-cell captures checks hierarchy and contrast in the reference dark and light palettes.

The sanitizer allows only syntactically valid CSI SGR sequences after Markdown rendering. Tests cover known raw and entity-decoded control paths, OSC 8 links, OSC 52 clipboard sequences, title OSC, BEL, carriage return, and C1 controls. This is a terminal-output boundary, not a general HTML security claim.
