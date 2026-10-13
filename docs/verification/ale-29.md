# ALE-29: Claude Code browsing and explanations

Select `--agent claude-code` (the `claude` alias is accepted) with an explicit project directory, or omit the project to inspect personal skills only. `list`, `problems`, `read`, `explain`, `compare` and `browse` share the existing availability contract and source identity model. Explicit viewer roots remain inspection locations, not Claude launch arguments. Nothing invokes Claude or writes its settings during product use.

## Recorded native environment

Validated on macOS against **Claude Code 2.1.282**, at `/Users/alex/.local/bin/claude`. Reproduce with:

```sh
python3 tools/verify_claude.py /private/tmp/ale29-native-evidence
python3 tools/verify_claude_plugin_manifest.py /private/tmp/ale29-manifest-evidence
```

The script creates a canonical disposable `/private/tmp/ale29-native-*` home and Git project. Preparatory Git initialization uses the direct developer Git binary, fixture HOME/XDG/cache/cwd, empty global config and template, and a separate contained utility profile with only a `/dev/null` device write exception. The Claude profile retains the exact fixture write boundary. It gives Claude an empty environment apart from fixture paths, a literal dummy API key, disabled telemetry/nonessential traffic and an unreachable local API URL. `/usr/bin/sandbox-exec` denies all network access, all writes outside that exact canonical fixture root, caller/OS home reads, and native system settings reads under `/Library/Application Support/ClaudeCode`, `/Library/Managed Preferences`, and the Claude preferences plist. Only the exact resolved native executable may be read from a denied home. Before any Claude process, inert positive and negative controls verify fixture reads/writes, modeled home/system reads, outside/symlink write denial, and inherited-child restrictions. Failed controls or a sandbox error refuse native execution. Version, registry and initialization calls share the same fixture environment, cwd and sandbox. It sends one SDK `initialize` control request and terminates the process after the native catalog arrives. It sends **zero user/model prompts**, executes **zero skill workloads**, supplies an empty strict MCP configuration, and persists no session. Native `plugin list --json` separately checks registry interpretation. Source and settings digests are compared before and after each startup. Native startup metadata in the disposable config directory is allowed; it is not skill or settings input.

The first repair discovery matrix is preserved under `repair/native-final/`. The second repair's focused homepage validation is under `/Users/alex/Documents/skill-viewer/.agent-loop/selected-20261013/ale-29/repair/native-final-2/`: `summary.json`, four native validation results/catalogs, input manifests, `native-contained.sb`, `containment.json` and invocation-time executable hashes/environment/cwd in `invocations.json`. The focused oracle validates absent/valid HTTPS/invalid literal and malformed hierarchical homepage fields without rerunning unchanged discovery cases. The repaired helper checks exact 2.1.282 before initialization. System settings and enterprise enforcement remain unobserved because their live inputs are denied.

Earlier evidence under `native/accepted/` is preserved. Its final version call inherited ambient context and lacked the helper sandbox; historical live reads, writes, credentials and other effects were not observed. Earlier catalogs and the separately contained review catalogs retain their recorded observation limits. Early failed probes are preserved in sibling `matrix`, `verified` and `validated` directories. The earlier missing local marketplace source explains why its cache path did not activate a plugin. The corrected fixture has independent source and cache bundles with different descriptions.

## Native observations and resulting rules

| Fixture | Observed native result | Product behavior |
| --- | --- | --- |
| Personal, repository and nested same name | Personal description wins | Personal source is available; independent project copies are shadowed with its source ID |
| Same name in repository and selected subdirectory | Nearest project description wins | Nearest project source wins on 2.1.282 |
| Skills above Git root, below startup directory, recursive group container | Absent from startup catalog | Skills roots stop at the Git boundary and inspect direct child bundles |
| Shared directory aliases | One native command | One proven source, all discovery references retained |
| Manifest file symlink | Native command is listed | Parsed symlink manifests are eligible; independent bundle directories remain independent |
| Omitted `name`, renamed frontmatter | Folder fallback and explicit frontmatter command name | Native name is separate from source identity; optional names are accepted only for Claude snapshots |
| `CLAUDE_CONFIG_DIR` | Configured personal skills replace default home skills | Config root controls personal skills, settings and plugin registries |
| User `off`, selected-project `on`, local `user-invocable-only` | Off hides commands; project restores; local retains explicit command | Scalar overrides use user < selected project < local < managed |
| Ancestor settings file | Ignored from nested working directory | Skill ancestors are scanned, but settings are selected-working-directory inputs |
| `disable-model-invocation: true` | Explicit command remains listed | Explicit-only, never full disable |
| `user-invocable: false` with visibility `on` or user-only | User command remains absent | Visibility does not restore an invoker excluded by frontmatter |
| Root-only local plugin with no `skills/` or manifest `skills` key | Enabled catalog lists `root-demo:root-action`; disabled catalog omits it | Managed root bundle participates only under the same validated activation rules |
| Both invocation fields with true/false, yes/no, on/off, 1/0 and case variants | User false aliases disappear; all disable-model aliases retain user commands | Claude-specific normalization retains model-only/explicit-only/no-invoker distinctions; model enforcement is inferred from documented policy |
| Local marketplace and independent installed cache | Enabled catalog uses LOCAL-SOURCE description | Validated local marketplace source participates; independent cache copy does not supply a second command |
| Plugin homepage absent, valid HTTPS, invalid literal | Absent/valid validate successfully and load; invalid fails validation and omits command | Invalid homepage prevents positive activation; bundle remains inspectable with managed provenance and unknown activation |
| Additional unmodeled manifest fields | No full native schema oracle was run | Partial topology decoding does not establish full native manifest validity; activation constraints remain unknown |
| Global/project/local `enabledPlugins` | Project enables over global disable; local disables over project enable | Same scalar settings layering, with exact plugin provenance |
| Namespaced plugin plus `skillOverrides=off` | Plugin command remains listed | Plugin visibility is independent of ordinary skill overrides |
| `Skill` deny/ask | User command catalog remains listed | Permission governs model Skill-tool use; deny preserves explicit-only where user invocation is permitted |

Current documentation describes qualified nested names. The observed 2.1.282 startup catalog chooses the nearest project copy instead, so the adapter pins that observation. Native SDK catalog observations establish listing and source selection, not model enforcement or existing-session state.

## Evidence and capability limits

Filesystem roots, parsed settings layers, original/resolved references and plugin manifests are **observed** facts. Availability, invocation policy and precedence are **inferred** from those inputs and version-bound rules. Running-session participation and reload are **unknown**. Every writer remains unsupported. Native visibility settings target names, which can affect independent copies; plugin settings target whole plugins. Neither is exposed as an isolated source writer.

Malformed relevant native inputs produce `native-input`, incomplete coverage and unknown permission. Unknown installation-registry schemas do not establish inactive sources. Remote-marketplace activation, missing registrations and cache-only content remain unknown; explicit plugin disable and a proven nonapplicable/stale registered path are separately excluded. An ordinary alias into managed plugin content does not prove activation. Equal-priority independent definitions and shared references with different command names have unknown native winners. Noncanonical names and hidden ordinary bundle loading are not advertised as validated.

Hostless hierarchical URL forms remain unknown rather than native-valid; no HTTP-only scheme restriction is imposed. Other native URL edge cases and manifest schemas beyond the recorded bounded controls remain unverified. The adapter retains explicit activation uncertainty for manifest fields outside name/version/skills/homepage/repository; repository URL text is not constrained.

These documented assumptions remain unverified: enterprise filesystem precedence and local/remote/MDM managed enforcement; main-checkout fallback for linked worktrees; non-Git ancestor behavior; runtime loading of newly visited directories; launch flags such as `--add-dir`, bare/restricted/safe modes and `--setting-sources`; existing workspace trust; plugin aliases in permission enforcement; account sync, bundled skills and legacy command files. The product does not inventory legacy command files as skill bundles or infer account activation from downloaded folders. Session and scope explanations disclose these limits.

Official references for assumptions and rule interpretation are [skills](https://code.claude.com/docs/en/skills), [settings precedence](https://code.claude.com/docs/en/settings), and [plugin manifests](https://code.claude.com/docs/en/plugins-reference). Native observations above take priority over newer documentation where they differ.

## Public executable validation

`tests/acceptance/ale29_cli_test.go` and `ale29_repair_test.go` exercise source identity, same-name winners, shared aliases, optional names, custom versus configured roots, all applicable availability states, global/project/local settings, local plugin source versus cache, unsupported schemas, missing projects and explicit network denial. It checks actual output, exit codes and unchanged fixture trees, and includes an executable script that must never run.

`tests/acceptance/ale29_browser_test.go` drives tmux at **48x24** and **80x30**, pages actual source explanations, reads root-only plugin ownership/control limits, observes disabled-to-enabled plugin refresh, exercises a boolean alias, refreshes after an external setting edit, cycles availability filters and inspects selected-project scope. It checks terminal cell bounds and source/settings immutability. Long source paths and Claude's longer application name motivated wrapping the native header and scope line.

Normal public CLI, configured-root and direct PTY routes share the fail-closed fixture policy in `tests/acceptance/fixture_policy_test.go`. Every tmux product payload uses it too; the controller operates a separate explicit private socket. The product policy denies network, filesystem writes, real home and excluded content reads. Known absolute Claude/Codex system inputs return controlled `ENOENT`, keeping production defaults unchanged. Controller-owned models prove stat/open/directory absence, read positives, outside/symlink write denial and inherited child containment through actual ordinary and tmux routes. Failed sandbox compilation prevents the payload from running. This establishes controlled absence, not present enterprise settings, managed precedence or actual live-system content inspection.

Earlier ordinary fixture receipts lacked this system-input boundary and are preserved as historical behavior receipts, not current isolation proof. Actual historical live-content access was not established.

Inert helper regressions under `tests/native_verification` prove child invocation context and fail-closed controls without making claims about native semantics.

The repair standalone receipt is `/Users/alex/Documents/skill-viewer/.agent-loop/selected-20261013/ale-29/repair/acceptance-2/`, including the built executable, command receipts, offline profiles, fixture digests and actual `tmux/ale29/` frames. `build.md` and `checks.json` in the enclosing ticket artifact directory record the exact clean candidate SHA and final check exits. Independent review and no-mistakes belong to the coordinator and are not claimed by this build worker.
