# 261115-pvl Task 1, steps 5 to 7: notes

Commit: `6733c61` (`docs: rewrite README in the Xenobot-72 voice`) on `claude/ecstatic-goldberg-qsss05`, parent `83dd98e`. `git show --stat HEAD` lists exactly `README.md` (487 insertions, 440 deletions), no file deletions, nothing under `.planning/`. Nothing pushed, no branch created. `git rev-list --count 83dd98e..HEAD` is 1.

README.md is 1052 lines.

## Commands run and results

- Guard: branch `claude/ecstatic-goldberg-qsss05`, not protected, SEQUENTIAL=true, plan HEAD ledger written to `.git/gsd-plan-head-before-261115-pvl-01` (`83dd98e`).
- Final checker, run from the checkout root: `python3 .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/261115-pvl-check_readme.py --skip-link README.uk.md README.md` printed `OK [commands] README.md: all 231 registry commands appear in the Commands section`, `SUMMARY fails=0 reviews=1`, exit 0. The one INFO line `skipped existence test for README.uk.md` is the expected skip; the link itself is present on line 15 (the per-file sibling check passes).
- First run had 1 FAIL (the number 256 in "SHA-256", now in a code span) and 2 REVIEWs (spelled-out numbers, and the stem "kill" in "Docker does not kill the process", reworded). The spelled-number REVIEW remains and is judged in FACTS H5.
- No code was run besides the checker; no helper file was created in the repo (my part files live in the session scratchpad only).

## Section outline of README.md

Centred header (h1 Xenobot-72, tagline, four badges, nav line with `README.uk.md`), a three-paragraph in-voice introduction, then H2s in this order:

1. Table of contents
2. Features (H3: Moderation, Protection, Staff Group (with the Forward blocklist as a bold run-in paragraph), Lockdown and automatic raid detection, Member requirements, Message deletion, Settings menu, Content, Several groups, Languages and backup, Operations)
3. Commands (basis note with 231 and v2.24.0; H3: Moderation [Bans, Mutes, Warns, Purges, Admin, Approvals], Protection [Antiflood, Lockdown, Captcha, AI spam filter, Blacklists, Locks], Staff Group [one details block], Content [Notes, Filters, Greetings, Rules, Pins, Reactions, Formatting], Several groups [Federations, Connections, Log channels, Reports, Disabling], Utilities [Misc, Backup languages help, Dev, Settings], Retired [Raid mode: antiraid raidtime raidactiontime]); 30 details blocks in all, one note on the internal `Users` and `BotUpdates` modules
4. Architecture (mermaid flowchart with the real handler groups, five bullets, a handler group table)
5. Tech stack (table)
6. Getting started (Prerequisites; 1. Create the bot with BotFather; 2. Configure; 3a Docker Compose; 3b debug stack; 3c from source; Turnstile web captcha (six numbered steps); Other deployment targets; Webhook or polling table)
7. Configuration (nine details blocks: Required, Redis and cache, Telegram updates and HTTP, Database pool and migrations, AI checks, Web captcha (Turnstile), Monitoring maintenance and profiling, Tracing, Read by tooling)
8. Endpoints and observability
9. Project structure
10. Development
11. Documentation
12. License

Counts for the Ukrainian writer to mirror (parity checker): fenced blocks, details blocks and tables are what they are in README.md; the structure must be identical. Quick counts: `grep -c '<details' README.md` and the checker's `--parity` mode will tell.

## Claims I chose to leave out, and why

- Permissions marked T in FACTS C1: used only help-text wording ("These are admin commands" for Antiflood, Greetings, Rules, Disabling; the roles of the federation table are labelled "come from the shipped help text"). `/setlog`'s own admin check is not stated. `/staff` is described only as "in the Staff Group"; the command is not given an admin check.
- Settings menu: no page count; page names listed.
- The old README sentence that a photo hidden by privacy settings counts as missing (live Telegram behaviour, not proven by a fake): stated only as "No profile photo that I can see".
- `typesafe.ai` link: not used (TypeSafe is named without a link).
- Old claims dropped: "the stock compose image is another project's", dev-branch roadmap, sync instructions, credit text, `inventory` make target (its output undercounts), "docs deploy needs Cloudflare secrets", the Cloudflare Workers statement about the docs site (only "Blume on Bun" kept), "Admin bots receive every group message, so privacy mode does not need to change" (not verified), the 18 handler-group diagram with `antiraid`.
- `/captchaattempts` (help-text spelling) not used: `/captchamaxattempts` is the registered name.
- The Staff Group's `w` duration unit: stated (FACTS C1 says the staff commands also take `w`; `bans_help_msg` lists only `m`, `h`, `d`), and I re-checked `ParseDurationToken` (`fuku/utils/extraction/extraction.go:83-89`: m, h, d, w are all cases), so it is verified.
- Compose: stated the five passed variables, the override-file route (`local.docker-compose.yml`, which `.gitignore` line 27 lists), the demo credentials, 8080 published, `stop_grace_period: 75s`. The statement "Compose reads `${...}` from `.env`" is standard Compose interpolation, not from the fact sheet; it is true for a `.env` next to the compose files.
- PaaS manifests: named with links, no commentary; none of the five files mentions the original project (grep checked). `render.yaml` and `Procfile` still carry the old name inside their own text (follow-up list for Task 3), and the `Procfile` start path was not verified to work.

## Deviations from the plan

- None in scope or constraints. README written from the fact sheet, committed alone.
- Choices recorded: the forward blocklist is a bold run-in paragraph inside the Staff Group H3 (not its own heading), Lockdown appears twice (Features has the behaviour, Commands has the table), the Staff Group commands are one details block, and the handler groups are given as a table (numbers in tables are not figure-checked, and groups 9 and 11 are not in the Stated figures list).
- MESSAGE_DUMP is described as "Chat or channel ID (a channel ID looks like `-100…`)" because `ValidateConfig` rejects only 0 even though its message says "greater than 0" (`fuku/config/config.go:337-339`). Do not copy "greater than 0" from FACTS D into the Ukrainian file.

## What the Ukrainian writer must know

- Same H2/H3 order, same 30 details blocks in Commands (39 `<details>` in the file), same nine in Configuration, same fenced blocks (mermaid, the structure tree, bash, yaml, ini), same tables, same relative links (all docs links in README.md: `docs/src/content/docs/api-reference/lock-types.md`, `.../architecture/handler-groups.md`, `.../architecture/caching.md`, `.../architecture/request-flow.mdx`, `.../self-hosting/{captcha-web.md,webhooks.md,monitoring.md,profiling.md,docker.mdx,source.mdx,database.md,troubleshooting.md}`, `.../api-reference/environment.md`, `docs/README.md`, `AGENTS.md`, `LICENSE`, `go.mod`, `migrations`, `sample.env`, `docker-compose.yml`, `debug.docker-compose.yml`, `docker/alpine.debug`, `render.yaml`, `railway.toml`, `app.json`, `Procfile`, `nixpacks.toml`, `.github/workflows/ci.yml`, `fuku/config/config.go`, `fuku/utils/tracing/tracing.go`) and the same `/command` set in prose and spans (the parity mode compares slash words of the whole file, including `/newbot`, `/newapp`, `/myapps`, `/health`, `/metrics`, `/db_metrics`, `/webhook`, `/captcha` routes and `/debug/pprof/`).
- Env names in code spans must match: the same ALL-CAPS set as README.md (includes `OTEL_*`, `POSTGRES_*`, `PSQL_DB_*`, `TELEGRAM_API_*`, `FUKU_TEST_*`, `RESOURCE_*`). Table descriptions and prose are what you translate.
- Link text for paths with the old name must keep the code span (`` [`fuku/config/config.go`](fuku/config/config.go) ``): bare text would trip the names check.
- Jokes and flavour lines I wrote that need their own Ukrainian counterpart (do not translate word for word):
  - Intro: "I am Xenobot-72, version 72 of 74. Jaya Abadi built me to run this facility. A facility is a Telegram group." (the one in-world fact; keep "72 з 74" or similar once only), "because I do not sleep", and the advert shape "No sweat. I will handle it." (tagline also says "No sweat").
  - Moderation: "Procedure first. Rights are checked every time, mine included." Protection: "Spam gets the same treatment as everything else: a procedure, and no opinions."
  - Content: "Notes, filters and greetings are where I get to be friendly. Today's ration is tempe." (the only food joke besides Greetings in Commands).
  - Commands one-liners: Bans "Notice. Both the admin and I must hold the right to restrict members. I check each time."; Mutes "Notice. Same rights as for bans."; Warns "Notice. I keep the count."; Purges "Cleaning procedure."; Admin "Supervisor access is granted by procedure." (supervisor = the glossary word for admin, used once); Approvals "I keep the list."; Antiflood "Flood control. I count the messages so that you do not have to."; Blacklists "Words I remove on sight. You decide which."; Notes "Notes. I remember so that the group does not have to."; Filters "When a trigger appears, I answer."; Greetings "Today's ration is tempe." (second and last tempe); Rules "Regulations. Every facility has them." (regulations and facility from the glossary); Pins "Pinned notices stay at the top."; Reactions "I acknowledge a keyword with an emoji."; Log channels "Everything I do gets filed."; Reports "A report reaches the admins."; Misc "Small requests are processed too."; Dev "Request denied for everyone else."
  - Everything in Staff Group, Lockdown, member requirements, deletion, settings, captcha, forward blocklist, Getting started, Configuration, Endpoints, Structure, Development is plain; keep it plain. Section order, not wording, is what must match.
- Terms chosen: "Staff Group" is kept as a proper name (Ukrainian should pick one rendering and keep it; the locale files use it, check `locales/uk.yml` for `staff_*` keys), "lockdown" stays (локдаун), "captcha" (капча), "ban/mute/kick/warn" as Telegram users know them, "Turnstile", "Mini App", "federation" (федерація), "Requirements" is the settings page name for member requirements, "Self-destruct", "Edited messages", "Commands" are the Deletion page's own names (check `locales/uk.yml` `settings_del_*` for the Ukrainian words and reuse them). The member-rule letters `ы ъ э ё` must stay inside a code span (the Ukrainian checker rejects them in prose).
- The forward blocklist limit and "only the Staff Group's live creator changes the list" are exact statements and must be carried over without softening.
- The Turnstile steps must keep: all four settings or none; math and text need none of it; without Redis the web captcha is unavailable (math captcha fallback); proxy forwards only `/captcha` and `/captcha/*`; never commit the secret key.
- Compose facts to carry over exactly: five passed variables (`BOT_TOKEN`, `OWNER_ID`, `MESSAGE_DUMP`, `WEBHOOK_DOMAIN`, `WEBHOOK_SECRET`), the override file, demo credentials `fuku`/`fuku` and `redis` (inside code spans), 8080, `stop_grace_period: 75s`.
- Figures: use only those in FACTS G. README.md's prose uses 231, 2.24.0, 1.26, 72 and 74 (in-world), 8, 22, 39, 6, 3, 2, 5, 10, 15, 30, 60, 120, 24, 330, 500 (in a table only), 100 (tables), 150, 200, 366, 47, 80, 90, 75 (code span), 78, 17, 16, 7, 8080, 1, 0 and 12 (tables), 64 (structure tree only). The Ukrainian prose may reuse these.
- Ukrainian prose length: README.md has about 487 net new lines; keep the Ukrainian within 115 percent of the English prose characters (the parity mode prints the ratio).
