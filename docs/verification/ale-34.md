# ALE-34 Pi controls

Pi 1.1.0 on macOS supports ordinary source controls through exact lexical resource settings in `PI_CODING_AGENT_DIR/settings.json` (default `HOME/.pi/agent/settings.json`) and selected launch cwd/.pi/settings.json. CLI enable/disable/inherit and the terminal browser share the established preview, target-specific confirmation, stale checks, safe write and persisted-result verification. Preview performs no write; cancellation discards it. The viewer never launches Pi.

## Native contract

Independent contained validation on 2026-10-16 recorded installed `@earendil-works/pi-coding-agent` 1.1.0 and Node v22.22.3. The native module/source hashes, assertions, containment controls and RPC receipts are in the local run's `native/final/native.json`, `commands.json`, `containment.json` and `invocations/` under `.agent-loop/ale-34-20261016/`. The checked-in reproducible probe is:

```sh
python3 tests/native/pi-controls-contained.py /path/to/node_modules/@earendil-works/pi-coding-agent /private/tmp/ale34-native-evidence
```

It first proves containment, then imports the recorded native settings/resource manager and loader in disposable HOME/project, with network, live settings, extensions, credentials, installs, prompts, models and skill workloads excluded. A fresh offline bundled CLI RPC observes its commands catalogue. Assertions cover exact targets, patterns, operator precedence, aliases, independent names, project override/removal and cached settings reload. Existing ALE-33 discovery evidence remains in [ale-33.md](ale-33.md). Native loader evidence and executable Go writer evidence are distinct.

The writer emits absolute lexical instruction-file selectors using the [native resource matching rules](ale-33.md#observed-rules-and-adapter-behavior). The planner retains each occurrence's native base when matching existing relative file or directory choices, selects all applicable aliases of the exact source, and never substitutes a canonical path for an alias.

Project arrays do not filter inherited global occurrences. An inherited project choice therefore registers its exact instruction file as a configured project resource, together with its exact plus/minus choice. That project occurrence consumes the canonical slot even when disabled. Global and other-project settings stay unchanged. Native package insertion can already own the lexical resource; package controls remain unsupported.

## Inheritance and preservation

An absolute inherited instruction-file registration plus its exact plus/minus directive is the native project override representation. **Inherit explicitly removes that native pair**, irrespective of its author, and confirms the source remains reachable through inherited discovery. Preview displays the complete before/after skills array and the selected settings path. This does not assert tool ownership of settings. For project-only sources, inherit removes the exact local choice while retaining their discovery registration.

A pre-existing literal registration that discovers the selected inherited source is user discovery state, including equivalent absolute, relative and tilde file paths or directory roots. Inherit preserves a lone registration byte-for-byte, and a new override request refuses to turn that discovery into a removable pair. If a native pair is mixed with another retained registration of the selected source, inherit refuses rather than report inheritance while the retained project resource still wins. Other configured roots and patterns remain. A registration that cannot be removed without losing source discovery is refused. No sidecar ownership state is created, and no shared content, managed cache, package registration or broad exclusion is edited to simulate inheritance.

The JSON patch retains unrelated fields and untouched resource string spellings. Duplicate keys, legacy resource objects, malformed arrays and ambiguous registration combinations are refused. Project registration paths containing native resource operators are unverified and refused; exact global operators remain distinct. Unknown pattern/ignore semantics or incomplete discovery prevent unbounded mutation.

## Outcomes and refresh

Global and project actions write only their explicit settings destination. A project destination aliasing global configuration is refused. Project operations require the selected trusted launch project; controls never change trust settings. A higher-priority project choice or independent name winner can prevent enable, producing a non-success refusal with no fallback. Disabling the selected name winner can expose another independent copy; that copy's own permission and contents are preserved and the preview discloses the possibility.

Capability reporting checks native eligibility, scope, identity and the proposed JSON patch without rescanning the collection for every source. It explicitly requires preview effect checks; supported capability does not guarantee the requested contextual result. Explicit previews and apply rechecks evaluate prospective changes through the existing Pi discovery/availability implementation using an in-memory settings replacement. Independent source membership and native permissions must remain unchanged. Source/configuration identities and dependencies are pinned into the shared one-use confirmation token. Apply recomputes the preview before writing; external settings, trust, source and alias changes invalidate it. Settings links/hardlinks, unsafe ownership/permissions/ACLs/flags and destination identity changes retain the established safe-write refusals.

Applied, refused, unsupported and failed outcomes stay distinct. Zero status requires verified persistence and achievement of the selected source result. Saved choice does not fabricate native permission. Override removal can succeed while restoring an inherited disable. Persistence and contextual effective availability remain separate from use in a running session.

The recorded cached SettingsManager needed `reload()` before resource resolution observed external settings changes; fresh offline native sessions observed saved choices. Outcomes therefore report **reload required** for existing cached settings. Interactive `/reload` adoption and model execution were not tested; no running session is restarted automatically.

## Executable and terminal verification

`tests/acceptance/ale34_cli_test.go` exercises the built executable in isolated temporary homes/projects. It checks global controls, pattern preservation, exact minus precedence, configured resources, standalone Markdown files, project enable/disable and inherit, native override-pair removal, lone registration preservation, shared aliases, independent name conflicts, stale settings/source/trust/alias changes, write denial, malformed/legacy/duplicate-key settings, config links and no-fallback refusals. It inspects actual settings/content bytes, exit status and untouched filesystem state.

`tests/acceptance/ale34_inheritance_test.go` covers equivalent user registrations, mixed-pair refusal, ordinary override roundtrips and project-only controls. `tests/acceptance/ale34_native_bases_test.go` covers relative manifest/directory choices for global, selected-project and ancestor `.agents` occurrences, configured references and aliases, and refusal when removing a choice would affect an independent source.

`tests/acceptance/ale34_browser_test.go` exercises confirmation/cancellation, scope selection and stale confirmation at 48 and 80 columns. Actual terminal frames are saved under `tmux/ale34/` in the verification evidence. The shared [containment contract](ale-23.md#runtime-containment-boundary) owns the executable/PTY launch controls.

Run focused verification with the existing fixture harness:

```sh
SKILLGLASS_TEST_RUN='^TestALE34' sh tests/acceptance/run.sh /private/tmp/ale34-public-evidence
```

macOS sandbox-exec needs permission to establish the test's inner containment. Run the complete Go suite with the coordinator's contained collection-test launcher, followed by existing Python, formatting, vet and shell checks. Exact final commands, results, candidate head and binary hashes belong to the build receipt; this document does not claim that merely inspecting the native probe verifies the Go writer.
