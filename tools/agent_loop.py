#!/usr/bin/env python3
"""Durable local scheduling for a native agent coordinator."""

import argparse
import hashlib
import json
import re
import secrets
import sqlite3
import sys
from pathlib import Path


class Invalid(ValueError):
    pass


def require(condition, message):
    if not condition:
        raise Invalid(message)


def exact(value, fields, label):
    require(isinstance(value, dict), f"{label} must be an object")
    require(set(value) == set(fields.split()), f"{label} has missing or unknown fields")


def nonempty(value, label):
    require(isinstance(value, str) and bool(value.strip()), f"{label} must be a nonempty string")


def integer(value, label, minimum=None):
    require(type(value) is int, f"{label} must be an integer")
    if minimum is not None:
        require(value >= minimum, f"{label} must be at least {minimum}")


def sha(value, label):
    require(isinstance(value, str) and re.fullmatch(r"[0-9a-f]{40}", value),
            f"{label} must be a 40-character lowercase hexadecimal SHA")


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f"duplicate JSON field: {key}")
        result[key] = value
    return result


def invalid_constant(value):
    raise Invalid(f"invalid JSON constant: {value}")


def decode(raw):
    return json.loads(raw, object_pairs_hook=unique_object, parse_constant=invalid_constant)


def normalized(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def read_document(filename, limit=None):
    path = Path(filename).resolve()
    with path.open("rb") as source:
        raw = source.read(limit + 1 if limit is not None else -1)
    require(limit is None or len(raw) <= limit, "evidence exceeds 1 MiB")
    return raw, decode(raw)


def validate_plan(plan):
    exact(plan, "version run_id intent base_sha capacity jobs", "plan")
    integer(plan["version"], "version")
    require(plan["version"] == 1, "unsupported plan version")
    for key in ("run_id", "intent"):
        nonempty(plan[key], key)
    sha(plan["base_sha"], "base_sha")
    integer(plan["capacity"], "capacity", 1)
    require(isinstance(plan["jobs"], list), "jobs must be an array")
    jobs = {}
    for job in plan["jobs"]:
        exact(job, "id ticket role depends_on slots resources instructions context", "job")
        for key in ("id", "ticket", "role", "instructions"):
            nonempty(job[key], f"job {key}")
        require(job["id"] not in jobs, "job IDs must be unique")
        require(job["role"] in {"work", "review", "decision", "gate", "accept"}, "unknown role")
        integer(job["slots"], "slots", 1)
        require(job["slots"] <= plan["capacity"], "job slots exceed capacity")
        require(isinstance(job["resources"], dict) and job["resources"], "resources must be a nonempty object")
        for resource, mode in job["resources"].items():
            nonempty(resource, "resource key")
            require(isinstance(mode, str) and mode in {"read", "write"}, "unknown resource mode")
        for key in ("depends_on", "context"):
            require(isinstance(job[key], list), f"{key} must be an array")
            for item in job[key]:
                nonempty(item, key)
        require(len(job["depends_on"]) == len(set(job["depends_on"])), "duplicate dependencies")
        jobs[job["id"]] = job
    for job in jobs.values():
        for dependency in job["depends_on"]:
            require(dependency in jobs, f"unknown dependency: {dependency}")
            parent = jobs[dependency]
            require(parent["ticket"] == job["ticket"] or parent["role"] == "accept",
                    "cross-ticket dependencies must point to accept jobs")
        if job["role"] == "accept":
            require(any(jobs[d]["ticket"] == job["ticket"] for d in job["depends_on"]),
                    "accept jobs require a same-ticket dependency")
    remaining = set(jobs)
    while remaining:
        ready = {key for key in remaining if not (set(jobs[key]["depends_on"]) & remaining)}
        require(ready, "dependency graph contains a cycle")
        remaining -= ready
    return plan


def validate_evidence(document, job, token, attempt, base):
    exact(document, "job token attempt base_sha head_sha verdict summary checks", "evidence")
    for key in ("job", "token", "verdict", "summary"):
        nonempty(document[key], f"evidence {key}")
    integer(document["attempt"], "attempt", 1)
    require((document["job"], document["token"], document["attempt"]) == (job, token, attempt),
            "evidence does not match the active assignment")
    sha(document["base_sha"], "base_sha")
    sha(document["head_sha"], "head_sha")
    require(document["base_sha"] == base, "evidence base_sha differs from the pinned run base")
    require(document["verdict"] in {"passed", "blocked"}, "unknown evidence verdict")
    require(isinstance(document["checks"], list), "checks must be an array")
    for check in document["checks"]:
        exact(check, "command exit_code evidence", "check")
        nonempty(check["command"], "check command")
        nonempty(check["evidence"], "check evidence")
        integer(check["exit_code"], "check exit_code")
    if document["verdict"] == "passed":
        require(document["checks"] and all(c["exit_code"] == 0 for c in document["checks"]),
                "passed evidence requires at least one check and every check must pass")


SCHEMA = (
    "CREATE TABLE IF NOT EXISTS run (id INTEGER PRIMARY KEY CHECK(id=1), plan TEXT NOT NULL, authorization TEXT NOT NULL)",
    "CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, position INTEGER NOT NULL, state TEXT NOT NULL, attempt INTEGER NOT NULL DEFAULT 0, reason TEXT)",
    "CREATE TABLE IF NOT EXISTS attempts (job TEXT NOT NULL REFERENCES jobs(id), attempt INTEGER NOT NULL, token TEXT NOT NULL UNIQUE, owner TEXT NOT NULL, agent TEXT, state TEXT NOT NULL, evidence BLOB, digest TEXT, PRIMARY KEY(job,attempt))",
    "CREATE UNIQUE INDEX IF NOT EXISTS active_owner ON attempts(owner) WHERE state='active'",
    "CREATE UNIQUE INDEX IF NOT EXISTS active_agent ON attempts(agent) WHERE state='active' AND agent IS NOT NULL",
    "CREATE TABLE IF NOT EXISTS events (seq INTEGER PRIMARY KEY AUTOINCREMENT, created TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')), action TEXT NOT NULL, job TEXT, attempt INTEGER, detail TEXT NOT NULL)",
)


def connect(path, create=False):
    target = Path(path).resolve()
    connection = sqlite3.connect(target.as_uri() + ("?mode=rwc" if create else "?mode=rw"),
                                 uri=True, timeout=5, isolation_level=None)
    connection.row_factory = sqlite3.Row
    connection.execute("PRAGMA busy_timeout=5000")
    connection.execute("PRAGMA foreign_keys=ON")
    return connection


def event(db, action, job=None, attempt=None, **detail):
    db.execute("INSERT INTO events(action,job,attempt,detail) VALUES (?,?,?,?)",
               (action, job, attempt, normalized(detail)))


def load_run(db):
    row = db.execute("SELECT plan,authorization FROM run WHERE id=1").fetchone()
    require(row is not None, "database is not initialized")
    return decode(row["plan"]), decode(row["authorization"])


def assignment(plan, row):
    job = next(job for job in plan["jobs"] if job["id"] == row["job"])
    return {"job": row["job"], "ticket": job["ticket"], "role": job["role"],
            "owner": row["owner"], "token": row["token"], "attempt": row["attempt"],
            "agent": row["agent"], "slots": job["slots"], "resources": job["resources"],
            "instructions": job["instructions"], "context": job["context"]}


def snapshot(db, plan, authorization):
    records = {row["id"]: dict(row) for row in db.execute("SELECT * FROM jobs ORDER BY position")}
    active = [assignment(plan, row) for row in db.execute(
        "SELECT a.* FROM attempts a JOIN jobs j ON j.id=a.job AND j.attempt=a.attempt WHERE a.state='active' ORDER BY j.position")]
    used = sum(item["slots"] for item in active)
    ready, dependency_waits, resource_waits, authorization_waits = [], [], [], []
    for job in plan["jobs"]:
        if records[job["id"]]["state"] != "pending":
            continue
        waiting = [key for key in job["depends_on"] if records[key]["state"] != "passed"]
        if waiting:
            dependency_waits.append({"job": job["id"], "depends_on": waiting})
            continue
        if job["role"] == "gate" and not authorization["local_commits"]:
            authorization_waits.append(job["id"])
            continue
        conflicts = [item["job"] for item in active if any(
            resource in item["resources"] and (mode == "write" or item["resources"][resource] == "write")
            for resource, mode in job["resources"].items())]
        if conflicts or used + job["slots"] > plan["capacity"]:
            resource_waits.append({"job": job["id"], "conflicts": conflicts,
                                   "capacity_wait": used + job["slots"] > plan["capacity"]})
            continue
        ready.append(job["id"])
    return {"policy": {"run_id": plan["run_id"], "intent": plan["intent"], "base_sha": plan["base_sha"],
                       "capacity": plan["capacity"], "authorization": authorization},
            "assignments": active, "ready_jobs": ready,
            "blocked_jobs": [{"job": row["id"], "reason": row["reason"]} for row in records.values() if row["state"] == "blocked"],
            "dependency_waits": dependency_waits, "resource_waits": resource_waits,
            "authorization_waits": authorization_waits,
            "capacity": {"used": used, "total": plan["capacity"], "available": plan["capacity"] - used},
            "jobs": [{"job": row["id"], "state": row["state"], "attempt": row["attempt"]} for row in records.values()]}


def lease(db, job, token, active=True):
    row = db.execute("SELECT a.*, j.state AS job_state FROM attempts a JOIN jobs j ON j.id=a.job AND j.attempt=a.attempt WHERE a.job=? AND a.token=?",
                     (job, token)).fetchone()
    require(row is not None, "stale or unknown lease")
    require(row["job_state"] == row["state"], "stale lease")
    if active:
        require(row["state"] == "active", "lease is no longer active")
    return row


def initialize(args):
    _, plan = read_document(args.plan)
    validate_plan(plan)
    authorization = {"local_commits": args.allow_local_commits, "merge": False,
                     "push": False, "pr_publication": False, "ticket_closure": False}
    with connect(args.db, create=True) as db:
        db.execute("BEGIN IMMEDIATE")
        for statement in SCHEMA:
            db.execute(statement)
        existing = db.execute("SELECT plan,authorization FROM run WHERE id=1").fetchone()
        noop = existing is not None
        if existing:
            require(existing["plan"] == normalized(plan) and existing["authorization"] == normalized(authorization),
                    "database already contains a different plan or authorization")
        else:
            db.execute("INSERT INTO run(id,plan,authorization) VALUES (1,?,?)",
                       (normalized(plan), normalized(authorization)))
            db.executemany("INSERT INTO jobs(id,position,state) VALUES (?,?,'pending')",
                           [(job["id"], index) for index, job in enumerate(plan["jobs"])])
            event(db, "init", authorization=authorization)
        result = {"noop": noop, "status": snapshot(db, plan, authorization)}
        db.commit()
        return result


def command(args):
    if args.command == "init":
        return initialize(args)
    raw, document = read_document(args.evidence, 1024 * 1024) if args.command == "finish" else (None, None)
    with connect(args.db) as db:
        db.execute("BEGIN" if args.command in {"status", "evidence"} else "BEGIN IMMEDIATE")
        plan, authorization = load_run(db)
        result = {}
        if args.command == "status":
            result["status"] = snapshot(db, plan, authorization)
        elif args.command == "claim":
            nonempty(args.owner, "owner")
            state = snapshot(db, plan, authorization)
            current = next((item for item in state["assignments"] if item["owner"] == args.owner), None)
            if current is None and state["ready_jobs"]:
                job = state["ready_jobs"][0]
                db.execute("UPDATE jobs SET state='active', attempt=attempt+1, reason=NULL WHERE id=?", (job,))
                attempt = db.execute("SELECT attempt FROM jobs WHERE id=?", (job,)).fetchone()[0]
                token = secrets.token_hex(24)
                db.execute("INSERT INTO attempts(job,attempt,token,owner,state) VALUES (?,?,?,?,'active')",
                           (job, attempt, token, args.owner))
                event(db, "claim", job, attempt, owner=args.owner, token=token)
                current = assignment(plan, lease(db, job, token))
            result = {"assignment": current, "status": snapshot(db, plan, authorization)}
        elif args.command == "bind":
            nonempty(args.agent, "agent")
            row = lease(db, args.job, args.token)
            require(row["agent"] is None or row["agent"] == args.agent, "agent is already bound")
            result["noop"] = row["agent"] == args.agent
            if row["agent"] is None:
                db.execute("UPDATE attempts SET agent=? WHERE job=? AND attempt=?", (args.agent, args.job, row["attempt"]))
                event(db, "bind", args.job, row["attempt"], agent=args.agent)
            result["assignment"] = assignment(plan, lease(db, args.job, args.token))
        elif args.command == "finish":
            row = lease(db, args.job, args.token, active=False)
            validate_evidence(document, args.job, args.token, row["attempt"], plan["base_sha"])
            digest = hashlib.sha256(raw).hexdigest()
            result["noop"] = row["evidence"] is not None
            if row["evidence"] is not None:
                require(row["evidence"] == raw, "accepted evidence cannot be changed")
            else:
                require(row["state"] == "active", "lease is no longer active")
                verdict = document["verdict"]
                db.execute("UPDATE attempts SET state=?, evidence=?, digest=? WHERE job=? AND attempt=?",
                           (verdict, raw, digest, args.job, row["attempt"]))
                db.execute("UPDATE jobs SET state=?,reason=? WHERE id=?", (verdict, document["summary"], args.job))
                event(db, "finish", args.job, row["attempt"], verdict=verdict, sha256=digest)
            result.update({"sha256": digest, "status": snapshot(db, plan, authorization)})
        elif args.command == "recover":
            require(args.owner_stopped, "recovery requires --owner-stopped after ownership inspection")
            nonempty(args.reason, "reason")
            row = lease(db, args.job, args.token)
            db.execute("UPDATE attempts SET state='blocked' WHERE job=? AND attempt=?", (args.job, row["attempt"]))
            db.execute("UPDATE jobs SET state='blocked',reason=? WHERE id=?", (args.reason, args.job))
            event(db, "recover", args.job, row["attempt"], reason=args.reason, owner_stopped=True)
            result["status"] = snapshot(db, plan, authorization)
        elif args.command == "retry":
            row = db.execute("SELECT * FROM jobs WHERE id=?", (args.job,)).fetchone()
            require(row is not None and row["state"] == "blocked", "only blocked jobs can be retried")
            db.execute("UPDATE jobs SET state='pending',reason=NULL WHERE id=?", (args.job,))
            event(db, "retry", args.job, row["attempt"])
            result["status"] = snapshot(db, plan, authorization)
        elif args.command == "evidence":
            row = db.execute("SELECT evidence,digest,attempt FROM attempts WHERE job=? AND evidence IS NOT NULL ORDER BY attempt DESC LIMIT 1", (args.job,)).fetchone()
            require(row is not None, "no accepted evidence for job")
            result = {"evidence": decode(row["evidence"]), "sha256": row["digest"], "attempt": row["attempt"]}
        db.commit()
        return result


class Parser(argparse.ArgumentParser):
    def error(self, message):
        raise Invalid(message)


def arguments():
    parser = Parser(description=__doc__)
    parser.add_argument("--db", required=True)
    commands = parser.add_subparsers(dest="command", required=True, parser_class=Parser)
    init = commands.add_parser("init")
    init.add_argument("--plan", required=True)
    init.add_argument("--allow-local-commits", action="store_true")
    claim = commands.add_parser("claim")
    claim.add_argument("--owner", required=True)
    bind = commands.add_parser("bind")
    bind.add_argument("--job", required=True)
    bind.add_argument("--token", required=True)
    bind.add_argument("--agent", required=True)
    finish = commands.add_parser("finish")
    finish.add_argument("--job", required=True)
    finish.add_argument("--token", required=True)
    finish.add_argument("--evidence", required=True)
    recover = commands.add_parser("recover")
    recover.add_argument("--job", required=True)
    recover.add_argument("--token", required=True)
    recover.add_argument("--owner-stopped", action="store_true")
    recover.add_argument("--reason", required=True)
    for name in ("retry", "evidence"):
        commands.add_parser(name).add_argument("--job", required=True)
    commands.add_parser("status")
    return parser.parse_args()


def main():
    try:
        result = command(arguments())
    except (Invalid, OSError, sqlite3.Error, ValueError, UnicodeError) as exc:
        print(json.dumps({"error": str(exc)}, ensure_ascii=False))
        return 2
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
