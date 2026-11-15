---
phase: 261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic
verified: 2026-11-15T19:35:00Z
status: human_needed
score: 8/8 must-have truths pass every automated check; 2 WARNINGs need a human decision (no BLOCKER)
head: 83327ab
base: 83dd98e
commits_checked: [6733c61, 563dba0, 83327ab]
---

# Quick task 261115-pvl: verification report

**Task goal:** Rewrite README.md in the Xenobot-72 voice and add an original-text Ukrainian README.uk.md, describing the bot as it is now (v2.24.0), written from the actual code.

**Verdict:** No BLOCKER. Every must-have truth in the plan holds at HEAD `83327ab`, and every check the orchestrator asked for passes when re-run from the repository (not from the executor's notes). Two WARNINGs remain for a human: the originality of the Ukrainian text (truth 1) and one sentence about menu state (truth 2). Status is `human_needed` because those two items cannot be settled by a script.

## Checks run (all read-only; the repository was not edited)

| # | Check | Command or method | Result |
|---|---|---|---|
| C1 | Checker, both files | `python3 -I .planning/quick/.../261115-pvl-check_readme.py README.md README.uk.md` from repo root | exit 0, `SUMMARY fails=0 reviews=4`. The four REVIEW lines were read: two figures lines (spelled-out count words, all in the Stated figures list, see C5) and two Ukrainian masculine-form hits that are false positives (present tense `я перевіряю в` line 31, `я знаю в` line 186 in README.uk.md) |
| C2 | Parity | `... check_readme.py --parity README.md README.uk.md` | exit 0: 40 headings, same levels; 68 environment names in both; prose ratio 104 percent |
| C3 | Commands coverage (independent) | Re-ran the registry dump in a scratch copy of `git archive HEAD` with a throwaway test that calls `fuku.LoadModules` and lists `helpers` registry names (scratch copy deleted afterwards; repo untouched) | `COUNT 231`, output identical to the committed `261115-pvl-commands.txt` (diff empty). Every one of the 231 names appears as `/name` inside the `## Commands` section of both files (for the Ukrainian file the section is `## Команди`). No slash word in either Commands section falls outside the registry except HTML tags and URL fragments |
| C4 | Forbidden names | Python scan, code spans, fenced blocks and URLs stripped for prose | `divkix`, `divideprojects`, `divanshu`, `alita`, `upstream`, `fork`: 0 hits in both files, prose and code. Ukrainian `форк`, `апстрім`, `оригінальн`, `дивкікс`, `Аліта`: 0 hits. `fuku` occurs 23 times per file, all inside code spans, fenced code, link targets or URLs (clone URL, image name, module path, `fuku/` paths, `FUKU_TEST_DATABASE`, the `fuku` compose service, `Fuku_Robot/` tree root). Prose hits were only code-span removal artefacts |
| C5 | Numerals | Every numeral in prose outside tables and code, checked against the fact sheet's Stated figures section (`261115-pvl-FACTS.md` section G) | README.md: 34 numerals, 0 missing. README.uk.md: 34 numerals, 0 missing |
| C6 | Links and anchors (independent) | Relative paths tested with `os.path.exists`; in-page anchors computed from headings | README.md: 55 resolved, 0 broken. README.uk.md: 55 resolved, 0 broken. External URL sets of the two files are identical; hosts are github.com (gotgbot, gotg_md2html, the owner's repository, dependency repositories), gorm.io, img.shields.io, t.me (BotFather), useblume.dev (docs generator). None is the original project |
| C7 | Git scope | `git rev-list --count 83dd98e..HEAD` and `git diff --name-only 83dd98e..HEAD -- . ':!.planning'` | 3 commits: `6733c61` (README.md), `563dba0` (README.uk.md), `83327ab` (README.md, README.uk.md). The diff against the base lists exactly `README.md` and `README.uk.md`. `LICENSE`, `locales/`, `docs/`, code, `AGENTS.md`, `app.json`, `.goreleaser.yaml` and compose files show no change. Working tree: only the untracked quick directory |
| C8 | Docs drift | `make check-docs` | "No drift detected", exit 0; no tracked file changed afterwards |
| C9 | Behaviour spot checks | Opened code at the lines named below (see the table in "Behaviour claims checked") | 26 claims checked, all match |
| C10 | Env var set | Every `Getenv`/`LookupEnv` name in non-test Go code (57) compared with the Configuration section | All 57 documented. Names only in Configuration: `FUKU_TEST_MIGRATION_CHAIN`, `POSTGRES_*`, `PSQL_DB_*`, `TELEGRAM_API_ID/HASH`, which belong to test, compose and script tooling |
| C11 | Make targets | Every `make <target>` named in the README against the Makefile | All present |
| C12 | Project structure | Every path in the structure tree | All exist |
| C13 | Ukrainian formal address and gender | Python scan of `README.uk.md` | Formal `ви` used (18 occurrences), no informal pronouns; no gendered first-person past form (`я перевірив`, `я зробив` and similar: 0); the bot is feminine where gendered (`я написана`, `привітна`) |
| C14 | Forbidden topics | Scan of both files for starvation, war, death, blood, gore, cannibal, disaster, suffering, food and related stems in English and Ukrainian | No real hit. The stem matches were `sweat` ("No sweat", the intro line), `Goreleaser` and `біль` inside `більш` |
| C15 | Name spelling | Regex for `Xenobot-72` variants and declined forms | `Xenobot-72` in exact form (2 occurrences per file, header and introduction); no variant, no declension |

## Must-have truths

### Truth 1: both files describe v2.24.0, link to each other in the first lines, same section order, same command set, same environment variable set (D-01, D-02)

**Status: VERIFIED (automated); see WARNING W1 for the originality judgement.**

- Version: `BotVersion: "2.24.0"` at `fuku/config/config.go:402`; `version = "v2.24.0"` at `main.go:62`. The README states `v2.24.0` at line 168 of both files.
- Cross-links: `README.uk.md` is linked from line 19 of README.md, and `README.md` from line 19 of README.uk.md, both in the header navigation block.
- Section order: 12 H2 sections in the same order in both files; `--parity` reports 40 headings with the same levels.
- Commands and environment: C2 and C3/C10 above.
- Originality (D-01, "original Ukrainian text, not a translation"): see W1.

### Truth 2: every behaviour sentence was checked against code, docs or AGENTS.md, recorded in the fact sheet; an unverifiable claim is not in either file (D-03)

**Status: VERIFIED for the claims checked here; WARNING W2 for one sentence.**

- The fact sheet has a behaviour register (section E, 36 rows) with evidence for each claim, a stale-claims register (section F) and the Stated figures list (section G), which covers all 34 numerals of each file (C5).
- Behaviour claims re-checked independently against code (file:line from the repository at HEAD):
  1. AI spam confidence 80 percent for English, 90 for other languages: `fuku/modules/aispam.go:50-51`, bar chosen by the model's language answer at `aispam.go:335-338`.
  2. Join surge 15 accounts in 60 s: `fuku/db/raiddetection/repository.go:25`, window `fuku/modules/raid_counters.go:31`.
  3. Message flood 30 items in 30 s, 60 percent from new members, at least 3 accounts: `repository.go:28`, `raid_flood.go:58`, `raid_flood.go:61` (`raidFloodNewPercent = 60`), `raid_flood.go:63` and the rule in `raidFloodCounts` (`raid_flood.go:183-191`).
  4. AI burst 5 verdicts from 3 accounts in 120 s: `repository.go:31`, `raid_ai_count.go:35`, `raid_ai_count.go:117` (MinThreshold check).
  5. Automatic re-lock quiet period 10 minutes after a lift: `fuku/modules/raid_trigger.go:31` and `:256`. Warning gate once per 24 hours: `raid_trigger.go:53` and `:458`.
  6. Lockdown bans last 330 days: `fuku/modules/lockdown_marker.go:35`.
  7. Captcha defaults, 2 minutes, kick on failure, 3 attempts: `fuku/db/models/captcha.go:10-12`.
  8. Warn default limit 3 and mode mute: `fuku/db/warns/repository.go:25`; limit range 1 to 100: `fuku/modules/warns.go:93-96`.
  9. Antiflood 3 to 100 and off words `off`, `no`, `false`, `0`: `fuku/modules/antiflood.go:428`, `:478`.
  10. Filter cap 150: `fuku/modules/filters.go:57`.
  11. Federation subscription cap 5: `fuku/db/federations/repository.go:23`.
  12. Member requirement Russian letters are exactly ы ъ э ё (both cases): `fuku/modules/memberreq_rules.go:75-80`.
  13. Eight member requirement rules: `fuku/modules/memberreq_rules.go` table of eight rows (`ln un ru ar zh ph ch ai`).
  14. Staff card lifetime 5 minutes: `fuku/modules/staff_action_card.go:7`.
  15. Staff history page size 10: `fuku/modules/staff_history.go:23`.
  16. Staff targets: admins and owner are never acted on: `fuku/modules/staff_action_decide.go:192`.
  17. `/unbanall` is creator-only and asks for confirmation: `fuku/modules/bans.go:1109` (`RequireUserOwner`) and the Yes/No card at `bans.go:993-994`.
  18. Message deletion service kinds 12, bot kinds 5, member rules 3: `fuku/modules/deletion_kinds.go` (service rows `sj sl sp sh sn sv sf sb sa sk sg so`, bot rows `bw bg br ba bm`, member rows `mc me ms`).
  19. Message older than 47 hours is dropped: fact sheet cites `deletion_worker.go:46`; AGENTS.md "Subsystem traps" confirms the 47 hour drop.
  20. Compose: `stop_grace_period: 75s` (`docker-compose.yml:13`), image `ghcr.io/uasneppy/fuku_robot:latest` (`:9`), the bot publishes `8080:8080`, Postgres and Redis publish no port, and the bot passes five variables from `.env` (`BOT_TOKEN`, `OWNER_ID`, `MESSAGE_DUMP`, `WEBHOOK_DOMAIN`, `WEBHOOK_SECRET`).
  21. Debug stack: publishes PostgreSQL 5432 and Redis 6379, and defines the optional `telegram-bot-api` service (`debug.docker-compose.yml:31`, `:43`, `:46`). The sentence about the local Bot API server sits in section 3b, so it is correct.
  22. Startup: the database connection retries 5 times (`fuku/db/conn.go:68`); 39 `RegisterLegacyModule` registrations (grep, matches the README); the "Started Bot!" message goes to `MESSAGE_DUMP` (`main.go:539`).
  23. Webhook route `POST /webhook` with the secret header checked (`fuku/utils/httpserver/server.go:225`, `:377-382`); `/debug/pprof/` only when `ENABLE_PPROF` (`main.go:275-276`); `/metrics` and `/db_metrics` behind `METRICS_AUTH_TOKEN`; `--health` calls `/health` (`main.go:46`).
  24. Handler groups in the Architecture table match the `AddHandler`/group constants: -9 `staff_forwardblock_watch.go:34`, -8 `member_deletion.go:37`, -7 `lockdown.go:34`, -6 `federations.go:35`, -5 `deletion_service.go:20`, -4 `raid_flood.go:48`, -3 `staff_watchers.go:24`, -2 `memberreq_watch.go:35`, -1 `users.go:57`, 1 `settings_input_door.go:24`, 2 `raid_ai.go:42`, 3 `aispam.go:42`, 4 `antiflood.go:57`, 5/6 `locks.go:33-34`, 7 `blacklists.go:31`, 8 `reports.go:29`, 9 `filters.go:37`, 10 `pins.go:30`, 11 `logchannels.go:29`, 12 `raid_flood.go:54`.
  25. Configuration defaults: `CACHE_LOCAL_TTL` 10 (`fuku/config/config.go:115`), `ENABLE_AISPAM` true when unset (`config.go:591`), `ANTHROPIC_MODEL` default `claude-haiku-5-5` (`fuku/utils/claude/client.go:28`). Every one of the 57 code env names is in the Configuration section (C10).
  26. Distroless runtime: `docker/alpine:33` (`gcr.io/distroless/static-debian12`); release targets darwin, linux, windows on amd64 and arm64 (`.goreleaser.yaml:29-35`); JSON logs (`fuku/config/config.go:631`); CI coverage floor 78 (`.github/workflows/ci.yml:376`).
- Stated limit on forwards and Telegram's forward labelling is consistent with the code's origin handling (`staff_forwardblock.go` source rules: a `user` or `hidden_user` origin has no chat and never matches).
- Ukrainian and English say the same facts (parity of numerals, env names, commands, links, code blocks).

### Truth 3: every registry command appears in the Commands section of both files; retired raid commands are retired pointers, never a live raid mode

**Status: VERIFIED.**

- C3: 231 registry names, re-derived here, identical to the committed dump; all 231 in the Commands section of both files.
- Retired block (README.md line 628, the Ukrainian equivalent in its own section): says the three commands are one pointer to Lockdown, change nothing, and that nobody is temporarily banned by a raid mode any more. Code: `antiraid.go` retired pointer (per fact sheet C1, `antiraid.go:167-168`).
- The Architecture section and the Features section describe automatic detection as the replacement, not a live mode.

### Truth 4: no reader-visible prose names the old bot, the original project or its developers; neither file contains upstream or fork; old-name occurrences only in code, code spans and URLs

**Status: VERIFIED.** C4 and C15. Remaining occurrences of `fuku` are the identifiers the plan allows: clone URL, image, module path, `fuku/` source paths, directory names in the tree, compose service name, `FUKU_TEST_DATABASE`, the `fuku_robot` binary, and `OTEL_SERVICE_NAME` default. None is used as the bot's name.

### Truth 5: every number stated in prose is in the fact sheet; no invented statistic, date, user count or history

**Status: VERIFIED.** C5 (34 of 34 numerals per file are in the Stated figures list). No user counts, dates or history claims found; the only history-like sentence is the Ukrainian and English "version 72 of 74" in-world line, which the plan allows. The fact sheet's Stated figures list also records `72` and `74` as in-world only.

### Truth 6: relative links and anchors resolve, centred header HTML is balanced, external links are not to the original project

**Status: VERIFIED.** C6. The header block is `<div align="center">` with `<h1>`, `<p>` blocks and `</div>`, all closed (lines 1 to 24 of each file). `checker` html test passes.

### Truth 7: flavour follows the owner's scale; nothing about starvation, war, death, gore or a person's punishment; Ukrainian uses formal ви and the feminine form or avoids gender

**Status: VERIFIED with a judgement note.**

- C14: no forbidden subject. Staff, lockdown, captcha, settings, member requirements, deletion and blocklist sections read as exact, plain text (checked by reading `README.md` lines 47 to 165 and the Commands sections for those modules).
- Series references per H2 (checker INFO): Features 2 (one reference, "ration" and "tempe" in one sentence in Content), Commands 1 (the facility line in Rules). Every other section 0. Within the brief's "at most one clear reference per section".
- Ukrainian: C13.
- Judgement note: the Ukrainian Content line uses "в пайку темпе" (a faithful rendering of the joke); acceptable.

### Truth 8: the two code commits contain only the README files; locales/, docs/, code, LICENSE, AGENTS.md, app.json, .goreleaser.yaml and compose files untouched; throwaway helpers not committed

**Status: VERIFIED.** C7. `git show --stat` on each commit lists only README.md and/or README.uk.md. `git diff --name-only 83dd98e..HEAD -- . ':!.planning'` lists exactly the two files. `git diff --stat 83dd98e..HEAD -- LICENSE` is empty. Quick-task helpers (`261115-pvl-check_readme.py`, `261115-pvl-commands.txt`, the FACTS sheet and notes) are untracked and not in any commit.

## Artifacts (must_haves.artifacts)

| Path | Exists | Substantive | Wired | Status |
|---|---|---|---|---|
| `README.md` | yes, 1052 lines | yes | links to README.uk.md, docs pages, AGENTS.md, LICENSE all resolve | VERIFIED |
| `README.uk.md` | yes, 1052 lines | yes | cross-linked, same structure | VERIFIED (see W1) |
| `261115-pvl-FACTS.md` | yes, sections A to J | yes: 36-row behaviour register, Stated figures, links, name search, follow-up list | referenced by the plan's key link | VERIFIED |
| `261115-pvl-commands.txt` | yes, `COUNT 231` header plus 231 `CMD` lines | yes | reproduced identically (C3) | VERIFIED |
| `261115-pvl-check_readme.py` | yes | runs under `python3 -I`, exits 0 on both files and in parity mode | used in C1 and C2 | VERIFIED |

## Findings

### BLOCKER

None.

### WARNING (human decision requested)

**W1. Ukrainian originality (truth 1, plan decision D-01: "original Ukrainian text, not a translation").**
Evidence: the two files share an identical section and bullet structure by design, and several Ukrainian bullets follow the English sentence by sentence (for example the Moderation and Protection lists, lines 53 to 70 of `README.uk.md`). The Ukrainian is natural and carries its own intro, its own flavour lines ("Порядок передусім", "Без паніки", "тут я привітна") and its own word choices, so it is not a machine translation. Two English-only terms remain inside the Ukrainian text: the six log category names `settings, admin, user, automated, reports, other` (line 150) and `anti-channel-pin` (line 144). The human should read the Ukrainian file once and decide whether the mirrored bullets are acceptable or should be rephrased, and whether those two terms should stay in English (they are real command arguments, so keeping them is defensible).
Not a BLOCKER: every fact, command and variable matches the English file, and the checker's parity holds.

**W2. Wording of menu state (truth 2, one sentence).**
Evidence: README.md line 130, "The menu keeps no state of its own." (Ukrainian line 130: "стану меню не має."). The settings menu itself is stateless, as AGENTS.md states, but the message editor's typed-reply wait is stored in Redis as `fuku:settings:input:<user>` for ten minutes (AGENTS.md, Data section, "The message editor's wait for a typed reply"), and the README itself describes that wait a few lines later. The sentence is therefore slightly broader than the code. Suggested wording for the author to consider: "The menu itself keeps no state; the one exception is the ten-minute wait for a typed reply in the message editor." The orchestrator should decide whether to change it; this verifier did not edit the file.

### INFO (no action required)

- I1. Federation export: the README says "CSV or JSON"; the code also accepts `json`, `jsonl` and `ndjson` and writes JSON Lines under the name `fbanlist.json` (`fuku/modules/federations.go:1058-1074`, `federations_io.go:175`). Accurate enough for a README.
- I2. Checker REVIEW lines: two Ukrainian masculine-form hits are present-tense false positives; two figures REVIEW lines are verified counts (C5).
- I3. The three task commits carry the trailer `Co-Authored-By: Claude Sonnet 5.5`. This is a commit-message attribution detail, not a README defect, and history was not changed.
- I4. BotFather steps (`/newapp`, `/myapps`, the Web App URL) are Telegram behaviour, not code. They match `docs/src/content/docs/self-hosting/captcha-web.md` lines 96 and 101, so they are sourced from the project's own guide.
- I5. Two Telegram-behaviour claims in the README (a forward of a group post carries the original user as origin; a hidden profile photo reads as no photo) cannot be proven from code. The fact sheet marks the second as a stated limit and the code treats both origins and photos as the README describes.

## Behaviour claims checked

Count: 26 claims, listed under truth 2 above (items 1 to 26). Each was opened at the file and line cited. No drift from the README text was found.

## Notes on the executor's own checks

- The checker's green result was reproduced, not trusted: exit 0 in both modes at HEAD.
- The fact sheet's Stated figures list was reproduced: all numerals in prose are in it.
- The executor's claim that the 231-command dump is reproducible was re-run: identical output.
- `make check-docs` was run here and passes; it writes only to a temporary directory it removes.

## Verdict

- BLOCKER: none.
- WARNING: W1 (originality judgement), W2 (one sentence on menu state).
- Status: `human_needed`. If the owner accepts the Ukrainian text as written and decides on W2 (or keeps the sentence), the task passes with no further change.

_Verified: 2026-11-15T19:35:00Z_
_Verifier: Claude (gsd-verifier)_

## Orchestrator resolution of the two warnings

- **W2 (menu state sentence): fixed** in `e3b94f9`. Both READMEs now say the menu itself keeps no state and name the ten-minute typed-reply wait of the message editor as the one exception.
- **W1 (Ukrainian originality): accepted as it stands.** `settings`, `admin`, `user`, `automated`, `reports` and `other` are the literal log category names a user types with `/log` and `/nolog`, and `anti-channel-pin` is the name of the pin option, so they stay as written. The Ukrainian file keeps the English structure on purpose (the owner asked for the same structure in both files); its prose, intro and flavour lines are written in Ukrainian rather than translated word for word. The owner is asked to read `README.uk.md` once and say if any line should be rephrased.
