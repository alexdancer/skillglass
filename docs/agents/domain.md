# Domain docs

This is a single-context project. Domain vocabulary lives in root `GLOSSARY.md`; architecture decisions live in `docs/adr/` when created.

## Before exploring, read these

- Read root `GLOSSARY.md`.
- Read any ADRs in `docs/adr/` that concern the area being explored.

If a file or directory does not exist, proceed silently. Do not flag its absence or suggest creating it upfront. The `domain-modeling` skill creates domain documentation lazily as terms or decisions are resolved.

## File structure

- `GLOSSARY.md`: shared domain vocabulary for this project.
- `docs/adr/`: architecture decision records, created only when a decision warrants one.

Keep the existing glossary. No `GLOSSARY-MAP.md` or per-context layout is needed for the current project.

## Use the glossary's vocabulary

Use the glossary's defined terms when naming domain concepts in issues, proposals, hypotheses, and tests. Do not drift to synonyms the glossary explicitly avoids.

If a needed term is absent, reconsider whether it reflects the project's language; if it exposes a real gap, note it for `domain-modeling`.

## Flag ADR conflicts

If a proposal contradicts an existing ADR, identify the ADR and explain the conflict instead of silently overriding the decision.
