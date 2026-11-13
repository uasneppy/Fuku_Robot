# Phase 18: the remaining settings pages, live checks and confirmations

This file collects the checks that need a live Telegram client, a real supergroup or your decision. It is for you, the
owner. No tool exists for them: you do them by hand on a test supergroup (and a second small group), with the bot as an
administrator and again as a plain member, and record each answer here. It was written by the page plans one at a time and
completed, ordered and renumbered once by plan 18-15 against the finished code.

**None of these checks blocks the phase.** You chose to defer the live checks, as in Phases 7 to 17 ("defer live checks,
keep going"). The phase is complete when its automatic gate passes; this list is how you confirm or change the starting
values afterwards. Nothing here changes whether the code is safe: for every outcome (an unreadable setting, a refused or
failed press, a bot that is not an administrator, an unreachable Telegram, a lost Redis, a stale or forged button) a page
shows the cannot-open edit or a plain notice, a refused or failed press writes nothing, the AI line never names a cause, the
log channel is never named, and no button is ever disabled. The checks change wording, defaults and your confidence, not
safety.

**This file records no result.** The code is written against Telegram's documented behaviour and the fakes of the tests;
what the real server does is yours to measure.

## Before you rely on it in a real group

- **The pages change the same settings as the commands.** A value set on a page is the value its command reads, and the
  reverse (`/setwarnlimit` and `/setwarnmode`, `/reports`, `/aispam`, `/autoapprove`, the language buttons of `/lang`,
  `/log` and `/nolog`, `/anonadmin`, `/lock` and `/unlock`); there is no second copy, no migration and no new stored state.
  Another replica can still show the old value for up to `CACHE_LOCAL_TTL` (10 s by default), exactly as after the command
  (check 7 measures it for the language).
- **A Warnings change applies at the next warn.** Warns a member already has are kept; the new limit and action are used the
  next time that member is warned. Nothing is applied or cleared when you change the page.
- **Anonymous-admin mode lowers a check.** With the mode on, an anonymous admin's command runs without the proof button. Only
  the group's creator can switch it (and only while the bot is an administrator), but the page turns it on with one tap
  (A18-8, below).
- **The `all` and `messages` locks act at once.** A lock deletes every non-exempt member's message from the next message on;
  `all` and `messages` delete everything a member sends until someone taps the lock off. The page toggles them with one tap,
  as `/lock all` does (A18-8, below). Admins and approved members are exempt.
- **The language change is for every member.** It is the group's language, written as the language buttons of `/lang` write
  it; your own language stays set with `/lang` in a private chat with the bot.
- **The bot must be an administrator to change most of these from the page**, as for the commands; the page refuses with
  the bot's own notice (for example "I'm not admin!") and writes nothing.

## The checks

### 1. The warn limit and each action reached in a real supergroup

**Why:** the page stores exactly what the commands store, but only a live group shows the action being applied to a real
member and the warns being cleared afterwards.

**Steps:** in the test supergroup open `/settings`, Warnings. Set the limit to 2 and the action to Mute. As a plain test
member, get warned twice (an admin replies `/warn` twice). Check the member is muted and `/warns` shows none left. Repeat
with Ban (the member is banned; unban them) and with Kick (the member is removed and can rejoin). Then set the limit to 1
and 100 and confirm the stepper shows the toast at each end and edits nothing.

**Record:** Mute applied at the limit and warns cleared: yes / no. Ban: yes / no. Kick: yes / no. `/warnings` shows the
same limit and action as the page after each change: yes / no.

### 2. A stored `tban` shows the inert line and does nothing at the limit

**Why:** the table's CHECK allows `tban`, `tmute` and an empty action, which the warn module does not act on. They cannot
be set by any command or button, only by a database edit or a backup import, so the page says plainly that the action is
not acted on (the inert line) and offers Mute, Ban and Kick to replace it.

**Steps:** in a test group set the warn mode by hand in the database (`UPDATE warns_settings SET warn_mode = 'tban'
WHERE chat_id = <group>;`), then open Warnings. Check the page shows the stored word, the inert line and no marked choice.
Warn a test member up to the limit and confirm nothing is applied to them. Press Mute and confirm the line is gone and
the next warn at the limit mutes.

**Record:** inert line shown: yes / no. Nothing happened at the limit: yes / no. Mute replaced it: yes / no.

### 3. `/reset warnings` stores an empty action (a pre-existing defect, left for you to schedule)

**Why:** the reset module writes an empty warn mode (`backup.go`, the `WarnSettings{ChatId: chatID, WarnLimit: 3}` row), which
the warn module does not act on, so a group reset this way stops applying an action at the limit until someone picks one. The
page shows that state as Not set with the inert line. Phase 18 does not change the reset (D-17 deferred it): it is not a
Phase 18 regression, and the Warnings page is now the one place where it is visible and fixable by a press.

**Steps:** in a test group run `/reset warnings` as the creator, open Warnings and check the action reads Not set with the
inert line. Pick Mute and confirm it is stored.

**Follow-up to decide:** store `mute` in the reset instead of the empty string (one line in `backup.go` and a backup test),
outside this phase. **Record:** your decision: _____

### 4. The AI line of the Switches page in each configuration, and a stale On press

**Why:** the page decides whether the AI spam filter can run from the bot's configuration alone (a provider value and the
global switch), never from a call. Only a live bot shows the three states in a real group and what a stale button does.

**Steps:** (a) with `TYPESAFE_API_KEY` set and `ENABLE_AISPAM=true`, open `/settings`, Switches in a test group. Check the AI
block shows the state sentence followed by the data notice (read it: it is the consent a toggle should sit next to) and the AI
toggle is drawn. Turn it on and off with the button and compare `/aispam` after each. (b) Restart the bot with
`TYPESAFE_API_KEY` empty. Open Switches again: the AI block must show the state sentence and a line starting with a stop sign
saying the filter is not available on this bot, with no data notice and no name of a variable or provider; with the filter
stored off the AI toggle must be gone. (c) With the filter stored on, the toggle must still be there and Off must work.
(d) Repeat (b) with the key set and `ENABLE_AISPAM=false`. (e) Keep an older Switches message open from step (a), restart as
in (b), and press its On button: the bot must answer an alert saying the filter is not available and the message must redraw
without the AI toggle, with nothing stored.

**Record:** (a) the notice and toggle shown: yes / no. (b) the unavailable line and no notice, toggle gone: yes / no.
(c) Off worked with the toggle shown: yes / no. (d) same as (b): yes / no. (e) alert, redraw, nothing stored: yes / no.

### 5. Auto-approve's first press on a group with no greetings row, on PostgreSQL

**Why:** the first write of Auto-approve creates the whole greetings row (Welcome and Goodbye on), exactly as
`/autoapprove on` does. The Welcome and Goodbye pages read a missing row as those same defaults for a group the bot knows,
which the tests prove on SQLite. A group the bot has not met reads Goodbye as off before the first write and on after it,
for any first greetings write; every group the bot serves is known, so only a fresh database shows it.

**Steps:** on a PostgreSQL deployment, pick a group that has no `greetings` row (`SELECT 1 FROM greetings WHERE chat_id =
<group>;` returns nothing). Open Welcome and Goodbye and note the Welcome, Goodbye and Delete last lines. Open Switches and
press Auto-approve. Open Welcome and Goodbye again.

**Record:** the Welcome and Goodbye lines unchanged by the press: yes / no. The row now exists with `auto_approve` true:
yes / no.

### 6. Reports on and off from the page and the `/report` path

**Why:** the page writes `enabled` and `status` through `/reports`' own function, and the report path reads `status`.

**Steps:** in a test group with an admin and a plain member, open Switches and turn Reports off. As the member reply to a
message with `/report` and check no admin is notified. Turn Reports on from the page and repeat: the admins must be
notified. Compare `/reports` after each change.

**Record:** off: nobody notified: yes / no. On: admins notified: yes / no. `/reports` agrees with the page: yes / no.

### 7. The Language page redraws a group menu on two replicas within 10 s

**Why:** a group menu redraws in the new language from the code just written, but `chat_lang`, `chat_settings` and `chat` are
not in `skipLocal`, so another replica can answer in the old language for up to `CACHE_LOCAL_TTL` (10 s by default), exactly
as after `/lang`. Only a live deployment with two replicas shows how long the old language lasts.

**Steps:** with two replicas behind one bot, open `/settings` in a test group, Language, and pick another language. Send
`/lang` and any command that replies, repeatedly, for about 15 seconds, and note which language each reply is in and when it
changes.

**Record:** the page, its notice and its Back and Close redrawn in the new language at once: yes / no. The longest time a reply
came in the old language: ___ s (accepted up to 10 s).

### 8. A private menu keeps the opener's language and never writes it

**Why:** the page's private menu names the group's language and writes it, while the opener's own language (`/lang` in
private) is a different setting that the page must never change.

**Steps:** with your own language set to English (`/lang` in a private chat with the bot), open `/settings` in private, pick a
group that answers in another language, Language, and pick a third language. Then send `/lang` in the private chat.

**Record:** the page stayed in English and named the group's new language: yes / no. `/lang` still says your own language is
English: yes / no.

### 9. The Log channel page with a real channel: which posts stop, and the `#SETTINGS` line

**Why:** the page flips one category at a time exactly as `/nolog` does and posts the same `#SETTINGS` audit line, with the
presser as the admin. Only a real log channel shows that the bot's own posts (admin actions, user actions, automated, reports,
other) stop for the category turned off and how the audit line reads to an administrator.

**Steps:** link a real log channel with `/setlog`, open `/settings`, Log channel, and turn off one category at a time. After
each, cause an event of that category (a `/ban`, a member joining, a blacklist hit, a `/report`, a lockdown) and look at the
channel; turn the category back on and repeat. Then, with the Settings category on, turn the Admin actions category off and on
again on the page and compare the two `#SETTINGS` lines with the ones `/nolog admin` and `/log admin` post. Turn the Settings
category itself off.

**Record:** for each of the six categories, nothing is posted while it is off and the post returns when it is on: yes / no.
The two lines from the page match the commands' (same words, the presser named): yes / no. Turning the Settings category off
posts no line: yes / no. Accepted (T-18-32): with the Admin actions category off the Staff Group and lockdown copies of the
log channel are silent too, exactly as after `/nolog admin`; the page's hint says what it controls.

### 10. No channel title or ID in a group menu

**Why:** a group menu is read by every member while the log channel is private to the administrators, so the page must never
show which channel the log goes to.

**Steps:** with a log channel linked, open the Log channel page in the group as an administrator and as the owner, in a group menu
and in a private menu, and press a category. Read every line, button and toast. Then unlink with `/unsetlog` and press a cell.

**Record:** no line, button or toast names the channel or its ID: yes / no. The page says Linked or Not linked and a press without
a channel is refused with the plain notice: yes / no.

### 11. The creator switches anonymous-admin mode on and off from the page

**Why:** the page writes the mode through the function `/anonadmin` calls, and only a real group shows that an anonymous admin's
command then runs without the proof button.

**Steps:** as the group's owner open `/settings`, Anonymous admins, and turn the mode on, then send a command as an anonymous
admin (an admin with "Remain anonymous" on); then turn it off and send the same command again.

**Record:** with the mode on the anonymous admin's command runs with no proof button and with it off the proof button appears: yes /
no. `/anonadmin` with no argument then reads the same mode as the page: yes / no.

### 12. A non-owner administrator, a demoted bot, and the page opening while the bot is not an administrator

**Why:** the toggle declares the owner and the bot-an-administrator right, so each refusal and its order must read right in a real
group. The page itself opens for any administrator the owner gate admits, also while the bot is not an administrator, which is
looser than `/anonadmin`'s own status view (that view needs the bot as an administrator): a stated looseness, accepted
because viewing writes nothing and the text is what any administrator sees in the group.

**Steps:** as an administrator who is not the owner (with the owner switch on) open the page and press the toggle. Then, as the
owner, demote the bot to a plain member and press the toggle while the page is still open, and open the page again.

**Record:** the non-owner gets the alert "You need to be the group creator to do this!" and nothing changes: yes / no. With the bot demoted the
owner's press gets the notice that the bot is not an admin and nothing changes: yes / no. The page still opens while the bot is
demoted (the stated looseness): yes / no. Keep it, or make the page refuse to open without the bot as an administrator: keep / refuse.

### 13. The broad locks in a real group: `all`, `messages`, `url`, `bots` and `previews`

**Why:** a lock deletes a non-exempt member's message at once, and `all` and `messages` delete everything a member sends. The page
toggles them with one tap, as `/lock all` does (A18-8), and says only that admins and approved members are exempt. The test
fakes prove the stored rows and the watchers' reads, not what Telegram does to a live member's message.

**Steps:** in a test supergroup with the bot an administrator (with the delete right), a plain member, an approved member and
an administrator, open `/settings`, Locks, page 2 and lock `bots`, `messages` and `all` one after another (turn each off before
the next); on page 1 lock `url` and, separately, `previews` (Link previews), and have each of the four send what the lock
targets (a link, a message with a link preview, an added bot).

**Record:** the plain member's message is deleted and the administrator's and the approved member's are kept for every one:
yes / no. `url` and `previews` behave the same for a link, or differ in what they delete: same / differ (the page does not claim
they differ). Unlocking restores sending at once: yes / no.

### 14. The lock kinds that need a particular live message: `comments`, `anonchannel`, `rtl` and `forward`

**Why:** `comments` (a channel post's discussion), `anonchannel` (a message sent as a channel), `rtl` and `forward` depend on what
Telegram puts in the update, which the fakes only imitate. The page lists all 22 and claims nothing beyond the command.

**Steps:** in a test supergroup linked to a channel, with the bot an administrator, lock each of the four in turn and send what it
targets (a comment on a channel post, a message sent as the channel or an anonymous admin, a right-to-left text, a forwarded
message); unlock it again.

**Record:** each lock deletes what the command's `/locktypes` text says it does: yes / no, per lock. A lock that does nothing
live, or deletes more than its name says: _____

### 15. The right line of the Locks page when the bot cannot delete

**Why:** every draw of the Locks page makes one live read of the bot's own record and says in words when a lock cannot work (the
bot cannot delete messages), never as an error. Only a real group shows the three states.

**Steps:** open the Locks page with the bot an administrator with the delete right (no line), then take the delete right away
(the stop-sign line), then make the bot a plain member (the same line), and open the page again after each.

**Record:** no line, the stop-sign line, the stop-sign line: yes / no. No press was ever disabled: yes / no.

### 16. Phone widths: the 22 lock cells, the eight language cells and the six new Main cells

**Why:** the 22 lock cells (page 1 has six rows of two, each cell an icon and a name of at most 20 code points, the tallest
page at the private row limit by design, A18-A11), the eight language cells (a flag or a check mark, a space and the
language's own name, `Bahasa Indonesia` the longest) and the six new Main cells (`🚩 Warnings`, `🔐 Locks`, `🔘 Switches`,
`🌐 Language`, `🧾 Log channel`, `🎭 Anonymous admins`) are measured in code points, and Telegram clips a button label to the
screen width, which only a real phone shows. The Hindi marks look wider than the count.

**Steps:** on a narrow phone (about 320 dp) and a desktop client, in Hindi, Russian, Ukrainian and Indonesian (and French for
the lock names), open Main, the Language page (set the group to each of those languages) and both Locks pages in a private
menu and a group menu.

**Record:** every cell label reads whole, two to a row, with no cut-off name: yes / no, per language. Both Locks pages fit without
scrolling past the keyboard: yes / no. On the Language page the check mark replaces the flag on the stored language: yes / no.

### 17. The flag width of `Bahasa Indonesia`

**Why:** the Indonesian flag is two regional-indicator code points and renders as one wide glyph on some clients and as the two
letters `ID` on others; the cell length limit counts code points, not glyphs.

**Steps:** open the Language page on Android, iOS and Telegram Desktop and look at the Bahasa Indonesia cell, unmarked and then
marked.

**Record:** the cell fits on one line on each client: yes / no. The marked cell (`✅ Bahasa Indonesia`) fits: yes / no.

### 18. Main at ten rows on a phone

**Why:** Main now has 15 grid cells, so eight grid rows, Access for the owner and the navigation row: ten rows for the owner and
nine for an administrator, over the content-page budget (eight rows private, nine in a group) by two and one in a private
menu and by one and none in a group menu (A18-A1, check 21 asks whether that is acceptable). Only a real phone shows whether the
keyboard still sits on the screen without scrolling past the text.

**Steps:** open `/settings` as the owner, in a group menu and in a private one, on a small phone in Hindi, Russian, Ukrainian and
Indonesian, and again as an administrator who is not the owner.

**Record:** the whole keyboard and the status block are usable without scrolling past the text: yes / no. The 16 state lines read
as 16 lines (no line wraps into the next): yes / no.

### 19. A native-speaker read of the 55 new strings

**Why:** the executors translated the 55 new `settings_` keys into es, fr, hi, id, pt, ru and uk under length ceilings, and the
tests prove length and presence, not whether a native reader finds the words natural. None of the 55 should be relied on before
this read; the lock names and the two lock state words come first.

**Steps:** have a native reader of each language read these against the English: the six `settings_page_*` names (at most 18 code
points; Anonymous admins in Russian "Анонимные админы", Hindi "गुमनाम एडमिन"); the 22 `settings_lock_*` names (each at most 18, the
terms must be the ones Telegram clients use: video note, channel posts, link preview; `settings_lock_other` is "Game/sticker/GIF"
in English and may become "Other" where a three-way list does not fit; Russian and Ukrainian single words are long);
`settings_locks_state_on` and `_off` (they must read as opposites: Locked / Unlocked); `settings_state_warns` ("Limit {n},
{action}" must read as a phrase); `settings_warns_hint` and `settings_warns_action_inert` ("clear their warns" must be the term
the warn feature uses, and "I don't act on this action" must read as a fact, not an error); `settings_btn_sw_*` and
`settings_btn_anon_toggle` (with the longest state word, at most 40, Russian, Ukrainian and Hindi are the risk);
`settings_sw_ai_off` and `settings_sw_ai_unavailable` ("not available on this bot" must be neutral, naming no key or operator, and
the off sentence must match the tone of `aispam_status_enabled`); `settings_lang_hint` ("your own language" against "the group's
language" must be unambiguous, `/lang` stays Latin); the six `settings_log_cat_*` names (matching the words the log channel uses,
`/setlog` and `/unsetlog` stay Latin); `settings_anon_hint` ("owner" is the creator, the word the Access page uses).

**Record:** per language, the keys changed: _____

### 20. The `/settings` help text and the generated docs page do not name the newer pages

**Why:** `settings_help_msg` (shown by `/help` and copied into `docs/src/content/docs/commands/settings/index.md` by the docs
generator) still says the menu covers the Staff Group link, lockdown, captcha, antiflood, member requirements and the welcome,
goodbye and rules messages. It names neither the Deletion page (Phase 16 and 17) nor any of the six Phase 18 pages, and its list
of matching commands omits their commands. This was left unchanged on purpose (A16-A11 and A18-A15): changing it is a text change
in all 8 locale files under a one-message length ceiling (`TestSettingsHelpFitsOneMessage`), and the documentation and README
pages do describe the pages.

**Steps:** read the help text and decide whether to add one line about the newer pages.

**Record:** leave it / add one line (a small change in 8 locale files plus `make generate-docs`): _____

### 21. The owner confirmations A18-1 to A18-9, as amended

Each was recommended at planning and is in the code. Confirm or change:

- **A18-1 (D-08, D-15)** The Anonymous admins page uses the rights `/anonadmin` needs today, which is the live creator plus the
  bot as an administrator, so a non-owner administrator sees the page and is refused the toggle (check 12). The page is not
  `OwnerOnly`. _____
- **A18-2 (D-04, D-17)** The warn action is Mute, Ban or Kick; there is no warn duration, and a stored mode outside those (`tban`,
  `tmute`, empty) is shown as stored and inert (checks 2 and 3). _____
- **A18-3 (D-07)** The log channel's title and ID are never shown on the page, in any menu; linking stays `/setlog` and
  `/unsetlog` (check 10). _____
- **A18-4 (D-06)** In a private menu the language stays the opener's own and the group's language is shown in it (check 8). _____
- **A18-5 (D-10, D-23)** The six pages join the main grid in the two-column layout; a second hub is not built (check 18 and the
  flagged point A18-A1 below). _____
- **A18-6 (D-11)** The six pages are shown in a Staff Group like the existing pages, never naming it. _____
- **A18-7 (D-20)** The page posts the same `#SETTINGS` audit line to the log channel that `/log` and `/nolog` post (check 9). _____
- **A18-8 (research, A18-A12)** No confirmation card for the `all` and `messages` locks (parity with `/lock all`; one tap undoes it)
  or for turning Anonymous admins on (only the owner can press it, the sentence beside the button says what it does, and Off is one
  tap). The Access page's two-tap card is the pattern if you want one. **Record:** one tap is acceptable for the locks, or a card
  should guard `messages` and `all`: one tap / card. One tap is acceptable for Anonymous admins On, or a card should guard it
  (the one switch of Phase 18 that lowers a check): one tap / card. _____
- **A18-9 (D-16)** Two composite rights are added to the menu (change-info plus bot-admin for Auto-approve, owner plus bot-admin
  for Anonymous admins, the bot part judged first) so the menu refuses exactly what `/autoapprove` and `/anonadmin` refuse. The
  non-owner refusal key is `chat_status_owner_button_error` (deviation note 2). _____

### 22. The flagged UI-SPEC points

The UI-SPEC chose a default for each; they were flagged for you. Confirm or change:

- **A18-A1** Main keeps the flat two-column grid at 15 cells (10 rows for the owner, 9 for an administrator, over the content-page
  budget as check 18 describes, and over by one more row each when Phase 19 adds three cells). The fallback is a fully specified
  "More" page `h` that holds the Switches, Language, Log channel and Anonymous admins cells; it was not built because nothing forced
  it (D-23), and it is a table change. _____
- **A18-A3** The Anonymous admins right is the owner plus the bot as an administrator, because `/anonadmin` already needs the live
  creator for every change. The page opens for every administrator the owner gate admits. _____
- **A18-A4** The two composite rights are new menu rights (the alternative, declaring the older rights and letting a bot that is not
  an administrator fail at the write, was rejected as looser than the command). _____
- **A18-A7** The Warnings page offers Mute, Ban and Kick only, and draws any other stored action with the inert line (the
  Antiflood note "cannot be set from the menu" would be false here). _____
- **A18-A8** The AI toggle is not drawn while the filter cannot run and is stored off, is drawn when stored on (so it can be switched
  off), and a stale or forged On press is an alert plus an Off-only redraw (deviation note 3). Availability is the TypeSafe value and
  `ENABLE_AISPAM`, not the Anthropic key. _____
- **A18-A9** The language grid is `ENABLED_LOCALES` in its order, the stored language is marked by a check mark in place of its flag,
  the "Help Us Translate" link is not drawn (check 16). _____
- **A18-A10** The category change posts the commands' `#SETTINGS` line; a failed write shows `common_settings_save_failed` where the
  command replies with an empty list. _____
- **A18-A12** No confirmation card for Anonymous admins On (see A18-8). _____
- **A18-A13** Reused command strings are reused as they are, including `greetings_auto_approve_disabled`, whose English ends in two
  periods (an existing typo; fixing it is one existing-key edit in 8 files and changes the command's reply too). _____

### 23. The four deviation notes and the planning-level choices

Each amends or narrows a decision or the contract; they were accepted at planning or execution and are in the code. Confirm or
change:

- **Deviation note 1 (18-01)** D-23 said a control with an allow-list should accept or reject `v=1` and not be exempt; the UI-SPEC
  makes the Warnings limit and the later toggles `st` only or `tg` only. The UI-SPEC applied, and the Phase 13 subtest
  "every control of the other pages still accepts tg and st alike" skips only a control that declares an `Actions` list whose
  `Validate` accepts `v=1`; every control it checked before is still checked, and each new control has its own forged-press test.
- **Deviation note 2 (18-04)** The non-owner refusal key is `chat_status_owner_button_error` (the command's permission responder
  answers a button with it), not the UI-SPEC's `chat_status_owner_cmd_error`; D-15 left the key to the planner.
- **Deviation note 3 (18-06)** The UI-SPEC disagrees with itself on a refused AI On press (nothing edited, or an Off-only
  redraw); the alert plus the redraw of C18-11 and the Phase 15 precedent was applied.
- **Deviation note 4 (18-11)** The lock control key is the UI-SPEC's two-letter code (D-18 permits the lock's name; D-09 and the
  UI-SPEC choose the code, which keeps every button short).
- **Planning-level choices** The warn-limit tracer added its 4 keys and plan 18-02 the other 51 before any other page read them;
  the Main-shape assertions read three constants (`settingsMainShapeGroup`, `settingsMainShapePrivate`, `settingsMainOwnerRows`)
  that each page plan updated; README, the docs pages and this file were written by each page plan in its own commits and checked
  once against the code by plan 18-15.

**Record:** confirmed / changed, per item: _____

### 24. Accepted limits found while building

Read these as the known edges; none needs a decision unless you want it changed.

- **Another replica can show an old value for up to `CACHE_LOCAL_TTL`** after a write to `warn_settings`, `ai_spam_settings`,
  `greetings`, `chat_lang`, `chat_settings`, `chat`, `log_channel` or `locks_map`, exactly as after the command; reports and the
  `admin` row are not cached.
- **The Anonymous admins page opens while the bot is not an administrator** (check 12).
- **A refused press that answers with a notice redraws the page** through the common open path, so the Locks page's redraw states the
  right line again (one more bot read); a forged press or an alert refusal makes no read.
- **The Locks page's bot check loads the admin cache with one `getChatMember` of the bot while the cache is cold**; the page is not
  the cause.
- **Auto-approve's first press on a chat missing from the `chats` table** reads Goodbye off before and on after (check 5); no served
  group is in that state.
- **With the mode off, an anonymous admin's `/warn` gets the proof prompt and then `chat_status_restrict_cmd_error`** from the
  moderation gates, as every moderation command does for an anonymous sender. This is pre-existing and the page does not change it.
- **A refused `/anonadmin on` by a non-owner still creates the `admin` row** (the command's own read does); the page's reader never does.
- **`/reset warnings` stores an empty action** (check 3).
- **The Language page does not change the opener's own language** and a menu opened in private shows the group's language in the opener's.
- **No spike result is assumed anywhere in the code or the requirements**: PAGE-01 to PAGE-06 and PAGE-10 are complete on the automatic gate.

### 25. CI: lint, the full test run and the PostgreSQL integrity target

**Why:** `make lint` cannot run in the build sandbox (the installed golangci-lint is built with Go 1.25 and refuses Go 1.26.0) and
the `fuku/modules` harness does not run on PostgreSQL. CI covers both. The phase adds no PostgreSQL guarantee (no migration, no new
statement, index or CHECK), so `make test-postgres-integrity` is unchanged. `make test` now allows each package 40 minutes (it was
30): CI measured the `fuku/modules` binary at 1129 s to 1760 s of the old 1800 s limit, and every plan of the phase added tests.

**Steps:** after the branch is pushed, read the CI run: `make lint` (including `dupl`, `gocyclo`, `godox` and govet's `inline`
analyzer), `make test`, and `make test-postgres-integrity`.

**Record:** green / findings: _____
