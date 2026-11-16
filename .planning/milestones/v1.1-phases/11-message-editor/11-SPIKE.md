# Phase 11: Message editor, real-device and live checklist

This checklist is for you, the owner. It tells you how to prove, on real Telegram clients and a deployed bot, the things the automatic tests cannot prove, what each result means and what to change when a check fails. There is no tool to run: you work through the numbered checks by hand, in a test group.

**None of these checks blocks the phase.** You chose to defer the live checks ("Plan and build now, check later"), as in Phases 7 to 10. The phase is complete when its automatic gate passes; this list is how you confirm or change the starting values afterwards and how you settle the assumptions about Telegram that nobody could fetch while building (core.telegram.org was blocked during research). The assumption numbers (A1 to A9) are the ones in the "Assumptions Log" of `11-RESEARCH.md`.

## Why this exists

The test suite fakes Telegram with a hand-written bot client and runs on SQLite and miniredis. So it proves the server logic: who may press what, in which order the checks run, what a typed reply is checked against, what is written and what is sent, that a preview goes only to the presser's own private chat, that no button carries more than 64 bytes, that no screen exceeds 3800 UTF-16 units and that no screen names the Staff Group, in all 8 languages. It cannot prove how real clients and the real Bot API behave, or how the deployed stack behaves with two replicas.

Until you have run this list, the code uses its starting values: a 10 minute wait, 4096 characters for a Welcome or Goodbye text (1024 once the group has media), 3800 for the rules, at most 100 buttons and 8 to a row, and the assumptions below.

## Before you start

1. **A test supergroup** with the bot as an administrator (Restrict members and Change info rights), and a second test group. Use groups that are not your real communities.
2. **Accounts besides the bot**: the creator of the test supergroup (the owner), one administrator who has never started the bot (admin B) and one plain member.
3. **Four kinds of client**: Android, iOS, Telegram Desktop and Telegram Web. Use at least two per check where a check lists clients.
4. **A phone set to Hindi, one to Russian and one to Ukrainian** (or test accounts with those languages) for the truncation check.
5. **A GIF, a photo, a sticker, a voice message and an album of three photos** ready to send.
6. **The bot's log open.** It logs refusals and failed sends by chat ID and message ID only; it never logs a message text, a caption, a button name, a link or a file ID.
7. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a screenshot.
8. **Restore anything you change** in a borrowed group (the welcome, goodbye and rules you set, the Access switch).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it passes or when you have recorded what you changed. Open the editor in private: send `/settings` to the bot, pick the group, tap Welcome, Goodbye or Rules.

### 1. A3: a GIF as the welcome media

- [ ] done

Steps: on the Welcome page tap Set media and send the GIF. Tap Full preview. Then add a test account to the group (or have one join) and look at the real welcome.

Pass looks like: the saved page says the media is File or GIF, Full preview and the real welcome both show the GIF playing as an animation. Record which it was.

If not: if it shows as a plain file, the GIF is stored as a document and resent with `sendDocument`, exactly as `/setwelcome` by reply does today. Changing it to `sendAnimation` touches the join path, which this phase left alone, so record it as a follow-up and do not change the editor alone.

### 2. A4: an album sent for the media part

- [ ] done

Steps: tap Set media and send the album of three photos in one go. Read the replies.

Pass looks like: exactly one saved page (Media saved) with the line that you sent an album and the bot kept only one item. No second reply, no refusal for the other photos. Then tap Set media again, send an album that contains a video note or an unsupported kind as its first item, and read the replies.

If not: more than one reply means the items were handled one after another instead of in parallel (assumption A4); the album memory key `fuku:settings:inputalbum:<user>:<media_group_id>` should make the second and third items silent. Look in Redis and in the log for that key and report the order the items arrived in.

### 3. A5: Preview from a group menu by an admin who never started the bot

- [ ] done

Steps: as admin B, open `/settings` in the group (the owner must have turned the Access switch on), open the Welcome page and tap Full preview.

Pass looks like: an alert that says the bot cannot message admin B in private yet and to open the private chat and tap Start; nothing is sent to the group. After admin B taps Start in the bot's private chat and presses Full preview again, the preview arrives in private and the group sees only a short toast ("Preview sent to you in private").

If not: if the alert is the generic "I couldn't send the preview" instead, Telegram answered something other than 403 for a user who never started the bot (assumption A5). Record the error code from the log and change `settingsEditIsForbidden` in `fuku/modules/settings_edit_preview.go`.

### 4. A6: fast taps on Preview and a switch

- [ ] done

Steps: on the Welcome page tap Full preview, then Preview text and Full preview again as fast as you can, then tap the Welcome on/off switch quickly three times.

Pass looks like: every preview arrives (a header line and then the content), or a tap that was rate limited answers the generic preview alert and the next tap works. The switch ends in the state of the last tap, never a mix. No tap leaves a spinner.

If not: a preview that always fails on the second send means Telegram's per-chat limit is stricter than assumed (A6); report it. The code never retries a send.

### 5. A1, A2, A7, A8: long texts, many buttons, custom emoji, long button names

- [ ] done

Steps, each through Full preview and a real join: (a) set a welcome text close to 4096 characters, with emoji, then one just over the limit; (b) set 8 buttons in one row using the `:same` suffix, then 9 in a row; (c) set 100 buttons, then 101; (d) put a custom emoji (a premium emoji from a pack) in the text; (e) set a button with a very long name (60 characters or more) and one with a very long link.

Pass looks like: (a) the text under the limit is saved and sent, the one over it is refused with the number of characters it would be once the names are filled in; (b) and (c) the limits are enforced with the "too many buttons" refusal, and what is saved is sent; (d) the custom emoji shows as its fallback emoji unless the bot has a Fragment username, which is cosmetic; (e) Telegram sends the button or refuses it at the join, and Full preview shows the same result first.

If not: if Telegram counts characters differently from the editor (A1) a text that the editor refused would have sent, or one it accepted fails at the join: record the length and change `settingsEditTextLimit` or `settingsEditCaptionLimit` in `fuku/modules/settings_edit_check.go`. If the button limits differ (A2) change `settingsEditMaxButtons` or `settingsEditMaxRowButtons`. If Telegram has a limit on button name or link length (A8) that Full preview catches, report the numbers.

### 6. A9: a normally typed private message reaches the door

- [ ] done

Steps: tap Set text and type the new text in the normal Telegram input box, with bold and a link made through the formatting menu, then send it. Repeat with the text pasted from another chat, and with a text sent through a client that wraps messages (Telegram Web).

Pass looks like: the text is saved with its formatting and the saved page appears. An ordinary private message with no wait gets no reply from the bot at all.

If not: if the message is ignored, Telegram sent it with a `rich_message` instead of plain text (assumption A9); the door treats such a message as not a candidate and consumes no wait. Report which client produced it.

### 7. Real clients: tap to copy, tap to send, truncation

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: open the Set text prompt and the Set buttons prompt on every client. Tap a placeholder such as `{first}` and a button example. Tap `/cancel` in the footer. Read the end time. Then open the Welcome, Goodbye and Rules pages on the Hindi, Russian and Ukrainian phones.

Pass looks like: the placeholders and the button examples copy on tap (they are `<code>` text), `/cancel` sends on tap and answers that the request was cancelled, the end time reads as a clock time in UTC, and the editor buttons (Set, Remove, Preview, Full preview, Cancel) and the Main grid names are not cut off with an ellipsis at phone width.

If not: a cut-off button in a language means its label is too long for half the width. Shorten that locale string (`settings_btn_edit_*`, `settings_page_*`) in `locales/<lang>.yml`; the sizes are held by `TestSettingsLabelLengths` in `fuku/i18n/settings_locale_test.go`. A placeholder that does not copy means the `<code>` tag was lost in that locale.

### 8. Two replicas: Set on one, the reply on the other

- [ ] done

Steps: run two replicas against one Redis and one database. Press Set on the page (it is served by one replica), then send the reply as often as needed until the other replica handles it (the log shows which replica logged the save). Also press Cancel on one replica after Set on the other.

Pass looks like: the reply is saved whichever replica handles it, exactly once; Cancel on one replica ends a wait set by the other; no reply is lost and none is saved twice.

If not: a lost wait means the Redis key `fuku:settings:input:<user>` was not shared (check `REDIS_DB` on both replicas). A double save means the claim script did not hold; report the log lines of both replicas.

### 9. A reply after 10 minutes

- [ ] done

Steps: tap Set text and wait 11 minutes without sending anything. Send the text. Then send an ordinary private message such as "hello".

Pass looks like: the text gets one reply saying the wait for the text ended and nothing changed, with the instruction to press Set again; nothing is saved. The ordinary message that follows gets no reply at all. Pressing Cancel on the old prompt answers that the 10 minutes are over.

If not: a second "wait ended" reply means the marker was not taken once; a missing reply means the marker key `fuku:settings:inputdone:<user>` expired earlier than its 15 minutes (look for a Redis flush).

### 10. A native-speaker read of the new strings

- [ ] es  [ ] fr  [ ] hi  [ ] id  [ ] pt  [ ] ru  [ ] uk

Steps: ask a native speaker of each language to read the 83 new editor strings of this phase (`settings_edit_*`, `settings_btn_edit_*`, the Welcome, Goodbye and Rules page names) and the changed `/settings` help text (`settings_help_msg`) in `locales/<lang>.yml`, in the bot, not in the file.

Pass looks like: the words say what the English says, the placeholders such as `{first}` and the button syntax stay as written, and nothing is a word-for-word machine translation that a native speaker would not use.

If not: edit the string in `locales/<lang>.yml` and keep the placeholders and the `<code>` tags. `make check-translations` and `TestSettingsLabelLengths` guard keys, placeholders and sizes, not wording.

### 11. `make lint` on a golangci-lint built with Go 1.26, and `make test` in CI

- [ ] done

Steps: the lint tool in the build sandbox was built with Go 1.25 and refuses the repo's Go 1.26.0 target, so `make lint` did not run there; a local stand-in ran the `dupl`, `gocyclo`, `godox`, `staticcheck` and `errcheck` checks on the lines this phase added. On a machine with golangci-lint v2 built with Go 1.26 or newer, run `make lint`. Push the branch and read the CI run of `make test` (the sandbox's Go lacks the `covdata` tool, so `make test` was judged there by its test events).

Pass looks like: no new findings from `make lint`, and a green `make test` in CI.

If not: fix the finding in the file it names; the policy is `new: true`, so only findings in this phase's code fail. Report a CI test failure with the test name.

## Follow-ups the phase found

These are decisions for you, not checks. **None of them blocks the phase.**

- **The pre-existing `/rules` double escaping** (research Pitfall 3). `/setrules` stores the text through `tgmd2html.MD2HTMLV2`, which already escapes `'`, `&` and `<` for a text with no formatting, and `/rules` then escapes it again, so a rules text such as `Don't spam & be nice` shows `Don&#39;t spam &amp; be nice` literally unless it has some formatting. The editor keeps parity with `/setrules` and `/rules` on purpose (Rules Full preview shows exactly what `/rules` sends), so it shows the same wrong text. A fix changes `/rules` for every group that has such a text stored, so it needs your decision and its own change.
- **Goodbye is On by default for a group with no greetings row** (research Pitfall 13 and question Q1). The database default of `goodbye_enabled` is true, so the first Set (or `/setwelcome`) on a group the bot has no greetings row for also stores a Goodbye that is on, and members who leave start getting the default goodbye. The editor pages show the stored state (Goodbye: On) so you see it, and the page offers the switch. Changing the default would change `/setwelcome` for every group and is a separate decision.
- **`/cancel` in a group** (resolved by the review-fix pass, NEW-01): `settingsCancelPassOn` takes every `/cancel` of a non-private chat first, makes no reply, write or Telegram call, and lets the update go on to the later handler groups (aispam, antiflood, locks, blacklists, filters, the log channel and the late raid flood counter), so an anonymous admin's `/cancel` is no longer answered "cannot identify user" and a member's `/cancel <spam>` is no longer a way past moderation. Nothing to decide; a live check is to send `/cancel` in a test group and see that the bot stays silent.
