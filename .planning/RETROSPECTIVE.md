# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.1 — PM Settings Hub & Member Screening

**Shipped:** 2026-11-13
**Phases:** 10 | **Plans:** 126 | **Sessions:** not tracked (the autonomous run spanned many context windows, each continued from a summary)

### What Was Built
- A private-chat settings hub gated by the group's owner, with a roster of manageable groups, a Main grid of 18 pages and one press pipeline (live `getChatMember`, owner gate, per-control right) that every page and every typed reply goes through.
- A message editor for Welcome, Goodbye and Rules (text, media, buttons, previews, media under text, greet once) on a Redis-backed typed-reply door.
- Member requirements: name, photo, channel and AI spam-name rules with per-rule punishments, one notice per episode and an "I fixed it" button.
- A durable deletion queue for service messages, the bot's own messages, members' commands, edits and self-destruct.
- Pages for warnings, switches, language, log channel, anonymous admins and locks, then blacklist, filters and notes on one list engine with a generic two-step add.

### What Worked
- One apply function per setting, with a characterization golden committed alone before each extraction, plus parity, drift and authority-parity tests: no page ever drifted from its command, and the goldens were never re-recorded.
- Gating every plan boundary myself (build, gofmt, vet, focused tests under the CI environment and bare, the lint stand-in, a skip grep, `go.mod`) caught what executor reports missed: a hard-coded Main-entry count, a settings callback that bypassed its apply function (Phase 18), a pager code that collided with a Deletion page code (Phase 19).
- Executors that commit after every sub-step survived two container restarts mid-plan; the work was resumed from the working tree.
- Provisional owner defaults recorded as owner confirmations, each owner-visible command change in its own revertable commit, and every rule written into AGENTS.md in the same commit as the code.
- A separate audit pass per phase (security, verification, then an integration check across phases) found nothing blocking and one low finding that was fixed.

### What Was Inefficient
- Test time: `fuku/modules` grew from about 600 s to about 1,900 s locally and 28 to 30 minutes on CI against a 40 minute limit; every plan added 2 to 16 s and had to be argued down.
- Plan text went stale against the code (counts of codes, apply functions and keys, a pager code); executors and I had to count from the registry each time.
- Each push cancels the in-flight CI run, so CI verified plan boundaries only, and a cancelled Test Suite repeatedly produced a "CI Success failure" notice that had to be checked by hand.
- Sandbox limits: `make lint` and gosec cannot run here (golangci-lint is built with Go 1.25), disk filled once, and the write guard's documented milestone-reset sentinel did not work because the guard is registered twice.
- Ten phases of Telegram behaviour were verified only on a faked client; the live checks all sit with the owner.

### Patterns Established
- Characterization golden first, apply function second, owner-visible change third, each its own commit.
- Unknown never punishes and never deletes; lookups that cannot be answered fail closed; caches hold positives only.
- Menus are stateless, authority is a live `getChatMember` on every press, and a group menu never reads private data.
- Counts and codes are asserted from the registry, never from prose; sweeps keep only the worst few of a notice and the full product only where the keyboard changes.
- Deferred live checks are written as a numbered `NN-SPIKE.md` per phase and indexed in STATE.md.

### Key Lessons
1. Run a short live-check pass after every two or three phases of the next milestone instead of deferring all of them; unverified Telegram behaviour compounds across phases.
2. Give every plan a test-time budget up front and measure it per plan, because the cost of the last 10 percent of a 40 minute limit is a whole class of work.
3. Have the orchestrator re-verify an executor's numbers (counts, codes, byte sizes) against the code rather than the plan or the report.
4. Commit after each sub-step and keep executor briefs short and explicit about what never to push or amend.
5. Treat guards that are registered in two places (a hook in both the user and project settings) as one broken hatch and fix the registration before relying on a single-use sentinel.

### Cost Observations
- Model mix: not measured. Planning and execution ran on the balanced profile for most phases and on the budget profile (sonnet executors, haiku verifier and integration checker) from Phase 19.
- Sessions: not tracked.
- Notable: the whole-package test run (about 33 minutes) dominates wall-clock time per plan; executors spent most of their time waiting on it.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | not tracked | 9 | Staff Group, lockdowns, Turnstile captcha, first settings menu |
| v1.1 | not tracked | 10 | Golden-first apply functions, per-boundary orchestrator gates, deferred live checks as SPIKE files |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v1.0 | not recorded | not recorded | 0 |
| v1.1 | about 1,840 top-level tests in `fuku/modules` plus the repository packages (about 201,000 lines of test code) | not measured here | 0 (`go.mod` unchanged against the v1.0 closing commit) |

### Top Lessons (Verified Across Milestones)

1. Live checks deferred to the owner accumulate; both milestones ended with a long checklist (v1.0: 10 of 11 UAT checks, v1.1: 10 SPIKE files).
2. A rulebook kept in the same commits as the code (`AGENTS.md`) stayed true enough to audit against; it is the cheapest guard against drift.
