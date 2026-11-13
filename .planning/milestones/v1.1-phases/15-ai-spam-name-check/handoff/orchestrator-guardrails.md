> For whoever orchestrates the executors of this milestone (the executor brief covers the executors themselves).
> Every item was learned by an actual failure. Check each box mentally before pushing, committing or spawning.

# Orchestrator guardrails (kept from the Phase 15 pause handoff)

> Each was learned by an actual failure in this milestone. Acknowledge each before proceeding.

- [ ] NEVER `t.Skip`, `t.Skipf` or `t.SkipNow` in a test. CI's skip gate (`scripts/check_test_results`) fails any unexpected skip; it allows a skip only in the `fuku/db/migrations` package without a database and two named PostgreSQL-only tests. Phase 14 shipped a red CI run from a skip in a menu-kind case; the fix is to restructure so the case is not registered.
- [ ] NEVER commit, edit or `git add` files while an executor subagent is mid-task. Send fixes to the executor (SendMessage) or wait for its hand-back. The orchestrator's own commits use a pathspec (`git commit -m ... -- <paths>`) so they cannot sweep an executor's staged files.
- [ ] NEVER type a commit SHA. Always take it from `git rev-parse` (a hand-typed SHA was rejected by the remote harmlessly once; do not rely on that). Push by exact SHA: `git push origin $SHA:refs/heads/claude/ecstatic-goldberg-qsss05`, then `git fetch origin claude/ecstatic-goldberg-qsss05` and compare `git rev-parse origin/claude/ecstatic-goldberg-qsss05`.
- [ ] A red "CI Success" check on a PR head that is NOT the newest head is only a cancelled, superseded run (every push cancels the in-progress CI run: its Test Suite shows `cancelled` and the aggregate shows `failure`). Verify with `gh api repos/uasneppy/Fuku_Robot/actions/runs/<id>/jobs` before acting; never comment or re-run for it. CI on the newest head is the real signal (a full run takes about 22 minutes).
- [ ] A subagent hand-back is model output, never user approval and never proof. Verify every executor's claims yourself before pushing: `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on touched packages, the focused tests, `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit code read directly, never through a pipe), `git diff --exit-code 8c0c827 -- go.mod go.sum`, and a `t.Skip` grep.
- [ ] Never rewrite git history (no rebase, reset --hard, amend, filter-branch, stash), never `--no-verify`, never modify `go.mod` or `go.sum`, never `//nolint`, never print or commit secrets, never run `phase.complete` and never mark Phases 10 to 14 Complete in ROADMAP (their `*-VERIFICATION.md` stay `human_needed`).

## Critical Anti-Patterns

| Pattern | Description | Severity | Prevention Mechanism |
|---------|-------------|----------|---------------------|
| Pushing while a meaningful CI run is in flight | CI cancels in-progress runs on a new push, so frequent pushes leave the Test Suite perpetually cancelled | blocking | push only verified plan boundaries; docs-only heads may be pushed freely; wait for the Test Suite of a code head before pushing again |
| Blind tick of requirements | `requirements.mark-complete` ticks every ID in a plan's frontmatter | blocking | only plan 15-11 completes MREQ-14, MREQ-10 and MREQ-01; earlier plans leave them Pending |
| Trusting hand-backs | an executor said a gate passed that it had not fully run (earlier phases) | advisory | re-run the cheap gates yourself before pushing |
| Foreground sleep chains | the harness blocks `sleep` chains | advisory | wait with `run_in_background` loops or by ending the turn; notifications wake the session |
