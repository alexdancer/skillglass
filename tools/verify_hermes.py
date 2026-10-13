#!/usr/bin/env python3
"""Retired uncontained native startup validator; historical receipts are preserved."""
import sys

sys.stderr.write(
    "verify_hermes.py is retired: uncontained native startup validation is disabled. "
    "Use tools/verify_hermes_policy.py for bounded environment/config helper validation only. "
    "Full startup/catalog/plugins/sessions/writers/reload remain unverified.\n"
)
raise SystemExit(2)
