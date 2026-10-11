# Skillglass

A clear view into your local agent skills.

Skillglass is a planned Go terminal tool for browsing, reading, comparing, and controlling skills across agent applications. It explains where a skill comes from, which applications and projects can use it, and what a proposed change would affect.

## Status

Planning is complete; application implementation has not started. This repository contains the approved specification, ticket breakdown, research, and coding workflow. There is no runnable application or build/test suite yet.

The project was planned as **Skill Viewer**. Existing Linear titles and historical planning documents retain that name; **Skillglass** is the product and repository name.

## Planned capabilities

- Browse and read skill bundles through an interactive terminal browser and ordinary CLI commands.
- Explain availability by agent application and project, including precedence and unsupported controls.
- Compare independent copies while distinguishing them from shared references.
- Preview and apply validated enable/disable controls, or permanently delete explicitly selected content after confirmation.
- Work offline, with the first release targeting macOS.

Planned integrations cover Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF. Capabilities must be validated per application, version, and scope. CodeAF browsing with explicit persistent-control limitations is an accepted first-release exception.

## Start here

- [Specification](docs/specs/skill-viewer-v1.md)
- [Published ticket breakdown](docs/plans/skill-viewer-v1-ticket-breakdown.md)
- [Coding workflow](docs/agents/coding-workflow.md)
- [Domain glossary](GLOSSARY.md)
- [Agent instructions](AGENTS.md)

[Linear](https://linear.app/alex-dancer/project/skill-viewer-58706f175bed) holds live ticket requirements, status, and dependencies. The local copies are references. Start by checking [Prototype the terminal journeys (ALE-22)](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) and its current blockers.

## Development workflow

Follow the Matt Pocock skill sequence in the [coding workflow](docs/agents/coding-workflow.md): implement one authorized ticket, verify the actual result, and review Standards and Spec independently. Stop before committing or closing the ticket unless the user has authorized those actions. The initial repository setup does not change that rule for future implementation work.

Skill links in the workflow point to the maintainer's installed local skills; those external skill files are not bundled in this repository.

## Planning records

The `.scratch/skill-viewer/` documents preserve earlier discovery and decision history. Their historical open decisions and status text are not the current implementation queue; the published Linear specification and tickets take precedence.

Downloaded toolchains under `.tools/` and local review artifacts under `.lavish/` are excluded from version control. Links to those artifacts in historical notes refer to the original local workspace.
