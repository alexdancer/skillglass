#!/usr/bin/env python3
"""Throwaway ALE-22 terminal interaction prototype.

This program reads one inert JSON fixture. Canonical and discovered paths inside
that fixture are display strings only. Disable and delete actions mutate an
in-memory copy that disappears when the process exits.
"""

from __future__ import annotations

import argparse
import copy
import difflib
import fcntl
import hashlib
import json
import os
import pty
import re
import select
import shutil
import struct
import subprocess
import sys
import tempfile
import termios
import textwrap
import time
import tty
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Sequence


BANNER = "FIXTURE DATA - agent behavior unverified"
EXIT_RUNTIME = 1
EXIT_USAGE = 2
EXIT_REFUSED = 3
EXIT_INTERRUPTED = 130
ANSI_RE = re.compile(r"\x1b\[[0-?]*[ -/]*[@-~]")


class PrototypeError(Exception):
    exit_code = EXIT_RUNTIME


class UsageRefusal(PrototypeError):
    exit_code = EXIT_USAGE


class OperationRefusal(PrototypeError):
    exit_code = EXIT_REFUSED


class FixtureArgumentParser(argparse.ArgumentParser):
    def error(self, message: str) -> None:
        print(BANNER, file=sys.stderr)
        self.print_usage(sys.stderr)
        print(f"ERROR: {message}", file=sys.stderr)
        raise SystemExit(EXIT_USAGE)


def stable_digest(value: Any) -> str:
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def middle_elide(value: str, width: int) -> str:
    if len(value) <= width:
        return value
    if width <= 7:
        return value[:width]
    left = (width - 3) // 2
    right = width - 3 - left
    return f"{value[:left]}...{value[-right:]}"


def owner_label(source: dict[str, Any]) -> str:
    owner = source["owner"]
    if owner["kind"] == "plugin":
        return f"managed plugin {owner['plugin_id']}"
    return owner["kind"]


@dataclass(frozen=True)
class ViewContext:
    project: str | None
    application: str
    scope: str
    source_id: str


class FixtureState:
    """Normalized fixture state and pure operation plans."""

    def __init__(self, document: dict[str, Any], fixture_path: Path):
        self.document = copy.deepcopy(document)
        self.fixture_path = fixture_path
        self._validate()

    @classmethod
    def load(cls, fixture_path: str) -> "FixtureState":
        path = Path(fixture_path)
        if not path.is_file():
            raise UsageRefusal(f"Fixture file does not exist: {fixture_path}")
        try:
            document = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as error:
            raise UsageRefusal(f"Cannot read fixture JSON: {error}") from error
        return cls(document, path)

    def _validate(self) -> None:
        required = {"fixture", "sources", "references", "rules"}
        missing = sorted(required - self.document.keys())
        if missing:
            raise UsageRefusal(f"Fixture is missing top-level fields: {', '.join(missing)}")
        meta = self.document["fixture"]
        if meta.get("validation") != "unverified":
            raise UsageRefusal("Fixture validation must be 'unverified'")
        sources = self.document["sources"]
        source_ids = [source.get("id") for source in sources]
        if len(source_ids) != len(set(source_ids)) or any(not value for value in source_ids):
            raise UsageRefusal("Fixture source IDs must be present and unique")
        known = set(source_ids)
        dangling = [ref.get("id", "<unknown>") for ref in self.document["references"] if ref.get("source_id") not in known]
        if dangling:
            raise UsageRefusal(f"Fixture references unknown sources: {', '.join(dangling)}")
        bad_certainty = [
            rule.get("source_id", "<unknown>")
            for rule in self.document["rules"]
            if rule.get("certainty") not in {"declared", "inferred", "unknown"}
        ]
        if bad_certainty:
            raise UsageRefusal("Fixture certainty must be declared, inferred, or unknown")

    @property
    def meta(self) -> dict[str, Any]:
        return self.document["fixture"]

    @property
    def sources(self) -> list[dict[str, Any]]:
        return self.document["sources"]

    @property
    def references(self) -> list[dict[str, Any]]:
        return self.document["references"]

    @property
    def rules(self) -> list[dict[str, Any]]:
        return self.document["rules"]

    def digest(self) -> str:
        return stable_digest(self.document)

    def source(self, selector: str, mutation: bool = False) -> dict[str, Any]:
        exact = [source for source in self.sources if source["id"] == selector]
        if exact:
            return exact[0]
        names = [source for source in self.sources if source["name"] == selector]
        if names:
            ids = ", ".join(source["id"] for source in names)
            action = "Mutations require" if mutation else "Use"
            raise UsageRefusal(f"'{selector}' is a display name, not a source ID. {action} one of: {ids}")
        raise UsageRefusal(f"Unknown source ID: {selector}")

    def references_for(self, source_id: str) -> list[dict[str, Any]]:
        return [ref for ref in self.references if ref["source_id"] == source_id]

    def duplicate_name_ids(self, source: dict[str, Any]) -> list[str]:
        return [item["id"] for item in self.sources if item["name"] == source["name"] and item["id"] != source["id"]]

    def applications(self) -> list[str]:
        values = sorted({ref["application"] for ref in self.references})
        default = self.meta["default_application"]
        return [default] + [value for value in values if value != default]

    def application_for(self, source_id: str) -> str:
        refs = self.references_for(source_id)
        return refs[0]["application"] if refs else self.meta["default_application"]

    def rule_candidates(self, source_id: str, application: str) -> list[tuple[int, dict[str, Any]]]:
        return [
            (index, rule)
            for index, rule in enumerate(self.rules)
            if rule["source_id"] == source_id and rule["application"].casefold() == application.casefold()
        ]

    def effective_rule(self, source_id: str, application: str, project: str | None) -> tuple[int | None, dict[str, Any], list[dict[str, Any]]]:
        candidates = self.rule_candidates(source_id, application)
        if project:
            exact = [(index, rule) for index, rule in candidates if rule.get("project") == project]
            if exact:
                winner_index, winner = exact[0]
                shadowed = [rule for _, rule in candidates if rule.get("project") is None]
                return winner_index, winner, shadowed
        global_rules = [(index, rule) for index, rule in candidates if rule.get("project") is None]
        if global_rules:
            winner_index, winner = global_rules[0]
            return winner_index, winner, []
        unknown = {
            "source_id": source_id,
            "application": application,
            "project": project,
            "layer": "no fixture rule",
            "state": "unknown",
            "certainty": "unknown",
            "reason": "The fixture contains no availability rule for this source, application, and project.",
            "observation": "unknown",
            "control": {"disable": "unverified", "project_override": "unverified", "delete": "unverified"},
        }
        return None, unknown, []

    def compare(self, left_id: str, right_id: str) -> dict[str, Any]:
        left = self.source(left_id)
        right = self.source(right_id)
        if left_id == right_id:
            return {"left": left, "right": right, "same_source": True, "instructions": [], "resources": []}
        instruction_diff = list(
            difflib.unified_diff(
                left["instructions"].splitlines(),
                right["instructions"].splitlines(),
                fromfile=left_id,
                tofile=right_id,
                lineterm="",
            )
        )
        left_resources = {item["path"]: item for item in left["resources"]}
        right_resources = {item["path"]: item for item in right["resources"]}
        changes: list[str] = []
        for path in sorted(left_resources.keys() | right_resources.keys()):
            if path not in left_resources:
                changes.append(f"ADDED {path} ({right_resources[path]['digest']})")
            elif path not in right_resources:
                changes.append(f"REMOVED {path} ({left_resources[path]['digest']})")
            elif left_resources[path]["digest"] == right_resources[path]["digest"]:
                changes.append(f"IDENTICAL {path} ({left_resources[path]['digest']})")
            else:
                changes.append(
                    f"CHANGED {path} ({left_resources[path]['digest']} -> {right_resources[path]['digest']})"
                )
        return {
            "left": left,
            "right": right,
            "same_source": False,
            "instructions": instruction_diff,
            "resources": changes,
        }

    def disable_plan(self, source_id: str, application: str, scope: str, project: str | None) -> dict[str, Any]:
        source = self.source(source_id, mutation=True)
        if scope == "project" and not project:
            raise UsageRefusal("Project scope requires --project")
        selected_project = project if scope == "project" else None
        rule_index, rule, _ = self.effective_rule(source_id, application, selected_project)
        support = rule["control"]["disable"]
        if source["owner"]["kind"] == "plugin":
            raise OperationRefusal(
                f"REFUSED: {source_id} is managed by {source['owner']['plugin_id']}. Isolated per-skill disable is unavailable; whole-plugin management is broader than this request."
            )
        if support == "unsupported":
            extra = ""
            if application.casefold() == "codeaf":
                extra = " Conversation detachment and another application's setting are not evidence of persistent CodeAF disablement."
            raise OperationRefusal(f"UNSUPPORTED: persistent per-skill disable for {application}.{extra}")
        if support not in {"supported-in-prototype"}:
            raise OperationRefusal(
                f"UNVERIFIED: no validated {application} writer exists for {scope} scope. The fixture remains unchanged."
            )
        if scope == "project" and rule.get("project") != project:
            rule_index = None
        return {
            "kind": "disable",
            "source_id": source_id,
            "source_path": source["canonical_path"],
            "owner": copy.deepcopy(source["owner"]),
            "source_digest": source["digest"],
            "application": application,
            "scope": scope,
            "project": selected_project,
            "before": rule["state"],
            "after": "disabled",
            "rule_index": rule_index,
            "fixture_digest": self.digest(),
        }

    def _assert_plan_current(self, plan: dict[str, Any]) -> dict[str, Any]:
        if self.digest() != plan["fixture_digest"]:
            raise OperationRefusal("REFUSED: preview is stale because in-memory state changed")
        source = self.source(plan["source_id"], mutation=True)
        if (
            source["canonical_path"] != plan["source_path"]
            or source["owner"] != plan["owner"]
            or source["digest"] != plan["source_digest"]
        ):
            raise OperationRefusal("REFUSED: source identity no longer matches the preview")
        return source

    def _unrelated_projection(self, source_id: str, application: str, project: str | None) -> dict[str, Any]:
        return {
            "sources": copy.deepcopy(self.sources),
            "references": copy.deepcopy(self.references),
            "rules": [
                copy.deepcopy(rule)
                for rule in self.rules
                if not (
                    rule["source_id"] == source_id
                    and rule["application"].casefold() == application.casefold()
                    and rule.get("project") == project
                )
            ],
        }

    def apply_disable(self, plan: dict[str, Any]) -> dict[str, Any]:
        self._assert_plan_current(plan)
        before_unrelated = stable_digest(
            self._unrelated_projection(plan["source_id"], plan["application"], plan["project"])
        )
        rule_index = plan["rule_index"]
        if rule_index is None:
            self.rules.append(
                {
                    "source_id": plan["source_id"],
                    "application": plan["application"],
                    "project": plan["project"],
                    "layer": "project override" if plan["scope"] == "project" else "global",
                    "state": "disabled",
                    "certainty": "inferred",
                    "reason": "The in-memory prototype applied the previewed disable.",
                    "observation": "unknown",
                    "control": {
                        "disable": "supported-in-prototype",
                        "project_override": "supported-in-prototype",
                        "delete": "supported-in-prototype",
                    },
                }
            )
        else:
            self.rules[rule_index]["state"] = "disabled"
            self.rules[rule_index]["certainty"] = "inferred"
            self.rules[rule_index]["reason"] = "The in-memory prototype applied the previewed disable."
        after_unrelated = stable_digest(
            self._unrelated_projection(plan["source_id"], plan["application"], plan["project"])
        )
        return {
            "before": plan["before"],
            "after": "disabled",
            "unrelated_digest_before": before_unrelated,
            "unrelated_digest_after": after_unrelated,
            "unrelated_unchanged": before_unrelated == after_unrelated,
        }

    def delete_plan(self, source_id: str) -> dict[str, Any]:
        source = self.source(source_id, mutation=True)
        if source["owner"]["kind"] != "user":
            manager = source["owner"].get("plugin_id", source["owner"]["kind"])
            raise OperationRefusal(
                f"REFUSED: {source_id} is managed by {manager}. Isolated deletion is unavailable and no managed cache is changed."
            )
        return {
            "kind": "delete",
            "source_id": source_id,
            "source_path": source["canonical_path"],
            "owner": copy.deepcopy(source["owner"]),
            "source_digest": source["digest"],
            "references": copy.deepcopy(self.references_for(source_id)),
            "same_name_remaining": self.duplicate_name_ids(source),
            "fixture_digest": self.digest(),
        }

    def apply_delete(self, plan: dict[str, Any]) -> dict[str, Any]:
        self._assert_plan_current(plan)
        before_other_sources = {
            source["id"]: stable_digest(source) for source in self.sources if source["id"] != plan["source_id"]
        }
        self.document["sources"] = [source for source in self.sources if source["id"] != plan["source_id"]]
        self.document["references"] = [
            ref for ref in self.references if ref["source_id"] != plan["source_id"]
        ]
        self.document["rules"] = [rule for rule in self.rules if rule["source_id"] != plan["source_id"]]
        after_other_sources = {source["id"]: stable_digest(source) for source in self.sources}
        return {
            "deleted": plan["source_id"],
            "remaining_sources": [source["id"] for source in self.sources],
            "unrelated_sources_unchanged": before_other_sources == after_other_sources,
        }

    def mutable_state(self) -> dict[str, Any]:
        return {
            "sources": [
                {
                    "id": source["id"],
                    "name": source["name"],
                    "canonical_path": source["canonical_path"],
                    "owner": source["owner"],
                    "digest": source["digest"],
                }
                for source in self.sources
            ],
            "references": copy.deepcopy(self.references),
            "rules": copy.deepcopy(self.rules),
        }


class Console:
    def __init__(self, state: FixtureState, width: int | None = None):
        terminal_width = width or shutil.get_terminal_size((80, 30)).columns
        self.width = max(30, terminal_width)
        self.state = state

    def wrapped(self, value: str, width: int | None = None, indent: str = "") -> list[str]:
        use_width = max(12, (width or self.width) - len(indent))
        lines = textwrap.wrap(
            str(value),
            width=use_width,
            break_long_words=True,
            break_on_hyphens=False,
            replace_whitespace=False,
        ) or [""]
        return [f"{indent}{line}" for line in lines]

    def field(self, label: str, value: Any, width: int | None = None) -> list[str]:
        available = width or self.width
        text = "none" if value is None else str(value)
        if available < 60:
            return [f"{label}:"] + self.wrapped(text, available, "  ")
        prefix = f"{label}: "
        first_width = max(12, available - len(prefix))
        parts = textwrap.wrap(
            text,
            width=first_width,
            break_long_words=True,
            break_on_hyphens=False,
        ) or [""]
        return [prefix + parts[0]] + [(" " * len(prefix)) + part for part in parts[1:]]

    def rail(self, context: ViewContext) -> list[str]:
        fixture_id = self.state.meta["id"]
        project = context.project or "none"
        if self.width >= 96:
            line_one_prefix = f"DATA FIXTURE {fixture_id} | PROJECT "
            line_one_suffix = f" | APP {context.application}"
            project_width = max(12, self.width - len(line_one_prefix) - len(line_one_suffix))
            line_one = f"{line_one_prefix}{middle_elide(project, project_width)}{line_one_suffix}"
            line_two = (
                f"SCOPE {context.scope} | SOURCE {context.source_id} | VALIDATION {self.state.meta['validation']}"
            )
            return [line_one, line_two]
        if self.width >= 60:
            prefix = f"DATA FIXTURE {fixture_id} | PROJECT "
            return [
                prefix + middle_elide(project, self.width - len(prefix)),
                f"APP {context.application} | SCOPE {context.scope}",
                f"SOURCE {context.source_id} | VALIDATION {self.state.meta['validation']}",
            ]
        return [
            f"DATA FIXTURE {fixture_id}",
            f"PROJECT {middle_elide(project, self.width - len('PROJECT '))}",
            f"APP {context.application}",
            f"SCOPE {context.scope}",
            f"SOURCE {context.source_id}",
            f"VALIDATION {self.state.meta['validation']}",
        ]

    def screen(
        self,
        context: ViewContext,
        title: str,
        body: Iterable[str],
        footer: str | None = None,
        status: str | None = None,
        clear: bool = False,
        repeat_context: bool = False,
    ) -> None:
        if clear:
            print("\x1b[2J\x1b[H", end="")
        print(BANNER)
        for line in self.rail(context):
            print(line)
        print("-" * min(self.width, 72))
        print(title)
        for line in body:
            print(line)
        if repeat_context:
            print("-" * min(self.width, 72))
            for line in self.rail(context):
                print(line)
        if status:
            print("-" * min(self.width, 72))
            for line in self.wrapped(status):
                print(line)
        if footer:
            print("-" * min(self.width, 72))
            for line in self.wrapped(footer):
                print(line)
        sys.stdout.flush()

    def fields(self, values: Sequence[tuple[str, Any]], width: int | None = None) -> list[str]:
        lines: list[str] = []
        for label, value in values:
            lines.extend(self.field(label, value, width))
        return lines

    def pane_widths(self) -> tuple[int, int]:
        left_width = min(44, max(34, self.width // 3))
        right_width = self.width - left_width - 3
        return left_width, right_width

    def two_pane(
        self,
        left: list[str],
        right: list[str],
        widths: tuple[int, int] | None = None,
    ) -> list[str]:
        left_width, right_width = widths or self.pane_widths()
        left_wrapped = self._rewrap_lines(left, left_width)
        right_wrapped = self._rewrap_lines(right, right_width)
        count = max(len(left_wrapped), len(right_wrapped))
        result = []
        for index in range(count):
            left_line = left_wrapped[index] if index < len(left_wrapped) else ""
            right_line = right_wrapped[index] if index < len(right_wrapped) else ""
            result.append(f"{left_line:<{left_width}} | {right_line}")
        return result

    def _rewrap_lines(self, lines: list[str], width: int) -> list[str]:
        result: list[str] = []
        for line in lines:
            result.extend(self.wrapped(line, width))
        return result


def default_context(
    state: FixtureState,
    source_id: str = "none",
    application: str | None = None,
    project: str | None = None,
    scope: str | None = None,
) -> ViewContext:
    return ViewContext(
        project=state.meta["default_project"] if project is None else project,
        application=application or state.meta["default_application"],
        scope=scope or state.meta["default_scope"],
        source_id=source_id,
    )


def list_lines(state: FixtureState, console: Console, query: str = "") -> list[str]:
    needle = query.casefold().strip()
    sources = [
        source
        for source in state.sources
        if not needle
        or needle in source["id"].casefold()
        or needle in source["name"].casefold()
        or needle in source["description"].casefold()
    ]
    lines: list[str] = []
    if query:
        lines.extend(console.field("Search", query))
    if not sources:
        return lines + ["No fixture sources match the search."]
    for source in sources:
        refs = state.references_for(source["id"])
        warning = " | DUPLICATE NAME: independent copy" if state.duplicate_name_ids(source) else ""
        lines.extend(
            console.wrapped(
                f"{source['id']} | name {source['name']} | owner {owner_label(source)} | references {len(refs)}{warning}"
            )
        )
        lines.extend(console.field("  path", source["canonical_path"]))
        if refs:
            lines.extend(console.field("  applications", ", ".join(ref["application"] for ref in refs)))
    return lines


def read_lines(state: FixtureState, console: Console, source: dict[str, Any], width: int | None = None) -> list[str]:
    refs = state.references_for(source["id"])
    lines = console.fields(
        [
            ("Source ID", source["id"]),
            ("Name", source["name"]),
            ("Owner", owner_label(source)),
            ("Canonical path", source["canonical_path"]),
            ("Digest", source["digest"]),
            ("Certainty", "declared fixture content; native-unverified"),
            ("Instructions", source["instructions"]),
        ],
        width,
    )
    lines.append("Resources:")
    for resource in source["resources"]:
        lines.extend(
            console.wrapped(
                f"  {resource['path']} | {resource['kind']} | {resource['digest']} | {resource['summary']}", width
            )
        )
    lines.append("Shared references:" if len(refs) > 1 else "References:")
    for ref in refs:
        project = f" | project {ref['project']}" if ref.get("project") else ""
        lines.extend(
            console.wrapped(
                f"  {ref['id']} | {ref['application']} | {ref['discovered_path']}{project}", width
            )
        )
    return lines


def explain_lines(
    state: FixtureState,
    console: Console,
    source_id: str,
    application: str,
    project: str | None,
    width: int | None = None,
) -> tuple[list[str], str]:
    _, winner, shadowed = state.effective_rule(source_id, application, project)
    scope = "project" if winner.get("project") else "global"
    values = [
        ("Effective state", winner["state"]),
        ("Winning source", source_id),
        ("Application", application),
        ("Selected project", project or "none"),
        ("Winning layer", winner["layer"]),
        ("Precedence scope", scope),
        ("Certainty", winner["certainty"]),
        ("Reason", winner["reason"]),
        ("Session observation", winner.get("observation", "unknown")),
        ("Disable control", winner["control"]["disable"]),
        ("Project override control", winner["control"]["project_override"]),
        ("Delete control", winner["control"]["delete"]),
        ("Native agent enforcement", "not validated"),
    ]
    lines = console.fields(values, width)
    if winner["state"] == "explicit-only":
        lines.extend(console.wrapped("Meaning: explicit-only is a distinct state; it is not disabled.", width))
    if shadowed:
        lines.append("Lower-precedence context:")
        for rule in shadowed:
            project_label = rule.get("project") or "global"
            lines.extend(
                console.wrapped(
                    f"  {rule['layer']} | {project_label} | {rule['state']} | certainty {rule['certainty']}", width
                )
            )
    return lines, scope


def compare_lines(state: FixtureState, console: Console, left_id: str, right_id: str, width: int | None = None) -> list[str]:
    result = state.compare(left_id, right_id)
    left = result["left"]
    right = result["right"]
    lines = console.fields(
        [
            ("Left source", left["id"]),
            ("Left path", left["canonical_path"]),
            ("Left digest", left["digest"]),
            ("Right source", right["id"]),
            ("Right path", right["canonical_path"]),
            ("Right digest", right["digest"]),
            ("Write result", "none; comparison is read-only"),
            ("Certainty", "declared fixture bundles; native-unverified"),
        ],
        width,
    )
    if result["same_source"]:
        lines.extend(console.wrapped("SHARED SOURCE: both selections identify one bundle, not independent copies.", width))
        return lines
    lines.append("Instruction diff:")
    for line in result["instructions"] or ["  IDENTICAL"]:
        lines.extend(console.wrapped(f"  {line}", width))
    lines.append("Resource comparison:")
    for line in result["resources"] or ["IDENTICAL resource inventory"]:
        lines.extend(console.wrapped(f"  {line}", width))
    return lines


def disable_preview_lines(state: FixtureState, console: Console, plan: dict[str, Any], width: int | None = None) -> list[str]:
    return console.fields(
        [
            ("Action", "DISABLE AVAILABILITY - content retained"),
            ("Source ID", plan["source_id"]),
            ("Canonical path", plan["source_path"]),
            ("Application", plan["application"]),
            ("Scope", plan["scope"]),
            ("Project", plan["project"] or "none"),
            (
                "Affected fixture rule key",
                f"source_id={plan['source_id']} | application={plan['application']} | project={plan['project'] or 'global'}",
            ),
            ("Current state", plan["before"]),
            ("Proposed state", plan["after"]),
            ("Content", "remains in the in-memory collection"),
            ("Certainty", "inferred prototype transition; native-unverified"),
            ("Recheck", "source ID, canonical path, owner, digest, and preview state before apply"),
        ],
        width,
    )


def delete_preview_lines(state: FixtureState, console: Console, plan: dict[str, Any], width: int | None = None) -> list[str]:
    lines = console.fields(
        [
            ("Action", "PERMANENT - no built-in recovery"),
            ("Simulation boundary", "removes only the in-memory fixture source for this process"),
            ("Source ID", plan["source_id"]),
            ("Canonical path", plan["source_path"]),
            ("Owner", plan["owner"]["kind"]),
            ("Certainty", "declared fixture ownership; native-unverified"),
            ("External link targets", "not followed"),
        ],
        width,
    )
    lines.append("Known shared references affected:")
    if plan["references"]:
        for ref in plan["references"]:
            lines.extend(
                console.wrapped(f"  {ref['application']} | {ref['id']} | {ref['discovered_path']}", width)
            )
    else:
        lines.append("  none")
    remaining = plan["same_name_remaining"] or ["none"]
    lines.extend(console.field("Independent same-name copies that remain", ", ".join(remaining), width))
    return lines


def state_lines(state: FixtureState, console: Console, width: int | None = None) -> list[str]:
    snapshot = state.mutable_state()
    lines = ["FULL RELEVANT IN-MEMORY STATE"]
    lines.append("Sources:")
    for source in snapshot["sources"]:
        lines.extend(console.wrapped(f"  {source['id']} | {source['digest']} | {source['canonical_path']}", width))
    lines.append("References:")
    for ref in snapshot["references"]:
        project = ref.get("project") or "global"
        lines.extend(console.wrapped(f"  {ref['id']} | {ref['source_id']} | {ref['application']} | {project}", width))
    lines.append("Availability rules:")
    for rule in snapshot["rules"]:
        project = rule.get("project") or "global"
        lines.extend(
            console.wrapped(
                f"  {rule['source_id']} | {rule['application']} | {project} | {rule['state']} | {rule['certainty']}", width
            )
        )
    return lines


def show_refusal(state: FixtureState, console: Console, context: ViewContext, message: str) -> None:
    body = console.wrapped(message)
    body.extend(state_lines(state, console))
    console.screen(context, "CONTROL REFUSED", body)


def command_list(state: FixtureState, args: argparse.Namespace) -> int:
    console = Console(state)
    context = default_context(state, project=args.project)
    console.screen(context, "SOURCE INVENTORY", list_lines(state, console, args.query or ""))
    return 0


def command_read(state: FixtureState, args: argparse.Namespace) -> int:
    source = state.source(args.source_id)
    app = state.application_for(source["id"])
    console = Console(state)
    context = default_context(state, source["id"], app, args.project)
    console.screen(context, f"READ {source['id']}", read_lines(state, console, source))
    return 0


def command_explain(state: FixtureState, args: argparse.Namespace) -> int:
    source = state.source(args.source_id)
    project = args.project or state.meta["default_project"]
    console = Console(state)
    body, scope = explain_lines(state, console, source["id"], args.agent, project)
    context = default_context(state, source["id"], args.agent, project, scope)
    console.screen(context, f"WHY {source['id']}", body)
    return 0


def command_compare(state: FixtureState, args: argparse.Namespace) -> int:
    state.source(args.left_id)
    state.source(args.right_id)
    console = Console(state)
    context = default_context(state, f"{args.left_id}<->{args.right_id}", "all", args.project, "read-only")
    body = compare_lines(state, console, args.left_id, args.right_id)
    console.screen(context, "BUNDLE COMPARISON", body)
    return 0


def command_disable(state: FixtureState, args: argparse.Namespace) -> int:
    console = Console(state)
    project = args.project if args.scope == "project" else None
    context = default_context(state, args.source_id, args.agent, project, args.scope)
    try:
        plan = state.disable_plan(args.source_id, args.agent, args.scope, project)
    except OperationRefusal as error:
        show_refusal(state, console, context, str(error))
        return error.exit_code
    console.screen(
        context,
        "DISABLE PREVIEW",
        disable_preview_lines(state, console, plan),
        repeat_context=sys.stdin.isatty(),
    )
    if args.preview:
        print("Preview only - no fixture state changed")
        return 0
    apply = args.apply
    if not apply:
        if not sys.stdin.isatty():
            print("Noninteractive disable requires --apply; no fixture state changed")
            return EXIT_USAGE
        try:
            response = input("Apply this disable? [y/N] ").strip()
        except EOFError:
            response = ""
        if response.casefold() != "y":
            print("Cancelled - no fixture state changed")
            return 0
    result = state.apply_disable(plan)
    body = console.fields(
        [
            ("Result", "APPLIED to in-memory fixture state"),
            ("Before", result["before"]),
            ("After", result["after"]),
            ("Unrelated application and project rules unchanged", str(result["unrelated_unchanged"]).lower()),
            ("Fixture file", "unchanged"),
            ("Native enforcement", "not validated"),
        ]
    )
    body.extend(state_lines(state, console))
    console.screen(context, "DISABLE RESULT", body)
    return 0


def command_delete(state: FixtureState, args: argparse.Namespace) -> int:
    console = Console(state)
    context = default_context(state, args.source_id, state.application_for(args.source_id) if any(s["id"] == args.source_id for s in state.sources) else state.meta["default_application"], args.project, "content deletion")
    try:
        plan = state.delete_plan(args.source_id)
    except OperationRefusal as error:
        show_refusal(state, console, context, str(error))
        return error.exit_code
    console.screen(
        context,
        "DELETE PREVIEW",
        delete_preview_lines(state, console, plan),
        repeat_context=sys.stdin.isatty(),
    )
    confirmed = False
    if args.confirm_delete is not None:
        if args.confirm_delete != args.source_id:
            print(f"Confirmation mismatch. Required --confirm-delete {args.source_id}; no fixture state changed")
            return EXIT_USAGE
        confirmed = True
    elif not sys.stdin.isatty():
        print(f"Missing target-specific confirmation. Required --confirm-delete {args.source_id}; no fixture state changed")
        return EXIT_USAGE
    else:
        print("Type this exact phrase:")
        print(f"DELETE {args.source_id}")
        try:
            response = input("Confirmation: ").strip()
        except EOFError:
            response = ""
        if response in {"", "n", "N", "\x1b"}:
            print("Cancelled - no fixture state changed")
            return 0
        if response != f"DELETE {args.source_id}":
            print("Confirmation did not match; no fixture state changed")
            return EXIT_USAGE
        confirmed = True
    if not confirmed:
        raise AssertionError("confirmation state is unreachable")
    result = state.apply_delete(plan)
    body = console.fields(
        [
            ("Result", f"Deleted {result['deleted']} permanently from this in-memory run"),
            ("Canonical path", plan["source_path"]),
            ("Fixture file", "unchanged"),
            ("Unrelated sources unchanged", str(result["unrelated_sources_unchanged"]).lower()),
            ("Recovery", "no built-in recovery in the proposed contract"),
        ]
    )
    body.extend(state_lines(state, console))
    console.screen(context, "DELETE RESULT", body)
    return 0


class RawTerminal:
    def __init__(self) -> None:
        self.fd = sys.stdin.fileno()
        self.original: list[Any] | None = None

    def __enter__(self) -> "RawTerminal":
        self.original = termios.tcgetattr(self.fd)
        tty.setcbreak(self.fd)
        return self

    def __exit__(self, exc_type: Any, exc: Any, traceback: Any) -> None:
        if self.original is not None:
            termios.tcsetattr(self.fd, termios.TCSADRAIN, self.original)

    def read_key(self) -> str:
        first = os.read(self.fd, 1)
        if first == b"\x03":
            raise KeyboardInterrupt
        if first != b"\x1b":
            return first.decode("utf-8", errors="ignore")
        sequence = bytearray(first)
        deadline = time.monotonic() + 0.04
        while time.monotonic() < deadline:
            ready, _, _ = select.select([self.fd], [], [], 0.005)
            if not ready:
                continue
            sequence.extend(os.read(self.fd, 8))
            if bytes(sequence) in {b"\x1b[A", b"\x1b[B"}:
                break
        if bytes(sequence) == b"\x1b[A":
            return "UP"
        if bytes(sequence) == b"\x1b[B":
            return "DOWN"
        return "ESC"

    def prompt_line(self, prompt: str) -> str:
        if self.original is None:
            raise RuntimeError("raw terminal not active")
        termios.tcsetattr(self.fd, termios.TCSADRAIN, self.original)
        try:
            print(prompt, end="", flush=True)
            return sys.stdin.readline().rstrip("\r\n")
        finally:
            tty.setcbreak(self.fd)


class Browser:
    sections = ["Read", "Why", "Compare", "Control"]

    def __init__(self, state: FixtureState, project: str | None, application: str):
        self.state = state
        self.project = project
        self.application = application
        self.query = ""
        self.selected_index = 0
        self.mode = "inventory"
        self.section_index = 0
        self.compare_index = 0
        self.compare_source_id: str | None = None
        self.pending_plan: dict[str, Any] | None = None
        self.flash = ""

    def visible_sources(self) -> list[dict[str, Any]]:
        needle = self.query.casefold().strip()
        values = [
            source
            for source in self.state.sources
            if not needle
            or needle in source["id"].casefold()
            or needle in source["name"].casefold()
            or needle in source["description"].casefold()
        ]
        if values:
            self.selected_index %= len(values)
        else:
            self.selected_index = 0
        return values

    def selected_source(self) -> dict[str, Any] | None:
        values = self.visible_sources()
        return values[self.selected_index] if values else None

    def scope(self, source_id: str) -> str:
        _, rule, _ = self.state.effective_rule(source_id, self.application, self.project)
        return "project" if rule.get("project") else "global"

    def context(self) -> ViewContext:
        source = self.selected_source()
        source_id = source["id"] if source else "none"
        scope = "content deletion" if self.mode == "delete_preview" else self.scope(source_id) if source else "inventory"
        return ViewContext(self.project, self.application, scope, source_id)

    def inventory_panel(self, console: Console, width: int | None = None) -> list[str]:
        sources = self.visible_sources()
        lines = [f"Search: {self.query or '<all>'}"]
        if not sources:
            return lines + ["No matching sources"]
        for index, source in enumerate(sources):
            marker = ">" if index == self.selected_index else " "
            duplicate = " [duplicate name]" if self.state.duplicate_name_ids(source) else ""
            refs = len(self.state.references_for(source["id"]))
            lines.extend(console.wrapped(f"{marker} {source['id']} | {source['name']} | refs {refs}{duplicate}", width))
        return lines

    def selected_summary(self, console: Console, width: int | None = None) -> list[str]:
        source = self.selected_source()
        if not source:
            return ["No source selected"]
        _, rule, _ = self.state.effective_rule(source["id"], self.application, self.project)
        return console.fields(
            [
                ("Selected", source["id"]),
                ("Name", source["name"]),
                ("Path", source["canonical_path"]),
                ("Owner", owner_label(source)),
                ("State", rule["state"]),
                ("Certainty", rule["certainty"]),
            ],
            width,
        )

    def detail_panel(self, console: Console, width: int | None = None) -> list[str]:
        source = self.selected_source()
        if not source:
            return ["No source selected"]
        nav = " | ".join(
            f"[{name}]" if index == self.section_index else name for index, name in enumerate(self.sections)
        )
        lines = console.wrapped(nav, width)
        section = self.sections[self.section_index]
        if section == "Read":
            lines.extend(read_lines(self.state, console, source, width))
        elif section == "Why":
            explanation, _ = explain_lines(
                self.state, console, source["id"], self.application, self.project, width
            )
            lines.extend(explanation)
        elif section == "Compare":
            lines.extend(console.wrapped("Press c or Enter to choose another source by source ID.", width))
            lines.extend(self.selected_summary(console, width))
        else:
            _, rule, _ = self.state.effective_rule(source["id"], self.application, self.project)
            lines.extend(
                console.fields(
                    [
                        ("Disable availability", rule["control"]["disable"]),
                        ("Permanently delete bundle", rule["control"]["delete"]),
                        ("Disable key", "d"),
                        ("Delete key", "D"),
                    ],
                    width,
                )
            )
        return lines

    def compare_choices(self) -> list[dict[str, Any]]:
        selected = self.selected_source()
        if not selected:
            return []
        values = [source for source in self.state.sources if source["id"] != selected["id"]]
        same_name = [source for source in values if source["name"] == selected["name"]]
        if same_name:
            first_id = same_name[0]["id"]
            values.sort(key=lambda source: 0 if source["id"] == first_id else 1)
        if values:
            self.compare_index %= len(values)
        return values

    def render(self) -> None:
        console = Console(self.state)
        source = self.selected_source()
        context = self.context()
        pane_widths = console.pane_widths() if console.width >= 96 else None
        footer = "j/k or arrows move | / search | Enter open | Esc back/cancel | Tab section | a app | p project | c compare | d disable preview | D delete preview | q quit"
        body: list[str]
        title = "SKILLGLASS BROWSER"
        if self.mode == "inventory":
            if pane_widths:
                left_width, right_width = pane_widths
                left = self.inventory_panel(console, left_width)
                body = console.two_pane(left, self.selected_summary(console, right_width), pane_widths)
            else:
                left = self.inventory_panel(console)
                body = left
                body.extend([""] + self.selected_summary(console))
        elif self.mode == "detail":
            title = f"SOURCE DETAIL - {self.sections[self.section_index]}"
            if pane_widths:
                left_width, right_width = pane_widths
                detail = self.detail_panel(console, right_width)
                body = console.two_pane(self.inventory_panel(console, left_width), detail, pane_widths)
            else:
                body = self.detail_panel(console)
        elif self.mode == "compare_pick":
            title = "CHOOSE COMPARISON SOURCE"
            body = []
            for index, candidate in enumerate(self.compare_choices()):
                marker = ">" if index == self.compare_index else " "
                body.extend(console.wrapped(f"{marker} {candidate['id']} | {candidate['name']} | {candidate['canonical_path']}"))
        elif self.mode == "compare":
            title = "BUNDLE COMPARISON"
            if source is None or self.compare_source_id is None:
                body = ["Comparison selection is incomplete"]
            else:
                body = compare_lines(self.state, console, source["id"], self.compare_source_id)
        elif self.mode == "disable_preview":
            title = "DISABLE PREVIEW"
            body = disable_preview_lines(self.state, console, self.pending_plan or {})
            footer = "DISABLE AVAILABILITY: y apply in memory | n or Esc cancel"
        elif self.mode == "delete_preview":
            title = "DELETE PREVIEW"
            body = delete_preview_lines(self.state, console, self.pending_plan or {})
            footer = "PERMANENT DELETE: Enter type target-specific confirmation | n or Esc cancel"
        else:
            body = ["Unknown browser mode"]
        status = self.flash or None
        self.flash = ""
        console.screen(context, title, body, footer, status=status, clear=True, repeat_context=True)

    def move(self, delta: int) -> None:
        if self.mode in {"inventory", "detail"}:
            values = self.visible_sources()
            if values:
                self.selected_index = (self.selected_index + delta) % len(values)
        elif self.mode == "compare_pick":
            values = self.compare_choices()
            if values:
                self.compare_index = (self.compare_index + delta) % len(values)

    def cycle_application(self) -> None:
        values = self.state.applications()
        index = values.index(self.application) if self.application in values else 0
        self.application = values[(index + 1) % len(values)]

    def toggle_project(self) -> None:
        self.project = None if self.project else self.state.meta["default_project"]

    def begin_disable(self) -> None:
        source = self.selected_source()
        if not source:
            return
        scope = self.scope(source["id"])
        try:
            self.pending_plan = self.state.disable_plan(source["id"], self.application, scope, self.project)
        except OperationRefusal as error:
            self.flash = str(error) + " No fixture state changed."
            self.mode = "detail"
            return
        self.mode = "disable_preview"

    def begin_delete(self) -> None:
        source = self.selected_source()
        if not source:
            return
        try:
            self.pending_plan = self.state.delete_plan(source["id"])
        except OperationRefusal as error:
            self.flash = str(error) + " No fixture state changed."
            self.mode = "detail"
            return
        self.mode = "delete_preview"

    def handle_key(self, key: str, terminal: RawTerminal) -> bool:
        if key in {"j", "DOWN"}:
            self.move(1)
            return True
        if key in {"k", "UP"}:
            self.move(-1)
            return True
        if key == "a" and self.mode in {"inventory", "detail"}:
            self.cycle_application()
            return True
        if key == "p" and self.mode in {"inventory", "detail"}:
            self.toggle_project()
            return True
        if self.mode == "inventory":
            if key == "q":
                return False
            if key == "/":
                self.query = terminal.prompt_line("Search name, ID, or description: ").strip()
                self.selected_index = 0
            elif key in {"\r", "\n"} and self.selected_source():
                self.mode = "detail"
                self.section_index = 0
            return True
        if self.mode == "detail":
            if key == "ESC":
                self.mode = "inventory"
            elif key == "\t":
                self.section_index = (self.section_index + 1) % len(self.sections)
            elif key == "c":
                self.mode = "compare_pick"
                self.compare_index = 0
            elif key == "d":
                self.begin_disable()
            elif key == "D":
                self.begin_delete()
            elif key in {"\r", "\n"}:
                if self.section_index == 1:
                    pass
                elif self.section_index == 2:
                    self.mode = "compare_pick"
                    self.compare_index = 0
                elif self.section_index == 3:
                    self.begin_disable()
            return True
        if self.mode == "compare_pick":
            if key == "ESC":
                self.mode = "detail"
            elif key in {"\r", "\n"}:
                choices = self.compare_choices()
                if choices:
                    self.compare_source_id = choices[self.compare_index]["id"]
                    self.mode = "compare"
            return True
        if self.mode == "compare":
            if key == "ESC":
                self.mode = "detail"
            return True
        if self.mode == "disable_preview":
            if key in {"ESC", "n", "N"}:
                self.pending_plan = None
                self.mode = "detail"
                self.flash = "Cancelled - no fixture state changed"
            elif key in {"y", "Y"} and self.pending_plan:
                result = self.state.apply_disable(self.pending_plan)
                self.pending_plan = None
                self.mode = "detail"
                self.section_index = 1
                self.flash = (
                    "APPLIED to in-memory fixture state. "
                    f"State is {result['after']}. Unrelated application and project rules unchanged: "
                    f"{str(result['unrelated_unchanged']).lower()}. Reinspected below."
                )
            return True
        if self.mode == "delete_preview":
            if key in {"ESC", "n", "N"}:
                self.pending_plan = None
                self.mode = "detail"
                self.flash = "Cancelled - no fixture state changed"
            elif key in {"\r", "\n"} and self.pending_plan:
                target = self.pending_plan["source_id"]
                print("\r\nType this exact phrase:")
                print(f"DELETE {target}")
                response = terminal.prompt_line("Confirmation: ")
                if response == f"DELETE {target}":
                    result = self.state.apply_delete(self.pending_plan)
                    self.pending_plan = None
                    self.query = ""
                    self.selected_index = 0
                    self.mode = "inventory"
                    self.flash = (
                        f"Deleted {result['deleted']} permanently from this in-memory run. "
                        f"Unrelated sources unchanged: {str(result['unrelated_sources_unchanged']).lower()}. "
                        "Fixture file unchanged."
                    )
                elif response in {"", "n", "N", "\x1b"}:
                    self.pending_plan = None
                    self.mode = "detail"
                    self.flash = "Cancelled - no fixture state changed"
                else:
                    self.pending_plan = None
                    self.mode = "detail"
                    self.flash = "Confirmation did not match; no fixture state changed"
            return True
        return True

    def run(self) -> int:
        if not (sys.stdin.isatty() and sys.stdout.isatty()):
            console = Console(self.state)
            console.screen(self.context(), "BROWSER REQUIRES A TTY", ["Use list, read, explain, compare, disable, or delete for redirected output."])
            return EXIT_USAGE
        with RawTerminal() as terminal:
            while True:
                self.render()
                key = terminal.read_key()
                if not self.handle_key(key, terminal):
                    break
        print()
        for line in Console(self.state).wrapped("Exited fixture browser. No on-disk fixture or live setting changed."):
            print(line)
        return 0


def command_browse(state: FixtureState, args: argparse.Namespace) -> int:
    project = args.project or state.meta["default_project"]
    application = args.agent or state.meta["default_application"]
    return Browser(state, project, application).run()


def pause_walkthrough(enabled: bool, step: str) -> None:
    if enabled:
        input(f"Press Enter for {step}: ")


def command_walkthrough(state: FixtureState, args: argparse.Namespace) -> int:
    console = Console(state)
    project = args.project or state.meta["default_project"]
    pause = not args.no_pause and sys.stdin.isatty()
    steps = 0

    def show(title: str, context: ViewContext, body: list[str]) -> None:
        nonlocal steps
        steps += 1
        pause_walkthrough(pause, title)
        console.screen(context, f"GUIDED STEP {steps} - {title}", body)

    show("Find shared source", default_context(state, project=project), list_lines(state, console, "research"))
    show("Find independent same-name copies", default_context(state, project=project), list_lines(state, console, "review"))
    source = state.source("copy.codex")
    show(
        "Read selected source",
        default_context(state, source["id"], "Codex", project),
        read_lines(state, console, source),
    )
    explanation, scope = explain_lines(state, console, "copy.codex", "Codex", project)
    show("Explain availability", default_context(state, "copy.codex", "Codex", project, scope), explanation)
    show(
        "Compare independent copies",
        default_context(state, "copy.codex<->copy.claude", "all", project, "read-only"),
        compare_lines(state, console, "copy.codex", "copy.claude"),
    )
    plan = state.disable_plan("shared.research", "Codex", "project", project)
    show(
        "Preview scoped disable",
        default_context(state, "shared.research", "Codex", project, "project"),
        disable_preview_lines(state, console, plan),
    )
    show(
        "Cancel scoped disable",
        default_context(state, "shared.research", "Codex", project, "project"),
        ["Cancelled - no fixture state changed"] + state_lines(state, console),
    )
    plan = state.disable_plan("shared.research", "Codex", "project", project)
    result = state.apply_disable(plan)
    show(
        "Apply scoped disable",
        default_context(state, "shared.research", "Codex", project, "project"),
        console.fields(
            [
                ("Result", "APPLIED to in-memory fixture state"),
                ("Before", result["before"]),
                ("After", result["after"]),
                ("Unrelated application and project rules unchanged", str(result["unrelated_unchanged"]).lower()),
                ("Fixture file", "unchanged"),
            ]
        )
        + state_lines(state, console),
    )
    reinspection, scope = explain_lines(state, console, "shared.research", "Codex", project)
    show(
        "Reinspect applied state",
        default_context(state, "shared.research", "Codex", project, scope),
        reinspection,
    )
    for source_id, application, action in [
        ("plugin.docs", "Codex", "managed plugin disable"),
        ("shared.research", "CodeAF", "CodeAF persistent disable"),
    ]:
        try:
            state.disable_plan(source_id, application, "global", None)
            raise AssertionError("expected refusal")
        except OperationRefusal as error:
            show(
                f"Refuse {action}",
                default_context(state, source_id, application, project, "per-application"),
                console.wrapped(str(error)) + state_lines(state, console),
            )
    delete_plan = state.delete_plan("shared.research")
    show(
        "Preview shared-source delete",
        default_context(state, "shared.research", "Codex", project, "content deletion"),
        delete_preview_lines(state, console, delete_plan),
    )
    show(
        "Cancel permanent delete",
        default_context(state, "shared.research", "Codex", project, "content deletion"),
        ["Cancelled - no fixture state changed"] + state_lines(state, console),
    )
    delete_plan = state.delete_plan("copy.claude")
    deleted = state.apply_delete(delete_plan)
    show(
        "Confirm exact in-memory delete",
        default_context(state, "copy.claude", "Claude Code", project, "content deletion"),
        console.fields(
            [
                ("Typed confirmation", "DELETE copy.claude"),
                ("Deleted", deleted["deleted"]),
                ("Unrelated sources unchanged", str(deleted["unrelated_sources_unchanged"]).lower()),
                ("Fixture file", "unchanged"),
            ]
        )
        + state_lines(state, console),
    )
    return 0


def stripped_terminal_output(raw: bytes) -> str:
    text = raw.decode("utf-8", errors="replace").replace("\r\n", "\n").replace("\r", "")
    return ANSI_RE.sub("", text)


def bare_line_feed_count(raw: bytes) -> int:
    """Count output newlines that a real terminal would not return to column zero."""
    return sum(byte == 10 and (index == 0 or raw[index - 1] != 13) for index, byte in enumerate(raw))


def visible_terminal_frames(raw: bytes, rows: int = 30) -> list[str]:
    """Render the visible row tail after each browser clear sequence."""
    frames: list[str] = []
    for payload in raw.split(b"\x1b[2J\x1b[H")[1:]:
        cleaned = stripped_terminal_output(payload)
        lines = cleaned.splitlines()
        frames.append("\n".join(lines[-rows:]))
    return frames


def drain_fd(fd: int, duration: float) -> bytes:
    chunks: list[bytes] = []
    deadline = time.monotonic() + duration
    while time.monotonic() < deadline:
        ready, _, _ = select.select([fd], [], [], min(0.03, max(0.0, deadline - time.monotonic())))
        if not ready:
            continue
        try:
            chunk = os.read(fd, 65536)
        except OSError:
            break
        if not chunk:
            break
        chunks.append(chunk)
        deadline = time.monotonic() + 0.04
    return b"".join(chunks)


def run_pty_browser(command: list[str], env: dict[str, str], width: int) -> tuple[int, str, list[str], int]:
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, width, 0, 0))
    process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, env=env, close_fds=True)
    os.close(slave)
    raw = bytearray(drain_fd(master, 0.3))
    steps = [
        b"\x1b[B",
        b"\x1b[A",
        b"a",
        b"a",
        b"a",
        b"a",
        b"p",
        b"p",
        b"/",
        b"review\n",
        b"\r",
        b"\t",
        b"c",
        b"\r",
        b"\x1b",
        b"\x1b",
        b"/",
        b"research\n",
        b"\r",
        b"\t",
        b"d",
        b"\x1b",
        b"d",
        b"y",
        b"\x1b",
        b"/",
        b"review\n",
        b"j",
        b"a",
        b"\r",
        b"D",
        b"\r",
        b"n\n",
        b"D",
        b"\r",
        b"DELETE copy.codex\n",
        b"\x1b",
        b"q",
    ]
    for index, payload in enumerate(steps, start=1):
        try:
            os.write(master, payload)
        except OSError as error:
            raw.extend(f"\nPTY WRITE FAILED AT STEP {index}: {error}\n".encode())
            break
        raw.extend(drain_fd(master, 0.18))
    try:
        return_code = process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.terminate()
        process.wait(timeout=2)
        return_code = EXIT_RUNTIME
        raw.extend(b"\nPTY DRIVER TIMEOUT\n")
    raw.extend(drain_fd(master, 0.2))
    os.close(master)
    raw_bytes = bytes(raw)
    return (
        return_code,
        stripped_terminal_output(raw_bytes),
        visible_terminal_frames(raw_bytes),
        bare_line_feed_count(raw_bytes),
    )


def run_pty_dialog(command: list[str], env: dict[str, str], responses: Sequence[bytes]) -> tuple[int, str]:
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 80, 0, 0))
    process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, env=env, close_fds=True)
    os.close(slave)
    raw = bytearray(drain_fd(master, 0.3))
    for payload in responses:
        os.write(master, payload)
        raw.extend(drain_fd(master, 0.2))
    try:
        return_code = process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.terminate()
        process.wait(timeout=2)
        return_code = EXIT_RUNTIME
        raw.extend(b"\nPTY DIALOG TIMEOUT\n")
    raw.extend(drain_fd(master, 0.2))
    os.close(master)
    return return_code, stripped_terminal_output(bytes(raw))


def command_verify(_state: FixtureState, args: argparse.Namespace) -> int:
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    script = Path(__file__).resolve()
    fixture_source = Path(args.fixture)
    records: list[dict[str, Any]] = []
    with tempfile.TemporaryDirectory(prefix="ale-22-verify-") as temporary:
        run_root = Path(temporary)
        fixture = run_root / "fixture.json"
        shutil.copyfile(fixture_source, fixture)
        document = json.loads(fixture.read_text(encoding="utf-8"))
        project = document["fixture"]["default_project"]
        fixture_hash = hashlib.sha256(fixture.read_bytes()).hexdigest()
        env = {
            "HOME": str(run_root / "home"),
            "PATH": "/usr/bin:/bin:/usr/sbin:/sbin",
            "PYTHONDONTWRITEBYTECODE": "1",
            "PYTHONUTF8": "1",
            "TMPDIR": str(run_root / "tmp"),
            "XDG_CACHE_HOME": str(run_root / "xdg-cache"),
            "XDG_CONFIG_HOME": str(run_root / "xdg-config"),
            "XDG_DATA_HOME": str(run_root / "xdg-data"),
        }
        for directory in [env["HOME"], env["TMPDIR"], env["XDG_CACHE_HOME"], env["XDG_CONFIG_HOME"], env["XDG_DATA_HOME"]]:
            Path(directory).mkdir(parents=True, exist_ok=True)

        def capture(
            name: str,
            command_args: list[str],
            expected: int,
            tokens: Sequence[str],
            *,
            include_fixture: bool = True,
            first_line: str | None = None,
        ) -> str:
            command = [sys.executable, str(script)] + command_args
            if include_fixture:
                command += ["--fixture", str(fixture)]
            result = subprocess.run(command, text=True, capture_output=True, env=env, stdin=subprocess.DEVNULL)
            combined = result.stdout + result.stderr
            (output / f"{name}.txt").write_text(combined, encoding="utf-8")
            current_hash = hashlib.sha256(fixture.read_bytes()).hexdigest()
            missing = [token for token in tokens if token not in combined]
            actual_first_line = combined.splitlines()[0] if combined.splitlines() else ""
            passed = (
                result.returncode == expected
                and not missing
                and (first_line is None or actual_first_line == first_line)
                and current_hash == fixture_hash
            )
            records.append(
                {
                    "name": name,
                    "command": command,
                    "expected_exit": expected,
                    "actual_exit": result.returncode,
                    "missing_tokens": missing,
                    "expected_first_line": first_line,
                    "actual_first_line": actual_first_line,
                    "fixture_unchanged": current_hash == fixture_hash,
                    "passed": passed,
                }
            )
            if not passed:
                raise AssertionError(f"{name} failed; inspect {output / f'{name}.txt'}")
            return combined

        capture(
            "malformed-missing-fixture",
            ["list"],
            EXIT_USAGE,
            [BANNER, "usage: skillglass list", "--fixture"],
            include_fixture=False,
            first_line=BANNER,
        )
        capture("list-research", ["list", "--project", project, "--query", "research"], 0, [BANNER, "shared.research", "references 3"])
        capture("list-review", ["list", "--project", project, "--query", "review"], 0, ["copy.codex", "copy.claude", "DUPLICATE NAME"])
        capture("read-copy", ["read", "copy.codex"], 0, ["Instructions", "references/checklist.md", "native-unverified"])
        capture("explain-project", ["explain", "shared.research", "--agent", "Codex", "--project", project], 0, ["explicit-only", "project override", "not disabled", "Native agent enforcement: not validated"])
        capture("compare-copies", ["compare", "copy.codex", "copy.claude"], 0, ["Instruction diff", "CHANGED references/checklist.md", "ADDED references/security.md", "REMOVED references/codex-output.md"])
        capture("disable-preview", ["disable", "shared.research", "--agent", "Codex", "--scope", "project", "--project", project, "--preview"], 0, ["DISABLE PREVIEW", "content retained", "Preview only"])
        capture("disable-missing-apply", ["disable", "shared.research", "--agent", "Codex", "--scope", "project", "--project", project], EXIT_USAGE, ["Noninteractive disable requires --apply", "no fixture state changed"])
        capture("disable-apply", ["disable", "shared.research", "--agent", "Codex", "--scope", "project", "--project", project, "--apply"], 0, ["DISABLE RESULT", "After: disabled", "Unrelated application and project rules unchanged: true", "Fixture file: unchanged"])
        capture("plugin-disable-refusal", ["disable", "plugin.docs", "--agent", "Codex", "--scope", "global", "--apply"], EXIT_REFUSED, ["REFUSED", "whole-plugin management is broader", "FULL RELEVANT IN-MEMORY STATE"])
        capture("plugin-delete-refusal", ["delete", "plugin.docs", "--confirm-delete", "plugin.docs"], EXIT_REFUSED, ["REFUSED", "managed by openai-bundled", "no managed cache is changed"])
        capture("codeaf-refusal", ["disable", "shared.research", "--agent", "CodeAF", "--scope", "global", "--apply"], EXIT_REFUSED, ["UNSUPPORTED", "Conversation detachment", "not evidence"])
        capture("delete-missing-confirmation", ["delete", "copy.claude"], EXIT_USAGE, ["Missing target-specific confirmation", "--confirm-delete copy.claude"])
        capture("delete-mismatch", ["delete", "copy.claude", "--confirm-delete", "copy.codex"], EXIT_USAGE, ["Confirmation mismatch", "no fixture state changed"])
        capture("delete-exact", ["delete", "copy.claude", "--confirm-delete", "copy.claude"], 0, ["DELETE RESULT", "Deleted copy.claude permanently", "copy.codex", "Unrelated sources unchanged: true"])
        capture("name-selection-refusal", ["delete", "review", "--confirm-delete", "review"], EXIT_USAGE, ["display name, not a source ID", "copy.codex", "copy.claude"])
        capture("guided-walkthrough", ["walkthrough", "--project", project, "--no-pause"], 0, ["GUIDED STEP", "Reinspect applied state", "CodeAF", "DELETE copy.claude"])

        def capture_dialog(
            name: str,
            command_args: list[str],
            responses: Sequence[bytes],
            expected: int,
            tokens: Sequence[str],
        ) -> None:
            command = [sys.executable, str(script)] + command_args + ["--fixture", str(fixture)]
            return_code, text = run_pty_dialog(command, env, responses)
            (output / f"{name}.txt").write_text(text, encoding="utf-8")
            current_hash = hashlib.sha256(fixture.read_bytes()).hexdigest()
            missing = [token for token in tokens if token not in text]
            passed = return_code == expected and not missing and current_hash == fixture_hash
            records.append(
                {
                    "name": name,
                    "command": command,
                    "expected_exit": expected,
                    "actual_exit": return_code,
                    "missing_tokens": missing,
                    "fixture_unchanged": current_hash == fixture_hash,
                    "passed": passed,
                }
            )
            if not passed:
                raise AssertionError(f"{name} failed; inspect {output / f'{name}.txt'}")

        capture_dialog(
            "disable-tty-cancel",
            ["disable", "shared.research", "--agent", "Codex", "--scope", "project", "--project", project],
            [b"n\n"],
            0,
            ["Apply this disable? [y/N]", "Cancelled - no fixture state changed"],
        )
        capture_dialog(
            "delete-tty-cancel",
            ["delete", "copy.claude"],
            [b"\n"],
            0,
            ["DELETE copy.claude", "Cancelled - no fixture state changed"],
        )
        capture_dialog(
            "delete-tty-confirm",
            ["delete", "copy.claude"],
            [b"DELETE copy.claude\n"],
            0,
            ["DELETE RESULT", "Deleted copy.claude permanently", "Fixture file: unchanged"],
        )

        for width in [48, 60, 80, 96, 120]:
            command = [
                sys.executable,
                str(script),
                "browse",
                "--fixture",
                str(fixture),
                "--project",
                project,
                "--agent",
                "Codex",
            ]
            return_code, text, viewports, bare_line_feeds = run_pty_browser(command, env, width)
            capture_path = output / f"browser-{width}x30.txt"
            capture_path.write_text(text, encoding="utf-8")
            viewport_path = output / f"browser-{width}x30-viewports.txt"
            viewport_path.write_text(
                "\n\n".join(f"VISIBLE FRAME {index + 1}\n{frame}" for index, frame in enumerate(viewports)) + "\n",
                encoding="utf-8",
            )
            required = [BANNER, "copy.codex", "copy.claude", "BUNDLE COMPARISON", "explicit-only", "DISABLE PREVIEW", "Cancelled - no fixture state changed", "APPLIED to in-memory fixture state", "DELETE PREVIEW", "DELETE copy.claude", "Confirmation did not match"]
            missing = [token for token in required if token not in text]
            overflow = [line for line in text.splitlines() if len(line) > width]
            viewport_missing_context = [
                index + 1
                for index, frame in enumerate(viewports)
                if not all(token in frame for token in ["DATA FIXTURE", "APP ", "SCOPE ", "SOURCE ", "VALIDATION unverified"])
            ]
            status_minimums = {
                "Cancelled - no fixture state changed": 2,
                "APPLIED to in-memory fixture state": 1,
                "Confirmation did not match": 1,
            }
            visible_status_counts = {
                token: sum(token in frame for frame in viewports) for token in status_minimums
            }
            missing_visible_statuses = [
                token for token, minimum in status_minimums.items() if visible_status_counts[token] < minimum
            ]
            initial_viewport = viewports[0] if viewports else ""
            intact_duplicate_markers = initial_viewport.count("[duplicate name]")
            fragmented_duplicate_marker_rows = sum(
                (line.lstrip().startswith("[duplicate") and "[duplicate name]" not in line)
                or line.lstrip().startswith("name]")
                for line in initial_viewport.splitlines()
            )
            wide_marker_failure = width >= 96 and (
                intact_duplicate_markers != 2 or fragmented_duplicate_marker_rows != 0
            )
            current_hash = hashlib.sha256(fixture.read_bytes()).hexdigest()
            passed = (
                return_code == 0
                and not missing
                and not overflow
                and not viewport_missing_context
                and not missing_visible_statuses
                and not wide_marker_failure
                and bare_line_feeds == 0
                and current_hash == fixture_hash
            )
            records.append(
                {
                    "name": f"browser-{width}x30",
                    "command": command,
                    "expected_exit": 0,
                    "actual_exit": return_code,
                    "missing_tokens": missing,
                    "overflow_line_count": len(overflow),
                    "visible_frame_count": len(viewports),
                    "visible_frames_missing_context": viewport_missing_context,
                    "visible_status_counts": visible_status_counts,
                    "missing_visible_statuses": missing_visible_statuses,
                    "intact_duplicate_markers_in_initial_viewport": intact_duplicate_markers,
                    "fragmented_duplicate_marker_rows_in_initial_viewport": fragmented_duplicate_marker_rows,
                    "bare_line_feed_count": bare_line_feeds,
                    "fixture_unchanged": current_hash == fixture_hash,
                    "passed": passed,
                }
            )
            if not passed:
                raise AssertionError(f"PTY width {width} failed; inspect {capture_path}")

    summary = {
        "fixture_sha256": fixture_hash,
        "script": str(script),
        "fixture": str(fixture_source.resolve()),
        "output": str(output.resolve()),
        "checks": records,
        "passed": all(record["passed"] for record in records),
        "boundary": "Fixture-only simulation; no native agent behavior validated",
    }
    (output / "verification.json").write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(BANNER)
    print(f"Verification checks passed: {len(records)}")
    print(f"Fixture SHA-256 unchanged: {fixture_hash}")
    print(f"Evidence directory: {output.resolve()}")
    print("Native agent behavior validated: no")
    return 0


def add_fixture_argument(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--fixture", required=True, help="explicit inert JSON fixture")


def build_parser() -> argparse.ArgumentParser:
    parser = FixtureArgumentParser(prog="skillglass", description="ALE-22 fixture-only terminal prototype")
    subparsers = parser.add_subparsers(dest="command", required=True)

    browse = subparsers.add_parser("browse", help="open the interactive fixture browser")
    add_fixture_argument(browse)
    browse.add_argument("--project")
    browse.add_argument("--agent")
    browse.set_defaults(handler=command_browse)

    list_parser = subparsers.add_parser("list", help="list fixture sources")
    add_fixture_argument(list_parser)
    list_parser.add_argument("--project")
    list_parser.add_argument("--query")
    list_parser.set_defaults(handler=command_list)

    read = subparsers.add_parser("read", help="read one source bundle")
    read.add_argument("source_id")
    add_fixture_argument(read)
    read.add_argument("--project")
    read.set_defaults(handler=command_read)

    explain = subparsers.add_parser("explain", help="explain contextual fixture availability")
    explain.add_argument("source_id")
    add_fixture_argument(explain)
    explain.add_argument("--agent", required=True)
    explain.add_argument("--project")
    explain.set_defaults(handler=command_explain)

    compare_parser = subparsers.add_parser("compare", help="compare two source bundles")
    compare_parser.add_argument("left_id")
    compare_parser.add_argument("right_id")
    add_fixture_argument(compare_parser)
    compare_parser.add_argument("--project")
    compare_parser.set_defaults(handler=command_compare)

    disable = subparsers.add_parser("disable", help="preview or apply an in-memory disable")
    disable.add_argument("source_id")
    add_fixture_argument(disable)
    disable.add_argument("--agent", required=True)
    disable.add_argument("--scope", choices=["global", "project"], required=True)
    disable.add_argument("--project")
    mode = disable.add_mutually_exclusive_group()
    mode.add_argument("--apply", action="store_true")
    mode.add_argument("--preview", action="store_true")
    disable.set_defaults(handler=command_disable)

    delete = subparsers.add_parser("delete", help="preview or confirm an in-memory permanent delete")
    delete.add_argument("source_id")
    add_fixture_argument(delete)
    delete.add_argument("--project")
    delete.add_argument("--confirm-delete")
    delete.set_defaults(handler=command_delete)

    walkthrough = subparsers.add_parser("walkthrough", help="run the guided fixture journey")
    add_fixture_argument(walkthrough)
    walkthrough.add_argument("--project")
    walkthrough.add_argument("--no-pause", action="store_true")
    walkthrough.set_defaults(handler=command_walkthrough)

    verify = subparsers.add_parser("verify", help="run semantic commands and real-PTY captures")
    add_fixture_argument(verify)
    verify.add_argument("--output", required=True)
    verify.set_defaults(handler=command_verify)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        state = FixtureState.load(args.fixture)
        return int(args.handler(state, args))
    except PrototypeError as error:
        print(BANNER)
        print(f"ERROR: {error}")
        return error.exit_code
    except KeyboardInterrupt:
        print("\nCancelled - no fixture state changed")
        return EXIT_INTERRUPTED


if __name__ == "__main__":
    raise SystemExit(main())
