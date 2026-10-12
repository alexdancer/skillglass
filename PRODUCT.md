# Product

## Product promise

Skillglass gives people a clear, trustworthy view of the agent skills selected from explicit local roots. It helps them find a skill, distinguish independent sources, read instructions, and inspect provenance or collection problems without executing or changing anything.

The current product is a local macOS terminal application. It operates offline after dependencies are installed and exposes one immutable collection snapshot through command output and the interactive browser.

## Primary users and jobs

The primary user maintains skills for one or more agent applications and projects. Their main jobs are:

1. Find a skill by name or description.
2. Know which independent source is focused or selected.
3. Read its instructions immediately.
4. Inspect exact identity, paths, references, metadata, resources, and diagnostics when needed.
5. Understand whether discovery coverage is complete and which filters or roots define the current view.

Problem investigation, scope inspection, and refresh are supporting jobs. Future mutation and agent-integration work is outside the current read-only product boundary.

## Experience principles

### Instructions lead

A skill's instructions are the primary content. Wide terminals keep the collection beside a live instruction preview. Smaller terminals open a focused reader. Exact IDs and provenance remain available through Details without displacing the ordinary reading task.

### Identity stays visible

The authoritative source identity is the stable `SourceID`, not a display name, manifest bytes, path text, or a shared manifest target. The browser leads with human-readable name and root provenance. When those labels would collide, it adds the shortest stable source-ID hint needed to distinguish independent sources. A hidden retained selection always carries a source-ID hint that is unique across the snapshot because its compact summary may omit provenance. Width allocation preserves the hidden marker, identity hint, and compact provenance before truncating a long human name. Exact IDs and paths remain in Details.

### Scope and coverage are never implicit

The browser always shows coverage, source/problem mode, selected root count, active filters, and the focused or selected item. The Scope view contains the full project and root paths. Filtering changes the view, not the meaning of coverage or the selected snapshot.

### Read-only behavior earns trust

Browsing never executes bundled instructions or scripts, follows no resource symlink for content, writes no source or settings file, and requires no runtime network access. Refresh occurs only on explicit request. Human-facing output renders untrusted control characters as inert text.

### Narrow terminals remain complete

At 48, 60, and 80 columns the browser uses one focused pane, keeps navigation understandable, and makes complete help scrollable. At 80 by 30, an ordinary opened skill shows its title, description, and at least eight instruction lines without scrolling when that content exists.

### Color supports structure

Text labels, focus markers, headings, and spacing carry meaning in every theme. A restrained blue accent and one selected-row surface support hierarchy in dark and light themes. No-color output preserves the same structure without terminal styling.

## Current product boundary

Skillglass currently supports:

- explicit custom and project-associated discovery roots;
- independent source identity and complete reference provenance;
- source, problem, context, root, and text filtering;
- human and JSON list, problem, and read commands;
- a responsive keyboard browser with source instructions, details, problems, scope, help, and explicit refresh;
- rendered Markdown with an exact escaped raw view;
- recursive supporting-resource inventory without content execution;
- useful partial results and explicit incomplete coverage.

Skillglass does not currently infer roots, watch the filesystem, decide ownership or availability without evidence, edit agent settings, enable or disable skills, delete content, or synchronize with a remote service.

## Success criteria

A successful browser session lets a user identify and read the intended source without first interpreting internal IDs or long paths. Independent same-name sources remain distinguishable before Details. Problems and incomplete coverage remain visible without blocking healthy sources. Every compact representation leads to exact scope, provenance, and raw content, and every supported terminal size remains navigable without horizontal overflow.
