# 261115-pvl Task 1, steps 0 to 4: notes

Executor scope: guard, command-registry dump, checker, red baseline, fact sheet. No README written, nothing committed, nothing pushed, no branch created. Tree state at the end: `git status --porcelain` shows only `?? .planning/quick/261115-pvl-rewrite-readme-md-in-the-xenobot-72-voic/`.

## What was built (all untracked, in the quick task directory)

- `261115-pvl-commands.txt`: `COUNT 231` plus 231 `CMD <name>` lines, from the real dispatcher.
- `261115-pvl-check_readme.py`: the README checker (python3 standard library only). Per-file checks: names, brand, links and anchors, html, tables, hygiene, figures, commands, ukrainian (only for `*.uk.md`), voice, plus a per-file sibling-link check. `--parity A B` mode. `--skip-link TARGET` supported.
- `261115-pvl-FACTS.md`: sections A (identity), B (counts and handler groups), C0 (partition of all 231 names into 29 groups), C1 (inventory with who may run each and evidence), D (environment, endpoints, compose facts, shutdown), E (35 behaviour claims E1 to E35), F (stale claims register), G (Stated figures, 42 number tokens), H (links inventory, allow-lists, placeholders for Task 3), I (the red baseline).

## Commands run and results

1. Guard: `git branch --show-current` printed `claude/ecstatic-goldberg-qsss05`; `git status --porcelain` showed only the untracked quick directory. HEAD `83dd98e`. SEQUENTIAL=true (no commit made, so the HEAD assertion was not needed).
2. Registry dump: wrote `fuku/utils/helpers/zz_export_test.go` (package `helpers`, `ZZNames` reading `botCommands` under `botCommandsMu.RLock()`) and `zz_count_test.go` (package `helpers_test`, `fuku.LoadModules` on `ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})`). `CGO_ENABLED=1 go test -tags testtools -count=1 -v -run '^TestZZCount$' ./fuku/utils/helpers` passed in 0.04 s and printed `COUNT 231`. Both files were deleted in the same command; `git status --porcelain -- fuku` printed nothing.
3. Red baseline on the OLD `README.md`: `python3 <dir>/261115-pvl-check_readme.py README.md` exit 1, `SUMMARY fails=99 reviews=2`. FAIL lines: names 83, links 9, figures 6, commands 1. Highlights: forbidden strings (`fork`, `Divkix`, `Alita`, `upstream`) from line 7 on; external links to the original project (lines 7, 12, 976, 990 to 992); `no link to README.uk.md within the first 30 lines`; `13 registry command(s) missing from the Commands section: /addfwdblock /autolockdown /fwdblockaction /fwdblocklist /linkstaff /lockdown /lockdownstatus /rmfwdblock /setstaff ...`; six numbers not in the fact sheet (18, 658, 9, 2020, 2025, 2026). The full condensed output is pasted in FACTS section I. The first baseline run (with an empty figures list) gave `fails=123`; the figure FAILs fell from 31 to 6 once section G existed.
4. Checker self-test on throwaway files in the session scratchpad: a clean sample passes html, links, tables and figures checks; a deliberately broken sample trips brand, names, links, anchors, html (img alt, unclosed div, `</summary>` blank line), tables, hygiene, figures and ukrainian checks as intended; `--parity` returns exit 0 for a matching pair.
5. `make check-docs`: "No drift detected" (nothing outside the quick directory was changed).

## Deviations from the plan, with reasons

- Commands check is scoped to the Commands H2 section, so the old README fails with 13 missing names. The plan's "four missing" counted names absent from the whole file. Both are consistent; the section-scoped one is what the plan's checker spec asks for.
- Tables check counts every unescaped pipe, code spans included (the plan says "outside code spans"). GitHub splits cells on pipes inside code spans, so a literal `|` in a code span inside a table must be written `\|`. Writers must escape the pipes in argument hints such as `[joins\|flood\|ai]`.
- Added a per-file check not in the plan: `README.md` must link `README.uk.md` within its first 30 lines and the reverse. This is what makes the baseline red on the missing sibling link even without `--parity`, and it is run before the sibling exists (so Task 1 must still pass `--skip-link README.uk.md` for the existence test, but the link text itself must be present).
- Parity mode ignores the two cross-links when comparing relative link target sets (each file legitimately links to the other).
- The checker's heading-step stripping removes leading `1.`, `3a.` and `### 3a.` markers before the figures test; ordered list numbers are not "figures".
- The checker's figures test also reads `alt` and `title` attribute text (badge alt text is reader-visible).
- The fact sheet is a skeleton-first file. FACTS section H4 and H5 are placeholders for Task 3.

## Stated figures (the only numbers allowed in prose)

231 (command names), 2.24.0 (release), 1.26 (Go), 2, 8 (languages and member rules), 22 (lock types), 39 (module registrations), 64 (migrations), 6 (log categories), 3, 5, 10, 15, 30, 60, 120, 24, 330, 500, 100, 150, 200, 366, 47, 80, 90, 75, 78, 17, 16, 7, 8080, 1, 0, 20, 12, 29, 72 and 74 (in-world only: version 72 of 74), 4, 57, 56. Each has its meaning and evidence line in FACTS section G. Use a number only in the sense given there. Numbers inside code spans, code blocks, URLs and pipe-table cells are not checked, but the evidence for the configuration table values is in FACTS section D.

## What the README writers must know

- Fact sheet sections: A identity, B counts and handler groups, C0 partition, C1 inventory by group, D environment (table with defaults, requirements, file:line), endpoints, compose facts, shutdown, E behaviour claims, F stale claims, G stated figures, H links and allow-lists.
- Forbidden strings (checker FAILS, even inside code): `divkix`, `divideprojects`, `divanshu`, `alita`, the original repository path, the word `upstream`, the word `fork` and its forms. The old bot name `fuku` may appear only in code blocks, code spans, URLs, link targets and HTML attributes, never in prose or `alt` text. `uasneppy` is fine. Do not repeat the LICENSE copyright holder's name.
- Claims I could NOT fully verify, which writers must not use as stated:
  - Permissions marked `T` in FACTS C1 (greetings, rules, reactions, federations, connections, disabling, reports details, `/setlog`'s own admin check) rest on the shipped help text only. Use the help text's own words or leave the permission out.
  - `/staff` has no member check of its own (`staff.go:171-190`); only say it works inside a Staff Group.
  - Badge URLs (`img.shields.io`) and the `typesafe.ai` link were not fetched; do not link `typesafe.ai`.
  - Page count of the settings menu: AGENTS says 18 grid cells plus Access; I listed page names (E23) and did not verify a count, so state no count.
- Surprising findings:
  - The shipped help text for captcha writes `/captchaattempts`, which is not registered. The README must use `/captchamaxattempts`.
  - `/clearall` (notes), `/stopall` (filters) and `/resetallwarns` are group-creator-only with a confirmation; the old README did not say so.
  - `/adminlist` needs the bot to be an admin.
  - Compose passes only five variables from `.env`; Turnstile and `METRICS_AUTH_TOKEN` need an override file. The stack uses fixed credentials (`fuku` and `redis`); neither service publishes a port.
  - The Russian-letters member rule is the four letters ы ъ э ё. `README.uk.md` is checked for those letters outside code, so put them in a code span or leave them out.
  - A forwarded message from a normal member's own post in another group is labelled with that member, so it can never match the forward blocklist (E26).
- Stale claims in the old README beyond the plan's list (FACTS F, last five rows): the "build a local override because the stock image is another project's" advice, "/adminlist is for everyone", missing creator-only notes, the `/captchaattempts` mismatch, unversioned debug stack. The old Getting started and Configuration text is otherwise largely accurate (defaults re-verified in FACTS D); the old table lines for `OTEL_*`, `PSQL_*`, `FUKU_TEST_MIGRATION_CHAIN` are correct.
- Still carrying the old name or the original developers outside the READMEs (for Task 3's follow-up list, observed now): `.goreleaser.yaml:115-116` and `docker/alpine`, `docker/alpine.debug`, `docker/pr-build` (OCI vendor, authors, url labels), `render.yaml` (service names), `Procfile` (`bin/Fuku_Robot`), docs pages and the generated docs index, `locales/*.yml` help texts (for example `languages_help_msg` and `help_help_msg`), `AGENTS.md` and `.claude/CLAUDE.md` intro lines, compose comments, `sample.env` database name.
- Docs generator note: the docs index says "30 modules, 159 commands" and `make inventory` another number; both undercount (they parse source patterns). Cite neither.
- Do not read `AGENTS.md` whole; section names used in the evidence column are valid `grep -n` anchors.
