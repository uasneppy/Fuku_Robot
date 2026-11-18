---
type: quick
id: 261118-p4r
status: complete
---
# Summary: formatting cleanup, the rest

- Owner approval of the sample on 2026-11-18; decisions recorded in the style guide (topic sections kept on long help
  pages, persona phrase joined with a colon, /warn count on its own line).
- Converted: every Ukrainian and English help page (31 keys each), the replies still outside the guide, and the
  documentation keys (trailing whitespace only). TestLocaleStyle's allow-list went from 234 entries to 0.
- Keys changed since the sample (e43fa45): uk 121, en 121, es/fr/hi/id/pt/ru 18 each (mechanical markup only).
- Parse mode: every Markdown send site (14 commands, 15 call sites) moved to HTML, with spliced values escaped.
- Guards: TestLocalePlaceholdersMatchOtherLanguages, TestHelpPagesKeepTheirCommands, TestMarkdownSitesEscapeSplicedValues,
  TestSetWarnLimitEscapesItsArgument.
- Goldens: 39 literals in 8 TestLegacy*Requests files re-recorded once (D6), in commit 1e56324.
  `python3 golden_text_only.py 1e56324~1` prints "246 goldens compared, 39 re-recorded, 0 differences beyond text".
- Checks: golangci-lint v2.15.0 (built with Go 1.26) 0 issues; make check-translations and make check-docs pass;
  `go test -tags testtools -race ./...`: every package passes except the root package, whose
  TestCloseDBConnectionsNilDBReturnsNil races with the deletion worker started by TestPostInitSetsCommandsAndStartupMessage;
  the same race fails at e43fa45, before this task, so it is not caused here.
