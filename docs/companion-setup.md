# Companion setup

`setup` installs one exact bundled companion selected by `--companion`; the
default remains `skillglass-review`. The executable also bundles the primary
agent workflow `skillglass-consolidate`. Both are version 1.0.0 with separate
review and consolidation contracts. Exact file bytes and documented command
capabilities are authoritative; matching version labels alone do not establish
compatibility. `companion --json` shows the default review bundle; use
`companion --name skillglass-consolidate --json` for the named workflow. Reading,
browsing and collection review never install either companion.

## Preview and explicitly install

The executable must be named `skillglass` and discoverable on the PATH used by
the native application. A desktop application's PATH can differ from a shell's.
`skillglass setup --check --json` checks the default review executable without
installing. Use `skillglass setup --check --companion skillglass-consolidate --json`
to require the named workflow. For another executable found on PATH the check
requests that exact bundle and verifies its bytes and required help commands with
bounded output and a timeout. A review-only old executable fails the named
consolidation check. The currently running exact filesystem executable already
owns its embedded contract. Setup does not pin an absolute binary location or
write shell/application configuration.

```sh
skillglass setup --app codex --app opencode --allow-shared-app codeaf --json
skillglass setup --app codex --app opencode --allow-shared-app codeaf --install --json
skillglass setup --companion skillglass-consolidate --app codex --app opencode --allow-shared-app codeaf --json
skillglass setup --companion skillglass-consolidate --app codex --app opencode --allow-shared-app codeaf --install --json
```

Commands without `--install` preview destinations and shared effects without
writes. Repeat the exact bundle name, `--app` targets and `--allow-shared-app`
consent with `--install` only after reviewing the preview. Repeat `--app` for
every installation target. Repeat `--allow-shared-app` to acknowledge
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
| `codex` | `.codex/skills/<selected-companion>` (`skillglass-review` by default) | Codex 0.162.1 `skills/list` |
| `claude-code` | `.claude/skills/<selected-companion>` (`skillglass-review` by default) | Claude Code 2.1.303 initialization catalog |
| `opencode` | `.config/opencode/skills/<selected-companion>` (`skillglass-review` by default) | OpenCode 1.17.15 `debug skill`, including the shared Claude route separately |
| `hermes`, `pi`, `gemini`, `codeaf` | No setup target | Companion installation discovery is unverified; existing read/control support is separate |

The table describes validated user-level destination mechanisms. Catalog
observations in the existing native setup record cover the review companion;
the new consolidation bundle is checked as an exact artifact and installed in
disposable acceptance fixtures. These results do not establish native session
adoption or add setup targets for Pi, Hermes, Gemini or CodeAF. Setup `--app`
values select installation targets only; they do not select the applications
whose personal skill sources will be consolidated.

Only macOS setup is validated. Non-default `CODEX_HOME`, `CLAUDE_CONFIG_DIR` and
`XDG_CONFIG_HOME` destinations are reported unsupported without a default-folder
fallback. Directory aliases, relevant prospective aliases, unresolved sharing,
managed ownership and uncertain native configuration refuse safely. Shared
consent does not override unsupported destination or ownership checks. Known
loader rules inform possible consumers in the current environment; fresh native
catalog baselines do not prove custom routes or running-session adoption.

## Results and recovery

The JSON object has `schema_version: 1`, operation `setup`, the exact bundle
name and version, its `review_contract_version` or
`consolidation_contract_version`, `applications`, `allowed_shared_applications`,
`install`, the executable check, and one result per requested installation
target. Each result
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

## Destination-specific isolation

Setup can proceed with incomplete inventory when the selected destination and
all possible affected applications are proven. The `isolation.warnings` array
and human `Warning:` lines retain unrelated discovery or ownership gaps; a
successful install does not claim complete inventory coverage. For example, a
broken skill link whose prospective target is proven disjoint or incomplete
inactive Pi package-cache inspection does not block a proven ordinary destination.
Inactive cache storage is still checked for destination ownership, and registered
package roots remain prospective discovery routes.

Pi ignore uncertainty can remain a warning in a disjoint root or an unrelated
sibling skill within the same root. This requires adapter-classified ignore
diagnostics with consistent source, reference and root provenance, plus a proven
ignore-input chain from the participating root through every prospective
destination ancestor and the destination itself. Unsupported rules on that chain
still refuse, even when no existing manifest produces an inventory diagnostic.
Missing or inconsistent provenance also refuses. The [bounded Pi ignore
contract](verification/ale-50.md#native-ignore-decisions) defines qualified syntax.

Unknown native configuration, plugin hooks and external-secret startup policy
can redirect discovery. Their uncertainty continues to refuse even when the
configuration file itself is elsewhere. Shared-consumer consent does not waive
unknown consumers. Related, cyclic or unresolved aliases, target ownership
uncertainty, and symlinked destination ancestry still refuse. Setup recomputes
the destination proof immediately before each installation and refuses changed
evidence; descriptor-relative no-follow traversal and exclusive publication
continue to protect the destination and conflicting copies.
