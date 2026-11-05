---
phase: 11
review: 11-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: deferred
    title: "A text the Telegram client splits into two messages can save its tail; the losing message is swallowed with no reply"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "On Rules, a failed bot-admin lookup ends the wait with \"I'm not admin!\" instead of keeping it"
  - id: WR-03
    severity: Warning
    disposition: deferred
    title: "A claim that is never finished or released silently swallows the user's private messages until the wait expires"
  - id: WR-04
    severity: Warning
    disposition: fixed
    title: "The text part's dry run does not count the `{rules}` button against the stored buttons, so a saved text can make every join's welcome fail"
  - id: WR-05
    severity: Warning
    disposition: fixed
    title: "A Cancel that meets a Redis error still turns the prompt into the page, while the wait may still be live"
  - id: IN-01
    severity: Info
    disposition: fixed
    title: "A preview of a greeting with `{rules}` creates a rules row (and a chats row), contradicting \"a preview touches no row\""
  - id: IN-02
    severity: Info
    disposition: fixed
    title: "A stale prompt of the same group, page and part ends the newer wait"
  - id: IN-03
    severity: Info
    disposition: fixed
    title: "Three \"end\" refusals do not say the wait ended, so the next message gets silence"
  - id: IN-04
    severity: Info
    disposition: fixed
    title: "Rolling deploy: a typed reply delivered to a replica on Phase 10 code is dropped silently"
  - id: NEW-01
    severity: Warning
    disposition: fixed
    title: "Security audit: `/cancel` in a group ends every later handler group, which bypasses moderation"
  - id: NEW-02
    severity: Info
    disposition: fixed
    title: "Security audit: a preview can write a rules row"
  - id: NEW-03
    severity: Info
    disposition: fixed
    title: "Security audit: GORM's logger can print the owner's text (pre-existing)"
  - id: NEW-04
    severity: Info
    disposition: deferred
    title: "Security audit: the door's pass-through Redis read is bounded by 5 s; two-key scripts have no shared hash tag"
open: 0
total: 13
---

# Phase 11: Code Review Disposition

Findings WR-01 to IN-04 come from `11-REVIEW.md` (deep review, 0 critical, 5 warnings, 4 info). NEW-01 to NEW-04 come
from the security audit (`11-SECURITY.md`, verdict SECURED, 0 open threats) and went through the same fix pass
(`11-REVIEW-FIX.md`).

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | Warning | deferred | A client-split over-limit paste is handled message by message; Telegram gives no ordering signal for plain texts, so the first part to claim wins and the rest are dropped without a reply. It only happens when the paste is already over the limit, the owner sees the saved page with its length, and nothing is written without authority. Documented as an accepted limit in the AGENTS.md editor text (`b66d165`); recorded as AR-04 in 11-SECURITY.md. Live check in 11-SPIKE.md. |
| WR-02 | Warning | fixed | Commit 81a9d50: the Rules bot check proceeds only when `IsBotAdmin` is true (never looser than `/setrules`); a false is classified by one live `getChatMember` of the bot: a definite non-administrator or a 4xx other than 429 ends the wait, a 429, a 5xx, a transport error, a timeout or a live answer of administrator keeps it. Tests for the 429, 500 and live-administrator worlds. |
| WR-03 | Warning | deferred | A stranded claim needs a crash, a panic in the write or a Redis timeout on finish, and the effect is silence for at most the wait's 10 minutes, not a wrong write; pressing Set again replaces the hash. Documented as an accepted limit (`b66d165`, `6f9a6dc` words how the two cancel paths answer it); recorded as AR-04. |
| WR-04 | Warning | fixed | Commit a22acf1: the text part counts the stored buttons plus the `{rules}` button the text adds and refuses with `settings_edit_refuse_buttons_limit` past 100 buttons or 8 in a row. Table test in both directions. |
| WR-05 | Warning | fixed | Commit 872a2c5: a Cancel that meets a Redis error answers the alert and edits nothing, so the prompt and its Cancel stay; no Redis and every outcome the script answered still redraw the page. |
| IN-01 | Info | fixed | Commit b66d165 (text only): AGENTS.md and the `settings_edit_preview.go` comment now say a `{rules}` preview creates an empty rules row (and a chats row) as a join does, and that the Rules Full preview heading is in the presser's language. |
| IN-02 | Info | fixed | Commit 8620da7: the Cancel script also compares the pressed message's ID with the stored prompt ID (no new callback field). A refusal reply carries its own Cancel button, so a seventh Lua script notes refusal message IDs on the wait (at most 40 fields, wiped by every Set). Tests: two open menus of one group, page and part; `TestSettingsEditRefusalCancel`. |
| IN-03 | Info | fixed | Commit 01f91a3: the three refusals that end the wait append the new key `settings_edit_wait_stopped` (8 locales, limits row added). |
| IN-04 | Info | fixed | Commit b66d165 (text only): the rolling-deploy caveat of the typed reply is in the AGENTS.md editor text. |
| NEW-01 | Warning | fixed | Commit 4890409: `settingsCancelPassOn` takes a `/cancel` of a non-private chat first and returns `nil`, so every later handler group runs (`ext.ContinueGroups` would only try the pipeline's own `/cancel` and end the update). Tests: a group-7 sentinel is reached and `raidFloodLateWatcher` counts it. One existing test changed on purpose: `/cancel` in a group is now compared with an unknown slash word. |
| NEW-02 | Info | fixed | Documented, not changed (`b66d165`); see IN-01. Accepted as AR-02. |
| NEW-03 | Info | fixed | Commit 32f8a31: `ParameterizedQueries: true` on the GORM logger, so SQL log lines carry no bound values. |
| NEW-04 | Info | deferred | Informational: a 250 ms bound for the pass-through read alone is a possible later hardening; the missing hash tag is irrelevant for the single-node client in use. Accepted as AR-03. |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
