# Agent skill discovery and control research

Status: planning evidence only. This note records observations and candidate directions. It does not select product behavior. No agent configuration, skill, plugin, or application file was changed during the investigation.

Snapshot date: 2026-10-10. The workspace is not a Git repository, so the research-branch step in the research workflow could not be performed. Git was not initialized.

## Method and limits

The local scan covered these user roots only: `~/.codex/skills`, `~/.agents/skills`, `~/.claude/skills`, `~/.hermes/skills`, `~/.config/opencode/skills`, `~/.pi/agent/skills`, and `~/.gemini/skills`. Top-level entry and symlink counts used `find` with `-mindepth 1 -maxdepth 1`. Manifest counts followed links and searched for exact `SKILL.md` names at bounded depth 4 or 5. Broken-link checks tested only top-level symlinks. Canonical grouping resolved every observed manifest with `realpath`, then grouped equal paths.

These are observed filesystem counts, not unique logical skill counts. Manifest counts can exceed root entries because some roots contain nested skills. A host can also discover another host's root, so a root count is not that host's complete effective inventory. The scan did not read skill bodies, unrelated user files, credentials, or secret-bearing configuration values.

| Root | Top-level entries | Top-level symlinks | Bounded `SKILL.md` occurrences | Broken top-level links | `SKILL.md.disabled` |
|---|---:|---:|---:|---:|---:|
| `~/.codex/skills` | 260 | 253 | 264 | 0 | 0 |
| `~/.agents/skills` | 269 | 115 | 272 | 0 | 0 |
| `~/.claude/skills` | 415 | 373 | 417 | 0 | 0 |
| `~/.hermes/skills` | 200 | 124 | 272 | 0 | 0 |
| `~/.config/opencode/skills` | 58 | 53 | 58 | 0 | 0 |
| `~/.pi/agent/skills` | 144 | 100 | 145 | 0 | 0 |
| `~/.gemini/skills` | 1 | 1 | 1 | 0 | 0 |

Across those roots, 1,429 manifest occurrences resolved to 548 canonical manifest paths. Of those canonical paths, 260 appeared through more than one occurrence, with a maximum of eight occurrences for one canonical manifest. The occurrence histogram was: 288 seen once, 40 twice, 2 three times, 96 four times, 67 five times, 51 six times, 2 seven times, and 2 eight times.

The installed tools observed were Codex CLI 0.155.1 at `~/.local/bin/codex`, Claude Code 2.1.282, Hermes Agent 0.20.0 (2026.8.3), OpenCode 1.17.15, and Pi 1.1.0 from `@earendil-works/pi-coding-agent`. No Gemini CLI binary was found, so Gemini behavior below is documentation-only on this machine.

## Local topology and its product implications

Directory symlinks are the dominant topology. For example, the five manifest paths below all resolve to the same canonical file, `~/.agents/skills/research/SKILL.md`:

- `~/.codex/skills/research/SKILL.md`
- `~/.agents/skills/research/SKILL.md`
- `~/.claude/skills/research/SKILL.md`
- `~/.hermes/skills/research/SKILL.md`
- `~/.pi/agent/skills/research/SKILL.md`

This means a canonical-file mutation can affect several agents at once. It also means removing one alias may not remove a skill from a host that scans another root. OpenCode officially scans `~/.agents/skills` and `~/.claude/skills` in addition to its own root. Pi also officially scans `~/.agents/skills`. Host visibility therefore needs to be calculated from discovery rules and precedence, not inferred from the host-named directory alone.

Managed sources add another layer. The bounded scan found 88 skill manifests under `~/.codex/plugins/cache`, spanning 19 version roots and six top-level sources. It found four manifests under `~/.claude/plugins/cache` and 46 under `~/.claude/plugins/marketplaces`. No manifests were observed under `~/.hermes/plugins` or `~/.pi/agent/extensions`. Hermes also has profile skill roots at `~/.hermes/profiles/oryx/skills` and `~/.hermes/profiles/coder/skills`; the active profile was not established.

Plugin caches, marketplace checkouts, synced sources, and package-managed directories should be modeled as managed until their ownership is known. OpenAI's plugin documentation says Codex loads an installed plugin copy from its cache and a marketplace refresh can refresh cached files even while the plugin is disabled. Direct edits in such locations can be overwritten.

## Native controls found

The [Agent Skills specification](https://agentskills.io/specification) defines an exact `SKILL.md` manifest and standard metadata, but it does not define a portable enabled or disabled state. Each agent therefore needs its own adapter if the product wants host-specific control.

| Agent | Native control | Scope and limits | Primary evidence |
|---|---|---|---|
| Codex | `[[skills.config]]` with `path` and `enabled = false` | Path-based per-skill control. Restart is documented after config edits. `agents/openai.yaml` can make a skill explicit-only with `policy.allow_implicit_invocation: false`; that is not full disable. | [Codex skills](https://developers.openai.com/codex/skills/), [config reference](https://developers.openai.com/codex/config-reference/) |
| Claude Code | `skillOverrides` values `on`, `name-only`, `user-invocable-only`, or `off` | Name-based. `/skills` writes project-local `.claude/settings.local.json`. Plugin skills are explicitly excluded from `skillOverrides` and must be managed through `/plugin`. Frontmatter `disable-model-invocation: true` is explicit-only and changes the shared manifest. | [Claude Code skills](https://code.claude.com/docs/en/skills) |
| Hermes | `skills.disabled` and `skills.platform_disabled.<platform>` in `~/.hermes/config.yaml` | Name-based. Global and platform lists are unioned. The CLI exposes `hermes skills`; local code also supports qualified plugin names. | `~/.hermes/hermes-agent/hermes_cli/skills_config.py:1-72`, `~/.hermes/hermes-agent/tools/skills_tool.py:623-745`, `~/.hermes/hermes-agent/agent/skill_utils.py:436-471` |
| OpenCode | `permission.skill` patterns with `allow`, `ask`, or `deny`; per-agent overrides are documented | Name or pattern based. `deny` hides the skill and rejects access. `metadata["opencode/autoinvoke"] = "false"` is explicit-only. The whole skill tool can also be disabled for an agent. | [OpenCode skills](https://docs.opencode.ai/docs/skills/) |
| Pi | Entries in the `skills` resource array support `!pattern` glob exclusion and exact `-path` exclusion | Path-based. User and project resource arrays both participate. Frontmatter `disable-model-invocation: true` is explicit-only. | Installed docs `.../@earendil-works/pi-coding-agent/docs/settings.md:145-160` and `docs/skills.md:41-89` |
| Gemini CLI | `/skills disable <name>` and `/skills enable <name>`; settings expose `skills.enabled` and `skills.disabled` | Name-based and documented to require restart for settings changes. This host has no local binary, so behavior and version compatibility are unverified. | [Gemini skills](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/using-agent-skills.md), [configuration](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/configuration.md), [commands](https://github.com/google-gemini/gemini-cli/blob/main/docs/reference/commands.md) |

Observed state matters before any write. `~/.pi/agent/settings.json` already contains configured skill paths, a glob exclusion, and an exact skill exclusion. The product would need to import and preserve that state rather than replace the array. At this snapshot, `~/.codex/config.toml` had no `skills.config`, `~/.claude/settings.json` had no `skillOverrides`, and `~/.hermes/config.yaml` had no `skills.disabled` entry. Only relevant keys were inspected.

There is a Codex documentation ambiguity that needs a version-specific test before implementation: the skills guide shows `path` pointing to `SKILL.md`, while the configuration reference describes it as the path to a skill folder. The official current user root is `$HOME/.agents/skills`; the local machine also has a populated `~/.codex/skills`, which should be treated as a local or legacy compatibility root until behavior is verified against Codex 0.155.1.

Name-based controls also need collision handling. A disabled name can affect whichever same-name skill wins precedence, or more than one definition, depending on the agent. Path-based controls can distinguish occurrences, but symlink alias versus canonical-path matching has not yet been tested.

## Candidate fallback semantics

These are implementation candidates for later validation, not user decisions.

1. **Prefer a native adapter for host-specific disable.** This preserves canonical files and directory symlinks and can expose richer states such as auto, explicit-only, ask, and off. It also requires config-aware merge and undo behavior per agent.

2. **Rename the canonical manifest for an intentional all-consumers disable.** An atomic `SKILL.md` to `SKILL.md.disabled` rename should remove the exact manifest expected by these loaders while preserving directory symlinks. It would affect every alias that resolves to that canonical file, but not independent copies with the same name. This needs an explicit blast-radius preview, a destination-collision check, a journal of the original `lstat`, hash, and raw symlink text, and a restore conflict check. Managed caches and synced roots should be blocked or warned until ownership is understood.

3. **Move a directory alias outside every scanned root for occurrence-level disable.** This is only host-specific when that host has no alternate discovery path to the same skill. A staging directory inside a recursively scanned root is ineffective for Hermes and Pi, and may be ineffective elsewhere. Moving a real canonical directory can break inbound links. Moving a relative symlink preserves its raw target for undo but leaves it broken while staged.

The native-first option is the safest direction to investigate because shared canonical manifests are common in this environment. The filesystem fallbacks remain useful as explicit operations when the user understands their scope.

## Validation gaps before implementation

- Enumerate project-local roots, ancestor-walk stopping rules, trust gates, and configuration precedence for each installed version.
- Test Codex's folder-versus-manifest `path` interpretation and symlink canonicalization on 0.155.1.
- Test Pi exact exclusions against symlink aliases and canonical paths, plus user-versus-project override behavior on 1.1.0.
- Confirm OpenCode's current JSON or JSONC schema and control behavior on 1.17.15.
- Install or otherwise obtain a matching Gemini CLI only if Gemini integration becomes in scope; current evidence is documentation-only. Gemini scope merging has changed historically and should not be assumed.
- Resolve Hermes's active profile, platform selection, external directories, plugin namespace behavior, and reload latency.
- Map plugin-provided skills to plugin identifiers and determine whether host per-skill controls apply, or whether only whole-plugin disable is supported.
- Determine whether caches, organization mirrors, and account-synced roots are immutable, refreshable, or user-owned before offering mutations.
- Measure running-session reload behavior for every agent. Some require restart, some expose reload commands, and Hermes caches discovery with a short TTL.
- Define collision behavior for same-name skills and precedence winners before exposing name-based toggles.

## Capability gaps that could guide the product

A useful viewer could group occurrences by canonical realpath while showing every agent, source root, raw symlink target, precedence winner, duplicate name, and managed-source owner. Controls could show their true scope before applying: one host, one occurrence, explicit-only, whole plugin, or every alias of a canonical file. Existing native state should be imported with its configuration layer and provenance.

Undo should be conflict-aware rather than a blind inverse. A journal can record config fingerprints, file hashes, raw symlink text, and prior native values, then refuse or reconcile when outside changes occurred. Diagnostics for broken links, duplicate names, stale cache entries, and reload requirements would address gaps already visible in the local topology.

## Local primary source paths

- Hermes control UI and persistence: `~/.hermes/hermes-agent/hermes_cli/skills_config.py:1-72`
- Hermes discovery and disabled-name filtering: `~/.hermes/hermes-agent/tools/skills_tool.py:623-745`
- Hermes platform union, external roots, and symlink-following recursion: `~/.hermes/hermes-agent/agent/skill_utils.py:436-504` and `:877-896`
- Pi resource exclusions: `~/Library/pnpm/store/v11/links/@earendil-works/pi-coding-agent/1.1.0/b82525293ef6c215cd97793d9e272745c15afd04d245c0cc411473215984c040/node_modules/@earendil-works/pi-coding-agent/docs/settings.md:145-160`
- Pi discovery, manual-only mode, collisions, and reload: the adjacent `docs/skills.md:41-89`
