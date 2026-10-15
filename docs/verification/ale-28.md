# ALE-28 project controls

Codex 0.155.1 does not have a validated enforceable project per-skill writer. Project enable, disable and removal of an override therefore return `unsupported`, with no confirmation token and no settings change. A global setting is never substituted for the requested project scope. The ticket's supported project write journeys remain unavailable on this recorded version.

The [README](../../README.md#build-and-use) owns command usage, browser controls and exit statuses. The CLI and browser use the same `Snapshot.PreviewChange` refusal; unsupported previews have no applicable write plan.

## Native evidence

Independent disposable validation used the installed **codex-cli 0.155.1**, with the authoritative binary digest pinned by `PINNED_NATIVE_SHA256` in [the containment helper](../../tools/codex_containment.py), credential-free temporary homes and two genuine Git projects, denied network and sealed reader/writer containment controls. It tested canonical manifest and shared directory alias selectors, independent same-name sources, trusted and untrusted project configuration, fresh app-server instances, `forceReload`, global denial with project enable, and project disable with global permission. `config/read` showed trusted project array replacement, while native `skills/list` retained startup/user skill choices. Canonical global manifest and CLI override controls worked; folder targeting did not.

Starting an idle thread and reloading did not establish project enforcement. No inference turn or skill workload ran. Thread start exposed no per-skill permission state; `thread/read` with turns was unsupported. These observations establish that project enforcement is unverified, not that every possible inference path ignores project settings. A local configuration inference cannot authorize a writer that promises native enforcement. Fresh version-specific evidence is required before enabling project writes.

Reproduce the project limitation and the built product's no-write refusals with:

```sh
python3 tools/verify_codex_project.py --codex /absolute/path/to/codex \
  --product /absolute/path/to/skillglass --isolation-probe /absolute/path/to/isolationprobe \
  --out /absolute/path/to/fresh-evidence
```

Build the product from `./cmd/skillglass` and probe from `./tests/acceptance/cmd/isolationprobe` using offline Go dependencies. The portable harness reuses the sealed containment and pin checks of [the global harness](ale-27.md#native-evidence), proves controls before launching native or product payloads, and records native version/hash, fresh-server transcripts for both projects, positive global exact-source controls, per-action product refusals, and retained configuration/content bytes. A different native binary is refused by the pin check before execution. The expected limitation checks also fail if observed project behavior differs; neither refusal permits enabling a writer without fresh scope validation. It starts no threads or inference. The independent session probe described above is retained separately in the run artifacts.

Project arrays replace lower arrays in effective configuration, including when a project array has only an unrelated selector. Removing one row would therefore not generally restore that source's global inheritance while preserving unrelated rows. No copied global selectors, empty-array fallback, global edit, shared-source edit or source-content workaround is offered.

The coordinator retains the independent raw native transcripts, executable/profile digests, containment controls, exact invocation receipts and report under `.agent-loop/ale-28-20261014/native/`. This ignored evidence is local validation material, not a portable production runtime dependency. The preexisting [global writer](ale-27.md) remains the only supported Codex writer, including its stale-preview, cancellation, safe-write, persisted-result and reload behavior. Session adoption remains unknown; fresh load or restart may be required, and this tool does not operate native sessions.

## Product verification

Unsupported project previews retain the selected project in JSON and show its full path in ordinary output, or `Inspection project: none selected` when absent. Terminal control characters in displayed paths remain escaped. The browser scope menu, preview and result retain full inspection context; scroll with arrows, Page Up/Down and Home/End where needed. `TestDecisionALE28UnsupportedPreviewRetainsProject` checks public human and JSON context for all three actions; its companion checks absent context and hostile path escaping. The 48/80-column browser journey pages to the complete long project path in scope and refusal views.

`TestALE28ProjectControlRefusesNativeIneffectiveScope` drives the built executable. It requests enable, disable and inheritance with and without forged confirmation, preserves an existing trusted project override and unrelated project configuration, checks a second project inheriting a global disable, and verifies the whole fixture tree is unchanged. Global inheritance removal receives a specific refusal. The first run failed because the old project reason contained no recorded native evidence; `inherit` was also absent from public dispatch.

`TestALE28BrowserProjectAndInheritanceRefusal` drives real tmux terminals at 48 and 80 columns through project enable/disable/removal, unavailable confirmation and cancellation. It checks bounded cells and unchanged sources/settings. Existing global executable and terminal acceptance remains required. No project write-success, project write-failure or project stale-write claim is made: no project write plan exists. Those supported-write journeys remain unavailable until the native capability is proven.

Run the [combined verification procedure](ale-24.md#reproduce-the-acceptance-evidence), with its proved contained Go test launcher, after the final change. Exact final commands and outcomes belong in the build report; independent review and no-mistakes remain coordinator-owned. Leave ALE-28 open and report the native capability limitation.
