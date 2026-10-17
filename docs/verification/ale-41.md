# ALE-41 permanent deletion

## Shared behavior

`Snapshot.PreviewDelete`, `DeletePreview.View`, and one-use `DeletePreview.Apply` supply the ordinary `delete` command and browser reader's `X` action. Deletion is separate from settings controls. CodeAF participates in consumer discovery even though its application-specific controls remain unsupported.

The target is one proven source object and its canonical path. Directory aliases remain links after canonical content removal. Manifest-only links belong to their enclosing independent directory. Interior links are unlinked without traversing external targets. Equal names, equal bytes, and shared manifest objects do not merge independent bundle directories. A standalone Pi Markdown source removes only its canonical regular file. Multiple links to a standalone file refuse deletion. Internal hardlinks are disclosed and may be unlinked without changing their external aliases. This operation does not promise physical byte erasure.

The preview shows every selected-scope reference and seven native observations. Each observation retains its selected project, native environment overrides, searched roots, coverage, exact references, availability, and provenance. Zero exact matches differ from discovery failure. Discovery covers those roots and the selected project, not other projects or the whole machine. Unrelated inaccessible search roots remain visible and sealed. Ambiguous manager evidence or inaccessible scope relevant to the selected target refuses deletion.

## Ownership and boundary policy

Deletion derives its own positive local ownership evidence. Generic discovery's unknown ownership does not grant deletion permission. The effective UID must own the target, all entries, and the actual boundary parent. Canonical ancestors may include trusted root-owned directories. Shared sticky temporary ancestors are allowed only above a private UID-owned anchor. Group or world writable tree entries and boundary parents refuse deletion. ACLs are conservatively unsupported, including ACLs that might otherwise be harmless. Unreadable metadata, immutable or append flags, special entries, and device crossings refuse deletion.

All native manager evidence takes precedence over a custom path or alias. The filesystem root, home, selected project, application homes, semantic native collection roots, manager boundaries, their ancestors, and a target enclosing another independent source refuse deletion. A custom discovery root that is exactly one ordinary manifest-bearing bundle may pass. Child directory manifests establish additional bundle boundaries without following links.

The exact inventory includes hidden entries, names, object identities, ownership, modes, flags, change stamps, regular-file SHA256 digests, and symlink text. It refuses instead of truncating at these bounds:

- 10,000 entries, including the selected root.
- Depth 128 below the selected root.
- 256 MiB per regular file.
- 1 GiB of aggregate hashed regular-file bytes.

These checks establish local filesystem control and absence of known manager ownership within bounded discovery. They do not prove intellectual-property ownership.

## Consent and removal results

CLI preview without `--confirm` exits nonzero and never writes. The complete deterministic token binds the exact source, canonical target, ancestors, tree, ownership, native inputs, discovery roots, consumers, and provenance. Another invocation accepts that token only while the facts remain unchanged. Empty, wrong, stale, and already-used confirmation cannot apply. The browser uses the held preview and requires typing `delete FULL_SOURCE_ID`; bare `y`, empty Enter, and Escape cannot remove content. Leaving the preview discards consent.

Apply reconstructs native discovery and exact target facts before removal. It uses anchored descriptors, no-follow child opens, parent-child identity checks, membership checks, and postorder `unlinkat`. It never runs recursive ambient-path deletion or retries newly observed names. Cancellation is checked between operations. An advisory parent lock coordinates cooperating viewer deletion processes.

Results distinguish refused, stale, cancelled, failed, partial, and removed. Successful unlink operations retain an in-memory journal. Reinspection reports observed remaining entries, replacements, and unknown paths without following replacements. Only complete original-object accounting plus observed absence of the canonical root's original parent entry produces verified removal and exit zero. Partial removal has no rollback. Settings, remaining aliases, and references are never repaired. Running sessions may retain content already loaded into memory.

## Darwin concurrency limit

Darwin has no compare-and-unlink syscall. A same-UID writer can substitute a name after final validation or move an open directory elsewhere. Descriptor-relative traversal protects external symlink targets, but advisory locking does not exclude that writer. The guarantee covers changes observed before a syscall and normal preservation of independent sources. It does not promise atomic isolation from arbitrary hostile same-UID changes.

Held non-directory descriptors allow link-count checks after unlink, so a substituted name cannot alone establish that the original object was removed. APFS directory descriptors retain their link counts and last path after removal. Directory verification therefore combines the held descriptor's path, completed guarded operations, and no-follow absence in the anchored original parent. This remains subject to the stated name-race limit. No quarantine, permission rewrite, snapshot, trash, or privileged helper changes the source before removal.

## Executable verification

`tests/acceptance/ale41_delete_test.go` uses the built executable under the existing isolated subprocess policy and launch-receipt harness. Fixtures cover shared references in all seven integrations, independent equal and diverged copies, standalone Markdown and standalone hardlink refusal, directory aliases, manifest-only links, interior links, internal hardlinks, custom direct roots, managed custom aliases, protected home/project/native collection roots, writable permissions, ACLs, FIFO entries, file/depth limits, unrelated inaccessible discovery, missing/wrong confirmation, and stale bytes, membership, root, consumer, native input, and alias facts. Tests inspect actual survivors and settings.

`tests/acceptance/ale41_browser_test.go` drives real terminals at 48 and 120 columns with long canonical paths. Captures show the full wrapped target, preview scroll, typed consent, empty-input refusal, cancellation, held-preview staleness, verified result, and process exit. Bare `y` is exercised without removal. Captures are inspected as rendered terminal text, with cell bounds asserted.

`internal/collection/delete_fault_test.go` supplies deterministic checkpoints where the public executable cannot reliably interrupt a syscall. It covers first-operation failure, partial interruption, cancellation, symlink substitution before validation, child and anchored-directory rename, new membership, root replacement, and the final validation/unlink gap. The gap fixture proves that a substituted symlink can itself be unlinked while its external target and the moved original remain intact; the result is partial and unverified. The preview is also tested for single use after wrong confirmation.

Reproduce the checks from the repository root:

```sh
GOPROXY=off GOSUMDB=off go test ./internal/... -count=1
GOPROXY=off GOSUMDB=off go vet ./...
SKILLGLASS_TEST_RUN=TestALE41 tests/acceptance/run.sh /absolute/path/to/evidence
tests/acceptance/run.sh /absolute/path/to/full-evidence
```

The acceptance runner builds the executable offline, constrains HOME/project/environment to disposable fixtures, and retains commands, outputs, sandbox policies, launch receipts, and terminal captures. It never deletes installed skills or changes live agent settings.
