# Set privacy and automation boundaries

Type: grilling
Status: resolved
Parent: [Skill Viewer: product direction and constraints](../map.md)

## Question

Must browsing, search, cleanup, and control work fully offline? May optional features read agent activity logs or send skill text to a model? Should the first release only recommend and apply explicitly selected changes, or also perform any automatic cleanup?

## Context

Proposed baseline: local operation, no account requirement, and explicit user choice for changes. AI suggestions, telemetry, activity-log access, and background mutation have not been requested or authorized.

## User decisions

On October 10, 2026, the user chose **offline core; optional online features later**. Core browsing, reading, comparison, and control must work locally. Online features are future options, not first-release commitments or permission to upload skill contents.

The cleanup-automation boundary is being clarified in [Choose first-release cleanup actions](11-cleanup-actions.md). Activity-log access and background modification have not been requested.

## Answer

The offline-core decision above and [Choose first-release cleanup actions](11-cleanup-actions.md) establish local operation and explicit user-selected changes for the first release. Optional online capabilities are later opportunities. There is no current requirement for accounts, telemetry, activity-log ingestion, automatic cleanup, or background mutation; adding any of those would be a scope change.
