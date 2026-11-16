# 261115-rcd Task 1 notes (untracked, never committed)

Base commit: a8e25413c10dcd44bd8bcbc25f0c76b4eb82495e (in 261115-rcd-base.txt). Branch: claude/ecstatic-goldberg-qsss05.
Scope of this file: Task 1 (code and locale text). Tasks 2 and 3 extend the table below.

## Recomputed hit list (git grep and PyYAML, excluding .planning, migrations, LICENSE, READMEs and the GSD tooling under .claude)

PAT-DEV files (count of matching lines): .github/CODEOWNERS 1, .github/FUNDING.yml 2, .github/ISSUE_TEMPLATE/config.yml 2,
.github/dependabot.yml 3, .goreleaser.yaml 2, docker/alpine 3, docker/alpine.debug 3, docker/pr-build 3, docs/README.md 2,
docs/blume.config.ts 1, docs/pages/index.astro 4, docs/src/content/docs/{commands/languages/index.md, contributing/index.mdx,
getting-started/quick-start.mdx} 1 each, fuku/modules/help.go 3 (News, Support x2), fuku/modules/language.go 1 (crowdin),
fuku/modules/language_legacy_request_test.go 2 (golden), locales: 17 keys per language scanned by PyYAML (see below).
migrations/20261004130000_add_staff_role_exclusivity_trigger.sql has one comment hit: applied, never edited (class a, untouched).

Name-word (PAT-NAME-WORD) non-test hits outside docs: .claude/CLAUDE.md 2, .github/CODEOWNERS 1, .github/workflows/ci.yml 1,
.golangci.yml 1 (comment, not in the plan's file list: left; Task 2 may take it), .goreleaser.yaml 1, AGENTS.md 1, docker-compose.yml 1,
docker/alpine 2, docker/alpine.debug 2, docker/pr-build 2, scripts/generate_docs/main.go 2, scripts/migrate_psql.sh 1, locales/fr.yml and
pt.yml line 1 (YAML comment "# ... for Fuku Robot"; fixed in Task 1), fuku/db/backup/types.go 1, fuku/db/devs/repository.go 1.
docs: 32 files (Task 3).

## Classification table (Task 1 area)

Class (b) rename, class (c) remove, class (d) decision, class (a) stays.

| Where | Text | Class | Decision | Fixed in |
|---|---|---|---|---|
| fuku/modules/help.go getAboutKb | News and Support URL buttons | c | row removed; about keyboard 4 rows -> 3 | Task 1 |
| fuku/modules/help.go getStartMarkup | Support URL button | c | removed; add-to-chat button alone in its row | Task 1 |
| fuku/modules/help.go getStartHelp | appends help_news_channel_text | c | returns help_bot_intro only | Task 1 |
| fuku/modules/language.go genFullLanguageKb | translation-platform URL row | c | removed, translator for it removed | Task 1 |
| fuku/modules/bot_updates.go | creators plug appended to thanks | c | removed | Task 1 |
| locales x8 bot_updates_creators_plug, help_news_channel_text, help_button_news_channel, help_button_support_group, language_help_translate | link-only keys | c | five keys deleted in all 8 files; no reader left (git grep empty) | Task 1 |
| locales x8 help_about | credits paragraph, "online since 2020 / thousands of groups" paragraph, "Why <name>" | c + b | both paragraphs deleted; heading renamed; en and uk gain one in-world line "version 72 of 74" | Task 1 |
| locales x8 help_donatetext | contact handle, funds claims | c | rewritten as a short no-donation-page statement in each language (decision 1) | Task 1 |
| locales x8 help_pm_intro | /donate bullet promising support for creator; support-group clause | c | bullet reworded to "whether this bot accepts donations"; only the clause removed | Task 1 |
| locales x7 help_configuration_step-3 (hi has none) | invitation to the support group + "we would love to help" | c | last two sentences removed | Task 1 |
| locales x8 languages_help_msg | translation-platform paragraph | c | final paragraph removed (ru keeps its pre-existing stray quote tail) | Task 1 |
| locales x8 help_help_msg | "/about ... useful links", "/donate ... supporting the project", name | d + b | /about now "info and version", /donate "whether this bot accepts donations" (statements made false by this task) | Task 1 |
| locales x8 help_extended_docs | same /about and /donate sentence | d | same rewording | Task 1 |
| locales x8 filters_help_msg, formatting_help_msg, formatting_large_options, formatting_random, help_bot_intro, help_button_add_fuku, help_configuration_step-1, help_info_about_header, reports_help_msg | name in values | b | name swap only; en/uk drop the sparkle after the name; fr "d'Fuku" -> "de Xenobot-72", ru "об Fuku" -> "о Xenobot-72"; uk formatting_random rebuilt ("зробити бота більш індивідуальним") to avoid a gendered form after the name | Task 1 |
| locales x7 captcha_help_msg | /captchaattempts | d | -> /captchamaxattempts (registered command). es.yml has no attempts line: confirmed by script, left alone | Task 1 |
| locales fr.yml, pt.yml line 1 | YAML comment naming the old project | b | comment renamed | Task 1 |
| fuku/db/devs/repository.go | "Fuku's Stats" heading | b | "Xenobot-72 Stats" | Task 1 |
| fuku/db/backup/types.go | BotName written into every export | b | NewBackupFormat writes Xenobot-72; Validate unchanged (non-empty name only), comment added | Task 1 |
| AGENTS.md | /lang translate-link sentence, language golden sentence | d | both corrected; one bullet added under Handlers about names, help screens, donate and the backup name | Task 1 |
| .github/*, .goreleaser.yaml, docker/*, docker-compose files, scripts, .claude/CLAUDE.md, AGENTS.md line 3, ci.yml label, .golangci.yml comment | see hit list | b / c | Task 2 | Task 2 |
| docs/** | see hit list | b / c | Task 3 | Task 3 |

Class (a), kept (one row per file, identifiers and fixtures):

| File | Identifier kinds |
|---|---|
| go.mod, go.sum, all import paths | module path github.com/uasneppy/fuku_robot |
| fuku/db/backup/roundtrip_test.go (line ~417) | `"bot_name":"FukuRobot"` is an import fixture of an old export (binding clarification 4): no edit |
| fuku/db/backup/backup_test.go (lines ~155, 167), pure_test.go struct literals and the `bot_name` JSON fixtures | old-name fixtures of old exports; kept on purpose, and a new test pins that they still import |
| fuku/main_test.go, main_test.go, fuku/modules/*_test.go, fuku/utils/cache/*_test.go (about 90 lines) | fake bot first_name "Fuku" and username FukuTestBot in Telegram fakes; titles "Fuku Test Group", "Fuku & Co", "Fuku Staff" used as group titles in fixtures |
| fuku/modules/help_render_test.go | username "FukuRobot" passed to getStartMarkup and the add-to-chat URL built from it (a test username) |
| locales/*.yml key `help_button_add_fuku` | locale key name (O-01) |
| app.json | `"name": "fuku-robot"` is also the suggested Heroku app name (Heroku accepts lowercase letters, digits and dashes only), so it is an identifier; the other fields (description, keywords, env descriptions, repository URL) name neither the old bot nor the original developers. Byte-identical to the base commit (binding clarification 3). Owner can pick a Heroku-safe slug later |
| migrations/*.sql, LICENSE, README.md, README.uk.md | untouched by rule |
| .planning/PROJECT.md, STATE.md, ROADMAP.md | untouched in Task 1 |

## Decisions (class d): the planner's thirteen, with changes

1. /donate stays registered; text says there is no donation page; no handle, link, contact, creator or funds claim. Done. Owner can remove the command later.
2. FUNDING.yml and CODEOWNERS deleted, dependabot reviewers removed, issue template contact link removed: Task 2.
3. OCI authors, vendor and url labels removed; title and description say Xenobot-72: Task 2.
4. docs/blume.config.ts github owner points at the repository's own owner: Task 3.
5. Docs lose the old hosted bot username instruction: Task 3.
6. Generic contribution steps ("Fork the repository") kept: Task 3.
7. No "online since" or "thousands of groups" claim anywhere: done in all 8 help_about values.
8. es.yml has no attempts line in captcha_help_msg: not added. Done.
9. help_donatetext is read with a hard-coded Ukrainian translator in help.go: that code is left alone; the value is rewritten in all 8 files. Done.
10. /lang golden: exactly the translation-platform row removed from the two entries (command_group_admin, command_private); a comment above the map says so (O-02, quick task 261115-rcd). No other byte changed. AGENTS.md sentence updated.
11. app.json not edited. Done (see class a).
12. api-reference/commands.md totals: Task 3.
13. .planning/PROJECT.md: Task 2, left uncommitted. Not touched in Task 1.

Executor decisions beyond the plan (class d):
- help_help_msg and help_extended_docs also described /about as showing "useful links" and /donate as "supporting the project". Both were made false by this task, so I reworded them in all 8 languages (small native phrasings, no extra claim).
- en and uk drop the sparkle after the name ("Fuku ✨" -> "Xenobot-72", no emoji for flavour); the other six keep it ("Xenobot-72 ✨"), as they receive the name swap only.
- French "d'Fuku" became "de Xenobot-72" and Russian "об Fuku" became "о Xenobot-72": the elision and preposition depend on the sound of the name, so the one-word swap is adjusted, nothing else in those sentences.
- Pre-existing quirks left as they were (out of scope): pt help_about and ru help_about / step texts carry spaces or literal backslash-n sequences instead of line breaks; ru help_donatetext used to carry stray quote characters (the value is rewritten cleanly now). Unchanged keys over 4096 chars in the base (es 4097, fr 4101, pt 4104 for lockdown_help_msg) are not mine; the checker reports them as NOTE, not as failures.
- filters_limit_exceeded still says the bot runs "free without any donations by users": it names no contact or link and is not in the plan's list, so it is kept (accepted d).
- A new test, TestImportChatData_EarlierBotNameExport (SQLite-backed, runs in this container), imports an old-name export through ImportChatData for the warns module, in addition to the pure TestBackupImportToleratesEarlierBotName. The existing legacy test in roundtrip_test.go already covers old-name imports of two more modules.

## Checks and results (Task 1)

- Removed buttons are plain URL buttons: no callback data and no handler existed for them (grep for the callback names found none). Nothing dead left behind.
- Checker locales mode: 0 failures. Notes: 3 untouched pre-existing over-4096 values (above). REVIEW lines: help_button_add_fuku grew in en (19 -> 25) and uk (22 -> 28): the name itself is longer; the i18n label-length tests pass.
- Before the edits the same mode failed with 217 failures (not vacuous).
- Each locale file was re-parsed by PyYAML after the edit script, and the parsed dict was asserted equal to "base dict with exactly the planned edits" (no collateral change in any other key).
- make check-translations: pass (scripts/check_translations go.mod and go.sum unchanged). make check-docs: no drift. make generate-docs changed no file under docs/ (none of the edited keys feed a generator-owned page, so Task 1 stages no docs path).
- go build ./... (CGO off): pass. go test -tags testtools -race ./fuku/i18n/... ./fuku/db/backup ./fuku/db/devs: pass. Focused ./fuku/modules run (TestHelp|TestAbout|TestDonate|TestGetBotUsername|TestRenderModuleHelp|TestLegacyLanguage|TestLanguage|TestSettings*Language|TestBotUpdates|TestHandleDeepLink|TestCaptchaMode): pass.
- make lint: golangci-lint at /usr/local/bin is built with go1.25 and aborts ("the Go language version (go1.25) used to build golangci-lint is lower than the targeted Go version (1.26.0)"), so it cannot run in this container. Substituted: go vet -tags testtools over ./fuku/modules ./fuku/db/backup ./fuku/db/devs ./fuku/i18n/... : clean. gofmt: my files are clean (help.go has an import-order gofmt difference that exists at the base commit too; not touched).
- Pattern greps of the verify step: PAT-DEV over locales, fuku, main.go empty; name-word over fuku and main.go non-test empty; README.md, README.uk.md, LICENSE, migrations, go.mod, go.sum, app.json identical to base.
- make test not run (Task 3 runs it once).
