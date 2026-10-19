# Direct reading and availability controls

Use `skillglass help` for complete current syntax. Flags precede source IDs and paths.

```sh
skillglass list --agent codex --json
skillglass read --agent codex --json SOURCE_ID
skillglass read-resource --agent codex --json SOURCE_ID references/guide.md
skillglass explain --agent codex --json SOURCE_ID
skillglass disable --agent codex --scope global --json SOURCE_ID
```

Copy the exact ID from the selected application's inventory. For a selected project, include `--project PATH` consistently. `read-resource` reads one regular bundle-relative file up to 1 MiB; binary bytes and failed reads are not interpreted evidence. Inspect `coverage.discovery`, `coverage.selected` and diagnostics independently. A partial inventory does not prove complete collection coverage.

The last command is a preview. Inspect its source, application, scope, settings effects, shared references and limitations. Ask for explicit user approval of those effects, then repeat the command with `--confirm TOKEN` from that exact preview. A changed preview needs renewed approval. Use `enable` similarly. Native support varies by application and scope; refusal never authorizes a broader fallback. `--agents APP --agents APP` previews one exact source across an explicit application group, in flag order. It is not a collection batch.

Re-inspect after a confirmed direct change. Report persisted configuration, effective configured outcome and reload/session limits separately. A preview or unsupported request writes nothing. A confirmed command may stop after a partial result; report actual results without claiming rollback or atomicity.
