# ALE-32 Hermes controls

Hermes global enable/disable uses the existing source-bound preview, confirmation, safe write and reinspection workflow in both CLI and terminal browser. Global means the selected active profile, not all profiles. Only that profile's `config.yaml` `skills.disabled` name membership changes. A configured external project skill remains governed by profile settings.

## Validated native contract

Reference native version is Hermes 0.20.0, clean source commit `5a16635f409e0f5e3d754bb6256bd27707bcec4c`, on macOS. Fresh contained execution of whole native `hermes_constants`, `gateway.session_context` and `agent/skill_utils.py` bodies demonstrated target inclusion in `discover_all_skill_config_vars()`, exclusion after global disable and restoration after removing the denial. Platform-denial union, shell/session platform fallback, context-local platform override and profile-home override were observed. Both project config locations were ignored by this utility. Fixture atomics model the proposed saved bytes; native CLI save/config UI was not executed.

The runner establishes an OS sandbox before imports, uses owned read/write/network/child-execution canaries, bypasses executable package initializers and records prelaunch source/interpreter/helper/policy hashes. Only selected source files, Python runtime and fixture data are readable; real settings, credentials, bundled skills and plugin trees are excluded. Source/runtime and final fixture bytes remain unchanged. Native full CLI startup, full skill index, prompt builder, explicit view, plugins, sticky startup selection, dotenv startup and live sessions were not executed. The related read rules remain pinned-source inference or the established ALE-31 bounded evidence.

The full-name native utility differs from index truncation and prompt directory-name matching. Controls therefore require consistent directory, full-content and index names of at most 64 characters, proven source identity, eligible ordinary native participation and complete known collision scope. Independent same-name sources are refused even when currently excluded or shadowed. Uncertain names, managed/plugin sources, broad plugin actions and unsupported target syntax do not produce an executable preview. Shared content is never edited.

This revision has no validated native project override mechanism. Project enable, disable and inheritance removal return `unsupported`, explain the version-specific limitation and make no filesystem change. They never fall back to global scope. Global inherit is unsupported as well.

## Persistence and effective results

Enable removes only the selected global name denial. An applicable `skills.platform_disabled` denial, source exclusion, precedence or unknown state can still prevent effective availability. Such a persisted change is reported separately from an achieved result and returns nonzero when the requested enablement was not achieved. Running sessions may use context-local home/platform settings and caches. Adoption and whether reload/restart is necessary remain unknown.

The writer patches an admitted block YAML entry without reserializing unrelated bytes. Target aliases, merges, tags, multiline values and flow mappings are conservatively refused. Literal disabled lists, null and absent settings are supported; scalar strings remain readable but write controls refuse their cross-consumer membership ambiguity. The same admission check applies to the selected messaging-platform disabled policy, including mappings, scalar primitives and non-string list entries, before either enable or disable can write. Whitespace-padded selected entries never substitute for an exact-name denial when disabling. Unrelated disabled names, sibling settings and platform lists remain intact. Preview identity covers source/configuration and policy inputs; external edits, inode replacements and changes of profile, roots, policy or collisions invalidate confirmation. The shared descriptor-based writer retains permissions/ownership and verifies persisted bytes.

## Verification

`tests/acceptance/ale32_cli_test.go` exercises the built executable's profile-scoped disable/enable, byte preservation, platform denies, unsupported scopes, collisions, unsafe configuration and manifest identities, stale inputs, no-op choices and write denial. `ale32_browser_test.go` drives actual 48/120-column terminal confirmation, cancellation, unsupported project requests and stale confirmation. The inherited shared writer fault suite covers write/sync/rename/cleanup errors.

Use the combined-suite procedure in [ALE-24](ale-24.md#reproduce-the-acceptance-evidence), including the proved `SKILLGLASS_TEST_EXEC` launcher and separate `/private/tmp` scratch. Fresh receipts and native limits are recorded under the ALE-32 coordinator run. Review and no-mistakes status are owned by that run and are not inferred from this document. No CI result is claimed here.

Rerun the bounded native consumer proof with the installed clean pinned source and a fresh output directory:

```sh
python3 tools/verify_hermes_controls.py /Users/alex/.hermes/hermes-agent /private/tmp/ale32-native-evidence
```

`tools/hermes_control_probe.py` is an internal helper for that runner, not a standalone startup entrypoint. A native sandbox launch denied by an outer workspace sandbox must be reported or rerun with the host's scoped permission; do not remove the native OS boundary. The runner intentionally refuses another source revision. It does not claim equivalent behavior for a newer installed Hermes release.
