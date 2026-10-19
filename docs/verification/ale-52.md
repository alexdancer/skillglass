# ALE-52: macOS-managed settings replacement provenance

The shared Darwin settings writer allows only `com.apple.provenance` to differ on its temporary replacement, including an OS-generated or absent tag. Skillglass never sets, removes or restores that tag. Original source observations and preview freshness remain exact, including all extended attributes and change times. Every other extended attribute must retain its exact name and bytes. Mode, UID, GID, flags, single-link status, ACL refusal, parent descriptors, no-follow reads, locking, exact postimage validation, atomic publication and cleanup remain enforced. Staged replacement identity and metadata are checked before publication.

Existing-file write previews explain this narrow policy in their notes. If replacement provenance actually differs and publication commits, the outcome reason records that observed difference, even when later verification reports a failure. A preview or refusal never claims that settings were saved.

## Evidence and limits

The original real built-CLI review apply refused with `temporary clone did not preserve file metadata`, no writes and all ten selected Pi sources still available. Independently observed native `Clonefileat` on a disposable destination changed only provenance before opening for write, with unchanged source metadata and matching mode, ownership, flags and bytes. These preserved run artifacts establish the original failure; tests never manufacture a security tag.

`TestALE52MacOSReplacementMetadata` exercises the built public CLI with a naturally created settings file, records whether the host supplied provenance, verifies JSON and human policy disclosure, persistence, protected metadata, an ordinary xattr, source retention and cleanup. On this host fresh files share one process origin, so their cloned provenance may be identical. The test checks committed-receipt disclosure when a natural difference occurs; it does not claim deterministic OS provenance divergence.

`TestALE52OriginalMetadataFreshnessRefuses` covers changed/added ordinary xattrs, mode, hardlinks, symlinks and flags between preview and apply. Focused native writer controls reject injected changes to temporary ordinary xattrs, mode, flags and link count. A small in-memory comparator test covers changed/added/absent provenance and rejects every non-exact attribute exemption. This supplements public behavior evidence and does not simulate OS metadata generation.

Focused verification uses the established contained acceptance runner and the contained collection test wrapper. Full acceptance and independent Standards/Spec review are performed by the run coordinator on the frozen commit. No live settings, installed executable or live metadata were modified by the implementer.

## Post-publication verification

Persistence is verified against the exact staged replacement identity, bytes, mode, owner/group, flags, links, ACL absence and full xattr map, including that replacement's provenance. It is never checked against the old object's provenance after a permitted replacement. Only this fully verified committed fact can advance ordered review. A same-byte external inode replacement or any mismatch from staged metadata remains unverified. No-op inspection retains the exact original observation, including provenance and change time.

The preserved independent receipt regression models a distinct original provenance observation in memory and now verifies persistence of the exact committed replacement. Additional supporting controls refuse mismatched no-op provenance/ordinary attrs, absent staging evidence, staged provenance/ordinary attrs/flags/links and a same-byte inode replacement. Two native writes exercise successful exact committed-fact review-guard progression with modeled original provenance differences, followed by a strict subsequent provenance-freshness refusal. The existing public ordered Pi review test still exercises full prepare/select/approve/apply behavior with real fixtures. No filesystem security tag is changed by these tests.

Hermes postwrite controls that deliberately hardlink the published settings file now expect committed-but-unverified persistence while preserving achieved native-choice reporting. An independent manifest hardlink still changes only writer eligibility and retains verified saved settings. Metadata verification failures report a configuration mismatch rather than incorrectly claiming that matching saved bytes differ.
