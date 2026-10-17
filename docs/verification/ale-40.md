# Explicit group skill controls

Group controls compose the six completed native writers for one proven source. They introduce no native capability or native-version claim. CodeAF controls, Codex project controls, and each adapter's existing unsupported cases remain unsupported. The adapter verification documents linked from the README retain authority for native behavior and reload requirements.

`disable`, `enable`, and `inherit` accept repeated `--agents APP` flags in execution order, excluding singleton `--agent`. Every member uses the same explicit scope, selected project, and exact source ID. Unknown and duplicate IDs refuse. A requested unsupported or unverified member blocks the entire preview without a token or writes. A supported subset requires a distinct explicit selection, preview, and confirmation. The browser starts its group checklist unchecked and shares the collection policy with ordinary commands.

`GroupPreview` owns immutable native child previews. Its confirmation binds the exact source, ordered selection, scope, project, discovered consumers, bounded coverage evidence, and native child digests. Public views copy nested evidence and omit child confirmation tokens. Before any write, the group reconstructs its full preflight. Each original child then retains its native immediate recheck and guarded writer. Independent same-name sources are never substituted.

The private footprint check compares actual write destinations and later observed inputs. It resolves parent aliases, compares filesystem object identities, accounts for missing-parent creation, and rejects configuration writes that change later discovery. Stable shared ancestors and the common immutable skill source are allowed. No-op children introduce no replacement or directory-creation effects. No observation overlay or consent rebasing is used.

Applications run sequentially. A native outcome succeeds only when its existing `ChangeOutcome.ExitCode()` succeeds, including verified persistence and achieved effective result. The first non-success stops execution. Earlier successful changes remain; remaining applications are `not-attempted`. Attempted results retain write state, persistence, effective availability, reload evidence, and retained artifacts. A committed write followed by failed verification remains reported as committed. Cancellation before an application starts leaves it not attempted. Group operations make no rollback or cross-file atomicity promise.

Discovery covers only the roots and project shown in each consumer's scope. Incomplete unrequested discovery remains visible and does not authorize an unverified requested writer. The consumer list does not assert installation or complete machine-wide discovery. Running application sessions are not observed or restarted.

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
