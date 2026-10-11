# Coding workflow

Use the installed Matt Pocock skills in the sequence below. Read each linked `SKILL.md` when its stage applies, using the Skill tool if available or reading the file directly. This document supplies project scope and adaptations.

**Default run:** one requested ticket, implemented, tested, and reviewed. Outside the agentic loop, stop before commits or ticket closure. For an explicitly requested parallel or autonomous run, follow [agentic-loop.md](agentic-loop.md). The user authorized local feature-branch commits and delegated ordinary review decisions for that loop. Pushes, PR publication, merges, and ticket closure still require authorization. Continue fixes within the authorized scope.

## 1. Orchestrate with implement-spec

Use [implement-spec](/Users/alex/.agents/skills/implement-spec/SKILL.md) to read the live parent specification, selected ticket, comments, and native blockers through the [Linear conventions](issue-tracker.md). Treat tickets as a dependency graph. Confirm eligibility, respect existing assignees, and claim the selected ticket when implementation starts. Report blockers instead of silently switching tickets.

Setup, `to-spec`, and `to-tickets` are complete. The local [specification](../specs/skill-viewer-v1.md) and [ticket breakdown](../plans/skill-viewer-v1-ticket-breakdown.md) are references; Linear holds current requirements and state. Read the glossary, relevant ADRs, code, local changes, and verification commands. State a short plan mapping acceptance criteria to observable behavior. CodeGraph was explicitly skipped; proceed without initializing it or asking again.

Use one integration branch for the spec once Git is established. Read [launch-subagent](/Users/alex/.agents/skills/launch-subagent/SKILL.md), then assign the selected ticket to an implementer in its own branch/worktree based on the integration tip. Pass context pointers and the no-commit stopping rule. Verify ancestry before work, preserving existing changes instead of blindly resetting them. Review in the ticket worktree; integration waits for step 6.

**Before Git exists:** preserve an exact scoped before-state and file manifest, and use an isolated directory for the implementer. Record branch/worktree stages as pending. Do not create an initial commit to bypass the approval boundary. Preserve the baseline and reviewed changes for the subsequently authorized Git setup.

## 2. Route prototype, feature, or bug work

**ALE-22:** use [prototype](/Users/alex/.agents/skills/prototype/SKILL.md) for the throwaway interaction experiment. The approved ticket requires a runnable terminal prototype, overriding the skill's HTML/page defaults. Use explicit fixtures, visible source/application/project/scope, narrow-terminal and long-path inspection, and recorded interaction contracts. Keep live settings and installed skills untouched. Its usability criteria still apply, but production tests and native-agent validation are not claimed. Retain the prototype locally until its capture commit is authorized.

**Production:** use [codebase-design](/Users/alex/.agents/skills/codebase-design/SKILL.md) when shaping modules and interfaces. Implement one observable journey at a time through the shared CLI/browser behavior core. Follow the specification's Testing Decisions: test the built Go executable, output, exit status, and filesystem effects in isolated temporary homes/projects, with focused pseudoterminal tests. ALE-23 establishes the reusable harness; narrower tests are exceptions under that contract.

**TDD adaptation:** the installed [tdd](/Users/alex/.agents/skills/tdd/SKILL.md) is bug-focused. Invoke it for an explicit TDD/regression-test request or an obvious cheap local regression target, overriding `implement-spec`'s unconditional invocation. Where applicable, prove the intended failure before the fix, then the passing result. Its allowance to skip impractical new regression tests never waives required acceptance coverage.

For bugs, use [diagnose](/Users/alex/.agents/skills/diagnose/SKILL.md) to reproduce the actual failure end to end before fixing it. Use [design-an-interface](/Users/alex/.agents/skills/design-an-interface/SKILL.md) for an unresolved interface choice worth comparing, and [domain-modeling](/Users/alex/.agents/skills/domain-modeling/SKILL.md) when changing vocabulary or recording an architectural decision, following [domain conventions](domain.md).

## 3. Verify the actual result

Run established formatting, static checks, build, and relevant acceptance tests. Exercise affected journeys offline; confirm temporary environments cannot fall back to real user settings. Inspect actual terminal rendering for interface changes and unaffected sources/settings for mutation work. Native writers require independent version-specific validation in a disposable environment before being enabled.

Fix discovered failures, flakiness, and visible defects. Rerun applicable checks after the last change. Record exact commands/results and any failed, blocked, or unrun checks. ALE-22 uses its runnable walkthrough and terminal inspection. Unmet criteria remain explicit gaps, not completed work.

## 4. Run code-review on the complete candidate

Use [code-review](/Users/alex/.agents/skills/code-review/SKILL.md) with two independent, parallel, read-only reviewers: **Standards** and **Spec**. Give both the same frozen candidate and base. Supply repository standards and the skill's full smell baseline to Standards; supply the originating live ticket/spec to Spec. Keep hard violations distinct from smell judgements, let repository standards override the baseline, and skip tooling-enforced checks.

**Pre-commit adaptation:** `git diff <fixed-point>...HEAD` excludes uncommitted work. Pin the before-state before editing and freeze a reproducible candidate bundle containing the complete scoped change: staged, unstaged, and untracked files, deletions, and relevant modes/symlinks. Record its path manifest and digest. Preserve unrelated local work separately.

For Git review, record and validate the immutable fixed-point SHA. Use a user-supplied point when present. In the agentic loop, delegated decision authority permits selecting the starting base without another question. Outside that loop, propose the starting integration SHA and ask as `code-review` requires. Include the scoped tracked working-tree delta against that base and complete untracked contents for pre-commit reviews. An invalid base or empty complete candidate must be resolved before dispatch.

Present separate **Standards** and **Spec** reports, with counts and the worst finding within each axis. Do not merge or rerank the axes.

## 5. Verify findings, fix, and hand back

Use [finding-verification](/Users/alex/.agents/skills/finding-verification/SKILL.md) for concrete behavioral defect claims, keeping investigation isolated from the delivery candidate. Judge standards/smell findings against the cited rule and actual design; a smell label alone does not justify a rewrite.

Have one implementer fix confirmed defects, rerun affected checks, and re-review material changes against a newly frozen candidate. Report unavailable review and inconclusive findings explicitly.

Present changed behavior, acceptance coverage, exact verification results, both review outcomes, and a runnable artifact plus diff or snapshot manifest. **Stop with the ticket open.** The ordinary workflow leaves changes uncommitted. The agentic loop may retain authorized local feature-branch commits and no-mistakes fix commits. Preserve pending work for the user's approval.

## 6. Integrate, then use pr when authorized

After commits/integration are authorized, resume `implement-spec`: commit scoped work, synchronize the ticket branch with the current integration tip, and use a merger subagent to integrate it. Resolve conflicts conservatively. Rerun applicable checks on the integrated artifact and re-review changed behavior or conflict resolutions.

For ALE-22, retain throwaway code on its prototype branch as `prototype` directs; carry only validated decisions into production. Once Git exists, use the preserved baseline and reviewed delta rather than losing the pre-commit review context.

If the user authorizes a broader run, recompute the live frontier after integration and dispatch only eligible, authorized tickets in separate branches/worktrees. Run final `code-review` on the complete integration branch using the approved fixed point, `git diff <fixed-point>...HEAD`, and `git log <fixed-point>..HEAD --oneline`.

Use [pr](/Users/alex/.agents/skills/pr/SKILL.md) whenever drafting a PR body: **Summary** with the smallest useful diagram/tree/diff, **Evidence** with actual before/after proof, and **Merge Danger** with Door and Blast Radius. Use glossary language and disclose missing evidence. Prepare the body locally before requesting any needed publication approval.

Linear is the tracker; a GitHub PR is optional. Publish or mark it ready only when authorized and verified. Use `gh-axi` for GitHub operations and attach created PRs to this chat. Closing references must cover only completed work, never the whole spec from a partial-ticket PR.

Close tickets only when authorized, with evidence and the tracker conventions. Clean up worktrees after their work is safely integrated or recoverably preserved. Completing a child never automatically closes ALE-21.

## Start prompt

> Use docs/agents/coding-workflow.md and its Matt Pocock skills for ALE-XX. Implement, test, and review this ticket, then stop before committing or closing it.
