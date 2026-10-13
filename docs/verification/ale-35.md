# ALE-35: browse and explain OpenCode skills

`--agent opencode` adds offline OpenCode discovery and local configured-availability explanations to the accepted shared collection contract. CLI list/read/explain/compare and the browser use the same immutable snapshot. All writers remain unsupported. Production commands never launch OpenCode, run plugin hooks, load a skill, evaluate config substitutions, fetch skill indexes, or write sources/settings.

## Native version and discovery

The recorded rule version is **OpenCode 1.17.15**. It identifies validated rules, not the user's currently installed version or a running session. The installed Homebrew binary reported `1.17.15` in disposable HOME/XDG directories. Verification uses fresh noninteractive `--pure debug skill`, `debug config`, and `debug agent build` processes, with no credentials and an OS sandbox denying network access and reads/writes to the real HOME.

Global compatibility roots are HOME/.claude/skills and HOME/.agents/skills. Their project equivalents are searched from the explicitly selected directory to the nearest genuine Git repository, inclusive. Without Git, only the selected directory participates. No project is selected implicitly from CWD. Global native roots are XDG_CONFIG_HOME/opencode/{skill,skills} (default HOME/.config) and HOME/.opencode/{skill,skills}. Project .opencode/{skill,skills} directories run from nearest to repository root. OPENCODE_CONFIG_DIR adds its singular/plural roots last. Configured `skills.paths` is an array of recursive roots; `~/` expands HOME and relative roots resolve against the explicitly selected project. Relative configured roots without a selected project remain unknown. Optional missing native roots are reported, while missing required configured paths limit coverage.

Native discovery follows directory and manifest aliases and visits nested bundles even below another SKILL.md. External .claude/.agents roots include dot directories; native/configured roots exclude them. The viewer bounds traversal and reports cycles, unreadable entries and exhaustion. Generic explicit roots retain their established direct-child behavior and remain inspection-only unless another native reference reaches the same proven source. Native names come from frontmatter, independently of source identity. Proven directory aliases share one source; same-name independent directories and directories sharing only a manifest remain separate targets.

OPENCODE_DISABLE_EXTERNAL_SKILLS excludes both compatibility families; OPENCODE_DISABLE_CLAUDE_CODE or OPENCODE_DISABLE_CLAUDE_CODE_SKILLS excludes only .claude. Excluded roots remain inspectable with their flag evidence. OPENCODE_DISABLE_PROJECT_CONFIG omits project config and project .opencode roots, while external project scans retain their independent flag semantics. Unsupported or malformed native inputs remain unknown and limit coverage.

## Configuration and permissions

The local config order was validated independently from raw discovery:

1. Global config.json, opencode.json, opencode.jsonc.
2. OPENCODE_CONFIG file.
3. Project opencode.json/jsonc, repository to selected directory.
4. Native directories: global configuration, nearest-to-root project .opencode directories, HOME/.opencode, then OPENCODE_CONFIG_DIR. Directory config loads json before jsonc.
5. OPENCODE_CONFIG_CONTENT.
6. Managed opencode.json/jsonc in `/Library/Application Support/opencode`, or the supplied native OPENCODE_TEST_MANAGED_CONFIG_DIR fixture override.
7. OPENCODE_PERMISSION, followed by the default agent's permission rules.

OpenCode deep-merges objects, replaces arrays, and retains the original insertion position when replacing an existing key. JSON and JSONC preserve property order. The viewer supports comments and trailing commas, rejects ambiguous duplicate JSON keys and excessive nesting, and reads only bounded regular files. Config content and unrelated credentials are never reported. Native `{env:...}` and `{file:...}` substitutions remain unsupported instead of reading secrets or arbitrary files.

Native permission strings normalize to `*` before merging. Legacy tools.skill normalizes to an allow/deny rule below explicit permissions. Legacy per-agent tools normalizes before the agent config merge. The default native agent is build, or locally configured default_agent; its permission overrides append after the context-wide rules. Wildcards `*` and `?` use the native last-matching-rule semantics, including native slash normalization and home expansion. Source explanations identify the selected action, rule, input path and configuration layers. Allow maps to available, ask remains ask, and deny maps to disabled with permission denied. Display filters never recompute permissions or precedence.

Hidden, disabled and subagent-only configured default agents cannot establish native availability. Selected-agent markdown and legacy mode overrides are conservatively unknown. Runtime CLI --agent selection, saved/session approvals, account/organization remote policy and managed preference effective values are unverified. Presence or inaccessible metadata of relevant managed preferences limits coverage. A valid local config inference does not claim actual session tool enforcement. Invalid permission shapes/actions, config substitutions, FIFOs, oversized inputs and broken config links do not silently fall back to allow.

A definition without description is listed by the installed debug command and remains loadable by exact name, but v1 1.17.15 guidance excludes it. It therefore reports explicit-only. Later documentation describes `metadata["opencode/autoinvoke"]`; the pinned v1 source does not implement that field, so the viewer does not infer a 1.17.15 state from newer documentation. Generic-parser differences remain unknown rather than claiming native acceptance of malformed manifests.

## Precedence and managed sources

The installed v1 discovery path stores one definition per name, but loads discovered manifests using unbounded concurrent Effect.forEach. Pinned source and repeated native observations therefore do not establish deterministic project/global precedence. Equal-name eligible independent sources remain separate, with related-source IDs, unknown precedence and an explicit concurrent-overwrite reason. The viewer does not invent a winning source or label another source shadowed. This version-specific limitation is the acceptance exception agreed with the coordinator; known unique local names retain their configured permissions.

The native debug skill command returns raw definitions before agent permission filtering. Its presence alone cannot prove allow, ask or deny. The native agent's resolved ordered rules were validated separately. No inference session or tool call was started.

XDG_CACHE_HOME/opencode/skills is inventoried as managed cache content with an excluded cache-only route. Neither cached bytes, .opencode-version nor a plugin folder proves current index activation. The viewer never fetches `skills.urls`; configured remote indexes make effective discovery/precedence unknown. An independently discovered native alias into cache content participates through that alias, while ownership stays managed. Cache ownership uses canonical scope and does not merge independent copies.

Configured/local plugin hooks can change config and native capabilities dynamically. The viewer records their presence, limits coverage and marks native effective state unknown, while preserving ordinary discovery references. It does not execute hooks, guess a plugin identity from folder names, or scan every package cache. Explicitly inspected canonical cache bundles expose managed provenance and unsupported controls. Remote accounts, plugin-generated skills, v2 runtime behavior and embedded customize-opencode content remain unverified; the embedded skill has no local bundle and is reported as an inventory gap.

## Reproduce and inspect

The disposable validation utility creates all temporary ancestry under /private/tmp, independent of Git checkouts:

```sh
python3 tools/verify_opencode.py --opencode /opt/homebrew/bin/opencode --out /private/tmp/opencode-native-evidence
```

The utility reports 22 checks. It retains exact argv/cwd, stdout/stderr, exit codes, observed version and collision observations. It checks global/project/configured roots, Git boundary, hidden roots, nested bundles, manifest aliases, allow/ask/deny, deep merge/key order, project/directory/custom/inline/managed ordering, per-agent overrides, environment permission order, compatibility flags and cache inactivity. Fixture content is inert; --pure prevents external plugins. Native startup may create disposable native metadata and dependencies, which are removed with the temporary fixture. Native evidence does not claim native fixture immutability; public-product acceptance does.

`tests/acceptance/ale35_cli_test.go` tests the built executable for source identity, global/project/configured roots, shared aliases, inspection-only extras, unknown collisions, permission distinctions, JSONC/config/default-agent rules, environment layers, flags, cache participation/ownership, malformed/private/missing/bounded inputs, unsupported hooks and no writes. The OpenCode journey also runs under the established sandbox that denies network, writes, child execution and unselected content reads.

`tests/acceptance/ale35_browser_test.go` tests real tmux at 48x24 and 80x30 with long paths: immediately visible ask permission/session summary, full explanation paging, external config edit and refresh, retained selection, filtered hidden selection, availability cycling and application/version scope. Actual final terminal cells must be inspected along with automated bounds checks.

Run formatting, go vet, uncached go test -mod=readonly ./..., standalone tests/acceptance/run.sh with an evidence directory, scheduler Python tests, shellcheck/sh syntax and git diff --check. Set TMPDIR and GOTMPDIR to a dedicated /private/tmp directory outside all Git checkouts. Final results, actual terminal artifact paths, complete commit SHA and outstanding workflow gates are recorded in the assigned external build/check reports. Independent reviews and no-mistakes remain coordinator-owned; local checks do not close ALE-35.

Primary sources: [OpenCode skills documentation](https://opencode.ai/docs/skills/), [configuration](https://opencode.ai/docs/config/), [permissions](https://opencode.ai/docs/permissions/), and pinned [v1 skill discovery](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/opencode/src/skill/index.ts), [config loading](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/opencode/src/config/config.ts), [directory order](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/opencode/src/config/paths.ts), [permission evaluation](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/opencode/src/permission/index.ts), [permission normalization](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/core/src/v1/config/permission.ts), and [agent normalization](https://github.com/anomalyco/opencode/blob/v1.17.15/packages/core/src/v1/config/agent.ts). Current documentation can differ from the recorded installed version; pinned source and disposable observations govern this adapter.
