# Define deletion and recovery behavior

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

When the user deletes an entire skill, should it first move to recoverable storage or be removed permanently? What must the user see before deleting shared content, and how should later edits or source-manager updates affect recovery?

## Context

Deletion is explicitly requested. Proposed direction: recoverable deletion with an impact preview, and a separate explicit permanent-delete action. No deletion policy has been accepted.

## Answer

On October 10, 2026, the user chose **permanent deletion after confirmation**, rejecting recoverable-by-default deletion.

- Delete permanently removes the selected skill content after confirmation.
- The product must not describe this action as recoverable or offer a misleading undo.
- Disabling a skill remains distinct from deleting it; potential undo for availability changes is a separate feature decision.
- The exact content and affected references selected for deletion depend on [Define a skill's identity across copies](07-skill-identity.md).

The earlier recoverable-delete recommendation is superseded by this user decision. This is a product requirement, not an instruction to delete any existing skill during planning.
