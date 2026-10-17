# Gemini CLI scoped controls

The writer contract is validated against real `@google/gemini-cli` **0.63.0** in disposable, credential-free, network-denied fixtures. The maintained probe is [verify_gemini_controls.py](../../tools/verify_gemini_controls.py). It records 22 native invocations, exact arguments/environment/exits, settings before/after and bundle SHA-256 hashes:

```sh
python3 tools/verify_gemini_controls.py /path/to/node_modules/@google/gemini-cli /tmp/gemini-control-evidence
```

The package must already exist. The probe uses the existing Gemini containment profile, does not install software, and submits no prompt, skill activation, hook or extension workload. Parent code writes evidence outside the contained fixture. `/dev/null` is the explicit nonpersistent write sink. Runtime and package paths are canonicalized. Native version mismatch fails validation.

## Supported choices

CLI and keyboard browser share source-bound `PreviewChange` and explicit-confirmation `Apply`. Preview is read-only. Repeat the exact request with its `--confirm gemini1:...` token; browser `D`/`e`, `g`/`p`, `i`, `y` and Enter/Escape select disable/enable, scope, inherit, confirm and cancel.

Global disable appends one native name to the user's `.gemini/settings.json` `skills.disabled` array. Global enable removes every comparison-equivalent spelling from that array. `GEMINI_CLI_HOME` selects the native home consistently. A trusted explicit project's disable or enable edits only its `.gemini/settings.json`; inherit removes only its matching project entries. Empty arrays remain valid inheritance. Ordinary enabled skills remain **ask**, preserving native activation consent.

Native disabled arrays union across admitted settings layers and match names case-insensitively. Therefore project enable cannot override a global disabled name. Enable refuses when another layer or the disabled subsystem prevents its result and never changes that broader setting. Inherit succeeds when its project entry is removed even if inherited global state remains disabled. Global choices are checked both without a selected project and within the inspected project, preventing a project scalar from masking an unachievable global result.

Native `skills enable` is deliberately never invoked by the product: 0.63.0 accepts no scope flag and removes entries from both workspace and user settings. Its removal uses exact case even though disabled matching is case-insensitive. Direct bounded native settings patches avoid that broader effect.

## Preservation and refusals

The Gemini-specific typed JSONC document patches only disabled-name string/comma tokens or inserts the required array/object. Unrelated settings, number and escaped-string spellings, comments, other disabled names, source content and other applications remain intact. Duplicate object keys, unpaired UTF-16 escapes, invalid shapes and environment-expanded disabled names refuse instead of normalizing ambiguous input. Existing file modes and metadata use the established safe commit protocol.

Controls require proven native source identity and complete bounded discovery. Independent comparison-equivalent names refuse, including shadowed or excluded native copies. The recorded 0.63.0 built-in names `skill-creator` and `antigravity-support` also refuse under case-insensitive comparison, even when no local executable or recognizable launcher layout exposes their bundle. Strict document validation and native comparison-key membership are computed once per snapshot and reused for all source capabilities and recipes. Proven shared references remain one source. Project-only sources cannot be controlled globally. Managed extensions, built-ins, inspection-only sources, uncertain native inputs and untrusted projects expose reasons without broader fallback. Settings symlinks/hardlinks, destination aliases to another configuration role, content or managed storage refuse. Rechecking source, settings, trust, registries and discovery membership invalidates stale consent. Locked descriptor-based safe writes verify persisted bytes and fresh configured state. Persisted choice and effective permission are reported independently; any refused, failed or unachieved request exits nonzero.

## Reload evidence and limits

Fresh native processes observe every validated saved choice. Installed 0.63.0 bundle toggle feedback prescribes `/skills reload`; reload calls `config.reloadSkills()`, rereads merged disabled settings, rediscovers skills and updates activation tools/system instructions. Pinned bundle locations are `chunk-ED56GFWH.js:101500-101595` (toggle/reload feedback), `gemini-O7LOMBXL.js:8645-8652` (settings reload) and `chunk-2V7FO5QV.js:387891-387925` (skill/tool/instruction refresh). The probe records hashes of these installed bundle files. This supplies **source-verified** `/skills reload` or `/skills refresh` guidance, with restart as an alternative. Adoption by an existing authenticated running session remains **unobserved**. The product never launches or restarts sessions. Subsystem changes require restart and are outside this per-name writer.

Secure root-owned system settings precedence is source-verified, with insecure-system rejection observed natively. Secure administrative runtime precedence, remote admin policy, CLI/shell overrides, IDE trust and authenticated-session behavior remain explicit gaps. Unknown or blocking relevant inputs refuse. Native fixtures establish local settings and fresh-process behavior, not a claim of complete machine-wide inventory or session execution.

## Executable verification

[ALE-38 CLI acceptance](../../tests/acceptance/ale38_cli_test.go) covers global/project changes, inheritance, consent, case entries, exact JSONC preservation, shared references, independent collisions, untrusted/project-only sources, subsystem and project masking, malformed settings, configuration/content aliases, stale external/source/trust/registry changes, wrong confirmation and denied writes. [Focused PTY acceptance](../../tests/acceptance/ale38_browser_test.go) exercises actual confirmation, cancellation, stale refusal and exit status at 48 and 80 columns with long paths. Captured cells and fixture receipts are retained in the run evidence outside the candidate.
