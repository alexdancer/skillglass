# ALE-31 Hermes read validation

The viewer supports `--agent hermes` for list, read, explain, compare, problems and the terminal browser. This is read-only. Availability uses the ALE-26 explanation contract and the same filters and terminal renderers. Hermes policy lives in `internal/collection/hermes.go` and `hermes_plugins.go`.

## Recorded native version and provenance

Validated on macOS with installed Hermes 0.20.0, source commit `5a16635f409e0f5e3d754bb6256bd27707bcec4c`. Relevant installed source files were clean at that commit. The native receipt additionally records their SHA-256 hashes.

The initial metadata/index evidence remains preserved in the coordinator's original ALE-31 native receipts. Its legacy runner, `tools/verify_hermes.py`, is retired: it now refuses before resolving arguments, creating fixtures/output, or importing native startup code. Historical receipts are not fresh startup/catalog evidence. The bounded helper below covers environment/configuration only. Fresh repair validation runs only installed environment/configuration helpers through `tools/verify_hermes_policy.py`. Every helper invocation has a disposable HOME/profile/project/config environment and an OS sandbox denying network, child processes, reads outside fixture/code/runtime, and writes outside that fixture. Owned canaries must verify that boundary before native imports. Machine defaults `/etc/hermes` and `/etc/codex` are modeled absent. Installed-source `.env`, `.op.env` and config contents are excluded.

```sh
TMPDIR=/private/tmp/ale31-verification python3 tools/verify_hermes_policy.py \
  /Users/alex/.hermes/hermes-agent ARTIFACT_DIR/native
```

The runner fails before native execution when OS containment or exact clean source provenance is unavailable. Native helpers report Hermes 0.20.0, profile-versus-shell platform selection, session-platform fallback, managed plugin denies, per-leaf list replacement, and managed environment expansion. Disconfirming native controls show that multiline inert values do not create policy assignments; quoted keys and tab-separated export are natively parsed. The viewer supports tab-separated export and conservatively marks multiline/quoted-key syntax unknown. Managed `skills.disabled` is independently confirmed to have no effect on the raw skill utility. Fixture bytes remain unchanged. No plugin initialization, native catalog refresh, full CLI startup or inference session is run by repair validation.

Fresh repair receipts are held under `.agent-loop/selected-20261013/ale-31/repair/evidence/`. Public executable and terminal checks use `tests/acceptance/run.sh ARTIFACT_DIR/acceptance`.

Official sources consulted on October 13, 2026:

- [Installed skill loader source](https://github.com/NousResearch/hermes-agent/blob/5a16635f409e0f5e3d754bb6256bd27707bcec4c/tools/skills_tool.py).
- [Installed skill utilities](https://github.com/NousResearch/hermes-agent/blob/5a16635f409e0f5e3d754bb6256bd27707bcec4c/agent/skill_utils.py).
- [Installed profile startup selector](https://github.com/NousResearch/hermes-agent/blob/5a16635f409e0f5e3d754bb6256bd27707bcec4c/hermes_cli/main.py).
- [Installed plugin registration and discovery](https://github.com/NousResearch/hermes-agent/blob/5a16635f409e0f5e3d754bb6256bd27707bcec4c/hermes_cli/plugins.py).
- [Installed portable plugin validation](https://github.com/NousResearch/hermes-agent/blob/5a16635f409e0f5e3d754bb6256bd27707bcec4c/hermes_cli/agent_plugins.py).
- [Current skill documentation](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills).
- [Current plugin documentation](https://hermes-agent.nousresearch.com/docs/user-guide/features/plugins).

The current skill documentation describes newer project and `skills.create_dir` precedence. The recorded installed loader does not implement those discovery rules. The adapter pins installed behavior rather than guessing from documentation for another revision.

## Native rules represented

| Input | Reported rule and validation |
| --- | --- |
| `HERMES_HOME` and sticky `active_profile` | CLI startup honors a specific `<root>/profiles/<name>` home directly. A root home follows its sticky profile unless `HERMES_S6_SUPERVISED_CHILD` is set. Default root selection is bounded by the supplied `HOME`; missing/invalid profile input limits coverage. Native probe confirms case-normalized sticky Research profile instead of default. |
| Active profile `config.yaml` | Profile config only. Default-profile and project config are not merged. Missing config is an observed absence; unreadable or unsupported shapes retain uncertainty. |
| Profile `skills/` and `skills.external_dirs` | Recursive native scan, profile first, configured external roots in order. External paths expand home/environment variables; supported behavioral keys use the same profile/managed environment snapshot as platform policy, while references to other persisted bindings retain uncertainty. External paths resolve relative to the active profile. Identical resolved roots are deduplicated. Missing configured roots remain visible as coverage gaps although native Hermes silently skips them. |
| Project | This native revision has no automatic project skill roots or ancestor config precedence. A project skill can be a configured external root. Explicit `--project-root` is inspection-only. Project plugin discovery is exact-project and opt-in through `HERMES_ENABLE_PROJECT_PLUGINS`. |
| Names and conflicts | Native `skills_list` index uses frontmatter name, truncated at 64 characters, and first name wins. `skill_view` separately refuses multiple independent bare-name matches. Same-profile definitions may both appear in prompt offers, so effective automatic precedence is unknown for that conflict. External later definitions are index-shadowed. Winner/source IDs and collision uncertainty remain visible. |
| Sharing | Directory aliases retain multiple references to one source. Independent same-name directories remain separate sources. Several native names for one shared source have reference-specific permissions and unknown aggregate precedence. Shared-manifest independent directories are not merged. |
| Disabled lists | Raw active-profile `skills.disabled` union selected `skills.platform_disabled`. Profile `.env` literal behavioral values override stale shell values; managed `.env` wins last. `HERMES_PLATFORM` precedes `HERMES_SESSION_PLATFORM`; empty primary uses the fallback, and unset applies only the global list. These are inert, bounded reads. Unquoted profile values recognize native Unicode-whitespace inline comments; quoted hashes and attached hashes remain literal. Managed environment parsing retains its distinct native semantics. Unsupported relevant interpolation, encoding/normalization or external secret providers retain unknown state. Credential values are omitted. |
| OS and relevance | Frontmatter OS `platforms` is distinct from messaging-platform disabled lists. Session-dependent `environments` and Hermes tool/toolset conditions retain unknown offer state. `disable-model-invocation` is retained content but is not a Hermes index-disable setting in this version. |
| Organization mirrors | `_org/.active_org` permits only the matching organization's mirror. Other mirrors are inventory-visible but excluded. Archives, dependency folders and bundles inside progressive-disclosure support directories are outside active native scan scope. Exact excluded directory components apply to the full discovered manifest path, including configured-root ancestors; the organization marker remains relative to its discovery root. |
| Plugins | User/project plugin directory survey is visible and inspection-only. Valid `plugin.yaml`, `plugin.yml` and `plugin.json` roots are read as data; placement/cache alone does not prove participation. Profile plugin lists are expanded only for supported behavioral environment policy references. Native managed-directory resolution honors a nonempty `HERMES_MANAGED_DIR` without falling back when it is missing, otherwise `/etc/hermes`. Managed plugin values replace each matching profile leaf while preserving siblings. This merged plugin policy never redefines raw skill lists. `plugins.disabled` wins over `plugins.enabled`. Project opt-in and `HERMES_SAFE_MODE` are honored. Later opted-in project manifest with the same key supersedes the user manifest. |
| Portable namespace | Namespace is `agent-plugin-<sanitized-key>-<sha256-prefix>`. Key can include a category. Valid enabled portable skills are explicit-only. In-root manifest/skill validation is checked; escaped or invalid bundles remain excluded/unknown. Native probe confirms category namespace and declarative package discovery. |
| Native plugins and ownership | Dynamic `register_skill` calls require executable plugin code, which the viewer never runs. An enabled native plugin's bundled skill path remains unknown, rather than assumed enabled. Plugin provenance is managed, configured external provenance is external, and primary ownership is conservatively unknown from location alone. |

## Behavior and containment evidence

`tests/acceptance/ale31_cli_test.go` exercises active/default profiles, root versus specific-profile environment precedence, global/project configured scope, same-name sources, aliases, winning/shadowed definitions, same-profile offer ambiguity, reading/explanation, availability filters, OS and organization exclusion, plugin namespaces/opt-in/safe mode, missing directories and malformed config. Each read journey hashes the fixture tree and asserts no mutation. Inert plugin Python raises if executed.

`TestALE31HermesReadOnlyOfflinePublicExecutableAndPTY` runs the built executable's list, explain, read and browser under the accepted Darwin sandbox profile. Network, filesystem writes, reads outside the allowed fixture and child execution are denied. The existing harness's positive/negative controls independently verify that denial. It records JSON, explanation text, full instructions, raw PTY output and the sandbox profile.

`tests/acceptance/ale31_browser_test.go` captures actual tmux cells at 48 and 80 columns, including long paths, visible Hermes/project scope, availability explanation, refresh after a fixture config edit and availability filtering. Scope version text wraps across rows instead of clipping.

## Explicit evidence gaps

- No native writer is advertised. Enable, disable, removal of project overrides and deletion remain unverified capabilities.
- No running session, model invocation, session-local home/platform override, secret capture, or reload was observed.
- General dotenv interpolation, multiline or unclosed bindings, quoted policy keys, non-UTF-8 normalization and secret providers are not evaluated. Plugin config expansion is bounded to behavioral Hermes environment keys; unsupported/unresolved references are unknown.
- Machine managed policy is excluded from fixture evidence. Its real installed values and enterprise behavior were not observed. Explicit fixture managed roots exercise the supported policy semantics.
- Native Python plugins are not executed; their registration names, load errors and effective skills are unknown.
- Bundled installation plugins and pip entry points are outside this bounded home/project survey, and are called out in scope gaps. Their absence is not presented as complete machine-wide discovery.
- Legacy flat `.md` explicit loads and malformed frontmatter fallback are outside the strict SKILL.md bundle contract; unsupported native parse cases remain unknown.
- OS portability beyond the inspected macOS environment is unverified. Platform-specific disabled settings refer to messaging surfaces, not an invented universal OS configuration.
- Full native CLI skill listing was not run because startup can migrate configuration or seed bundled skills. The original metadata/read probe is preserved as historical evidence from the retired runner; only the bounded environment/configuration helper remains runnable.
- Independent Standards/Spec reviews and no-mistakes execution are coordinator-owned stages after the clean frozen build candidate.
