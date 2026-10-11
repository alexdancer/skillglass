# Establish CodeAF skill controls

Type: research
Status: resolved
Claimed by: disable_semantics research agent
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Where does the installed CodeAF application discover skills, and which native controls exist for per-agent and project-scoped availability? What limits should the product show?

## Context

The user explicitly added CodeAF to the intended application coverage. The local command resolves to `/Users/alex/.codeaf/bin/codeaf`. Investigation is read-only and will be captured in [CodeAF skill controls](../research/codeaf-skill-controls.md).

## Answer

[CodeAF skill controls](../research/codeaf-skill-controls.md) records installed-build evidence and primary-source findings. CodeAF reads several agents' skill roots directly, applies its own name precedence, and retains shadowed definitions. Its conversation skill attachment is not persistent disabling. No persistent per-skill disable control was found in the installed build.

The installed dirty build and inspected upstream commit differ, so exact integration behavior needs validation before a writer is implemented. No agent state was changed during research.
