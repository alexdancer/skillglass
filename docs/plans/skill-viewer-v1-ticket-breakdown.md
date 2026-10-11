# Skill Viewer v1: published ticket breakdown

Parent: [Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

Status: approved and published. All 21 Linear sub-issues and 39 native blocking links were verified. Each issue is in Backlog with the ready-for-agent label. This document is a local reference; Linear holds live status.

Each production ticket includes an end-to-end CLI and interactive-terminal journey plus public-executable tests in isolated temporary homes/projects. Native writers also require version-specific validation. The prototype ticket is deliberately throwaway. No prefactoring is needed because application code does not exist.

Numbers below preserve the approved review order. Follow the linked Linear issues for current status and native dependencies. Publication left the parent specification content and status unchanged.

## Published breakdown

1. **[Prototype the terminal journeys](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys)** (ALE-22)
   - **Blocked by:** None (can start immediately)
   - **What it delivers:** A runnable throwaway terminal interaction prototype makes source selection, project context, explanation, comparison, scoped disable, and delete confirmation concrete before the production interface is built.

2. **[Browse and read a selected skill collection](https://linear.app/alex-dancer/issue/ALE-23/browse-and-read-a-selected-skill-collection)** (ALE-23)
   - **Blocked by:** [Prototype the terminal journeys](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) (ALE-22)
   - **What it delivers:** The first Go executable can search and read skill bundles in explicitly selected roots through ordinary commands and the interactive terminal browser.

3. **[Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems)** (ALE-24)
   - **Blocked by:** [Browse and read a selected skill collection](https://linear.app/alex-dancer/issue/ALE-23/browse-and-read-a-selected-skill-collection) (ALE-23)
   - **What it delivers:** Both terminal interfaces distinguish shared references from independent copies and show a searchable, refreshable inventory of discovery problems.

4. **[Compare independent skill bundles](https://linear.app/alex-dancer/issue/ALE-25/compare-independent-skill-bundles)** (ALE-25)
   - **Blocked by:** [Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems) (ALE-24)
   - **What it delivers:** A user can select two independent copies and inspect identical or diverged instructions and resources through CLI output and the terminal browser.

5. **[Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills)** (ALE-26)
   - **Blocked by:** [Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems) (ALE-24)
   - **What it delivers:** A user can select Codex and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

6. **[Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally)** (ALE-27)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can preview, apply, and verify a global Codex enable/disable choice through both interfaces; this first write journey establishes the reusable mutation interaction.

7. **[Control Codex skills within one project](https://linear.app/alex-dancer/issue/ALE-28/control-codex-skills-within-one-project)** (ALE-28)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
   - **What it delivers:** A user can apply a validated project override for a global Codex skill, inspect the resulting project state, and remove the override to inherit again.

8. **[Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills)** (ALE-29)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can select Claude Code and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

9. **[Enable and disable Claude Code skills](https://linear.app/alex-dancer/issue/ALE-30/enable-and-disable-claude-code-skills)** (ALE-30)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills) (ALE-29)
   - **What it delivers:** A user can preview, apply, and verify supported Claude Code enable/disable choices and project overrides without changing other applications or unrelated settings.

10. **[Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills)** (ALE-31)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can select Hermes and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

11. **[Enable and disable Hermes skills](https://linear.app/alex-dancer/issue/ALE-32/enable-and-disable-hermes-skills)** (ALE-32)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills) (ALE-31)
   - **What it delivers:** A user can preview, apply, and verify supported Hermes enable/disable choices and project overrides without changing other applications or unrelated settings.

12. **[Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills)** (ALE-33)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can select Pi and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

13. **[Enable and disable Pi skills](https://linear.app/alex-dancer/issue/ALE-34/enable-and-disable-pi-skills)** (ALE-34)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills) (ALE-33)
   - **What it delivers:** A user can preview, apply, and verify supported Pi enable/disable choices and project overrides without changing other applications or unrelated settings.

14. **[Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills)** (ALE-35)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can select OpenCode and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

15. **[Enable and disable OpenCode skills](https://linear.app/alex-dancer/issue/ALE-36/enable-and-disable-opencode-skills)** (ALE-36)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills) (ALE-35)
   - **What it delivers:** A user can preview, apply, and verify supported OpenCode enable/disable choices and project overrides without changing other applications or unrelated settings.

16. **[Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills)** (ALE-37)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** A user can select Gemini CLI and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

17. **[Enable and disable Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-38/enable-and-disable-gemini-cli-skills)** (ALE-38)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills) (ALE-37)
   - **What it delivers:** A user can preview, apply, and verify supported Gemini CLI enable/disable choices and project overrides without changing other applications or unrelated settings.

18. **[Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills)** (ALE-39)
   - **Blocked by:** [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)
   - **What it delivers:** CodeAF appears in the same inventory and explanation workflows, with its own discovery, precedence, plugin provenance, and explicit persistent-control limitations.

19. **[Control a skill across the first supported agents](https://linear.app/alex-dancer/issue/ALE-40/control-a-skill-across-the-first-supported-agents)** (ALE-40)
   - **Blocked by:** [Enable and disable Claude Code skills](https://linear.app/alex-dancer/issue/ALE-30/enable-and-disable-claude-code-skills) (ALE-30), [Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills) (ALE-39)
   - **What it delivers:** A user can use the multi-agent workflow with Codex, Claude Code, and CodeAF: preview a requested group, apply a separately selected supported subset, and inspect per-application results. Other completed adapters join through the same contract.

20. **[Permanently delete a selected skill bundle](https://linear.app/alex-dancer/issue/ALE-41/permanently-delete-a-selected-skill-bundle)** (ALE-41)
   - **Blocked by:** [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27), [Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills) (ALE-29), [Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills) (ALE-31), [Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills) (ALE-33), [Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills) (ALE-35), [Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills) (ALE-37), [Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills) (ALE-39)
   - **What it delivers:** A user can preview the exact selected user-owned bundle and known affected references, then confirm permanent deletion or cancel without change.

21. **[Run the complete offline workflow from a Mac build](https://linear.app/alex-dancer/issue/ALE-42/run-the-complete-offline-workflow-from-a-mac-build)** (ALE-42)
   - **Blocked by:** [Compare independent skill bundles](https://linear.app/alex-dancer/issue/ALE-25/compare-independent-skill-bundles) (ALE-25), [Control Codex skills within one project](https://linear.app/alex-dancer/issue/ALE-28/control-codex-skills-within-one-project) (ALE-28), [Enable and disable Hermes skills](https://linear.app/alex-dancer/issue/ALE-32/enable-and-disable-hermes-skills) (ALE-32), [Enable and disable Pi skills](https://linear.app/alex-dancer/issue/ALE-34/enable-and-disable-pi-skills) (ALE-34), [Enable and disable OpenCode skills](https://linear.app/alex-dancer/issue/ALE-36/enable-and-disable-opencode-skills) (ALE-36), [Enable and disable Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-38/enable-and-disable-gemini-cli-skills) (ALE-38), [Control a skill across the first supported agents](https://linear.app/alex-dancer/issue/ALE-40/control-a-skill-across-the-first-supported-agents) (ALE-40), [Permanently delete a selected skill bundle](https://linear.app/alex-dancer/issue/ALE-41/permanently-delete-a-selected-skill-bundle) (ALE-41)
   - **What it delivers:** A user can build or install the local Mac executable and complete the first-release workflows offline with all seven integrations and an accurate support summary.

## Dependency rationale

The prototype sets the terminal interaction contract before the first production browser. Source identity unlocks comparison and the first Codex read journey. That first integration establishes one working availability-explanation contract, after which the other discovery integrations can proceed in parallel. The first global Codex writer delivers the reusable preview/apply/revalidation interaction needed by subsequent writers and deletion. Project controls are a separate Codex slice. These dependencies represent reused working behavior, not a preference for one agent.

The multi-agent workflow starts once two working writers and the CodeAF limitation exist; it does not wait unnecessarily for every adapter. Deletion waits for all discovery integrations so known consumers and managed ownership can inform the preview. The final Mac-artifact journey waits for the remaining integration leaves and verifies that every completed adapter participates in aggregate controls. Basic tests, isolation, safe writes, and honest unsupported states belong to every relevant slice.

## Published ticket contents

### 1: [Prototype the terminal journeys](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) (ALE-22)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A runnable throwaway terminal interaction prototype makes source selection, project context, explanation, comparison, scoped disable, and delete confirmation concrete before the production interface is built.

#### Acceptance criteria

- [ ] Demonstrate a shared source, independent same-name copies, a project override, a managed plugin, and an unsupported CodeAF control using clearly identified fixture data.
- [ ] Walk from finding and reading a skill to availability explanation, comparison, mutation preview, cancellation, and target-specific permanent-delete confirmation.
- [ ] Keep source, agent application, project, and scope visible; distinguish disable from delete and uncertainty from verified state.
- [ ] Record the chosen navigation and ordinary-command interaction contracts, including noninteractive confirmation; inspect narrow terminals and long paths.
- [ ] Do not touch installed skills or live settings. Record unresolved implementation facts rather than presenting mock behavior as agent validation.

#### Blocked by

None (can start immediately).

### 2: [Browse and read a selected skill collection](https://linear.app/alex-dancer/issue/ALE-23/browse-and-read-a-selected-skill-collection) (ALE-23)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

The first Go executable can search and read skill bundles in explicitly selected roots through ordinary commands and the interactive terminal browser.

#### Acceptance criteria

- [ ] Build and run a Go executable on the current Mac; include repeatable build and test instructions.
- [ ] Select a project context and custom discovery locations explicitly, show the effective search scope, and list names, descriptions, skill metadata, source locations, full instructions, and supporting resource inventory.
- [ ] Search names and descriptions; select and read a skill with keyboard navigation and ordinary commands, using the same behavior core.
- [ ] Handle an empty collection and missing or unreadable locations with useful output; avoid default machine-wide scans and never execute bundled instructions or scripts.
- [ ] Introduce the reusable public-executable fixture harness with isolated temporary homes and projects, plus a minimal pseudoterminal browsing test. Tests must never fall back to the real home.
- [ ] Demonstrate the journey offline and verify browsing makes no changes to sources or agent settings. Availability remains unknown until an agent integration supplies evidence.

#### Blocked by

- [Prototype the terminal journeys](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) (ALE-22)

### 3: [Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems) (ALE-24)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

Both terminal interfaces distinguish shared references from independent copies and show a searchable, refreshable inventory of discovery problems.

#### Acceptance criteria

- [ ] Retain original discovery locations and resolved source identity; group only proven shared references and keep independent copies separately selectable even when names or contents match.
- [ ] Show duplicate-name warnings, known references, and explicit identity uncertainty; sharing only a manifest must not erase differences in the rest of a bundle.
- [ ] Report broken references, cycles, malformed manifests, unreadable entries, and incomplete discovery without discarding the usable inventory.
- [ ] Filter by reported problems and source/project context, refresh explicitly, and retain a stable unambiguous selected source for subsequent operations.
- [ ] Retain ownership and provenance supplied by adapters; unknown ownership stays unknown rather than being treated as user-owned.
- [ ] Executable and terminal tests cover directory and manifest symlinks, independent copies, cycles, malformed inputs, missing sources, refresh after external edits, and zero writes.

#### Blocked by

- [Browse and read a selected skill collection](https://linear.app/alex-dancer/issue/ALE-23/browse-and-read-a-selected-skill-collection) (ALE-23)

### 4: [Compare independent skill bundles](https://linear.app/alex-dancer/issue/ALE-25/compare-independent-skill-bundles) (ALE-25)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select two independent copies and inspect identical or diverged instructions and resources through CLI output and the terminal browser.

#### Acceptance criteria

- [ ] Compare selected sources without collapsing same-name or equal-content independent copies into one identity.
- [ ] Show readable text differences and added, removed, or changed supporting resources; identify changed binary resources without rendering them as text.
- [ ] Explain when selections are shared references to one source rather than duplicate copies.
- [ ] Report unreadable, missing, or externally linked resources and incomplete comparison coverage; handle a source changing during comparison without claiming a complete match.
- [ ] Executable and terminal tests verify meaningful bundle differences and confirm that comparison never writes, executes content, repairs references, or consolidates copies.

#### Blocked by

- [Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems) (ALE-24)

### 5: [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select Codex and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Establish the shared availability explanation contract with this first real integration: discovery, configured permission, source precedence, observed/inferred/unknown evidence, capability scope, and session reload state remain distinct.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded Codex version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Resolve the researched ambiguity around path targeting and shared-reference matching against the selected version; report configured permission separately from what an existing session has observed.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Distinguish sources and surface collection problems](https://linear.app/alex-dancer/issue/ALE-24/distinguish-sources-and-surface-collection-problems) (ALE-24)

### 6: [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify a global Codex enable/disable choice through both interfaces; this first write journey establishes the reusable mutation interaction.

#### Acceptance criteria

- [ ] Deliver the first reusable end-to-end mutation workflow: select a source and scope, preview exact changes, apply, and re-inspect persisted and effective state.
- [ ] Validate native Codex enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Implement validated global controls only in this slice. Keep project overrides visibly unavailable until the separate project-control slice supplies validated support; do not broaden a project request to global scope.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Document the validated target representation and effective scope. Enabling must explain a higher-priority rule or source that still prevents use.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, and write failures. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 7: [Control Codex skills within one project](https://linear.app/alex-dancer/issue/ALE-28/control-codex-skills-within-one-project) (ALE-28)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can apply a validated project override for a global Codex skill, inspect the resulting project state, and remove the override to inherit again.

#### Acceptance criteria

- [ ] Use the existing source selection, project context, explanation, preview/apply, safe-write, and revalidation workflows rather than duplicating them.
- [ ] Validate Codex project configuration precedence and exact source matching against the recorded version before enabling a project writer.
- [ ] Implement supported project enable/disable and override removal; explain inherited state and higher-priority settings that still prevent availability.
- [ ] If the requested scope cannot be enforced, provide a specific unsupported reason and make no global or shared-source change. An unavailable native scope is an evidenced limitation, not an invented fallback.
- [ ] Preserve unrelated project settings and all other projects. Cover source/config changes between preview and apply, cancellation, write failure, and reload requirements.
- [ ] Public-executable and focused terminal tests verify an overridden project versus a second inheriting project, removal of the override, scope refusal, and isolation from the real home.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)

### 8: [Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills) (ALE-29)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select Claude Code and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded Claude Code version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Distinguish ordinary skills from plugin-owned skills and show project-local versus global settings without equating reduced model invocation with full disable.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 9: [Enable and disable Claude Code skills](https://linear.app/alex-dancer/issue/ALE-30/enable-and-disable-claude-code-skills) (ALE-30)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify supported Claude Code enable/disable choices and project overrides without changing other applications or unrelated settings.

#### Acceptance criteria

- [ ] Reuse the established mutation workflow while keeping native settings and capability decisions specific to this agent.
- [ ] Validate native Claude Code enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Support validated global controls, project overrides, and removal of overrides to restore inheritance; show unsupported or unverified scopes with reasons and perform no broader fallback.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Validate name-collision behavior and ordinary-skill versus plugin restrictions; a whole-plugin action must never be executed as an isolated skill toggle.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, write failures, and override removal. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills) (ALE-29)

### 10: [Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills) (ALE-31)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select Hermes and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded Hermes version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Resolve the active profile, platform-specific settings, external directories, and plugin namespaces for the inspected context; do not assume the default profile is always active.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 11: [Enable and disable Hermes skills](https://linear.app/alex-dancer/issue/ALE-32/enable-and-disable-hermes-skills) (ALE-32)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify supported Hermes enable/disable choices and project overrides without changing other applications or unrelated settings.

#### Acceptance criteria

- [ ] Reuse the established mutation workflow while keeping native settings and capability decisions specific to this agent.
- [ ] Validate native Hermes enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Support validated global controls, project overrides, and removal of overrides to restore inheritance; show unsupported or unverified scopes with reasons and perform no broader fallback.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Test the interaction of applicable profile/platform rules and native disabled state; removing one setting must not falsely claim enablement while another effective rule still denies use.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, write failures, and override removal. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills) (ALE-31)

### 12: [Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills) (ALE-33)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select Pi and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded Pi version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Explain configured resource inclusions/exclusions and their project precedence, including symlink references and independently selected resource roots.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 13: [Enable and disable Pi skills](https://linear.app/alex-dancer/issue/ALE-34/enable-and-disable-pi-skills) (ALE-34)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify supported Pi enable/disable choices and project overrides without changing other applications or unrelated settings.

#### Acceptance criteria

- [ ] Reuse the established mutation workflow while keeping native settings and capability decisions specific to this agent.
- [ ] Validate native Pi enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Support validated global controls, project overrides, and removal of overrides to restore inheritance; show unsupported or unverified scopes with reasons and perform no broader fallback.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Validate exact-target and pattern exclusions, symlink matching, and scope precedence while preserving existing configured resources and exclusions.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, write failures, and override removal. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills) (ALE-33)

### 14: [Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills) (ALE-35)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select OpenCode and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded OpenCode version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Explain relevant permission rules and contextual overrides while preserving allow, ask, and deny distinctions in the selected application/project context.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 15: [Enable and disable OpenCode skills](https://linear.app/alex-dancer/issue/ALE-36/enable-and-disable-opencode-skills) (ALE-36)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify supported OpenCode enable/disable choices and project overrides without changing other applications or unrelated settings.

#### Acceptance criteria

- [ ] Reuse the established mutation workflow while keeping native settings and capability decisions specific to this agent.
- [ ] Validate native OpenCode enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Support validated global controls, project overrides, and removal of overrides to restore inheritance; show unsupported or unverified scopes with reasons and perform no broader fallback.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Validate native configuration format, permission-pattern matching, and contextual overrides; preserve unrelated permission rules and meaningful ask state.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, write failures, and override removal. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills) (ALE-35)

### 16: [Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills) (ALE-37)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can select Gemini CLI and a project, browse its known global and project skills, and see the source and rule behind each reported availability state.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate discovery, configuration precedence, project boundaries, configured roots, and ownership against a recorded Gemini CLI version; identify documented assumptions and unverified cases.
- [ ] Show the effective search scope, inherited global skills, project skills, winning and shadowed definitions, and shared references without merging independent copies.
- [ ] Provide availability explanations and filters in ordinary commands and the terminal browser; distinguish observed, inferred, and unknown state and preserve applicable explicit-only or ask states.
- [ ] Represent plugin provenance and capability limits, exclude stale or inactive managed sources from effective availability as appropriate, and do not treat a cache or folder alone as proof of enablement.
- [ ] Use a suitable disposable validation environment because the planning survey found no installed Gemini CLI on the user's Mac; do not require modifying the user's live setup to validate discovery.
- [ ] Public-executable fixtures and a focused terminal interaction verify global/project precedence, configured native states, same-name conflicts, sharing, missing inputs, and read-only offline behavior.
- [ ] Record version-specific effective-state validation from a disposable home/project using the real agent where available; label gaps explicitly rather than enabling unverified writers.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 17: [Enable and disable Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-38/enable-and-disable-gemini-cli-skills) (ALE-38)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview, apply, and verify supported Gemini CLI enable/disable choices and project overrides without changing other applications or unrelated settings.

#### Acceptance criteria

- [ ] Reuse the established mutation workflow while keeping native settings and capability decisions specific to this agent.
- [ ] Validate native Gemini CLI enable/disable in a disposable environment with a recorded version before exposing a writer. A blanket browsing-only implementation does not satisfy this ticket.
- [ ] Support validated global controls, project overrides, and removal of overrides to restore inheritance; show unsupported or unverified scopes with reasons and perform no broader fallback.
- [ ] Refuse ambiguous targets and name-based or plugin-level actions that would affect additional independent copies or broaden the selected scope; do not edit shared content or managed caches.
- [ ] Cancellation makes no change. Recheck source identity and configuration before applying; invalidate stale previews, preserve unrelated settings, and verify persisted results with safe writes.
- [ ] Expose CLI and terminal outcomes for applied, refused, unsupported, and failed requests; report non-success exit status when the requested result was not achieved and distinguish reload/restart requirements.
- [ ] Obtain version-specific validation in an isolated environment, including scope merging and restart requirements. Missing local installation is not permission to ship an untested writer.
- [ ] Executable tests assert actual filesystem changes and preservation of unrelated state, including external edits, higher-priority settings, duplicate-name scope conflicts, write failures, and override removal. A focused terminal test covers confirmation and cancellation.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills) (ALE-37)

### 18: [Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills) (ALE-39)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

CodeAF appears in the same inventory and explanation workflows, with its own discovery, precedence, plugin provenance, and explicit persistent-control limitations.

#### Acceptance criteria

- [ ] Reuse the first availability explanation contract and terminal presentation established by the Codex read journey, while keeping discovery and native precedence specific to this application.
- [ ] Validate the inspected build identifier and independent discovery rules in a disposable home/project; retain uncertainty where installed behavior differs from source or documentation.
- [ ] Show the winning and shadowed definitions across CodeAF-discovered roots, the explicit project, and the relevant home/profile environment.
- [ ] Reflect CodeAF plugin ownership and enablement rules rather than inheriting another agent's ordinary per-skill availability wholesale.
- [ ] Explain persistent CodeAF-only per-skill disabling as unsupported for the inspected behavior. Conversation detachment and another agent's setting must not be presented as disabling CodeAF.
- [ ] Expose consumers and unsupported-control reasons to aggregate previews; never mutate shared sources or settings as a CodeAF-only workaround.
- [ ] CLI and terminal tests cover project precedence, same-name conflicts, shared roots, plugin state, attachment-versus-disable distinctions, and read-only offline operation.

#### Blocked by

- [Browse and explain Codex skills](https://linear.app/alex-dancer/issue/ALE-26/browse-and-explain-codex-skills) (ALE-26)

### 19: [Control a skill across the first supported agents](https://linear.app/alex-dancer/issue/ALE-40/control-a-skill-across-the-first-supported-agents) (ALE-40)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can use the multi-agent workflow with Codex, Claude Code, and CodeAF: preview a requested group, apply a separately selected supported subset, and inspect per-application results. Other completed adapters join through the same contract.

#### Acceptance criteria

- [ ] Use the completed Codex and Claude Code controls plus the CodeAF limitation to demonstrate mixed-consumer behavior through ordinary commands and the terminal browser.
- [ ] Build the consumer set from the selected source and discovered references, preserve independent same-name copies, show project scope and incomplete coverage, and never claim machine-wide completeness.
- [ ] If a requested consumer is unsupported or unverified, leave the aggregate request unapplied; applying a supported subset requires a new explicit selection.
- [ ] For a fully supported selection, revalidate and apply through each adapter with preserved unrelated settings and truthful per-application verification; do not promise cross-file atomicity.
- [ ] On runtime failure, report what succeeded, failed, or was not attempted and return non-success for an unmet request. Show each application's reload requirement.
- [ ] Discover subsequent completed adapters through the same capability contract rather than hardcoding the initial two writable applications.
- [ ] Executable and terminal tests cover full success, preflight refusal, explicit subset selection, cancellation, stale state, independent copies, incomplete discovery, and partial runtime failure.

#### Blocked by

- [Enable and disable Claude Code skills](https://linear.app/alex-dancer/issue/ALE-30/enable-and-disable-claude-code-skills) (ALE-30)
- [Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills) (ALE-39)

### 20: [Permanently delete a selected skill bundle](https://linear.app/alex-dancer/issue/ALE-41/permanently-delete-a-selected-skill-bundle) (ALE-41)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can preview the exact selected user-owned bundle and known affected references, then confirm permanent deletion or cancel without change.

#### Acceptance criteria

- [ ] Reuse the established source selection and preview/revalidation interaction, with delete visibly distinct from disable in both interfaces.
- [ ] Include provenance and known affected consumers from all seven completed discovery integrations, including CodeAF. Disclose any remaining inaccessible or unverified discovery scope.
- [ ] Show exact content, known consumers/references, ownership, and discovery limits; refuse managed content, uncertain ownership, or an unsafe boundary encompassing a broader root.
- [ ] Require target-specific confirmation; noninteractive invocation with missing confirmation fails safely. Cancellation makes no change.
- [ ] Remove only the selected bundle, retain independent copies and external targets of links inside the bundle, and do not treat unlinking one reference as deleting shared content.
- [ ] Invalidate stale previews and report actual removal results, including partial failure. Never promise trash recovery or undo, and do not repair remaining references automatically.
- [ ] Executable fixtures and focused terminal tests cover standalone and shared sources, independent copies, manifest and directory links, external link targets, ownership refusal, cancellation, and concurrent changes. Use a narrower deterministic fault-injection test for interruption during removal when the public executable boundary cannot induce it reliably.

#### Blocked by

- [Enable and disable Codex globally](https://linear.app/alex-dancer/issue/ALE-27/enable-and-disable-codex-globally) (ALE-27)
- [Browse and explain Claude Code skills](https://linear.app/alex-dancer/issue/ALE-29/browse-and-explain-claude-code-skills) (ALE-29)
- [Browse and explain Hermes skills](https://linear.app/alex-dancer/issue/ALE-31/browse-and-explain-hermes-skills) (ALE-31)
- [Browse and explain Pi skills](https://linear.app/alex-dancer/issue/ALE-33/browse-and-explain-pi-skills) (ALE-33)
- [Browse and explain OpenCode skills](https://linear.app/alex-dancer/issue/ALE-35/browse-and-explain-opencode-skills) (ALE-35)
- [Browse and explain Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-37/browse-and-explain-gemini-cli-skills) (ALE-37)
- [Browse and explain CodeAF skills](https://linear.app/alex-dancer/issue/ALE-39/browse-and-explain-codeaf-skills) (ALE-39)

### 21: [Run the complete offline workflow from a Mac build](https://linear.app/alex-dancer/issue/ALE-42/run-the-complete-offline-workflow-from-a-mac-build) (ALE-42)

#### Parent

[Build Skill Viewer v1: browse, explain, compare, and control local skills](https://linear.app/alex-dancer/issue/ALE-21/build-skill-viewer-v1-browse-explain-compare-and-control-local-skills)

#### What to build

A user can build or install the local Mac executable and complete the first-release workflows offline with all seven integrations and an accurate support summary.

#### Acceptance criteria

- [ ] Produce a repeatable local Mac build and concise installation/run instructions without requiring a public release, signing service, account, or live network service for runtime.
- [ ] Exercise the built artifact from a clean disposable setup through discovery, reading, explanation, comparison, project controls, aggregate control, and permanent-delete confirmation in both interfaces.
- [ ] Verify all seven integrations appear and aggregate controls include every completed adapter, with version/scope evidence and CodeAF's accepted exception.
- [ ] Run the cross-agent cases that cannot be covered by a single adapter: shared consumers, independent same-name sources, plugin ownership, project/global precedence, incomplete coverage, and mixed support.
- [ ] Verify no core journey uploads content, executes bundled scripts, ingests logs, or mutates anything during read-only use; run with network access blocked and no credentials.
- [ ] Inspect actual terminal rendering at normal and narrow sizes with long paths; repair defects and rerun affected checks. Report any unverified capability or release requirement honestly.
- [ ] Keep behavior tests in each earlier ticket; this ticket proves the delivered artifact and cross-integration journey rather than postponing basic testing or safety to the end.

#### Blocked by

- [Compare independent skill bundles](https://linear.app/alex-dancer/issue/ALE-25/compare-independent-skill-bundles) (ALE-25)
- [Control Codex skills within one project](https://linear.app/alex-dancer/issue/ALE-28/control-codex-skills-within-one-project) (ALE-28)
- [Enable and disable Hermes skills](https://linear.app/alex-dancer/issue/ALE-32/enable-and-disable-hermes-skills) (ALE-32)
- [Enable and disable Pi skills](https://linear.app/alex-dancer/issue/ALE-34/enable-and-disable-pi-skills) (ALE-34)
- [Enable and disable OpenCode skills](https://linear.app/alex-dancer/issue/ALE-36/enable-and-disable-opencode-skills) (ALE-36)
- [Enable and disable Gemini CLI skills](https://linear.app/alex-dancer/issue/ALE-38/enable-and-disable-gemini-cli-skills) (ALE-38)
- [Control a skill across the first supported agents](https://linear.app/alex-dancer/issue/ALE-40/control-a-skill-across-the-first-supported-agents) (ALE-40)
- [Permanently delete a selected skill bundle](https://linear.app/alex-dancer/issue/ALE-41/permanently-delete-a-selected-skill-bundle) (ALE-41)

## Publication verification

- 21 separate child issues under the parent specification.
- 39 native blocking links, checked in both directions.
- Approved descriptions, Backlog status, ready-for-agent labels, team, and project verified.
- Parent specification content and status preserved.
- Initial frontier: [Prototype the terminal journeys](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) (ALE-22).
- This publication does not start implementation or migrate the historical Wayfinder records.
