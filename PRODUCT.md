# Product

## Product promise

Skillglass gives people a clear, trustworthy view of the agent skills selected from explicit local roots or native agent discovery. It helps them find a skill, distinguish independent sources, read instructions, and inspect provenance or collection problems without executing skill content. Explicit global Codex controls offer a separate preview and confirmation flow.

The current product is a local macOS terminal application. It operates offline after dependencies are installed and exposes one immutable collection snapshot through command output and the interactive browser.

## Primary users and jobs

The primary user maintains skills for one or more agent applications and projects. Their main jobs are:

1. Find a skill by name or description.
2. Know which independent source is focused or selected.
3. Read its instructions immediately.
4. Inspect exact identity, paths, references, metadata, resources, and diagnostics when needed.
5. Understand whether discovery coverage is complete and which filters or roots define the current view.

Problem investigation, scope inspection, and refresh are supporting jobs. Native discovery supports Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF. Comparison and explicit global Codex choices are supported.

## Experience principles

### Instructions lead

A skill's instructions are the primary content. Wide terminals keep the collection beside a live instruction preview. Smaller terminals open a focused reader. Exact IDs and provenance remain available through Details without displacing the ordinary reading task.

### Identity stays visible

Independent sources must remain distinguishable before Details, even when names match or long labels are truncated. Compact presentation must not change source identity or rebind a retained selection. [Terminal design](DESIGN.md#source-inventory-and-identity) owns the labeling and width-allocation rules.

### Scope and coverage are never implicit

Scope, coverage, filters, and target identity must remain understandable without inferring them from truncated paths. [Terminal design](DESIGN.md#information-hierarchy) owns their per-view presentation, and the [README](README.md#build-and-use) explains full scope inspection. Filtering changes the view, not the meaning of coverage or the selected snapshot.

### Read-only behavior earns trust

Browsing never executes bundled instructions or scripts, follows no resource symlink for content, writes no source file or settings file during inspection, and requires no runtime network access. Refresh occurs only on explicit request. Human-facing output renders untrusted control characters as inert text.

### Narrow terminals remain complete

At 48, 60, and 80 columns the browser uses one focused pane, keeps navigation understandable, and makes complete help scrollable. At 80 by 30, an ordinary opened skill shows its title, description, and at least eight instruction lines without scrolling when that content exists.

### Color supports structure

Text labels, focus markers, headings, and spacing carry meaning in every theme. Color reinforces navigation, source identity, and readable content structure without being the only signal. The [visual system](DESIGN.md#visual-system) owns palette roles, selected-row styling, and no-color presentation.

## Current product boundary

Skillglass currently supports:

- explicit custom and project-associated discovery roots;
- independent source identity and complete reference provenance;
- source, problem, context, root, and text filtering;
- human and JSON list, problem, read, explain, and comparison commands;
- native availability evidence and filtering;
- global Codex enable and disable with a preview, explicit confirmation, freshness validation, and persisted-result verification;
- a responsive keyboard browser with source instructions, details, problems, scope, help, and explicit refresh;
- rendered Markdown with an exact escaped raw view;
- recursive supporting-resource inventory without content execution;
- useful partial results and explicit incomplete coverage.

Skillglass does not currently watch the filesystem, decide ownership or availability without evidence, change settings outside supported global Codex controls, delete content, or synchronize with a remote service.

## Success criteria

A successful browser session lets a user identify and read the intended source without first interpreting internal IDs or long paths. Independent same-name sources remain distinguishable before Details. Problems and incomplete coverage remain visible without blocking healthy sources. Every compact representation leads to exact scope, provenance, and raw content, and every supported terminal size remains navigable without horizontal overflow.
