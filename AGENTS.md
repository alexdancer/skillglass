## Agent skills

### Coding workflow

When implementing a ticket or fixing a bug, follow `docs/agents/coding-workflow.md` for the Matt Pocock skill sequence, project adaptations, and approval boundary.

For an explicitly requested parallel or autonomous run, use `docs/agents/agentic-loop.md` and `tools/agent_loop.py`. Local feature-branch commits and ordinary delegated review decisions are authorized for that loop. Pushes, PR publication, merges, and ticket closure remain separate. A no-mistakes validation-step agent performs only its assigned phase and never becomes the outer loop coordinator.

### Issue tracker

Issues are tracked in the Skill Viewer project in Linear, under the Alex Dancer (ALE) team. See `docs/agents/issue-tracker.md`.

### Triage labels

Use the five canonical triage labels already present in Linear: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, and `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Use a single-context layout: root `GLOSSARY.md` and `docs/adr/`. See `docs/agents/domain.md`.
