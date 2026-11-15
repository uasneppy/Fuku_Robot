# 261115-pvl Task 3: cross-file pass notes

Fix-up commit: `83327ab` (`docs: fix README series references and one Ukrainian line`), parent `563dba0`, touches only `README.md` (1 line) and `README.uk.md` (2 lines). `git rev-list --count 83dd98e..HEAD` is 3. Nothing pushed, no branch created, nothing under `.planning/` committed.

## Checks run

1. Oracle re-run: throwaway `zz_export_test.go` and `zz_count_test.go` in `fuku/utils/helpers`, output to the scratchpad (`commands.now.txt`), both files deleted in the same command, `git status --porcelain -- fuku` empty. `diff` against `261115-pvl-commands.txt`: identical (COUNT 231). Other counts re-run: 39 `RegisterLegacyModule("` calls, 64 migrations, 8 languages plus `config.yml`, 57 env names, `BotVersion: "2.24.0"`, `go 1.26.0`. All match the Stated figures.
2. Name search: 0 hits of divkix, divideprojects, divanshu, alita, upstream, fork, форк, апстрім, оригінальний проєкт in either file (Python); `fuku` 31 hits per file, all in code, URLs or targets; 0 in prose. Pasted into FACTS H4.
3. Checker: `check_readme.py README.md README.uk.md` exit 0 (`fails=0`, README.uk.md `reviews=4`, README.md 1, all judged); `--parity` exit 0 (40 headings, 68 env names, prose ratio 104%); `make check-docs`: "No drift detected". Every relative link and anchor resolves (checker, no skip), external hosts github.com, gorm.io, img.shields.io, t.me, useblume.dev.
4. Voice: both files read end to end. Decision 1 applied (see below). No joke in near-zero sections, no never-joke subject, no gloating, the in-world "version 72 of 74" once in the intro.
5. Truth spot check, 17 claims re-opened at their file:line: captcha defaults (math, 2 minutes, kick, 3 attempts: `fuku/db/captcha/repository.go:36-44`), warn defaults (3, mute: `warns/repository.go:25`), raid defaults 15, 30 and 5 and minimum 3 (`raiddetection/repository.go:25-33`), windows 60, 30 and 120 s (`raid_counters.go:31`, `raid_flood.go:58`, `raid_ai_count.go:35`), 60 percent and 3 accounts (`raid_flood.go:61-63`, `raid_ai_count.go:38`), lift quiet 10 minutes (`raid_trigger.go:31`), warn gate 24 h (`raid_trigger.go:53`), ban span 330 days (`lockdown_marker.go:35`), staff card 5 minutes (`staff_action_card.go:36`), blocklist cap 200 (`staff/forwardblock.go:23`), AI spam 0.8 and 0.9 (`aispam.go:50-51`), filters cap 150 (`filters.go:57`), backup cooldowns 5 minutes, 10 minutes and 1 hour (`ratelimit/backup_ratelimit.go:39-41`), 366 days (`extraction.go:40`), 47 hours (`deletion_worker.go:46`), federation subscription cap 5 (`federations/repository.go:23`), antiflood 3 to 100 (`antiflood.go:484`), CI coverage floor 78 (`ci.yml:376`), "Started Bot!" (`main.go:539`). No drift. The Turnstile paragraph (all four settings or none, math and text need none, no Redis means unavailable, proxy forwards only `/captcha` and `/captcha/*`, never commit the secret) and the forward blocklist paragraph (the stated limit, creator-only writes) are present in both files.

## Fix-up (decision 1 and one wording defect)

- Commands H2 held two references (tempe in Greetings, facility in Rules). Kept Rules, dropped the repeated tempe joke (Features already uses it) from Greetings in both files. New lines: EN "Greetings. I welcome new members and say goodbye to the ones who leave."; UK "Привітання. Вітаю нових учасників і прощаюся з тими, хто йде."
- UK `/reports block` row reworded («Заборонити користувачам скаржитися й показати їх список»); the old text read as "block reports for users".
- Checker INFO after: Features 1 reference (2 words), Commands 1, all other H2 0, both files.

## Decisions and accepted choices

- Code blocks (structure tree, bash, ini, mermaid) keep their English comments and labels in both files; they are byte-identical and parity depends on it.
- License section unchanged; it points to `LICENSE` and names no holder. `LICENSE` untouched.
- The glossary word "supervisor" (Admin line, EN and UK «наглядача») is kept: it is not a series reference a reader must know.
- README.md-only spelled-out numbers on lines 150 and 922 stay (verified counts).

## Surprises

- `help.go`, the developer `/stats` heading, the `/export` file's `BotName` and 132 locale hits still show the old name to users; the en help text also claims "online since 2020 ... thousands of groups". Full list in FACTS section J and in the SUMMARY.
- The shipped captcha help text names `/captchaattempts` (7 of 8 locales; es has neither); the registered command is `/captchamaxattempts`.
