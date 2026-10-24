# ALE-86 consolidation evidence and limits

This record supplements [companion setup](../companion-setup.md) with the
bounded native relocation observations and the public-executable acceptance
scope for `skillglass-consolidate`. It distinguishes the deterministic CLI
contract from application-loader observations. None of these checks proves
that an already-running session adopted a changed filesystem.

## Native relocation observations

Each test used a disposable fixture under `/private/tmp` and queried the
application's actual catalog after replacing a personal skill directory with a
symlink to a private canonical directory. The skill contained an inert
`SKILL.md`, a text reference, and a non-executed 0755 script. An unrelated
sibling was checked for preservation. No model request, skill workload, tool
execution, or network access was part of these probes.

| Application | Pinned runtime | Observation | Limit |
| --- | --- | --- | --- |
| Codex | `codex-cli 0.162.1`; executable SHA-256 `74c6a6d263f40d65f99c27a815275f5b4ed0bcc2baecc396b4cf3255498bdfc0` | App-server `skills/list` reported the selector-target skill disabled before and after moving its complete directory and replacing the exact original path with a symlink. `config/read` and the config bytes/hash stayed identical. The sibling stayed enabled; two pre-existing `.agents/skills` aliases and a `.codex/skills` alias still reached the same bundle. | This is a separate 0.162.1 relocation profile. It does not extend the discovery/control adapter's 0.155.1 baseline or establish another Codex version, another selector shape, or a running-session result. |
| Claude Code | `2.1.309`; executable SHA-256 `5eadc0009cdcdbc0d06e58110d7b2baf88ced6088b1c33a60a21217fe410619c` | The native initialize catalog listed one `portable-probe` before and after relocation. Its manifest, text resource and executable resource resolved through the original personal-path symlink with identical bytes and modes; the sibling was unchanged. | Separate from adapter baselines 2.1.282/2.1.300 and companion setup catalog 2.1.303. This exact profile does not prove another version, disabled-selector behavior, or a current session. Its sealed boundary retains the documented allow-default-read and inherited child-identity limits. |
| OpenCode | `1.17.15`; executable SHA-256 `7bdefaeaef5cc4f661988eaba00de047f5f65547fd22a3bed5ba7c4d86a275d3` | `--pure debug skill` found one `portable-probe` before and after relocation (37/37 bounded checks passed). The catalog kept the personal alias location; filesystem resolution reached the canonical `SKILL.md` and its reference resource. The sibling was unchanged. | Version- and fixture-specific observation. It does not establish all OpenCode versions or current-session adoption. |
| Pi | `1.4.0`; CLI bundle SHA-256 `e79626f2dd6f94aa45d30f3fa63cd84319a6eefcd150b353cfaf274366926774` | The native catalog kept one `portable-probe` with its original personal `filePath` and `baseDir`; the manifest and referenced resource resolved through the alias. The 0755 inert file and sibling were unchanged; 18 bounded checks passed. | This is a separate 1.4.0 observation. It does not replace or extend the maintained ALE-50 Pi 1.1.0 contract. The fixture's temporary directory key was not asserted to equal SkillGlass's bundle fingerprint. |
| Hermes Agent | `0.20.0` pinned source revision `5a16635f409e0f5e3d754bb6256bd27707bcec4c`; Python 3.11.15 runtime SHA-256 `e3938f2a272dafdc33f3dd12b093dfc85354629476cb97a7d7d4a7201b8482dd`; `agent/prompt_builder.py` SHA-256 `d420eeafd237d7beccb91bf68014f1bc6cb96d3e06ec45cb6c707587d25b909b` | The actual `agent.prompt_builder.build_skills_system_prompt` index retained `portable-probe` and an unchanged sibling before and after relocation, under both default HOME `.hermes/skills` with no `HERMES_HOME` or `active_profile`, and one direct `HERMES_HOME=~/.hermes/profiles/<name>` path. The lexical personal alias remained in the index; manifest, text and executable resources resolved through the private library with bytes and modes unchanged. | This is a bounded Hermes 0.20.0 source/runtime observation, not a Hermes binary hash. It does not prove CLI/startup/gateway/session behavior, active_profile selection, other roots/config, disable/reload or session resource reads. The default profile applies only when the marker is absent and `HERMES_HOME` is unset; the explicit profile applies only to that direct profile-root shape. |
| Gemini CLI | Not run | No matching native personal-directory relocation profile is recorded. | Read/discovery evidence does not prove relocated catalog behavior. Share/archive effects remain unsupported until a contained native profile is recorded. |

The CLI records the exact profile IDs, observed versions, artifact kind and
hash, observations and limits in each scan. Hermes also reports the separately
recorded source revision and loader source hash; neither is mislabeled as a
binary hash. A share plan requires a matching profile
for every physical source-owning application and every selected destination
that would receive a new link; an archive requires a profile for every source
owner and is allowed only when an approved shared winner recreates that exact
native source leaf with a profiled symlink. No profile currently proves a
standalone native-source absence, so an archive that leaves the leaf missing is
refused. Missing profiles leave sources inspectable and keepable but block
those moves. An explicit unknown-portability approval cannot waive the native
mechanism gate. These profiles are the exact test observations above, not claims
about the installed application version or current session.

For the Codex disabled-choice case, the exact selector was an absolute
`skills.config.path` ending in the existing source's `SKILL.md`, with
`enabled = false`. The before/after catalog and config responses, config hash,
tree manifests, aliases, invocation receipts, binary pin, and sealed-boundary
results were retained at
`/private/tmp/skillglass-ale86/native-codex-relocation-final/evidence/` for
this run. The probe first proved fixture reads/writes and native launch, then
proved denials for outside reads/writes, symlink escape, network, unapproved
child execution, and live home/config/skills access. The app-server calls were
limited to `initialize`, `initialized`, `skills/list` with `forceReload`, and
`config/read` with layers. The exact 0.162.1 binary was checked before launch;
the repository's older 0.155.1 containment pin was not treated as evidence.

The Claude, OpenCode and Pi receipts and native catalogs are retained at
`/private/tmp/skillglass-ale86/native-claude-relocation/`,
`/private/tmp/skillglass-ale86/native-relocation-matrix/opencode-final/` and
`/private/tmp/skillglass-ale86/native-relocation-matrix/pi-final/`. OpenCode's
repository containment regression passed. Pi's boundary checks denied live
home, external canary reads, outside writes, network, and child execution.
Both results are bounded observations, not portable promises. They do not
establish whether a product is installed in a particular user's environment
or whether a current session has loaded the skill.

Hermes receipts for the default-root and explicit-profile index probes are at
`/private/tmp/skillglass-ale86/native-hermes-default-relocation/receipt.json`
and `/private/tmp/skillglass-ale86/native-hermes-relocation/receipt.json`.
The current run's immutable retained native-evidence set is also recorded under
`.agent-loop/ale-86/native-evidence/`, with manifest
`.agent-loop/ale-86/native-evidence-manifest.json`. This is ignored run evidence,
not a shipped application artifact. The retained evidence manifest SHA-256 is
`84b88205457011feb6de9ecb68f241f36273bc185b5cbc72246e6b3ee91c6f42`; it covers
the native proof trees, not product source or generated documentation.

## Public executable acceptance

The public tests build and invoke the actual `skillglass` executable through
the acceptance fixture's sealed macOS boundary. Each fixture sets HOME, XDG
paths, working directory, and PATH explicitly; native roots, test bundles,
config and output live under the disposable fixture. Tests do not fall back to
the live home directory.

Run the ALE-86 public journeys with:

```sh
SKILLGLASS_EVIDENCE=/private/tmp/skillglass-ale86/delivery-matrix-tests \
GOCACHE=/private/tmp/skillglass-ale86/go-cache GOPROXY=off GOSUMDB=off \
go test ./tests/acceptance -run '^Test(ALE86|EDIT1|OWN1|REC)' -count=1 -v
```

Coverage includes selected Codex/Pi/OpenCode sources with excluded Hermes
unchanged; exact indirect CodeAF route acknowledgements; omitted acknowledgments
and stale/extra route IDs refusing; remembered applications, roots and library;
`--clear-roots`; preview and exact apply; verified no-op and selection-only
reporting; fresh recovery; stale source/selection refusal; exact disabled
Codex selector preservation with config bytes unchanged; custom roots and
library overlap refusal; subset portability and unknown approval; identical
complete bundles with large opaque assets; divergent resource winner/archive
recovery; managed Codex plugin exclusion; excluded Hermes physical ownership;
nested-source protection; keep-only behavior without a native relocation
profile; unknown portability approval failing to waive an unsupported
destination; required exact native-profile IDs on successful effects; and exact
default/named companion setup bytes.

`TestOWN1SelectedPersonalSourcesAreNotOwnedByForeignCodeAFRoots` protects
selected Codex/OpenCode personal sources from false ownership due only to a
foreign CodeAF loader route. `TestEDIT1EditedCanonicalLibrarySourceCanBeDeduplicatedOnFreshRun`
protects a user-edited canonical source's stable address, reuses it for a fresh
identical copy, proves successful recovery before any later edit, and then
proves recovery refuses to overwrite a newer edit.

These public tests validate Skillglass's deterministic scan/preview/apply and
setup effects against fixture files. The disabled-selector test asserts the
adapter's interpreted state and unchanged configuration; the separate native
Codex 0.162.1 probe above is the evidence that the tested native catalog kept
the disabled state across relocation. Neither assertion is a semantic model
test or a claim that the application used the bundle in a live session.
