---
phase: 11-message-editor
reviewed: 2026-11-04T00:00:00Z
depth: deep
files_reviewed: 18
files_reviewed_list:
  - fuku/modules/settings_input_door.go
  - fuku/modules/settings_input.go
  - fuku/modules/settings_edit.go
  - fuku/modules/settings_edit_actions.go
  - fuku/modules/settings_edit_check.go
  - fuku/modules/settings_edit_preview.go
  - fuku/modules/settings_menu.go
  - fuku/modules/settings_pages.go
  - fuku/modules/settings.go
  - fuku/modules/greetings.go
  - fuku/modules/rules.go
  - fuku/db/greetings/repository.go
  - fuku/db/rules/repository.go
  - fuku/utils/content/content.go
  - fuku/utils/formatting/formatting.go
  - locales/en.yml
  - locales/uk.yml
  - AGENTS.md
findings:
  critical: 0
  warning: 5
  info: 4
  total: 9
status: issues_found
---

# Phase 11: Code Review Report

**Reviewed:** 2026-11-04
**Depth:** deep
**Files Reviewed:** 18
**Status:** issues_found

## Summary

The phase adds the Welcome, Goodbye and Rules editor pages, the Redis wait for a typed reply (six Lua scripts), the typed-reply door at handler group 1, the pure check, the previews, the apply functions shared with the commands, and the one-part greetings writers. I traced every path that ends in a write.

**Authority.** I found no way for someone without authority to write.
- A Set, Remove or switch press goes through `settingsPress.verify` (live `getChatMember`, `settingsGate`, then `getChat` for a private menu) and then `checkRight`.
- A typed reply goes through `authorize()`: live member, gate, `settingsRightRefusalFor`, `IsBotAdmin` on Rules, live `getChat`. Only after that come the check, the claim and the write.
- A demotion, a removal, the owner switch turned off or a Telegram 4xx all end the wait. A 429, a 5xx, a timeout and an undecided gate keep it. The one exception is the Rules bot check (WR-02).
- No group menu creates a wait.
- The wait key is built only from `query.From.Id` or `msg.From.Id`.
- Service identities are refused at Set and at the door.

**Other checks that came out clean:**
- **Previews** send only to `query.From.Id`. They never call the clean-old senders, and a 403 is classified by `TelegramError.Code`.
- **Escaping.** The group title is escaped through `staffDisplayTitle` / `html.EscapeString`.
- **Logs** carry no text, caption, button name, link or file ID.
- **Writes.** The one-part writers update only their own columns, and `upsertGreetingSettings` invalidates `greetings:<chat>`.
- **Command parity.** The command refactors (`/welcome on|off`, `/resetwelcome`, `/cleanwelcome`, `/setrules`, `/clearrules`, `FormattingReplacer` through `FillPlaceholders`, `preFixes` through `TextLengthRefusal`/`ValidButtonURL`) behave as before.
- **Handlers.** No other handler uses group 1, and the door returns `ContinueGroups` for pass-through (a panic before line 108 leaves `ret` nil, which also passes through).
- **Locales.** The 78 new `settings_edit*`/`settings_btn_edit*` keys have the same placeholders and tags in all 8 locales.
- **Vet.** `go vet -tags testtools` on the touched packages is clean.

**Remaining problems:**
- The wait store can be left claimed, and then it silently swallows every private message.
- A long message that the Telegram client splits can save the wrong half.
- One unknown answer wrongly ends the wait.
- A failed Cancel hides a wait that is still live.
- The text part's dry run does not count the `{rules}` button against the stored buttons.

## Warnings

### WR-01: A text the Telegram client splits into two messages can save its tail; the losing message is swallowed with no reply

**File:** `fuku/modules/settings_input_door.go:194-217` (check, then claim; `if !claimed { return }`), `fuku/modules/settings_input.go:142-147`
**Issue:** The client splits a pasted text over 4096 characters into several consecutive messages, and the dispatcher handles them concurrently (up to 200 goroutines). Each message reads the same live wait, passes `authorize()` and `settingsEditCheck` on its own, and races for `settingsInputClaim`. The first message to reach the claim script wins, whichever part of the text it holds. The loser gets `claimed == false` and returns `EndGroups` with no reply.
**Failing scenario:** The owner presses Set on the Rules text and pastes a 5,000-character rules text.
1. Telegram delivers part 1 (4,096 characters) and part 2 (904 characters).
2. Part 2 is shorter, so its check finishes first, it claims the wait, and its 904 characters are stored as the group's rules.
3. The owner gets "Text saved." and the page. Part 1 is consumed silently.

The same happens when part 1 is refused by the dry run (for example a `{fullname}` that pushes it past 4096): the wait stays open by design ("send it again"), and the tail is then accepted as the new text. `/setrules` with the same paste stores the head deterministically, so the menu and the command store different texts for the same input.
**Fix:**
- Make the claim depend on message order. Have the claim script record the claimant's `message_id` and refuse any claim from a higher one, or accept a claim only from the lowest `message_id` seen for this nonce. You can track the lowest ID with a small `HSET lo` min-update script run before `authorize()`.
- Answer the losing message instead of dropping it, for example: "I used only your first message; send the text as one message."
- Optionally, after a refusal, ignore messages with consecutive IDs that arrive within about 2 s.

### WR-02: On Rules, a failed bot-admin lookup ends the wait with "I'm not admin!" instead of keeping it

**File:** `fuku/modules/settings_input_door.go:269-273`; `fuku/utils/chat_status/chat_status.go:314-319`, `127-137`
**Issue:** `chat_status.IsBotAdmin` returns `false` both when the bot is not an administrator and when `getUserMemberWithCache` cannot read the bot's membership (admin cache miss, then a live `GetMember` that fails on a 429, a 5xx or a timeout). The door maps `false` straight to `r.end("chat_status_bot_not_admin")`, which deletes the wait and the marker. The function's own contract (lines 234-237) and `docs/.../permissions.md` ("if the bot cannot tell, it says the check failed and keeps waiting") say an answer that cannot be established keeps the wait.
**Failing scenario:** The bot is an administrator, but its admin-cache entry has expired. The owner types the new rules and the live `getChatMember` of the bot hits a 429. The owner gets "I'm not admin!" (false), the wait is gone, and because the marker was deleted too, sending the text again passes through to nobody with no reply.
**Fix:** Do the bot check with a call that reports an error. For example, use `lockdownLiveMember(r.b, r.wait.GroupID, r.b.Id)`, or a helper that returns `(admin bool, err error)`. Then:
- an error maps to `r.keep("settings_check_failed")`;
- only a definite non-administrator status ends the wait, with a text that says the wait ended.

To keep "the menu refuses what the command refuses", compare the definite answer the same way `IsBotAdmin` does.

### WR-03: A claim that is never finished or released silently swallows the user's private messages until the wait expires

**File:** `fuku/modules/settings_input_door.go:208-228`; `fuku/modules/settings_input.go:110`, `126`, `142-147`
**Issue:** The claim script sets `k` and keeps the wait's remaining TTL (up to 10 minutes). `k` is removed only by `settingsInputFinish` or `settingsInputRelease`, and the door only logs a failure of either (lines 220-222, 226-228). The claim also stays held in three other cases:
- the process is killed between the claim and the finish;
- `r.apply` panics (the recover at the top keeps the update consumed);
- Redis times out on the finish after a successful write.

While `k` is held:
- every non-command private message of that user is a candidate. It makes the live `getChatMember`, the gate (possibly `getChatAdministrators`) and `getChat`, loads the page, then fails the claim and is dropped with no reply (line 214-217);
- the prompt's Cancel answers `settings_edit_prompt_stale` (Busy);
- `/cancel` answers `settings_edit_cancel_nothing`.

**Failing scenario:** A Redis timeout on `settingsInputFinish` after the welcome text was saved. For up to 10 minutes everything the owner sends the bot in private (a corrected text, a question, anything that is not a command) gets no answer at all, and both cancel paths say there is nothing to cancel.
**Fix:**
- Store the claim time in the hash (`k` plus a `kt` timestamp). Have the claim, cancel and cancel-any scripts treat a claim older than, say, 30 s as abandoned, which is longer than the settle bound of `settingsInputRedisTimeout` plus `settingsReadTimeout`.
- When the claim fails because `k` is held, answer the message once ("your previous message is still being saved") instead of returning silently.
- Make `/cancel` say "a save is in progress" for Busy rather than "nothing to cancel".

### WR-04: The text part's dry run does not count the `{rules}` button against the stored buttons, so a saved text can make every join's welcome fail

**File:** `fuku/modules/settings_edit_check.go:129-163` (no call to `settingsEditButtonRows`), compared with `settings_edit_check.go:300-304` and `fuku/utils/formatting/formatting.go:333-344`
**Issue:**
- The buttons part counts the `{rules}` button that the stored text adds, holding the keyboard to 100 buttons and 8 per row. The text part does not do the reverse check against the stored buttons.
- `FormattingReplacer` appends the rules button to the stored buttons, or adds it to the last row for `{rules:same}`, whenever the group has rules.
- The file's own contract (lines 49-53) is to refuse "whatever Telegram would refuse when the message is sent at a join".

**Failing scenario:** A group has rules and a welcome with 8 buttons in its last row, stored through Set buttons (accepted). The owner then sets the text to "Welcome {first}! {rules:same}". The text check passes and the text is stored. At every join, `renderGreeting` builds a 9-button row, `sendMessage` is refused, and no welcome is posted. The same happens with 100 stored buttons and any `{rules}` token.
**Fix:** In `settingsEditCheckGreetingText`, after the length checks, run `settingsEditButtonRows(in.State.Buttons, stored)` and refuse with `settings_edit_refuse_buttons_limit` when `total > 100 || widest > 8`. A table test covering both directions would keep the two parts symmetric.

### WR-05: A Cancel that meets a Redis error still turns the prompt into the page, while the wait may still be live

**File:** `fuku/modules/settings_edit_actions.go:170-185`
**Issue:** When `settingsInputCancel` returns an error other than `errSettingsInputUnavailable` (a timeout, a dropped connection), the handler shows the alert `settings_edit_unavailable` and then falls through to `settingsEditRedraw`. That edits the prompt into the ordinary page and removes its Cancel button. The Lua script may never have run (or its reply was lost), so the wait can still be in Redis with up to 10 minutes left. Set, in the same situation, "edits nothing" (AGENTS.md).
**Failing scenario (PLAUSIBLE, needs a transient Redis error):**
1. The owner taps Cancel on a "Set text" prompt and Redis times out.
2. The prompt turns back into the Welcome page, which looks cancelled.
3. A minute later the owner sends the bot an unrelated private message.
4. The door finds the live wait, and that message is checked and saved as the group's welcome text.

**Fix:** On `err != nil`, answer the alert and return `nil` (edit nothing), so the prompt and its Cancel button stay. Keep the redraw only for the outcomes in which the script answered.

## Info

### IN-01: A preview of a greeting with `{rules}` creates a rules row (and a chats row), contradicting "a preview ... touches no row"

**File:** `fuku/modules/settings_edit_preview.go:73-84` → `fuku/modules/greetings.go` `renderGreeting` → `fuku/utils/formatting/formatting.go:318` (`rules.GetChatRulesInfo`) → `fuku/db/rules/repository.go:16-37`; `AGENTS.md:1538-1544`
**Issue:** A Full preview, or a text preview, of a Welcome or Goodbye whose text contains `{rules}` calls `rules.GetChatRulesInfo`. For a group with no rules row, that runs `chats.EnsureChatInDb` and `db.CreateRecord`. AGENTS.md says a preview "touches no row" and that "a page, a preview and a typed save never create a row". The join path does the same, so the impact is only the documented guarantee. Also, the Rules Full preview builds its heading in the presser's language (`buildRulesText(p.tr, ...)`), while `/rules` uses the group's language, so it is not exactly "as members see it" (D-10).
**Fix:** Give the preview a side-effect-free rules read (`rules.LoadRulesContext`) for the button decision, for example a `FillPlaceholders` plus rules-button helper that takes the stored rules as an argument. Otherwise, correct the AGENTS.md sentence. Use the group's language for the Rules preview heading.

### IN-02: A stale prompt of the same group, page and part ends the newer wait

**File:** `fuku/modules/settings_input.go:125-136`, `fuku/modules/settings_edit_actions.go:170`; `AGENTS.md` (Data: "the button also compares `c`, `p`, `t`, so a stale prompt never ends a newer wait")
**Issue:** The Cancel script compares only `c|p|t`, not the prompt message ID or the nonce.
**Failing scenario:**
1. The owner has two private menus of one group open. They press Set text on menu 1, then Set text on menu 2. Menu 2's wait is refreshed with `m` set to menu 2's message.
2. A tap on menu 1's Cancel deletes the live wait and the marker.
3. Menu 2 still says "I'll wait until …". The text the owner then sends passes through with no wait and no marker, so it gets no reply at all.

**Fix:** Put the nonce (or the prompt message ID) on the prompt's Cancel button and compare it in the script. Or, when Set refreshes a wait that has a different `m`, edit the old prompt to `settings_edit_prompt_closed`. Correct the AGENTS.md sentence either way.

### IN-03: Three "end" refusals do not say the wait ended, so the next message gets silence

**File:** `fuku/modules/settings_input_door.go:257-259`, `265-267`, `269-272`; `locales/en.yml:1571`, `3101`, `3188`
**Issue:** `end()` deletes the wait and the marker and replies `settings_gate_owner_only`, `settings_no_change_info` or `chat_status_bot_not_admin`. Unlike `settings_edit_refuse_access_lost`, none of these says "I stopped waiting". "Nothing changed" reads as "try again". The next message then has no wait and no marker, so it passes through and gets no answer.
**Fix:** Send one shared ended line after the reason (for example, the reason plus `settings_edit_wait_ended_note`, or a new `settings_edit_wait_stopped`), as `renderSettingsInputRefusal` does for the still-waiting case.

### IN-04: Rolling deploy: a typed reply delivered to a replica on Phase 10 code is dropped silently

**File:** `fuku/modules/settings_input.go:523-529`; `AGENTS.md` (Subsystem traps, rolling-deploy caveats)
**Issue:** The wait is in shared Redis, but older replicas have no group-1 door and no `/cancel` (webhook mode behind a load balancer). A reply that lands on one of them passes through with no answer, and the wait stays until it expires. Other subsystems document their rolling-deploy caveat; this one does not.
**Fix:** Add the accepted caveat to the editor bullet in AGENTS.md, or deploy the editor pages after every replica runs the door.

---

_Reviewed: 2026-11-04_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: deep_
