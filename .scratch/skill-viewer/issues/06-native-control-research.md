# Establish native skill controls and shared-source limits

Type: research
Status: resolved
Claimed by: disable_semantics research agent
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Which discovery paths and native availability controls exist in the local agent applications, and what limits do shared files, project settings, and managed plugins place on per-agent and global operations?

## Context

The findings will be captured in [Agent skill controls](../research/agent-skill-controls.md). This is a factual investigation, not authorization to modify any installed skill or agent configuration. There is no Git repository for the skill's suggested throwaway research branch, so research is recorded locally.

## Answer

[Agent skill controls](../research/agent-skill-controls.md) records the October 10, 2026 filesystem observations and primary-source research. The main agent independently confirmed the 1,429 occurrences, 548 canonical files, and 260 shared sources, and checked official Codex, Claude, and OpenCode controls.

The installed applications expose different native controls. Shared sources make host-only file moves unreliable, and managed plugins may require plugin-level controls. Existing settings must be preserved. Folder presence alone does not establish effective availability.

Project precedence, exact installed-version behavior, symlink matching, and reload semantics still require targeted validation before implementing an adapter. This research resolves the initial capability survey, not those implementation checks, and does not select the product's control mechanism.
