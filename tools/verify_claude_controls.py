#!/usr/bin/env python3
"""Validate Claude Code 2.1.282/2.1.300 ordinary skill controls in isolation.

Usage: verify_claude_controls.py OUTPUT --claude /absolute/path/to/claude
Only SDK initialization is sent. No model requests or skill workloads run.
"""
from verify_claude import main

if __name__ == "__main__":
    main(controls=True)
