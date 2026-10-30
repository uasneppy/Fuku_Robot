# Phase 9: Settings menu real-device checklist

This checklist is for you, the owner. It tells you how to prove, on real Telegram clients, the things the automatic tests cannot prove, what each result means, and which constant to change when a check fails. There is no tool to run: you work through the numbered checks by hand, in a test group.

## Why this exists

You chose to defer the live Telegram checks ("Plan and build now, check later"), as in Phases 7 and 8. A live check needs a real bot, real groups, a second and third account and several kinds of client, and none of those exist during the build.

The test suite fakes Telegram with a hand-written bot client. So it proves the server logic: who may press what, in which order the checks run, what is written, what is edited, that each press is answered once, and that no button carries more than 64 bytes. It cannot prove how real clients and the real Bot API behave: whether a `t.me` link reaches `/start` for someone who never started the bot, how an alert looks, whether an old message can still be deleted, what a forwarded keyboard does, how a forum topic behaves, what Telegram does when a stepper is tapped fast, or how long a callback stays valid. This checklist does.

Until you have run it, the code uses its starting values (a 5 minute card lifetime, the stepper sizes, 8 groups to a Staff Group page, a 45 second build bound and a 64 character title cap). The menu is built to work with them. This page is how you confirm or change them.

## Before you start

1. **A test supergroup** and **a test basic group** (not one of your real communities). The bot is an administrator in the supergroup with the Restrict members right. In the basic group the bot is an administrator too.
2. **Three accounts besides the bot**: two admins (call them admin A and admin B) and one plain member. Admin A is the creator of the test supergroup.
3. **One more admin account that has never started the bot** (admin C). It must be an administrator of the test supergroup, and its chat with the bot must have never been opened. Use a fresh account, or delete the chat with the bot and block and unblock it, so Telegram shows the Start button again.
4. **A Staff Group you created** with admin A (and a second Staff Group, also created by admin A, for the picker). Check 10 and check 12 use them; check 10 is better with at least 9 linked groups, so make a few spare test groups.
5. **Four kinds of client**: Android, iOS, Telegram Desktop and a Telegram Web client. Use at least two of them per check where a check lists a client.
6. **A phone set to Hindi and one set to Russian** (or a test account whose language is hi or ru), for the truncation check.
7. **The bot's log open.** It logs refusals and failed edits by chat ID and message ID only. It never logs a token or a key.
8. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a screenshot. This checklist only ever names things you can see in Telegram.
9. **A test group only.** Restore anything you change in a group you borrowed (`/captcha`, `/setflood`, `/autolockdown`, links, lockdowns).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not" and "Change". Tick the box when it passes or when you have recorded what you changed.

### 1. /settings and Open here in the group (the choice message, Main, Back, Close)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: as admin A, send `/settings` in the test supergroup. Read the choice message. Tap "💬 Open here". Tap Captcha, then "⬅️ Back", then "✖️ Close". Send `/settings` again and tap Close from the Main page. Do the same in the test basic group with the Captcha page.

Pass looks like: the reply is the choice message ("Where do you want to open the settings for <group>?") with two buttons, "💬 Open here" and "📩 Open in private". Open here turns the same message into the Main page (the heading "Settings", a hint line, one button per page and a line "Only <your name> can use this menu."). The name is plain text, not a mention, and nobody is pinged. Back returns to Main in the same message. Close deletes the message. Only one message is ever on screen and no new message appears for a page change.

If not: if Open here sends a second message instead of editing, the edit failed (look in the log for the chat and message ID). If a button does nothing and no alert shows, the press was not answered. If the name pings you, the line is a mention (it must not be).

Change: nothing in a constant. Report the client and the log line. The choice screen is built in `alita/modules/settings.go` (`settingsChoiceScreen`) and the pages in `alita/modules/settings_pages.go`.

### 2. Open in private for an admin who never started the bot (research A-row on deep links)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: as admin C (never started the bot), join the test supergroup as an admin if needed and send `/settings`. Tap "📩 Open in private". Telegram opens the bot's chat with a Start button. Tap Start. Then, as admin C, run the same on a second test group where you are an admin, and compare which group's menu arrives. Finally, as the plain member (not an admin), tap Open in private on a menu somebody else opened and tap Start.

Pass looks like: the URL button opens the bot's chat, Start sends `/start settings_<chat id>`, and the Main page of that group (and only that group) arrives in the admin's language, without a second tap. For the second group, the menu arrives for the second group, not the first. For the plain member, the bot answers "You are not an admin of that group, so I can't open its settings." and shows no menu. A private menu has no "Only <name>" line.

If not: if the URL button does nothing, Telegram refused the link (check the bot's username). If Start opens the chat but no menu arrives, the `/start` payload was lost: some clients drop it when the chat is opened by a first Start (record the client). If the bot answers "That settings link is not valid", the payload was cut or changed. If a menu for the wrong group arrives, stop and report it: the payload names the group and nothing else may decide it.

Change: nothing in a constant. The handler `settingsStartDeepLink` and the URL builder `settingsPrivateURL` are both in `alita/modules/settings.go`.

### 3. Another member and a demoted admin pressing the opener's menu

- [ ] done

Steps: as admin A, open `/settings`, Open here, and leave the Main page on screen. As the plain member, press a page button. As admin B (a different admin), press a page button. Then, as admin A, demote admin A's own second account, or ask admin B to open a menu, then remove admin B's admin status, and have admin B press a button on their own old menu. Repeat once in a private menu: open it as admin C, remove admin C's admin status in the group, and press a button.

Pass looks like: the plain member and admin B both get an alert "This menu belongs to the admin who opened it. Send /settings to open your own." and nothing changes. The demoted admin gets "You are not an admin of this group any more. Nothing changed." and nothing changes. In the private menu, the demoted admin gets the same "not an admin" alert and nothing changes. Every alert is a single answer: no second toast appears and no button keeps a spinner.

If not: if the plain member or admin B changes a value, stop and treat it as a bug; do not run the menu on a real group. If a press shows no reaction, the callback was not answered.

Change: this is a server rule, not a constant. Report the log lines.

### 4. Alert rendering (show_alert) on each client, and the bound toast

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: provoke each refusal from check 3 on each client, plus: (a) "I couldn't check your rights right now" is hard to cause by hand, so mark it "not reproduced" if you cannot; (b) on the Antiflood page step the limit down to Off and tap "-1" once more, then step it up to 100 and tap "+1" once more; (c) with an administrator who may not restrict members, tap Lift on the Lockdown page while a lockdown is active; (d) press a button of a menu whose bot was removed from a private chat (see check 9 for a way to do it).

Pass looks like: the refusals show as a centred alert with an OK button on Android, iOS and Desktop (a pop-up that stays until dismissed) and a readable dialog on Web. The alert text is complete, not cut, and has no markup characters. The bound messages ("The lowest value is 3." and "The highest value is 100.") show as a short toast or alert and the stepper keeps its value. The restrict refusal reads "This needs the right to restrict members, which you don't have here. Nothing changed."

If not: record which client shows a toast where you expected an alert, or an alert cut at a line limit. An alert over about 200 characters is the first suspect for a cut.

Change: alert text lives in `locales/*.yml` (keys `settings_not_your_menu`, `settings_not_admin`, `settings_no_restrict`, `settings_check_failed`, `settings_min_value`, `settings_max_value`, `settings_cannot_open_alert`). Shorten a text there if a client cuts it, in all seven files.

### 5. Button text truncation in hi and ru on a phone

- [ ] hi  [ ] ru  (Android and iOS)

Steps: set the test account's language to Hindi, then Russian, and send `/settings` in the test group. Look at the choice message buttons, the Main page buttons, then open every page: Captcha (toggle, modes, actions, stepper), Antiflood (limit stepper, Mute/Ban/Kick, the delete switch), Lockdown (Lift, the three trigger toggles and steppers), Staff Group (Link, Unlink, the picker, the confirm card Yes and No). Do it in a narrow window on Desktop too.

Pass looks like: every button label is readable. A label may end in an ellipsis on the narrowest phone, but its meaning is still clear and the value part (for example the state word or the number) is visible. No two stepper buttons collapse into the same text.

If not: record the page, the button and the language. A label that loses its state word or its number is the failure to fix.

Change: the label text in `locales/hi.yml` or `locales/ru.yml` (keys named `settings_btn_*`, `settings_opt_*`, `settings_state_*` and `settings_page_*`). Shorten the wording in that language only. Keep the placeholders. After a change run `make test` and `make check-translations`.

### 6. A forwarded menu pressed in another chat (research A2)

- [ ] done

Steps: as admin A, open `/settings`, Open here in the test supergroup, so the Main page is on screen. Forward that message to a second test group where admin A is also an admin. Forward it to a chat where admin A is not an admin. Press its buttons in each place. Then open a private menu (Open in private) and forward that message into a test group, and press its buttons there.

Pass looks like: Telegram may drop the buttons on a forward; if there are no buttons, note that and the check passes trivially. If the buttons are kept: in a chat where the presser is not an admin, the press is refused ("not an admin" or "belongs to the admin who opened it") and nothing changes in either group. In another group where admin A is an admin, the menu acts on that group (the message's own chat) and never on the original group, and only after the live admin check for that group. A forwarded private menu pressed in a group is refused as an invalid request and nothing is edited or written.

If not: if a forwarded menu ever changes the original group from another chat, stop and report it. If an edit fails and the menu shows nothing new, the user has to send `/settings` again (accepted).

Change: nothing in a constant. This is a server rule: a private menu's `c` is accepted only in a private chat and a group menu's `u` only outside one.

### 7. A forum-topic supergroup (research Open Question 1)

- [ ] done

Steps: turn Topics on in a test supergroup (the bot is an administrator). In a topic that is not General, send `/settings` as admin A, tap Open here, open Captcha, step the time limit, go Back, then Close. Repeat in the General topic. Open a menu in private for that group and use it while the topics are on.

Pass looks like: the choice message appears in the topic where the command was sent, the same message becomes the menu in that topic, and every edit stays there. No message jumps to General. Close deletes the menu from the topic. The private menu manages the group as usual.

If not: if an edit lands in General or fails, record the topic type and the log line. Edits that fail leave the menu as it was and the user can send `/settings` again.

Change: nothing yet. If edits fail in topics, the fix is a code change in the menu's edit path (`settingsPress` in `alita/modules/settings_menu.go`) to carry the thread of the message; open an issue with the evidence.

### 8. Fast stepper tapping and the 429 on edits (research A5)

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps: open the Antiflood page. Tap "+10" about ten times as fast as you can, then "-1" about ten times fast. Wait ten seconds, then send `/flood` and compare the limit with the page. Repeat on the Lockdown page with a trigger stepper and on the Captcha page with the time stepper.

Pass looks like: each button sends an absolute target value drawn when the page was drawn, so a burst of taps on one button lands on that value and does not add up. After the burst, the stored value equals the value of the last press the bot processed, the page shows that same value (a later press or Refresh redraws it) and `/flood` agrees. If Telegram rate-limits an edit, the log shows the failed edit once (by chat and message ID), nothing is retried, no wrong value is stored, and the next press redraws the page. "Message is not modified" is not an error.

If not: if the stored value and what `/flood` shows disagree after the burst, record the sequence. If the page stays frozen on an old value after a rate limit, the next press must fix it; if it does not, report it.

Change: nothing in a constant. If 429s are common in daily use, the stepper sizes below can be made coarser (see "Which constants to change afterwards").

### 9. Close on a private menu older than 48 hours (research A3)

- [ ] done

Steps: as admin A, open a menu in private (Open in private, then Start). Leave that private message in place for more than 48 hours, with the bot still in the group. Then tap Close on it. Separately, remove the bot from the test group, then press any button on a private menu of that group.

Pass looks like: the bot cannot delete a private message that old, so the message is edited to "Menu closed." with no buttons (and if the delete does work, the message simply disappears). For the removed bot, the press edits the menu to the cannot-open text ("I can't open this menu right now: the group isn't available to me. Send /settings in the group to try again.") with no buttons, and the alert says "I can't open this menu right now."

If not: if Close on an old message shows nothing and leaves the buttons, the fallback edit failed too; record the Telegram error text from the log (it is logged, not shown). If the removed-bot press shows the generic "couldn't check your rights" alert instead, the error was not recognised as a missing bot; record the log line.

Change: the edit fallback is in the menu's Close path (`settingsPress` in `alita/modules/settings_menu.go`); the missing-bot recognition is `chat_status.IsBotMissingError` in `alita/utils/chat_status`. Add the new error text there if Telegram words it differently.

### 10. The Staff Group page: callback validity after a slow build (research A4)

- [ ] a group with several linked groups  [ ] inside the Staff Group itself

Steps: link at least 9 test groups to your Staff Group (a spare group counts). Open `/settings` in one of the linked groups, Open here, and tap "Staff Group" while watching the tap. Do the same inside the Staff Group: send `/settings`, Open here and open the Staff Group page, then page through the list with the next and previous buttons. Time how long each page takes to appear.

Pass looks like: the tap stops its spinner almost at once (the bot answers the press first, then does its live health checks, up to 2N+1 Bot API calls, bounded to 45 seconds), and the page then appears by an edit. There is no "query is too old" message in the log. In a group menu the page shows "Linked to a Staff Group." and never the Staff Group's title or ID. In a private menu it shows the title and ID. Inside the Staff Group the page is read-only (8 groups a page, with paging) and has no Link, Unlink, Add group or Recent actions.

If not: if the spinner runs for more than a few seconds before the page, the answer was not sent first; report the log. If the page takes close to 45 seconds with many groups, the build bound is the limit.

Change: `staffPanelPageSize` (8) and `staffPanelBuildTimeout` (45 s) in `alita/modules/staff_panel.go`. Lower the page size if the build is too slow; do not raise the timeout past what a callback allows.

### 11. Lift from the menu, in both variants

- [ ] group menu  [ ] private menu

Steps: in the test supergroup run `/lockdown` as admin A (the bot must have the Restrict members right). Open `/settings`, Open here, tap Lockdown. The first content row should be "🔓 Lift lockdown". Tap it. A confirm card names the group, with "✅ Yes" and "❌ No". Tap "❌ No" and confirm the Lockdown page returns and the group is still locked. Tap Lift again, then tap Yes twice quickly, or tap Yes on the old card a second time. Then lock the group again and repeat from a private menu (Open in private). Also let a card sit for more than 5 minutes and tap Yes.

Pass looks like: Yes restores the group's message rights and the group shows exactly one public "lockdown lifted" notice, in both the group variant and the private variant. The second Yes says the lockdown is already lifting or already lifted and posts no second notice. The page shows the result and no Lift button after the lift. A Yes after 5 minutes says "This confirmation expired. Nothing changed. Ask again." and lifts nothing. For an administrator who may not restrict members, the Lift button shows the restrict alert and no card.

If not: if the notice is missing from the group in the private variant, report it: a lift by button must never be silent. If two notices appear, the double Yes lifted twice.

Change: the card lifetime, `settingsCardLifetime` in `alita/modules/settings_card.go`. Everything else is a server rule (`applyLockdownLift`).

### 12. Link and Unlink from a group menu: nothing in the group names the Staff Group

- [ ] one Staff Group  [ ] two Staff Groups (picker)  [ ] Unlink

Steps: as admin A (creator of the test supergroup and of the Staff Group), open `/settings` in the test supergroup, Open here, tap Staff Group, tap "➕ Link to a Staff Group". Watch the group and the Staff Group. Then unlink: tap "➖ Unlink Staff Group", then "✅ Yes". With two Staff Groups of the same creator, repeat the link and read the picker in the group menu and in a private menu. As a user who did not create the group or the Staff Group, try Link.

Pass looks like: with one Staff Group, Link links at once; the Staff Group receives its "group linked" notice, and the test supergroup shows only "This group is now linked to your Staff Group." in the menu as a notice (no title, no ID, nothing else posted in the group). With two, the group menu's picker shows positions ("👥 Staff Group 1", "👥 Staff Group 2") and the private menu shows their names; picking one links that group only. Unlink asks first, Yes unlinks, the Staff Group gets the unlink notice, and the group gets only "This group is no longer linked to a Staff Group." as a notice. The Link button refused for someone who is not the creator, with the command's own wording. The Staff Group's name never appears anywhere in a group menu in any language you try.

If not: if any group menu text, button, alert or notice names the Staff Group, stop and report the language and screen. If the Staff Group gets no notice, report the log line.

Change: this is a server rule (`settingsStaffViewFor` in `alita/modules/settings_staff.go` is the one place that decides what a group menu may show). The picker labels are `settings_btn_staff_pick_n` in `locales/*.yml`.

### 13. Captcha, Antiflood and Lockdown trigger pages against the commands

- [ ] Captcha  [ ] Antiflood  [ ] Lockdown triggers

Steps: in the test supergroup open the menu and change each setting from the menu, then run the matching command and compare, then change it by command and refresh the menu. Captcha: the toggle (On and Off), Math, Text, Turnstile, Kick, Ban, Mute and the time stepper (-1 and +1), against `/captcha`. With a member pending a captcha, press Off and check the member is released and unmuted. With the web captcha not set up, check that Turnstile is shown with the 🚫 mark and its press shows the "not set up" notice and keeps the stored mode. Antiflood: from Off, +1 gives 3 and +10 gives 12; step down to Off; Mute, Ban, Kick; the delete switch; against `/flood`. Remove the bot's admin status and change a value. Lockdown: switch each trigger (Join surge, Flood, AI spam) off and on, step each threshold, against `/autolockdown`. Repeat as an administrator without the Restrict members right.

Pass looks like: the menu and the command always show the same values, in both orders. The Turnstile press with the web captcha unset changes nothing. Without the bot as an administrator, an Antiflood change shows the "I'm not an admin in this chat!" notice and writes nothing. For the administrator without the restrict right, a trigger press gives the restrict alert and nothing changes; viewing the page still works. Antiflood on a chat that never ran `/setflood`: setting Mute, Ban, Kick or the delete switch on such a chat may also turn antiflood on at 5, exactly as the commands do (see "Accepted limits you will see").

If not: record the page, the control, the order and both outputs. A disagreement between the menu and the command is a bug in the apply function's use.

Change: nothing in a constant. Report with the two outputs.

### 14. Read the new strings in es, fr, hi, id, pt and ru

- [ ] es  [ ] fr  [ ] hi  [ ] id  [ ] pt  [ ] ru

Steps: the six translations were written without a native speaker. Read the pages and alerts in each language you can read, or ask a native speaker: the choice message, Main, every page, the alerts of check 4 and the Lift and Unlink cards. The English source is `locales/en.yml`, keys starting `settings_`.

Pass looks like: the wording is natural and the meaning matches English, especially the three lines that say nothing changed. Hindi and Russian text must also stay readable on a phone (check 5).

If not: record the key, the language and the better wording.

Change: the wording in `locales/<language>.yml`. Keep every `{placeholder}` and only the `<b>` and `<code>` tags. Run `make check-translations` and `make test` afterwards.

## What to read in the output

There is no tool output, only three places to look:

- **The client**: the choice message, the pages, the cards, the alerts, the group's permissions and the public notices.
- **The bot log**: refusals and failed edits, always by chat ID and message ID. It never logs a callback payload, a title, a name or a token, and Telegram's own error text goes to the log only.
- **The matching command**: `/captcha`, `/flood`, `/autolockdown`, `/lockdownstatus` and `/staff` show the stored values. Compare them with the page (checks 8, 11 and 13).

## Accepted limits you will see

These are known and accepted. A check that shows one of them is a pass, not a failure.

- **No auto-close.** A group menu and the choice message stay until someone uses or closes them. There is no timer. An old menu still works for the opener if they are still an admin, because every press is checked live.
- **The opener line is a name, not a mention.** "Only <name> can use this menu." carries an escaped name so it pings nobody. The text is readable by every member of the group.
- **Up to 10 seconds of skew between replicas for captcha and antiflood values (D-28).** The `captcha_settings` and antiflood cache keys are not shared across replicas instantly, so a second replica can show an old value for up to `CACHE_LOCAL_TTL` (10 seconds), exactly as the commands can. The replica that took the press is consistent. With one replica you will not see it.
- **The antiflood first-write limit (D-27).** On a chat that never ran `/setflood`, the first `/setfloodmode` or `/delflood` (and so the menu's Mute, Ban, Kick or delete switch) writes the default limit of 5 and so turns antiflood on. The page shows the stored limit, and the menu does the same as the commands. It is a known trap kept identical to the commands for this phase; a repository fix would change the commands too.
- **A stored Warn, Timed ban or Timed mute antiflood action is shown as read-only.** The menu does not offer to set those; pressing Mute, Ban or Kick replaces them.
- **A failed or rate-limited edit is logged once and never retried.** The value is already saved; the next press redraws the page.
- **An old or forwarded menu may show no change if Telegram refuses the edit.** The user sends `/settings` again.

## Decisions

| Question | Keep the current value if | Change it if |
|---|---|---|
| Card lifetime (5 minutes) | a confirm card is answered inside it | cards expire before people read them |
| Stepper sizes | fast tapping lands on sensible values | 429s are common or steps are too coarse |
| 8 groups a Staff Group page | the page builds in a few seconds | the build is slow or a page is cut |
| 45 second build bound | the page appears before the bound | the bound is hit with many groups |
| 64 character title cap | long titles fit on every page | a long title breaks a page on a phone |
| Button labels in hi and ru | every state word and number is visible | a label loses its meaning when cut |

## Which constants to change afterwards

| What | Where |
|---|---|
| Card lifetime, `settingsCardLifetime` (5 minutes) | `alita/modules/settings_card.go` |
| Captcha time stepper (-1, +1) | `alita/modules/settings_captcha.go` |
| Antiflood stepper (-10, -1, +1, +10) and its range, `settingsFloodMinLimit` (3) and `settingsFloodMaxLimit` (100) | `alita/modules/settings_antiflood.go` |
| Lockdown trigger steppers (-10, -1, +1, +10) | `alita/modules/settings_lockdown.go` |
| Staff Group page size, `staffPanelPageSize` (8), and build bound, `staffPanelBuildTimeout` (45 s) | `alita/modules/staff_panel.go` |
| The 64 character group title cap (`maxStaffDisplayTitleRunes`, used by `staffDisplayTitle`) and the 20 character button title cap | `alita/modules/staff_panel.go` |
| Alert, label and notice wording | `locales/*.yml` (all seven files, keys starting `settings_`) |
| Missing-bot error recognition | `chat_status.IsBotMissingError` in `alita/utils/chat_status` |

After any code change run `make test`, `make check-translations` and `make lint`.

## Checks only a live run can do

These are the behaviours `09-VALIDATION.md` lists as manual-only, with the check that covers each:

1. The Open in private URL button into `/start` for a user who never started the bot, and the menu managing only that group: check 2.
2. Close on a private menu older than 48 hours: check 9.
3. Alert rendering on Android, iOS, Desktop and Web, and the bound toast: check 4.
4. Button text truncation in hi and ru: check 5.
5. A forwarded menu: check 6. A forum-topic group: check 7.
6. Fast stepper tapping and the 429 on edits: check 8.
7. Callback validity after a slow Staff Group build: check 10.
8. Lift from a group menu and from a private menu posts the public notice once: check 11.
9. Link and Unlink from a group menu leave no trace of the Staff Group in the group: check 12.
10. The pending human items of plans 09-01, 09-07, 09-08, 09-09, 09-10, 09-11 and 09-12: checks 1, 2, 5, 9, 10, 11, 12 and 13.
11. The new `settings_*` strings read naturally in es, fr, hi, id, pt and ru: check 14.

## What the checklist never does

- It makes no automated call to Telegram. You do each step by hand.
- No key, token or environment value goes into this file or any other file.
- It touches no real community: use a test group, and restore anything you change in a group you borrowed.
- It does not start or touch a database. There is no database check in this phase; the menu adds no migration, index or constraint.
- It asks nothing of PostgreSQL, Redis or the deployment beyond a running bot.
