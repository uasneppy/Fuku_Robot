---
phase: 12
review: 12-REVIEW.md
titles: json
findings:
  - id: WR-01
    severity: Warning
    disposition: fixed
    title: "Greet once fails open when the greetings read fails: an already-greeted member is welcomed again, with no claim made"
  - id: WR-02
    severity: Warning
    disposition: fixed
    title: "A greetings import whose payload omits `welcome_settings` or `goodbye_settings` aborts the whole import on PostgreSQL"
  - id: IN-01
    severity: Info
    disposition: fixed
    title: "A panic between the Greet once claim and the send keeps the claim, so that member is never welcomed"
  - id: IN-02
    severity: Info
    disposition: deferred
    title: "`/setrules` still stores a text that cannot be the caption of the stored rules media, and replies \"set successfully\""
  - id: IN-03
    severity: Info
    disposition: deferred
    title: "`TestGreetedMembersStayOutOfBackup` can only run on SQLite: the full reset fails on PostgreSQL at the warns module (pre-existing)"
  - id: NEW-03
    severity: Info
    disposition: deferred
    title: "Security audit: the owner-only backup import writes rules media, buttons and the Media under text flag without the editor's checks"
open: 0
total: 6
---

# Phase 12: Code Review Disposition

Findings WR-01 to IN-03 come from `12-REVIEW.md` (deep review, 0 critical, 2 warnings, 3 info). The security audit
(`12-SECURITY.md`, 0 open threats) found four unregistered items. NEW-01 is WR-01, NEW-02 is IN-01 and NEW-04 is IN-03,
found by a second reader and dispositioned once above; NEW-03 is its own entry. The fixes went through one
single-writer fix pass (`12-REVIEW-FIX.md`).

| Finding | Severity | Disposition | Source |
|---------|----------|-------------|--------|
| WR-01 | Warning | fixed | Commits e92a260 (test) and 25cde9f: `SendWelcomeMessage` reads its settings through the new `greetings.LoadGreetingSettings`, which returns the read's error. On a successful read and on not-found it behaves exactly as `GetGreetingSettings` (the row creation for a known chat included); on any other error it logs the chat ID and the error, sends nothing and claims nothing. `GetGreetingSettings`, the buttons readers and the goodbye path keep their fallback. Tests: `TestGreetOnceReadErrorFailsClosed` (Greet once on, one member already recorded, the read fails: no `sendMessage`, no new row), `TestLoadGreetingSettingsReturnsReadError`, `TestLoadGreetingSettingsMatchesGetGreetingSettings`. Same finding as the security audit's NEW-01. |
| WR-02 | Warning | fixed | Commits d614491 (test) and 230f82c: `importGreetings` fills a missing `WelcomeSettings` or `GoodbyeSettings` with an empty block before the write, so the three new `NOT NULL DEFAULT FALSE` columns never meet a SQL NULL. Reproduced first on PostgreSQL 16 (SQLSTATE 23502 for the three omitted-block payloads), green on PostgreSQL and SQLite after. `importRules` has no hole (flat model, `ButtonArray` writes `[]`). `TestImportGreetingsOmittedBlocks` joins the `test-postgres-integrity` list, so CI proves it on PostgreSQL. |
| IN-01 | Info | fixed | Commits f993551 (test) and 3ffb87c: the deferred recover of `SendWelcomeMessage` gives the claim back when a claim was won and the send was not yet attempted (`releaseUnsentWelcome`); a panic after the send was attempted keeps it, because the outcome is unknown. The existing give-back after a send error cannot run twice. Tests: `TestGreetOncePanicBeforeSendReleasesClaim`, `TestGreetOncePanicAfterSendKeepsClaim`. Same finding as the security audit's NEW-02 and the orchestrator's own note. |
| IN-02 | Info | deferred | D-03 is enforced in the editor only, and `/setrules` keeps its old behaviour with a documented, accepted deviation (12-07): a text too long to be the caption of the stored media makes `/rules` take Telegram's 400 and send the text without the media, logged at Warn with the chat ID. The optional fix, a short note in `/setrules`'s success reply, needs a new locale key in 8 languages and changes the command's reply, so it is left for the owner to decide. Recorded in 12-SPIKE.md follow-ups. |
| IN-03 | Info | deferred | Out of scope and pre-existing: `clearWarns` writes an empty `warn_mode`, which `chk_warns_mode` rejects on PostgreSQL, so a full reset rolls back there (the old `TestClearChatData_AllModules` fails on PostgreSQL too). The guarantee Phase 12 relies on, that no reset function touches `greeted_members`, holds on code reading and SQLite. Same finding as the security audit's NEW-04. To be fixed as its own change (an attempt to queue it as a separate task timed out; it is carried into the milestone summary). |
| NEW-03 | Info | deferred | Accepted as AR-04 in 12-SECURITY.md: only the creator can import (`RequireUserOwner`), so no privilege is widened. A foreign file ID or an over-long caption falls back to text through the member-path backstop; a button link Telegram rejects breaks `/rules` until the owner fixes it. Welcome buttons had the same exposure before this phase. |

Dispositions: `open` (recorded, not yet triaged), `fixed`, `skipped`, `deferred`.
Set `deferred` by hand and put the reason in the Source cell; both are preserved. A `|` in the reason is kept as prose and escaped on the next run.
