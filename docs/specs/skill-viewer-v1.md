# Skill Viewer v1: browse, explain, compare, and control local skills

Published in Linear: [Build Skill Viewer v1: browse, explain, compare, and control local skills (ALE-21)](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills). This file is a local reference copy.

## Problem Statement

My local skills are spread across several agent applications, shared locations, projects, and managed plugins. I cannot easily see the complete collection within a known search scope, read a skill, or understand which applications can actually use it. Repeated names and symlinks make it hard to distinguish independent copies from shared references. A change that looks local to one agent can affect other applications, while a skill disabled in one application can remain available in another.

I need a terminal tool that makes this collection understandable and lets me deliberately control availability or delete selected content. I should be able to trust the scope of an action, see unsupported controls, and understand why the tool reports a particular state.

## Solution

Build Skill Viewer in Go, with an interactive terminal browser and ordinary CLI commands. Start with the user's Mac and an offline core. Discover and read skills, explain availability for an agent application and project, compare independent copies, and apply explicitly selected controls using each application's validated capabilities.

The first release targets Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF. It supports project skills and project overrides of globally available skills where enforceable. CodeAF browsing with clearly unsupported persistent per-skill controls is an explicitly accepted first-release exception. Unknown or unsupported capabilities must remain visible for every application.

The distinguishing workflows are availability explanations and comparison of duplicate or diverged copies. Cleanup means finding problems, comparing content, and letting the user choose disable or permanent deletion after confirmation. Independent copies remain separate; shared references expose the common source and known effects of changing it.

## User Stories

1. As a skill owner, I want an inventory of skills in the configured discovery scope, so that I can understand my collection from one place.
2. As a terminal user, I want a keyboard-driven browser, so that I can explore skills without remembering commands.
3. As a CLI user, I want ordinary commands for inventory, reading, explanation, comparison, and controls, so that I can perform a specific task directly.
4. As a skill owner, I want to search names and descriptions, so that I can find a relevant skill quickly.
5. As a skill owner, I want to filter by agent application, project, availability, and reported problems, so that I can focus on the part of the collection I am managing.
6. As a reader, I want to read a skill's full instructions, so that I can decide whether it is useful.
7. As a reader, I want to see the skill's source and supporting resources, so that I understand what the bundle contains.
8. As a skill owner, I want to see which locations were searched and which could not be read, so that I know the limits of the inventory.
9. As a skill owner, I want to include a selected project or custom skill location, so that skills outside default locations can be inspected deliberately.
10. As a user with several projects, I want the active project to be explicit, so that I do not mistake one project's state for another's.
11. As a project user, I want to see project skills alongside inherited global skills, so that I can understand the project's effective collection.
12. As a project user, I want to distinguish an inherited choice from a project override, so that I can understand which setting controls availability.
13. As a skill owner, I want shared references to identify their common source, so that I understand which changes could affect several applications.
14. As a skill owner, I want independent copies with the same name to remain separately selectable, so that one copy is never silently substituted for another.
15. As a skill owner, I want a duplicate-name warning, so that I notice possible conflicts before changing anything.
16. As a skill owner, I want to distinguish identical independent copies from diverged copies, so that I can make an informed cleanup choice.
17. As a skill owner, I want to compare instructions and supporting resources, so that differences outside the main instructions are visible.
18. As a skill owner, I want comparison to leave all copies intact, so that inspection does not become automatic consolidation.
19. As an agent user, I want an explanation of why an application can or cannot use a skill, so that I can act on evidence instead of an unexplained toggle.
20. As an agent user, I want an explanation to identify the winning source, relevant setting, and precedence rule, so that I can understand shadowed copies and inherited state.
21. As an agent user, I want observed facts, inferred state, and unknown state to be distinguishable, so that uncertainty is not presented as verified availability.
22. As an agent user, I want explicit-only, ask, disabled, shadowed, and unavailable states to remain distinguishable where applicable, so that reduced automatic use is not mistaken for full disable.
23. As a skill owner, I want to choose which agent applications may use a selected skill, so that I can tailor my setup while retaining the content.
24. As an agent user, I want to disable a skill for one selected application, so that other applications are unaffected when that scope is supported.
25. As an agent user, I want to enable a previously disabled skill through a supported control, so that availability can be restored without reinstalling the content.
26. As a project user, I want a project override of a global skill, so that one project can have a different availability choice.
27. As a project user, I want to remove a project override and inherit again, so that the project can return to the applicable global behavior.
28. As a project user, I want unsupported project overrides to be explained, so that a project action cannot silently change my global setup.
29. As a skill owner, I want to request disable for all discovered affected applications, so that I can stop use of a selected source across my known setup.
30. As a skill owner, I want unsupported consumers and incomplete discovery called out during that request, so that partial coverage is never represented as universal success.
31. As a skill owner, I want a control preview to identify the source, applications, project scope, and affected settings, so that I understand the operation before applying it.
32. As a skill owner, I want canceling an operation to make no changes, so that I can inspect its effects without committing to it.
33. As an agent user, I want existing unrelated settings preserved, so that managing skills does not damage the rest of my setup.
34. As an agent user, I want reload or restart requirements separated from successful configuration changes, so that I know when a running application will observe the result.
35. As a skill owner, I want stale previews and external changes detected, so that an operation does not overwrite newer state.
36. As a skill owner, I want per-application results and actionable failures, so that I can understand a partially completed operation.
37. As a plugin user, I want plugin ownership and control limits shown, so that I do not mistake a whole-plugin action for an isolated skill toggle.
38. As a CodeAF user, I want to browse its discovered skills and understand precedence, so that CodeAF is represented alongside my other applications.
39. As a CodeAF user, I want unsupported persistent per-skill disabling shown clearly, so that detaching a conversation attachment or changing another application's setting is not presented as disabling CodeAF.
40. As a skill owner, I want broken references, unreadable sources, malformed manifests, and copy conflicts reported, so that I can decide what needs attention.
41. As a skill owner, I want permanent deletion to identify the exact selected content and known affected references before confirmation, so that I can make a deliberate destructive choice.
42. As a skill owner, I want deletion of one independent copy to leave other copies intact, so that matching names cannot cause unintended removal.
43. As a skill owner, I want deleting a shared source to explain the consequences for its references, so that I understand why other applications may lose that content.
44. As a skill owner, I want deletion to be described as permanent without an undo promise, so that its recovery limits are clear.
45. As an offline user, I want discovery, reading, comparison, and supported controls to work without a network service or account, so that my local collection remains usable privately.
46. As a skill owner, I want changes to happen only when I select them, so that browsing never performs cleanup or background mutation.

## Implementation Decisions

- **Language and delivery.** Implement the production tool in Go for the user's current macOS environment. Keep application-specific rules behind adapters so additional environments can be supported later. A public open-source release is a possible future direction, not a requirement to select a license or publish now. At specification publication, no terminal framework, command spelling, packaging tool, or database had been selected. The [README](../../README.md#build-and-use) owns the implemented interface.

- **One behavior core, two terminal interfaces.** The CLI and interactive browser use the same inventory, availability, comparison, and mutation operations. Build logical components for discovery and inventory, agent adapters, availability explanations, bundle comparison, and scoped changes. The public terminal interface is the primary test boundary; private component layout is an implementation choice. Read operations produce no changes to agent settings or skill contents.

- **Discovery is bounded and explainable.** Cover the known roots, configured locations, project rules, and plugin sources supported by each adapter. Let users select the project and add a discovery location. Show missing or inaccessible locations and the effective search scope. Do not scan the entire machine by default or infer application installation from a folder alone. A custom location can be browseable without being a source an agent actually loads. Handle symlink cycles and malformed or unreadable entries without losing the rest of the inventory. Refresh on explicit request and before mutation verification.

- **Identity is source-based, not name-based.** Retain the discovered location and the resolved underlying source. Present shared references together while keeping their application and project provenance visible. Equal names or equal contents do not merge independent copies. Every change targets an unambiguous source and scope; ambiguous name-only selection must be resolved before a write. An all-applications action does not implicitly include independent copies with the same name.

- **Reading and comparison cover the bundle.** Show full instructions, metadata, source, and supporting resource inventory. Compare selected independent copies with added, removed, and changed resources and readable text differences; identify changed binary resources without pretending they are text. A shared reference is not a duplicate copy. Missing, unreadable, or externally linked resources must make comparison coverage explicit. Do not execute skill instructions or bundled scripts while scanning, reading, or comparing.

- **Availability is contextual evidence.** Compute state for the selected source, agent application, and project using discovery, precedence, configuration layers, and ownership. Explanations identify the relevant rule or setting, winner and shadowed sources where known, and any remaining uncertainty. Preserve meaningful native distinctions such as explicit-only or ask. Separate discovery, configured permission, and observation by a running session. The viewer's own preferences cannot be used as evidence that an agent has disabled a skill.

- **Agent support is validated per operation and scope.** For Codex, Claude Code, Hermes, Pi, OpenCode, and Gemini CLI, validate supported discovery and native enable/disable controls against a recorded version before enabling writes. Validate project discovery, precedence, project overrides, shared-reference matching, name collisions, plugin ownership, and reload behavior separately. The initial research is a starting point rather than a tested adapter contract. Gemini requires a suitable validation environment because the inspected Mac did not have its CLI installed. Show unknown or unsupported controls with reasons. This is not a blanket reduction of the other six integrations to browsing-only support.

- **CodeAF has an explicit release exception.** Include browsing and availability explanations. For the inspected behavior, persistent CodeAF-only per-skill disable is unsupported. Conversation attachment changes are not persistent disable, and another application's per-skill setting does not establish CodeAF state. A future native writer requires fresh version-specific validation. First release does not depend on inventing such a writer.

- **Supported controls must honor the requested scope.** Prefer validated native application controls. Distinguish a machine-level choice, a project override, and removal of an override. Enabling cannot promise availability when a higher-priority setting or source still prevents it; show the resulting effective state. If a name-based or plugin-level mechanism would affect additional independent copies or a broader scope, do not execute it as an isolated source action. Do not simulate unsupported per-agent or project controls by relocating shared content, renaming manifests, or editing skill instructions.

- **All-applications control is an explicit aggregate.** Preflight the selected source against all discovered consumers and report incomplete discovery. If any requested consumer cannot be controlled, do not apply a partial subset as though the all-applications request succeeded. Offer a separately selected supported subset, or leave the request unapplied. For an executable multi-application request, retain per-application outcomes and report any runtime failure or partial completion. Do not claim cross-file atomicity or complete machine-wide coverage. CodeAF can therefore prevent a truthful all-applications success for a source that it also discovers.

- **Mutation previews are tied to current state.** Show the selected source, applications, project, exact intended settings or content changes, and known broader effects. Applying a preview requires rechecking relevant source identity and configuration; changed state invalidates the preview. Preserve unrelated settings and native states rather than replacing entire lists. Use safe writes and verify persisted state after applying. Distinguish unsupported, refused, failed, partially applied, and applied outcomes; CLI outcomes must provide human-readable reasons and a non-success exit status when the requested result was not achieved. Do not automatically launch or restart agent sessions.

- **Managed content remains owned by its manager.** Expose plugin provenance and whether the host offers per-skill, whole-plugin, or no relevant control. Whole-plugin management is broader than this spec's isolated skill control; do not mutate a managed cache or disable an entire plugin under a per-skill action. Where deletion or a control cannot be performed at the requested scope through a validated supported mechanism, explain the limitation and leave content intact.

- **Permanent deletion has a separate contract.** Delete removes the selected user-owned skill bundle after explicit confirmation of its source and known impact. It does not merely unlink one shared reference, does not follow links inside the bundle to delete external content, and does not silently repair references left behind. Independent copies remain intact. Validate that the selected bundle is a safe deletion boundary; refuse uncertain ownership or a target that would encompass a broader root. Cancellation and missing confirmation perform no write. Noninteractive invocation must not bypass confirmation implicitly; it must carry an explicit, target-specific confirmation or fail safely. Report actual outcomes, including partial removal if a filesystem failure interrupts deletion. No trash, automatic recovery, or deletion undo is promised.

- **Interaction details remain flexible within these contracts.** The browser must make the selected source, agent, project, and action scope visible, allow movement from inventory to reading, explanation and comparison, and separate disable from delete. Exact panes, keybindings, command names, and visual styling are not previously accepted decisions. Work them out during implementation with a small interaction prototype covering a shared source, independent same-name copies, a project override, a managed plugin, and an unsupported CodeAF control.

- **Offline and deliberate by default.** Core operations need neither an account nor a network service. Do not upload skill text, ingest agent activity logs, install skills or agent applications, or perform background cleanup as part of these workflows. Local metadata may support the UI, but skill sources and agent settings remain the authority for content and availability.

## Testing Decisions

- **Confirmed primary boundary.** On October 10, 2026, the user approved testing the built Go executable through its public commands with temporary home and project folders containing realistic skills and agent settings. Assert observable output, exit status, and actual filesystem changes. Use a small set of pseudoterminal tests for browser navigation and confirmations. Both exercise the same public application boundary.
- **Good tests prove user outcomes.** Prefer a complete discover, explain, preview, apply, and re-inspect journey over tests of private helpers or expected call order. Do not mirror the implementation. Check untouched sources and unrelated settings as well as the intended result. Assertions should tolerate irrelevant ordering and formatting while checking stable, meaningful state.
- **Components covered through that boundary.** Exercise discovery and inventory, every agent adapter, availability resolution, bundle comparison, mutation planning and application, and both terminal interfaces. Add narrower tests only where a failure cannot reasonably be induced or diagnosed at this boundary, such as deterministic interruption during a filesystem write.
- **Fixtures represent the user's topology.** Include shared directory and manifest references, independent identical and diverged copies, colliding names, global and project settings, inherited overrides, managed plugins, inaccessible locations, malformed manifests, broken references, and cycles. Temporary environments must not fall back to the user's real home, settings, plugins, or installed skills.
- **Agent claims need independent validation.** Viewer fixture tests alone do not prove an agent enforces a setting. Before advertising a writer, validate the relevant operation and scope with the recorded agent version in a disposable home/project and inspect its effective discovery or native state. Check fresh-session versus reload requirements without running skill workloads. Where validation is unavailable, report that capability as unverified and do not enable its writer. The supported-version evidence accompanies the integration.
- **Terminal interaction tests stay focused.** Verify search and selection, reading, visible project context, an availability explanation, comparison, a scoped disable, an unsupported action, and delete confirmation/cancellation. Use semantic output and key sequences rather than extensive snapshots of decorative layout. Manually inspect the actual terminal rendering, including narrow windows and long paths, before considering the interface complete.
- **Offline verification is explicit.** Run core acceptance journeys without network access and without credentials. Establish that browsing and comparison never execute bundled scripts or alter the collection.
- **Reuse the public-executable harness.** At specification publication, no Skillglass production application or application test suite existed; the agent scheduler's tests covered separate workflow tooling. ALE-23 establishes the high-level fixture and executable harness for subsequent journeys; see its [verification guide](../verification/ale-23.md). No implementation tests were run while writing this specification.

The following outcomes are required acceptance coverage:

| Journey | Observable result |
| --- | --- |
| Inventory and reading | Known and selected locations are reported; full instructions can be read; missing or unreadable sources do not silently disappear from coverage reporting. |
| Identity and sharing | Several references resolve to one shared source; independent same-name copies remain distinct targets with a warning. |
| Comparison | Identical and diverged bundles are distinguished, including supporting resources, with zero writes. |
| Availability | The selected application and project show a supported state plus its source, rule, precedence, and uncertainty; an explicit-only state is not called disabled. |
| Per-agent disable and enable | A validated control affects the selected application and scope, retains content, preserves unrelated settings, and reports the resulting effective state. |
| Project override | A supported project choice differs from global behavior; removal restores inheritance; an unsupported operation makes no global change. |
| All-applications preflight | Unsupported or unverified consumers prevent a false universal-success result; a supported subset requires a distinct user selection. |
| Runtime failure and external edits | Stale previews refuse to overwrite changed state; multi-application runtime failure reports each outcome and does not hide partial completion. |
| Plugin scope | A managed per-skill limitation is visible; no cache edit or whole-plugin change occurs under an isolated skill operation. |
| CodeAF | Its inventory and precedence are represented independently; conversation detachment and another application's toggle do not count as persistent disable. |
| Permanent deletion | Cancel leaves all content intact; confirmation removes only the selected bundle; independent copies and external link targets survive; shared-reference effects and failures are reported without an undo claim. |
| Interactive and ordinary CLI parity | Both interfaces produce the same inventory, explanation, comparison, and control outcomes for the same context. |
| Offline use | Core journeys complete without accounts, network access, uploads, or writes during read-only operations. |

## Out of Scope

- Automatic cleanup, link repair, merging or consolidating independent copies, and automatic grouping of same-name copies into one identity.
- Recoverable deletion, trash management, deletion undo, and a general preview-history or undo system for other changes.
- Named task profiles, favorites, tags, reading notes, and usage-based recommendations.
- Online features, accounts, telemetry, cloud sync, a skill marketplace, or sending skill content to a model.
- Agent activity-log ingestion or claims that timestamps or installed files prove usage frequency or active context cost.
- Editing skill contents, installing skill packs, updating agents, operating agent sessions, or changing unrelated agent configuration.
- Whole-plugin management presented as a separate product workflow, direct mutation of managed caches, and filesystem workarounds that broaden a requested control scope.
- A guaranteed persistent CodeAF-only per-skill disable for the first release.
- A graphical desktop application or production browser UI, mandatory support beyond the current Mac, and a public launch or license decision.

## Further Notes

This specification synthesizes the user's accepted direction and the October 10, 2026 planning evidence. The testing boundary was explicitly confirmed for this spec. Implementation safeguards and observable acceptance criteria make those choices actionable; exact command names, terminal layout, and library choices were not fixed at specification publication.

The earlier terminal-workflow prototype and release-boundary planning tickets were still open when this spec was requested. This document does not claim that a prototype was built or reviewed. The implementation can begin with the interaction prototype and adapter validation described above. Findings that would remove an agreed capability or broaden a mutation's scope require an explicit scope decision rather than silently weakening this contract.

The prior capability research is a dated survey with validation gaps, especially project precedence, shared-reference matching, managed sources, and runtime refresh behavior. It does not prove every agent supports every requested control. No particular manifest-renaming fallback, quarantine mechanism, config schema, or persistence library was selected by the user.

The local product brief, glossary, decision tickets, and research remain provenance for this specification. This publication is a new specification issue, not a migration of the historical Wayfinder map or its child tickets. Linear is the live tracker for the spec; the local copy is reference material. Publishing the spec does not implement the application or change any installed skill or live agent setting.
