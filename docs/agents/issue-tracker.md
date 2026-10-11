# Issue tracker: Linear

Issues for this project live in Linear.

- Workspace: [Alex Dancer](https://linear.app/alex-dancer)
- Team: **Alex Dancer**, key **ALE**, ID `2fc2779d-b7e3-44f7-9d57-57b42e506dba`
- Project: [Skill Viewer](https://linear.app/alex-dancer/project/skill-viewer-58706f175bed)
- Project UUID: `98ccef31-35e9-422e-80a3-a57774c3b9ef`

## Access and conventions

Use the connected Linear tools when available, or the authenticated Linear API through the Linear skill. Never store credentials in this repository.

- Create one Linear issue per ticket in the team and project above. Search for an existing matching issue before creating one.
- Use Markdown descriptions and link related issues and supporting documents.
- Refer to issues by their linked titles. Retain the Linear identifier and URL in context pointers.
- Fetch tickets by their Linear issue URL, identifier, or UUID. Follow native parent and dependency relations when gathering context.
- Triage labels follow `triage-labels.md`. Reuse the existing labels; do not create duplicate names.
- Labels and workflow states have different roles. New unclaimed work normally starts in **Backlog**. Claim active work by assigning it to the person driving it and moving it to **In Progress**. Use **Done** only when the work or decision is resolved, and **Canceled** for work deliberately ruled out. Verify current team states before writing.
- Local Markdown may hold drafts, research, and source documents. New canonical tickets belong in Linear, not parallel files under `.scratch/`.

## When a skill says "publish to the issue tracker"

Create or update the relevant Linear issue in Skill Viewer. For a multi-ticket effort, use a parent issue with separate child issues; do not combine every ticket into one description. Preserve unrelated labels, relationships, and fields when updating existing records.

## When a skill says "fetch the relevant ticket"

Read the referenced Linear issue, including its description, current status, labels, parent, blockers, and relevant comments. Never infer live status from a local copy.

## Wayfinding operations

- **Map:** one parent issue in Skill Viewer labelled `wayfinder:map`. Its description contains Destination, Notes, Decisions so far, Not yet specified, and Out of scope.
- **Child ticket:** one native sub-issue of the map for each decision, labelled `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, or `wayfinder:task`. Reuse labels when present and create only missing type labels when needed.
- **Blocking:** use Linear's native blocked-by and blocks relationships. Create tickets first, then wire dependencies using their returned identifiers. Body text alone is not the dependency record.
- **Frontier:** the map's open, unassigned child issues whose blocking issues are all closed. Take the oldest eligible issue first unless the user names one.
- **Claim:** assign the issue to the person driving the decision before work, then set In Progress. Do not take over another assignee's claim.
- **Resolve:** add the answer as a resolution comment, set Done, then add a linked title and one-line gist to the map's Decisions so far. Keep the detailed answer in the child issue.
- **Out of scope:** cancel the child issue and link it with a brief reason in the map's Out of scope section.

## Existing local planning records

The existing map, decision tickets, research, and brief under `.scratch/skill-viewer/` predate this setup. They have not been migrated into Linear by this configuration step. Preserve them and do not describe them as already-published Linear issues.

Before continuing that existing Wayfinder effort in Linear, migrate or reconcile the map and child issues, preserve resolved answers and dependency relationships, and add Linear links to the local records. Search the target project first to avoid duplicates. Once migrated, Linear is the live tracker and the local files are reference material.
