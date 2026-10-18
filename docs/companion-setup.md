# Companion setup

`setup` installs only this executable's exact bundled `skillglass-review` skill.
The unreleased companion is version 1.0.0 with review contract 1. Exact file bytes
and documented command capabilities are authoritative; matching version labels
alone do not establish compatibility. `companion --json` shows every bundled
file, content and SHA-256. Reading, browsing and collection review never install.

## Preview and explicitly install

The executable must be named `skillglass` and discoverable on the PATH used by
the native application. A desktop application's PATH can differ from a shell's.
`skillglass setup --check --json` checks executable discovery without installing.
For another executable found on PATH it checks the exact companion bytes and
help's documented reading, setup and review commands, with bounded output and
a timeout. The currently running exact filesystem executable already owns that
embedded contract. Setup does not pin an absolute binary location or write
shell/application configuration.

```sh
skillglass setup --app codex --app opencode --allow-shared-app codeaf --json
skillglass setup --app codex --app opencode --allow-shared-app codeaf --install --json
```

The first command previews destinations and shared effects without writes. The
second deliberately installs after those effects have been reviewed. Repeat
`--app` for every installation target. Repeat `--allow-shared-app` to acknowledge
indirect discovery by named applications without installing separate targets for
them. Omit `--install` for any fresh preview. Flags accept exact application IDs;
wildcards, duplicate selections, overlapping target/consent entries, arbitrary
content, destination paths and project scope are rejected before writes.

Every possible consumer found through the bounded native adapters must either
be an installation target or have explicit shared consent. Default Codex,
Claude and OpenCode folders are imported by CodeAF's recorded source loader.
The installed CodeAF executable is a dirty build whose discovery remains
unverified. Its `known-loader-native-unverified` evidence and explicit consent
mean possible exposure, never verified CodeAF setup support. Claude's personal
folder is additionally imported by OpenCode. A Claude-only installation normally
requires both `--allow-shared-app opencode` and `--allow-shared-app codeaf`.

Acknowledging CodeAF is separate from selecting `--app codeaf`. The latter is
an unsupported installation target and yields a nonzero result; the former can
allow successful verified target installations with status zero. Consent alone,
without an installation target, cannot install anything. There is no silent
fallback to another folder or broader application selection.

## Supported destinations

These are native user folders, not project folders. HOME must be explicit and
absolute. Actual native sessions are not inspected or restarted.

| Target | Exact default destination under HOME | Fresh native catalog validation |
| --- | --- | --- |
| `codex` | `.codex/skills/skillglass-review` | Codex 0.162.1 `skills/list` |
| `claude-code` | `.claude/skills/skillglass-review` | Claude Code 2.1.300 initialization catalog |
| `opencode` | `.config/opencode/skills/skillglass-review` | OpenCode 1.17.15 `debug skill`, including the shared Claude route separately |
| `hermes`, `pi`, `gemini`, `codeaf` | No setup target | Companion installation discovery is unverified; existing read/control support is separate |

Only macOS setup is validated. Non-default `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and
`XDG_CONFIG_HOME` destinations are reported unsupported without a default-folder
fallback. Directory aliases, relevant prospective aliases, unresolved sharing,
managed ownership and uncertain native configuration refuse safely. Shared
consent does not override unsupported destination or ownership checks. Known
loader rules inform possible consumers in the current environment; fresh native
catalog baselines do not prove custom routes or running-session adoption.

## Results and recovery

The JSON object has `schema_version: 1`, operation `setup`, the exact bundle
version/contract, `applications`, `allowed_shared_applications`, `install`, the
executable check, and one result per requested installation target. Each result
has its destination, validated native baseline, affected applications, isolation
evidence, reason and next action. Consent-only consumers have no installation
result. Human output displays the same selection, destinations and limitations.

| Per-target state | Meaning and next action |
| --- | --- |
| `ready` | Read-only preview passed; repeat exact targets and consent with `--install` after reviewing effects. |
| `installed` | Exact bundled bytes published exclusively and reread. Start a fresh native session or use its documented reload. |
| `already-present` | Exact bytes and file set exist; no writes are needed. |
| `refused` | Unacknowledged shared effect, existing differing/custom content, unsafe destination or uncertain isolation. Inspect the reason before retrying. |
| `unsupported` | No independently validated setup destination/mechanism in this environment. No broader fallback occurs. |
| `failed` | A filesystem or verification operation failed; inspect destination permissions and contents, then retry explicitly. |

Status zero means all requested targets are ready, installed or already present.
Usage errors return 2. Unsupported/refused/failed requests return 1. A mixed
outcome is `partial`, also status 1; successful prior installations remain. Setup
processes each independent eligible target and reports every target's result.
It does not claim a transaction across applications or roll back completed
writes. Permission failures can leave new parent directories or unpublished
staging content; preserve evidence and inspect the exact reported destination
before retrying. Existing unrelated skills and settings are never overwritten.

A customized copy, extra file, extra directory or nonregular bundle entry is a
preserved conflict. Compare it with `companion --json`; preserve or move the copy
yourself if desired, then preview again. Setup never upgrades arbitrary existing
content. An identical bundle is idempotent even if other skills are present.

For a missing or moved executable, restore a compatible build named `skillglass`
on the native application's PATH and rerun `setup --check`. For an incompatible
build, obtain the matching bundle/contract and command capabilities before
invoking the installed companion. Recheck in the actual application environment;
a successful shell check alone does not establish a running agent's PATH.
