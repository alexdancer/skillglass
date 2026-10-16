# ALE-36: OpenCode skill controls

The writer uses the existing source-bound preview, explicit confirmation, stale-preview recheck and safe atomic file writer. CLI and browser share that implementation. The validated native version is **OpenCode 1.17.15**. These controls describe local saved configuration and prospective default-agent permission. They do not establish a running session's agent selection, approval state or loaded skills.

## Supported contract

- Global controls require an ordinary eligible global source. Output is `$XDG_CONFIG_HOME/opencode/opencode.json`, defaulting to `~/.config/opencode/opencode.json`; an existing `opencode.jsonc` is preferred. Project controls require explicit `--project` with native project configuration enabled and use the selected project's corresponding pair. A sole exact own-layer project override is removed from its actual pair member for `inherit`.
- The native selector is frontmatter name, independent of directory/source ID. Supported names are 1-64 lowercase ASCII alphanumeric characters with single hyphen separators. Independent equal-name copies, embedded `customize-opencode` collisions, managed/cache targets, inspection-only sources and incomplete or dynamic discovery are refused or unsupported. Shared directory aliases retain one proven source identity.
- Disable saves exact `permission.skill.NAME = "deny"`. Enable saves `"allow"` when needed; an already effective `"ask"` is preserved and may be a no-write outcome. A current denial contains no evidence of historical ask, so a later enable explicitly proposes allow. Inherit removes only the exact project property; remaining wildcard, global, native-directory and agent rules still apply and may deny or ask.
- Config file selection never moves to another scope or higher-priority layer to force success. All native load roles remain in their original order, including repeat loads of the same lexical file. Config aliases, managed outputs, global/project overlaps and unverified output roles are refused, including missing files reached through aliased parents.
- The JSONC patch changes only the selected property/value and required comma tokens. It retains surrounding comments, other fields, native integer-key enumeration and unrelated rule order. A same-reader prospective overlay checks the selected result, independent sources and the complete merged ordered config. This also detects scalar-to-object conversion that would resurrect lower-layer rules for names absent from the inventory. No reordering fallback is used.
- Preview confirmation binds source/reference identity, inventory, destination bytes and metadata, all relevant config roles and virtual inline/runtime inputs. Cancellation writes nothing. External edits, file replacement, changed aliases or context invalidate confirmation. Safe writes preserve existing file mode/ownership/xattrs and verify persisted bytes. ACLs/flags, links, unsafe owners and unsupported parent creation are refused by the shared writer.

```sh
bin/skillglass disable --agent opencode --scope global --json SOURCE_ID
bin/skillglass enable --agent opencode --scope project --project /path/to/project SOURCE_ID
bin/skillglass inherit --agent opencode --scope project --project /path/to/project SOURCE_ID
# Repeat the exact preview request with --confirm opencode1:PREVIEW_TOKEN.
```

The browser uses `D` or `e` for scope selection, `g` for global, `p` for project and `i` for inherit. `y` applies the held preview. Enter/Esc cancels. Refused, unsupported and failed requests return nonzero. Results distinguish write state, verified persistence, saved exact permission where present, effective configured result and unknown session adoption. OpenCode may require reload or restart; the viewer never launches or restarts it.

## Independent native evidence and rerunnable lever

The prerequisite evidence was independently collected in the ALE-36 run directory before enabling this writer. Seven containment tests, 30 native config checks and 21 native evaluator checks passed. Canonical `/private/tmp/ale-36-controls-*` fixtures contain HOME, XDG, TMPDIR, controlled managed configuration, copied OpenCode and Git executables. Sandbox policy denies network and live settings and limits persistent writes to the fixture. Every native launch records final argv/cwd/environment, executable/source/profile hashes, immutable profile snapshot, stdout/stderr, exit, timestamps and process-group cleanup. Prepared timestamps are not claimed as OS spawn times.

The production lever retains the independent probes and the existing containment/receipt helpers:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 tools/verify_opencode_controls.py \
  --opencode /opt/homebrew/bin/opencode \
  --out /absolute/path/to/new-evidence-directory
```

The output directory must be new. `result.json` records source hashes and aggregate status. `config/receipts.json` and `evaluator/receipts.json` contain the version and individual native checks; each has an immutable `invocations/` journal. Nested sandbox establishment may require reviewed execution on the host. A failed containment positive control is a blocked verification, never permission to weaken the fixture policy.

Native config observations establish exact allow/deny, meaningful ask inheritance, JSONC precedence, project/global/native-directory/inline/controlled-managed/runtime/agent order, replacement retaining insertion position and fresh-process reads after edit. Duplicate native names collapsed to one definition without proving a stable winner. Actual native debug tool dispatch distinguished denial before the isolated ripgrep blocker and exercised exact/prefix, literal brackets, wildcard `?`/`*`, case, slash normalization and last-match behavior.

The non-denied tool branch reached `ripgrep execution failed`. This is permission-branch evidence, **not a successful full skill load**. Debug ask intentionally bypasses prompting. No model turn was started. Live-session enforcement, approval prompting or persistence, existing-session reload, exact restart requirements and full tool loading remain unverified. Controlled managed config is not evidence of enterprise policy behavior on an actual managed machine. The reader retains its prior version-specific gaps from [ALE-35](ale-35.md).

## Executable and terminal verification

`tests/acceptance/ale36_cli_test.go` drives the built executable in isolated HOME/project/XDG fixtures with containment receipts. Journeys cover preview with no mutation, global/project disable and enable, second-context inheritance behavior, exact override removal, idempotence, ask preservation, JSONC comment/trailing comma preservation, scalar baselines and resurrection refusal, inherited insertion order and wildcard tool blocking, integer keys, independent-name collisions, shared aliases, embedded names, managed/inspection-only targets, malformed/substituted config, ambiguous overrides, output overlap, hardlinks, higher-priority native/agent/inline/runtime/managed rules, changed virtual input, external edits/source/file identity and sandboxed write denial. Other application settings and skill content are checked for preservation.

`tests/acceptance/ale36_browser_test.go` exercises actual PTY confirmation, cancellation and stale confirmation at 48 and 80 columns with long paths. Captures include preview top/bottom, committed result, return to detail, stale result and exit. Shared safe-writer fault checks plus `TestOpencodeChangeWriteFaultOutcomes` cover deterministic failures during staging, rename, directory sync and postread that cannot be induced reliably through public commands. A committed durability or postread failure remains failed and nonzero while exposing the actual observed persistence.

The ALE-36 build report identifies the exact final head, commands, receipt/capture directories, passed checks and any failed or unrun checks. Independent Standards/Spec review and final ticket acceptance are separate coordinator-owned steps.
