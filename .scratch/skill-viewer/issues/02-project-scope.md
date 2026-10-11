# Define what project-specific control means

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Should project support only discover skills stored in a project, or also provide project-specific choices about globally installed skills? How should the interface distinguish an inherited global setting from a project override, and what should happen when an agent cannot honor that override?

## Context

The user explicitly requested project-specific skills. Proposed direction: show project skills together with inherited global skills, and allow local overrides where the agent supports them. This recommendation is not yet accepted.

## User decisions

On October 10, 2026, the user chose "Project skills plus project overrides of global skills."

That product requirement is accepted. Agent-specific support, inheritance display, and the behavior when an override cannot be enforced remain open. The earlier recommendation is superseded by this accepted scope.

## Answer

The user requires both project-local skills and project overrides of globally available skills. The display and exact control behavior belong to [Design the terminal browsing and control workflow](13-terminal-workflow.md); supported operations must be grounded in each agent's actual capabilities. This resolution does not claim every agent supports every project override.
