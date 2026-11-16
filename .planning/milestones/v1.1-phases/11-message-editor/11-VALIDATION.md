---
phase: "11"
slug: "message-editor"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: validated
nyquist_compliant: true
wave_0_complete: true
created: "2026-11-04"
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `11-RESEARCH.md` § Validation Architecture; the per-task map is filled by the planner and executor as tasks get IDs.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes, no mock libraries |
| Config file | none; `Makefile` target `test` |
| Quick run command | `go test -tags testtools -race -count=1 -run '^(TestSettings\|TestLoadSettings)' ./fuku/modules ./fuku/i18n` (AGENTS), plus `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/utils/content ./fuku/utils/formatting` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs` |
| **Estimated runtime** | quick: under 30 s for a focused `-run`; full: several minutes |

---

## Sampling Rate

- **Per task commit:** the focused `-run` of the task's tests (under 30 s each), plus `go test -tags testtools -race -count=1 ./fuku/utils/content ./fuku/utils/formatting` when those packages change.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/greetings ./fuku/db/rules ./fuku/utils/...` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; manual device checklist (GIF rendering, tap-to-copy `<code>` examples, `/cancel` tap, label truncation in hi/ru/uk). Nothing here joins `make test-postgres-integrity` (no migration, no index, no CHECK, no conditional update on PostgreSQL).

---

## Per-Requirement Verification Map

The planner expands each row into task-level rows (task ID, plan, wave) in its plans' `<verify>` blocks.

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| EDIT-01 | Three pages register, Main grid `[Welcome \| Goodbye] [Rules] [Access]`, status lines, state lines per part, Set/Remove drawn per state, Remove only while set, Preview buttons only for set parts, Open-in-private row only in a group menu, at most 8 rows | integration | `go test -tags testtools -race -count=1 -run '^(TestSettingsEditPages\|TestSettingsMainGrid\|TestSettingsEditRegistry)$' ./fuku/modules` | ✅ green |
| EDIT-01 | Pages and Main read without creating rows; a failed read is the cannot-open edit; no-row defaults equal the runtime (Goodbye On) | integration + db | `-run '^(TestSettingsEditReadsCreateNothing\|TestSettingsEditReadFailure\|TestSettingsEditDefaultsMatchRuntime)$' ./fuku/modules`; `-run '^(TestLoadGreetingSettingsCreatesNothing\|TestLoadRulesCreatesNothing)$' ./fuku/db/greetings ./fuku/db/rules` | ✅ green |
| EDIT-01 | Group menu: Set and Remove write nothing, show `settings_edit_group_note` and Open in private; no wait key created | integration | `-run '^TestSettingsEditGroupMenuRefusal$' ./fuku/modules` | ✅ green |
| EDIT-01 | Forged `t`: missing, unknown, `m` on Rules, non-ASCII, extra key refused before any `getChatMember` | unit/integration | `-run '^(TestSettingsEditCallbackFields\|TestSettingsPageActionValidate)$' ./fuku/modules` | ✅ green |
| EDIT-02 | Set writes the wait (10 min TTL, fields c/p/t/m/n, marker 15 min), edits into the prompt; Set on another part replaces and says so; same part refreshes without the note | integration | `-run '^(TestSettingsEditSetWritesWait\|TestSettingsEditReplaceWait)$' ./fuku/modules` | ✅ green |
| EDIT-02 | Set on replica A, reply on replica B (second handler instance, shared miniredis) | integration | `-run '^TestSettingsEditWaitAcrossReplicas$' ./fuku/modules` | ✅ green |
| EDIT-02 | Cancel button, refusal-message Cancel, `/cancel` (private, group ignored, none), stale Cancel leaves the live wait, expiry via `FastForward`, marker reply once, no marker after save/cancel | integration | `-run '^(TestSettingsEditCancelButton\|TestSettingsEditCancelCommand\|TestSettingsEditStaleCancel\|TestSettingsEditWaitExpiry\|TestSettingsEditMarkerReplyOnce)$' ./fuku/modules` | ✅ green |
| EDIT-02 | Door passes through: no wait and no marker, service identity, edited message, bot, command, caption command, no Redis; one Redis read, zero writes, zero Telegram calls | integration | `-run '^TestSettingsEditDoorPassThrough$' ./fuku/modules` | ✅ green |
| EDIT-02 | Door authority: same decision as a press for the same member record in the four owner worlds plus unknown (sixth door in `TestSettingsGateAllDoors`); lost access ends the wait; 429/5xx stays | integration | `-run '^(TestSettingsGateAllDoors\|TestSettingsEditDoorAuthority)$' ./fuku/modules` | ✅ green |
| EDIT-02 | Double delivery and a 3-item album write once; refused album gets one refusal; expired wait plus album gets one reply | integration | `-run '^(TestSettingsEditDoubleDelivery\|TestSettingsEditAlbumWritesOnce\|TestSettingsEditRefusedAlbumOneRefusal)$' ./fuku/modules` | ✅ green |
| EDIT-02 | Refusal never consumes the wait (TTL not extended); failed write releases the claim | integration | `-run '^(TestSettingsEditRefusalKeepsWait\|TestSettingsEditWriteFailureReleases)$' ./fuku/modules` | ✅ green |
| EDIT-03 | Text keeps entities (bold, link, spoiler, emoji, RTL, overlapping) and placeholders; stored HTML equals the command's | integration | `-run '^TestSettingsEditTextParity$' ./fuku/modules` | ✅ green |
| EDIT-03 | Limits: stored form over 4096 (`content_text_too_long`), over 1024 with media (`content_caption_too_long`), filled-in worst case over (`settings_edit_refuse_too_long_filled`), minimal-user empty, text with button lines, media over a long text | integration + unit | `-run '^(TestSettingsEditTextLimits\|TestSettingsEditFilledLength\|TestSettingsEditTextButtonLines\|TestSettingsEditMediaTextLong)$' ./fuku/modules`; `-run '^(TestFillPlaceholders\|TestVisibleLength)' ./fuku/utils/formatting`; `-run '^TestTextLengthRefusal$' ./fuku/utils/content` | ✅ green |
| EDIT-04 | Each accepted media kind stores the commands' `file_id` and type (GIF as DOCUMENT); refused kinds (video note, contact, poll, text); caption ignored with notice; album keeps one and says so; sticker note | integration | `-run '^(TestSettingsEditMediaParity\|TestSettingsEditMediaKinds\|TestSettingsEditCaptionIgnored\|TestSettingsEditAlbumNotice)$' ./fuku/modules` | ✅ green |
| EDIT-05 | Buttons: valid replace the set, `:same`, default label, empty name; refuse non-http, no host, none readable, over 100 / 8 per row, count includes `{rules}`; stored equals the command's | integration | `-run '^(TestSettingsEditButtonsParity\|TestSettingsEditButtonRefusals)$' ./fuku/modules`; `-run '^TestValidButtonURL$' ./fuku/utils/content` | ✅ green |
| EDIT-06 | Preview text/media/buttons and Full preview go only to the presser's private chat with header, filled with the presser's name and the group title; write nothing (row, `LastMsgId` unchanged, no delete); group menu toast; need-start (403) and failed alerts; Rules full preview equals `/rules` composition | integration | `-run '^(TestSettingsEditPreview\|TestSettingsEditFullPreview\|TestSettingsEditPreviewWritesNothing\|TestSettingsEditPreviewErrors\|TestSettingsEditRulesPreview)$' ./fuku/modules` | ✅ green |
| EDIT-10 | Switches store the same row and the same cache key as the commands; absolute values; failure notice keeps old state; rights per control | integration | `-run '^(TestSettingsParityGreetingSwitches\|TestSettingsDriftGreetings\|TestSettingsAuthorityParityGreetings)$' ./fuku/modules` | ✅ green |
| EDIT-11 | Per part the stored row equals what one `/setwelcome`, `/setgoodbye`, `/setrules` stores (text/media/buttons/Rules) and Remove equals the reset/clear commands; commands still pass their existing tests | integration | `-run '^(TestSettingsEditTextParity\|TestSettingsEditMediaParity\|TestSettingsEditButtonsParity\|TestSettingsEditRulesParity\|TestSettingsEditRemoveParity)$' ./fuku/modules` + existing `TestSetAndResetGreetingTextCommands`, `TestSetRulesStoresCommandTextAndReplies` | ✅ green |
| EDIT-11 | Rights: Welcome/Goodbye Set/Remove/Delete-last need change-info (alert), Rules need bot admin (notice), switch/Preview/Cancel admin only | integration | `-run '^TestSettingsAuthorityParityEdit$' ./fuku/modules` | ✅ green |
| UI / locale | 83 keys (the UI-SPEC's 79 plus the four of Q9) in 8 locales, ceilings, alerts <= 160 and no markup, tokens `{first}` ... `:same` `buttonurl://` byte-identical in the prompts, help text fits one message | unit | `-run '^(TestSettingsLocaleKeys\|TestSettingsLabelLengths\|TestSettingsEditLocaleTokens\|TestSettingsHelpFitsOneMessage)$' ./fuku/i18n` | ✅ green |
| UI / size | Every page, prompt, refusal and saved message <= 3800 UTF-16 units in 8 locales with a 64-rune multibyte title; every callback of every screen <= 64 bytes with `Omitted` 0 | integration | `-run '^(TestSettingsEditPagesFitEveryLocale\|TestSettingsEditCallbackBytes)$' ./fuku/modules` | ✅ green |
| Privacy | No new screen names the Staff Group, in a group menu and in a non-creator private menu, 8 languages | integration | `-run '^TestSettingsStaffPrivacy$' ./fuku/modules` | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `fuku/modules/settings_edit_env_test.go`: `newSettingsEditEnv` (wraps `newSettingsEnv`, seeds a greetings/rules cleanup), `sendPrivateMessage(from, *gotgbot.Message)`, `sendPrivateAlbum(from, items)` (same `MediaGroupId`, concurrent), `shiftEntities(text, prefix)` (UTF-16 shift for command parity), `settingsEditAdminJSON(userID, canChangeInfo)`, fake responses for `sendSticker/sendVideo/sendAudio/sendVoice/sendAnimation`, wait readers over `env.redis`, `editCleanup(chatIDs...)` deleting `GreetingSettings` and `RulesSettings` rows
- [x] `fuku/modules/settings_input_test.go`, `settings_edit_test.go`, `settings_edit_check_test.go`, `settings_edit_preview_test.go`
- [x] `fuku/db/greetings` and `fuku/db/rules`: tests for the new loaders and column writers (SQLite, `testdb.Run`)
- [x] `fuku/utils/content`, `fuku/utils/formatting`: tests for the new exported helpers (and keep the existing ones green)
- [x] `settings_gate_test.go`: one more door and driver in `TestSettingsGateAllDoors`
- [x] `fuku/i18n/settings_locale_test.go`: limits rows for every new `settings_` key (83) and the `settings_help_msg` ceiling check, new alert keys in `settingsAlertKeys`, label rows for the three grid names, new `TestSettingsEditLocaleTokens`
- [x] Update `settings_main_test.go` expectations (grid, status lines)
- [x] `AGENTS.md`: Handlers (group 1, the door, `/cancel`), Callbacks (the three pages' codes, `settingsRightChangeInfo`), Permissions, Data (three key families), the locale count; `make generate-docs`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| GIF as the welcome media renders as a GIF (stored as DOCUMENT, sent through the group's send path) | EDIT-04, EDIT-06 | Needs a real Telegram client and a real animation (`11-SPIKE.md` step 1, A3) | Set a GIF as the welcome media, run Full preview, then a real join; record how it shows (research assumption A1) |
| Tap-to-copy `<code>` examples in the waiting prompts, the `/cancel` tap, button labels in hi / ru / uk at phone width | EDIT-02 | Client rendering (`11-SPIKE.md` steps 7 and 10) | Run the live-check list the owner keeps in the phase SPIKE file, as for Phases 8 to 10 |
| Telegram-side behaviour the research could not fetch (core.telegram.org was blocked): album item order, a 403 when the owner never started the bot, caption entities on media | EDIT-04, EDIT-06 | Needs the owner's bot and a real group (`11-SPIKE.md` steps 2, 3, 5 and 6, A1 to A9) | Record the answers, then confirm or drop research assumptions A1 to A8 |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 60 s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-11-04. Audit: every test name in the map exists, selects tests and ran green with no skip on SQLite/miniredis (52 top-level `fuku/modules` tests, the four `fuku/i18n` tests, `fuku/db/greetings`, `fuku/db/rules`, `fuku/utils/content`, `fuku/utils/formatting`). No gaps found, none escalated. CI-only: `make test`, `make lint`. Manual-only table: GIF rendering, tap-to-copy and label truncation, two live replicas, Telegram-side assumptions A1 to A9, native-speaker read (all in `11-SPIKE.md`).

---

## Validation Audit 2026-11-04

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |
