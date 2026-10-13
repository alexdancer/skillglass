# ALE-26: browse and explain Codex skills

This slice adds read-only Codex discovery and local configured-availability explanations to the existing immutable collection snapshot. It does not complete the wider Skillglass specification. All enable, disable, project-override and delete capabilities remain unsupported. Production commands never launch Codex, invoke a model, install plugins, execute skill content, or write agent configuration.

## Use

```sh
bin/skillglass list --agent codex --project /path/to/repository/nested --json
bin/skillglass list --agent codex --availability disabled
bin/skillglass explain --agent codex --project /path/to/repository/nested SOURCE_ID
bin/skillglass read --agent codex --project /path/to/repository/nested --json SOURCE_ID
bin/skillglass browse --agent codex --project /path/to/repository/nested
```

`--agent codex` selects native user, compatibility, system, builtin and selected-project roots. Omitting `--project` selects only machine-wide roots, even when the command runs in a project. `HOME` is required; `CODEX_HOME` overrides HOME/.codex. Extra explicit roots remain inspection references and do not become runtime extra roots. Generic commands still require explicit roots and retain their existing direct-child discovery and unknown ownership/availability contract.

`--availability` accepts all, available, disabled, explicit-only, ask, shadowed, unavailable and unknown. Codex boolean configuration does not invent ask or name-based shadowing states. Availability is resolved before display filters; search/context/root filters never recalculate permission. The browser uses `a` to cycle availability, `c` to cycle reference context, `s` for scope and `r` to refresh local inputs. Detail begins with configured availability, evidence confidence and unknown session/reload state before long paths. All full provenance remains scrollable.

## Contract and decisions

The source's availability retains `state` and `reason` and adds per-reference participation, permission with configuration-layer provenance, source coexistence, invocation policy, session observation, reload and scoped capability explanations. Each evidence record states observed, inferred or unknown and carries its controlling rule, input path, relevant setting and validated version. Filesystem/config input facts and the recorded version receipt are observed; local native participation and effective permission are inferred; current inference-session state is unknown. User/global discovery scope does not prove user ownership. System and plugin content is managed.

Native `.agents/skills` and compatibility `.codex/skills` are included at **every ancestor** from the selected directory to the actual Git repository, inclusive. Repository detection recognizes normal repositories and linked worktree gitdir/commondir files; an empty `.git/` marker is insufficient. Without a real Git boundary, only the selected directory is inspected and native traversal/permission coverage is explicitly unknown and incomplete. Native roots recursively discover ordinary containers and directory aliases, stop at bundles, omit hidden containers, and separately include the observed CODEX_HOME/skills/.system root. Traversal is capped at 10,000 directories with visible incomplete coverage. Generic explicit-root discovery remains direct children.

Independent sources with the same native name coexist. The explanation never fabricates a winner. Plugin namespaces come from a valid ancestor plugin manifest when comparing native names; a cache folder identity alone never creates a namespace. Orphan aliases without a valid manifest retain their plain names. The source's skill manifest name is retained. Proven shared directory aliases retain all references and one source identity. Native roots have stable IDs independent of extra viewer roots. Merely sharing a manifest does not merge directories.

Configuration uses the pinned `github.com/pelletier/go-toml/v2 v2.3.1` parser. Canonical project trust keys govern whether project layers participate; untrusted project config is identified without reading its contents. Trusted closer project `skills.config` arrays replace lower arrays, including replacement by an empty array. Replaced and ignored layers remain visible. Only relevant skill selectors are exposed; raw config, provider keys, credentials and unrelated settings are never serialized. Optional absent roots/config are normal absence. Broken links, invalid TOML, unreadable or oversized relevant inputs report `native-input`, preserve useful output, and return incomplete coverage. Input reads require bounded regular files and use nonblocking opens to avoid FIFO races.

For validated 0.155.1 behavior, an absolute selector ending in SKILL.md matches the canonical manifest, including a directory alias. A folder selector does not disable the skill. Relative selectors and contradictory entries targeting the same canonical manifest remain unknown instead of receiving guessed semantics. YAML `policy.allow_implicit_invocation: false` yields explicit-only independently of permission; wrong types, malformed or unreadable policy remain unknown. Generic metadata parsing is stricter than Codex's native parser: a malformed generic manifest remains inspectable with diagnostics and unknown native eligibility. Missing description and file-manifest symlinks are observed native exclusions.

Local plugin activation requires all of: enabled plugin configuration, trusted selected-project local marketplace registration, matching source plugin manifest, matching installed versioned cache manifest, and exactly matching versions/skill-root declarations. No invented installed-plugin registry schema is used. Disabled plugin caches and registered stale versions are unavailable; orphan caches and unsupported registrations remain unknown. The implementation does not pick a largest version. Native user/project-root directory aliases into cached sources participate through their own discovery routes, independently of plugin activation. Only plugin-cache discovery references inherit registered-plugin activation. Canonical ownership is assessed against the entire version-cache boundary, including bundles outside declared skill roots; a symlink target outside that boundary receives no manager ownership merely from its original cache spelling. Declared cached skill roots must remain within their canonical cache version. Cache content is managed, and whole-plugin control is broader than per-skill control. Remote/workspace-managed activation remains unverified. A cache or folder alone cannot establish availability.

## Recorded native evidence

Reference version: **codex-cli 0.155.1**, observed on 2026-10-12/13. This is the validated rule version, not a claim about the installed executable or running session on another machine. The original primary probe reported **14 passing checks**: five independent names coexist; Git ancestor boundary; folder targets ineffective; manifest targets effective; alias/canonical shared matching; canonical alias deduplication; explicit API extra roots; guessed TOML skills.paths ignored; plugin source alone inactive; native installed plugin provenance; inactive plugin omitted; orphan stale cache omitted; installed plugin per-skill path disable. The repaired standalone primary utility reports **19 passing checks**, adding disabled-plugin user aliases, outside-declaration manifest namespaces, plain orphan aliases, and canonical/alias manifest targeting for those independent routes.

Reproduce with a local Codex executable, Python 3 and Git in a restricted disposable environment:

```sh
python3 tools/verify_codex.py --codex /absolute/path/to/codex --out /tmp/codex-native-evidence
python3 tools/verify_codex_eligibility.py --codex /absolute/path/to/codex --out /tmp/codex-eligibility-evidence
```

Both direct commands share the small `tools/codex_probe.py` fixture/transport helper and retain independent case ownership. The scripts use isolated HOME/CODEX_HOME/XDG_CONFIG_HOME/TMPDIR and a genuine temporary Git repository. They send initialize, skills/list, config/read and extra-root requests; the primary probe installs only a fixture local plugin into its disposable cache. No thread/start, turn/start or inference request is made. They record raw transcripts and stderr outside the repository. Codex can still read system /etc/codex/config.toml, which was empty in the recorded primary evidence. Run under an OS sandbox if host system reads must also be contained. These utilities are verification tools and are never called by browsing.

Corrected native alias evidence observes that declared-root, outside-declaration and orphan-cache user aliases load enabled even while the installed plugin is disabled, with user scope and native pluginId null. Manifest targeting still disables the canonical skill. This corrects the preliminary claim that every cache alias inherits plugin activation. Managed ownership and capability scope are separate from discovery participation. Stale-version alias participation follows the independently validated native directory-alias route as an inference; every stale topology was not independently probed.

Supplementary native eligibility evidence records that nested `group/nested` loads, hidden `.hidden/hidden-skill` and file-symlink SKILL.md do not load, missing description reports a native error, missing name falls back to the folder, and malformed `name: [` becomes the literal native name `[`. The product preserves generic parse diagnostics and uncertainty for those parser differences rather than substituting the native parser.

Critical limitation: native `config/read` reports trusted project array replacement while `skills/list` can continue using startup/user disables. The probe did not start an inference session. Local configured permission, a native inspect-list observation and actual session enforcement are distinct. Production session/reload fields therefore remain unknown; browser refresh proves only that local files were reread. The primary probe observed inspect-cache refresh via forceReload; fresh load or restart may be required for session adoption.

Other gaps: custom root markers and traversal outside Git, exact depth/metadata limits, CLI overrides supplied to a running agent, selected profiles, cloud policy, enforced requirements, remote/curated/workspace-managed plugin activation and model invocation-policy enforcement. Relevant unsupported config causes unknown coverage; absence of supplied runtime/session information is displayed as a gap rather than erasing otherwise inspectable local permission.

## Executable and terminal verification

`tests/acceptance/ale26_cli_test.go` exercises the built executable with temporary homes and genuine Git projects: native user/project/compatibility scope, recursive discovery, coexistence, alias matching, inspection-only custom roots, detail/explain equality, filter stability, trusted/untrusted/closest/empty-array precedence, folder selectors, explicit-only policy, native eligibility exclusions, plugin enabled/disabled/stale/orphan cases, malformed and missing inputs, FIFO/oversized/broken config, strict YAML policy types, unrelated-secret omission, hostile path escaping, no-Git uncertainty and unchanged fixture trees.

`tests/acceptance/ale26_cache_alias_test.go` retains the independently reproduced alias regression and checks declared/outside/orphan/stale routes, manifest-derived namespaces, outside-cache targets, per-reference participation, canonical/alias selectors, ownership/capability limits, unknown session and unchanged trees. Cache-only disabled/stale/orphan controls remain covered separately.

`tests/acceptance/ale26_browser_test.go` drives the same executable through real tmux at 48x24, 48x30 and 80x30 with long source paths. It verifies immediate summary visibility, full permission/session explanation, external config edits, retained directory selection, hidden selection under filters, availability cycling, scope and bounded terminal cells. It also verifies generic accepted global/system/managed contexts recover to all with `c`, and preserves native all/project/custom/global/system/managed cycle order. The existing suite retains PTY/browser, source identity, comparison and sandbox positive/negative controls. Word-aware wrapping preserves readable prose and still bounds long paths by terminal cells.

Run the complete checks on the final candidate:

```sh
gofmt -l cmd internal tests/acceptance
GOPROXY=off GOSUMDB=off go vet ./...
GOPROXY=off GOSUMDB=off go test -count=1 -mod=readonly ./...
tests/acceptance/run.sh /absolute/unique/evidence-directory
python3 -m unittest discover -s tests/agent_loop
shellcheck tests/acceptance/run.sh
sh -n tests/acceptance/run.sh
git diff --check
```

Acceptance evidence contains the built binary, candidate/build receipt, CLI argv/stdout/exit outcomes, tmux cells, isolation controls and before/after tree manifests. Runtime acceptance runs offline with GOPROXY=off/GOSUMDB=off. The feature-specific fixture environment explicitly supplies a disposable HOME and only the bounded native configuration locations; it cannot use the real user's HOME or skills. The separate native probes provide version evidence, not inference-session validation. Review, no-mistakes, publication and ticket closure remain separate workflow gates.

## Candidate verification outcome

The implementation candidate passed formatting, go vet, the complete uncached Go suite, the standalone built-executable acceptance runner, all 20 scheduler tests, shellcheck, shell syntax and whitespace checks. Real terminal cells were inspected at 48x24, 48x30 and 80x30. Existing generic, identity, comparison, PTY, sandbox and tree-immutability acceptance remained green. The exact commit and external evidence paths are recorded in the local run's build report for the independent frozen-candidate reviews. Native evidence above is retained as reference-version validation; inference-session enforcement and every writer remain explicitly unverified/unsupported.
