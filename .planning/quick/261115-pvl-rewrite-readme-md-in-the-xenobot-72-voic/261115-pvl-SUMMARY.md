---
phase: 261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic
plan: 01
subsystem: docs
tags: [readme, documentation, i18n, voice, ukrainian]

requires: []
provides:
  - "README.md: current English README in the Xenobot-72 voice, written from verified facts"
  - "README.uk.md: original Ukrainian README with the same structure, commands and variables"
  - "Fact sheet, command-registry dump and README checker in the quick task directory"
affects: [documentation, follow-up rename of locales, docs and deploy files]

actuals:
  tokens: 50062
  tasks: 3
  commits: 3

plan_head_before: 83dd98e7a2f86fff8d81646e509a1d680294e392
plan_head_after: 83327ab9e552d4e575b44a0e5c5aa9c6334e92c6

tech-stack:
  added: []
  patterns:
    - "Documentation gate: a python3 checker (names, links and anchors, html, tables, hygiene, figures, command coverage, Ukrainian rules, parity) backed by a fact sheet"

key-files:
  created:
    - README.uk.md
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py
  modified:
    - README.md

key-decisions:
  - "Code blocks keep their English comments in both files (byte-identical, parity depends on it)"
  - "Commands section holds one series reference: Rules facility line kept, repeated tempe line dropped from Greetings in both files"
  - "Plain-text headings, no emoji, so GitHub anchors stay robust (Cyrillic slugs in the Ukrainian file)"
  - "License section points to LICENSE and names no holder"

requirements-completed: [QUICK-261115-pvl]

duration: 3 tasks, three executors
completed: 2026-11-15
status: complete
---

# Quick task 261115-pvl: Rewrite README.md in the Xenobot-72 voice and add README.uk.md

**Two current READMEs, English and original Ukrainian, written from a fact sheet checked against the code at release v2.24.0, with all 231 registered command names covered and no trace of the old name or the original developers in prose.**

## Commits

| Task | Commit | Files |
| --- | --- | --- |
| 1: checker, fact sheet, English README | `6733c61` `docs: rewrite README in the Xenobot-72 voice` | `README.md` |
| 2: Ukrainian README | `563dba0` `docs: add original Ukrainian README` | `README.uk.md` |
| 3: cross-file fix-up | `83327ab` `docs: fix README series references and one Ukrainian line` | `README.md`, `README.uk.md` |

Base `83dd98e`. `git rev-list --count 83dd98e..HEAD` is 3 and `git diff --name-only 83dd98e..HEAD -- . ':!.planning'` lists exactly `README.md` and `README.uk.md`. Nothing was pushed, no branch was created. The quick directory (fact sheet, command dump, checker, four notes files and this summary) is uncommitted for the orchestrator.

## What was built

- `README.md` (1052 lines): centred header, in-voice introduction, table of contents, Features (Moderation, Protection, Staff Group with the forward blocklist, Lockdown and automatic raid detection, Member requirements, Message deletion, Settings menu, Content, Several groups, Languages and backup, Operations), Commands (30 collapsible modules covering all 231 registered command names, a Retired block for `/antiraid`, `/raidtime`, `/raidactiontime`), Architecture (mermaid flowchart with the real handler groups, bullets and a group table), Tech stack, Getting started (BotFather, Docker Compose, debug stack, source, Turnstile in six steps, other targets, webhook or polling), Configuration (every variable the code reads, grouped, defaults verified), Endpoints, Project structure, Development, Documentation, License.
- `README.uk.md` (1052 lines): original Ukrainian text, same H2 and H3 order, same 39 `<details>` blocks, tables, nine fenced blocks (byte-identical), link targets, commands and environment names; prose 104 percent of the English length.
- Quick directory: `261115-pvl-FACTS.md` (sections A to J: identity, counts, command inventory, environment, 35 behaviour claims, stale-claims register, stated figures, links, name search, voice review, red baseline, follow-up list), `261115-pvl-commands.txt` (the registry oracle, COUNT 231) and `261115-pvl-check_readme.py` (stdlib checker with `--parity`).

## Checks run and results

| Check | Result |
| --- | --- |
| Checker on the OLD README (red baseline) | exit 1, `fails=99` (original-project names, 13 missing registry commands, missing sibling link, 6 unbacked numbers) |
| `check_readme.py README.md README.uk.md` (final, after fix-up) | exit 0, `fails=0`; REVIEW lines judged (spelled-out counts are verified; two Ukrainian masculine-form hits are false positives) |
| `check_readme.py --parity README.md README.uk.md` | exit 0: 40 headings with the same levels, 68 environment names in both, prose ratio 104 percent |
| `make check-docs` | "No drift detected" |
| Registry oracle re-run in Task 3 (throwaway tests in the scratchpad, deleted, `git status -- fuku` empty) | identical to the committed dump, COUNT 231 |
| Counts re-run: modules 39, migrations 64, languages 8 plus `config.yml`, env names 57, version 2.24.0, Go 1.26.0 | all match the Stated figures |
| Name search (Python and grep) | 0 hits of the original project, developers, `upstream`, `fork`, Ukrainian equivalents; `fuku` only inside code, URLs and targets (31 hits per file), 0 in prose |
| Links | every relative link and anchor resolves; external hosts github.com, gorm.io, img.shields.io, t.me, useblume.dev; none is the original project |
| Spot check of 17 behaviour claims re-opened at file:line (defaults, raid thresholds and windows, ban span, card lifetime, caps, cooldowns, shutdown and CI facts) | no drift |
| Turnstile and forward blocklist paragraphs | present with all required statements, both files |

## Deviations and decisions

All four earlier notes files were read (`261115-pvl-task1a-notes.md`, `-task1b-notes.md`, `-task2-notes.md`, `-task3-notes.md`).

- Task 1 (plan said four missing blocklist commands): the checker scopes command coverage to the Commands section, so the old README showed 13 missing names, not four. Consistent, not a defect.
- Checker additions beyond the plan: per-file sibling-link check, the pipe-in-code-span rule (a literal `|` in a table cell must be written `\|`), figures test also reads `alt` and `title` text, parity ignores the two cross-links.
- Task 1 content choices: the forward blocklist is a bold run-in paragraph in the Staff Group section; Lockdown appears under Features and under Commands; the handler groups are a table; `MESSAGE_DUMP` is described as "chat or channel ID" because the code rejects only 0; `/captchamaxattempts` is used, not the help text's `/captchaattempts`; no page count for the settings menu; PaaS manifests are linked without commentary; the `typesafe.ai` link is not used; permissions marked T in the fact sheet (greetings, rules, reactions, federations, connections, disabling) use only the shipped help text's words.
- Task 2: code blocks stay byte-identical with English comments; badge alt texts translated; `Xenobot-72` never declined; Ukrainian "Конфігурація" (not "Налаштування") to avoid a clash with the settings menu; one calque-regex false hit reworded.
- Task 3 fix-up (decision 1, applied per H2): Commands held two series references, so the repeated tempe line in Greetings was replaced with a plain line in both files and the facility line in Rules was kept; one Ukrainian row (`/reports block`) was reworded because it read as "block reports for users". Accepted choices: English comments inside code blocks stay in both files; "supervisor" and «наглядача» in the Admin line stay (not a series reference a reader must know); the License section is unchanged and `LICENSE` untouched.
- No other deviation from the plan. No auth gates, no stubs.

## What was left out and why

- Claims that could not be verified: `/staff` has no member check of its own, so no admin check is stated; settings menu page count; the live Telegram behaviour that a hidden profile photo reads as missing (stated only as "no profile photo that I can see"); the `typesafe.ai` link and the badge URLs were not fetched.
- Dropped stale claims from the old README: main is the original code unchanged, 30 modules and 200 triggers, the live raid mode and its timings, 42 migrations, the anti-raid handler group, the "stock image is another project's" advice, the development-branch roadmap and sync instructions, all credit text, `make inventory` (it undercounts), the Cloudflare Workers docs claim, the privacy-mode statement.
- Nothing outside the two READMEs was edited: locales, docs, code, `AGENTS.md`, `.claude/CLAUDE.md`, deploy files and `LICENSE` are as they were.

## Follow-up list (places that still show the old name or the original developers)

Details with line numbers are in FACTS section J. U marks what a bot user or the bot owner can see at run time.

- Locales (U): `@DivideProjects` and `@DivideProjectsBot` in the about, donate and creators-plug texts, 3 hits in each of en, es, fr, id, pt, ru, uk and 2 in hi; the old bot name 14 to 18 times per language (132 in all) in `help_about`, `help_bot_intro`, `formatting_help_msg`, the configuration steps, the "Add Fuku to chat!" button, the news channel link `t.me/FukuRobotUpdates` and a Crowdin link. The en about text also claims "online since 2020" and "thousands of groups".
- `fuku/modules/help.go` (U): the News button URL (line 97) and the add-to-chat button (lines 466 and 471). `fuku/db/devs/repository.go:203` (U for the owner): the developer `/stats` heading "Fuku's Stats". `fuku/db/backup/types.go:33` (U): `BotName: "FukuRobot"` written into every `/export` file.
- `.goreleaser.yaml` (2 hits on the developer, 4 on the old name), `docker/alpine`, `docker/alpine.debug`, `docker/pr-build` (3 developer hits each: OCI authors, url, vendor), `docker/goreleaser` 2. Visible with `docker inspect`.
- `.github/CODEOWNERS` 1, `.github/FUNDING.yml` 2 (a Sponsor button on the repository page), `.github/dependabot.yml` 3 (assignees).
- Docs: 38 files with 364 old-name hits (largest: `self-hosting/binary.mdx` 79, `architecture/caching.md` 50, `source.mdx` 26, `docker.mdx` 25); `docs/pages/index.astro` (2, links `t.me/DivideProjects`, U on the docs site), `docs/blume.config.ts` (1), `docs/README.md` (the `fuku.divkix.me` live-docs link and old name). The docs pages linked from the READMEs still carry the old name. The generated docs index count ("30 modules, 159 commands") and `make inventory` both undercount.
- Deploy manifests: `docker-compose.yml` 12, `debug.docker-compose.yml` 5, `render.yaml` 6, `Procfile` 1 (`bin/Fuku_Robot`), `app.json` 2, `nixpacks.toml` 2, `sample.env` 2. Mostly identifiers; a rename needs the image tag and module path decided together.
- `AGENTS.md` (173 hits) and `.claude/CLAUDE.md` (39 hits; line 9 still says "fork"). Not user-visible.
- `migrations/20261004130000_add_staff_role_exclusivity_trigger.sql` has 4 developer-handle hits; an applied migration, must not be edited.
- BotFather steps for the owner: `/setname` (display name `Xenobot-72`), `/setusername` (decide first whether to change the username, because it changes every `t.me` link), `/setdescription`, `/setabouttext` (in the new voice, per language if wanted) and `/setuserpic`. The news channel `FukuRobotUpdates` also carries the old name.
- Shipped help text bug: the captcha help text says `/captchaattempts <1-10>` while the registered command is `/captchamaxattempts`; present in en, fr, hi, id, pt, ru and uk (es has neither spelling). The READMEs use the registered name.

## Self-Check: PASSED

- README.md, README.uk.md, FACTS, commands dump, checker and the four notes files exist.
- Commits `6733c61`, `563dba0` and `83327ab` are ancestors of HEAD (`git merge-base --is-ancestor` verified).
- Final checker, parity and `make check-docs` exit 0 at HEAD; `git status --porcelain` shows only the untracked quick directory.
