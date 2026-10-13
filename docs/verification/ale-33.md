# ALE-33 Pi browsing evidence

## Validated application

The recorded native application is `@earendil-works/pi-coding-agent` 1.1.0 on macOS, whose `pi --version` returned `1.1.0` on 2026-10-13. This adapter does not claim rules for older `@mariozechner` builds or other operating systems. It does not execute Pi when browsing.

Primary sources are the installed package's `dist/core/package-manager.js`, `skills.js`, `settings-manager.js`, `trust-manager.js`, and `project-trust.js`. The native receipt records their SHA-256 hashes. The [official Pi repository](https://github.com/earendil-works/pi) and its [skill documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/skills.md), [resource settings](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/settings.md), and [package documentation](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/packages.md) provide context. The installed version, rather than current main, determines this implementation.

Run the reproducible native probe against an explicitly selected installed package.

```sh
node tests/native/pi-read.mjs /path/to/node_modules/@earendil-works/pi-coding-agent
```

The probe creates an `ale33-native-*` disposable home and nested project. It imports the native settings manager, package resource manager, trust resolver and skill loader. It forbids missing-package installation. It also launches the real bundled Pi CLI in RPC mode with `--offline --no-session --no-extensions --no-mcp --no-context-files --no-tools`, under a network-denying macOS sandbox, and requests only `get_commands`. The child receives a credential-free environment. No prompt, skill workload, bundled script, extension, install or model request runs. The disposable fixture is removed after the receipt is emitted.

## Observed rules and adapter behavior

| Native rule | Viewer behavior and evidence |
| --- | --- |
| Global resources include `PI_CODING_AGENT_DIR/skills` or `HOME/.pi/agent/skills`, and `HOME/.agents/skills`. | Optional roots are displayed even when absent. `HOME` bounds discovery. `PI_CODING_AGENT_DIR` can independently select the native agent directory. |
| Project resources include selected cwd `.pi/skills`, and cwd/ancestor `.agents/skills` through the nearest existing `.git` marker. Without a marker, Pi walks to the filesystem root. | The adapter follows the recorded Pi marker rule, rather than Codex's repository validation. Ancestor `.pi/skills` and ancestor `.pi/settings.json` are not inherited. |
| Project participation requires trust when project resources exist. Native persisted trust uses nearest canonical ancestor in `agentDir/trust.json`, then global `defaultProjectTrust`. | Remembered or default allowed state is inferred. Denied project references are unavailable. An unresolved ask is reported as `ask`. Session trust flags and extension decisions remain unknown. Project settings are read for inspection even when trust prevents their participation. |
| Name precedence is project settings, project automatic resources, global settings, global automatic resources, packages. First enabled name wins. | Independent copies remain separate IDs. Winners and shadowed sources identify each other. Same-tier file discovery follows the recorded macOS filesystem ordering. Shared references preserve one bundle identity. |
| Resource settings are not one merged global/project array. Paths resolve relative to the owning `agentDir` or cwd `.pi`. | Project exclusions do not disable inherited global resources. The explanation includes the owning file and applicable `skills` or `packages.skills` rules. |
| Plain top-level entries independently discover file or directory roots. Glob patterns filter these discovered candidates. `!` excludes, exact `+` restores, exact `-` finally excludes. | Directory, manifest-file and standalone Markdown paths are supported. Exact selection checks lexical occurrence paths, including aliases, rather than substituting canonical targets. Glob-only entries do not create new top-level roots. |
| Native Pi auto roots load direct Markdown files and recursive `SKILL.md` bundles. `.agents` auto roots load recursive bundle manifests and nested Markdown files, but skip direct ordinary Markdown files. Configured directory roots collect Markdown recursively. Hidden entries and `node_modules` are omitted unless explicitly selected as roots. | Native traversal is application-specific and bounded. Generic explicit viewer roots still discover a root bundle or direct child bundles. Source identities do not merge independent files or directories. |
| A description is required. Missing or non-string names fall back to the parent directory. `disable-model-invocation: true` omits a skill from the model prompt. | Native name evidence, unavailable invalid manifests and `explicit-only` state are shown. `enableSkillCommands=false` is described separately because it disables command registration without disabling discovery. |
| Project package registrations replace global registrations with the same native identity. A package `skills: []` disables its skill resources. A configured package owns its resources. | Local package convention and `pi.skills` exact paths are read. Package provenance includes source registration and observed version. Managed cache entries without participating registration are unavailable, not enabled from folder presence. |

Native source checks and executable fixtures are distinct evidence. Viewer availability remains inferred from current local files. Neither the native probe nor the viewer observes the user's running sessions.

## Explicit limitations

The scope and relevant source explanations disclose uncertainty for ignore-file rules, complex minimatch braces and extglobs, package manifest glob expansion and filter overrides, and `autoload=false` package deltas. These cases produce native-input diagnostics rather than fabricated availability. Unsupported remote URL forms also report incomplete coverage. Managed npm legacy fallback outside the configured agent directory is not searched, and pinned remote version satisfaction is unknown. No network query or installed-package-manager subprocess is launched by the viewer.

Native `--skill`, `--no-skills`, `--approve`, `--no-approve`, SDK overrides and extension-provided resources are session-specific inputs the viewer cannot observe. Native reload and model execution are not verified. All enable, disable, project-override and delete writers remain unsupported.

A standalone Markdown file is its own source. It has no proven bundle ownership or deletion scope. It can be listed, read and explained. Existing bundle comparison reports incomplete coverage when given standalone files. It never scans or compares an entire parent directory on behalf of one file.

## Public acceptance and terminal inspection

`tests/acceptance/ale33_cli_test.go` verifies global/project precedence, independent conflicts, inherited resources, same-tier winners, standalone reading and name fallback, native explicit-only metadata, configured roots, include/exclude rules, symlink reference selectors, package ownership and inactive resource filters, unknown invalid inputs, unresolved/denied trust, availability filters, read-only refresh and fixture immutability.

`TestALE33PiOfflineBrowseAndCompare` runs list, read, explain and bundle compare with no network, writes or child execution. Its sandbox permits only fixture contents, the executable and system runtime files, plus literal ancestor directories needed for anchored comparison. The common isolation acceptance also proves denied network/write/child-execution controls with positive controls. The fixture HOME and explicit project boundary prevent falling back to the real home.

`tests/acceptance/ale33_browser_test.go` runs a focused tmux journey at 48 and 80 columns. It verifies visible Pi/project context, read and session explanation, explicit refresh after an external fixture change, availability cycling and the validated-version scope. The receipt stores actual terminal frames under `acceptance/tmux/ale33/`; review those after the final run.

The worker receipt contains exact formatting, vet, Go test, standalone acceptance, Python scheduler test, shell syntax, ShellCheck and diff-check commands, exit codes, full candidate head and artifact paths. Independent Standards and Spec reviews and no-mistakes are coordinator-owned later gates.

## Historical failed probe

Before isolation was corrected, `pi --help | head -120` attempted native settings initialization against the live home. Pi printed `Warning: Invalid settings file /Users/alex/.pi/agent/settings.json: EPERM: operation not permitted, mkdir '/Users/alex/.pi/agent/settings.json.lock'`. This is a failed isolation probe, not passed validation. No before-state receipt exists for the live paths, so this document does not claim proof that every possible live effect was absent. All subsequent native behavior probes use only disposable HOME and configuration. No live settings or skills were intentionally edited.
