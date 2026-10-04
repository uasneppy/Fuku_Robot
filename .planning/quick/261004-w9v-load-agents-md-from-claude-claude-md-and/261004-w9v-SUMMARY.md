---
phase: quick-261004-w9v
plan: 01
subsystem: docs
tags: [claude-md, agents-md, i18n, locales]
requires: []
provides:
  - "@../AGENTS.md import on line 1 of .claude/CLAUDE.md"
  - "Corrected locale list (en, es, fr, hi, id, pt, ru) in CLAUDE.md, ARCHITECTURE.md, STRUCTURE.md"
affects: [.claude/CLAUDE.md, .planning/codebase/ARCHITECTURE.md, .planning/codebase/STRUCTURE.md]
key-files:
  modified:
    - .claude/CLAUDE.md
    - .planning/codebase/ARCHITECTURE.md
    - .planning/codebase/STRUCTURE.md
decisions:
  - "Import placed above the first GSD marker so generate-claude-md preserves it"
status: complete
commits: 2
plan_head_before: 30219d93bdab12554a34a7aed1007891ce18769c
plan_head_after: 43ba5c3
actuals:
  tokens: 2000
  tasks: 2
  commits: 2
completed: 2026-10-04
---

# Quick 261004-w9v: Load AGENTS.md from .claude/CLAUDE.md and fix locale list

Bare `@../AGENTS.md` import on line 1 of `.claude/CLAUDE.md` (above all GSD markers, survives regeneration), plus the i18n locale list corrected to en, es, fr, hi, id, pt, ru in all three docs.

## Tasks

| Task | Commit | Files |
| ---- | ------ | ----- |
| 1. Import AGENTS.md on line 1 | 413f48f | .claude/CLAUDE.md |
| 2. Correct locale list | 43ba5c3 | .claude/CLAUDE.md, .planning/codebase/ARCHITECTURE.md, .planning/codebase/STRUCTURE.md |

## Verification

- Line 1 is `@../AGENTS.md`, line 2 blank, line 3 the unchanged GSD project-start marker; import appears once.
- The import took effect in the session immediately (AGENTS.md content was loaded after the edit).
- generate-claude-md round-trip on a scratch copy: `diff -B` clean after both tasks; the regenerated copy keeps the import and the corrected Purpose line, with no old locale codes.
- No `ro.yml`, `tr.yml`, `pt, ro`, Romanian or Turkish left in `.claude/CLAUDE.md` or `.planning/codebase/*.md`.
- STRUCTURE.md tree lists en, es, fr, hi, id, pt, ru, config in order, matching `ls locales/`.

## Deviations from Plan

**1. [Rule 3 - Blocking] Verify commands run in split form with the main checkout's gsd-tools**
- The worktree's `.claude/gsd-core/bin/lib/vendor/` is missing (`re2js.cjs`), so `node .claude/gsd-core/bin/gsd-tools.cjs` failed in the worktree. The harness also refused the plan's compound one-line verify. I ran the same checks as separate commands, using `/home/user/Fuku_Robot/.claude/gsd-core/bin/gsd-tools.cjs` with cwd = worktree and `--output` to a scratch copy. Same assertions, same results. No file changes.

**2. SUMMARY location**
- The harness refused a Write to the main-checkout path, so this SUMMARY was written in the worktree at the same repo-relative path, uncommitted. The orchestrator needs to copy it to the main checkout.

## Known Stubs

None.

## Self-Check: PASSED

- 413f48f and 43ba5c3 exist on branch worktree-agent-a8c2f1126df12098d; `git rev-list --count` from base = 2.
- Only the three planned files changed; no deletions.

Manual check remaining (human): start a fresh Claude Code session at repo root and run /memory to confirm AGENTS.md is listed as imported.
