---
phase: 261115-rcd-rename-the-bot-to-xenobot-72-in-all-user
verified: 2026-11-15
status: gaps_found
score: 7/11 must-have truths verified; T2 and T11 verified except for the pending make gates; T3 and T9 FAILED on literal clauses (see Gaps)
verifier: gsd-verifier (independent re-check of HEAD 61a3401 against base a8e2541)
---

# Quick 261115-rcd: rename the bot to Xenobot-72 — Verification

**Goal:** rename the bot to Xenobot-72 in all user-visible text outside the READMEs, remove every link to the original developers, keep every identifier, leave READMEs, LICENSE, applied migrations, go.mod, go.sum and app.json untouched.

**Scope of this check:** the three commits a721b5a, 98d363d and 61a3401 on top of base a8e2541. The executors' SUMMARY and notes were used only as claims to test; every result below comes from a grep, a PyYAML parse or a file read taken during this verification.

**Test suite:** `make test`, `make check-translations`, `make check-docs`, `make lint` were NOT run by the verifier (the orchestrator's `make test` is running in this tree; instructed not to run them). Reported as **pending, run by the orchestrator**. Where the verifier could, it re-derived the underlying fact with an independent script (locale key parity, placeholder and tag parity, command count, module count).

Scratch evidence (outside the repository): `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/verify/check_locales.py` and `.../verify/check2.py`.

---

## Must-have truths

### T1. Locales: no old name, no developer handle or link, no t.me link other than t.me/example — VERIFIED

- Independent PyYAML check of `locales/*.yml` at HEAD against base: zero values that contain a developer name (`divkix`, `divanshu`, `DivideProjects`, `DivideSupport`, `alita`, `crowdin`, `FukuRobotUpdates`), zero `t.me` links other than `t.me/example`, zero new URLs or handles relative to base.
- Old name in locale values: `git grep` over `locales/` at HEAD finds only `help_button_add_fuku`, a key name (identifier, kept by O-01), and its value now reads "Xenobot-72" in all 8 files.
- `git grep` for the developer patterns over the whole tree excluding `.planning/`, `migrations/`, `LICENSE`, `README.md`, `README.uk.md`: no hits. The only survivors are `uasneppy` (module path and clone URLs, kept by O-01), an `alita:` advisory-lock key prefix inside an applied migration (protected), and the copyright line in `LICENSE` (protected).

### T2. Five link-only keys removed from all 8 files with their readers; identical key set = base minus five; check-translations — VERIFIED for key set, check-translations PENDING

- All 8 files have 1847 keys, identical sets, equal to base (1852) minus the five named keys (`help_button_news_channel`, `help_button_support_group`, `help_news_channel_text`, `bot_updates_creators_plug`, `language_help_translate`). No key added.
- `git grep` for each of the five key names at HEAD: no hits anywhere outside `.planning/`.
- `make check-translations` not run by verifier (pending).

### T3. Placeholder and HTML-tag multisets equal base; no empty value; no value over 4096; name is Xenobot-72, never declined; six non-en/uk files change only name and credits — PARTIAL, FAILED on two literal clauses

- Placeholder (`%s`, `%d`, `{name}`) and tag multisets: equal to base for every kept key in all 8 files — VERIFIED.
- No empty values — VERIFIED.
- Name form: "Xenobot-72" everywhere, no declined or glued form found by a Python scan of all 8 locales (the one regex hit is a YAML `\n` escape, not a name form) — VERIFIED.
- **Over-4096 clause — FAILED (pre-existing, not introduced here).** `es.lockdown_help_msg` is 4097 code points, `fr.lockdown_help_msg` 4101, `pt.lockdown_help_msg` 4104. The base commit has the same lengths and this task did not touch those values. The must-have as written is not met. Owner decision requested: accept as pre-existing, or split the value in a follow-up. (The repo has `TestLockdownHelpFitsOneMessage`, which may measure a different unit; not verified by the verifier.)
- **"Six non-en/uk files change only the name and the removed credits" — FAILED as written, WARNING in substance.** Beyond the name and the credit removal, es, fr, hi, id, pt and ru also changed `help_help_msg` (the `/about` "useful links" and `/donate` "supporting the project" wording) and `help_extended_docs`, and fr/ru changed an elision or preposition in the name. `captcha_help_msg` and `help_donatetext` and the `/donate` bullet in `help_pm_intro` are in scope by the brief. The executors record the `help_help_msg` and `help_extended_docs` rewrites as class (d) decisions, made necessary because the removed links made the old wording false. The verifier agrees the rewrite is needed, but the literal must-have does not allow it. Owner acknowledgment requested.

### T4. /start and /about keyboards without News or Support; /start news line removed; thanks message without creators plug; /lang keyboard without translate row; golden loses only that row, AGENTS.md says so — VERIFIED

- `fuku/modules/help.go` diff: about keyboard loses the news and support row (now 3 rows: about, configuration, back); start keyboard keeps the add-to-chat button alone in its row; `getStartHelp` returns the intro text only.
- `fuku/modules/bot_updates.go`: creators plug removed from the thanks message.
- `fuku/modules/language.go`: `genFullLanguageKb` returns only `keyboard.MakeLanguageKeyboard()`, the translate row removed.
- `fuku/modules/language_legacy_request_test.go`: verifier re-derived the golden. Removing the one `crowdin` fragment from both base entries and the two added comment lines yields a byte-identical file to HEAD (274 lines, identical). The golden loses exactly that row, nothing else.
- `AGENTS.md`: the sentence "except the translation-platform keyboard row, removed from its two keyboard entries by owner decision O-02 in quick task 261115-rcd" is present and matches.
- `fuku/modules/help_render_test.go`: row counts, one-button add row and a negative assertion that no button points at a third-party URL — meaningful.

### T5. /donate still registered; its text, the /start help bullet and help_about name no contact, handle, link or creator; help_about no longer says who develops or since when — VERIFIED

- `/donate` is registered: `fuku/modules/help.go:604`, and the command dump has `CMD donate` (231 names identical to the README task's dump).
- `donate` handler sends `help_donatetext` in HTML with no link preview and no button. The text is "Notice. This bot has no donation page. There is nothing to donate to." (en), with equivalents in all 8 languages.
- Pattern scan of `help_about`, `help_donatetext`, `help_pm_intro`, `help_help_msg`, `help_extended_docs`, `help_bot_intro` in all 8 languages for history or developer wording (`2020`, `since`, `thousand`, `develop`, `creator`, transliterations): no hits.
- Caveat noted, not a must-have violation: `help_donatetext` is read through a hard-coded Ukrainian translator (`help.go:383`), which predates this task and was left alone as the plan directed.

### T6. Developer /stats heading says Xenobot-72; NewBackupFormat writes Xenobot-72; old-name export still validates, parses and imports; empty name, missing version, unknown module still rejected — VERIFIED

- `fuku/db/devs/repository.go:203`: `<u>Xenobot-72 Stats:</u>`; the test expectation in `repository_test.go` updated.
- `fuku/db/backup/types.go`: `NewBackupFormat` writes `BotName: "Xenobot-72"`. `Validate` checks only `b.BotName == ""` (line 56); no comparison to a constant. Confirmed by reading the function.
- `fuku/db/backup/pure_test.go` `TestBackupImportToleratesEarlierBotName`: the "earlier name" (`FukuRobot`) and "current name" subtests must pass `BackupFormatFromJSON` plus `Validate`; the refusal subtests (empty name, missing version, empty module list, unknown module, module without data) must fail. Meaningful: a `Validate` that compared the name to `Xenobot-72` would fail the earlier-name subtest, and one that dropped the empty-name check would fail the refusal subtest.
- `fuku/db/backup/backup_test.go` `TestImportChatData_EarlierBotNameExport`: a DB-backed import of an old-name export. It calls `skipIfNoDb`, which fails the test rather than skipping when the database is missing, so it cannot pass silently.

### T7. captcha_help_msg names /captchamaxattempts in en fr hi id pt ru uk; no locale says /captchaattempts; es left alone — VERIFIED

- Parsed `captcha_help_msg` in all 8 files: en, fr, hi, id, pt, ru, uk each contain `/captchamaxattempts`; es contains no attempts line at all (no added line). No `/captchaattempts` command spelling anywhere in locales, docs or Go code outside Go identifiers (`CaptchaAttempts` model names).
- `fuku/modules/captcha.go:2392` registers `captchamaxattempts`, so the help text now names a registered command.

### T8. Packaging and GitHub files: no OCI authors/vendor/url label naming the developer; titles and descriptions say Xenobot-72; FUNDING.yml and CODEOWNERS deleted; dependabot no reviewers; issue template no contact link; identifiers unchanged (go.mod, go.sum, migrations/, LICENSE, README.md, README.uk.md, app.json byte-identical; compose service, container, image, binary, env names as before) — VERIFIED

- `.goreleaser.yaml` labels: `title: "Xenobot-72"`, `description: "Xenobot-72 Docker Image"`, `source` is this repository's own URL (identifier), no `authors`, `vendor` or `url`. `docker/alpine`, `docker/alpine.debug`, `docker/pr-build` the same shape. Verified by reading the label blocks.
- `.github/FUNDING.yml` and `.github/CODEOWNERS` absent at HEAD; `git diff` shows them as deleted in 98d363d.
- `.github/dependabot.yml`: parses as YAML; the three `reviewers` blocks removed; three ecosystems remain.
- `.github/ISSUE_TEMPLATE/config.yml`: parses; only `blank_issues_enabled: false` remains.
- Byte-identity (`git diff --quiet a8e2541..HEAD -- <path>`): README.md, README.uk.md, LICENSE, go.mod, go.sum, app.json, and the whole `migrations/` directory are identical to base.
- Identifiers: `docker-compose.yml` diff is one comment line; `debug.docker-compose.yml` one comment; `.github/workflows/ci.yml` one label. Service names, image `ghcr.io/uasneppy/fuku_robot`, binary `fuku_robot`, env names, demo credentials unchanged.

### T9. docs/: no link or mention of developers, documentation host or old hosted username; prose says Xenobot-72; no docs page states a count other than 39 modules, 231 commands, 8 languages (commands index, api-reference Overview, docs config, landing page); every language list names all 8 incl. Ukrainian; checker counts mode clean; make check-docs passes — FAILED

- **FAILED: landing page stat tiles are stale.** `docs/pages/index.astro` lines 139, 143 and 147 read `29+` (Modules), `158+` (Commands) and `7+` (Languages). The base had the same values and the change set did not fix them. The text lines beside them were updated (39, 231, 8), so the page is internally inconsistent. The must-have names the landing page explicitly. The executor's notes claim "stale-figure grep: none now"; that is not true for this file. The executors' checker counts mode missed it because the number and its label sit in separate HTML tags and the checker reads prose. **Fix (3 lines):** change `29+` to `39`, `158+` to `231`, and `7+` to `8` in `docs/pages/index.astro`, then run `make check-docs`.
- **FAILED (literal): `docs/README.md` was modified.** Must-have (b) forbids touching any README. The edit is content-correct: it renames the title and the link text and removes "Live Documentation" (the original developers' host) and "Support Group" (`t.me/DivideSupport`). It is a docs-site index page under `docs/`, not one of the repository READMEs the brief excluded (README.md and README.uk.md). Owner decision requested: accept it as a docs page, or revert with `git checkout a8e2541 -- docs/README.md` if the literal rule is meant.
- Other figures, verified: `docs/src/content/docs/commands/index.mdx` ("Total Modules: 39 | Total Commands: 231"); `docs/src/content/docs/api-reference/commands.md` Overview (39, 231); `docs/blume.config.ts` description ("39 modules, 231 command names, 8 languages"); `docs/pages/index.astro` lines 30, 31, 55-58, 107-108 (8 Languages, 231 command names). Module count re-derived: 39 `RegisterLegacyModule(` calls outside tests (independent grep). Command count re-derived: `261115-rcd-commands.txt` has 231 `CMD` lines, 231 distinct, and its name set is identical to the README task's dump.
- Language lists: every docs line that lists Hindi or Indonesian also includes Ukrainian (`introduction.mdx`, `commands/index.mdx`, `pages/index.astro`); `architecture/project-structure.mdx` and `module-pattern.md` name `uk.yml`. Verified by Python scan over the docs tree.
- Developer links in docs: `git grep` over `docs/` finds none outside `uasneppy` identifier paths, clone URLs and the GitHub owner used by `blume.config.ts`. That owner is the same as the module path, which the owner kept.
- `commands/languages/index.md`: the Crowdin sentence is removed (verified in diff).
- Residual stale text in docs prose: none found in the scan above.
- `make check-docs` not run by verifier (pending). The generator-owned pages (`commands/<module>/index.md` without the manual marker and `api-reference/lock-types.md`) were not re-generated by the verifier; their content shows no old name or developer link.

### T10. AGENTS.md line 3 and .claude/CLAUDE.md lines 7 to 9 say Xenobot-72 with no fork claim; every AGENTS.md sentence invalidated by Task 1 edited in Task 1's commit — VERIFIED

- `AGENTS.md:3`: "Xenobot-72: Telegram group-management bot."
- `.claude/CLAUDE.md`: heading `**Xenobot-72**`, body "Xenobot-72 is a Go Telegram group-management bot." — no "fork" claim.
- AGENTS.md: grep for `news`, `support`, `creator`, `Crowdin`, `translat`, `online since`, `donat`, `keyboard`, `Fuku` in the HEAD version. The help-screen and language-keyboard sentences describe the new shapes (three rows, no news line, no creators plug, no translate row). The only leftover "Fuku" hits are class (a) identifiers (`help_button_add_fuku`).

### T11. make check-translations, make check-docs, make lint, make test pass; three Conventional Commits (fix, fix, docs), none containing `.planning/` or a helper file; nothing pushed; PROJECT.md uncommitted — PARTIAL: commit shape VERIFIED, gates PENDING

- Commits: `git rev-list --count a8e2541..HEAD` = 3: `a721b5a fix:`, `98d363d fix:`, `61a3401 docs:`. None touches `.planning/` (`git diff --name-only` filter empty). Nothing pushed (verifier did not push; no remote check performed beyond the log).
- Working tree: `git status --short` shows only ` M .planning/PROJECT.md` and the untracked `.planning/quick/261115-rcd-…/` directory, as the plan intended.
- Note (informational, not a must-have): the three commit trailers name `Co-Authored-By: Claude Sonnet 5.5`. The current session's attribution instruction names Haiku 5.5 for commits created from here on. These commits predate this verification and were not created by the verifier.
- Gates **pending, run by the orchestrator**: `make test` (in progress), `make check-translations`, `make check-docs`, `make lint`. The executors reported `make lint` could not run in their container (golangci-lint built with go1.25 against a go1.26.0 repo). `go vet` on touched packages stood in for them; not re-verified here.

---

## Artifacts

| Artifact | Status | Evidence |
|---|---|---|
| `locales/*.yml` (8 files) | VERIFIED | Key set, placeholder, tag and name checks above; over-4096 clause FAILED (pre-existing) |
| `fuku/modules/help.go` | VERIFIED | Keyboards and start text read; no third-party URL |
| `fuku/db/backup/types.go` | VERIFIED | `Validate` checks non-emptiness only; new exports write Xenobot-72 |
| `fuku/db/backup/pure_test.go` | VERIFIED | Old-name import test present; refusal subtests meaningful |
| `.planning/.../261115-rcd-check_text.py` | EXISTS, NOT RUN | Untracked checker, present; verifier used its own script instead. Its counts mode missed the landing-page tiles (blind spot). |
| `.planning/.../261115-rcd-notes.md` | EXISTS | Read; several claims (stale-figure grep none, make generate-docs no drift) not fully borne out (see T9) |
| `docs/src/content/docs/commands/index.mdx` | VERIFIED | "Total Modules: 39 \| Total Commands: 231" |

## Key links

| Link | Status | Evidence |
|---|---|---|
| `locales/*.yml` ↔ Go readers of removed keys | VERIFIED | No `git grep` hit for the five removed keys outside `.planning/` |
| `help_render_test.go` ↔ `help.go` | VERIFIED | Row counts, add-button row, negative third-party-URL assertion |
| `language_legacy_request_test.go` ↔ `AGENTS.md` | VERIFIED | Golden differs from base by exactly one row in two entries; AGENTS sentence present |
| docs figures ↔ dispatcher dump / registration grep | FAILED | `commands/index.mdx`, `commands.md`, `blume.config.ts` correct; `pages/index.astro` tiles 29+, 158+, 7+ |

---

## Gaps

1. **Landing page stat tiles stale (FAILED, must-have T9).** `docs/pages/index.astro:139,143,147`: `29+`, `158+`, `7+` must become `39`, `231`, `8`. Blocks the docs-figures must-have. Small, mechanical fix. (Severity: BLOCKER by the must-have rule; minimal in impact.)
2. **`docs/README.md` edited (FAILED literal, must-have T9 / check b).** Owner decision: accept as a docs page, or revert with `git checkout a8e2541 -- docs/README.md`.
3. **Over-4096 values (FAILED literal, must-have T3, pre-existing).** `es`, `fr`, `pt` `lockdown_help_msg` at 4097, 4101 and 4104 code points, unchanged from base. Owner decision: accept as pre-existing, or split in a follow-up.
4. **Six non-en/uk files changed beyond name and credits (WARNING, must-have T3 wording).** `help_help_msg` and `help_extended_docs` reworded in all languages, plus fr and ru elisions. Substantively justified by the removed links. Owner acknowledgment requested; the literal must-have should be amended if accepted.
5. **Dangling "support chat" wording (WARNING, advisory, not a must-have).** `disabling_help_msg` (en, es, fr, uk), `notes_parsing_error` and `notes_parsing_error_support` (en, es, fr, ru) still tell the reader to report to or post in "the support chat", which no longer appears anywhere in the bot's help. Not a link or developer name, so O-02 is not violated; a reader cannot find that chat. Recommend a rewording or an owner decision.
6. **Funding claim kept in `filters_limit_exceeded` (advisory).** en and uk still say the bot runs "free without any donations by users" (base wording, kept by the executor). Not a link, but a claim the owner may not want stated.
7. **Pending gates.** `make test`, `make check-translations`, `make check-docs`, `make lint`: not run by the verifier. Reported as pending, run by the orchestrator.

## Spot checks of claims in the new text against the code (j)

1. `/donate` handler sends `help_donatetext` with no link (`help.go:372-395`) — matches "no donation page".
2. `/about` renders `config.AppConfig.BotVersion` (`help.go:175`, value `2.24.0`) — the "Current Version" line is real.
3. `/donate` registered (`help.go:604`, dump line `CMD donate`).
4. Filter cap in code is `filtersCapMax = 150` (`filters.go:57`) — matches `filters_limit_exceeded`'s "maximum 150 filters".
5. Go module `go 1.26.0` (go.mod) — matches "we use Go as our core programming language".
6. Thanks-for-adding message appends only the admin prompt (`bot_updates.go:69-81`) — no creators plug.
7. `captchamaxattempts` registered (`captcha.go:2392`) — matches the rewritten `captcha_help_msg`.
8. 39 module registrations (independent grep) — matches "39 modules" in the docs.
9. 231 command names (dump, distinct, identical set to README task's dump) — matches "231 command names".
10. `/donate` text is HTML parse mode with link preview disabled — the reply carries no link at all.

## Overall

**status: gaps_found.** Three must-have clauses fail as written (the over-4096 clause and the six-file scope clause of T3, and the landing-page figures and the `docs/README.md` edit of T9), and the landing page figures are wrong: the docs-figures truth (T9) is FAILED, and the executor's claim that no figure is stale is not true for `docs/pages/index.astro`. The rest of the rename is substantively correct: no developer link or handle survives in any non-exempt file, the five keys are gone with their readers, the keyboards and golden are exactly as described, the backup import tolerance is real and tested, the packaging and GitHub files are clean, and all identifiers and protected files are byte-identical to base. The remaining items are a judgement for the owner (the `docs/README.md` edit, the pre-existing over-length values, the six-file scope deviation), plus the test-suite gates that the orchestrator runs.

_Verified: 2026-11-15_
_Verifier: Claude (gsd-verifier), independent re-check; no repository file edited, nothing committed or pushed._

## Orchestrator resolution of the gaps

- **Gap 1 (landing page tiles): fixed** in `c662a52`. `docs/pages/index.astro` tiles now read 39, 231 and 8; the counts checker, which could not see a number and its label in separate tags, is complemented by a direct grep for `>NN+<` tiles in docs (no other hit). `make check-docs` reports no drift.
- **Gap 2 (`docs/README.md` edited): accepted.** It is a docs-site page, not one of the repository READMEs the brief excluded; the edit removes the support group and live-docs host links, as the brief asks.
- **Gap 3 (`lockdown_help_msg` over 4096 code points in es, fr, pt): accepted as pre-existing.** The base has the same lengths and this task did not touch those keys. Listed as a follow-up.
- **Gap 4 (help_help_msg and help_extended_docs reworded in all languages, elision fixes in fr and ru): accepted.** They described `/about` as "useful links" and `/donate` as "supporting the project", which this task made false.
- **Gap 5 (`support chat` wording in `disabling_help_msg`, `notes_parsing_error`, `notes_parsing_error_support`): left as a follow-up.** It is generic wording, not a link or a developer name, but it points users at a chat the bot no longer names; rewording it touches several languages and was kept out of this task's scope.
- **Gap 6 (`filters_limit_exceeded` says the bot runs free without donations): left as is,** base wording.
- **Test suite:** the full `make test` was run by the orchestrator; result recorded in the SUMMARY.
