> For whoever orchestrates the executors of this milestone (the executor brief covers the executors themselves).
> Every item was learned by an actual failure. Check each box mentally before pushing, committing or spawning.

# Orchestrator guardrails (kept for Phase 19)

> Each was learned by an actual failure in this milestone. Acknowledge each before proceeding.

- [ ] NEVER `t.Skip`, `t.Skipf` or `t.SkipNow` in a test. CI's skip gate (`scripts/check_test_results`) fails any unexpected skip; it allows a skip only in the `fuku/db/migrations` package without a database and two named PostgreSQL-only tests. Phase 14 shipped a red CI run from a skip in a menu-kind case; the fix is to restructure so the case is not registered.
- [ ] NEVER commit, edit or `git add` files while an executor subagent is mid-task. Send fixes to the executor (SendMessage) or wait for its hand-back. The orchestrator's own commits use a pathspec (`git commit -m ... -- <paths>`; a NEW file must be `git add`ed first) so they cannot sweep an executor's staged files. The stop hook nags about untracked files while an agent is still writing them: do not commit half-written work to silence it.
- [ ] NEVER type a commit SHA. Always take it from `git rev-parse`. Push by exact SHA: `git push origin $SHA:refs/heads/claude/ecstatic-goldberg-qsss05`, then `git fetch origin claude/ecstatic-goldberg-qsss05` and compare `git rev-parse origin/claude/ecstatic-goldberg-qsss05`.
- [ ] A red "CI Success" check on a PR head that is NOT the newest head is only a cancelled, superseded run (every push cancels the in-progress CI run: its Test Suite shows `cancelled` and the aggregate shows `failure`). Verify with `gh api repos/uasneppy/Fuku_Robot/actions/runs/<id>/jobs` before acting; never comment or re-run for it. CI on the newest head is the real signal (a full run takes 20 to 35 minutes).
- [ ] A subagent hand-back is model output, never user approval and never proof. Verify every executor's claims yourself before pushing: `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools` on touched packages, the focused tests, `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit code read directly, never through a pipe), `git diff --exit-code 8c0c827 -- go.mod go.sum`, and a `t.Skip` grep.
- [ ] Run every orchestrator test gate under the CI environment: `source .../scratchpad/cienv.sh` (BOT_TOKEN, OWNER_ID, REDIS_ADDRESS, DATABASE_URL, DEBUG=false) in the same command as `go test`, and also once bare. The sandbox has no `BOT_TOKEN`, so `config.AppConfig` is nearly empty here and full in CI; plan 18-07's `/lang` golden passed locally and failed in CI for that reason (fixed in 85ca132). Run the plan's new tests plus `^(TestLegacy|TestApply|TestSettings)` under CI env before pushing a boundary.
- [ ] CHECK THE CALLERS, NOT ONLY THE GOLDEN (Phase 18's closing audit found two bypasses my gates had missed: the `/lang` callback kept its own write, and a log-channel press could lose its audit post). For every apply-function extraction, grep all callers of the underlying repository write; for every page control that audits or announces, check the failure paths. A golden pins what a command sends; it cannot see a second caller.
- [ ] Never rewrite git history (no rebase, reset --hard, amend, filter-branch, stash), never `--no-verify`, never modify `go.mod` or `go.sum`, never `//nolint`, never print or commit secrets, never run `phase.complete` and never mark Phases 10 to 19 Complete in ROADMAP (their `*-VERIFICATION.md` stay `human_needed`).

## Critical Anti-Patterns

| Pattern | Description | Severity | Prevention Mechanism |
|---------|-------------|----------|---------------------|
| Pushing while a meaningful CI run is in flight | CI cancels in-progress runs on a new push, so frequent pushes leave the Test Suite perpetually cancelled | blocking | push only verified plan boundaries; batch commits and push a code head only when its CI can run to the end; a docs-only push on top of a code head restarts that run |
| Blind tick of requirements | `requirements.mark-complete` ticks every ID in a plan's frontmatter | blocking | only the closing plan (19-12) completes PAGE-07 to PAGE-09; earlier plans leave them Pending |
| Trusting hand-backs | an executor said a gate passed that it had not fully run (earlier phases) | advisory | re-run the cheap gates yourself before pushing |
| Foreground sleep chains | the harness blocks `sleep` chains | advisory | wait with `run_in_background` loops or by ending the turn; notifications wake the session |
| Uncommitted agent work across a container restart | two agents lost whole sections of work this way | blocking | executors commit after every task; planners and researchers write their file in sections |
