# Choose supported agents and operating systems

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Which agent applications need full browse-and-control support in the first release, and which operating systems must be supported? Should unrecognized skill folders still be browseable when availability cannot be determined?

## Context

Folders exist for Codex, Claude, Hermes, OpenCode, Pi, Gemini, and shared agent skills on this Mac. A folder's existence does not prove that its agent is installed, in use, or able to load every entry.

Proposed direction: discover all known local skill roots; prioritize reliable controls for the applications the user actually uses; target this Mac first while keeping the design portable. Await user priorities.

## User decisions

On October 10, 2026, the user answered "include codeaf also" when presented with Codex, Claude Code, Hermes, Pi, OpenCode, and Gemini. Include CodeAF alongside that discussed agent set. No ranking among agents or additional required operating systems was supplied.

`codeaf` resolves locally to `/Users/alex/.codeaf/bin/codeaf`. Its native capabilities are being investigated in [Establish CodeAF skill controls](10-codeaf-controls.md).

## Answer

Plan coverage for the discussed applications: Codex, Claude Code, Hermes, Pi, OpenCode, Gemini CLI, and CodeAF, with the user's current Mac as the first environment. No other operating system has been made a first-release requirement.

CodeAF browsing with clearly unsupported controls is explicitly accepted in [Decide whether CodeAF-specific disabling blocks the first release](12-codeaf-release-requirement.md). A folder or documentation entry does not prove a tested writer exists; each other adapter must also report only controls it can actually support. The Gemini CLI is not installed in the inspected environment, so its integration needs a separate validation environment if full controls are required.
