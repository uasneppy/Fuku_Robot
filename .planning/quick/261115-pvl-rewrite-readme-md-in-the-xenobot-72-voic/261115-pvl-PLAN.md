---
phase: 261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md
  - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt
  - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py
  - README.md
  - README.uk.md
autonomous: true
requirements: [QUICK-261115-pvl]

estimate:
  tokens: 150000
  raw_tokens: 150000
  tasks: 3
  confidence: low

must_haves:
  truths:
    - "README.md (English) and README.uk.md (original Ukrainian text, not a translation) both describe the bot as it is at release v2.24.0, link to each other in the first lines, and have the same section order, the same command set and the same environment variable set (D-01, D-02)."
    - "Every sentence about behaviour was checked against code, the generated docs pages or AGENTS.md before it was written, and the check is recorded in the fact sheet; an unverifiable claim is not in either file (D-03)."
    - "Every command name the running dispatcher registers (231 at planning time, aliases included) appears in the Commands section of both files. The retired raid commands are described as retired pointers to Lockdown, never as a live raid mode."
    - "No reader-visible prose names the old bot, the original project or its developers, and neither file contains the words upstream or fork. The only occurrences of the old name are unavoidable identifiers inside code blocks, code spans and URLs (the clone URL, image name, module path, directory and package names, Redis key prefixes, FUKU_TEST_DATABASE)."
    - "Every number stated in prose is listed in the fact sheet with the command that produced it. No invented statistic, date, user count or history."
    - "Every relative link and anchor in both files resolves, the centred header HTML is balanced, and external links point only to hosts that are not the original project."
    - "Flavour follows the owner's scale: intro and feature blurbs full to medium; command tables a light line then plain words; getting started, configuration, endpoints, structure and development near zero; staff, lockdown, raid, captcha, settings and blocklist text exact with no jokes; nothing about starvation, war, death, gore or a person's punishment; Ukrainian uses formal ви and the feminine form of the bot or avoids a gendered form."
    - "The two code commits contain only README.md and README.uk.md. locales/, docs/, code, LICENSE, AGENTS.md, app.json, .goreleaser.yaml and the compose files are untouched, and every throwaway helper file is deleted before any commit."
  artifacts:
    - path: "README.md"
      provides: "English README in the Xenobot-72 voice, written from verified facts"
    - path: "README.uk.md"
      provides: "Original Ukrainian README with the same structure, commands and variables"
    - path: ".planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md"
      provides: "Fact sheet: every count, command, env var, link and behaviour claim with the command or file:line that proves it"
    - path: ".planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt"
      provides: "Dump of the command names the real dispatcher registers, used as the command-coverage oracle"
    - path: ".planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py"
      provides: "The README checker: names, links and anchors, HTML, tables, placeholders, figures, command coverage, Ukrainian rules, parity"
  key_links:
    - from: "README.md"
      to: ".planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md"
      via: "every number in prose is a line of the Stated figures section, and every behaviour sentence has an evidence row"
    - from: "README.md"
      to: "README.uk.md"
      via: "top-of-file links both ways plus the checker's parity mode (headings, commands, env vars, link targets, block counts)"
    - from: "README.md"
      to: "docs/src/content/docs/"
      via: "relative links to docs pages, each proven with test -e by the checker"
    - from: ".planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt"
      to: "README.md"
      via: "the checker requires every CMD line to appear as /name in the Commands section of both files"
---

<objective>
Rewrite README.md in the Xenobot-72 voice (English) and add an original-text Ukrainian README.uk.md, both describing the bot exactly as it is now at release v2.24.0 (what main will be after draft PR #8 merges).

Purpose: the current README is a stale snapshot of the project before the Staff Group, lockdown, web captcha, settings menu, member requirements, scheduled deletion, list pages and the forward blocklist existed, and it still credits and names another project. The owner wants two honest, current, in-voice READMEs.
Output: README.md, README.uk.md, and a committed-with-the-docs fact sheet, command dump and checker in the quick task directory.

Owner decisions, locked, cited as D-01 to D-03:
- D-01: two files, English README.md and Ukrainian README.uk.md, same voice, linked to each other at the top; the Ukrainian is original text, not a word-for-word translation.
- D-02: describe the bot as it is NOW, written from the actual code and docs, not from the old README. Drop "What's different from upstream", the development-branch roadmap, "Keeping the fork in sync" and any upstream or fork credit. Add what milestone v1.1 and the Staff Group forward blocklist added.
- D-03: the old README is a stale snapshot. It says main is the original code unchanged, documents the retired raid mode as live, counts "30 modules and more than 200 command triggers", and omits staff groups, lockdown, automatic raid detection, the web captcha, the settings menu, member requirements, scheduled deletion, the list pages and the forward blocklist. Treat every sentence of it as unverified until checked against the code.

Decomposition note: tracer-first does not apply to a documentation-only quick task (no layers, no runnable path). The vertical slice is "checker plus fact sheet plus one complete file": Task 1 builds the checker first, proves it fails on the old README (red), then produces a complete verified README.md. Task 2 adds the second file from the same fact sheet. Task 3 is the cross-file pass. Each task leaves the tree clean and, for Tasks 1 and 2, ends in one atomic code commit. Nothing is pushed and no branch is created.
</objective>

<execution_context>
@/home/user/Fuku_Robot/.claude/gsd-core/workflows/execute-plan.md
@/home/user/Fuku_Robot/.claude/gsd-core/templates/summary.md
</execution_context>

<context>
## Required reading before any task

- The owner's rewrite brief, READ ALL OF IT FIRST: `/root/.claude/uploads/4ce545b5-6832-57a3-a863-43c972932c7c/2e719ace-xenobot-72-rewrite-instructions.md` (a copy is at `/tmp/claude-0/-home-user-Fuku-Robot/4ce545b5-6832-57a3-a863-43c972932c7c/scratchpad/xenobot-style-brief.md`). It was written for the locale files. For the README these parts apply: section 1 (the job), 3 (the rename), 3a (source material), 4 (voice, vocabulary, series references, what to never joke about), 5 (flavour by message type), 6 (meaning must not change), 8 (Ukrainian rules), 9 (examples). Section 7 (locale file mechanics) does not apply. The rules below restate what the executor needs, but the brief wins on any detail.
- `README.md` (the old one: structure only, every claim unverified).
- `.claude/CLAUDE.md` and `AGENTS.md` (AGENTS.md is very large: never read it end to end. Use `grep -n` for the exact bullet you need, then `Read` with offset and limit).

## Observed at planning time (live repo, HEAD 83dd98e on claude/ecstatic-goldberg-qsss05, clean tree). Re-verify, do not trust.

- Version: `BotVersion: "2.24.0"` at `fuku/config/config.go:402`, `v2.24.0` default at `main.go:62`. `go 1.26.0` in `go.mod`, module `github.com/uasneppy/fuku_robot`.
- A throwaway test that loads every module through `fuku.LoadModules` and dumps the `helpers` command registry printed `COUNT 231` (aliases included, one entry per distinct lower-cased name). The old README mentions all but four of them somewhere (`addfwdblock`, `fwdblockaction`, `fwdblocklist`, `rmfwdblock`), but it has no staff or lockdown command tables. The generated docs index (`docs/src/content/docs/commands/index.mdx`) says "30 modules, 159 commands" and `make inventory` says 148 source files and 160 commands: both undercount (they parse source patterns and miss `WrapCommand` descriptors), so cite neither.
- Module registrations: 39 `RegisterLegacyModule("` calls outside tests; `LoadHelp` loads last (`fuku/main.go`).
- `ls migrations/*.sql | wc -l` is 64 (the old README says 42). Eight languages (`ls locales`: en es fr hi id pt ru uk plus `config.yml`, which is not a language). Lock types: `lockMap` 16 plus `restrMap` 6, union 22.
- Distinct env names read by non-test code: 57, of which `FUKU_TEST_DATABASE` is test tooling. Five are missing from the old README: `CAPTCHA_WEB_BASE_URL`, `CAPTCHA_WEB_TRUSTED_PROXIES`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SECRET_KEY`, `TURNSTILE_SITE_KEY`. The `OTEL_*` names are read in `fuku/utils/tracing/tracing.go`, not `config.go`.
- The webhook route is `/webhook` (`fuku/utils/httpserver/server.go:225`), not the path an old sentence in `.claude/CLAUDE.md` mentions: code wins over any prose.
- Deploy files observed: `docker-compose.yml` (image `ghcr.io/uasneppy/fuku_robot:latest`, `stop_grace_period: 75s`, Postgres 17, Redis 7), `debug.docker-compose.yml`, `docker/alpine`, `docker/alpine.debug`, `docker/goreleaser`, `docker/pr-build`, `render.yaml`, `railway.toml`, `app.json` (its `repository` now points at the owner's repository), `Procfile`, `nixpacks.toml`, `.goreleaser.yaml`, `.github/workflows/{ci,release,docs,dependabot-native-merge,pullfrog}.yml`. CI coverage floor `THRESHOLD=78` and Postgres 16 are in `ci.yml`; commit 54dac65 moved the release image to this repository's registry, so the old README's "the compose file pulls the other project's image" warning is stale.
- Docs pages that exist and may be linked (verify each with `test -e`): `docs/README.md`; `docs/src/content/docs/self-hosting/{docker.mdx,source.mdx,binary.mdx,database.md,webhooks.md,monitoring.md,profiling.md,troubleshooting.md,captcha-web.md}`; `docs/src/content/docs/api-reference/{environment.md,lock-types.md,permissions.md,callbacks.md,database-schema.md}`; `docs/src/content/docs/architecture/{handler-groups.md,caching.md,request-flow.mdx,module-pattern.md,anonymous-admin.md,project-structure.mdx}`; `docs/src/content/docs/commands/<module>/index.md` for staff, lockdown, settings, captcha, antiraid and the rest. The docs pages still carry the old name in their own text (renaming docs is a separate job): link to them by relative path, never quote them, and list this in the final report.
- Help texts to read instead of AGENTS.md: `locales/en.yml` keys `captcha_help_msg` (line 290), `staff_help_msg` (2661), `lockdown_help_msg` (2882), `settings_help_msg` (3528) and the `*_help_msg` keys at lines 37 77 143 180 243 363 399 425 452 521 598 698 710 720 742 768 788 814 847 859 893 920 955 1825 2339 2369 2492 2503 2529. Command source: `fuku/modules/*.go` (`helpers.WrapCommand` descriptors, `helpers.NewBotCommand`, `MultiCommand`), the staff table in `fuku/modules/staff_action.go`, the forward blocklist in `fuku/modules/staff_forwardblock.go` and `staff_forwardblock_watch.go`.
- Handler groups per AGENTS.md "Handlers" (claim, verify against the `AddHandlerToGroup` calls and group constants): -10 captcha sweeper, -9 forward blocklist watcher, -8 member deletion, -7 lockdown join guard, -6 fed-ban, -5 service message deletion, -4 raid flood counter, -3 staff watchers, -2 admin cache and member requirements watcher, -1 users tracker, 0 commands/help/greetings, 1 settings typed-reply door, 2 raid AI candidate watcher, 3 aispam, 4 antiflood, 5/6 locks, 7 blacklists, 8 reports and reactions, 9 filters, 10 pins, 11 log channel, 12 late raid flood counter; command deletion wraps the dispatcher processor. The old README diagram (`-5 antiraid`) is wrong.

## Stale claims register seed (each old-README claim must be re-verified or dropped, and recorded in the fact sheet)

main is the original code unchanged (commit 34ba3f6, version 2.23.14); 30 modules, more than 200 triggers; AntiRaid as a live module with `/raidtime` and `/raidactiontime` timings; 42 migrations; the antiraid handler group and "Redis holds the anti-raid state"; compose pulls the other project's image; app.json points at the other project; captcha is only math or text; "from 80 % for English messages, 90 % for other languages" for the AI spam filter (re-verify in code or drop); the dev branch roadmap, "Keeping the fork in sync", all credit text.

## Voice rules (restated; the brief wins)

Name: `Xenobot-72` exactly (capital X, hyphen, no space), Latin letters in Ukrainian, never declined. No reader-visible text may say Fuku, Fuku Robot, Alita, Alita Robot, DivideProjects, Divkix, upstream or fork, or credit the original developers. The only allowed "fuku" occurrences are identifiers the reader must type or see: the clone URL `https://github.com/uasneppy/Fuku_Robot`, the module path, the image `ghcr.io/uasneppy/fuku_robot`, directory and package names in the structure tree, Redis key prefixes, env names such as `FUKU_TEST_DATABASE`, the binary name `fuku_robot`. They go inside code blocks, code spans or URLs, never as the bot's name in prose. Remove the fork badge and every link to the original repository, its support group and update channel. Do not replace the credit with invented history. The README must not say or hint that the bot had another name.

Flavour scale (brief section 5, applied): intro, about and feature blurbs full to medium; command tables a light flavoured opening line, then plain words, every command, alias, argument and explanation kept; getting started, configuration, endpoints, project structure and development near zero (exact steps, one announcer sentence at most); the staff, lockdown, raid, captcha, settings, member requirements, deletion and blocklist descriptions near zero (exact, "Notice." phrasing is enough, no jokes, plain words such as group and member are better than glossary words there); Documentation and License none. Clarity beats character. At most one clear series reference per section and none in most sections; a reader who never saw the series must understand every line. Allowed references: tempe and soy, rations, the facility, containment, Jaya Abadi, the safety lock, "Batas" for something unbreakable, cat photos, and the advert shapes ("No sweat! Xenobot-72 will handle it.", "It's the only bot in the group.", "Remember! Everything only from Jaya Abadi."). Proper names allowed: Xenobot-72 and Jaya Abadi only. In-world facts are allowed: version 72 of 74, built by Jaya Abadi to run the facility. Do not retell the plot.

Series voice in six lines: announcer sentences ("Request denied."); blunt labels in official clothing ("Notice. This cannot be undone. At all."); deadpan, no exclamation marks and no emoji for flavour; the bot says "I" and sometimes "this unit"; dry at the request, never at the person, no slurs or swearing; exact figures, but only verified ones.

Never joke about starvation or famine, war, real disasters, death, gore, a person's punishment. Never gloat in a ban or warn description. Never put the cannibalism lines near a ban, warn or report (do not use them at all in a README).

Glossary (use sparingly, consistently, and never in place of the real word where a reader must act; "banned" and "admin" and every command, argument and Telegram right name stay as they are):

| Plain word | English | Ukrainian |
|---|---|---|
| group / chat | the facility (first mention; "group" stays fine) | об'єкт |
| member | resident | мешканець |
| admin | supervisor (keep "admin" where a right or command is meant) | наглядач / адмін |
| ban | removal from the facility (always also say "banned") | видалення з об'єкта (і слово «бан») |
| mute | silence order | режим тиші |
| rules | regulations | регламент |
| warn | violation notice | попередження про порушення |
| lockdown | containment | ізоляція об'єкта (і слово «локдаун») |
| error | malfunction | збій |

Ukrainian rules (brief section 8): natural modern literary Ukrainian, no Russian calques or surzhyk (not "слідуючий", "приймати участь", "на протязі", "являється"); formal "ви" in the announcer voice; the bot is FEMININE ("я перевірила") and wording that avoids a gendered form is preferred; never assume a user's gender ("Користувача заблоковано", not "він заблокований"); wording that works for any number ("Попереджень: N"); «…» quotation marks; "темпе"; technical terms Telegram users know stay (бан, мут, адмін, капча, локдаун); keep it as short as the English, because Ukrainian runs longer. The Ukrainian README is NOT a translation: same facts, same structure, same section order, its own sentences and its own joke where a joke fits (and none where the section is near zero).
</context>

<tasks>

<task type="auto">
  <name>Task 1: Checker, fact sheet and the complete English README.md (one commit: README.md)</name>
  <files>.planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py, .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt, .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md, README.md</files>
  <read_first>
    - /root/.claude/uploads/4ce545b5-6832-57a3-a863-43c972932c7c/2e719ace-xenobot-72-rewrite-instructions.md (all of it)
    - README.md (old; structure and the sections that may still be true, every claim unverified)
    - fuku/config/config.go, sample.env, docker-compose.yml, debug.docker-compose.yml, docker/alpine, .goreleaser.yaml, Makefile, go.mod, .github/workflows/ci.yml, .github/workflows/release.yml, .github/workflows/docs.yml
    - docs/src/content/docs/self-hosting/captcha-web.md and the command pages docs/src/content/docs/commands/{staff,lockdown,settings,captcha,antiraid}/index.md
    - locales/en.yml help keys named in the context block (read with offset and limit)
    - .planning/PROJECT.md "What v1.1 delivered" and .planning/milestones/v1.1-ROADMAP.md phase goals (for what milestone v1.1 shipped)
  </read_first>
  <action>
Implements D-01, D-02, D-03. Every claim that goes into README.md is verified first; a claim that cannot be verified is left out, never softened into a guess.

0. Guard. `git branch --show-current` must print `claude/ecstatic-goldberg-qsss05` and `git status --porcelain` must list no tracked change. Stop and report if not. Never push, never create a branch.

1. Dump the real command registry (the oracle for command coverage). Create two throwaway files that are deleted in this same step. In `fuku/utils/helpers/zz_export_test.go` (package `helpers`) write a function that returns the sorted keys of `botCommands` read under `botCommandsMu.RLock()`. In `fuku/utils/helpers/zz_count_test.go` (package `helpers_test`, importing `github.com/PaulSonOfLars/gotgbot/v2/ext`, `github.com/uasneppy/fuku_robot/fuku` and `fuku/utils/helpers`) write `TestZZCount`: build `ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})`, call `fuku.LoadModules(d)`, print `COUNT <n>` and one `CMD <name>` line per name. Run `CGO_ENABLED=1 go test -tags testtools -count=1 -v -run '^TestZZCount$' ./fuku/utils/helpers 2>&1 | grep -E '^(COUNT|CMD) ' > .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-commands.txt` (about 15 s; `*.test` binaries skip the database). Then `rm -f` both throwaway files and confirm `git status --porcelain -- fuku` is empty. The first line is the figure you may state as "command names, aliases included"; planning time saw 231, state what you observe.

2. Write the checker `261115-pvl-check_readme.py` (python3 standard library only, run from the checkout root). CLI: `check_readme.py [--skip-link TARGET]... FILE...` runs the per-file checks (`--skip-link` exempts one named relative link target from the existence test and is for Task 1 only); `check_readme.py --parity README.md README.uk.md` runs parity. The fact sheet is read from the script's own directory (`261115-pvl-FACTS.md`) and the command dump likewise (`261115-pvl-commands.txt`). Exit 1 on any FAIL line; REVIEW lines are printed and do not fail. Per-file checks:
   - names (FAIL): case-insensitive search for the old bot name, the original project name, its developers' handle and name, the word upstream and the word fork. Strip fenced code blocks, inline code spans and URLs first and classify each hit: any hit of the original-project, developer, upstream or fork words anywhere FAILS (even in code); a hit of the old bot name outside code blocks, code spans and URLs FAILS; allowed hits are printed with line numbers so they can be pasted into the fact sheet.
   - brand (FAIL): `Xenobot-72` must not appear as `Xenobot 72`, `Xenobot72`, `XenoBot-72`, `xenobot-72` in prose or with a Cyrillic case ending glued on.
   - links (FAIL): every relative link, image and anchor. Relative targets are resolved against the file's directory with `#fragment` and `?query` stripped and must exist. Anchors resolve against the headings of the target markdown file using GitHub slugs: lower-case, delete every character that is not a Unicode word character, space or hyphen (emoji vanish, Cyrillic letters stay), spaces become hyphens, duplicates get `-1`, `-2`. External links must be https (http is a WARN), and none may contain the original project's host or handle.
   - html (FAIL): `html.parser` stack balance for non-void tags across the whole file; `<img>` has `alt`; `<a>` has `href`; no `style=`, `<script>`, `<iframe>`; every `<details>` has exactly one `<summary>`; every `</summary>` is followed by a blank line (GitHub needs it to render the markdown inside).
   - tables (FAIL): every row of a pipe table has the header's number of unescaped pipes outside code spans.
   - hygiene (FAIL): `TODO`, `FIXME`, `TBD`, `XXX`, `lorem`, `<insert`; secret shapes (bot-token shape digits-colon-35 chars, `sk-`, `ghp_`, a literal `Bearer` followed by 20 or more token characters).
   - figures (FAIL): every number written in prose (outside fenced blocks, inline code spans, URLs, HTML tags and pipe-table cells, which are covered by the command and environment evidence rows of sections C and D) must appear in the "Stated figures" list of the fact sheet (lines of the form `- <number> - <what and the command that produced it>`).
   - commands (FAIL, only for the file whose Commands H2 is `Commands` or `Команди`): every `CMD` name of `261115-pvl-commands.txt` appears as `/name` inside that section; every `/word` in a code span in that section is a registry name or listed in the fact sheet's allow-list of non-registry slash words.
   - ukrainian (README.uk.md only): FAIL on the letters ы э ъ ё outside code, on the calques listed in the rules, and on `Xenobot-72` followed by a Cyrillic letter; REVIEW on first-person masculine past forms (`я` plus a word ending in `в`), on the words `сам себе`, and on the pronouns `ти`, `тобі`, `тебе`, `твій`, `твоя` outside code.
   - voice (REVIEW): never-joke stems in prose (English: starv, famine, hunger, war, death, dead, gore, cannibal, kill, blood; Ukrainian: голод, війн, смерт, кров, канібал, вбив) and, per H2 section, a count of series-reference words (tempe, темпе, Jaya Abadi, ration, пайок, cat photo, facility, об'єкт) so the section caps can be read off.
   Parity mode: same sequence of heading levels and the same number of H2 sections; equal sets of `/commands`, of ALL-CAPS environment names in code spans, of relative link targets (file part); equal counts of fenced blocks, `<details>` blocks and tables; each file links to the other within its first 30 lines.

3. Prove the gate is red on the old content: run the checker on the OLD `README.md` before editing it. It must FAIL (names, the four missing blocklist commands, figures, README.uk.md link). Paste the failing summary into the fact sheet as the baseline; a checker that passes on the old file is wrong, fix it.

4. Build the fact sheet `261115-pvl-FACTS.md` with these sections, each row giving the claim, the exact evidence command or `file:line` (or the AGENTS.md line from `grep -n`), the observed output and a verdict: (A) Identity: version string with file:line, Go version, licence name, module path, binary name, image name. (B) Counts, each with the command that produced it: command names (the dump), module registrations (`grep -rn 'RegisterLegacyModule("' fuku/modules --include='*.go' --exclude='*_test.go' | wc -l`, then say whether you state it as "registered modules" and that help loads last), migrations, languages, lock types (count the `lockMap` and `restrMap` keys and their union), member requirement rules (count `memberreq.Rules`), env names (`grep -rhoE 'Getenv\("[A-Z][A-Z0-9_]+"\)' fuku main.go --include='*.go' --exclude='*_test.go' | sort -u`), handler groups (re-verified list). (C) Command inventory: for each Commands-section group, the names, aliases, arguments, who may run each (read the descriptor's `RequiredChecks` and handler guards, not the old README) and the source `file:line`; this is also where retired commands (`antiraid`, `raidtime`, `raidactiontime`) are shown to be pointer commands. (D) Environment: name, default, required or not, read at `file:line`, plus the default evidence; the five Turnstile and Mini App names and the rule that Turnstile needs all four of `CAPTCHA_WEB_BASE_URL`, `TELEGRAM_MINI_APP_SHORT_NAME`, `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET_KEY` (and that without Redis it is unavailable). (E) Behaviour claims register: one row per sentence-level claim the README will make (Staff Group, lockdown and its automatic triggers with their defaults, captcha modes, settings menu and owner gate, member requirements, scheduled deletion, list pages, forward blocklist, operations). For the forward blocklist state the real limit: a normal member's forward of a group message is labelled by Telegram with the original sender, so only channel posts and a group post made by an anonymous admin can match; confirm in `forwardBlockSourceOf` in `fuku/modules/staff_forwardblock_watch.go` and the `staff_fwdblock_*` keys of `locales/en.yml`, and confirm that only the Staff Group's live creator changes the list while other Staff Group admins can only view. (F) Stale claims register: each seed claim of the context block with verdict true, false (with the live value) or dropped. (G) Stated figures: the only numbers allowed in README prose, one line each with the producing command. Candidate figures to state only if re-verified in code with file:line: the automatic raid trigger defaults (join surge, message flood, AI burst thresholds and windows), the lockdown ban span, the shutdown budget and `stop_grace_period: 75s`. In-world figures ("version 72 of 74") are allowed and are marked in-world. (H) Links inventory (every internal link with `test -e`), the allow-list of non-registry slash words, and placeholders for the name-search and voice review results filled in by Task 3.

5. Write README.md. Plain-text headings (no emoji in headings, so anchors are robust; a discretion call, record it in the fact sheet). Section order, adjust only with a reason recorded in the fact sheet: centred header block (h1 `Xenobot-72`, one-line in-voice tagline, badges for Go 1.26 linking `go.mod`, MIT linking `LICENSE`, gotgbot v2 and PostgreSQL plus Redis, no fork badge, a navigation line that includes `README.uk.md`); a short in-voice introduction (first person is allowed here only; what the bot is, Go on gotgbot, storage, "Jaya Abadi built me to run this facility"; no claim of history, users or statistics; do not hard-code the software version in prose); Table of contents; Features by area (Moderation; Protection: antiflood, locks, blacklists, the AI spam filter, math, text and Turnstile captcha; Staff Group; Lockdown and automatic raid detection; Member requirements; Message deletion; Settings menu with the owner gate, the typed-reply editor and the list pages; Content; Several groups; Languages and backup; Operations); Commands (basis note "counted from the running dispatcher at release v2.24.0" next to the figure; collapsible `<details>` per module with a blank line after `</summary>`; a light flavoured opening line then plain words; cover every registry name with aliases and arguments; a "Retired" block that says `/antiraid`, `/raidtime` and `/raidactiontime` only point to Lockdown and that automatic detection is tuned with `/autolockdown` and `/autoantiraid`; the Staff Group, lockdown, forward blocklist, captcha, settings, member requirements and deletion entries exact and joke-free; `#note`, `@admin` and `/cancel` explained where they belong); Architecture (updated mermaid flowchart with the real handler groups and the raid counters in Redis, plus short bullets on startup, handler groups, data, shutdown and the command-deletion wrapper); Tech stack (re-verify every library and version in `go.mod`); Getting started (BotFather, configure, Docker Compose with the real image and `stop_grace_period: 75s`, the debug stack, from source, Turnstile setup in six lines pointing at `docs/src/content/docs/self-hosting/captcha-web.md` and saying the captcha routes share the HTTP port with `/metrics` so the proxy must forward only `/captcha` and `/captcha/*`, webhook versus polling table; mention PaaS manifests only if each file exists and does not point at another project, with no commentary about where they point); Configuration (every variable of the checker's env set, grouped, defaults and requirements verified against `config.go` and `tracing.go`, the five Turnstile and Mini App names added, Compose passing only five variables noted if still true); Endpoints and observability (`/health`, `/metrics`, `/db_metrics`, `/webhook`, `/debug/pprof/`, and `/captcha`, `/captcha/verify`, `/captcha/health` only when configured, `--health`, `--version`); Project structure (re-verify every path and comment; the tree may show `Fuku_Robot/` and `fuku/` as paths); Development (re-verify Go 1.26, every make target in the Makefile, CI facts, the testtools tag and CGO notes); Documentation (relative links to docs pages proven with `test -e`; no external docs site); License (MIT, full text and copyright notice in `LICENSE`; do not repeat the copyright holder's name in the README; gotgbot, gotg_md2html and the other libraries in `go.mod` may be credited as dependencies). Flavour per the voice rules above.

6. Run the checker on README.md until it exits 0, then work through its REVIEW lines and record each judgement in the fact sheet. Re-read the finished file once against the brief's "never joke" list and the near-zero sections.

7. Commit only the README: `git add README.md`, then `git commit` with a Conventional Commit subject beginning `docs: rewrite README in the Xenobot-72 voice` and the attribution trailer lines your own session reminder prescribes. `git show --stat HEAD` must list exactly `README.md`. Leave the three `.planning` files uncommitted for the quick task's docs commit. README.md links to README.uk.md before that file exists; Task 2 creates it on the same unpushed branch, and the checker's relative-link test is therefore run on README.md only after Task 2 (use `--skip-link README.uk.md` or record that FAIL line as expected for this task only).
  </action>
  <verify>
    <automated>python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py --skip-link README.uk.md README.md && test -z "$(git status --porcelain -- fuku locales docs LICENSE AGENTS.md main.go app.json .goreleaser.yaml docker-compose.yml)" && test "$(git show --name-only --format= HEAD)" = "README.md"</automated>
  </verify>
  <done>The checker exists, was shown red on the old README and is green on the new README.md. The fact sheet covers sections A to H with evidence for every count, command group, env var and behaviour claim, and its Stated figures list contains every number used in prose. README.md covers every registered command name, every env var the code reads, and the Staff Group, lockdown, automatic raid detection, Turnstile captcha, settings menu, member requirements, scheduled deletion, list pages and forward blocklist (with its stated limit); the retired raid commands are described as retired. No throwaway helper file remains, HEAD is one commit touching only README.md, and nothing was pushed.</done>
</task>

<task type="auto">
  <name>Task 2: Original Ukrainian README.uk.md from the same fact sheet (one commit: README.uk.md)</name>
  <files>README.uk.md</files>
  <read_first>
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md
    - README.md (the finished English file: structure and section order only, not a sentence source)
    - /root/.claude/uploads/4ce545b5-6832-57a3-a863-43c972932c7c/2e719ace-xenobot-72-rewrite-instructions.md sections 4, 5, 8 and 9
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py (usage)
  </read_first>
  <action>
Implements D-01 (original Ukrainian text in the same voice, linked both ways) and D-02 (same current facts). This is not a translation: write each paragraph from the fact sheet and the structure, in natural modern literary Ukrainian, with its own sentences and its own joke where the section allows a joke. Do not run the English text through a translation pass and do not mirror its clause order.

1. Keep identical to the English file, character for character: every command name, alias and argument, every env var name, URL, relative link target, file path, code block body and the `Xenobot-72` spelling. Translate only prose, headings, table cell explanations and code comments. Same H2 order and heading levels, same number of `<details>` blocks, tables and fenced blocks, same set of relative links, so parity mode passes. Headings are Ukrainian; the table of contents anchors are Cyrillic GitHub slugs (the checker computes them). The header block is the same centred HTML with the same badges and a link to `README.md` labelled in Ukrainian or English, in the first lines.

2. Voice in Ukrainian: use the glossary table (об'єкт, мешканець, наглядач або адмін, режим тиші, регламент, попередження про порушення, ізоляція об'єкта, збій) sparingly and never instead of the plain word where the reader acts (бан, мут, адмін, капча, локдаун stay). Formal «ви». The bot is feminine when a gendered form is unavoidable ("я перевірила"), and wording that needs no gender is preferred ("Перевірку завершено"). Never assume a user's gender. Wording must work for any number ("Попереджень: N"). "темпе". Ukrainian quotation marks «…». `Xenobot-72` stays in Latin and is never declined: rebuild the sentence instead. Flavour scale exactly as in the voice rules: full to medium in the intro and feature blurbs, a light line then plain words in command tables, near zero in getting started, configuration, endpoints, structure and development, and exact joke-free text for staff, lockdown, raid, captcha, settings, member requirements, deletion and the blocklist. No starvation, war, death, gore or punishment jokes; no cannibalism lines; no gloating in ban or warn descriptions; at most one clear series reference per section, none in most; no names but Xenobot-72 and Jaya Abadi. Avoid Russian calques and surzhyk; the checker rejects the letters ы э ъ ё and a short list of calques, and flags masculine first-person past forms and the informal pronouns for review.

3. Numbers: use only the figures in the fact sheet's Stated figures list, written the same way. Add nothing new; if a Ukrainian sentence needs a number the list lacks, add the evidence row to the fact sheet first or drop the number.

4. Length: keep it as short as the English. Print the ratio of prose characters (outside code) of README.uk.md to README.md and trim if it exceeds 115 %; cut words before meaning.

5. Run `check_readme.py README.uk.md` and `check_readme.py --parity README.md README.uk.md` until both exit 0, then read every REVIEW line (masculine forms, informal pronouns, never-joke stems, series-reference counts per section) and record each judgement in the fact sheet's voice review part. Read the whole file once aloud in your head for calques, ви/ти consistency and the feminine form.

6. Commit only the Ukrainian file: `git add README.uk.md`, `git commit` with a subject beginning `docs: add original Ukrainian README` and the attribution trailer lines your own session reminder prescribes. `git show --stat HEAD` must list exactly `README.uk.md`. Do not push and do not create a branch.
  </action>
  <verify>
    <automated>python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py README.md README.uk.md && python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py --parity README.md README.uk.md && test "$(git show --name-only --format= HEAD)" = "README.uk.md"</automated>
  </verify>
  <done>README.uk.md exists in original Ukrainian with the same section order, command set, env var set, link targets and block counts as README.md; both files pass the checker with no skipped link; the two files link to each other in their first lines; the Ukrainian-specific checks pass and every REVIEW line has a recorded judgement; HEAD is one commit touching only README.uk.md.</done>
</task>

<task type="auto">
  <name>Task 3: Cross-file consistency, name search, links, voice read-through and follow-up list (fix-up commit only if needed)</name>
  <files>.planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md, README.md, README.uk.md</files>
  <read_first>
    - README.md and README.uk.md in full
    - .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-FACTS.md
    - /root/.claude/uploads/4ce545b5-6832-57a3-a863-43c972932c7c/2e719ace-xenobot-72-rewrite-instructions.md sections 4, 5 and 8
  </read_first>
  <action>
Implements D-01, D-02, D-03 as a closing pass. No claim is added here that lacks an evidence row.

1. Re-run the oracle. Repeat the throwaway registry dump of Task 1 step 1 into a temporary file under your session scratchpad directory (never inside the repository), `diff` it against `261115-pvl-commands.txt`, delete the throwaway test files again and confirm `git status --porcelain -- fuku` is empty. A difference means the code changed: update the commands file, the fact sheet and both READMEs. Also re-run the env name command and the module and migration counts; each must still match the Stated figures list.

2. Name search. Run `grep -niE 'fuku|alita|divideprojects|divkix|upstream|fork' README.md README.uk.md` and the checker's names check. Paste every remaining hit into the fact sheet's name-search part with its line and the reason it is an allowed identifier (clone URL, image, module path, structure tree, Redis key prefix, env name, binary name). Any hit in prose, any hit of the original project, developer, upstream or fork words, and any "fork" or "upstream" in either file is fixed, not justified. Confirm neither file links to the original repository, its support group or its update channel, and that nothing says or hints at another name for the bot.

3. Links and HTML. The checker's links and html checks pass for both files with no skipped link. Every docs link target exists (`test -e`); every external host is listed in the fact sheet's links inventory and none is the original project. The centred header block is balanced and uses only tags GitHub renders.

4. Voice read-through, both files, against the scale: the REVIEW lines from the checker are all judged; the near-zero sections (getting started, configuration, endpoints, structure, development, staff, lockdown, raid, captcha, settings, member requirements, deletion, blocklist) hold no jokes and no series references; no section has more than one clear reference; no never-joke subject; no gloating near a ban or warn; no invented figure, date, user count or history; the intro does not hint at another name; the in-world "version 72 of 74" appears at most once and cannot be mistaken for the software version. Ukrainian: formal ви throughout, feminine or gender-free, no calques, nothing declined after `Xenobot-72`, the glossary used consistently.

5. Parity and truth. `--parity` passes. Spot-check ten behaviour sentences at random from README.md against their evidence rows and re-open the cited `file:line` (not the old README) to confirm they still say the same; fix any drift in both files. Confirm the Turnstile paragraph states that all four settings are required, that math and text need none of it, that without Redis the web captcha is unavailable, and that the proxy must forward only the captcha paths; confirm the forward blocklist paragraph states the limit and that only the Staff Group's creator changes the list.

6. Follow-up list for the final report (do not edit anything outside the two READMEs). Run `git grep -nIiE 'divkix|divideprojects|alita' -- . ':!.planning' ':!LICENSE'` and `git grep -cIi 'fuku' -- docs locales app.json .goreleaser.yaml docker-compose.yml debug.docker-compose.yml render.yaml fuku/db/devs fuku/modules/help.go` and record, per file with hit counts, where the old name or the original developers are still readable (locales, docs pages and the generated docs index figures, the OCI labels in `.goreleaser.yaml`, `help.go`, the developer `/stats` text, `AGENTS.md` and `.claude/CLAUDE.md` intro lines, compose comments). Mark which are user-visible. Also record that the docs pages linked from the READMEs still carry the old name.

7. Fix-up commit only if steps 1 to 5 changed a README: `git add README.md README.uk.md` (only the files changed), `git commit` with a subject beginning `docs: fix README` and the attribution trailer lines your own session reminder prescribes. If nothing changed there is no commit. Finish by confirming `make check-docs` passes (proof that no docs page drifted) and that `git diff --name-only 83dd98e..HEAD -- . ':!.planning'` lists exactly `README.md` and `README.uk.md`. Do not push, do not create a branch, do not touch `.planning` files in a code commit (the fact sheet, command dump and checker are committed by the quick task's docs commit).
  </action>
  <verify>
    <automated>python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py README.md README.uk.md && python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py --parity README.md README.uk.md && make check-docs && CHANGED=$(git diff --name-only 83dd98e..HEAD -- . ':!.planning') && test "$(printf '%s\n' "$CHANGED" | sort | tr '\n' ' ')" = "README.md README.uk.md " && test -z "$(git status --porcelain -- fuku locales docs LICENSE AGENTS.md main.go app.json .goreleaser.yaml docker-compose.yml)"</automated>
  </verify>
  <done>Both files pass the checker and parity with no skipped link; the name search shows only justified identifiers inside code; the commands, module, migration, language and env counts still match a fresh run; the fact sheet holds the name-search results, the voice review notes and the follow-up list of places that still carry the old name or the original developers; `make check-docs` passes; code commits touch only README.md and README.uk.md; nothing was pushed and no branch was created.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| README text to public readers | Operators copy commands, env vars and proxy advice from the README into production |
| Repo to public history | Anything written into README.md and README.uk.md is public once the branch merges |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-261115-pvl-01 | Information disclosure | README text and examples | medium | mitigate | Only placeholders in examples; the checker rejects bot-token, `sk-`, `ghp_` and long bearer shapes; Task 3 re-reads the Configuration and Getting started sections for real IDs, domains or secrets |
| T-261115-pvl-02 | Tampering | Turnstile and metrics exposure advice | high | mitigate | The Turnstile paragraph and the Endpoints section must state that the captcha routes share the HTTP port with `/metrics`, `/db_metrics`, `/health` and the webhook, that the public proxy must forward only `/captcha` and `/captcha/*`, that `METRICS_AUTH_TOKEN` and `CAPTCHA_WEB_TRUSTED_PROXIES` matter, and that the secret key is never to be committed; verified against captcha-web.md and AGENTS.md in Task 1 and re-checked in Task 3 |
| T-261115-pvl-03 | Spoofing | Misleading operational claims (retired raid mode shown as live, wrong defaults, wrong webhook path, wrong image) | medium | mitigate | Every behaviour and default claim carries a fact-sheet evidence row from code; the command registry dump is the coverage oracle; a spot check of ten claims in Task 3 |
| T-261115-pvl-04 | Repudiation | Old name or original developers readable in either file | low | mitigate | The checker's names check fails on any prose hit; Task 3 pastes every remaining hit into the fact sheet with its justification |
| T-261115-pvl-05 | Elevation of privilege | Throwaway test files left in the tree and committed | low | mitigate | Deleted in the same step that creates them; every task's verify asserts a clean `fuku` path and commits are made with explicit file paths |
| T-261115-pvl-SC | Tampering | npm/pip/cargo installs | low | accept | No package is installed: the checker uses the python3 standard library and the registry dump uses the existing Go module |
</threat_model>

<verification>
- `python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py README.md README.uk.md` exits 0 and the same script with `--parity README.md README.uk.md` exits 0.
- `make check-docs` passes (no docs drift) and `git diff --name-only 83dd98e..HEAD -- . ':!.planning'` lists exactly `README.md` and `README.uk.md`.
- A fresh registry dump equals `261115-pvl-commands.txt`; every name in it is in the Commands section of both files.
- The fact sheet's name-search part justifies every remaining hit, and its follow-up list names the places outside this task that still carry the old name.
</verification>

<success_criteria>
- D-01: two files linked both ways, the Ukrainian one original text with the same structure, commands and variables.
- D-02: both files describe the bot as it is now, from code and docs; the dev-branch roadmap, the fork-sync section, upstream and fork wording and all credit are gone; milestone v1.1 and the forward blocklist are covered with the blocklist's stated limit.
- D-03: no sentence of the old README survives without an evidence row; the retired raid mode is described as retired; the stale counts and diagram are replaced with verified ones.
- Voice follows the owner's scale, glossary, never-joke list and the Ukrainian gender and address rules; clarity wins wherever character and clarity conflict.
- Two atomic code commits (plus at most one fix-up), only the two READMEs, nothing pushed, no branch created.
</success_criteria>

<output>
Create `.planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-SUMMARY.md` when done.
</output>

<!-- planner-discipline-allow: fuku -->
<!-- planner-discipline-allow: alita -->
<!-- planner-discipline-allow: divideprojects -->
<!-- planner-discipline-allow: divkix -->
<!-- planner-discipline-allow: upstream -->
<!-- planner-discipline-allow: fork -->
