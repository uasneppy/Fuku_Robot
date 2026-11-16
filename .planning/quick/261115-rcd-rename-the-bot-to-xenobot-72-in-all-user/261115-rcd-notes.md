# 261115-rcd consolidated notes (untracked; the per-task details are in the task1, task2 and task3 notes next to this file)

Base commit: a8e25413c10dcd44bd8bcbc25f0c76b4eb82495e (261115-rcd-base.txt). Branch claude/ecstatic-goldberg-qsss05. Commits: a721b5a (Task 1, fix), 98d363d (Task 2, fix), 61a3401 (Task 3, docs). Nothing pushed.

## Classification table (final)

Class (a) identifier, stays. (b) text renamed to Xenobot-72. (c) link or mention of the original developers, removed (O-02). (d) judgement call, decided and recorded.

| Area | Hit | Class | Result | Task |
|---|---|---|---|---|
| help.go about keyboard | News and Support URL buttons | c | row removed, keyboard 4 -> 3 rows | 1 |
| help.go start keyboard and text | Support button, news-channel line | c | add-to-chat button alone in its row; text is the intro only | 1 |
| language.go | translation-platform URL row | c | removed, golden loses exactly that row | 1 |
| bot_updates.go | creators plug in the thanks message | c | removed | 1 |
| locales x8 | five link-only keys (help_button_news_channel, help_button_support_group, help_news_channel_text, bot_updates_creators_plug, language_help_translate) | c | deleted with their readers; 8 files keep one identical key set | 1 |
| locales x8 | help_about credits and "online since / thousands of groups" paragraphs | c | deleted; name renamed; en and uk gain one line "version 72 of 74" | 1 |
| locales x8 | help_donatetext | c / d1 | rewritten: this bot has no donation page, no contact | 1 |
| locales x8 | help_pm_intro, help_configuration_step-3, languages_help_msg | c | donate bullet reworded, support-group clauses and translation paragraph removed | 1 |
| locales x8 | help_help_msg, help_extended_docs (/about "useful links", /donate "supporting the project") | d | reworded, made false by this task | 1 |
| locales x8 | old name in about, intro, add button, formatting, filters, reports, help texts, YAML comments of fr and pt | b | name swap; en and uk drop the sparkle; fr and ru elision/preposition adjusted; uk formatting_random rebuilt | 1 |
| locales x7 | captcha_help_msg /captchaattempts | d | -> /captchamaxattempts; es has no attempts line, untouched | 1 |
| devs/repository.go | /stats heading | b | "Xenobot-72 Stats" | 1 |
| backup/types.go | bot_name in new exports | b | writes Xenobot-72; Validate unchanged (non-empty only); tests pin old-name import | 1 |
| AGENTS.md | /lang translate-link and golden sentences, names/help/donate/backup bullet; line 3 | b / d | corrected in Task 1; line 3 in Task 2 | 1, 2 |
| .goreleaser.yaml, docker/alpine, alpine.debug, pr-build | OCI authors, vendor, url labels; title, description | c / b | labels removed; title and description say Xenobot-72 | 2 |
| .github FUNDING.yml, CODEOWNERS | sponsor and owner files | c | git rm | 2 |
| .github dependabot.yml, issue template config | reviewers (3), contact link | c | removed | 2 |
| ci.yml, docker-compose.yml, debug compose, scripts, .golangci.yml, CLAUDE.md | old name in a label or comment | b / d | renamed | 2 |
| docs (33 files) | old name in prose; original-developer links; old hosted username; figures; language lists | b / c / d | see task3 notes | 3 |
| go.mod, go.sum, import paths, image `ghcr.io/uasneppy/fuku_robot`, compose service `fuku`, container names, env names, tables, cache keys, locale key `help_button_add_fuku`, binary and package names, nssm service `FukuRobot`, alert `FukuUnhealthy`, docs package name | identifier | a | untouched | - |
| app.json | `"name": "fuku-robot"` (suggested Heroku app name) | a | untouched, byte-identical | - |
| test fixtures (fake bot "Fuku", FukuTestBot, group titles, `bot_name` of old exports in roundtrip_test.go and backup fixtures) | fixture | a | kept on purpose: the old-name exports must still import | - |
| migrations/20261004130000_add_staff_role_exclusivity_trigger.sql comment, LICENSE, READMEs | applied or frozen | a | untouched | - |
| .planning/PROJECT.md | fork claim, old name | b / c | edited in Task 2, uncommitted | 2 |

Every remaining old-name hit outside tests and the READMEs is class (a); the class (d) calls are listed next.

## Decisions 1 to 13 as applied

1. /donate stays registered (README.md and the 231 count stay true); text says there is no donation page. Owner can remove the command later.
2. FUNDING.yml and CODEOWNERS deleted; dependabot reviewers and the issue template's contact link removed; nothing replaced.
3. OCI authors, vendor, url labels removed; title and description say Xenobot-72 (goreleaser title is the literal name).
4. docs/blume.config.ts github owner is `uasneppy` / `fuku_robot`, taken from the module path.
5. Docs lost the old hosted username; they say "your bot's username" and point at BotFather, with a placeholder URL in a code span. No username invented.
6. "Fork the repository" and "Push to your fork" kept.
7. No "online since" or "thousands of groups" claim anywhere in the 8 help_about values.
8. es.yml: no attempts line in captcha_help_msg, none added.
9. help_donatetext is still read with a hard-coded Ukrainian translator in help.go; code untouched, value rewritten in all 8 files.
10. The /lang golden lost exactly the translation-platform row from the command_group_admin and command_private entries; a comment says so; AGENTS.md says so.
11. app.json not edited (Heroku app name rule).
12. api-reference/commands.md Overview: 39 modules, 231 command names, split dropped; its per-module tables untouched; every language list and locale tree names all 8.
13. .planning/PROJECT.md edited in Task 2, uncommitted for the orchestrator.

Executor decisions beyond the plan: help_help_msg and help_extended_docs reworded in all 8 languages and the matching docs rows; en and uk drop the sparkle after the name; fr "de Xenobot-72" and ru "о Xenobot-72"; .golangci.yml comment renamed; debug.docker-compose.yml comment says "the bot"; PROJECT.md "Fork of ..." context bullet retitled "Identifiers keep the earlier name."; module-pattern.md locale sentence gains uk.yml; extra test TestImportChatData_EarlierBotNameExport; checker FENCE regex allows indented fences; filters_limit_exceeded still says the bot runs "without any donations" (names no contact or link, kept); pre-existing quirks left (pt and ru help_about whitespace, three untouched values over 4096 characters in es, fr, pt lockdown_help_msg, help.go gofmt import order).

## Baselines and counts

- Checker counts mode before the docs edit: 17 hits; prose mode: 94 hits; stale-figure grep: 7 lines; all now 0 (task3 notes list them). Locales mode before Task 1: 217 failures; now 0.
- Figures: 39 modules (RegisterLegacyModule outside tests), 231 command names (dispatcher dump, aliases included, identical to the README task's dump), 8 languages.
- The docs generator and `make inventory` undercount: the generated docs have 34 module directories against 39 registered modules, the card badges of commands/index.mdx sum to 159 over 27 cards, and the per-module tables of api-reference/commands.md hold 162 rows (the alphabetical index 161). Those cards, badges and tables were left as they are.

## Checks and results

- Task 1: checker locales 0 failures; make check-translations pass; make check-docs no drift; go build; tests for ./fuku/i18n/..., ./fuku/db/backup, ./fuku/db/devs and the focused ./fuku/modules set pass with -race; go vet -tags testtools clean.
- Task 2: YAML and JSON parse, bash -n, docker compose config, generate_docs tests, greps, identifiers present.
- Task 3: checker prose 0, counts 0, greps, make generate-docs changes nothing, make check-docs no drift, make check-translations pass, docs site `blume build` 63 pages in a scratchpad copy.
- NOT run: the full `make test` (the orchestrator runs it). `make lint` cannot run in this container (golangci-lint built with go1.25, repo targets 1.26.0); go vet over the touched Go packages stood in.

## Follow-ups (what remains, none required)

- Remove /donate in a later task if the owner wants the command gone (then README.md, locales and the 231 count change together).
- Pick a Heroku-safe slug for app.json if the new name should appear there.
- Existing group chats and the BotFather profile still show the old name until the owner changes them.
- The docs per-module cards and tables undercount (above); a generator fix would be a separate task.
- Old exports keep saying the old bot_name; harmless, import never compares it.
- OCI labels: images built before this change keep their old labels.

## Owner notes

BotFather to-do: /setname to Xenobot-72; /setusername (must end in "bot"; it changes every t.me link and the Mini App URL, so decide first whether to keep the current username); /setdescription and /setabouttext (per language if wanted); /setuserpic. No public link or username was invented anywhere: docs and help say "the bot" until the owner publishes one.
Deleted, not replaced: .github/CODEOWNERS (it made someone else the owner of every path) and .github/FUNDING.yml. OCI authors, vendor and url labels are gone.
Edited golden: TestLegacyLanguageRequests lost only the translation-platform keyboard row (two entries), by owner decision O-02.
/donate now answers "Notice. This bot has no donation page. There is nothing to donate to." (en) with an equivalent plain text in the other seven languages.
app.json keeps `fuku-robot` because Heroku app names allow only lowercase letters, digits and dashes.

## Left for the orchestrator

.planning/PROJECT.md is edited and uncommitted on purpose. Commit it with these quick-task files in the final docs commit, explicit paths. Run `make test` once.
