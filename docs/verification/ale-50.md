# ALE-50 bounded Pi controls

The user-selected policy is: allow proven operations and keep warnings. Incomplete collection inspection is reported independently from whether the selected exact native operation can be proven. This record extends the Pi 1.1.0/macOS contracts in [ALE-33](ale-33.md) and [ALE-34](ale-34.md).

## Native ignore decisions

The adapter retains inspected sources and separately records whether each native occurrence is excluded, eligible, or uncertain. It evaluates ignore rules while traversing. A `SKILL.md` with a known non-excluded ignore decision stops descent for that root; an ignored or ignore-uncertain manifest remains inspectable while traversal continues into children. Ignored references do not consume native lexical or canonical precedence slots. Files excluded by a known ignore rule remain inspectable; their occurrence does not participate in Pi loading.

The matcher uses Go Git's maintained gitignore implementation behind the native prefix transformations. `.gitignore`, `.ignore`, then `.fdignore` append rules in native depth-first order to the root's shared matcher. Literal paths, directory patterns, simple stars, positive ASCII alphanumeric character sets, case-insensitive ASCII matching and negation are supported. Ignored parents prevent child re-inclusion. Comments, empty files, CRLF and nested prefixes follow the recorded native cases. Each rule is qualified at the terminal occurrence before delegating to Go Git; inherited parent exclusions are checked separately. Re-including a parent does not erase an independent child exclusion.

Unqualified trailing spaces/tabs, `**`, `?`, repeated separators, escapes, negative/ranged character classes and non-ASCII ignore matching remain unknown. Each root bounds parsed ignore content to 1 MiB and 10,000 rules; exceeding either bound retains uncertainty. Unqualified rules are scoped conservatively to the directory prefix in which they were read, including every later occurrence that they could affect. Unreadable, oversized or invalid-UTF-8 ignore inputs retain uncertainty. This is a bounded native adapter, not a claim of complete gitignore grammar parity.

Run the differential probe against explicitly selected local artifacts:

```sh
python3 tests/native/pi-ignore-contained.py /path/to/@earendil-works/pi-coding-agent /absolute/path/to/skillglass /private/tmp/ale50-native-evidence
```

It creates disposable homes/projects and proves denied live-home settings reads, writes, networking and child execution before native imports. It imports only the installed Pi settings manager, resource manager and skill loader. No application session, extension, skill workload, installation or model request runs. It asserts each advertised-known occurrence agrees with native loaded membership across the baseline fixtures and the saved grammar and independently supplied edge cases in `tests/native/pi-ignore-cases.json`. Deliberately unqualified cases must explicitly report unknown coverage and refuse unconfirmed target controls. Its final PASS requires those assertions, rather than mere comparison completion. The CLI side uses a disposable HOME and environment; native imports have the explicit OS sandbox. It records results and installed version/source hashes, Node and binary hashes, script/profile hashes and raw outputs. Native runtime permission is needed to establish the inner macOS sandbox. Fixtures are retained at the reported temporary path for inspection.

## Exact control proof

A closed list of demonstrably unrelated inspection gaps may coexist with a Pi control preview:

- Inactive cache inspection failures do not imply native participation. A selected source within or ambiguously related to managed storage still refuses.
- A native dangling reference is stat-proven absent and cannot currently load. Its lexical link text, inode, ancestors and missing resolution remain guarded. Becoming a valid alias invalidates approval.
- A valid, understood registered package may declare an exact resource path that is absent. Unknown package registration, unreadable/invalid manifests and unbounded active membership remain refusals.
- Unqualified ignore rules for completely enumerated independent sources do not block a target whose every native occurrence is proven. Target ignore uncertainty still refuses.
- Viewer-selected references may retain inspection-only participation and unknown permission without blocking a proven native operation. The reference must belong to the selected source and match an explicit inspection root in both the request and root report, with no Pi native root fact. Missing or inconsistent provenance refuses; a viewer-only source still has no native control target.

Every other incomplete participating search refuses, including active traversal limits that could conceal a target alias, unresolved settings and unknown package boundaries. A target's native aliases, identity, hardlink restrictions, destination safety, trust and prospective achieved result are still checked. The existing independent-source permission/membership checks remain in force. No broad selector or package-manager write substitutes for an exact source operation.

Inspection coverage and diagnostics remain incomplete when those unrelated gaps remain. Preview notes disclose that proof is limited to the selected exact effects. Capability support still requires an explicit preview and never promises running-session adoption.

## Freshness and ordered review

Direct consent and review guards include directories where ignore inputs are consulted and all three candidate ignore-file paths, including absence, lexical identity, contents and resolved symlink targets. Exact native package/settings reader paths are pinned even when a package contributes no source. Changing an ignore file invalidates approval even when the resulting availability is unchanged. Existing settings, source, alias, trust and one-use confirmation guards remain. A dangling-link observation records its lexical text before reporting the resolution error; unrelated Pi absence may be retained without weakening the generic writer's strict error policy.

The same snapshot read-set helper is used during direct preview and review capture/check/prospective projection. Approved earlier settings writes advance only the existing expected postimage under the [shared ordered-review alias rules](ale-52.md#ordered-review-directory-alias-regression); external ignore/alias changes still stop the remaining selection. Warnings are retained in the approved proposal and final inspection.

## Executable regressions

`tests/acceptance/ale50_pi_test.go` exercises the built executable in isolated fixtures. It covers ordinary enable/disable with nonmatching root and actual live leaf ignore rules, dangling unrelated links, absent package roots and a 10,000-entry inactive cache; ordered two-item review under incomplete discovery; stale same-result/new/symlink-target ignore inputs and a broken link becoming a target alias; and refusal for active search limits, unknown settings and managed aliases. Existing unknown-ignore diagnostic tests now use deliberately unqualified `**` rather than newly supported `*`, retaining their original uncertainty-scoping contracts.

`tests/acceptance/ale50_inspection_controls_test.go` ports the independent six-application preview regression with and without an explicit viewer root, retaining unknown inspection permission diagnostics. It covers custom/project inspection roots across all Pi control operations and preserves refusals for native permission/participation uncertainty, unknown ignore rules, viewer-only sources and prevented prospective effects. `internal/collection/pi_change_proof_test.go` exercises missing/inconsistent reference and root provenance at the shared proof boundary. The original ALE-42 browser journey remains unchanged. Decision5's qualified higher-star-ignore case expects lower available and higher unavailable; its unsupported higher-pattern case still expects unknown.

Run the public regressions, the original ALE-42 browser journey, and adjacent discovery, direct control and ordered review coverage with:

```sh
SKILLGLASS_TEST_RUN='^Test(ALE(33|34|49|50|42DeliveredBrowserJourney)|Decision(ALE42RootGroup|5Precedence))' sh tests/acceptance/run.sh /private/tmp/ale50-acceptance
```

The provenance test runs separately through the [contained supporting-package checks](ale-24.md#reproduce-the-acceptance-evidence).

Exact candidate commands, receipts and results belong to the run artifacts. Neither these tests nor saved native settings prove current running-session adoption. Live retries require the exact-effect review workflow and do not permit deletion or direct settings edits.
