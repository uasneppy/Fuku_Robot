---
phase: 11-message-editor
fixed_at: 2026-11-04
review_path: .planning/phases/11-message-editor/11-REVIEW.md
iteration: 1
findings_in_scope: 12
fixed: 10
skipped: 2
status: partial
---

# Phase 11: Code Review Fix Report

Scope was WR-02, WR-04, WR-05, IN-02, IN-03, IN-01, IN-04 and the security audit items NEW-01, NEW-02 and NEW-03, plus the documentation of WR-01 and WR-03. WR-01 and WR-03 (code) and NEW-04 were deferred on purpose: WR-01 and WR-03 are only documented as accepted limits, so they count as skipped here.

| Finding | Action | Commit |
|---------|--------|--------|
| WR-02 | Code fix, tests first. `settingsInputReply.botAdmin` keeps the invariant that the Rules door proceeds only when `chat_status.IsBotAdmin` is true. When it is false, one live `getChatMember` of the bot classifies the false: a definite non-administrator status or a Telegram 4xx other than 429 ends the wait with `chat_status_bot_not_admin`; a 429, a 5xx, a transport error, a timeout, or a live answer of administrator (stale cache) keeps the wait and the marker with `settings_check_failed`. | 81a9d50 |
| WR-04 | Code fix, tests first. The text part's dry run now runs `settingsEditButtonRows(in.State.Buttons, stored)` and refuses with `settings_edit_refuse_buttons_limit` (no parameters, as in the buttons part) when the total passes 100 or the widest row passes 8. | a22acf1 |
| WR-05 | Code fix, test first. A Cancel whose script returns an error other than `errSettingsInputUnavailable` answers the alert `settings_edit_unavailable` and edits nothing, so the prompt and its Cancel button stay. No Redis and every outcome the script answered keep the redraw. | 872a2c5 |
| IN-02 | Code fix, tests first. The Cancel script also compares the ID of the pressed message (`query.Message`, no new callback field) with the wait's `m`; a mismatch answers the existing stale outcome and leaves the wait. The door's `end()` passes the wait's own `PromptMsgID`. Deviation, see below: a refusal reply also carries a Cancel button on a message that is not the prompt, so the door notes each refusal's message ID on the wait (new `settingsInputNoteRefusalScript`, field `r<id>`, at most 40 fields, wiped by every Set) and the script accepts a noted ID. AGENTS.md (Callbacks and Data) corrected, "six Lua scripts" is now seven. | 8620da7 |
| IN-03 | Code fix, test updated first. No existing key said the bot stopped waiting without a placeholder, so one new key `settings_edit_wait_stopped` was added to all 8 locales (placeholder-free, no markup) with its limits row. `end()` now sends the reason, a blank line and that line, except for `settings_edit_refuse_access_lost`, whose text already says it. `make check-translations`, `make generate-docs` and `make check-docs` clean. The same commit records the WR-02 classification and the WR-04 keyboard limit in AGENTS.md. | 01f91a3 |
| NEW-01 | Code fix, test first. `settingsCancelPassOn` is a raw handler registered at group 0 just before the pipeline's `/cancel`: for a `/cancel` of a non-private chat (matched by `handlers.NewCommand`'s own `CheckUpdate`) it replies, writes and calls nothing and returns `nil`. Deviation, see below: `nil`, not `ext.ContinueGroups`. `/cancel` stays on `helpers.WrapCommand`, private behavior unchanged. AGENTS.md Handlers text updated; docs `commands.md` still true ("Nothing happens in a group") and untouched. | 4890409 |
| NEW-03 | Code fix. `fuku/db/conn.go` sets `ParameterizedQueries: true` on the GORM logger, so failed and slow statements log placeholders, not values. One sentence added to AGENTS.md Go rules. No new test (as agreed). | 32f8a31 |
| IN-01, NEW-02 | Text only. AGENTS.md and the comment at the top of `settings_edit_preview.go` now say that a Full or text preview of a greeting with `{rules}` reads the rules through the join path's reader, which creates an empty rules row (and a chats row) for a group that has none, as a real join does, and nothing else is written; and that the Rules Full preview's heading is in the presser's language. No code changed. | b66d165 |
| IN-04, WR-01, WR-03 | Text only (accepted limitations in the AGENTS.md editor text): a typed reply on a replica still on the previous code passes through and the wait stays until it expires; a client-split over-limit text is handled message by message, the first part to claim wins and the rest are dropped without a reply; a claim never finished or released stays until the wait's 10 minutes end. WR-01 and WR-03 code deferred. | b66d165, 6f9a6dc |

## Deviations from the brief

- **NEW-01 returns `nil`, not `ext.ContinueGroups`.** In this gotgbot, `ext.ContinueGroups` means "keep trying the next handler of the current group", so a pass-on handler in front of `/cancel` that returned it would fall straight through to the pipeline's `/cancel` handler in the same group 0, which ends the update. `nil` leaves the group and moves on to group 1 and later, which is the wanted behavior. The first attempt with `ContinueGroups` failed the new test, which is how this showed up. AGENTS.md says so.
- **IN-02 needed the refusal note.** The brief said to compare the pressed message's ID with the stored `m` and add no callback field. A refusal reply carries its own Cancel button (`renderSettingsInputRefusal`) on a message that is not the prompt, so that comparison alone would have made every refusal's Cancel answer "stale". The door now notes each refusal reply's message ID on the wait. `TestSettingsEditRefusalCancel` covers it.
- **One existing test expectation changed.** `TestSettingsEditCancelCommandIsSilentInAGroup` asserted that `/cancel` in a group makes no Telegram call at all. That was true only because the update was ended. It now goes on to the later groups, which make the calls they make for any text, so the test compares `/cancel` with an unknown slash word (same calls, no reply, no write, the wait untouched).

## Tests

- `TestSettingsEditDoorAuthority` (`settings_input_door_test.go`): four new worlds on the Rules page (bot lookup 429, 500, 403, and a failed lookup with a live administrator answer) plus a kept-marker check; the existing worlds now also check that an ending refusal is the reason, a blank line and `settings_edit_wait_stopped` (IN-03), and that a kept one never says the wait stopped. The three 429, 500 and live-administrator worlds failed before the WR-02 fix.
- `TestSettingsEditTextRulesButtonLimit` (`settings_edit_check_test.go`): nine cases both ways (8 in the last row with `{rules:same}`, 100 buttons with `{rules}` and `{rules:up}`, boundaries, no token, no buttons). Three failed before the fix.
- `TestSettingsEditCancelRedisError` (`settings_edit_actions_test.go`): a Redis error injected with miniredis `SetError` on Cancel leaves the prompt unedited and the wait and marker in place; a retry after Redis is back cancels. It failed before the fix.
- `TestSettingsEditStalePromptSamePart` and `TestSettingsEditRefusalCancel` (`settings_edit_actions_test.go`), and three new subtests in `TestSettingsInputCancelOutcomes` (`settings_input_test.go`: another message is stale, a noted refusal deletes, a new Set forgets old refusals). The same-part test was checked to fail with the message comparison removed.
- `TestSettingsCancelInGroupPassesOn` (`settings_input_test.go`): `/cancel https://x.example` in a supergroup reaches a group 7 sentinel handler and is counted by `raidFloodLateWatcher` (3 items, 3 accounts), with no message sent and no `fuku:settings:*` key written; it failed with the new handler removed. `floodMessage` gained the `cancel` kind. Private `/cancel` is covered by the unchanged `TestSettingsEditCancelCommand*` and `TestSettingsInputWithoutRedis`.

## Verification

Run in the main checkout (no worktree), on branch `claude/ecstatic-goldberg-qsss05`, after the last code commit:

- `gofmt -l` is clean on every file touched (`settings_input.go`, `settings_input_door.go`, `settings_edit_check.go`, `settings_edit_actions.go`, `settings_edit_preview.go`, their tests, `raid_flood_test.go`, `fuku/db/conn.go`, `fuku/i18n/settings_locale_test.go`).
- `CGO_ENABLED=0 go build ./...` passes.
- `go vet -tags testtools ./fuku/modules ./fuku/db` passes.
- `python3 .../lint-local.py 72a692b` exits 0 (no dupl, gocyclo or godox findings on added lines).
- `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` passes (modules 233.6 s, i18n 6.0 s).
- `go test -tags testtools -race -count=1 -run '^(TestGreeting|TestWelcome|TestSetRules|TestSendRules|TestRaidFlood|TestLoadLockdown)' ./fuku/modules` passes (11.7 s).
- `go test -tags testtools -count=1 ./fuku/db` passes.
- `make check-translations` passes (all 8 locales), `make generate-docs` then `make check-docs` report no drift (no generated doc changed).
- `git diff --exit-code 72a692b -- go.mod go.sum` is clean.
- `make lint` and `make test` were not run (the installed golangci-lint refuses Go 1.26.0 here; lint-local is the stand-in).

---

_Fixed: 2026-11-04_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
