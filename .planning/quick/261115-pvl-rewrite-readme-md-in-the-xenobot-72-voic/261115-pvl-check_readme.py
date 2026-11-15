#!/usr/bin/env python3
"""README checker for quick task 261115-pvl (python3 standard library only).

Run from the checkout root.

  check_readme.py [--skip-link TARGET]... FILE...        per-file checks
  check_readme.py --parity README.md README.uk.md         parity between the two files

The fact sheet (261115-pvl-FACTS.md) and the command dump (261115-pvl-commands.txt)
are read from this script's own directory.

Output lines start with FAIL, REVIEW, WARN, INFO or OK. Exit status is 1 when any
FAIL line was printed. REVIEW lines never fail the run.

Fact sheet conventions the checker reads:
  * Section whose heading contains "Stated figures": every line "- <numbers> - <why>"
    adds its number tokens to the allowed set for numbers written in prose.
  * Lines "- allow-slash: word word ..." list slash words in the Commands section
    that are not registry command names.

Forbidden strings (the checker FAILS on any hit, even inside code, because none of
them may be readable or typeable anywhere in a README): the original project's
developer handle and name, its project name, the words upstream and fork.
The old bot name "fuku" is allowed only inside fenced code, inline code spans, URLs,
link targets and HTML attributes (identifiers the reader must type), never in prose.
The owner's handle "uasneppy" is allowed.
"""
import html.parser
import os
import re
import sys
import unicodedata
import urllib.parse

HERE = os.path.dirname(os.path.abspath(__file__))
FACTS = os.path.join(HERE, "261115-pvl-FACTS.md")
COMMANDS = os.path.join(HERE, "261115-pvl-commands.txt")

# Forbidden anywhere (case-insensitive). Keep this list in sync with the fact sheet.
FORBIDDEN = re.compile(r"divkix|divideprojects|divanshu|alita|\bupstream|\bfork", re.I)
OLD_NAME = re.compile(r"fuku", re.I)

FAILS = 0
REVIEWS = 0


def out(kind, check, where, msg):
    global FAILS, REVIEWS
    if kind == "FAIL":
        FAILS += 1
    elif kind == "REVIEW":
        REVIEWS += 1
    print(f"{kind} [{check}] {where}: {msg}")


def line_of(text, pos):
    return text.count("\n", 0, pos) + 1


def blank(s):
    """Replace every char but newlines with a space, keeping length and line numbers."""
    return re.sub(r"[^\n]", " ", s)


# --------------------------------------------------------------------------- masking

FENCE_RE = re.compile(r"^[ \t]*(`{3,}|~{3,})")


def mask_fences(text):
    """Blank fenced code blocks (markers included). Returns (masked, count)."""
    lines = text.split("\n")
    res = []
    fence = None
    count = 0
    for ln in lines:
        m = FENCE_RE.match(ln)
        if fence is None:
            if m:
                fence = (m.group(1)[0], len(m.group(1)))
                count += 1
                res.append(blank(ln))
            else:
                res.append(ln)
        else:
            if m and m.group(1)[0] == fence[0] and len(m.group(1)) >= fence[1] and ln.strip() == m.group(1):
                fence = None
            res.append(blank(ln))
    return "\n".join(res), count


SPAN_RE = re.compile(r"(?<!`)(`+)(?!`)(.+?)(?<!`)\1(?!`)", re.S)


def mask_spans(text):
    return SPAN_RE.sub(lambda m: blank(m.group(0)), text)


def mask_code(text):
    t, n = mask_fences(text)
    return mask_spans(t), n


URL_RE = re.compile(r"https?://[^\s)<>\"']+")
TARGET_RE = re.compile(r"\]\([^)\n]*\)")
TAG_RE = re.compile(r"<!--.*?-->|</?[A-Za-z][^<>]*>", re.S)
AUTOLINK_RE = re.compile(r"<https?://[^>\s]+>")


def prose_of(masked):
    """Code already blanked. Blank URLs, link targets and HTML tags."""
    t = TARGET_RE.sub(lambda m: "]" + blank(m.group(0)[1:]), masked)
    t = AUTOLINK_RE.sub(lambda m: blank(m.group(0)), t)
    t = URL_RE.sub(lambda m: blank(m.group(0)), t)
    t = TAG_RE.sub(lambda m: blank(m.group(0)), t)
    return t


ALT_RE = re.compile(r"\b(?:alt|title)=\"([^\"]*)\"")


def alts_of(masked):
    return [(m.group(1), line_of(masked, m.start(1))) for m in ALT_RE.finditer(masked)]


# --------------------------------------------------------------------------- headings

def heading_text_to_slug_source(raw):
    s = raw.strip()
    s = re.sub(r"!\[[^\]]*\]\([^)]*\)", "", s)
    s = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", s)
    s = re.sub(r"<[^>]*>", "", s)
    s = s.replace("`", "").replace("*", "")
    s = re.sub(r"\s+#+\s*$", "", s)
    return s.strip()


def slugify(src):
    s = src.lower()
    keep = []
    for ch in s:
        cat = unicodedata.category(ch)
        if ch == " " or ch == "-" or cat[0] in "LMN" or cat == "Pc":
            keep.append(ch)
    return "".join(keep).replace(" ", "-")


def headings(text):
    """List of (level, raw_text, line, slug) outside fenced blocks, GitHub slugs with duplicates."""
    masked, _ = mask_fences(text)
    res = []
    seen = {}
    for i, ln in enumerate(masked.split("\n"), 1):
        m = re.match(r"^ {0,3}(#{1,6})\s+(.*?)\s*$", ln)
        if m:
            src = heading_text_to_slug_source(m.group(2))
            base = slugify(src)
            n = seen.get(base, 0)
            seen[base] = n + 1
            slug = base if n == 0 else f"{base}-{n}"
            res.append((len(m.group(1)), src, i, slug))
    for m in re.finditer(r"<h([1-6])[^>]*>(.*?)</h\1>", masked, re.S | re.I):
        src = heading_text_to_slug_source(m.group(2))
        base = slugify(src)
        n = seen.get(base, 0)
        seen[base] = n + 1
        res.append((int(m.group(1)), src, line_of(masked, m.start()), base if n == 0 else f"{base}-{n}"))
    return res


def h2_sections(text):
    """Return list of (title, start_line, end_line_exclusive) for H2 sections."""
    hs = headings(text)
    nlines = text.count("\n") + 2
    res = []
    for idx, (lvl, src, line, _slug) in enumerate(hs):
        if lvl != 2:
            continue
        end = nlines
        for lvl2, _s, line2, _sl in hs[idx + 1:]:
            if lvl2 <= 2:
                end = line2
                break
        res.append((src, line, end))
    return res


def section_text(text, start, end):
    lines = text.split("\n")
    return "\n".join(lines[start - 1:end - 1])


# --------------------------------------------------------------------------- facts

def load_facts():
    figs = set()
    allow = set()
    if not os.path.exists(FACTS):
        return figs, allow, False
    lines = open(FACTS, encoding="utf-8").read().split("\n")
    in_g = False
    glevel = 0
    for ln in lines:
        hm = re.match(r"^(#{1,6})\s+(.*)$", ln)
        if hm:
            if "stated figures" in hm.group(2).lower():
                in_g = True
                glevel = len(hm.group(1))
                continue
            elif in_g and len(hm.group(1)) <= glevel:
                in_g = False
        am = re.match(r"^-\s*allow-slash:\s*(.*)$", ln)
        if am:
            for w in am.group(1).replace("/", " ").split():
                allow.add(w.strip("`,"))
        if in_g and ln.startswith("- "):
            body = ln[2:]
            left = re.split(r"\s[-—]\s", body, maxsplit=1)[0]
            for tok in NUM_RE.findall(left):
                figs.add(tok)
    return figs, allow, True


NUM_RE = re.compile(r"(?<![\d.,])\d+(?:[.,]\d+)*")


def load_commands():
    names = []
    if os.path.exists(COMMANDS):
        for ln in open(COMMANDS, encoding="utf-8"):
            m = re.match(r"^CMD (\S+)$", ln.strip())
            if m:
                names.append(m.group(1))
    return names


# --------------------------------------------------------------------------- checks

def check_names(path, text, masked_all, prose, alts):
    for m in FORBIDDEN.finditer(text):
        out("FAIL", "names", f"{path}:{line_of(text, m.start())}", f"forbidden string {m.group(0)!r}")
    for m in OLD_NAME.finditer(prose):
        out("FAIL", "names", f"{path}:{line_of(prose, m.start())}", "old bot name in reader-visible prose")
    for a, ln in alts:
        if OLD_NAME.search(a):
            out("FAIL", "names", f"{path}:{ln}", "old bot name in alt/title text")
    for m in OLD_NAME.finditer(text):
        if not OLD_NAME.match(prose, m.start()):
            out("INFO", "names", f"{path}:{line_of(text, m.start())}", "allowed identifier hit (code, URL, target or attribute)")


BRAND_BAD = re.compile(r"xenobot[\s_-]*72", re.I)


def check_brand(path, prose):
    for m in BRAND_BAD.finditer(prose):
        if m.group(0) != "Xenobot-72":
            out("FAIL", "brand", f"{path}:{line_of(prose, m.start())}", f"bad spelling {m.group(0)!r}, use Xenobot-72")
    for m in re.finditer(r"Xenobot-72(?=[Ѐ-ӿ])", prose):
        out("FAIL", "brand", f"{path}:{line_of(prose, m.start())}", "Xenobot-72 with a Cyrillic ending glued on")
    for m in re.finditer(r"Xenobot[Ѐ-ӿ]", prose):
        out("FAIL", "brand", f"{path}:{line_of(prose, m.start())}", "Xenobot with Cyrillic letters")


LINK_RES = [
    re.compile(r"!?\[[^\]\n]*\]\(\s*<?([^)\s>]+)>?(?:\s+\"[^\"]*\")?\s*\)"),
    re.compile(r"\b(?:href|src)=\"([^\"]*)\""),
    re.compile(r"^\s{0,3}\[[^\]]+\]:\s*(\S+)", re.M),
]


def extract_links(masked):
    res = []
    for rx in LINK_RES:
        for m in rx.finditer(masked):
            res.append((m.group(1), line_of(masked, m.start(1))))
    return res


def check_links(path, text, masked, skips, cache):
    base = os.path.dirname(os.path.abspath(path))
    own_slugs = {h[3] for h in headings(text)}
    hosts = set()
    for tgt, ln in extract_links(masked):
        where = f"{path}:{ln}"
        if re.match(r"^[A-Za-z][A-Za-z0-9+.-]*:", tgt):
            if tgt.startswith("mailto:"):
                continue
            if tgt.startswith("http://"):
                out("WARN", "links", where, f"plain http link {tgt}")
            elif not tgt.startswith("https://"):
                out("FAIL", "links", where, f"unsupported scheme {tgt}")
            host = urllib.parse.urlparse(tgt).netloc.lower()
            hosts.add(host)
            if FORBIDDEN.search(tgt):
                out("FAIL", "links", where, f"external link to a forbidden target {tgt}")
            continue
        if tgt.startswith("#"):
            frag = urllib.parse.unquote(tgt[1:])
            if frag not in own_slugs:
                out("FAIL", "links", where, f"anchor {tgt} matches no heading of this file")
            continue
        pathpart, _, frag = tgt.partition("#")
        pathpart = pathpart.split("?", 1)[0]
        pathpart = urllib.parse.unquote(pathpart)
        full = os.path.normpath(os.path.join(base, pathpart))
        if tgt in skips or pathpart in skips:
            out("INFO", "links", where, f"skipped existence test for {tgt}")
            continue
        if not os.path.exists(full):
            out("FAIL", "links", where, f"relative target {tgt} does not exist")
            continue
        if frag and full.endswith(".md") and os.path.isfile(full):
            if full not in cache:
                cache[full] = {h[3] for h in headings(open(full, encoding="utf-8").read())}
            if urllib.parse.unquote(frag) not in cache[full]:
                out("FAIL", "links", where, f"anchor #{frag} matches no heading of {pathpart}")
    for h in sorted(hosts):
        out("INFO", "links", path, f"external host {h}")


VOID = {"br", "hr", "img", "input", "meta", "link", "wbr", "col", "area", "base", "source", "track", "embed", "param"}


class Balance(html.parser.HTMLParser):
    def __init__(self, path, text):
        super().__init__(convert_charrefs=True)
        self.path = path
        self.text = text
        self.stack = []
        self.summary_in_details = []
        self.problems = []

    def where(self):
        return f"{self.path}:{self.getpos()[0]}"

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag in ("script", "iframe"):
            out("FAIL", "html", self.where(), f"<{tag}> is not allowed")
        if "style" in a:
            out("FAIL", "html", self.where(), "style attribute is not allowed")
        if tag == "img" and not a.get("alt"):
            out("FAIL", "html", self.where(), "<img> without alt")
        if tag == "a" and not a.get("href"):
            out("FAIL", "html", self.where(), "<a> without href")
        if tag == "details":
            self.summary_in_details.append(0)
        if tag == "summary":
            if not self.summary_in_details:
                out("FAIL", "html", self.where(), "<summary> outside <details>")
            else:
                self.summary_in_details[-1] += 1
        if tag in VOID:
            return
        self.stack.append((tag, self.getpos()[0]))

    def handle_startendtag(self, tag, attrs):
        a = dict(attrs)
        if tag == "img" and not a.get("alt"):
            out("FAIL", "html", self.where(), "<img> without alt")

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if tag == "details":
            if self.summary_in_details:
                n = self.summary_in_details.pop()
                if n != 1:
                    out("FAIL", "html", self.where(), f"<details> closed with {n} <summary> elements, need exactly 1")
        if not self.stack:
            out("FAIL", "html", self.where(), f"stray </{tag}>")
            return
        if self.stack[-1][0] == tag:
            self.stack.pop()
            return
        for i in range(len(self.stack) - 1, -1, -1):
            if self.stack[i][0] == tag:
                out("FAIL", "html", self.where(), f"</{tag}> closes while <{self.stack[-1][0]}> (line {self.stack[-1][1]}) is still open")
                del self.stack[i:]
                return
        out("FAIL", "html", self.where(), f"stray </{tag}>")


def check_html(path, masked):
    t = AUTOLINK_RE.sub(lambda m: blank(m.group(0)), masked)
    p = Balance(path, t)
    p.feed(t)
    p.close()
    for tag, ln in p.stack:
        out("FAIL", "html", f"{path}:{ln}", f"<{tag}> is never closed")
    for m in re.finditer(r"</summary>", t):
        rest = t[m.end():]
        if not re.match(r"[ \t]*\n[ \t]*\n", rest):
            out("FAIL", "html", f"{path}:{line_of(t, m.start())}", "</summary> must be followed by a blank line")


def split_cells(row):
    row = row.strip()
    cells = re.split(r"(?<!\\)\|", row)
    if row.startswith("|"):
        cells = cells[1:]
    if row.endswith("|") and not row.endswith("\\|"):
        cells = cells[:-1]
    return cells


def tables_of(masked_fences_only):
    """Yield (start_line, [rows]) for each pipe table outside fenced blocks."""
    lines = masked_fences_only.split("\n")
    i = 0
    res = []
    while i < len(lines) - 1:
        if "|" in lines[i] and re.match(r"^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$", lines[i + 1]) and "|" in lines[i + 1]:
            j = i + 2
            while j < len(lines) and lines[j].strip() and "|" in lines[j]:
                j += 1
            res.append((i + 1, lines[i:j]))
            i = j
        else:
            i += 1
    return res


def check_tables(path, text):
    mf, _ = mask_fences(text)
    for start, rows in tables_of(mf):
        n = len(split_cells(rows[0]))
        for k, r in enumerate(rows):
            if k == 1:
                continue
            c = len(split_cells(r))
            if c != n:
                out("FAIL", "tables", f"{path}:{start + k}", f"row has {c} cells, header has {n} (escape literal pipes as \\|)")


def check_hygiene(path, text):
    for pat in (r"\bTODO\b", r"\bFIXME\b", r"\bTBD\b", r"\bXXX\b", r"lorem", r"<insert"):
        for m in re.finditer(pat, text, re.I if pat in ("lorem", r"<insert") else 0):
            out("FAIL", "hygiene", f"{path}:{line_of(text, m.start())}", f"placeholder {m.group(0)!r}")
    secrets = [
        (r"\b\d{6,}:[A-Za-z0-9_-]{35}\b", "bot token shape"),
        (r"\bsk-[A-Za-z0-9_-]{10,}", "sk- key shape"),
        (r"\bghp_[A-Za-z0-9]+", "ghp_ token shape"),
        (r"\bBearer\s+[A-Za-z0-9._~+/=-]{20,}", "bearer token shape"),
    ]
    for pat, what in secrets:
        for m in re.finditer(pat, text):
            out("FAIL", "hygiene", f"{path}:{line_of(text, m.start())}", what)


STEP_PREFIX = re.compile(r"^[ \t]*(?:#{1,6}[ \t]+)?\d+[a-z]?[.)][ \t]+", re.M)


def check_figures(path, prose, alts, figs, have_facts):
    if not have_facts:
        out("FAIL", "figures", path, "fact sheet missing, cannot verify stated figures")
    t = prose.replace("Xenobot-72", " " * len("Xenobot-72"))
    t = STEP_PREFIX.sub(lambda m: blank(m.group(0)), t)
    lines = t.split("\n")
    # pipe-table rows are covered by the command and environment evidence rows
    mf = t
    skip_lines = set()
    for start, rows in tables_of(mf):
        for k in range(len(rows)):
            skip_lines.add(start + k)
    bad = {}
    for i, ln in enumerate(lines, 1):
        if i in skip_lines:
            continue
        for m in NUM_RE.finditer(ln):
            tok = m.group(0)
            if tok not in figs:
                bad.setdefault(tok, []).append(i)
    for a, aln in alts:
        for m in NUM_RE.finditer(a):
            if m.group(0) not in figs:
                bad.setdefault(m.group(0), []).append(aln)
    for tok, lns in sorted(bad.items()):
        out("FAIL", "figures", f"{path}:{','.join(str(x) for x in lns[:6])}", f"number {tok} is not in the fact sheet's Stated figures list")
    NUMWORDS = r"\b(two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|twenty|thirty|hundred|thousand|dozen|million)\b|\b(два|дві|три|чотири|п.ять|шість|сім|вісім|дев.ять|десять|двадцять|сто|тисяч\w*)\b"
    hits = [line_of(prose, m.start()) for m in re.finditer(NUMWORDS, prose, re.I)]
    if hits:
        out("REVIEW", "figures", path, f"spelled-out number words on lines {sorted(set(hits))[:20]}: each must be a verified count")


def check_commands(path, text, cmds, allow):
    secs = h2_sections(text)
    sec = None
    for title, s, e in secs:
        plain = re.sub(r"[^\w]+", "", title).lower()
        if plain in ("commands", "команди"):
            sec = (s, e)
            break
    if sec is None:
        out("FAIL", "commands", path, "no H2 section named Commands or Команди")
        return
    body = section_text(text, sec[0], sec[1])
    missing = []
    for c in cmds:
        if not re.search(r"(?<![A-Za-z0-9_/])/" + re.escape(c) + r"(?![A-Za-z0-9_])", body):
            missing.append(c)
    if missing:
        out("FAIL", "commands", path, f"{len(missing)} registry command(s) missing from the Commands section: {' '.join('/' + c for c in missing)}")
    else:
        out("OK", "commands", path, f"all {len(cmds)} registry commands appear in the Commands section")
    mf, _ = mask_fences(body)
    reg = set(cmds)
    for m in SPAN_RE.finditer(mf):
        for w in re.finditer(r"(?:^|[\s(|,])/([A-Za-z][A-Za-z0-9_]*)", m.group(2)):
            name = w.group(1)
            if name.lower() not in reg and name not in allow and name.lower() not in allow:
                out("FAIL", "commands", f"{path}:{sec[0] + mf.count(chr(10), 0, m.start())}", f"/{name} in a code span is neither a registry command nor in the fact sheet's allow-list")


UK_BAD_LETTERS = re.compile(r"[ыэъёЫЭЪЁ]")
UK_CALQUES = re.compile(
    r"слідуюч\w*|приймат\w*\s+участь|прийня\w*\s+участь|на\s+протязі|являєть?ся|являються|відноситься|відносяться|"
    r"на\s+данний|данн(?:ий|а|е|і)\b|кушат\w*", re.I)


def check_ukrainian(path, prose):
    for m in UK_BAD_LETTERS.finditer(prose):
        out("FAIL", "ukrainian", f"{path}:{line_of(prose, m.start())}", f"Russian-only letter {m.group(0)!r}")
    for m in UK_CALQUES.finditer(prose):
        out("FAIL", "ukrainian", f"{path}:{line_of(prose, m.start())}", f"calque {m.group(0)!r}")
    for m in re.finditer(r"\bя\s+(?:\S+\s+)?\w*в\b", prose, re.I):
        out("REVIEW", "ukrainian", f"{path}:{line_of(prose, m.start())}", f"possible masculine first-person past {m.group(0)!r}")
    for m in re.finditer(r"\bсам\s+себе\b|\bсама\s+себе\b", prose, re.I):
        out("REVIEW", "ukrainian", f"{path}:{line_of(prose, m.start())}", f"{m.group(0)!r}: check the gender form")
    for m in re.finditer(r"\b(ти|тобі|тебе|твій|твоя|твоє|твої|твого|твоєї|тобою|тебе)\b", prose, re.I):
        out("REVIEW", "ukrainian", f"{path}:{line_of(prose, m.start())}", f"informal pronoun {m.group(0)!r}")


NEVER = re.compile(
    r"starv\w*|famine\w*|hunger\w*|\bwar\b|wars\b|death\w*|\bdead\b|\bgore\b|cannibal\w*|\bkill\w*|\bblood\w*|"
    r"голод\w*|війн\w*|смерт\w*|кров\w*|канібал\w*|вбив\w*|вбит\w*", re.I)
SERIES = re.compile(r"tempe|темпе|jaya\s+abadi|\bration|пайок|пайк\w*|cat\s+photo|фото\s+котів|facility|об.єкт\w*|containment|ізоляці\w*\s+об", re.I)


def check_voice(path, text, prose):
    for m in NEVER.finditer(prose):
        out("REVIEW", "voice", f"{path}:{line_of(prose, m.start())}", f"never-joke stem {m.group(0)!r}: confirm it is exact text and not a joke")
    for title, s, e in h2_sections(text):
        body = section_text(prose, s, e)
        n = len(SERIES.findall(body))
        out("INFO", "voice", f"{path}:{s}", f"series-reference words in section {title!r}: {n}")


def check_file(path, skips, cmds, allow, figs, have_facts, cache):
    text = open(path, encoding="utf-8").read()
    masked, _n = mask_code(text)
    prose = prose_of(masked)
    alts = alts_of(masked)
    check_names(path, text, masked, prose, alts)
    check_brand(path, prose)
    check_links(path, text, masked, skips, cache)
    check_html(path, masked)
    check_tables(path, text)
    check_hygiene(path, text)
    check_figures(path, prose, alts, figs, have_facts)
    check_commands(path, text, cmds, allow)
    if path.endswith(".uk.md"):
        check_ukrainian(path, prose)
    check_voice(path, text, prose)
    check_sibling(path, text)


def check_sibling(path, text):
    base = os.path.basename(path)
    sib = {"README.md": "README.uk.md", "README.uk.md": "README.md"}.get(base)
    if sib is None:
        return
    head = "\n".join(text.split("\n")[:30])
    if not re.search(r"\]\(" + re.escape(sib) + r"\)|href=\"" + re.escape(sib) + r"\"", head):
        out("FAIL", "links", path, f"no link to {sib} within the first 30 lines")


# --------------------------------------------------------------------------- parity

def slash_set(prose):
    return set(re.findall(r"(?<![\w/.:-])/([a-z][a-z0-9_]*)", prose))


def env_set(masked_fences_only):
    s = set()
    for m in SPAN_RE.finditer(masked_fences_only):
        for w in re.finditer(r"\b[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+\b", m.group(2)):
            s.add(w.group(0))
    return s


def link_files(masked):
    s = set()
    for tgt, _ln in extract_links(masked):
        if re.match(r"^[A-Za-z][A-Za-z0-9+.-]*:", tgt) or tgt.startswith("#"):
            continue
        t = tgt.split("#", 1)[0].split("?", 1)[0]
        if t in ("README.md", "README.uk.md", ""):
            continue  # the two files link to each other, checked separately
        s.add(t)
    return s


def parity(a, b):
    ta = open(a, encoding="utf-8").read()
    tb = open(b, encoding="utf-8").read()
    ma, na = mask_code(ta)
    mb, nb = mask_code(tb)
    ha = [h[0] for h in headings(ta)]
    hb = [h[0] for h in headings(tb)]
    if ha != hb:
        out("FAIL", "parity", f"{a} vs {b}", f"heading level sequences differ ({len(ha)} vs {len(hb)} headings)")
    else:
        out("OK", "parity", f"{a} vs {b}", f"{len(ha)} headings, same levels")
    if sum(1 for x in ha if x == 2) != sum(1 for x in hb if x == 2):
        out("FAIL", "parity", f"{a} vs {b}", "different number of H2 sections")
    # commands: from Commands sections, plus whole-file slash words in prose and code spans
    def slashes(t, m):
        mf, _ = mask_fences(t)
        return set(re.findall(r"(?<![\w/.:-])/([a-z][a-z0-9_]*)", prose_of(mf)))
    sa, sb = slashes(ta, ma), slashes(tb, mb)
    if sa != sb:
        out("FAIL", "parity", f"{a} vs {b}", f"slash-command sets differ: only {a}: {sorted(sa - sb)}; only {b}: {sorted(sb - sa)}")
    ea = env_set(mask_fences(ta)[0])
    eb = env_set(mask_fences(tb)[0])
    if ea != eb:
        out("FAIL", "parity", f"{a} vs {b}", f"environment name sets differ: only {a}: {sorted(ea - eb)}; only {b}: {sorted(eb - ea)}")
    else:
        out("OK", "parity", f"{a} vs {b}", f"{len(ea)} environment names in both")
    la, lb = link_files(ma), link_files(mb)
    if la != lb:
        out("FAIL", "parity", f"{a} vs {b}", f"relative link target sets differ: only {a}: {sorted(la - lb)}; only {b}: {sorted(lb - la)}")
    if na != nb:
        out("FAIL", "parity", f"{a} vs {b}", f"fenced block counts differ ({na} vs {nb})")
    da = len(re.findall(r"<details", ma))
    db = len(re.findall(r"<details", mb))
    if da != db:
        out("FAIL", "parity", f"{a} vs {b}", f"<details> counts differ ({da} vs {db})")
    tca = len(tables_of(mask_fences(ta)[0]))
    tcb = len(tables_of(mask_fences(tb)[0]))
    if tca != tcb:
        out("FAIL", "parity", f"{a} vs {b}", f"table counts differ ({tca} vs {tcb})")
    for x, y, tx in ((a, b, ta), (b, a, tb)):
        head = "\n".join(tx.split("\n")[:30])
        if os.path.basename(y) not in head:
            out("FAIL", "parity", x, f"does not link to {os.path.basename(y)} within its first 30 lines")
    # prose length ratio
    pa = len(re.sub(r"\s+", " ", prose_of(ma)))
    pb = len(re.sub(r"\s+", " ", prose_of(mb)))
    out("INFO", "parity", f"{a} vs {b}", f"prose characters {pa} vs {pb} (ratio {pb / pa * 100:.0f}%)" if pa else "empty first file")


# --------------------------------------------------------------------------- main

def main(argv):
    skips = set()
    files = []
    mode_parity = False
    i = 0
    while i < len(argv):
        if argv[i] == "--skip-link":
            skips.add(argv[i + 1])
            i += 2
        elif argv[i] == "--parity":
            mode_parity = True
            i += 1
        else:
            files.append(argv[i])
            i += 1
    if mode_parity:
        if len(files) != 2:
            print("usage: check_readme.py --parity A B")
            return 2
        parity(*files)
    else:
        if not files:
            print("usage: check_readme.py [--skip-link T]... FILE...")
            return 2
        figs, allow, have = load_facts()
        cmds = load_commands()
        if not cmds:
            out("FAIL", "commands", COMMANDS, "no CMD lines found in the command dump")
        cache = {}
        for f in files:
            check_file(f, skips, cmds, allow, figs, have, cache)
    print(f"SUMMARY fails={FAILS} reviews={REVIEWS}")
    return 1 if FAILS else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
