# Phase 10: Private settings hub and owner gate, real-device and live checklist

This checklist is for you, the owner. It tells you how to prove, on real Telegram clients and a deployed bot, the things the automatic tests cannot prove, what each result means and what to change when a check fails. There is no tool to run: you work through the numbered checks by hand, in a test group.

**None of these checks blocks the phase.** You chose to defer the live checks ("Plan and build now, check later"), as in Phases 7, 8 and 9. The phase is complete when its automatic gate passes; this list is how you confirm or change the starting values afterwards and how you settle two assumptions about Telegram that nobody could fetch while building.

## Why this exists

The test suite fakes Telegram with a hand-written bot client and runs on SQLite and miniredis. So it proves the server logic: who may press what, in which order the checks run, what is read and written, what is edited, that each press is answered once, that no button carries more than 64 bytes and that no screen names the Staff Group in any of the 8 languages. It cannot prove how real clients and the real Bot API behave, or how the deployed stack behaves with several groups and two replicas.

Until you have run this list, the code uses its starting values: a 5 minute card lifetime, 8 groups to a list page, a 3 second owner lookup bound, a 30 second owner memo, a 250 ms pace between roster calls and the assumption (A1) that the Bot API shows a deleted owner as a creator named "Deleted Account" with no last name and no username.

## Before you start

1. **A test supergroup** with the bot as an administrator (Restrict members right), and a second test group. Use groups that are not your real communities.
2. **Accounts besides the bot**: the creator of the test supergroup (the owner), one administrator who is not the creator (admin B) and one plain member.
3. **A group whose creator deleted their account**, if you can get one (check 1). If you cannot, say so in the result and leave A1 as an assumption.
4. **Four kinds of client**: Android, iOS, Telegram Desktop and Telegram Web. Use at least two per check where a check lists clients.
5. **A phone set to Hindi, one to Russian and one to Ukrainian** (or test accounts with those languages) for the truncation check.
6. **The bot's log open.** It logs refusals and failed edits by chat ID and message ID only, never a token or a key.
7. **Keep secrets out of files.** Do not paste a key, a token or an environment value into this file, a ticket or a screenshot.
8. **Restore anything you change** in a borrowed group (the Access switch, links, `/captcha`, `/setflood`).

## What to run, in order

For every check: do the steps, compare with "Pass looks like", and if it does not match read "If not". Tick the box when it passes or when you have recorded what you changed.

### 1. A1: a group whose owner deleted their account

- [ ] done

Steps: in a group whose creator account was deleted, make the bot an administrator, make a real administrator (admin B) and call `getChatAdministrators` for it through the bot, or read the bot's debug log after `/admincache`. Look at the creator entry. Then, as admin B, send `/settings` in that group with the Access switch off (the default).

Pass looks like: the creator entry has the first name "Deleted Account", no last name and no username, so the bot treats the group as having no owner and lets admin B in (the choice message appears). Record the exact first name text the API returned. If the creator entry is missing from the list altogether, the bot also lets admin B in.

If not: if the creator entry shows another name, a username or a last name, admin B gets "Only the group's owner can open the settings menu." and the group is stuck behind a dead owner. Record the exact entry and change `settingsCreatorGone` (`isDeletedAccount` of the first name plus no last name and no username) in `fuku/modules/settings_gate.go`, then confirm or drop assumption A1 in `10-RESEARCH.md`.

### 2. A2: `getChatAdministrators` when the bot is only a plain member

- [ ] done

Steps: put the bot in a test group as a plain member (no admin rights) and call `getChatAdministrators` for it, then press a settings button as an administrator of that group.

Pass looks like: either the call returns the administrator list (Telegram allows it for members) or it fails with a 4xx. In both cases the bot never lets an administrator through on an unknown owner state: the press is answered "I couldn't check your rights right now. Nothing changed. Try again in a moment." when the owner state is unknown, or with the owner-only text when the list shows an owner. Record which of the two Telegram did.

If not: if an administrator gets in while the owner state was unknown, stop and report it: unknown must always refuse.

### 3. The hub list, the pick, Groups, the Access card and the gated refusals

- [ ] Android  [ ] iOS  [ ] Desktop  [ ] Web

Steps, as the owner: open the bot's private chat and send `/settings`. Read the list. Tap your test group. Read its Main page. Tap Access, then the toggle (Admins can change settings). Tap Yes on the card. Tap Back, then Groups at the bottom of Main, then Close. As admin B (switch off): send `/settings` in the group, then `/start settings_<chat id>` through the Open in private button, then press a button on an old menu. Turn the switch on as the owner and repeat. Send `/settings` to the bot as a user who administers no group.

Pass looks like: the list is one button per group with the group's title and no other text about it; the pick edits the same message into that group's Main page; Main is a two-column grid with the status block above it (one "icon name: state" line per page, Access last for the owner only) and, for the owner, Access on its own full-width row; Groups redraws the list at page 1 in the same message; Close deletes the message. The Access toggle first draws a card with Yes and No and writes nothing; Yes turns it on. Admin B is refused ("Only the group's owner can open the settings menu." on the command and the deep link, "Only this group's owner can use these settings now. Nothing changed." on a button) while the switch is off and gets in once it is on. A user with no group gets the one neutral empty text. Every alert is a single answer and no button keeps a spinner. Close also works on an old private message.

Also check on every client, with the hi, ru and uk phones: **half-width truncation.** The grid names ("icon name", at most 20 characters) and the Access toggle (with its On or Off word) are not cut off with an ellipsis. And the `<code>/admincache</code> and <code>/settings</code>` commands in the list hint copy when tapped.

If not: a cut-off grid name or toggle in a language means the label is too long for half the width. Shorten that locale string (`settings_page_*`, `settings_btn_access_toggle`) in `locales/<lang>.yml`; the limits are held by `TestSettingsLabelLengths` in `fuku/i18n/settings_locale_test.go`. A command that does not copy on tap means the `<code>` tag was lost in that locale. A Close that does nothing on an old message means the delete failed and the edit fallback did not run (look in the log).

### 4. The roster backfill and the hourly sweep, with several groups and two replicas

- [ ] done

Steps: deploy the bot with at least five groups where it is an administrator, and two replicas. After the start, wait 30 to 90 seconds and read the logs of both replicas, then open the private list as the owner. Wait an hour and read the logs again. In a group, send `/admincache` and then `/settings` and watch the roster refresh.

Pass looks like: exactly one replica runs each sweep cycle (the other logs nothing for it), the first cycle runs 30 to 90 seconds after the start and the list is filled right after it, and the sweep repeats every hour. The bot never exceeds about 4 Telegram calls a second per runner. `/admincache` and a group `/settings` that passes its checks refresh that group's rows without delaying the reply. A group the bot was removed from disappears from the list.

If not: two runners in one cycle mean the lock key `fuku:settings:sweep:lock` did not hold (check Redis); an empty list after the backfill means the roster feed did not run (look for `[AdminIndex]` log lines). The pace and interval are constants in `fuku/modules/admin_index_sweeper.go`.

### 5. `make test-postgres-integrity` in CI

- [ ] done

Steps: push the branch and read the CI job that runs `make test-postgres-integrity` against PostgreSQL. Check that it ran `TestAdminIndexReplaceConcurrent` and `TestMenuAccessUpsert`.

Pass looks like: both pass. `TestAdminIndexReplaceConcurrent` proves two replaces of one chat do not race into a unique violation or commit the union of two rosters; `TestMenuAccessUpsert` proves the column-targeted upsert writes `false` and never clobbers.

If not: report the failure with the test name. Both guarantees rest on PostgreSQL behaviour (the advisory lock and the conflict clause) that SQLite cannot prove.

### 6. A native-speaker read of the new strings

- [ ] es  [ ] fr  [ ] hi  [ ] id  [ ] pt  [ ] ru  [ ] uk

Steps: ask a native speaker of each language to read the 19 new `settings_` strings of this phase (the hub list, the Access page, the owner-gate refusals) and the new `/settings` help text (`settings_help_msg`) in `locales/<lang>.yml`, in the bot, not in the file.

Pass looks like: the words say what the English says (Admins can change settings; Owner only; the refusals), the Access toggle reads as a switch, and nothing is a word-for-word machine translation that a native speaker would not use.

If not: edit the string in `locales/<lang>.yml` and keep the placeholders. `make check-translations` and `TestSettingsLabelLengths` guard keys, placeholders and sizes, not wording.

### 7. `make lint` on a golangci-lint built with Go 1.26

- [ ] done

Steps: the lint tool in the build sandbox was built with an older Go and refuses the repo's Go 1.26.0 target, so lint did not run there. On a machine with golangci-lint v2 built with Go 1.26 or newer, run `make lint`.

Pass looks like: no new findings. The sandbox ran `go vet -tags testtools ./...` and `gofmt` instead, which found nothing new.

If not: fix the finding in the file it names; the policy is `new: true`, so only findings in this phase's code fail.
