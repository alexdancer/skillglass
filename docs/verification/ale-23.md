# ALE-23 verification record

ALE-23 adds the first production `skillglass` executable. Its immutable collection snapshot is shared by the `list`, `read`, and `browse` entry points. Discovery requires at least one explicit root. Project context is optional and visibly reports `none selected` when absent. Availability and ownership remain `unknown` until later integration tickets can establish them from evidence.

## Corrections made from executable evidence

- The first red journey failed with exit 127 because no production executable existed. The implemented vertical slice made the same public `list --root ... --json` journey pass.
- An unreadable empty root initially fabricated an unnamed source because an `Lstat` permission error was treated as proof that `SKILL.md` existed. Discovery now reports the root as incomplete and does not invent a bundle.
- The first browser frame repeated the selected-source header. The duplicate row was removed.
- The original end-of-document calculation could leave the last wrapped resource field beyond the reachable viewport. Detail pagination now reserves the actual wrapped footer height, and `G` exposes the complete last resource.
- Key help originally split words at narrow widths. Help wraps whole key-action groups at separators.
- A readable manifest inside a mode `0111` bundle was retained, but failure to enumerate its resources was initially missed. Resource discovery now handles a root `WalkDir` error before skipping the bundle directory, reports `unreadable-resource`, preserves the useful source, and exits with status 1 because coverage is incomplete.
- YAML non-finite floats initially made JSON encoding fail after discovery. Metadata normalization now recursively represents `.nan`, `.inf`, and `-.inf` as tagged JSON objects while preserving the exact raw manifest. Each affected manifest gets one non-coverage-limiting `metadata-non-finite-float` diagnostic, and otherwise complete `list` and `read` requests exit with status 0.
- Long project paths and many discovery roots could consume the entire browser viewport. The normal header is now four bounded rows. Pressing `s` opens a scrollable scope view containing the full project and every requested root, plus the resolved path when it differs; `Esc` or left returns to the prior inventory or detail view. Search input continues to treat `s` as query text.
- A long search query could still wrap across the whole browser viewport. Search now sanitizes the complete query, keeps that complete value for filtering and editing, and renders a one-row terminal-cell viewport with a leading ellipsis, rightmost query tail, and editing cursor.

## Reproduce the acceptance evidence

The harness builds with the checked-in module graph while Go network access is disabled, creates fresh HOME, XDG, project, and collection fixtures, and then tests the public executable:

```sh
./tests/acceptance/run.sh /tmp/skillglass-ale-23-evidence
```

It covers complete, empty, missing, unreadable, mixed, malformed, hostile-control, non-finite metadata, unlistable bundle resources, and renamed-directory cases. It checks exit codes 0, 1, and 2; exact JSON data; safe human rendering; full instructions and nested resources; fixture immutability; no script execution; and nonblocking FIFO discovery.

The browser coverage includes a public PTY journey and real tmux cell captures at 48, 60, 80, 96, and 120 columns by 30 rows. The journey searches, selects, opens, scrolls to the end, checks the complete final resource and footer, and exits with `q`. A separate 60 by 30 journey uses a long project and eight long roots to prove the bounded normal header, full scope top and end, search handling of `s`, and restoration of both inventory and detail. The single-root journey also exercises a live resize from 80 by 30 to 63 by 20 and back. A focused search journey covers a complete 1,800-character query, full-query filtering, wide Unicode, backspace, keep/cancel/clear transitions, and a 60 by 30 to 48 by 20 resize while the body, status, and controls remain visible.

The portable metadata tags are:

```json
{"kind":"non-finite-float","value":"nan"}
{"kind":"non-finite-float","value":"positive-infinity"}
{"kind":"non-finite-float","value":"negative-infinity"}
```

Run the repository checks separately:

```sh
find cmd internal tests/acceptance -name '*.go' -type f -exec /opt/homebrew/bin/gofmt -w {} +
/opt/homebrew/bin/go vet ./...
/opt/homebrew/bin/go test ./...
python3 -m unittest discover -s tests/agent_loop -v
```

The repair verification reran these commands after the final source and documentation changes. The executable acceptance suite, Go tests, vet, formatting check, and all 20 scheduler tests passed.

## Runtime containment boundary

The Darwin acceptance harness runs the public CLI and browser under `sandbox-exec`. The profile denies network access, file writes, unrelated file-content reads, and child execution. It permits content reads only for the exact executable, the fresh fixture, `/System`, `/usr/lib`, `/dev`, and the literal `/` needed during process startup. Process execution is limited to the exact tested binary path.

Positive controls prove that the excluded canary, write target, loopback listener, and `/usr/bin/true` work without the profile. Inside the profile, allowed fixture content remains readable while the excluded content read, write, loopback connection, and child execution are denied. The fixture tree is byte-for-byte unchanged afterward.

This is content isolation rather than a complete macOS capability sandbox. The profile starts from `allow default`, so ambient filesystem metadata reads and other non-content operations remain allowed. Tightening those metadata operations caused the dynamic loader or process startup to fail before application code ran, so the acceptance claim is limited to the controls above.
