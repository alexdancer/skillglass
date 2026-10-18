# ALE-28 native resolver fix

The upstream Codex correction is implemented and verified in a locally built Linux arm64 binary. Trusted project skill rules now affect both discovery and exact skill selection. The stock Codex release and Skillglass project writer remain unchanged. ALE-28 stays open because its macOS distribution and writer acceptance criteria are not yet satisfied.

The implementation starts at official Codex `rust-v0.162.1`, commit `092d3acd6bec3e3a14bdc7e7a2810ab628ab759d`. The [patch with regression tests](../../tools/native-probes/patches/codex-0.162.1-project-skills.patch) preserves the complete resolver change. The upstream [contribution guide](https://github.com/openai/codex/blob/main/docs/contributing.md) excludes external code contributions and pull requests, so this Skillglass PR preserves the correction for review.

## What changed

`skill_config_rules_from_stack` now iterates enabled configuration layers and includes `ConfigLayerSource::Project`. This excludes disabled or untrusted project layers. It retains the existing ordered selector resolver, so a higher-priority rule overrides the same skill without erasing unrelated lower-layer rules. Session settings retain priority over project settings.

Six behavioral regressions cover project disable, project re-enable over user denial, removing an override while retaining unrelated rules, disabled project isolation, session precedence in both directions, and nested project precedence. The public resolver API, selector parsing, canonical path matching, and shared skill contents are unchanged.

## Before and after

Tests were added before changing the resolver. The real test output was:

```text
test result: FAILED. 11 passed; 4 failed; 0 ignored; 0 measured; 351 filtered out; finished in 0.00s
```

After the correction, the full configuration crate passed. It passed again after the final build:

```text
test result: ok. 366 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.04s
```

The same reviewed native probe then exercised 42 turns against stock and 42 against the patched binary. It inspected accepted configuration origins, actual catalog entries, and complete skill bodies sent to a fixed local Responses endpoint.

| Behavior | Stock 0.162.1 | Patched build |
| --- | --- | --- |
| Project disable | Ignored | Exact target omitted |
| Project enable over global disable | Ignored | Exact target restored |
| Second project | Follows global | Follows global |
| Remove a working override | No working override to remove | Returns to global choice |
| Unrelated project rule retained during removal | Ignored | Still enforced |
| Untrusted project rule | Ignored | Ignored |
| Nested project precedence | Project rules ignored | Enforced |
| Session priority | Enforced | Enforced |
| Symlink-manifest selector | Project rule ignored | Matches canonical source |

The independent same-name copy and sibling controls remained distinct. All fixture hashes, request hashes, request identities, process-group teardown, and runtime-container cleanup checks passed. The probe did not use an external model or execute skill workloads. The built binary also passed a two-turn native baseline.

## Reload behavior

A separate comparison completed 16 native turns across eight isolated branches. Each branch first captured a catalog-only baseline, then changed the project rule and attempted exact skill selection. The baseline contained no skill body, so earlier selected content could not create a false positive.

| Action after project edit | Disable adopted | Removal adopted |
| --- | --- | --- |
| Continue existing thread | No | No |
| Force skill rescan, continue existing thread | No | No |
| Start a fresh thread in the same process | Yes | Yes |
| Restart process and start a thread | Yes | Yes |

For this build and path, start a new thread after changing project rules. A forced skill listing refresh does not replace an existing thread's project configuration. No additional cache change was justified. Historical catalog text in retained requests was not treated as evidence of current catalog adoption.

## Build and verification

The binary was built with the source-pinned Rust 1.95.0 toolchain in a disposable Linux container. All 9,018 tracked source inputs matched the reviewed checkout and recorded Cargo metadata reconciliation. The release tag declares local package version 0.162.1 while its lockfile records 0.0.0. Cargo reconciled only those 161 local version records. All 1,312 external dependency records stayed identical.

An initial build failed because host tar copied macOS AppleDouble sidecars into the Linux SQL migration folders. Those generated sidecars were removed only from the disposable source copy. A manifest records them. The successful build then completed without changing application source beyond the reviewed patch.

Final checks passed:

- `cargo build --locked -p codex-cli --bin codex`.
- `cargo test --locked --offline -p codex-config --lib`, 366 tests.
- `cargo clippy --locked --offline -p codex-config --all-targets -- -D warnings`.
- `cargo fmt -p codex-config -- --check`. Rustfmt emits the repository's documented warning for its nightly-only import-grouping option.
- The stock and patched 42-turn native matrices, and the 16-turn reload comparison.
- Independent Standards and Spec reviews, each with zero actionable findings.

The patched binary SHA-256 is `d3d8cd35b917f1518519a34bc34ba97cda5d1a4f985d5d477e8cb6551fce8dae`. The source patch SHA-256 is `abf32454369b85038903c500941d070094682776468984dacef4ff8fc7beadb0`. It reports version 0.162.1, but is a custom build, not the official release. Never use that version string alone to enable a production capability.

Local raw captures, frozen probes, the exact launcher, build receipts, manifests and review reports are preserved in `.agent-loop/ale-28-fix-20261018/`. The Linux executable is `bin/codex-linux-aarch64` under that directory. This binary does not run natively on macOS. The runtime image is `ale28-patched-probe:20261018`, image ID `sha256:553a2ae42a57e48564316e328ee842a362d94433b63ba8b3b3cb6ba220db0076`. Use the preserved launcher with the recorded image and binary/patch digests to repeat cold or `--warm` measurements. Its output path is relative to the launcher's directory; use a new run name each time.

## Remaining acceptance

The installed macOS binary still has the previously reproduced managed-preference validation blocker. This Linux correction does not establish macOS runtime support. Native command-sandbox enforcement was not exercised. Docker provided the reviewed outer isolation while the exact known user-namespace warning remained visible.

Skillglass must retain its stock project-scope refusal. Its production writer requires a validated supported distribution and platform, matching per-layer availability explanations, lossless override removal, shared preview/apply safety, and public executable/terminal acceptance. The complete upstream workspace suite, macOS capability matrix and Skillglass production journeys were not run for this upstream correction. No-mistakes was not run during the uncommitted implementation phase. Cursor-specific comment review is unavailable on this host. Native Standards and Spec reviewers ran instead.

No installed Codex binary or real skill configuration was replaced during verification. This patch package does not enable the Skillglass project writer or close ALE-28.

Fix Root Causes kept the change in the native resolver. Model the Domain retained ordered selector rules. Test Behavior, Not Implementation required failing and passing observable rule results. Prove It Works required the built native CLI's captured requests and measured reload behavior.
