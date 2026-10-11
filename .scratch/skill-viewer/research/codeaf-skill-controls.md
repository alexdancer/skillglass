# CodeAF skill discovery and controls

Status: planning evidence only, snapshot 2026-10-10. No CodeAF state, agent configuration, skill, plugin, or application file was changed. The workspace is not a Git repository, so no research branch was created and Git was not initialized.

## Identity and installed build

The product brand is **CodeAF**. Its executable and command name are lowercase `codeaf`. The official repository is [Agent-Field/CodeAF](https://github.com/Agent-Field/CodeAF), while its Go module path is lowercase `github.com/Agent-Field/codeaf`.

This machine has `/Users/alex/.codeaf/bin/codeaf`. Both `codeaf version` and `codeaf --version` are the appropriate read-only version probes. The installed binary reports:

```text
codeaf 7e30335 (dirty) built 2026-10-09 10:03 · go1.26.5 darwin/arm64
```

That is a commit-style dirty build identifier, not a release semver. An integration should retain the complete version string and avoid inferring a release tag. `codeaf doctor` confirms the active default state root is `~/.codeaf`. The official README says the installer places the binary in `~/.codeaf/bin`.

Confidence: high. These facts come from the installed executable and the official repository.

## Discovery behavior

CodeAF reads Agent Skills in place on launch. A candidate must be a direct child directory of a skills root and contain an exact `SKILL.md` with parseable frontmatter `name` and `description`. It does not recursively discover nested skill directories. A symlinked child directory counts, as does a `SKILL.md` symlink that resolves to a regular file. Non-regular manifests are skipped, and manifest reads are capped at 64 KiB.

For both the active project directory and login home, roots are checked in this exact precedence order:

1. `.codeaf/skills`
2. `.agents/skills`
3. `.claude/skills`
4. `.codex/skills`
5. `.cursor/skills`
6. `.gemini/skills`
7. `.opencode/skills`
8. `.goose/skills`

The user scope then adds `~/.config/opencode/skills` and `~/.config/goose/skills`. Claude Code plugin skills follow the hand-kept roots. Codex bundled skills under `.codex/skills/.system` come last.

The current implementation scans the one project workspace supplied to the session. It does not walk ancestor directories for additional project skill roots. Project skills beat user skills by frontmatter name. Within one scope, the earlier root wins. A losing same-name definition remains represented as `shadowed` rather than disappearing. CodeAF uses the original occurrence directory as the imported skill identity, while following symlinks to digest its content.

This matters for the viewer's duplicate model: two independent folders with the same frontmatter name are distinct occurrences with separate source, path, and availability. CodeAF chooses one winner for execution but preserves the shadowed status of the others. A removed winner can reveal the next same-name occurrence.

Primary evidence: the installed pages `codeaf manual skills-from-other-tools` and `codeaf manual use-skill`; [official discovery source at commit 973b7d2](https://github.com/Agent-Field/CodeAF/blob/973b7d22d6152380e27120c8646458a8adefa0bc/internal/skills/skills.go); [official imported-skill reconciliation source](https://github.com/Agent-Field/CodeAF/blob/973b7d22d6152380e27120c8646458a8adefa0bc/internal/resident/skills.go); and the [official bring-your-skills guide](https://agentfield.ai/docs/codeaf/playbooks/bring-your-skills).

Confidence: high for the installed build's documented behavior and matching current source. The installed build is dirty at commit `7e30335`, while the inspected repository source is commit `973b7d2`, so implementation details still need a smoke test before any writer is shipped.

## Local candidate counts

Counts below are exact `SKILL.md` files one level below each root, following links. They are discovery candidates, not effective winners: malformed manifests may be skipped, names may collide, and plugin candidates are separate.

| User root | Direct candidates |
|---|---:|
| `~/.codeaf/skills` | absent |
| `~/.agents/skills` | 269 |
| `~/.claude/skills` | 414 |
| `~/.codex/skills` | 258 |
| `~/.cursor/skills` | 57 |
| `~/.gemini/skills` | 1 |
| `~/.opencode/skills` | absent |
| `~/.goose/skills` | absent |
| `~/.config/opencode/skills` | 58 |
| `~/.config/goose/skills` | 43 |
| `~/.codex/skills/.system` | 5 |

All eight project roots under `/Users/alex/Documents/skill-viewer` were absent at the snapshot. CodeAF can still see the large user inventory in this project.

`CODEAF_HOME` changes more than the state database. The official source makes it the login base CodeAF uses for foreign agent roots too. A process started with a different `CODEAF_HOME` therefore has a different ambient skill inventory. This is useful as whole-profile isolation, but it is not a per-skill control. See [official home resolution source](https://github.com/Agent-Field/CodeAF/blob/973b7d22d6152380e27120c8646458a8adefa0bc/internal/home/home.go).

## Plugin ownership

CodeAF directly imports Claude Code plugin skills, not every plugin cache it can find. A plugin must be present in `~/.claude/plugins/installed_plugins.json` and enabled through Claude's `enabledPlugins` state. CodeAF applies user `~/.claude/settings.json`, then project `.claude/settings.json`, then project `.claude/settings.local.json`, with later values overriding earlier ones. A project-installed plugin is visible only in that project.

CodeAF reads only the skill folders named by the plugin manifest or marketplace catalog, falling back to the plugin's `skills` folder when none are named. It ignores disabled plugins, marketplace-only checkouts, and stale cached versions. Plugin provenance is retained as `name@marketplace`, but the skill is resolved by its folder name rather than Claude's `plugin:skill` spelling.

The installed manual explicitly says Claude Code plugin settings that hide an individual skill from the model or slash menu are not honored yet. Every skill in an enabled plugin is offered automatically and manually by CodeAF. Whole-plugin enablement is therefore an indirect native control, but it affects the plugin as a unit and belongs to Claude's configuration.

Primary evidence: `codeaf manual skills-from-other-tools` and the [matching official manual source](https://github.com/Agent-Field/CodeAF/blob/973b7d22d6152380e27120c8646458a8adefa0bc/internal/manual/chat/skills-from-other-tools.md).

Confidence: high.

## Native enable and disable controls

No persistent per-skill disable for imported `SKILL.md` skills was found in the installed manual, the relevant keys present in the current `config.json`, or the matching official discovery and reconciliation source.

`/skill` and `/skills` do not disable discovery. They toggle whether a skill is attached in front of the current conversation. A detached skill remains on the shelf, remains listed to the model, and can still be selected automatically or opened with `use_skill`. The built-in manual's phrase "turn a skill off" refers only to this conversation attachment.

CodeAF also does not consult Codex, Claude, OpenCode, or other agents' ordinary per-skill disable settings when scanning hand-kept roots. It reads the directories directly. A skill disabled natively in one of those hosts can therefore remain available in CodeAF. Claude whole-plugin enablement is the documented exception described above.

The only native isolation mechanism found is whole-profile relocation with `CODEAF_HOME`. It can start CodeAF against a separate base without ambient user skills, but it does not express one-skill, one-project, or explicit-only state.

Primary evidence: `codeaf manual putting-a-skill-in-front`, [official attachment manual source](https://github.com/Agent-Field/CodeAF/blob/973b7d22d6152380e27120c8646458a8adefa0bc/internal/manual/chat/putting-a-skill-in-front.md), and the discovery source above.

Confidence: high that `/skill` is attachment-only. Confidence is medium-high that no persistent per-skill control exists in this build, because absence claims remain version-sensitive.

## Planning implications and validation gaps

- CodeAF needs its own availability calculation over all of its roots. Reusing another host's enabled state would be incorrect.
- Availability explanations should name the winning occurrence and why others are shadowed. Disabling or moving a winner may activate the next same-name definition.
- A filesystem fallback for CodeAF can have cross-agent effects because most roots are owned or shared by other harnesses. Renaming a shared canonical `SKILL.md` affects every alias. Moving one alias may leave another CodeAF root active.
- Managed Claude plugin files should not be edited. Whole-plugin control is broader than a skill toggle and should be labeled with its Claude ownership and project scope.
- A future CodeAF-specific writer needs an installed-version smoke test for project root selection, symlink behavior, shelf refresh, and whether any newer build adds a persistent control.
- The viewer should detect `CODEAF_HOME` when launched in the same environment and explain that a different CodeAF process can resolve a different home inventory.
- CodeAF re-reads folders before the first message and reconciles a missing or unreadable manifest out of its shelf. The manual directs users to relaunch after plugin or skill changes. Live-session refresh behavior beyond the picker scan should be tested rather than assumed.

No identity question remains: the local command, official site, repository, and embedded module paths all identify this product as CodeAF.
