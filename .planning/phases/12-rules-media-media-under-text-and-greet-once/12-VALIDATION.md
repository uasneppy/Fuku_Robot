---
phase: "12"
slug: "rules-media-media-under-text-and-greet-once"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-11-05"
---

# Phase 12 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution. Seeded from `12-RESEARCH.md` § Validation Architecture; the per-task map is filled by the planner and executor as tasks get IDs.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, build tag `testtools`, miniredis, hand-written `gotgbot.BotClient` fakes, SQLite via `internal/testdb.Run`, no mock libraries |
| Config file | none; `Makefile` target `test` (15 min per package) |
| Quick run command | the focused `-run` names of the map below (seconds each); the AGENTS family `go test -tags testtools -race -count=1 -run '^(TestSettings|TestLoadSettings)' ./fuku/modules ./fuku/i18n` takes about 3.6 minutes, so it is a per-wave command; plus `go test -tags testtools -race -count=1 ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/utils/content ./fuku/utils/media ./fuku/utils/formatting ./fuku/db/migrations` |
| Full suite command | `make test` (never a bare `go test ./...`), then `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`; CI: `make test-postgres-integrity` |

---

## Sampling Rate

- **Per task commit:** the focused `-run` of the task's tests (under 30 s each), plus the package-level run of any `fuku/db/*` or `fuku/utils/*` package the task touched.
- **Per wave merge:** `go test -tags testtools -race -count=1 ./fuku/modules ./fuku/i18n ./fuku/db/greetings ./fuku/db/rules ./fuku/db/lang ./fuku/db/backup ./fuku/db/migrations ./fuku/utils/...` plus `make lint`.
- **Phase gate:** `make test`, `make lint`, `make check-translations`, `make generate-docs` then `make check-docs`, CI `make test-postgres-integrity`, and the owner's live checklist `12-SPIKE.md` (deferred like Phases 7 to 11 unless the owner says otherwise). Baseline: before this phase the settings family is green (modules 188.6 s, i18n 5.2 s, see Metadata).


---

## Per-Requirement Verification Map

The planner assigns task IDs, plans and waves when the plans are written; the executors set the status. Test function names are the plan's contract.

| Requirement | Behavior | Test Type | Automated Command | File Exists | Status |
|-------------|----------|-----------|-------------------|-------------|--------|
| EDIT-07 | `sendPhoto`, `sendVideo`, `sendAnimation` carry `show_caption_above_media` true only when the flag is set; documents, audio, voice, sticker, video note never; flag off adds no parameter | unit | `-run '^(TestSendGreetingMessageCaptionAbove | TestCaptionAboveEligible | ⬜ pending |
| EDIT-07 | A GIF through the editor and through `/setwelcome` by reply is `db.ANIMATION`; notes/filters/pins still `DOCUMENT`; a legacy document GIF stays document | unit + integration | `-run '^(TestExtractMedia | TestExtractGreetingMediaAnimation | ⬜ pending |
| EDIT-07 | Switch drawn exactly for photo, video, animation; not for document, audio, voice, sticker, video note, no media, legacy GIF; stored "on" ignored and not sent; returns on an eligible replacement; stale press writes nothing and shows `settings_edit_below_unavailable` | integration | `-run '^(TestSettingsEditMediaBelowEligibility | TestSettingsEditMediaBelowStalePress | ⬜ pending |
| EDIT-07 | The join, leave, `/welcome`, `/goodbye`, `/rules`, private rules and Full preview all send with the flag from the stored column | integration | `-run '^(TestGreetingsMediaBelowEverywhere | TestRulesMediaBelowEverywhere)$' ./fuku/modules` | ⬜ pending |
| EDIT-07 | Three switch writes are column-targeted, write a Go `false`, keep other columns, call `cache.DeleteCache`; drift and authority (Welcome/Goodbye `can_change_info` alert, Rules bot-admin notice) | integration + db | `-run '^(TestSettingsParityMediaBelow | TestSettingsDriftMediaBelow | ⬜ pending |
| EDIT-08 | Legacy rows: `/rules` and the deep link send the same request as today (recorded before the refactor) | integration (characterization) | `-run '^TestRulesLegacyRowSendsTodaysRequest$' ./fuku/modules` | ❌ Wave 0 (write first, green on old code) | ⬜ pending |
| EDIT-08 | `/rules` in the group, private rules (deep link) and a connected private chat show media with caption and buttons; media-only and buttons-only rules; click-to-read group message carries none | integration | `-run '^(TestRulesMediaAndButtonsInGroup | TestPrivateRulesMediaAndButtons | ⬜ pending |
| EDIT-08 | Rules page: three parts, Set/Remove/Cancel/Preview for `m` and `b`, `t=z` invalid, right = bot admin (notice), group menu refusal, Remove stores `/clearrules`' row, `/clearrules` clears all three | integration | `-run '^(TestSettingsEditRulesParts | TestSettingsEditRulesRemoveParity | ⬜ pending |
| EDIT-08 | Typed Rules media and buttons store what the Welcome path stores for the same input; caption rule parity table (text at 1000/1024/1025 less heading, every stored kind) between Welcome and Rules; prompt 1024 with media else 3800; video note refused as typed media | integration + unit | `-run '^(TestSettingsEditRulesTypedParity | TestSettingsRulesCaptionParity | ⬜ pending |
| EDIT-08 | Full preview equals `/rules` for the same row (heading in the group's language, header in the presser's), group language change seen next preview, unreadable language falls back to `uk`, language read creates no row | integration | `-run '^(TestSettingsEditRulesPreview | TestSettingsEditRulesPreviewGroupLanguage)$' ./fuku/modules`; `-run '^TestGetGroupLanguage$' ./fuku/db/lang` | ⬜ pending |
| EDIT-08 | Runtime degrade: a definite 4xx on a media rules send retries once as text-only in `/rules` and the deep link, not in Full preview | integration | `-run '^TestRulesDegradesToTextOnClientError$' ./fuku/modules` | ❌ Wave 0 | ⬜ pending |
| EDIT-08 | Rules loader creates no row; writers column-targeted; `ClearRules` stores zeros; `HasContent`; `{rules}` button for media-only rules | db + unit | `-run '^(TestLoadRulesSettingsCreatesNothing | TestRulesMediaWriters | ⬜ pending |
| EDIT-09 | First join welcomed, second not; welcome off -> no row; `Once` off -> no row and today's behaviour; captcha-failed and lockdown-banned joiners leave no row; the captcha-pass path claims; works without Redis | integration | `-run '^(TestGreetOnceFirstJoinOnly | TestGreetOnceWelcomeOffNoClaim | ⬜ pending |
| EDIT-09 | Concurrent `SendWelcomeMessage` for one `(chat, user)` -> exactly one welcome; definite refusal (403, 400, `ErrNoPermission`, 429) gives the row back; unknown outcome (timeout, 5xx) keeps it; claim error sends nothing | integration | `-run '^(TestGreetOnceConcurrentOneWelcome | TestGreetOnceDefiniteRefusalReleases | ⬜ pending |
| EDIT-09 | `ClaimGreeting`/`ReleaseGreeting` repository behaviour; PostgreSQL concurrency | db (+ PostgreSQL only) | `-run '^TestGreetedMemberClaim$' ./fuku/db/greetings`; CI `TestGreetedMemberClaimConcurrent` | ❌ Wave 0 | ⬜ pending |
| EDIT-09 | Switch `o`: Welcome only, `settingsRightAdmin`, absolute value, drift and authority, the four owner worlds through the press and the typed-reply door; preview and Full preview never claim; the page never reads `greeted_members` | integration | `-run '^(TestSettingsEditOnceSwitch | TestSettingsDriftOnce | ⬜ pending |
| EDIT-07/08/09 | Migration: statement shape (db-free) and defaults on PostgreSQL; backup round trip carries the new columns, an old export decodes to zeros, `greeted_members` untouched by export/import/reset | unit + PostgreSQL + db | `-run '^TestRulesMediaMigrationStatements$' ./fuku/db/migrations`; CI `TestRulesMediaMigrationDefaults`; `-run '^(TestAllModulesRoundTripEveryMeaningfulField | TestOldGreetingsExportDecodesToZeros | ⬜ pending |
| UI | Row budget (8 private / 9 group) at worst case for Welcome, Goodbye, Rules and the grid fill; callback bytes of every new button at worst-case IDs (largest 45) with `Omitted` 0; `tg` unknown `t`, `t=o` on g/r, `t=e | d` on r, `v=2`, missing `t`, extra key refused before any `getChatMember` | integration | `-run '^(TestSettingsEditSwitchGrid | ⬜ pending |
| UI / locale | 13 new + 4 renamed rows, exactly 199 `settings_` keys, five switch labels <= 20 rendered with `✅` and `❌` in 8 locales, tokens, hints, Rules hint keeps `<code>/rules</code>`; pages fit 3800 units in 8 locales with a 64-rune title; privacy (no Staff Group on any new screen) | unit + integration | `-run '^(TestSettingsLocaleKeys | TestSettingsLabelLengths | ⬜ pending |
| Success 4 | Rules, welcomes and goodbyes saved before the phase look exactly as before: legacy `/rules` request, flag-off requests carry no `show_caption_above_media`, an old export imports, an old GIF document sends with `sendDocument` | integration | the characterization test above plus `-run '^TestLegacyGreetingsRequestsUnchanged$' ./fuku/modules` | ❌ Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] **Characterization first:** `fuku/modules/rules_legacy_request_test.go` records the fake's `sendMessage` params for `/rules` (private rules off and on) and the deep link for an old row, and a legacy greeting/goodbye request with the flag off; commit it green against untouched code before any refactor.
- [ ] `fuku/utils/media/sender_test.go`: caption-above and animation tests; `fuku/utils/content`: greeting extraction tests.
- [ ] `fuku/db/greetings/greeted_test.go` and `testmain_test.go` (add `&models.GreetedMember{}`); writers' tests; `fuku/db/rules` loader/writer tests; `fuku/db/lang` `TestGetGroupLanguage`.
- [ ] `fuku/db/backup`: `testmain_test.go` gains `GreetedMember`; round-trip fields; `TestOldGreetingsExportDecodesToZeros`, `TestGreetedMembersStayOutOfBackup`.
- [ ] `fuku/db/migrations`: `rules_media_migration_test.go` (statements, and the PostgreSQL defaults test).
- [ ] `fuku/modules/test_harness_test.go`: `&models.GreetedMember{}`; `editCleanup` deletes `greeted_members`; a join-driving helper reusing the `TestMemberJoinAndLeaveSendConfiguredGreetings` pattern; fake answers for `sendAnimation` already exist (`settingsEditFakeSends`, `settings_edit_preview_test.go:27-37`).
- [ ] `fuku/i18n/settings_locale_test.go`: 13 rows, 4 renames, five-cell rendered check, new token rows.
- [ ] Makefile: add the new PostgreSQL test names and `./fuku/db/greetings` to `test-postgres-integrity`.
- [ ] `AGENTS.md` (S-07), `make generate-docs`.
- [ ] Owner's `12-SPIKE.md`: the live items of the S-01 table.

---

## Manual-Only Verifications

Covered by the owner's live checklist `12-SPIKE.md` (written by the phase gate plan; deferred like Phases 7 to 11 unless the owner says otherwise). None blocks the phase.

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The caption renders above the media (media below the text) for photo, video and GIF on Android, iOS, Desktop and Web; the sent message echoes `show_caption_above_media: true`; an old client ignores the flag harmlessly | EDIT-07 | Real-client rendering cannot be asserted by a fake bot client | Set a photo, a video and a GIF on a test group's Welcome, turn Media under text on, press Full preview and make a test account join |
| `sendAnimation` with a stored animation file ID loops as a GIF; the flag is accepted with an empty caption; a 1025-unit caption gets a 400 | EDIT-07 | Telegram's live API behaviour (core.telegram.org was unreachable during research) | Same group; Full preview with a GIF; a rules text of 1025 characters with media set (the editor refuses it before the send) |
| Greet once across two live replicas: a member who leaves and rejoins gets no second welcome | EDIT-09 | Two real replicas over one database | Run two bot instances against one PostgreSQL, turn Greet once on, join, leave and rejoin with a test account |
| `/rules` with media and buttons in the group and as private rules on real clients; a media-only rules row | EDIT-08 | Real-client rendering and the private-chat path | Set rules media and buttons in the editor, send `/rules` in the group, then with private rules on, open the link |
| Native-speaker read of the new locale strings in all 8 languages | EDIT-07, 08, 09 | Wording quality is human | Read each new `settings_edit_*` and `settings_btn_edit_*` key in en, es, fr, hi, id, pt, ru, uk |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency: a focused `-run` under 30 s; the settings family (about 3.6 minutes) only per wave
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
