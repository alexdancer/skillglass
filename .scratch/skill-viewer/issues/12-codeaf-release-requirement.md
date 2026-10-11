# Decide whether CodeAF-specific disabling blocks the first release

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Does full persistent per-skill disabling for CodeAF need to be available before the first release, or can the first release offer CodeAF browsing and availability explanations while showing that specific control as unsupported?

## Context

The initial [CodeAF skill controls investigation](10-codeaf-controls.md) found conversation attachment controls but no persistent per-skill disabling in the installed version. CodeAF discovers shared skill roots independently of other agents' per-skill settings.

Removing or renaming shared content is not equivalent to disabling it only for CodeAF. No workaround should silently broaden the requested scope.

## Answer

The user chose **include CodeAF browsing and show unsupported controls clearly** on October 10, 2026. A persistent CodeAF-only disable feature does not block the first release. The tool must not substitute a shared-source mutation or another agent's setting and claim that CodeAF alone has been disabled.
