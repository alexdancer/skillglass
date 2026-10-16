#!/usr/bin/env python3
"""Rerun the pinned OpenCode 1.17.15 control evidence in fresh contained fixtures.

The config probe observes native resolved rules. The tool probe exercises native
permission rejection only: the non-denied branch reaches the contained ripgrep
blocker. It does not establish full loading, approval prompting or session reload.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--opencode', required=True)
    parser.add_argument('--out', required=True)
    parser.add_argument('--git', default='/Library/Developer/CommandLineTools/usr/bin/git')
    args = parser.parse_args()
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=False)
    tools = Path(__file__).resolve().parent
    checks = []
    try:
        for name, script in [('config', 'opencode_controls_probe.py'),
                             ('evaluator', 'opencode_tool_probe.py')]:
            argv = [sys.executable, str(tools / script), '--opencode', args.opencode,
                    '--git', args.git, '--out', str(out / name)]
            with (out / (name + '.log')).open('w') as log:
                result = subprocess.run(argv, stdout=log, stderr=subprocess.STDOUT)
            receipts_path = out / name / 'receipts.json'
            receipts = json.loads(receipts_path.read_text()) if receipts_path.exists() else {}
            passed = (result.returncode == 0 and receipts.get('version') == '1.17.15'
                      and bool(receipts.get('checks'))
                      and all(row['passed'] for row in receipts['checks']))
            checks.append({'name': name, 'argv': argv, 'exit_code': result.returncode,
                           'passed': passed, 'receipts': str(receipts_path),
                           'checks': len(receipts.get('checks', []))})
            print(name, 'PASS' if passed else 'FAIL', flush=True)
            if not passed:
                raise SystemExit(1)
    finally:
        sources = {name: hashlib.sha256((tools / name).read_bytes()).hexdigest()
                   for name in ['verify_opencode_controls.py', 'opencode_controls_probe.py',
                                'opencode_tool_probe.py', 'native_containment.py',
                                'native_receipts.py']}
        (out / 'result.json').write_text(json.dumps({
            'checks': checks, 'passed': len(checks) == 2 and all(x['passed'] for x in checks),
            'source_hashes': sources,
            'limitations': ['No model turn or full skill load.',
                            'Debug ask bypasses prompting.',
                            'Running session adoption and reload/restart are unverified.']}, indent=2) + '\n')


if __name__ == '__main__':
    main()
