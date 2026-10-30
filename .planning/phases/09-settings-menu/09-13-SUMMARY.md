---
phase: 09-settings-menu
plan: 13
subsystem: help-and-docs
tags: [help, i18n, docs, agents-md, settings-menu]
requires:
  - phase: 09-settings-menu
    provides: the finished settings menu (plans 09-01 to 09-12), settingsModule and LoadSettings
provides:
  - "/help lists a Settings module whose text is settings_help_msg in all 7 languages; /help settings resolves it"
  - "the generated Settings command page, the /settings reference row and the Settings menu permissions section"
  - "AGENTS.md notes for the settings test toolkit and the antiflood first-write trap"
affects: [09-14, phase verification]
status: complete
plan_head_before: 8fa613e06824bded1259955b60ba2d10389c9108
plan_head_after: e1e708861f17577b44a07665b91884677442986d
actuals:
  tokens: 9500
  tasks: 3
  commits: 4
tech-stack:
  added: []
  patterns:
    - "A module is listed in /help by SetModuleEnabled at the top of its Load function, a <module>_help_msg key and an alt_names row"
key-files:
  created:
    - alita/modules/settings_help_test.go
    - docs/src/content/docs/commands/settings/index.md
  modified:
    - alita/modules/settings.go
    - alita/i18n/settings_locale_test.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/config.yml
    - docs/src/content/docs/api-reference/commands.md
    - docs/src/content/docs/api-reference/permissions.md
    - AGENTS.md
key-decisions:
  - "The help text uses plain /command text and only the b tag, as the UI-SPEC shows, not the code tags lockdown_help_msg uses"
  - "The antiflood first write is documented, not fixed (D-27): the pinned value is flood_limit 5, and a Mute press on a fresh chat writes no row"
requirements-completed: [MENU-01, MENU-02, MENU-03, MENU-04, MENU-05, MENU-06, MENU-07, MENU-08]
coverage:
  - id: D1
    description: "settings_help_msg exists in all 7 locales, fits one Telegram message with the module header, uses only b tags and names /settings and the eleven mirrored commands"
    requirement: "MENU-01"
    verification:
      - kind: unit
        ref: "alita/i18n/settings_locale_test.go#TestSettingsHelpFitsOneMessage"
        status: pass
  - id: D2
    description: "LoadSettings enables the Settings module in the help registry, the /help menu has a Settings button, /help settings resolves through alt_names.Settings, and the English help text is served"
    requirement: "MENU-01"
    verification:
      - kind: unit
        ref: "alita/modules/settings_help_test.go#TestLoadSettingsRegistersHelp"
        status: pass
  - id: D3
    description: "The generated Settings command page exists with no docs drift; the manual command and permissions references list /settings and the right per control"
    requirement: "MENU-02"
    verification:
      - kind: other
        ref: "make check-docs"
        status: pass
  - id: D4
    description: "AGENTS.md names the settings test toolkit and the parity, drift, authority-parity, callback-bytes and page-length tests, and records the antiflood first-write trap"
    requirement: "MENU-08"
    verification:
      - kind: other
        ref: "grep TestSettingsAuthorityParity|TestSettingsStaffPrivacy|first write AGENTS.md"
        status: pass
---

# Phase 9 Plan 13: Help entry, docs and AGENTS.md Summary

**The settings menu is now listed in /help in seven languages with an alt name, has a generated command page and permission references, and AGENTS.md records how it is tested and the one antiflood side effect it keeps on purpose.**

## Accomplishments

- Task 1 (RED then GREEN): `settings_help_msg` added to all 7 locale files (the UI-SPEC English text, six translations using each file's existing Staff and Lockdown terms; 638 to 689 characters, ceilings 900 and 1260), `Settings: [settings]` added to `alt_names` in `locales/config.yml`, and `LoadSettings` now starts with `SetModuleEnabled(settingsModule.moduleName, true)`. `TestSettingsHelpFitsOneMessage` (new) holds the length ceilings, the `b`-only tag rule, the twelve command names and the 4096 limit with the module header; `TestSettingsLocaleKeys` covers the key through a new row in the limits table (now 80 rows); `TestLoadSettingsRegistersHelp` (new, `alita/modules/settings_help_test.go`) clears the module flag first, so it fails without the `SetModuleEnabled` call.
- Task 2: `make generate-docs` produced only one new file, `docs/src/content/docs/commands/settings/index.md`; no other generated file changed. By hand: a Settings table with the `/settings` row and an alphabetical index row in `api-reference/commands.md`, and a "Settings menu" section with a control-and-right table in `api-reference/permissions.md` (frozen marker untouched). `make check-docs` reports no drift.
- Task 3: AGENTS.md Testing now gives the focused run command, the page-helper convention, the parity, drift, authority-parity, callback-bytes and page-fits-locale test families, `TestSettingsStaffPrivacy`, the help tests, and states that nothing of the menu joins `make test-postgres-integrity`. Subsystem traps gained the antiflood first-write bullet (limit 5 pinned for a first ban mode write, the Mute no-op, the menu keeping it on purpose, the repository fix as a separate follow-up).

## Verification

- `go test -tags testtools -race -count=1 -v -run '^TestSettings' ./alita/modules ./alita/i18n`: 66 top-level tests, 66 PASS, 0 FAIL, 0 SKIP (the modules package took 66 s, i18n 5 s). Also run and passing: `TestLockdownHelpFitsOneMessage`, `TestLoadSettingsRegistersHelp`.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./alita/modules/ ./alita/i18n/` clean; `gofmt -l alita/` empty; `make check-translations` and `make check-docs` pass.
- `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against the module's go1.26.0. `gofmt` and `go vet` stand in for it.
- The full `make test` was not run (the orchestrator runs it between waves).

## Deviations from Plan

None - plan executed as written. Notes that are not deviations:

- The commit trailers use the harness-given attribution (Sonnet 5.5) instead of the model line the plan text prints, as the dispatch instructions direct.
- `commands.md` carries stale module and command totals in its overview (Lockdown and Staff were never counted); I left them alone rather than guess new totals.
- The first-write trap says "pinned for ban" because `TestSetFloodModeOnFreshChatStoredLimit` pins only the ban action; kick is not claimed.

## Known Stubs

None.

## Threat Flags

None. The help text and docs name rights and commands only (T-09-49); the length test covers T-09-50.

## Self-Check: PASSED

- Files found: alita/modules/settings_help_test.go, docs/src/content/docs/commands/settings/index.md, and the edited files above.
- Commits found: 07c3ce4 (test), 6629853 (feat), d4702ac (docs pages), e1e7088 (docs AGENTS.md).
