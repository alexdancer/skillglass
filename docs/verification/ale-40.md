# Explicit group skill controls

The [README's group control instructions](../../README.md#build-and-use) own CLI and browser usage, capability limits, whole-request refusal, explicit subset selection, execution order, cancellation, and result semantics. The adapter verification documents linked there retain authority for native behavior and reload requirements. Group composition introduces no native capability or native-version claim.

`GroupPreview` owns immutable native child previews. Its confirmation binds the exact source, ordered selection, scope, project, discovered consumers, bounded coverage evidence, and native child digests. Public views copy nested evidence and omit child confirmation tokens. Before any write, the group reconstructs its full preflight. Each original child then retains its native immediate recheck and guarded writer. Independent same-name sources are never substituted.

## Preflight footprint limits

The private footprint check compares write destinations, later observed inputs, and immediate-parent creation effects. Existing paths retain filesystem object-identity comparison through resolved parent aliases. Beneath a shared existing object-identity anchor, differing components missing on both paths are treated as potentially aliasing when Unicode canonical decomposition and full case folding agree. This is conservative uncertainty, not proof of filesystem equality: the check does not infer a volume's case or Unicode rules or perform probe writes, and can therefore refuse such missing paths even on a case-sensitive volume. This same comparison governs write/write aliases, later read dependencies, parent creation, and discovery containment. Ordinary disjoint missing parents such as `control` and `control-other` remain independent.

Discovery containment includes requested and resolved roots and every inspected bundle directory, including child directory aliases outside their configured root and inspection-only bundles. It examines both selected and global views when a native preview records them separately. Standalone instruction files remain exact reads; external resource symlink targets do not become recursively inspected directories. Stable shared ancestors and the common immutable skill source are allowed. No-op children introduce no replacement or directory-creation effects. Passing preflight never relaxes a native freshness guard or guarantees runtime success. No observation overlay or consent rebasing is used.

The group delegates native success to `ChangeOutcome.ExitCode()`, requiring verified persistence and achieved effective result rather than just a completed write. Attempted results retain the full native outcome, including retained artifacts and committed-write verification failures. Cancellation before a child is invoked produces no native outcome for that child. Consumer records describe discovery references, not application installation.

## Executable and terminal evidence

Run the focused public suite on macOS with temporary storage outside the checkout:

```sh
TMPDIR=/private/tmp GOTMPDIR=/private/tmp GOCACHE=/private/tmp/skillglass-go-cache \
	SKILLGLASS_TEST_RUN=TestALE40 sh tests/acceptance/run.sh /private/tmp/ale40-evidence
```

The public suite builds the actual executable and uses the established contained fixture launcher. Synthetic home, project, and configuration paths remain inside each temporary allowed tree. Runtime network, real settings, and outside content remain denied. Launch receipts and terminal captures are saved under the supplied evidence directory.

`TestALE40PublicSupportedGroups` verifies Codex and Claude in both orders, all six writers in forward and reverse order, absent destination files, lossless unrelated settings, source retention, per-application verification, reload evidence, and verified no-ops. `TestALE40PublicSafelyMissingParents` verifies both orders with absent immediate user configuration directories. `TestALE40PublicProjectAndInheritance` exercises supported Claude and OpenCode project overrides and restoration of disabled global inheritance.

Other public cases verify whole-request refusal with CodeAF, unsupported Codex project controls, unknown and duplicate IDs, fresh explicit subsets, wrong/order/subset/reused tokens, stale source/configuration/project/coverage/consumers, independent copies, incomplete discovery, and aliased configuration destinations. `TestALE40PublicRuntimeFailureRetainsEarlierSuccess` denies writes only in the second application's synthetic configuration directory. Preflight reads still succeed. The executable persists the first choice, reports the second failure, and leaves the third configuration untouched and not attempted.

The public discovery-containment cases in [ale40_cli_test.go](../../tests/acceptance/ale40_cli_test.go) cover all six native readers, selected/global observation views, escaped child bundle aliases, inspection-only bundles, no-op writers, safe reverse order, standalone/resource-link boundaries, and configuration-parent creation. They distinguish deterministic cross-application interference from runtime failures without changing native guards.

`TestALE40MissingParentNamePrecision` covers conservative case/normalization uncertainty and disjoint-name controls in both orders. `TestALE40MissingParentAliasRefusesBeforeWrites` first proves the fixture volume aliases the case- or normalization-equivalent names, then verifies independently ready native children, absent parents, aggregate refusal without a token, ordered not-attempted forged-consent results, and unchanged source/tree evidence. Case-alias fixtures in [ale40_case_alias_test.go](../../tests/acceptance/ale40_case_alias_test.go) likewise prove existing parent identity. Tests requiring actual aliases explicitly skip when the fixture volume cannot supply them; those skips are missing coverage, not passing alias proof. Unexpected setup or I/O errors remain fatal.

`TestALE40BrowserGroupJourney` drives the built executable in real tmux terminals at 48 and 120 columns with long source paths. It covers unchecked selection, scope, paged preview, cancellation, mixed refusal, explicit subset reselection, success, stale confirmation, and deterministic partial failure. Captures include checklist, preview top and bottom, refusal, fresh subset, ordered results, and exit status. The browser exits nonzero after a failed confirmed operation.

The archived implementation pilot proved composition of original previews in both orders and all-six orderings. Those ordinary journeys remain covered by the public executable suite rather than a retained internal pilot test. Narrow tests use the existing safe-writer fault seam only for committed-write verification failure, a later external edit after earlier success, and cancellation between children. These conditions need deterministic timing that the ordinary command cannot provide. The public second-writer persistence failure uses the real filesystem policy, not a new product seam.
