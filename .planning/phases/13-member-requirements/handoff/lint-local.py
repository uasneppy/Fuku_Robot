#!/usr/bin/env python3
"""Local stand-in for the CI lint (golangci-lint cannot run in this sandbox).
Runs on the Go code added since a base commit: the repo's three extra linters (dupl at 100 tokens on non-test files,
  gocyclo over 19, godox TODO/FIXME/BUG) and the default linters that can run here (staticcheck, which includes
  unused, errcheck, and the QF quickfix checks via qfcheck, tests included).
Reports only findings that touch lines added since the base (committed or not), like CI's `new: true`.
Setup once (one install per tool; go install takes one versioned package at a time):
  for t in github.com/mibk/dupl github.com/fzipp/gocyclo/cmd/gocyclo honnef.co/go/tools/cmd/staticcheck github.com/kisielk/errcheck; do GOBIN=$LINT_TOOLS go install $t@latest; done
Also build qfcheck once: cd .planning/phases/11-message-editor/handoff/qfcheck && go mod tidy && go build -o $LINT_TOOLS/qfcheck .
Usage: python3 lint-local.py [base-rev]   (default 72a692b). Exit 1 when something is found."""
import subprocess, sys, re, os, collections
SP = os.environ.get("LINT_TOOLS", os.path.expanduser("~/.cache/fuku-lint-tools"))  # dir holding dupl, gocyclo, staticcheck, errcheck
os.chdir("/home/user/Fuku_Robot")
base = sys.argv[1] if len(sys.argv) > 1 else "72a692b"
def run(cmd, inp=None, env=None, merge=False):
    e = None if env is None else dict(os.environ, **env)
    if merge:   # qfcheck's multichecker prints its diagnostics to stderr
        return subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, input=inp, env=e).stdout
    return subprocess.run(cmd, capture_output=True, text=True, input=inp, env=e).stdout
added = collections.defaultdict(set)   # file -> added line numbers
diff = run(["git", "diff", "-U0", base, "--", "*.go"])
cur = None
for line in diff.splitlines():
    if line.startswith("+++ b/"):
        cur = line[6:]
    elif line.startswith("@@") and cur:
        m = re.search(r"\+(\d+)(?:,(\d+))?", line)
        s = int(m.group(1)); n = int(m.group(2) or 1)
        added[cur].update(range(s, s + n))
for f in run(["git", "ls-files", "--others", "--exclude-standard", "--", "*.go"]).split():
    added[f].update(range(1, 100000))
prod = {f: l for f, l in added.items() if not f.endswith("_test.go") and os.path.exists(f)}
problems = []
dirs = sorted({os.path.dirname(f) for f in prod})
for d in dirs:
    files = sorted(os.path.join(d, x) for x in os.listdir(d) if x.endswith(".go") and not x.endswith("_test.go"))
    out = run([SP + "/dupl", "-t", "100", "-files"], "\n".join(files) + "\n")
    clone = []
    for line in out.splitlines() + ["found"]:
        m = re.match(r"\s+(\S+\.go):(\d+),(\d+)", line)
        if m:
            clone.append((m.group(1), int(m.group(2)), int(m.group(3))))
        elif line.startswith("found") or not line.strip():
            if len(clone) >= 2:
                hit = [c for c in clone if c[0] in prod and c[1] in prod[c[0]]]  # golangci reports a clone at its first line
                if hit:
                    problems.append("dupl: " + " <-> ".join(f"{f}:{a}-{b}" for f, a, b in clone))
            clone = []
files = sorted(prod)
if files:
    for line in run([SP + "/gocyclo", "-over", "19"] + files).splitlines():
        m = re.match(r"(\d+)\s+\S+\s+(.+?)\s+(\S+\.go):(\d+):\d+", line)
        if m and int(m.group(4)) in prod.get(m.group(3), ()):
            problems.append("gocyclo: " + line)
# default golangci linters that can run here: staticcheck (incl. unused, U1000) and errcheck, tests included
allgo = {f: l for f, l in added.items() if os.path.exists(f)}
pkgs = sorted({"./" + os.path.dirname(f) for f in allgo})
for pkg in pkgs:
    for line in run([SP + "/staticcheck", "-tags", "testtools", "-checks", "all,-ST1000,-ST1003,-ST1016,-ST1020,-ST1021,-ST1022", pkg]).splitlines():
        m = re.match(r"(\S+\.go):(\d+):\d+: ", line)
        if m and int(m.group(2)) in allgo.get(m.group(1), ()):
            problems.append("staticcheck: " + line)
    # golangci's staticcheck also runs the QF quickfix checks. The multichecker ignores a -tags flag, so the build tag
    # goes through GOFLAGS; without it the test package fails to type-check and the QF checks silently report nothing.
    qf = run([SP + "/qfcheck", pkg], env={"GOFLAGS": "-tags=testtools"}, merge=True)
    if qf.startswith("-: "):
        problems.append("qfcheck could not type-check " + pkg + ": " + qf.splitlines()[0])
    for line in qf.splitlines():
        m = re.match(r"(\S+\.go):(\d+):\d+: ", line)
        if m:
            rel = os.path.relpath(m.group(1), os.getcwd()) if os.path.isabs(m.group(1)) else m.group(1)
            if int(m.group(2)) in allgo.get(rel, ()):
                problems.append("staticcheck-QF: " + rel + line[len(m.group(1)):])
    for line in run([SP + "/errcheck", "-tags", "testtools", pkg]).splitlines():
        m = re.match(r"(\S+\.go):(\d+):\d+:", line)
        if m and int(m.group(2)) in allgo.get(m.group(1), ()):
            problems.append("errcheck: " + line)
for f, lines in prod.items():
    for i, text in enumerate(open(f).read().splitlines(), 1):
        if i in lines and re.search(r"\b(TODO|FIXME|BUG)\b", text):
            problems.append(f"godox: {f}:{i}: {text.strip()}")
if problems:
    print("\n".join(problems)); sys.exit(1)
print("lint-local: no dupl, gocyclo or godox findings on lines added since", base)
