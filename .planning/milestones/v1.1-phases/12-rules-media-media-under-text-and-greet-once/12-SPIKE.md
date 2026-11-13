# Phase 12: Rules media, Media under text and Greet once, real-device and live checklist

This file collects the checks that need a live Telegram client or a real group. Each check is its own level-2 section so
a later plan can add its checks around it without renumbering these.

## EDIT-07 limit: a GIF stored before Phase 12

EDIT-07 limit (D-07, D-08): a GIF stored before Phase 12 is still a document, shows exactly as before and gets no Media under text switch until the owner sets that GIF again, which stores it as an animation and makes the switch appear.

### Live check

1. Pick a group whose welcome GIF was saved before Phase 12 (stored as a document), or save one with the previous build.
2. Open the group's settings in private, then the Welcome page. Expected: the media line reads File or GIF, the page shows
   the file hint and no Media under text switch.
3. Set the same GIF again through Set media (or `/setwelcome` by reply to the GIF).
4. Open the Welcome page again. Expected: the media line now reads GIF and the Media under text switch and its line
   appear.
5. Turn the switch on, press Full preview, then have a test account join the group. Expected: both show the caption above
   the GIF.

## Why the rest of this file exists

This part is for you, the owner. It tells you how to prove, on real Telegram clients and a deployed bot, what the automatic tests cannot prove, what each result means and what to change when a check fails. There is no tool to run: you work through the numbered checks by hand, in a test group.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 11 ("Plan and build now, check later"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting values afterwards and how you settle the assumptions about Telegram that nobody could fetch while building (core.telegram.org was unreachable during research). The assumption numbers (A1 to A10) are the ones in the "Assumptions Log" of `12-RESEARCH.md`.

The test suite fakes Telegram with a hand-written bot client and runs on SQLite and miniredis (PostgreSQL in CI). So it proves the server logic: who may press what, what is written and what is sent, that a caption flag is sent exactly for a photo, a video and an animation, that Greet once claims once under concurrency, that no button carries more than 64 bytes, that no screen exceeds 3800 UTF-16 units and that no screen names the Staff Group, in all 8 languages. It cannot prove how real clients and the real Bot API behave, or how two live replicas behave.

Until you have run this list, the code uses its starting values: a caption above the media only when Media under text is on and the media is a photo, a video or an animation, a rules or media caption limit of 1024 UTF-16 units, and a member-path fallback to text that drops the media when Telegram refuses a rules message.

## Before you start

1. **A test supergroup** with the bot as an administrator (Restrict members and Change info rights), and a second test group. Use groups that are not your real communities.
2. **Accounts besides the bot**: the creator of the test supergroup (the owner), one administrator who has never started the bot (admin B) and one plain member who can join and leave.
3. **Four kinds of client**: Android, iOS, Telegram Desktop and Telegram Web, plus one old client version if you can find one.
4. **Phones set to Hindi, Russian and Ukrainian** (or test accounts with those languages) for the truncation check.
5. **A photo, a video, a GIF, a sticker and a document** ready to send, and a rules text of about 1030 characters.
6. **The bot's log open.** It logs refusals and failed sends by chat ID and message ID only; it never logs a message text, a caption, a button name, a link or a file ID.
7. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a screenshot.
8. **Restore anything you change** in a borrowed group (the welcome, goodbye and rules, the switches).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it passes or when you have recorded what you changed. Open the editor in private: send `/settings` to the bot, pick the group, tap Welcome, Goodbye or Rules.

### 1. A1: where the caption sits (Media under text)

- [ ] done

Steps: for each of a photo, a video and a GIF: set it as the Welcome media (Set media), turn Media under text on, tap Full preview, then make a test account join the group. Repeat on Android, iOS, Desktop and Web. Read the bot's log or the Bot API answer: the sent message echoes `show_caption_above_media: true`.

Pass looks like: on every client the caption (the welcome text) is above the photo, video or GIF, in the preview and in the real welcome. The flag is echoed as true.

If not: if one client shows the text below the media, record the client and its version; the flag is the Bot API's `show_caption_above_media` and the code sends it only in `sendPhoto`, `sendVideo` and `sendAnimation` (`media.Content.CaptionAbove`). If the Bot API refuses the flag with a 400, record the description from the log; the greeting send then fails like any refused media.

### 2. A6: an older client

- [ ] done

Steps: open the same group on an old Telegram client (one that predates caption-above-media), and have a test account join with Media under text on.

Pass looks like: the old client shows today's layout (text under the media); nothing is broken, no error message, no missing media.

If not: if the old client shows nothing or an error, record the version. The flag only changes the layout, so this would be a Telegram client bug, not a bot change.

### 3. A7: a GIF set now loops through `sendAnimation`, and an older GIF still shows

- [ ] done

Steps: set a GIF as the Welcome media now (Set media), press Full preview, then have a test account join. Then pick a group whose welcome GIF was saved before Phase 12 and read the page. The old-GIF steps are the live check of the section "EDIT-07 limit: a GIF stored before Phase 12" above; run them here.

Pass looks like: the new GIF plays and loops as an animation (the page reads GIF). The old GIF still shows exactly as before (the page reads File or GIF, no Media under text switch) until it is set again, after which the switch appears.

If not: if the new GIF shows as a plain file, check that the stored type is the animation type (9) and that the send used `sendAnimation` in the log. If `sendAnimation` is refused with a stored animation file ID, record the description; the fallback is the document send, which is what an old GIF uses.

### 4. A3: a rules text near 1024 with media, and a 1025-unit caption

- [ ] done

Steps: set a rules photo. Then set a rules text of about 1030 characters through the editor. Then, to test Telegram's own limit, send `/setrules` with a text that makes heading plus text exactly 1025 UTF-16 units while the rules media is set, and run `/rules` in the group and open the private rules link.

Pass looks like: the editor refuses the long text with the true number (heading plus text, `settings_edit_refuse_text_rules_long`). A 1025-unit caption sent anyway gets Telegram's 400 in the log, and the member path then falls back to a text-only message (the media is dropped, the text and buttons stay), so the member still gets the rules.

If not: if Telegram accepts 1025, the caption limit constant is too low for current clients; record it. If the group form has no fallback and the member gets nothing, record it as a bug: the group form degrades, the preview does not (a refused preview shows `settings_edit_preview_failed`).

### 5. Greet once on two live replicas

- [ ] done

Steps: run two bot instances against one PostgreSQL. On the Welcome page turn Greet once on. With a test account: join, leave, join again. Do it through each replica in turn.

Pass looks like: exactly one welcome for the first join and none for the later joins, whichever replica handled each. Turning Greet once off makes the next join welcome again. The Welcome page shows no number of greeted members and no name.

If not: a second welcome means the claim in `greeted_members` was lost; check the table for the `(chat_id, user_id)` row and the log for the claim error. A missing first welcome means the claim was kept after a send that really failed with an unknown outcome (accepted, see the follow-ups).

### 6. `/rules` with media and buttons on real clients

- [ ] done

Steps: set rules text, a photo and two URL buttons in the editor. Send `/rules` in the group. Turn private rules on and tap the link. Then clear the text and keep only the media (a media-only row), and send `/rules` again. Press the Rules Full preview with your own phone language different from the group's.

Pass looks like: in the group `/rules` shows the photo with the text as its caption and the buttons; with private rules on the group message is the click-to-read button and the private message carries the media and buttons; a media-only row still sends. The Full preview is the message members get, with its heading in the group's language and its header line in yours.

If not: if a client shows the buttons without the photo or the other way round, record which and the Telegram client version. If the Full preview's heading language surprises you, that is follow-up A12 below.

### 7. Hindi, Russian and Ukrainian: the five switch cells and fast taps

- [ ] done

Steps: on a phone set to Hindi, then Russian, then Ukrainian, open the Welcome page in private and look at the five switch cells (Welcome, Goodbye, Delete last, Media under text, Greet once; the state is the icon alone). Then tap one switch five times quickly.

Pass looks like: no cell label is cut off with an ellipsis, in particular the Hindi ones with combining marks. After the fast taps the page shows a consistent state, nothing is lost and no tap is retried; some taps may be answered late, that is fine.

If not: if a cell is cut off, record the language, the cell and the phone; the fix is a shorter label for that key in that locale (the limit is 20 code points rendered). If a fast tap leaves a wrong state, record the order; the press sends an absolute value, so the next press redraws it.

### 8. A native-speaker read of the new strings

- [ ] done

Steps: have a native speaker of each of es, fr, hi, id, pt, ru and uk read the Phase 12 keys in `locales/`: the five switch labels (`settings_btn_edit_switch_*`), the Greet once and Media under text lines and hints (`settings_edit_line_once`, `settings_edit_line_below`, `settings_edit_hint_once`, `settings_edit_hint_below_file`), the notice `settings_edit_below_unavailable`, the two Rules caption refusals (`settings_edit_refuse_text_rules_long`, `settings_edit_refuse_media_rules_long`) and the changed Rules hint (`settings_edit_hint_rules`). They read them on the real pages.

Pass looks like: each reads naturally and says what the setting does; "Greet once" and "Media under text" are understood.

If not: record the key, the language and the wording you want, and change that locale file only; run `make check-translations` and the settings tests afterwards.

### 9. `make lint`, `make test` and `make test-postgres-integrity` in CI

- [ ] done

Steps: push the branch and watch CI. Run `make lint` on a machine whose golangci-lint was built with Go 1.26 (the one in the build sandbox was built with Go 1.25 and refuses the repo).

Pass looks like: `make lint` clean for the lines Phase 12 added (dupl, gocyclo, godox, staticcheck, errcheck), `make test` green, and `make test-postgres-integrity` green with `TestGreetedMemberClaimConcurrent` and `TestRulesMediaMigrationDefaults` really running (not skipped) on PostgreSQL.

If not: a skipped `TestRulesMediaMigrationDefaults` in the integrity step means `FUKU_TEST_DATABASE` or `DATABASE_URL` is not set for that step; fix the CI step, never the test.

## Follow-ups the phase found

These are decisions for you, not checks. **None of them blocks the phase.**

- **UI-SPEC A1, "Rules is set" by any part:** Rules counts as set when its text, its media or its buttons are non-empty (`RulesSettings.HasContent`). If you would rather have a text-only rule, Set media and Set buttons would refuse while the text is empty.
- **UI-SPEC A2, the right of Media under text:** Welcome and Goodbye need the creator or an admin who can change info; Rules needs an admin and the bot an admin. It is one constant per page if you want the admin-only right of the on/off switch.
- **UI-SPEC A3, resets leave the switches:** `/resetwelcome`, `/resetgoodbye`, `/clearrules` and Remove leave Media under text and Greet once as they are. If you expect a reset to clear them, that is one write per reset.
- **UI-SPEC A4, icon-only state on the five switch cells:** the state is the icon alone with the word in a body line, an exception to the Phase 11 rule for those five labels only; the alternative is a 10-row Welcome page the row limit refuses.
- **UI-SPEC A12, the Rules Full preview language:** with private rules on, Full preview shows the group form in the group's language, which is not what a member sees in private; one preview cannot show both.
- **429 as a definite refusal for Greet once (research Open Question 5):** a 429 gives the claim back, so a member whose first welcome hit a flood limit is welcomed on their next join. Literal reuse of `isTelegramClientError` would instead cost that member's welcome for good during a join flood.
- **The member-path text fallback deviates from D-03:** when Telegram refuses a rules message on the member path (the group form of `/rules`), the retry drops the owner's media and sends text and buttons so the member still gets the rules. The previews never fall back (a refused preview shows `settings_edit_preview_failed`).
- **Whitespace-only rules and the `{rules}` button:** a rules text that is only whitespace now counts as empty, so a Welcome `{rules}` button that previously showed for it follows the new `HasContent` test.
- **A chat migration welcomes members once more:** a basic group that becomes a supergroup changes its chat ID, so `greeted_members` rows no longer match and members are welcomed again once.
- **The pre-existing `/rules` double escaping:** a rules text with `'`, `&` or `<` and no formatting is escaped twice by `/rules`; the editor keeps it for parity. Not changed in this phase.
- **Goodbye On by default:** a first Set on a group with no greetings row also turns the goodbye on (the database default of `goodbye_enabled`); the page shows it.
- **Plan 12-15 was dropped and the Delete-last race is known:** two welcomes or goodbyes close together on two replicas can leave one old message undeleted (research Pitfall 22, cosmetic, no data loss). `AGENTS.md` records it. The fix, one transaction that swaps the stored last-message ID and deletes the old one, is a small follow-up if you want it.
- **A panic inside `renderGreeting` after the greet-once claim** (review finding IN-01, fixed in the review-fix pass): the recover now gives the claim back when the send was not yet attempted, so the member's next join is welcomed; a panic after the send was attempted keeps it, because that outcome is unknown. Nothing to decide.
- **A greetings read that fails on a join** (review finding WR-01, fixed): `SendWelcomeMessage` reads its settings through `greetings.LoadGreetingSettings` and, on a database error, sends nothing and claims nothing, because a guess could welcome a member twice. This also means the default welcome is no longer sent during such an error in a group with Greet once off. A successful read and a missing row behave as before. Nothing to decide.
- **`/setrules` still stores a text too long to be the caption of the stored media** (review finding IN-02, deferred): the command replies "set successfully" and `/rules` then takes Telegram's 400 and sends the text without the media (the documented member-path fallback, logged at Warn with the chat ID). The editor refuses that combination; the command keeps its old behaviour. A short note in the command's reply would need a new locale key in 8 languages: your decision.
- **The owner-only backup import writes the new rules columns without the editor's checks** (security audit NEW-03, accepted): a foreign file ID or an over-long caption makes `/rules` fall back to text; a button link Telegram rejects breaks `/rules` until you fix it. Only the creator can import, and Welcome buttons had the same exposure before this phase.
- **A full reset fails on PostgreSQL at the warns module** (review finding IN-03 and audit NEW-04, deferred, predates this phase): `clearWarns` writes an empty `warn_mode` that `chk_warns_mode` rejects, so `ClearChatData` over all modules rolls back there. It needs its own change; `TestGreetedMembersStayOutOfBackup` can join the PostgreSQL integrity list once it is fixed.
- **Phase 12 live checks are deferred like Phases 10 and 11:** the checks above wait for you; the phase is judged on its automatic gate.
