"""Disposable native-child policy. The parent owns evidence outside the fixture."""
from pathlib import Path
import json


def runtime_data_paths():
    source = Path('/var/db/timezone/icutz')
    try:
        target = source.resolve(strict=True)
    except FileNotFoundError:
        return []
    if not target.is_dir():
        return []
    return [{'source': str(source), 'canonical': str(target)}]


def native_profile(fixture, executable, protected=(), runtime_data=None):
    fixture, executable = Path(fixture).resolve(strict=True), Path(executable).resolve(strict=True)
    quote = json.dumps
    read = [f'(subpath {quote(str(fixture))})', f'(literal {quote(str(executable))})',
            '(literal "/")', '(subpath "/System")', '(subpath "/usr/lib")', '(subpath "/usr/bin")',
            '(subpath "/bin")', '(subpath "/usr/share/icu")', '(subpath "/dev")']
    if runtime_data is None:
        runtime_data = runtime_data_paths()
    read += ['(subpath '+quote(item['canonical'])+')' for item in runtime_data]
    excluded = ['/Users', '/Library/Application Support/opencode',
                '/Library/Managed Preferences', '/private/etc/codex', '/etc/codex']
    excluded += [str(Path(p).resolve()) for p in protected]
    return ('(version 1)\n(allow default)\n(deny network*)\n'
            '(deny file-write* (require-not (require-any (subpath '+quote(str(fixture))+') (literal "/dev/null"))))\n'
            '(deny file-read-data (require-not (require-any '+ ' '.join(read)+')))\n'
            + ''.join('(deny file-read* (subpath '+quote(p)+') (with errno 2))\n' for p in excluded))
