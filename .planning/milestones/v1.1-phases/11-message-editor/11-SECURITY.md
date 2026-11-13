---
phase: "11"
slug: "message-editor"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-11-04"
---

# Phase 11 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Audited by `gsd-security-auditor` against the code at `7eb0ecd` (all 14 plans executed); every mitigation was read in
> the code and the security-relevant tests were run (all green, no skips). The review-fix pass that followed
> (`cc96df0..ba7c3ca`) moved some lines and changed behaviour only where the New Threats section says so. Evidence below
> is `file:line` in `fuku/modules/` unless a path is given. Short names: `door` = `settings_input_door.go`,
> `input` = `settings_input.go`, `edit` = `settings_edit.go`, `act` = `settings_edit_actions.go`,
> `chk` = `settings_edit_check.go`, `prev` = `settings_edit_preview.go`, `menu` = `settings_menu.go`,
> `greet` = `greetings.go`, `rulesm` = `rules.go`, `fmt` = `fuku/utils/formatting/formatting.go`,
> `content` = `fuku/utils/content/content.go`, `loc` = `fuku/i18n/settings_locale_test.go`.

---

## Trust Boundaries

| # | Boundary | Description | Data Crossing |
|---|----------|-------------|---------------|
| 1 | Telegram user → bot | the typed reply (text, entities, media, album items, button lines), `/cancel`, and the `settings\|v1` editor callbacks, all attacker-controlled | message content, file IDs, chat IDs, user IDs, page and part codes |
| 2 | Bot → Telegram Bot API | live `getChatMember`, `getChatAdministrators` and `getChat` on every reply and press; previews are sent with `sendMessage` and `SendGreeting`; any of them can fail, time out or be slow | membership records, preview content |
| 3 | Replica → Redis wait store | `fuku:settings:input:<user>`, the marker and the album memory, built from the sender's ID alone | IDs, codes, a nonce, never message text or a file ID |
| 4 | Replica ↔ replica | the wait, the claim and the album gate live in shared Redis, so any replica may take the reply | claim nonce |
| 5 | Replica → PostgreSQL | the one-part greeting writers and the rules writers; reads are uncached and create nothing | stored text, media, buttons |
| 6 | Stored message → every later join | what the editor saves is sent at each join, so a saved message Telegram would refuse breaks the welcome for everyone | HTML text, media file ID, buttons |
| 7 | Preview → the presser's private chat | Preview and Full preview go only to `query.From.Id`, never to the group | the message as members see it |
| 8 | Private menu → group members | a menu opened in a group is readable by every member; no editor screen names the Staff Group to anyone but the creator's private menu | page state words |
| 9 | Locale files → rendered messages | 83 new keys in 8 locales; placeholders, markup and lengths | translated strings |
| 10 | Process logs and SQL log lines | no message text, caption, button name, link or file ID in any log line | log text |
| 11 | Third-party dependencies | `go.mod` / `go.sum` unchanged since baselines `72a692b` and `f662984` | — |

---

## Threat Register

`T-11-SC` (package installs, high, mitigate) is registered in every plan (11-01 to 11-14); it is closed in all 14 because
`go.mod` and `go.sum` are unchanged (`git diff --exit-code 72a692b -- go.mod go.sum` and the same against `f662984` both
exit 0). All other IDs appear in exactly one plan. 59 register entries in all: 58 closed, 1 accepted, 0 open.

| Threat ID | Plan | Category | Disposition | Mitigation (evidence) | Status |
|-----------|------|----------|-------------|-----------------------|--------|
| T-11-01 | 01 | I | mitigate | `loc` TestSettingsReusedKeyPlaceholders forbids `{staff}` and `{id}`; the `{id}` exemption covers only `settings_edit_wait_text`, where it is a literal token pinned by TestSettingsEditLocaleTokens; placeholder comparison at loc:316 | CLOSED |
| T-11-02 | 01 | T | mitigate | loc:327-351: only `b` and `code` allowed, tags balanced, alert keys hold no markup and are at most 200 runes | CLOSED |
| T-11-03 | 01 | D | mitigate | alert rows capped at 160 (loc:194-195 and others); TestSettingsLabelLengths at loc:385-391 holds 20 and 40 code points | CLOSED |
| T-11-04 | 02 | T | mitigate | fmt:226-286 escapes every value (MentionHtml at fmt:165 escapes the name; `@username` is escaped); single pass in replaceFormatPlaceholders, fmt:191; TestFillPlaceholders* | CLOSED |
| T-11-05 | 02 | T | mitigate | content:224 ValidButtonURL (http or https plus a host), used at content:252 and chk:296; TestValidButtonURL | CLOSED |
| T-11-06 | 02 | T | mitigate | preFixes calls the helpers at content:239 and :252; TestTextLengthRefusalMatchesExtractWelcome | CLOSED |
| T-11-07 | 03 | T | mitigate | greetings/repository.go:133 LoadSettingsContext and rules/repository.go:48 LoadRulesContext create nothing; edit:137-140 is the seam; TestLoad*CreatesNothing | CLOSED |
| T-11-08 | 03 | R | mitigate | edit:199-215 returns the read error; edit:425-438 shows it as the cannot-open edit; edit:404-411 shows the unknown mark on Main | CLOSED |
| T-11-09 | 03 | T | mitigate | greetings/repository.go:265-348 Set*Only maps go through upsertGreetingSettings (:218, DeleteCache at :236); TestGreetingColumnWritersKeepOtherParts | CLOSED |
| T-11-10 | 03 | I | accept | accepted risk AR-01 below: nothing reads `greetings:<chat>` through the cache (the auditor checked: only the DeleteCache at repository.go:236 and the backup module touch the key, and the editor reads uncached) | ACCEPTED |
| T-11-11 | 04 | I | mitigate | door:242-251 and :280-290 give one access-lost text for any 4xx and any non-admin status; en.yml:3178 has no placeholder | CLOSED |
| T-11-12 | 04 | T | mitigate | loc:348-351 alert keys hold no markup and are at most 200 runes | CLOSED |
| T-11-13 | 05 | T | mitigate | `/welcome` uses greet:431, clean toggles greet:637, reset greet:305, `/clearrules` rulesm:90, `/setrules` rulesm:251; the menu uses edit:443-457; TestGreetingApplyParity, TestGreetingRemovesEqualReset, TestRulesApplyParity | CLOSED |
| T-11-14 | 05 | R | mitigate | greet:202-217 applyReadBack returns greetingSaveFailed with the error; the command replies common_settings_save_failed (greet:436); TestGreetingApplyWriteFailure | CLOSED |
| T-11-15 | 05 | T | mitigate | greet:316-319 renderGreeting only builds; the clean-old bookkeeping stays in SendWelcomeMessage and leftMember (see NEW-02 for the rules lookup) | CLOSED |
| T-11-16 | 06 | T | mitigate | menu:277 runs Validate in step 1 before any I/O; TestSettingsPageActionValidate | CLOSED |
| T-11-17 | 06 | E | mitigate | menu:98-108: creator, or an administrator with CanChangeInfo; any other status fails closed; TestSettingsRightChangeInfo | CLOSED |
| T-11-18 | 06 | E | mitigate | menu:81-112 keeps the Restrict and Owner cases verbatim; the bot checks stay in settingsRightRefusal (menu:119); TestSettingsRightRefusalForParity | CLOSED |
| T-11-19 | 07 | S | mitigate | keys come from the user ID only (input:167-179): `/cancel` uses EffectiveUser (input:505-510), the door uses msg.From.Id with From == Chat (door:62-63, :89), Set uses query.From.Id (act:141); TestSettingsEditCancelCommandTouchesOnlyTheSender | CLOSED |
| T-11-20 | 07 | T | mitigate | input:142-164: the nonce claim works once; finish and release work only for the claim holder; album SET NX at input:457-468; TestSettingsInputCancelOutcomes, TestSettingsInputAlbumAndMarker | CLOSED |
| T-11-21 | 07 | T | mitigate | input:125-136: the Cancel script compares c, p and t, answers 2 while a claim is held, and 3 without touching the live wait (the review-fix pass added the prompt message ID to the comparison, IN-02) | CLOSED |
| T-11-22 | 07 | I | mitigate | input:95-102 and :286-292: the hash holds only c, p, t, m, n and x (and noted refusal IDs after the review-fix pass) | CLOSED |
| T-11-23 | 07 | D | mitigate | input:398-409: finish and release run on context.WithoutCancel with a 5 s bound | CLOSED |
| T-11-24 | 08 | I | mitigate | edit:255-306 shows state words, lengths and counts only and never reads the Staff Group; TestSettingsStaffPrivacy covers the editor screens in both views and all 8 languages | CLOSED |
| T-11-25 | 08 | E | mitigate | edit:522 control `d` uses settingsRightChangeInfo, like `/cleanwelcome`'s CanUserChangeInfo (greet:717); TestSettingsAuthorityParityGreetings | CLOSED |
| T-11-26 | 08 | T | mitigate | edit:443-457 call the commands' apply functions; TestSettingsParityGreetingSwitches, TestSettingsDriftGreetings | CLOSED |
| T-11-27 | 08 | D | mitigate | TestSettingsEditCallbackBytes (settings_edit_test.go:928) runs at the worst-case IDs; an overflow is counted in Omitted | CLOSED |
| T-11-28 | 08 | R | mitigate | edit:404-411 and :425-438; TestSettingsEditReadFailure | CLOSED |
| T-11-29 | 09 | T | mitigate | menu:407-434: live member check, gate, then getChat, which must be a group or supergroup, and p.group is the canonical chat; act:141-148 stores p.group.Id; act:34-43 validates `t` before any I/O; TestSettingsEditCallbackFields | CLOSED |
| T-11-30 | 09 | E | mitigate | EditRight at edit:116, :124 and :130 (ChangeInfo, ChangeInfo, AdminBotAdmin); the actions at act:266-267 require it; TestSettingsAuthorityParityEdit | CLOSED |
| T-11-31 | 09 | T | mitigate | act:104-113: in a group menu the action only shows the note and writes no key; TestSettingsEditGroupMenuRefusal | CLOSED |
| T-11-32 | 09 | T | mitigate | act:170 uses the c/p/t-scoped script at input:125; TestSettingsEditReplaceWait | CLOSED |
| T-11-33 | 09 | D | mitigate | input:249-255 and act:149-155 answer the `settings_edit_unavailable` alert and write nothing; TestSettingsEditSetUnavailable | CLOSED |
| T-11-34 | 10 | D | mitigate | chk:155-161 (empty or blank text, limit), chk:195-208 (three fills), chk:301-304 (100 buttons, 8 per row); TestSettingsEditFilledLength, TestSettingsEditButtonRefusals | CLOSED |
| T-11-35 | 10 | T | mitigate | chk:295-299: the refusal carries only the button's number | CLOSED |
| T-11-36 | 10 | D | mitigate | chk:95-103 recovers; all three converter calls run inside that closure (chk:134-138, :216-218, :281-284) and the door also recovers at door:84; TestSettingsEditCheckMalformedEntities | CLOSED |
| T-11-37 | 10 | I | mitigate | a grep of every new log line finds only IDs, codes and errors; chk:98 is a fixed string; the GORM logger leak (NEW-03) is fixed | CLOSED |
| T-11-38 | 10 | T | mitigate | chk:135 uses MD2HTMLButtonsV2(OriginalMDV2) as content.ExtractWelcome:162 does; chk:217 uses MD2HTMLV2 as `/setrules` (rulesm:251); media goes through content.ExtractMedia:234; parity tests | CLOSED |
| T-11-39 | 11 | E | mitigate | door:238-273 checks the live getChatMember, settingsGate and the part's right on every reply and ends the wait on a loss; TestSettingsEditDoorAuthority; TestSettingsGateAllDoors has a sixth door, "typed reply" | CLOSED |
| T-11-40 | 11 | S | mitigate | door:61-65 requires From == Chat, a private chat, not a bot and no service ID; the key is msg.From.Id | CLOSED |
| T-11-41 | 11 | T | mitigate | the group comes from the wait (door:239); getChat must show a group or supergroup (door:277-292); the nonce binds the claimed wait's fields | CLOSED |
| T-11-42 | 11 | T | mitigate | door:203-225: refuse and keep happen before the claim, the claim is by nonce, a failed write releases it; TestSettingsEditDoubleDelivery, TestSettingsEditRefusalKeepsWait, TestSettingsEditWriteFailureReleases | CLOSED |
| T-11-43 | 11 | I | mitigate | door:242-251 and :280-290 name no group and give one text for every case | CLOSED |
| T-11-44 | 11 | D | mitigate | door:86-98: cheap filter first, one pipeline (HGETALL plus GET), no Telegram call without a wait; TestSettingsEditDoorPassThrough | CLOSED |
| T-11-45 | 11 | I | mitigate | door logs carry only IDs, the part and the outcome; TestSettingsEditDoorLogsNoText (settings_input_door_test.go:804) uses a logrus hook | CLOSED |
| T-11-46 | 11 | D | mitigate | input:528 registers group 1; door:44-56 excludes commands and slash captions; door:87-97 returns ContinueGroups when not consumed | CLOSED |
| T-11-47 | 12 | I | mitigate | prev:40, :60 and :69 always send to query.From.Id; TestSettingsEditPreviewWritesNothing | CLOSED |
| T-11-48 | 12 | T | mitigate | prev.go calls only renderGreeting and media.SendGreeting: no SendWelcomeMessage, no last_msg_id, no delete (see NEW-02 for the rules lookup) | CLOSED |
| T-11-49 | 12 | I | mitigate | prev:21-26 classifies 403 by code; prev:95-107 sends Telegram's wording to the log only; TestSettingsEditPreviewErrors | CLOSED |
| T-11-50 | 12 | D | mitigate | settings_edit_test.go:928-944 checks every Preview and Full preview button against 64 bytes | CLOSED |
| T-11-51 | 13 | T | mitigate | the album SET NX (door:109-111, :127-137; input:457-468) runs before the nonce claim and Redis is shared by all replicas; TestSettingsEditAlbumWritesOnce passes under -race | CLOSED |
| T-11-52 | 13 | D | mitigate | siblings return at door:109-111 before r.run, so they get no refusal; TestSettingsEditRefusedAlbumOneRefusal | CLOSED |
| T-11-53 | 13 | T | mitigate | input:452 and :462, door:118-120: a key needs 1 to 20 ASCII digits; TestSettingsEditAlbumNotice ("abc" writes no key) | CLOSED |
| T-11-54 | 13 | T | mitigate | chk:260-262 refuses dataType -1 (poll and the like) and VIDEO_NOTE; TestSettingsEditMediaKindsDoor | CLOSED |
| T-11-55 | 13 | T | mitigate | TestSettingsEditMediaParity, TestSettingsEditButtonsParity, TestSettingsEditWholeMessageParity | CLOSED |
| T-11-56 | 14 | I | mitigate | settings_staff_privacy_test.go renders the editor pages, prompts and refusals in 8 languages and runs a non-creator's private editor flow end to end | CLOSED |
| T-11-57 | 14 | D | mitigate | TestSettingsEditScreensFitEveryLocale (3800 UTF-16 units, 64 bytes) | CLOSED |
| T-11-58 | 14 | R | mitigate | AGENTS.md (Handlers, Callbacks, Data, Permissions, Testing) and the manual docs are updated; `make check-docs` shows no drift | CLOSED |
| T-11-SC | 01-14 | T | mitigate | `git diff --exit-code 72a692b -- go.mod go.sum` exits 0, and so does the diff against `f662984` | CLOSED |

---

## New Threats Found by the Audit (unregistered)

The summaries raised no threat flags. The auditor found four threats of its own; none reached the `high` gate. Their
final dispositions after the review-fix pass:

| ID | Severity | Finding | Disposition |
|----|----------|---------|-------------|
| NEW-01 | medium | `/cancel` sent in a group made `helpers.WrapCommand` end the update, so such a message skipped aispam, antiflood, locks, blacklists, filters, the log channel and the late raid flood counter (a member could post `/cancel <spam link>` past moderation). Confirmed by a probe through `go test -overlay`. | **Fixed** in `4890409`: `settingsCancelPassOn` takes a `/cancel` of any non-private chat first and returns `nil`, which moves the update on to the next handler group (`ext.ContinueGroups` would only reach the pipeline's own `/cancel`). Tests: it reaches a group-7 sentinel and is counted by `raidFloodLateWatcher` like an unknown slash word. |
| NEW-02 | low | A Full or text preview of a greeting that contains `{rules}` reads the rules through the join path's reader, which creates an empty rules row (and a chats row) for a chat with none, as a real join does. | **Accepted** (AR-02): harmless; AGENTS.md and the comment in `prev` now say it plainly (`b66d165`). |
| NEW-03 | low (pre-existing) | The GORM logger printed bound values, so a failed or slow (over 1 s) write could log the owner's welcome, goodbye or rules text, button links included. `/setwelcome` had the same exposure. | **Fixed** in `32f8a31`: `ParameterizedQueries: true` in `fuku/db/conn.go`. |
| NEW-04 | info | The door's pass-through Redis read is bounded by 5 s (`settingsInputRedisTimeout`), so a Redis brownout could hold dispatcher routines per private message; the two-key scripts have no shared hash tag (fine for the single-node client in use, as the staff pacer already does). | **Accepted** (AR-03). |

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01 | T-11-10 | The greetings row is no longer read through the cache by anything, so there is no cached copy for a one-part write to leave stale; `upsertGreetingSettings` still calls `cache.DeleteCache` on `greetings:<chat>`. Research S-05. | orchestrator, from the audit verdict | 2026-11-04 |
| AR-02 | NEW-02 | A `{rules}` preview creates an empty rules row exactly as a join would; no other row is written. Stated in AGENTS.md. | orchestrator | 2026-11-04 |
| AR-03 | NEW-04 | The 5 s Redis bound of the pass-through read and the missing hash tag are informational; a 250 ms bound for the read alone is a possible later hardening. | orchestrator | 2026-11-04 |
| AR-04 | WR-01, WR-03, IN-04 (review) | A text over the limit that a client splits into several messages stores whichever part claims the wait first; a claim that is never finished or released (a crash or a Redis timeout on finish) blocks the sender's non-command private messages until the wait's 10 minutes end (pressing Set again replaces it); a typed reply that lands on a replica still on the previous code gets no answer. None writes anything without authority; all are in the AGENTS.md editor text. | orchestrator | 2026-11-04 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-11-04 | 59 | 58 (+1 accepted) | 0 | gsd-security-auditor (verdict), orchestrator (file) |

Tests run for the audit (all passing, no skips): `^(TestSettings|TestLoadSettings|TestGreeting|TestRules|TestFillPlaceholders|TestValidButtonURL|TestTextLengthRefusal|TestLoadGreetingSettings|TestLoadRules|TestVisibleLength|TestExtractMedia)`
over `./fuku/modules ./fuku/i18n ./fuku/db/greetings ./fuku/db/rules ./fuku/utils/content ./fuku/utils/formatting`; `make check-docs`
(no drift); `go.mod` and `go.sum` unchanged against `72a692b` and `f662984`; every test named in the register exists.
After the review-fix pass the orchestrator re-ran the settings set, the greeting, rules and raid flood set, `./fuku/db`,
`make check-translations` and `make check-docs`. No Phase 11 guarantee rests on PostgreSQL (no migration, partial index
or `CHECK`), so nothing joins `make test-postgres-integrity`.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed

**Approval:** verified 2026-11-04
