---
phase: 09-settings-menu
plan: 07
subsystem: settings-menu
tags: [telegram, deep-link, settings-menu, private-chat, gotgbot]

requires:
  - phase: 09-settings-menu
    provides: "plan 09-01 tracer: settingsStartPrefix, settingsPrivateURL, settingsVerifyPress private path, settingsPress.cannotOpen, renderSettingsMain, settingsMainEntries"
provides:
  - "settingsStartDeepLink: /start settings_<chat id> opens the Main page in the admin's private chat after a live admin check"
  - "chat_status.IsBotMissingError exported"
  - "A private press on a dead group or removed bot closes the menu with the cannot-open edit"
affects: [09-settings-menu]

actuals:
  tokens: 7000
  tasks: 2
  commits: 4

plan_head_before: e595cb83fa4a76a0e7ca611cac47c16bb11933c3
plan_head_after: bed5a497226eb10d943505f7fcf736a77f888ca5

tech-stack:
  added: []
  patterns:
    - "Deep link authority is a live getChatMember after a strict payload parse; the payload names a group and nothing more"

key-files:
  created:
    - alita/modules/settings_private_test.go
  modified:
    - alita/modules/settings.go
    - alita/modules/settings_menu.go
    - alita/utils/chat_status/owner.go
    - alita/utils/chat_status/owner_test.go
    - AGENTS.md

key-decisions:
  - "Group-menu pipeline left unchanged: only a private press maps a bot-missing answer to the cannot-open edit"
  - "The deep link uses b.SendMessage to the private chat for the Main page and replyHTML for every refusal"

requirements-completed: [MENU-01, MENU-02, MENU-03]

duration: 20min
completed: 2026-10-30
status: complete
---

# Phase 09 Plan 07: Open in private Summary

**`/start settings_<id>` opens the settings Main page in an admin's private chat for exactly that group after a strict payload parse and a live getChatMember, and a private menu whose group or bot is gone closes itself with the cannot-open note.**

## Performance

- **Duration:** about 20 min
- **Tasks:** 2 (TDD, RED then GREEN each)
- **Files:** 6 (1 created, 5 modified)

## Accomplishments

- `settingsStartDeepLink` (settings.go), registered with `RegisterDeepLinkHandler(settingsStartPrefix, ...)`. Order: `parseStaffChatIDArg` (invalid: `settings_start_invalid`, no Telegram call), `lockdownLiveMember` of the user (bot missing: `settings_cannot_open`; other error: `settings_check_failed`; not creator or administrator: `settings_start_not_admin`), one `getChat` under 5 s (bot missing: cannot open; other error: check failed; not group or supergroup: cannot open), then the Main page via the same `settingsMainEntries` and `renderSettingsMain` a press uses, with `c` in every button and no opener field. Every refusal is a plain reply with no keyboard. `help.go` and `deeplink_router.go` are untouched.
- `chat_status.IsBotMissingError` exported (was `isBotMissingError`), with `TestIsBotMissingError`.
- `settingsVerifyPress` private path: a bot-missing error from the `getChatMember` or `getChat` of `c` calls `p.cannotOpen()` (alert plus edit to `settings_cannot_open`, no keyboard); every other error stays the `settings_check_failed` alert with no edit.
- AGENTS.md: a Handlers bullet for the private entry and an addition to the `settings|v1` Callbacks bullet.

## Task Commits

1. Task 1 RED: `15cca98` test(09-07): show Open in private reaches no menu
2. Task 1 GREEN: `847e588` feat(09-07): open the settings menu in private through /start settings_<id>
3. Task 2 RED: `fb20b19` test(09-07): show a dead private menu repeats a vague alert
4. Task 2 GREEN: `bed5a49` feat(09-07): close a private settings menu whose group or bot is gone

## Deviations from Plan

### Adjusted expectations

**1. [Rule 1 - Plan expectation wrong] Payload "settings_ -100" cannot reply settings_start_invalid**
- **Found during:** Task 1 (writing TestSettingsStartDeepLink)
- **Issue:** The plan lists `settings_ -100` among payloads that reply `settings_start_invalid`. `/start settings_ -100` is three words, so the existing `start` handler sees three arguments, logs "Unexpected number of args" and sends nothing; the deep link handler never runs. Making it reply would need an edit to `help.go`, which D-29 forbids.
- **Fix:** The test asserts the real behaviour for that input (nothing sent, no `getChatMember` call) in its own subtest; I added `settings_-100abc` to the invalid table to keep a mixed digit and letter case. AGENTS.md notes the three-word case.
- **Files modified:** alita/modules/settings_private_test.go, AGENTS.md
- **Commit:** 15cca98, bed5a49

**Total deviations:** 1 (a wrong test expectation in the plan, no production impact). **Impact:** none; all listed invalid payloads that can reach the handler are refused before any lookup.

## Verification

- `CGO_ENABLED=0 go build ./...`: ok
- `TestIsBotMissingError` and the full `./alita/utils/chat_status` package: pass
- `TestSettingsStartDeepLink`, `TestSettingsPrivateRoundTrip`, `TestSettingsPrivateMenu`, `TestSettingsTracer`, `TestSettingsOwnership`, `TestSettingsPressPipeline`, `TestHandleGroupDeepLinkUsesLongestPrefix`: each ran with `-v` and passed; `-run 'TestSettings|DeepLink|TestStart|Staff.*Picker'` over `./alita/modules` also passes
- `go vet -tags testtools ./alita/modules/ ./alita/utils/chat_status/`: clean; `gofmt -l` on the touched files: empty
- `make lint` was not run: golangci-lint in this sandbox is built with go1.25 against a go1.26.0 module, so `gofmt` and `go vet` stood in
- The whole `make test` was not run (the orchestrator runs it between waves)
- No PostgreSQL was started; no migration or model was added

## Pending human verification

Live Telegram behaviour is deferred to 09-SPIKE.md: in a real supergroup run `/settings`, tap Open in private, confirm the bot's chat opens with `/start settings_<id>` and the Main page appears in your language; then remove the bot from the group and press a button on that private menu and confirm it turns into the cannot-open note.

## Known Stubs

None.

## Threat Flags

None. The deep link adds no new endpoint; it is a `/start` payload judged by a live `getChatMember` (T-09-24, T-09-25 mitigated and tested).

## Self-Check: PASSED

- alita/modules/settings_private_test.go exists; commits 15cca98, 847e588, fb20b19, bed5a49 exist in the log
- Acceptance greps: `func IsBotMissingError(` and its call in `FetchBotMember`, `func settingsStartDeepLink(` and `RegisterDeepLinkHandler(settingsStartPrefix, settingsStartDeepLink)`, `IsBotMissingError(` in both settings.go and settings_menu.go, `settingsStartDeepLink` in AGENTS.md: all found; `git diff --stat` for help.go and deeplink_router.go: empty
