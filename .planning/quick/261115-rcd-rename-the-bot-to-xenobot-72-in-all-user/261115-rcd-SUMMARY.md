---
phase: 261115-rcd-rename-the-bot-to-xenobot-72-in-all-user
plan: 01
subsystem: text, packaging, docs
tags: [rename, locales, i18n, docs, github, oci-labels, backup]

requires:
  - phase: 261115-pvl
    provides: README rewrite and the follow-up list this task works from
provides:
  - Bot messages, help screens, /stats heading and new /export files say Xenobot-72
  - No link, handle or contact of the original developers in code, locales, packaging, GitHub files or docs
  - Docs index with the real figures (39 modules, 231 command names, 8 languages)
affects: [release notes, BotFather profile, README follow-ups]

actuals:
  tokens: 54000
  tasks: 3
  commits: 3

key-files:
  created: []
  modified:
    - locales/*.yml (8 files)
    - fuku/modules/help.go, language.go, bot_updates.go and their tests
    - fuku/db/backup/types.go, fuku/db/devs/repository.go and tests
    - .goreleaser.yaml, docker/alpine, docker/alpine.debug, docker/pr-build, .github files, scripts
    - docs/** (33 files in Task 3)
  deleted: [.github/FUNDING.yml, .github/CODEOWNERS]

key-decisions:
  - "Visible text only (O-01); identifiers, module path, image and env names unchanged"
  - "Original developers' links removed, never replaced (O-02); no username or link invented"
  - "/donate stays and says there is no donation page"
  - "app.json keeps its Heroku-safe lowercase name"
  - "Docs index figures come from the dispatcher dump (231) and the registration grep (39), not from the generator"

requirements-completed: [QUICK-261115-rcd]

plan_head_before: a8e25413c10dcd44bd8bcbc25f0c76b4eb82495e
plan_head_after: 61a3401081c92e2c873ed3c79c5176b4115bb2cb
duration: n/a
completed: 2026-11-15
status: complete
---

# Quick 261115-rcd: rename the bot to Xenobot-72 outside the READMEs Summary

**Visible text renamed to Xenobot-72 across locales, help screens, /stats, new exports, image labels, GitHub files and the docs site, with every original-developer link removed and no identifier changed.**

## Commits

- a721b5a `fix: rename the bot to Xenobot-72 in its messages and drop the original developers' links`
- 98d363d `fix: rename the image labels and GitHub files to Xenobot-72 and drop the original developer's metadata`
- 61a3401 `docs: rename the docs site to Xenobot-72 and remove the original developers' links`

`git rev-list --count a8e2541..HEAD` is 3. None touches `.planning/`. Nothing is pushed.

## What was built

- Task 1 (code and locales): five link-only locale keys and their readers removed from all 8 locale files; the /start and /about keyboards lost the News and Support buttons, /start the news line, the thanks message the creators plug, /lang the translate row. Old-name text swapped in all 8 files, help_about lost the credits and the history claim, /donate says there is no donation page, captcha help names /captchamaxattempts in seven languages (es has no such line). Developer /stats heading and new export `bot_name` say Xenobot-72; `Validate` still checks only for a non-empty name, and tests prove old-name exports still validate, parse and import.
- Task 2 (packaging and GitHub): OCI authors, vendor and url labels gone, titles and descriptions renamed; FUNDING.yml and CODEOWNERS deleted; dependabot reviewers and the issue-template contact link removed; script and comment text renamed; AGENTS.md line 3 and CLAUDE.md say Xenobot-72 with no fork claim. `.planning/PROJECT.md` has the same edit and is left uncommitted.
- Task 3 (docs): 33 hand-maintained pages renamed; support group, updates channel, Crowdin, live-docs host and old hosted username links and instructions removed; `docs/blume.config.ts` points at this repository's own owner; /about and /donate rows match the new help text; figures 39, 231 and 8 in the docs index, config, landing page and the frozen api-reference Overview; every language list and locale tree names all 8 (uk.yml added).

## Verification (everything except `make test` and `make lint`)

- Checker: locales 0 failures (217 before), prose over docs 0 hits (94 before), counts over docs 0 hits (17 before). Stale-figure grep: 7 lines before, none now.
- Dispatcher dump: COUNT 231, identical to the README task's dump; 39 `RegisterLegacyModule` calls outside tests; 8 locale languages.
- `make generate-docs` changes nothing; `make check-docs`: no drift; `make check-translations`: pass.
- Docs site: `bun install --frozen-lockfile` and `blume build` in a scratchpad copy: 63 pages, exit 0; built output has none of the removed names or links.
- Tasks 1 and 2: go build, focused and package tests with -race, go vet with testtools, YAML/JSON parse, `bash -n`, `docker compose config`, identifier greps (see the task notes).
- Pattern greps over the repository (minus .planning, migrations, LICENSE, READMEs, GSD tooling) for the original developers and for the old name in prose: empty outside tests and class (a) identifiers.
- README.md, README.uk.md, LICENSE, migrations, go.mod, go.sum, app.json: byte-identical to the base commit. `git status`: only `.planning/PROJECT.md` modified, plus the untracked quick-task files.

## Deviations from the plan

None that needed a rule. Judgement calls (class d), all in the notes: reworded help_help_msg and help_extended_docs and their docs rows because Task 1 made them false; en and uk drop the sparkle after the name; fr and ru adjusted for elision; `.golangci.yml` comment renamed (not in the file list); module-pattern.md locale sentence gained uk.yml (not named in the plan, flagged by the counts checker); the checker's fence stripper now accepts indented fences, and the compose service name in docs prose was backticked rather than hidden; the frozen api-reference description was reworded so "Xenobot-72 commands" does not read as a count of 72.

## Not done, and why

- Full `make test`: the orchestrator runs it, by instruction.
- `make lint`: cannot run in this container (golangci-lint is built with go1.25, the repo targets Go 1.26.0). `go vet -tags testtools` over the touched Go packages stood in; Task 3 changed no Go.
- Docs per-module cards, badges and tables left as they are: they are hand lists that undercount (27 cards sum to 159; tables hold 162 and 161 rows; 34 module directories against 39 modules), as are the generator and `make inventory` counts.
- app.json, README.md, README.uk.md, LICENSE, migrations, go.mod, go.sum: untouched by rule.
- The BotFather steps (name, username, description, picture) are the owner's; no username or link was invented.

## Known Stubs

None.

## Left for the orchestrator

`.planning/PROJECT.md` is edited and uncommitted on purpose; commit it with the quick-task files (explicit paths). Run `make test` once.

## Self-Check: PASSED

Commits a721b5a, 98d363d and 61a3401 are ancestors of HEAD; the SUMMARY, notes and checker files exist in the quick directory.

## Orchestrator: full test suite and late fixes

- **`make test` (run once by the orchestrator at HEAD 61a3401 plus the docs figure fix, about 33 minutes):** all 69 test packages report `ok`, 0 `--- FAIL`, coverage 78.8% (CI floor 78). `make` still exited 2, because the suite's result checker (`scripts/check_test_results`) counted one `fail` event with an empty package and test name: `go: no such tool "covdata"` while compiling `fuku/utils/actionlog`, a package with no test files, under `-coverpkg`. That is the container's Go toolchain (go1.26.0 without the `covdata` tool), reproduced on its own with a one-package coverage run on the untouched `actionlog` package; it is not caused by this task and does not occur on the CI runners. No test failed, and no unexpected skip was reported.
- **`make lint`** cannot run here (golangci-lint is built with go1.25); the local lint stand-in (dupl, gocyclo, godox, staticcheck, errcheck, qf checks) reported no findings on the Go lines added since the base. CI runs the real `make lint`.
- **Late fix `c662a52`:** the verifier found three stale tiles on the docs landing page (`docs/pages/index.astro`: 29+, 158+, 7+); they now read 39, 231 and 8. See 261115-rcd-VERIFICATION.md for the other gaps and their resolution.
- **Commits (base a8e2541):** `a721b5a` (fix, locales and code), `98d363d` (fix, packaging and GitHub files), `61a3401` (docs, docs site), `c662a52` (docs, landing page figures), plus the docs commit with these task notes and `.planning/PROJECT.md`.
