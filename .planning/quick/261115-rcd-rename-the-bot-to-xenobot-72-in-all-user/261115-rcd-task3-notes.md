# 261115-rcd Task 3 notes (untracked, never committed)

Commit: 61a3401 "docs: rename the docs site to Xenobot-72 and remove the original developers' links" (on top of 98d363d). Branch claude/ecstatic-goldberg-qsss05. 33 docs files, explicit paths, nothing under .planning/.

## Baselines (before any docs edit, on the untouched pages)

- Checker counts mode over docs/: exit 1, 17 hits:
  blume.config.ts '30+ modules', '159+ commands', '7+ languages'; api-reference/commands.md 'Total Modules: 30', 'Total Commands: 160'; commands/index.mdx 'Total Modules: 30', 'Total Commands: 159'; pages/index.astro '7+ Languages', '158+ commands' x2, '29+ modules', '7+ languages'; language lists without Ukrainian: introduction.mdx (Multi-Language bullet), commands/index.mdx (language card), pages/index.astro (language card body); locale trees without uk.yml: architecture/project-structure.mdx and architecture/module-pattern.md (the second one was not named in the plan: its sentence listing the other locale files is fixed too).
  SKIP lines: per-module card badges of commands/index.mdx (27 badges, sum 159), the Popular/Core/Step/i18n style badges of other pages, and backup/index.md '17 modules' (the modules a backup file holds, a different subject).
- Checker prose mode over docs/: exit 1, 94 hits (11 original-developer hits in docs/README.md, pages/index.astro, languages/index.md, contributing/index.mdx, quick-start.mdx; 83 old-name hits in 32 files). Six of the 83 were the lowercase compose service name or user name `fuku`: three inside indented code fences (docker.mdx 249 and 254, binary.mdx 147) and three in real prose (docker.mdx 136, 203, 335); see the checker fix below.
- Stale-figure grep of the verify step: 7 hit lines at HEAD (blume.config.ts:8, pages/index.astro:30, :58, :107, api-reference/commands.md:13, :14, commands/index.mdx:8), as the plan expected.
- Dispatcher dump regenerated into 261115-rcd-commands.txt: COUNT 231, identical (CMD lines and COUNT) to the README task's 261115-pvl-commands.txt. RegisterLegacyModule count outside tests: 39. Locale files: 8 languages (config.yml is the help alt-name pseudo-locale). The two throwaway test files were created and deleted in the same shell command; `git status --porcelain -- fuku` empty.

## Checker change (own tooling, untracked)

The fence stripper of `prose` mode only recognised fences at the start of a line; docs pages put code fences inside indented Tabs and Steps blocks, so `docker compose logs -f fuku` and `useradd ... fuku` looked like prose hits. FENCE now allows leading whitespace (`^[ \t]*(```+|~~~+)...`). The compose service name in real prose and headings was backticked instead (docker.mdx: heading, resource table cell, one sentence), so the zero is not hidden by the regex change.

## Edits and classes

| Place | Class | Decision |
|---|---|---|
| 33 docs files, "Fuku Robot" and standalone "Fuku" in prose, titles, descriptions, link text, the compose header comment copy in docker.mdx, a mermaid label, a bash comment in caching.md | b | renamed to Xenobot-72 (docs are developer-facing: rename only, no voice rewrite) |
| docs/README.md Live Documentation line and Support Group line | c | removed; the repository line kept and renamed |
| pages/index.astro "Need help?" block | c | support group and updates channel links removed; the GitHub issues link kept |
| commands/languages/index.md Crowdin sentence (frozen page) | c | removed, with its blank line |
| contributing/index.mdx Support Group line | c | removed; GitHub issues line kept |
| getting-started/quick-start.mdx | c | "@FukuRobot" and the t.me/FukuRobot?startgroup links replaced by "your bot's username" (BotFather link, which is a general Telegram tool, kept) and a placeholder URL `https://t.me/<your_bot_username>?startgroup=true` in a code span; support-group clause removed from the tip. No username invented |
| getting-started/introduction.mdx | c | sentence that sent readers to the hosted bot by name replaced by a pointer to Quick Start and Self-Hosting |
| commands/index.mdx `/start@FukuRobot` | c | placeholder `/start@your_bot_username` |
| docs/blume.config.ts github owner | d (decision 4) | `{ owner: "uasneppy", repo: "fuku_robot" }`: this repository's own owner and name from the module path and the clone URL already in the repository. The `site` URL was already this repository's |
| frozen help/index.md and api-reference/commands.md rows for /about and /donate | d | made false by Task 1; reworded to match help_help_msg ("Show bot information and version", "Show whether this bot accepts donations") |
| Language lists: introduction.mdx, commands/index.mdx, pages/index.astro; trees: project-structure.mdx (uk.yml line added), module-pattern.md (uk.yml added to the sentence) | d (decision 12) | all eight, Ukrainian included |
| Figures: commands/index.mdx, api-reference/commands.md Overview (frozen, by hand), blume.config.ts, pages/index.astro | d (decision 12) | 39 modules, 231 command names (aliases included), 8 languages. Overview of commands.md: user-facing/internal split dropped. api-reference/commands.md front-matter description reworded to "all commands in Xenobot-72" because "Xenobot-72 commands" reads as "72 commands" to the counts mode |
| docker.mdx `fuku` service name in a heading, a table cell, a sentence | a | kept, backticked |
| binary.mdx nssm service name `FukuRobot` (5 lines in a code block), `useradd ... fuku`, docker/compose paths | a | untouched (nssm-style service names in code blocks, plan class a) |
| monitoring.md alert name `FukuUnhealthy` | a | untouched (alert name identifier in code) |
| docs/package.json name `fuku-robot-docs`, bun.lock, wrangler.jsonc, robots, source URLs `uasneppy/fuku_robot` | a | untouched |
| "Fork the repository", "Push to your fork" (docs/README.md, contributing, source.mdx) | d (decision 6) | kept: contributor instructions |

## Counts: why the cards and tables undercount

- commands/index.mdx: 27 per-module cards, badges sum to 159 command names (the old Total Commands figure); the page now says 231. Left as they are.
- api-reference/commands.md: per-module tables have 162 command rows and the Alphabetical Index 161; both are hand lists that miss commands. Left as they are.
- docs/src/content/docs/commands/ holds 34 module directories against 39 registered modules; the generator and `make inventory` count only what they parse, so they undercount (never copied: the index figures come from the dispatcher dump and the registration grep).

## Checks run (all after the edits)

- checker prose docs: 0 hits. checker counts docs: 0 hits (SKIP lines only: card badges, other badges, backup page).
- dev-pattern grep over docs: empty. Old-name grep over docs/blume.config.ts: empty. Old-name grep over all of docs: only the nssm `FukuRobot` service name lines (class a). Stale-figure grep: empty. `grep uk.yml` project-structure.mdx: found.
- Positive greps: Total Modules**: 39 and Total Commands**: 231 in commands/index.mdx and api-reference/commands.md; COUNT 231 in the dump; 39 registrations.
- make generate-docs: changed nothing (git status identical before and after); make check-docs: no drift. make check-translations: pass; checker locales: 0 failures.
- Docs site build: ran offline-safe in a copy under the scratchpad (never in the repository): `bun install --frozen-lockfile` (809 packages, through the proxy, local node_modules in the copy only) then `bun run build` (blume build): 63 pages built, exit 0; a scan of the built dist for DivideSupport, DivideProjects, divkix, crowdin, FukuRobot? and "Fuku Robot" found nothing. No link checker exists in docs/ or CI beyond the build.
- README.md, README.uk.md, LICENSE, migrations, go.mod, go.sum, app.json: byte-identical to base a8e2541.
- Repository-wide greps (verification section): PAT-DEV outside .planning, migrations, LICENSE, READMEs and GSD tooling: empty. PAT-NAME-WORD over fuku, main.go, .github, packaging, scripts, AGENTS.md, .claude/CLAUDE.md excluding tests: empty.
- `git status --porcelain`: only ` M .planning/PROJECT.md` and the untracked quick directory. `git log a8e2541..HEAD`: exactly the three commits (a721b5a, 98d363d, 61a3401), none touching .planning/.
- No Go file edited in Task 3, so the lint stand-in was not needed. make lint cannot run here (golangci-lint built with go1.25, repo targets 1.26.0).
- make test NOT run: the orchestrator runs it.

## Left for the orchestrator

.planning/PROJECT.md is edited (Task 2) and uncommitted on purpose; commit it, with these quick-task files, in the final docs commit (explicit paths).
