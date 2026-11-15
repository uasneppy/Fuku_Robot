#!/usr/bin/env python3
"""Checker for quick task 261115-rcd (untracked, never committed).

Modes:
  locales            key parity, forbidden patterns, placeholder and tag parity against the base commit
  prose PATH...      old-name prose forms and original-developer hits in .md/.mdx/.astro files
  counts PATH...     statements of module/command/language counts in docs
"""
import collections
import os
import re
import subprocess
import sys

try:
    import yaml
except ImportError:  # pragma: no cover
    print("PyYAML is required")
    sys.exit(2)

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
LANGS = "en es fr hi id pt ru uk".split()
REMOVED = {
    "help_button_news_channel",
    "help_button_support_group",
    "help_news_channel_text",
    "bot_updates_creators_plug",
    "language_help_translate",
}
PAT_DEV = re.compile(r"divkix|divanshu|divideprojects|dividesupport|divide projects|alita|FukuRobotUpdates|crowdin|fuku\.divkix", re.I)
OLD_NAME = re.compile(r"fuku", re.I)
TME = re.compile(r"t\.me/[^\s'\"<>)\]]*", re.I)
PLACEHOLDER = re.compile(r"%[-+# 0]*\d*(?:\.\d+)?[a-zA-Z%]|\{[A-Za-z_][A-Za-z0-9_-]*\}")
TAG = re.compile(r"</?([A-Za-z][A-Za-z0-9-]*)")


def base_sha():
    return open(os.path.join(HERE, "261115-rcd-base.txt")).read().strip()


def base_locale(lang):
    out = subprocess.check_output(["git", "show", f"{base_sha()}:locales/{lang}.yml"], cwd=REPO)
    return yaml.safe_load(out.decode("utf-8"))


def mode_locales():
    fails = 0
    cur = {}
    base = {}
    for l in LANGS:
        with open(os.path.join(REPO, "locales", f"{l}.yml"), encoding="utf-8") as f:
            cur[l] = yaml.safe_load(f)
        base[l] = base_locale(l)

    want = set(base["en"]) - REMOVED
    for l in LANGS:
        if set(base[l]) != set(base["en"]):
            print(f"FAIL {l}: base key set differs from en base")
            fails += 1
        have = set(cur[l])
        for k in sorted(want - have):
            print(f"FAIL {l}: missing key {k}")
            fails += 1
        for k in sorted(have - want):
            print(f"FAIL {l}: extra key {k}")
            fails += 1

    for l in LANGS:
        for k, v in cur[l].items():
            if not isinstance(v, str):
                continue
            bv = base[l].get(k)
            if v == "" or (len(v) > 4096 and not (isinstance(bv, str) and len(bv) > 4096 and len(v) <= len(bv))):
                print(f"FAIL {l}.{k}: empty or over 4096 chars ({len(v)})")
                fails += 1
            elif len(v) > 4096:
                print(f"NOTE {l}.{k}: over 4096 chars but not longer than the base value ({len(v)} <= {len(bv)}), untouched pre-existing")
            if PAT_DEV.search(v):
                print(f"FAIL {l}.{k}: original-developer pattern")
                fails += 1
            if OLD_NAME.search(v):
                print(f"FAIL {l}.{k}: old name")
                fails += 1
            for m in TME.findall(v):
                if "t.me/example" not in m.lower():
                    print(f"FAIL {l}.{k}: t.me link {m}")
                    fails += 1
            b = base[l].get(k)
            if isinstance(b, str):
                if collections.Counter(PLACEHOLDER.findall(v)) != collections.Counter(PLACEHOLDER.findall(b)):
                    print(f"FAIL {l}.{k}: placeholder multiset differs from base")
                    fails += 1
                if collections.Counter(TAG.findall(v)) != collections.Counter(TAG.findall(b)):
                    print(f"FAIL {l}.{k}: tag multiset differs from base")
                    fails += 1
                if l in ("en", "uk") and len(b) > 0 and len(v) > 1.2 * len(b):
                    print(f"REVIEW {l}.{k}: grew {len(b)} -> {len(v)}")
    print(f"locales: {fails} failure(s)")
    return 1 if fails else 0


FENCE = re.compile(r"^[ \t]*(```+|~~~+)[^\n]*\n.*?^[ \t]*\1[^\n]*$", re.M | re.S)  # fences may be indented inside Tabs/Steps
INLINE = re.compile(r"`[^`\n]*`")
URL = re.compile(r"https?://[^\s)>\"']+")
LINKTARGET = re.compile(r"\]\([^)]*\)")
ATTR = re.compile(r"=\s*\"[^\"]*\"|=\s*'[^']*'")
IMPORT = re.compile(r"^\s*(import|export)\b.*$", re.M)
WORD = re.compile(r"(?<![A-Za-z0-9_/.:-])fuku(?![A-Za-z0-9_/.:-])|FukuRobot|Fuku Robot|Fuku Go", re.I)


def iter_files(paths, exts):
    for p in paths:
        p = os.path.join(REPO, p) if not os.path.isabs(p) else p
        if os.path.isfile(p):
            yield p
            continue
        for root, dirs, files in os.walk(p):
            dirs[:] = [d for d in dirs if d not in ("node_modules", ".git")]
            for fn in files:
                if fn.endswith(exts):
                    yield os.path.join(root, fn)


def mode_prose(paths):
    hits = 0
    for fp in iter_files(paths, (".md", ".mdx", ".astro")):
        text = open(fp, encoding="utf-8", errors="replace").read()
        for m in PAT_DEV.finditer(text):
            line = text.count("\n", 0, m.start()) + 1
            print(f"HIT dev {os.path.relpath(fp, REPO)}:{line}: {m.group(0)}")
            hits += 1
        t = FENCE.sub(lambda m: "\n" * m.group(0).count("\n"), text)
        t = INLINE.sub("", t)
        t = LINKTARGET.sub("]()", t)
        t = URL.sub("", t)
        t = ATTR.sub("", t)
        t = IMPORT.sub("", t)
        for m in WORD.finditer(t):
            line = t.count("\n", 0, m.start()) + 1
            print(f"HIT name {os.path.relpath(fp, REPO)}:{line}: {m.group(0)}")
            hits += 1
    print(f"prose: {hits} hit(s)")
    return 1 if hits else 0


NUMWORDS = "one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve"
COUNT_RE = re.compile(
    r"(?P<num>\d+)\+?\s*(?:user-facing |internal |total |bot )?(?P<kind>modules?|commands?|languages?)\b"
    r"|\b(?P<word>" + NUMWORDS + r")\s+(?P<wkind>modules?|languages?)\b"
    r"|Total (?P<lab>Modules|Commands)\W{0,6}\s*:?\s*\W{0,3}(?P<labnum>\d+)",
    re.I,
)
EXPECT = {"module": 39, "command": 231, "language": 8}
WORDNUM = {w: i + 1 for i, w in enumerate(NUMWORDS.split("|"))}


def mode_counts(paths):
    hits = 0
    for fp in iter_files(paths, (".md", ".mdx", ".astro", ".ts")):
        raw = open(fp, encoding="utf-8", errors="replace").read()
        rel = os.path.relpath(fp, REPO)
        text = re.sub(r"\s+", " ", raw)
        # skip per-module card badges and the backup page's own module count
        skipped = []
        def skip_badge(m):
            skipped.append(m.group(0))
            return " "
        text = re.sub(r"<Badge[^>]*accent[^>]*>[^<]*</Badge>", skip_badge, text)
        text = re.sub(r"<Badge[^>]*variant=\"accent\"[^>]*/?>", skip_badge, text)
        if rel.endswith("backup/index.md") or "/backup/" in rel:
            for m in COUNT_RE.finditer(text):
                print(f"SKIP {rel}: backup page count '{m.group(0)}'")
            text = COUNT_RE.sub(" ", text)
        for s in skipped:
            print(f"SKIP {rel}: badge '{s[:60]}'")
        for m in COUNT_RE.finditer(text):
            if m.group("num"):
                n, kind = int(m.group("num")), m.group("kind").lower().rstrip("s")
            elif m.group("word"):
                n, kind = WORDNUM[m.group("word").lower()], m.group("wkind").lower().rstrip("s")
            else:
                n, kind = int(m.group("labnum")), m.group("lab").lower().rstrip("s")
            if n != EXPECT.get(kind):
                print(f"HIT count {rel}: '{m.group(0)}' (want {EXPECT.get(kind)} {kind})")
                hits += 1
        low = raw.lower()
        for line in raw.splitlines():
            ll = line.lower()
            if "hindi" in ll and "indonesian" in ll and "ukrainian" not in ll and "українськ" not in ll:
                print(f"HIT langlist {rel}: {line.strip()[:120]}")
                hits += 1
        if "id.yml" in low and "ru.yml" in low and "uk.yml" not in low:
            print(f"HIT locale tree without uk.yml {rel}")
            hits += 1
    print(f"counts: {hits} hit(s)")
    return 1 if hits else 0


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    mode = sys.argv[1]
    if mode == "locales":
        return mode_locales()
    if mode == "prose":
        return mode_prose(sys.argv[2:] or ["docs"])
    if mode == "counts":
        return mode_counts(sys.argv[2:] or ["docs"])
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main())
