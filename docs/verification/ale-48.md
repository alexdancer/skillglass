# Companion setup verification

The [setup contract](../companion-setup.md) owns target selection, explicit shared
consent, supported destinations, executable prerequisites, output and recovery.
This record describes executable and native proof and their limits.

## Public executable fixtures

`tests/acceptance/ale48_setup_test.go` exercises the built public executable in
disposable homes/projects with inherited runtime containment. Its oracle is
exit status, JSON/human output and actual filesystem effects. Coverage includes
two selected applications, default shared roots, unselected consumers, separate
CodeAF consent without a CodeAF target, unsupported targets/destinations,
identical installations, customized/extra/nonregular conflicts, prospective
aliases, filesystem denial with retained partial success, missing/moved and
incompatible executable discovery, invalid consent and unsupported scope flags.
Before/after manifests prove successful setup preserves unrelated entries and
writes only the exact selected bundle destinations and necessary parents.

Use the [repository acceptance procedure](ale-24.md#reproduce-the-acceptance-evidence)
with external TMPDIR/GOTMPDIR and a fresh evidence directory:

```sh
SKILLGLASS_TEST_RUN='TestALE48|TestALE46' ./tests/acceptance/run.sh /ABSOLUTE/FRESH/EVIDENCE
```

The existing supporting-package suite must run separately through the proved
contained launcher; never run its collection fault tests uncontained. The
coordinator's report owns exact commands, source heads, artifact hashes, results
and inspected artifacts. A filtered run does not claim the full aggregate suite.

## Independent native discovery

`tools/verify_companion_setup.py` installs the supplied exact artifact for Codex,
Claude and OpenCode, with explicit CodeAF shared consent, in a fresh disposable
HOME/project. It validates exact installed bundle bytes and independent native
catalogs without prompts, activation, inference or skill workloads:

```sh
python3 tools/verify_companion_setup.py \
  --product /ABSOLUTE/DELIVERED/skillglass \
  --probe /ABSOLUTE/ISOLATION/isolationprobe \
  --codex /ABSOLUTE/NATIVE/codex \
  --claude /ABSOLUTE/NATIVE/claude \
  --opencode /ABSOLUTE/NATIVE/opencode \
  --out /ABSOLUTE/FRESH/NATIVE-EVIDENCE
```

The isolation probe is built from `tests/acceptance/cmd/isolationprobe` using the
repository's offline cached Go build procedure. Supply authorized installed
native executables, not arbitrary programs. The helper requires the exact
recorded versions Codex 0.162.1, Claude Code 2.1.300 and OpenCode 1.17.15. It pins
supplied native bytes before proving the existing Codex containment controls;
that reuse does not inherit the older helper's 0.155.1 version claim. Claude's
separate live-home/system denial controls are proved before initialization.
OpenCode uses the existing denied-network, bounded fixture-write/outside-content
read policy, disabled remote updates/default plugins and an exact child-exec
allowlist. It copies its executable to separate immutable storage. Native
protocols retain the existing helpers' bounded initialization/transport limits.

Codex `skills/list` must include the exact installed manifest path. Claude's
initialization catalog must name the companion. OpenCode's `debug skill` must
include the exact manifest for its own root and the shared Claude root in two
separate catalog launches. The driver temporarily moves only those disposable
bundles between launches to avoid inferring both routes from a same-name
concurrent winner, then restores and verifies every byte. Settings, sources,
homes, working directories and temporary paths are disposable. Durable receipts
retain wrapped commands, environments, executable/profile hashes and exits;
interactive catalog transports additionally retain their protocol transcripts.

Fresh native catalogs establish discovery at these default user destinations,
not model interpretation, actual use, arbitrary home overrides, existing-session
reload/adoption or CodeAF binary equivalence. CodeAF remains a possible
source-loader consumer with installed dirty-build discovery explicitly
unverified. Application availability-control evidence is independently
versioned and is not silently relabeled by this setup validation.
