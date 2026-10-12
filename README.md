# Skillglass

A clear view into your local agent skills.

Skillglass is a Go terminal tool for browsing and reading local agent skills. The planned product will also compare and control skills across agent applications, explain where a skill comes from, and show what a proposed change would affect.

## Status

The first production read-only slice is implemented. It discovers explicit skill roots, returns machine-readable or human-readable inventories and full source detail, and provides an interactive keyboard browser. Ownership and availability are visibly unknown until later integrations can support evidence-based answers. The repository also contains the approved specification, ticket breakdown, local agent scheduler, and the [ALE-22 terminal prototype](prototypes/ale-22/README.md) that preceded the production implementation.

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
- [Parallel agentic loop](docs/agents/agentic-loop.md)
- [Domain glossary](GLOSSARY.md)
- [Agent instructions](AGENTS.md)
- [ALE-23 verification](docs/verification/ale-23.md)

[Linear](https://linear.app/alex-dancer/project/skill-viewer-58706f175bed) holds live ticket requirements, status, and dependencies. The local copies are references. Start by checking [Prototype the terminal journeys (ALE-22)](https://linear.app/alex-dancer/issue/ALE-22/prototype-the-terminal-journeys) and its current blockers.

## Build and use

Build with Go 1.27.1:

```sh
/opt/homebrew/bin/go build -trimpath -o bin/skillglass ./cmd/skillglass
```

Every command requires an explicit discovery root. Project context is optional and never selects a root implicitly.

```sh
bin/skillglass list --root /path/to/skills
bin/skillglass list --root /path/to/skills --search testing --json
bin/skillglass read --root /path/to/skills SOURCE_ID
bin/skillglass browse --root /path/to/skills --project /path/to/project
```

Exit status 0 means the requested result is complete, including an empty collection. Status 1 means discovery, reading, or selection was incomplete and useful partial output is preserved. Status 2 reports command usage errors.

## Development workflow

Follow the Matt Pocock skill sequence in the [coding workflow](docs/agents/coding-workflow.md). Implement authorized work, verify the actual result, and review Standards and Spec independently. For parallel tickets and ticket parts, delegated decisions, and automatic no-mistakes fixes, follow the [agentic-loop operating guide](docs/agents/agentic-loop.md), including its approval boundaries.

The active Codex chat drives the loop through native agents. The scheduler persists leases and evidence across interruptions. It does not launch models or keep running after the coordinating chat ends.

Run the scheduler checks with Python 3.10 or later.

```sh
python3 -m unittest discover -s tests/agent_loop -v
```

Run the executable acceptance suite with Go module downloads disabled:

```sh
./tests/acceptance/run.sh
```

Skill links in the workflow point to the maintainer's installed local skills; those external skill files are not bundled in this repository.

## Planning records

The `.scratch/skill-viewer/` documents preserve earlier discovery and decision history. Their historical open decisions and status text are not the current implementation queue; the published Linear specification and tickets take precedence.

Downloaded toolchains under `.tools/` and local review artifacts under `.lavish/` are excluded from version control. Links to those artifacts in historical notes refer to the original local workspace.
