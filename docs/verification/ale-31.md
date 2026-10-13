# ALE-31 Hermes read validation

The viewer supports `--agent hermes` for list, read, explain, compare, problems and the terminal browser. This is read-only. Availability uses the ALE-26 explanation contract and the same filters and terminal renderers. Hermes policy lives in `internal/collection/hermes.go` and `hermes_plugins.go`.

## Recorded native version and provenance

Validated on macOS with installed Hermes 0.20.0, source commit `5a16635f409e0f5e3d754bb6256bd27707bcec4c`. Relevant installed source files were clean at that commit. The native receipt additionally records their SHA-256 hashes.

The repeatable native probe runs the actual installed CLI startup profile selector, metadata scanner, explicit read resolver, prompt builder and portable plugin manifest reader in a disposable credential-free home/project. It does not start a conversation, execute a skill workload, discover/load Python plugins, or access the live user's settings. Python socket connections are rejected before imports. Its native prompt builder creates a snapshot only inside the disposable profile. Hashes of skill contents, profile config, plugin manifests and identity markers are unchanged before/after.

```sh
/Users/alex/.hermes/hermes-agent/venv/bin/python tools/verify_hermes.py \
  /Users/alex/.hermes/hermes-agent ARTIFACT_DIR/native
```

Native receipts are held in the coordinator's ALE-31 artifact directory under `.agent-loop/selected-20261013/ale-31/native/`. Public executable and terminal tests run separately through `tests/acceptance/run.sh ARTIFACT_DIR/acceptance`.

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
| Profile `skills/` and `skills.external_dirs` | Recursive native scan, profile first, configured external roots in order. External paths expand home/environment variables and resolve relative to the active profile. Identical resolved roots are deduplicated. Missing configured roots remain visible as coverage gaps although native Hermes silently skips them. |
| Project | This native revision has no automatic project skill roots or ancestor config precedence. A project skill can be a configured external root. Explicit `--project-root` is inspection-only. Project plugin discovery is exact-project and opt-in through `HERMES_ENABLE_PROJECT_PLUGINS`. |
| Names and conflicts | Native `skills_list` index uses frontmatter name, truncated at 64 characters, and first name wins. `skill_view` separately refuses multiple independent bare-name matches. Same-profile definitions may both appear in prompt offers, so effective automatic precedence is unknown for that conflict. External later definitions are index-shadowed. Winner/source IDs and collision uncertainty remain visible. |
| Sharing | Directory aliases retain multiple references to one source. Independent same-name directories remain separate sources. Several native names for one shared source have reference-specific permissions and unknown aggregate precedence. Shared-manifest independent directories are not merged. |
| Disabled lists | `skills.disabled` union selected `skills.platform_disabled`. `HERMES_PLATFORM` precedes `HERMES_SESSION_PLATFORM`; unset only applies global list. Native probe confirms global deny survives a platform's empty list. |
| OS and relevance | Frontmatter OS `platforms` is distinct from messaging-platform disabled lists. Session-dependent `environments` and Hermes tool/toolset conditions retain unknown offer state. `disable-model-invocation` is retained content but is not a Hermes index-disable setting in this version. |
| Organization mirrors | `_org/.active_org` permits only the matching organization's mirror. Other mirrors are inventory-visible but excluded. Archives, dependency folders and bundles inside progressive-disclosure support directories are outside active native scan scope. |
| Plugins | User/project plugin directory survey is visible and inspection-only. Valid `plugin.yaml`, `plugin.yml` and `plugin.json` roots are read as data; placement/cache alone does not prove participation. `plugins.disabled` wins over `plugins.enabled`. Project opt-in and `HERMES_SAFE_MODE` are honored. Later opted-in project manifest with the same key supersedes the user manifest. |
| Portable namespace | Namespace is `agent-plugin-<sanitized-key>-<sha256-prefix>`. Key can include a category. Valid enabled portable skills are explicit-only. In-root manifest/skill validation is checked; escaped or invalid bundles remain excluded/unknown. Native probe confirms category namespace and declarative package discovery. |
| Native plugins and ownership | Dynamic `register_skill` calls require executable plugin code, which the viewer never runs. An enabled native plugin's bundled skill path remains unknown, rather than assumed enabled. Plugin provenance is managed, configured external provenance is external, and primary ownership is conservatively unknown from location alone. |

## Behavior and containment evidence

`tests/acceptance/ale31_cli_test.go` exercises active/default profiles, root versus specific-profile environment precedence, global/project configured scope, same-name sources, aliases, winning/shadowed definitions, same-profile offer ambiguity, reading/explanation, availability filters, OS and organization exclusion, plugin namespaces/opt-in/safe mode, missing directories and malformed config. Each read journey hashes the fixture tree and asserts no mutation. Inert plugin Python raises if executed.

`TestALE31HermesReadOnlyOfflinePublicExecutableAndPTY` runs the built executable's list, explain, read and browser under the accepted Darwin sandbox profile. Network, filesystem writes, reads outside the allowed fixture and child execution are denied. The existing harness's positive/negative controls independently verify that denial. It records JSON, explanation text, full instructions, raw PTY output and the sandbox profile.

`tests/acceptance/ale31_browser_test.go` captures actual tmux cells at 48 and 80 columns, including long paths, visible Hermes/project scope, availability explanation, refresh after a fixture config edit and availability filtering. Scope version text wraps across rows instead of clipping.

## Explicit evidence gaps

- No native writer is advertised. Enable, disable, removal of project overrides and deletion remain unverified capabilities.
- No running session, model invocation, session-local home/platform override, secret capture, or reload was observed.
- Native Python plugins are not executed; their registration names, load errors and effective skills are unknown.
- Bundled installation plugins and pip entry points are outside this bounded home/project survey, and are called out in scope gaps. Their absence is not presented as complete machine-wide discovery.
- Legacy flat `.md` explicit loads and malformed frontmatter fallback are outside the strict SKILL.md bundle contract; unsupported native parse cases remain unknown.
- OS portability beyond the inspected macOS environment is unverified. Platform-specific disabled settings refer to messaging surfaces, not an invented universal OS configuration.
- Full native CLI skill listing was not run because startup can migrate configuration or seed bundled skills. The recorded probe uses the actual native metadata and read functions without those side effects.
- Independent Standards/Spec reviews and no-mistakes execution are coordinator-owned stages after the clean frozen build candidate.
