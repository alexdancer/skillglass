# Delivered Mac companion workflow

This procedure joins the existing resource-read, proposal, approved-batch and
setup contracts. It requires macOS and cached Go 1.27.1 dependencies. The CLI
uses no account or network service. External model inference is the caller's
responsibility. A local build is sufficient; signing and publication are outside
this procedure.

## Build and retain the delivered artifact

Use a clean candidate and fresh absolute output/evidence directories. Temporary
homes and Go temporary directories must be outside every checkout:

```sh
export TMPDIR="$(mktemp -d /private/tmp/skillglass-delivery.XXXXXX)"
export GOTMPDIR="$TMPDIR/go"
mkdir -p "$GOTMPDIR"
./tools/build-local.sh /ABSOLUTE/NEW/DELIVERY
GOPROXY=off GOSUMDB=off /opt/homebrew/bin/go build -mod=readonly -trimpath \
  -o "$TMPDIR/isolationprobe" ./tests/acceptance/cmd/isolationprobe
```

Retain `DELIVERY/build.txt`, the executable, `go version -m`, source head and
SHA-256. `build-local.sh` records whether the candidate was clean. The executable
carries the matching companion without a runtime repository dependency.
Inspect actual `help`, `companion --json`, and each emitted file. The companion's
file hashes must equal the installed bytes, not merely its version label.

## Explicit setup and compatible startup

In the intended application's environment, put the delivered executable named
`skillglass` on PATH and run `skillglass setup --check --json`. Preview and then
install the exact selected targets with separate shared-consumer acknowledgement:

```sh
skillglass setup --app codex --app claude-code \
  --allow-shared-app opencode --allow-shared-app codeaf --json
skillglass setup --app codex --app claude-code \
  --allow-shared-app opencode --allow-shared-app codeaf --install --json
```

These are two installation targets, with indirect OpenCode/CodeAF exposure
acknowledged explicitly. No separate companion is installed for an acknowledged
consumer. Inspect actual destinations, native audit evidence and outcomes before
continuing. Customized copies are preserved conflicts. Unsupported or uncertain
isolation refuses without a fallback. Project-local companion setup is unsupported;
reviewing one selected project is a separate operation.

The [setup contract](../companion-setup.md#supported-destinations) owns validated
default user destinations and native catalog versions. These catalog checks do
not refresh independently versioned availability-control evidence or prove
adoption by an already running session. See [setup evidence](ale-48.md).

## Repeat the journey in a disposable environment

`review_scenario.py` is an evaluation controller, not a model runner. This command
explicitly sets up Codex and Claude Code in a synthetic home, with the shared
acknowledgements above. It runs no installed agent and no skill workload:

```sh
python3 tools/review_scenario.py create --installed --execution \
  --binary /ABSOLUTE/NEW/DELIVERY/skillglass --probe "$TMPDIR/isolationprobe" \
  --output /ABSOLUTE/NEW/EVALUATION --case global-lean
```

The controller proves containment before product launches. `controller-setup/`
retains separate bundle, preview and installation commands, raw outputs,
profiles/hashes and before/after manifests. Actual installed copies are read and
checked against the delivered bundle, then retained under
`controller-setup/installed-companion/`. `session.json` records each independent
installation and exact file hash. Subsequent calls refuse changed installed or
retained companion evidence.

`context.txt` supplies only the evaluation surface and exact installed companion
bytes. `request.txt` supplies the synthetic user request separately. `prompt.txt`
is their convenience combination. Give fresh evaluators these inputs without
implementation sources, expected recommendations, previous evaluations or
prewritten drafts. Preserve the actual model, prompts, answers and tool choices.
Do not infer interpretation from deterministic tests or instruction-string checks.

Three fresh sessions can cover the joined matrix:

| Case | Inputs and fixture coverage |
| --- | --- |
| `global-lean` | Explicit Codex/Claude global lean goal, budgeting and occasional research, protected guardrails and installed companions; shared research, independent same-name copies and disabled spreadsheet skills |
| `missing-inputs --incomplete-evidence` | Initially missing apps/scope/goal/protection; controller answers the model's questions with a broad global request; external unavailable, binary and bounded supporting evidence |
| `project-broad` | Explicit Claude selected-project broad goal, protected guardrails/companions, project maintenance and budgeting skills, disabled project choices |

Create separate output directories for each. `--incomplete-evidence` changes the
fixture independently from the initial request. It is introduced after explicit
setup because setup correctly refuses unresolved alias uncertainty.
`fixture-variation.json` distinguishes that controller action from model reads.
Use fresh sessions for controller smokes and actual evaluations.

The model starts with the installed companion's compatible-executable check,
reads selected inventories including disabled sources, reads instructions and
needed supporting resources, and authors its own draft. The existing
[proposal contract](../review-contract.md) defines prepare, subset selection,
exact preview, explicit approval and apply. The controller acts as a synthetic
user only after inspecting the model's actual proposal. Choose a strict subset
when supported by the actual recommendations; approve only the subsequent exact
selection ID. Preserve every omitted recommendation and actual selected effects.
Reinspect inventories and persisted bytes after apply, reporting configured
outcomes and session/reload limits separately.

The evaluator's `run` surface accepts only the three exact read-only setup-check
forms. It cannot preview, install or repair setup. `--execution` permits only
`review apply` through the sealed writer; all other product operations retain
reader/no-write checks. Native writes are limited to explicit fixture settings.
Installed skills, reviewed contents and unrelated settings must remain intact.

## Verification and evidence limits

Run `tests/acceptance/run.sh` with a fresh evidence directory and external
TMPDIR/GOTMPDIR. `SKILLGLASS_BIN=/ABSOLUTE/NEW/DELIVERY/skillglass` verifies the
exact delivered artifact. `SKILLGLASS_TEST_RUN='^TestALE49'` selects the joined
journey; the unfiltered final suite is the broad regression checkpoint. Supporting
Go tests require the separately proved contained launcher described in
[production verification](ale-24.md#reproduce-the-acceptance-evidence); never run
those fault tests uncontained.

The integrated public tests install two targets, inspect exact bundle/startup,
read instructions/support, prepare and approve a strict subset, advance multiple
changes sharing one config, apply and reinspect. Separate joined cases prove an
unrelated external edit refuses before writes and a genuine denied native write
reports completed/failed/not-attempted results. They check omitted/kept sources,
selected/unselected setup destinations, customized conflicts, no-write review,
skill contents and unrelated settings. Earlier tickets retain per-slice coverage.

Run the real provider checks with both explicit prerequisites so startup tests
cannot silently skip:

```sh
SKILLGLASS_PROVIDER_TEST_BINARY=/ABSOLUTE/NEW/DELIVERY/skillglass \
SKILLGLASS_COLLECTION_PROBE="$TMPDIR/isolationprobe" \
SKILLGLASS_PROVIDER_TEST_EVIDENCE=/ABSOLUTE/NEW/PROVIDER-EVIDENCE \
python3 -m unittest discover -s tests/native -p 'test_review_scenario_*.py' -v
```

The run report must pin exact source/artifact hashes and all commands, failures,
skips and final manifests. Representative model interpretation remains pending
until fresh actual sessions and their filesystem receipts are independently
inspected. Prepared fixtures alone establish no model behavior. Native catalog
checks establish discovery only, not inference, usage or running-session adoption.
