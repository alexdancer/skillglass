# ALE-22 terminal prototype

This throwaway prototype asks one question: does a source-first terminal model make identity, project context, availability evidence, comparison, disable, and permanent delete understandable before the production interface exists?

It reads one inert JSON fixture. Canonical and discovered paths are display strings. The program never opens them. Disable and delete change only an in-memory copy for the current process. The fixture file, installed skills, plugin caches, and agent settings stay untouched.

## Start the browser

Use Python 3.10 or later on macOS or another Unix-like system. Browser interaction requires a TTY. From the repository root:

```sh
python3 prototypes/ale-22/terminal_prototype.py browse \
  --fixture prototypes/ale-22/fixture.json
```

The browser uses the fixture's Atlas project and Codex application by default and prints both in the context rail. There is no install step and no network dependency.

Run the complete guided journey with:

```sh
python3 prototypes/ale-22/terminal_prototype.py walkthrough \
  --fixture prototypes/ale-22/fixture.json
```

The walkthrough finds and reads sources, explains availability, compares independent copies, cancels and applies an in-memory disable, reinspects the applied state, refuses managed and CodeAF controls, cancels a shared-source delete, and confirms deletion of one independent copy.

## Browser contract

Every screen begins with `FIXTURE DATA - agent behavior unverified` and shows fixture, project, application, scope, source, and validation context.

| Key | Result |
| --- | --- |
| `j` / `k` or arrows | Move the selected source or comparison target |
| `/` | Search source ID, name, and description |
| `Enter` | Open a source or accept the current non-destructive selection |
| `Esc` | Go back or cancel a preview |
| `Tab` | Move through Read, Why, Compare, and Control |
| `a` | Cycle the visible agent application |
| `p` | Toggle between Atlas and global project context |
| `c` | Compare against another source ID |
| `d` | Preview scoped disable, which retains content |
| `D` | Preview permanent bundle deletion |
| `q` | Quit from inventory |

At 96 columns and wider, inventory and detail share a two-column view. Narrower terminals use one-column cards. The 60-column layout is the selected usability floor. A 48-column capture deliberately stresses that decision. Long paths are middle-elided only in the rail; detail, explanation, and previews wrap the complete value.

Wide layouts keep each duplicate-name marker intact. After an action, its result appears between the repeated context rail and the key footer so cancellation, refusal, mismatch, and applied-state feedback remain visible while the next screen is active.

`d` and `D` remain separate keys and use full action labels. A browser disable requires `y`. A browser delete requires the exact, case-sensitive phrase `DELETE <SOURCE_ID>` after its permanent-delete preview.

## Ordinary command contract

The proposed public spelling is `skillglass`. This Python prototype exposes the same shape:

```text
python3 terminal_prototype.py list --fixture FILE --project PATH [--query TEXT]
python3 terminal_prototype.py read SOURCE_ID --fixture FILE
python3 terminal_prototype.py explain SOURCE_ID --agent APP --project PATH --fixture FILE
python3 terminal_prototype.py compare LEFT_ID RIGHT_ID --fixture FILE
python3 terminal_prototype.py disable SOURCE_ID --agent APP --scope global|project --project PATH --fixture FILE [--apply]
python3 terminal_prototype.py delete SOURCE_ID --fixture FILE [--confirm-delete SOURCE_ID]
python3 terminal_prototype.py browse --fixture FILE --project PATH
```

Disable prints a preview first. At a TTY it asks `Apply this disable? [y/N]`; redirected input requires `--apply`. The prototype-only `--preview` option makes a scripted preview exit successfully without a prompt.

Delete never accepts a generic yes flag. At a TTY it requires `DELETE <SOURCE_ID>`. Redirected input requires the matching `--confirm-delete SOURCE_ID`. Missing or mismatched confirmation exits `2` without a state change. Unsupported, managed, or stale operations exit `3`. Cancellation exits `0`. Ctrl-C exits `130`.

Malformed invocations print `FIXTURE DATA - agent behavior unverified` before usage or error text and exit `2`.

A display name cannot select a control target. `copy.codex` and `copy.claude` are independent sources named `review`; `shared.research` is one source with Codex, Hermes, and CodeAF references.

## Fixture model

The domain is normalized into three collections:

- `sources` own identity, bundle content, resource inventory, digest, path text, and ownership.
- `references` retain application and project provenance for every path to a source.
- `rules` describe contextual state, precedence, certainty, and proposed control support.

Identity is always `sources.id`, never a display name. A preview records source ID, path, owner, digest, and in-memory state digest, then rechecks those values before apply. This demonstrates the stale-preview interaction contract; it does not prove a production filesystem implementation.

Fixture assertions use only `declared`, `inferred`, and `unknown`. `unverified` in the rail means no real agent behavior has been validated.

For the selected source and application, the first rule matching the selected project wins; otherwise the first global rule wins. A project winner lists global rules as lower-precedence context. Rules for other projects and equal-precedence project rules are not shadowed; a global winner has no lower-precedence context. No matching rule yields `unknown`. This is fixture logic, not validated native precedence.

## Repeatable evidence

The same script drives semantic commands and real pseudoterminals at 48, 60, 80, the 96-column layout boundary, and 120 columns:

```sh
python3 prototypes/ale-22/terminal_prototype.py verify \
  --fixture prototypes/ale-22/fixture.json \
  --output /tmp/ale-22-evidence
```

The verifier copies only the inert fixture into a temporary root, supplies temporary HOME, XDG, and TMPDIR values, checks command text and exit status, hashes the fixture after every case, drives the browser through a PTY, requires action feedback in the live 30-row viewport, rejects fragmented duplicate markers and horizontal overflow, rejects bare line feeds that would produce staircase rendering in a real terminal, and saves text captures. It is a walkthrough harness for this throwaway artifact, not production test architecture.

The context and navigation regression checks run separately:

```sh
python3 -B -m unittest discover -s prototypes/ale-22 -p 'test_*.py' -v
```

Earlier transcript captures normalized carriage returns and therefore cannot prove terminal rendering. Actual tmux pane captures supersede that rendering evidence. The semantic and fixture-isolation checks remain valid.

## Decisions carried forward

- Source ID is the control boundary. Matching names and matching content do not merge independent sources.
- Context remains visible at inventory, detail, explanation, comparison, preview, refusal, and result screens.
- Availability state and evidence certainty are separate. `explicit-only`, `disabled`, `unsupported`, and `unknown` remain distinct.
- Disable retains content and targets one application and scope. Delete targets one user-owned source and lists known reference effects.
- Managed plugin controls are refused when only broader whole-plugin management is known.
- CodeAF persistent per-skill disable is unsupported. Conversation detachment and another application's setting are not evidence of CodeAF disablement.
- Confirmed ordinary-command actions print the full relevant mutable state and unchanged-state checks. The browser reports the result and unchanged-state checks in its status area; an applied disable opens Why for reinspection.

## Unresolved implementation facts

This prototype does not settle real discovery roots, source identity across filesystems, native configuration schemas, precedence, reload behavior, per-version writer support, plugin management APIs, CodeAF session observation, production deletion boundaries, stale filesystem races, interrupted writes, recovery behavior, production Go structure, or the ALE-23 test harness. Every production writer remains disabled until its application, operation, scope, and version are validated independently in a disposable environment.
