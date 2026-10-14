#!/usr/bin/env python3
"""Offline Codex 0.155.1 probes using a fully disposable home and project."""
import json
from codex_probe import Probe, override

probe = Probe(__doc__)
OUT, BASE = probe.out, probe.base
FIXTURE_HOME, CH = probe.home, probe.codex_home
REPO, CWD = probe.repo, probe.cwd
RESULTS = probe.results
skill, config, case, Server = probe.skill, probe.config, probe.case, probe.server

r = FIXTURE_HOME / '.agents/skills'
skill(r, 'valid')
skill(r, 'group/nested')
skill(r, '.hidden/hidden-skill')
missing_name = skill(r, 'missing-name')
missing_name.write_text('---\ndescription: Description only.\n---\nBody.\n')
missing_description = skill(r, 'missing-description')
missing_description.write_text('---\nname: missing-description\n---\nBody.\n')
malformed = skill(r, 'malformed')
malformed.write_text('---\nname: [\ndescription: malformed\n---\nBody.\n')
source = skill(BASE/'external', 'manifest-target', 'linked-manifest')
(r/'manifest-link').mkdir()
(r/'manifest-link/SKILL.md').symlink_to(source)
result = case('manifest-eligibility')
OUT.joinpath('result.json').write_text(json.dumps(RESULTS,indent=2)+'\n')
summary = [{'name':s['name'],'path':s['path'],'enabled':s['enabled']} for d in result.get('result',{}).get('data',[]) for s in d['skills'] if '/.system/' not in s['path']]
errors = [e for d in result.get('result',{}).get('data',[]) for e in d['errors']]
OUT.joinpath('summary.json').write_text(json.dumps({'version':RESULTS['version'],'skills':summary,'errors':errors},indent=2)+'\n')
