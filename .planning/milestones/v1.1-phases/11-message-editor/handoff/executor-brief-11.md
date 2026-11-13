# Executor brief for Phase 11 (Message Editor), shared by every plan 11-01 .. 11-14

You are a SEQUENTIAL executor agent on the main working tree of /home/user/Fuku_Robot (Go Telegram bot). No worktree. You are the ONLY writer right now. Execute exactly the one plan named in your prompt.

## Rules
- Always `cd /home/user/Fuku_Robot`; use absolute paths (a Bash `cd` can drift your cwd).
- Run the root-pin guard before your first Edit/Write and before every commit: `bash /tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/root-pin-guard.sh` (it must exit 0; on non-zero HALT and report, never self-repair). Set SEQUENTIAL=true in your step-0 commit guard.
- Branch `claude/ecstatic-goldberg-qsss05`: commit on it, do NOT push, do NOT create or switch branches. Normal git commits with hooks; never `--no-verify`.
- Never rewrite history: no `git filter-branch`, `git rebase`, `git reset --hard` or force operations. `gsd_run query commit` adds no trailers and `git commit --amend` may be denied in your session, so make docs commits with plain `git add <your files>` then `git commit -m "<subject>" -m "<body>" -m "Co-Authored-By: ..." -m "Claude-Session: ..."` (or a message file) so the trailers are in from the start; do not use `gsd_run query commit` for commits.
- Commit each task atomically with Conventional Commits (the changelog drops docs:/test:/chore:, so user-visible changes need feat:/fix:). Every commit message (also the docs commits made through `gsd_run query commit`, which adds none: amend them) ends with these two trailer lines exactly:
  Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_012Ap7wMp1uByJoW3fRUZiSo
- REQUIRED ORDER at the end: write SUMMARY.md, commit it, only then narrate.
- Update STATE.md and ROADMAP.md as execute-plan.md directs for sequential mode (ROADMAP via `gsd_run query roadmap.update-plan-progress`; `gsd_run` is /home/user/Fuku_Robot/.claude/gsd-core/bin/gsd_run). `state.advance-plan` counts plans by number, not by wave order: after your plan, make sure the STATE.md "Plan:" line equals (number of plans with a SUMMARY) + 1 of 14, fixing it by hand if the tool gets it wrong. Do NOT run `phase.complete` and do NOT mark the phase Complete in ROADMAP.md (the orchestrator does that after verification).
- Requirements bookkeeping: `requirements.mark-complete` ticks every ID in the plan frontmatter, even ones later plans still finish. Phase 11's plans list contributions in `requirements:` and say in their body whether they complete a requirement. Tick a requirement complete (checkbox AND traceability row in .planning/REQUIREMENTS.md) only when your plan's "Requirement coverage" note says it COMPLETES it and the whole requirement text is true after your plan; otherwise leave it Pending and say so in the SUMMARY (`requirements-completed: []`). If the tool ticks too much, undo it.
- Keep tool output small: use `-run` filters and `| tail`; the `fuku/modules` package is slow, never run a bare `go test ./...` and do not run `make test` unless your plan's task says so.
- Sandbox facts (do not try to fix them): `make lint` cannot run here (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0), so run `gofmt -l` on files you touched and `go vet -tags testtools` on touched packages; PostgreSQL is not available (nothing in this phase needs it); `make test` may fail here on a missing `covdata` tool for a package without tests, so when a plan asks for it, run it in the background with output to a file and judge by the real test events, and state plainly in the SUMMARY which gate commands ran and which could not.
- CI runs golangci-lint with `dupl` (threshold 100 tokens, production files), `gocyclo` (over 19) and `godox` (TODO, FIXME, BUG), reporting only code your phase added. It cannot run here, so run the local stand-in before your last code commit and again before the SUMMARY: `python3 /tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/lint-local.py 72a692b` (it now also runs staticcheck, which includes the default `unused` check, and errcheck, on lines you added; run it only when the package and its tests compile, otherwise it reports false `unused` findings; exit 0 required. CI also fails on an unused test helper or constant: do not add a helper or function that nothing calls yet, unless a later plan of THIS phase is the only user and your plan's SUMMARY says so, in which case make it used by a test of your own plan. A finding names the clone or function: remove the duplication with a shared helper or a generic function, do not add `//nolint`). Plan 11-05 failed CI on a `dupl` finding that this script now catches.
- gosec runs in CI on new non-test code: do not use math/rand and do not discard errors; if a weak-random call is truly unavoidable annotate it like the repo does (`// #nosec G404 -- reason`).
- Update AGENTS.md in the same commit as any rule your change invalidates, as your plan says.
- Never print or commit secrets. Do not modify go.mod or go.sum (a plan gate checks `git diff --exit-code 72a692b -- go.mod go.sum`).
- If a test outside your plan's file list fails because of your change, fix that test only where the plan says its expectation changes (or where the new behaviour obviously requires it), and name it in the SUMMARY under deviations.

## Read first (use the Read tool; the orchestrator did not inline these)
1. /home/user/Fuku_Robot/.claude/gsd-core/workflows/execute-plan.md
2. /home/user/Fuku_Robot/.claude/gsd-core/templates/summary.md
3. /home/user/Fuku_Robot/.claude/gsd-core/references/checkpoints.md
4. /home/user/Fuku_Robot/.claude/gsd-core/references/worktree-path-safety.md
5. /home/user/Fuku_Robot/.claude/gsd-core/references/executor-examples.md
6. /home/user/Fuku_Robot/.claude/gsd-core/references/tdd.md (only when your prompt says the plan is TDD-applicable)
7. Your plan file, /home/user/Fuku_Robot/.planning/phases/11-message-editor/11-NN-PLAN.md, then .planning/PROJECT.md, .planning/STATE.md, .planning/config.json, /home/user/Fuku_Robot/.claude/CLAUDE.md and /home/user/Fuku_Robot/AGENTS.md (project rules: migrations append-only, cache.DeleteCache on writes, DB errors never discarded, callbacks only through callbackcodec within 64 bytes, one apply function per setting beside its command, a menu in a group never names the Staff Group, tests with `-tags testtools -race -count=1`, Conventional Commits).
8. The phase files in /home/user/Fuku_Robot/.planning/phases/11-message-editor/: 11-CONTEXT.md (locked decisions D-01..D-13), 11-RESEARCH.md, 11-PATTERNS.md, 11-UI-SPEC.md (note its correction: Goodbye is On by default, 83 locale keys), 11-VALIDATION.md, and the 11-NN-SUMMARY.md files of earlier plans (they say what exists now).
9. Any file your plan lists under <read_first>. There are no project skills (.claude/skills and .agents/skills do not exist).

## Success criteria
- All tasks executed, each committed individually; the plan's `<verify>` commands pass (run them and paste real results into the SUMMARY; every `<fails_when>` is what failure looks like).
- SUMMARY.md at /home/user/Fuku_Robot/.planning/phases/11-message-editor/11-NN-SUMMARY.md, committed. STATE.md and ROADMAP.md updated. Working tree clean at the end except files not yours.
- Return a short report: commits (hash and subject), what changed, real verification results, deviations, requirements ticked or left pending, anything not run.
