# ALE-28 project controls

Codex 0.155.1 does not apply the tested project per-skill choices during native skill loading. Project enable, disable and removal of an override therefore return `unsupported`, with no confirmation token and no settings change. A global setting is never substituted for the requested project scope. This PR delivers the ticket's unsupported-scope behavior; actual project controls remain unavailable, and ALE-28 stays open.

The [README](../../README.md#build-and-use) owns command usage, browser controls and exit statuses. The CLI and browser use the same `Snapshot.PreviewChange` refusal; unsupported previews have no applicable write plan.

## Native evidence

The initial disposable inspection validation used the installed **codex-cli 0.155.1**, with the authoritative binary digest pinned by `PINNED_NATIVE_SHA256` in [the containment helper](../../tools/codex_containment.py), credential-free temporary homes and two genuine Git projects, denied network and sealed reader/writer containment controls. It tested canonical manifest and shared directory alias selectors, independent same-name sources, trusted and untrusted project configuration, fresh app-server instances, `forceReload`, global denial with project enable, and project disable with global permission. `config/read` showed trusted project array replacement, while native `skills/list` retained startup/user skill choices. Canonical global manifest and CLI override controls worked; folder targeting did not.

Starting an idle thread and reloading did not establish project enforcement. That initial inspection ran no inference turn or skill workload. Thread start exposed no per-skill permission state; `thread/read` with turns was unsupported. A subsequent session experiment, described below, tested actual native catalog construction and skill-body loading. A local configuration inference alone cannot authorize a writer that promises native enforcement.

### Actual native sessions

On October 15, 2026, a separate frozen harness completed three baseline calibration turns and a 33-turn matrix through `app-server --stdio`. Each matrix turn used a fresh native process and thread. Startup, requested/returned thread and turn working directories agreed, and both projects were trusted Git roots sharing the same disposable home. A deterministic local Responses server captured the requests emitted by Codex and returned a fixed assistant response. No external model inference was used.

The matrix used canonical absolute `SKILL.md` selectors. Unique descriptions identified the target, an independent same-name copy and a sibling skill. Body-only markers were absent from client inputs and mock responses. The native catalog contained exact source paths, and structured skill input selected the target's exact manifest. These controls distinguish catalog exposure and native body injection from model selection, compliance or skill execution.

| Configuration | Native target catalog | Exact target body on structured selection |
| --- | --- | --- |
| No override, in either project | Present | Present |
| Global target disabled, in either project | Absent | Absent |
| Project A target disabled | Present | Present |
| Project A disabled, fresh-process repeat | Present | Present |
| Project A enabled over global disable | Absent | Absent |
| Project B without a project choice | Follows global choice | Follows global choice |
| Project override absent, global disable | Absent | Absent |

Project disable and enable appeared in accepted `config/read` output with project-layer origins, but neither changed catalog exposure or exact target-body loading. Global controls worked in both projects and preserved the independent same-name copy and sibling catalog entries. With the target globally disabled, text-only `$name` selected the remaining independent same-name copy; exact structured target selection did not substitute it. These results establish a native capability blocker on this tested path, beyond the earlier inspection-only limitation.

All 33 matrix turns completed with matching request-header thread/turn identities, one captured request per turn, raw request/response hashes, unchanged fixture hashes and verified process-group teardown. The harness allowed its assigned localhost endpoint and rejected another live local port and an external address before native execution. Credential-bearing requests were rejected; no native credential headers were observed. An independent read-only review passed 567 integrity and observation checks over 84 sealed evidence files and 42 invocation receipts.

The native binary SHA-256 was `8eaf1ad12fe6bf89b1710330f58900014322c7c5af677e43be116d8ac5fc0a9e`. The session harness manifest SHA-256 was `c304905aa30951bd0564578c9cff3e1c4d25921c50b7535ae3087c3bb25abe40`; the sealed evidence manifest was `33072801bfe6c256c741508108766b2f15b09b430f4175113640efe66e8ea24c`, and the aggregate matrix result was `b9529240a36a67ebb4eeff36de92c9b1ce664277ba2709e093e8d1346503c8eb`.

[OpenAI Codex issue #20210](https://github.com/openai/codex/issues/20210) corroborates this limitation: maintainers described global-only skill toggling as intended behavior, treated project filtering as an enhancement, and later closed the request. Its historical source explanation is not an independently matched source audit of this installed binary.

This evidence is limited to the pinned macOS arm64 binary, local Responses provider, canonical manifest selectors and fresh sessions. It does not establish every selector, desktop/provider, platform or version's behavior. Alternative actual-session selectors and shared aliases, managed/CLI restrictions, untrusted projects, working-directory disagreement and existing-session adoption remain untested. Static absent/unrelated-row states do not prove a successful override-removal lifecycle. Fresh version-specific capability evidence and the remaining writer acceptance journeys are required before project writes can be enabled.

### Reproducing the inspection and product refusals

Reproduce the project limitation and the built product's no-write refusals with:

```sh
python3 tools/verify_codex_project.py --codex /absolute/path/to/codex \
  --product /absolute/path/to/skillglass --isolation-probe /absolute/path/to/isolationprobe \
  --out /absolute/path/to/fresh-evidence
```

Build the product from `./cmd/skillglass` and probe from `./tests/acceptance/cmd/isolationprobe` using offline Go dependencies. The portable harness reuses the sealed containment and pin checks of [the global harness](ale-27.md#native-evidence), proves controls before launching native or product payloads, and records native version/hash, fresh-server transcripts for both projects, positive global exact-source controls, per-action product refusals, and retained configuration/content bytes. A different native binary is refused by the pin check before execution. The expected limitation checks also fail if observed project behavior differs; neither refusal permits enabling a writer without fresh scope validation. This command starts no threads or inference and does not rerun the session matrix. The separate session probe has host-specific paths and is retained with its archived source and raw evidence in `.agent-loop/ale-28-session-20261015/`; it is not a portable checkout test.

Project arrays replace lower arrays in effective configuration, including when a project array has only an unrelated selector. Removing one row would therefore not generally restore that source's global inheritance while preserving unrelated rows. No copied global selectors, empty-array fallback, global edit, shared-source edit or source-content workaround is offered.

The coordinator retains the initial inspection transcripts, executable/profile digests, containment controls, exact invocation receipts and report under `.agent-loop/ale-28-20261014/native/`, and the actual-session matrix and independent review under `.agent-loop/ale-28-session-20261015/`. This ignored evidence is local validation material, not a portable production runtime dependency. The preexisting [global writer](ale-27.md) remains the only supported Codex writer, including its stale-preview, cancellation, safe-write, persisted-result and reload behavior. Existing-session adoption remains unknown; fresh load or restart may be required, and this tool does not operate native sessions.

## Product verification

Unsupported project previews retain the selected project in JSON and show its full path in ordinary output, or `Inspection project: none selected` when absent. Terminal control characters in displayed paths remain escaped. The browser scope menu, preview and result retain full inspection context; scroll with arrows, Page Up/Down and Home/End where needed. `TestDecisionALE28UnsupportedPreviewRetainsProject` checks public human and JSON context for all three actions; its companion checks absent context and hostile path escaping. The 48/80-column browser journey pages to the complete long project path in scope and refusal views.

`TestALE28ProjectControlRefusesNativeIneffectiveScope` drives the built executable. It requests enable, disable and inheritance with and without forged confirmation, preserves an existing trusted project override and unrelated project configuration, checks a second project inheriting a global disable, and verifies the whole fixture tree is unchanged. Global inheritance removal receives a specific refusal. The first run failed because the old project reason contained no recorded native evidence; `inherit` was also absent from public dispatch.

`TestALE28BrowserProjectAndInheritanceRefusal` drives real tmux terminals at 48 and 80 columns through project enable/disable/removal, unavailable confirmation and cancellation. It checks bounded cells and unchanged sources/settings. Existing global executable and terminal acceptance remains required. No project write-success, project write-failure or project stale-write claim is made: no project write plan exists. Those supported-write journeys remain unavailable until the native capability is proven.

Run the [combined verification procedure](ale-24.md#reproduce-the-acceptance-evidence), with its proved contained Go test launcher, after the final change. Exact final commands and outcomes belong in the build report; independent review and no-mistakes remain coordinator-owned. Leave ALE-28 open and report the native capability limitation.
