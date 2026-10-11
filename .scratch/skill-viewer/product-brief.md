# Skill Viewer: product direction

Status: agreed product direction, with workflow and acceptance details proposed for review.
Date: October 10, 2026
Map: [Skill Viewer: product direction and constraints](map.md)

## Purpose

Make the user's local skill collection easy to find, read, understand, and control from a terminal. The tool should explain which agent can use which skill in a given project, reveal duplicate or diverged copies, and let the user deliberately disable or delete selected skills.

Go is the preferred implementation language. Build around the user's current Mac first, while keeping a possible public open-source release in mind. Provide both an interactive terminal browser and ordinary CLI commands.

## Accepted product choices

The linked decision tickets hold the detailed answers; this brief consolidates their product implications.

| Area | Direction | Decision |
| --- | --- | --- |
| Main jobs | Find and read skills, clean up the collection, control which agents use skills | [Purpose and scope](issues/01-purpose-and-scope.md) |
| Control scope | Choose individual agents or all affected agents; keep disable distinct from deletion | [Purpose and scope](issues/01-purpose-and-scope.md) |
| Projects | Include project-local skills and project overrides of global skills | [Project-specific control](issues/02-project-scope.md) |
| Deletion | Permanent deletion after confirmation, with no recovery or undo claim | [Deletion behavior](issues/03-deletion-and-recovery.md) |
| Coverage | Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF | [Agent coverage](issues/04-agent-coverage.md) |
| Distinctive workflows | Explain availability; compare duplicate or diverged copies | [Distinctive workflows](issues/05-distinctive-workflow.md) |
| Identity | Independent same-name sources remain separate with a duplicate warning | [Skill identity](issues/07-skill-identity.md) |
| Network | Offline core; optional online features later | [Privacy and automation](issues/08-privacy-and-automation.md) |
| Cleanup | Identify problems and compare copies; the user chooses disable or delete | [Cleanup actions](issues/11-cleanup-actions.md) |
| CodeAF | Include browsing; show unsupported controls clearly | [CodeAF release requirement](issues/12-codeaf-release-requirement.md) |

## Two distinctive workflows

### Explain availability

Select a skill, agent application, and project. The tool explains the source, the relevant global or project setting, any higher-priority copy, and whether the agent can use it. It distinguishes an observed fact from an inference or an unknown state.

For example, a skill may be disabled in Codex yet remain available to CodeAF, because CodeAF reads the shared source independently. A project copy may also take precedence over a global copy of the same name. These explanations are more useful than an unexplained green or red toggle.

### Compare duplicates and drift

Show the difference between several references to one shared source, separate identical copies, and independent copies whose contents differ. Keep independent sources distinguishable and expose their differences before the user chooses what to disable or delete.

Do not infer that a same-name copy is redundant, obsolete, or safe to delete. Comparison supports the user's decision; it does not make the decision automatically.

## First-release behavior to design and verify

These are proposed observable acceptance journeys, not claims that the application exists.

1. **Find and read:** launch the terminal browser, search the local collection, select a skill, and read its full instructions and source location. Use ordinary commands for the same inventory and inspection tasks.
2. **Understand sharing:** select a shared skill and see its source plus the discovered references to it. Independent same-name copies remain separately selectable with a warning.
3. **Explain agent and project state:** choose an agent and project, then inspect local skills and inherited global skills. Explain the rule or source behind the state, and show uncertainty where the agent's effective state has not been verified.
4. **Disable within the selected scope:** preview the supported action, apply it to the chosen agent or project, and verify the resulting configuration. Do not silently change global or shared state to imitate an unsupported narrower operation.
5. **Disable for all affected agents:** show the exact selected source and discovered consumers. Report unsupported or unverified consumers instead of claiming universal success. Independent same-name copies are not silently included.
6. **Compare and clean up:** inspect duplicate or diverged copies, then choose a supported disable operation or permanent deletion. Link repair, copy consolidation, and automatic cleanup are outside the first release.
7. **Delete deliberately:** make the selected content and known affected references clear before confirmation. Cancel makes no change; confirm permanently removes the selected content. Do not advertise undo for deletion.
8. **Handle CodeAF honestly:** show CodeAF's discovered skills and precedence. Mark persistent CodeAF-only disabling unsupported for the inspected build; detaching a skill from a conversation does not count as disabling it.
9. **Work offline:** perform core discovery, reading, comparison, and supported controls without an account or network service.

## Engineering constraints implied by the chosen behavior

- Skill files and existing agent settings are the starting source of truth. The viewer must preserve unrelated settings and report externally changed state.
- Agent discovery rules differ. Folder names alone cannot establish effective availability, and the existence of a cached plugin does not prove it is enabled.
- Show controls according to the agent's validated capabilities. Some plugins require whole-plugin controls, which must never be presented as an isolated per-skill toggle.
- Distinguish the configuration change from when a running agent will observe it. Show reload or restart requirements where applicable.
- Permanent deletion can affect references in other locations. Known effects must be visible, and incomplete discovery must not be represented as a complete machine-wide guarantee.
- The current scan covers known roots, not every possible skill file on the disk. Project discovery and custom-root selection belong in the workflow design.

## Outside the first release

- Automatic cleanup, merging independent copies, or repairing links.
- Named task profiles, favorites, tags, and reading notes unless the user later promotes them.
- Online recommendations, cloud sync, accounts, and a public skill marketplace.
- Usage analytics, agent activity-log ingestion, or claims that filesystem timestamps indicate skill usage.
- Editing skill contents, installing skill packs, or managing unrelated agent configuration.
- A production graphical app or browser UI. The current HTML page is only a planning review aid.

## What remains before implementation

The next planning ticket is [Design the terminal browsing and control workflow](issues/13-terminal-workflow.md). It should make project selection, discovery boundaries, comparisons, unsupported controls, and deletion confirmation concrete enough to review. Then [Set the first release's boundaries and success criteria](issues/09-first-release-boundary.md) can freeze the implementation specification.

Each agent integration still needs validation against the installed version. In particular, Gemini has no installed CLI in the inspected setup, and CodeAF lacks a persistent per-skill disable control in the inspected build. These limits are recorded rather than hidden behind a generic toggle.

## Evidence

- [Agent skill discovery and control research](research/agent-skill-controls.md): seven-root inventory, shared sources, native settings, plugin limits, and outstanding validation.
- [CodeAF skill discovery and controls](research/codeaf-skill-controls.md): installed build, discovery precedence, project behavior, and attachment-versus-disable distinction.

No production application code, live agent settings, or installed skills were changed during this planning work.
