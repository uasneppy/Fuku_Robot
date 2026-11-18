"""Proof that the one re-recording of the TestLegacy* goldens (owner decision D6, 2026-11-18) changed text only.

Usage: python3 golden_text_only.py <base commit>

For every golden literal in fuku/modules/*_legacy_request_test.go it pairs the literal at the base commit with the
literal at the same place in the working tree (by constant name or map key), decodes each Telegram request and
compares them with the message text, the caption and the button labels blanked. parse_mode may change only from
Markdown to HTML (the plan's parse-mode switch). Anything else that differs fails the run.
"""
import json, re, subprocess, sys

base = sys.argv[1]
root = subprocess.check_output(['git', 'rev-parse', '--show-toplevel'], text=True).strip()
files = subprocess.check_output(['git', 'ls-files', 'fuku/modules/*_legacy_request_test.go'], text=True, cwd=root).split()
lit = r'"((?:[^"\\]|\\.)*)"'
entry = re.compile(r'^\s*(golden\w+|"[\w]+")\s*[:=]\s*' + lit + r',?\s*$', re.M)


def unquote(s):
    return json.loads('"' + s + '"')


def literals(src):
    return {m.group(1): unquote(m.group(2)) for m in entry.finditer(src)}


def blank(req):
    req = json.loads(req.replace('<chat>', '0'))
    pm = req.pop('parse_mode', None)
    for k in ('text', 'caption'):
        if k in req:
            req[k] = '<TEXT>'
    kb = req.get('reply_markup')
    if isinstance(kb, str):
        kb = json.loads(kb)
    if isinstance(kb, dict):
        for row in kb.get('inline_keyboard', []):
            for btn in row:
                btn['text'] = '<LABEL>'
        req['reply_markup'] = kb
    return req, pm


failures, changed, total = [], 0, 0
for f in files:
    old = literals(subprocess.check_output(['git', 'show', f'{base}:{f}'], text=True, cwd=root))
    new = literals(open(f'{root}/{f}', encoding='utf-8').read())
    if set(old) != set(new):
        failures.append(f'{f}: the set of goldens changed: {sorted(set(old) ^ set(new))}')
    for name in sorted(set(old) & set(new)):
        total += 1
        if old[name] == new[name]:
            continue
        changed += 1
        a, b = old[name].split('\n'), new[name].split('\n')
        if len(a) != len(b):
            failures.append(f'{f} {name}: {len(a)} requests became {len(b)}')
            continue
        for ra, rb in zip(a, b):
            (qa, pa), (qb, pb) = blank(ra), blank(rb)
            if qa != qb:
                failures.append(f'{f} {name}: more than text changed:\n  {qa}\n  {qb}')
            if pa != pb and not (pa == 'Markdown' and pb == 'HTML'):
                failures.append(f'{f} {name}: parse_mode {pa} -> {pb}')
print(f'{total} goldens compared, {changed} re-recorded, {len(failures)} differences beyond text')
for x in failures:
    print(x)
sys.exit(1 if failures else 0)
