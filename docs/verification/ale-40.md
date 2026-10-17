# Explicit group skill controls

The [README's group control instructions](../../README.md#build-and-use) own CLI and browser usage, capability limits, whole-request refusal, explicit subset selection, execution order, cancellation, and result semantics. The adapter verification documents linked there retain authority for native behavior and reload requirements. Group composition introduces no native capability or native-version claim.

`GroupPreview` owns immutable native child previews. Its confirmation binds the exact source, ordered selection, scope, project, discovered consumers, bounded coverage evidence, and native child digests. Public views copy nested evidence and omit child confirmation tokens. Before any write, the group reconstructs its full preflight. Each original child then retains its native immediate recheck and guarded writer. Independent same-name sources are never substituted.

The private footprint check compares actual write destinations and later observed inputs. It resolves parent aliases, compares filesystem object identities, accounts for missing-parent creation, and rejects configuration writes that change later discovery. Stable shared ancestors and the common immutable skill source are allowed. No-op children introduce no replacement or directory-creation effects. No observation overlay or consent rebasing is used.

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

`TestALE40BrowserGroupJourney` drives the built executable in real tmux terminals at 48 and 120 columns with long source paths. It covers unchecked selection, scope, paged preview, cancellation, mixed refusal, explicit subset reselection, success, stale confirmation, and deterministic partial failure. Captures include checklist, preview top and bottom, refusal, fresh subset, ordered results, and exit status. The browser exits nonzero after a failed confirmed operation.

The archived implementation pilot proved composition of original previews in both orders and all-six orderings. Those ordinary journeys remain covered by the public executable suite rather than a retained internal pilot test. Narrow tests use the existing safe-writer fault seam only for committed-write verification failure, a later external edit after earlier success, and cancellation between children. These conditions need deterministic timing that the ordinary command cannot provide. The public second-writer persistence failure uses the real filesystem policy, not a new product seam.
