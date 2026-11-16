---
phase: 10
review: 10-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: deferred
    title: "A live owner whose first name is \"Deleted Account\" (no last name, no username) is treated as \"no owner\", which lets every admin in"
  - id: WR-02
    severity: Warning
    disposition: deferred
    title: "A forged `c` on a non-pick private press still tells the sender whether the bot is in that chat"
  - id: WR-03
    severity: Warning
    disposition: fixed
    title: "The roster clear fires on any Telegram 4xx of the bot's own `getChatMember`, including 401 and 403 causes unrelated to the chat"
  - id: IN-01
    severity: Info
    disposition: deferred
    title: "A group's \"Admins can change settings\" switch is lost silently when a basic group migrates to a supergroup"
  - id: IN-02
    severity: Info
    disposition: deferred
    title: "The private list can show a group title to someone the roster has not yet noticed was demoted"
  - id: IN-03
    severity: Info
    disposition: fixed
    title: "`main.go` comment says the admin index sweeper stop \"returns at once\", but it waits for the sweep goroutine"
  - id: IN-04
    severity: Info
    disposition: deferred
    title: "Every admin-cache load in a group where the bot is not an administrator now does a database transaction"
open: 0
total: 7
---

# Phase 10: Code Review Disposition

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | Warning | deferred | Behaviour kept: Telegram gives no second signal that tells a deleted owner from an owner named "Deleted Account", so detection stays name-based (the same helper the lockdown code uses). The false claim in the comment and AGENTS.md was corrected (commit 0bdbfac). The fail direction is that admins are let in, never that the owner is locked out. Owner live check A1 in 10-SPIKE.md. |
| WR-02 | Warning | deferred | Phase 9 decision D-11 is locked: a private press for a group the bot has left answers "cannot open", which is right for a real owner holding an old menu. Giving every forged `c` press the pick's neutral answer would show that owner "not admin" instead. The leak needs a hand-built callback and only says whether the bot is in a group. Optional hardening for a later phase; flagged to the owner in the phase summary. |
| WR-03 | Warning | fixed | Commit 0be0f65: only Bot API 400 and 403 clear the roster (`cache.IsBotGoneError`); 401, 404, 409, 429, 5xx and transport errors leave rows. Tests added. |
| IN-01 | Info | deferred | The switch resets to owner-only on a basic-group to supergroup migration, which is the safe direction. Re-keying `chat_settings_access` in `staff.RekeyChat`'s migration path is a possible later change. |
| IN-02 | Info | deferred | The pick is checked live, so a demoted admin is refused; only the title shows until the next roster refresh. Accepted (the roster is a hint by design, D-02). |
| IN-03 | Info | fixed | Commit 420dc69: the main.go comment now describes the wait. |
| IN-04 | Info | deferred | Accepted as research R-05.1; performance is out of scope. |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
