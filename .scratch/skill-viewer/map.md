# Skill Viewer: product direction and constraints

Label: wayfinder:map
Status: active

## Destination

Agree on a product brief for a Go terminal tool that finds, reads, cleans up, and controls local agent skills. The brief should specify constraints, distinctive features, first-release boundaries, and observable success criteria before implementation begins.

## Notes

- Planning only. The user explicitly paused implementation to explore direction, constraints, and useful original features.
- Use the `wayfinder`, `grilling`, and `domain-modeling` skills. Use `research` for facts outside this project.
- Linear is the configured issue tracker. This local map and its tickets predate setup and have not been migrated. Preserve them as planning records; before continuing this effort in Linear, follow `docs/agents/issue-tracker.md` to migrate or reconcile the map, child tickets, answers, and dependencies.
- Tickets live in `issues/`. Read their `Status:`, `Type:`, and `Blocked by:` fields to find the next open, unblocked decision. A ticket is claimed before work and its answer is recorded under `## Answer` when resolved.
- Human decisions remain with the user. Recommendations and research findings are not accepted requirements.
- CodeGraph was explicitly skipped by the user.
- No application source exists. A Go toolchain was downloaded under `.tools/go/` before the planning pivot; it does not represent an implementation decision beyond the user's Go preference.
- The [direction review](../../.lavish/skill-viewer-direction.html) is a discussion aid. This map and its named tickets remain the planning record.
- The [product brief](product-brief.md) consolidates the agreed direction and proposed acceptance journeys. The next human decision is [Design the terminal browsing and control workflow](issues/13-terminal-workflow.md).

## Decisions so far

- [Define the purpose, audience, and control scope](issues/01-purpose-and-scope.md): browse, read, clean up, and control skills across agents and projects; personal setup first; interactive terminal UI plus ordinary commands.
- [Establish native skill controls and shared-source limits](issues/06-native-control-research.md): shared sources and differing native controls require explicit scope and capability reporting; project and installed-version behavior remain to validate before implementation.
- [Define deletion and recovery behavior](issues/03-deletion-and-recovery.md): deleting a skill permanently removes its content after confirmation; it is not recoverable.
- [Choose the first distinctive workflow](issues/05-distinctive-workflow.md): prioritize availability explanations and comparison of duplicate or diverged copies.
- [Define a skill's identity across copies](issues/07-skill-identity.md): independent same-name copies remain separate with a duplicate warning; shared references are a different concept.
- [Define what project-specific control means](issues/02-project-scope.md): include project-local skills and project overrides of global skills.
- [Choose supported agents and operating systems](issues/04-agent-coverage.md): cover the discussed agent set plus CodeAF, starting with the user's current Mac; report capability limits.
- [Set privacy and automation boundaries](issues/08-privacy-and-automation.md): the core works offline, with optional online features reserved for later and changes selected by the user.
- [Establish CodeAF skill controls](issues/10-codeaf-controls.md): CodeAF directly scans shared roots and has no persistent per-skill disabling in the inspected build.
- [Choose first-release cleanup actions](issues/11-cleanup-actions.md): detect problems, compare copies, and let the user choose disable or delete; no automatic repair or consolidation.
- [Decide whether CodeAF-specific disabling blocks the first release](issues/12-codeaf-release-requirement.md): CodeAF browsing with clearly unsupported controls is sufficient for the first release.

## Not yet specified

- Further agent-specific validation questions may emerge from the terminal workflow, especially around project precedence and managed plugins.
- Packaging and extension design will become specific enough to discuss after the intended user journeys are reviewed.

## Out of scope

- Application implementation, installing or deleting skills, changing live agent settings, and publishing anything during this planning effort.
- A public launch or license selection before the user chooses that direction.
- Automatic cleanup, link repair, and consolidation of independent skill copies in the first release, per [Choose first-release cleanup actions](issues/11-cleanup-actions.md).
