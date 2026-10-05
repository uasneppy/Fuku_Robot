---
phase: "2"
slug: "staff-actions-across-groups"
status: verified
# threats_open = count of OPEN threats at or above workflow.security_block_on severity (the blocking gate)
threats_open: 0
asvs_level: 1
register_authored_at_plan_time: true
created: "2026-10-05"
---

# Phase 2 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> The register comes from the `<threat_model>` blocks of plans 02-01 to 02-07, and the threat flags from their SUMMARY.md files. All seven summaries report "Threat Flags: None", meaning no surface outside the plans' threat models.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Staff Group member to staff command | Any member of the Staff Group can send the command. The arguments, the reply context and the sender identity are all influenced by the sender | Target, duration, reason, sender identity |
| Telegram callback to card | Every chat member can see and replay callback_data | 16-hex card token |
| Several bot replicas to one card | Any replica may receive a Confirm tap. Only Redis is shared between them | Card state (pending → running → terminal) |
| Card creation to Confirm | Up to 5 minutes pass, and links and other staff actions can change in that time | Link set, target lock |
| Bot to linked groups | Each moderation call uses the issuer's authority in a different chat. restrictChatMember and unbanChatMember replace the member's status on the server | Ban, restrict and unban calls |
| Live member status to decision | The target's status comes from Telegram and is trusted only for the group it came from | getChatMember status, until_date |
| Staff member text to duration | Any Staff Group member types the duration token as free text | Duration token |
| Staff member text to target resolution | Usernames can change hands, stored rows can be stale, and entities carry user IDs | @username, text_mention, numeric ID |
| Users table to Staff Group reply | The ambiguity list shows stored names, IDs and dates | Display name, user ID, last-seen date |
| User text to HTML messages | Reasons, names, titles and Telegram error text are rendered as HTML | Free text |
| Bot fleet to Telegram Bot API | All replicas share one bot token and one flood budget | Paced API calls, retry_after |
| Redis to pacer | Pacing state is shared, and Redis can fail mid-run | Slot and block keys |
| Bot to the Staff Group card | Edits are rate limited per chat, and staff can delete the card | Progress and final summary |
| Process lifecycle to in-flight runs | A shutdown can interrupt a fan-out part-way through | Unfinished group lines |

---

## Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation | Status |
|-----------|----------|-----------|----------|-------------|------------|--------|
| T-02-01 | Elevation of privilege | runStaffActionInGroup | critical | mitigate | Live `getChatMember(group, issuer)` per group, accepting only creator or administrator with `can_restrict_members`; fails closed on error; `recheckLink` runs first. No admin-cache predicate appears in `staff_action*.go` (non-comment grep is empty). Tests: `TestStaffActionSkipsWhereIssuerNotAdmin`, `TestStaffActionPerGroupGates` | closed |
| T-02-02 | Spoofing | staffActionConfirm / staffActionCancel | high | mitigate | The issuer is compared with `query.From.Id` inside the Lua compare-and-set, and the chat is checked against `card.StaffChat`. Test: `TestStaffActionCardIssuerOnly` | closed |
| T-02-03 | Tampering | callback data | high | mitigate | Callback data carries only a `crypto/rand` 16-hex token; all state stays on the server; the token is parsed strictly and used once. Test: `TestStaffActionCardTapAnswers` | closed |
| T-02-04 | Elevation of privilege | decideStaffAction | high | mitigate | Creator and administrator targets are skipped before any branch; the bot and Telegram service IDs short-circuit with no call. Test: `TestStaffActionPerGroupGates` | closed |
| T-02-05 | Spoofing | handleStaffAction | high | mitigate | `IsAnonymousSender` runs before parsing or any lookup, and there is no anonymous-admin re-entry. Test: `TestStaffActionAnonymous` | closed |
| T-02-06 | Tampering | staff_action_summary.go | medium | mitigate | Every user value goes through `html.EscapeString` or `staffDisplayTitle`, and tokens are spliced in after translation | closed |
| T-02-07 | Elevation of privilege | confirm-time checks | medium | mitigate | At Confirm: `GetStaffGroupFresh`, a live membership check and a fresh link list. Test: `TestStaffActionConfirmAborts` | closed |
| T-02-08 | Tampering | interceptor ordering (STAFF-13) | high | mitigate | Outside Staff Groups the interceptor returns `ext.ContinueGroups` with no side effects; a baseline dispatcher is compared with the full one. Test: `TestStaffActionNonStaffUnchanged` | closed |
| T-02-09 | Information disclosure | fan-out | medium | mitigate | Nothing is posted into linked groups except the moderation call; tests assert zero sendMessage calls to linked groups | closed |
| T-02-10 | Tampering | executeStaffCall / decideStaffAction | high | mitigate | A status-first decision table: restrict only for a member or restricted target, a member-removing unban only for kick, unban always with `only_if_banned=true`. Tests: `TestStaffActionDecisionInvariants`, `TestStaffActionNeverLiftsBan` | closed |
| T-02-11 | Elevation of privilege | staff /unban and /unmute | low | accept | See AR-02-01 | closed |
| T-02-12 | Tampering | staff /kick on a muted member | low | accept | See AR-02-02 | closed |
| T-02-13 | Tampering | parseStaffActionArgs | medium | mitigate | Strict lowercase `^[0-9]+[mhdw]$` grammar, and the card always shows the parsed duration or "permanent" before Confirm. Test: `TestStaffActionDurationTokenRules` | closed |
| T-02-14 | Denial of service | ParseDurationToken | low | mitigate | Overflow is checked before multiplying, and anything over the limit maps to permanent with no clamp. `duration_token_test.go` covers the overflow cases | closed |
| T-02-15 | Spoofing | resolveStaffTarget | high | mitigate | Case-insensitive multi-match refusal, unseen usernames refused, no channel or live-Telegram fallback, and the resolved name and ID shown on the card. Tests: `TestStaffActionUsernameAmbiguous`, `TestStaffActionUsernameUnknown` | closed |
| T-02-16 | Information disclosure | ambiguity listing | low | accept | See AR-02-03 | closed |
| T-02-17 | Tampering | text_mention parsing | medium | mitigate | UTF-16 offsets through `extractEntityText`, and the entity must be the first argument. Test: `TestStaffActionTextMention` (non-ASCII names) | closed |
| T-02-18 | Elevation of privilege | refused variants | medium | mitigate | In a Staff Group, `/sban`, `/dban`, `/skick`, `/dkick`, `/smute` and `/dmute` reply with a hint and make no write. Test: `TestStaffActionRefusedVariants` | closed |
| T-02-19 | Tampering | Confirm across replicas | high | mitigate | Lua compare-and-set on the Redis card, with no card state held in process. Tests: `TestStaffActionConfirmExactlyOnce`, `TestStaffActionConfirmAcrossReplicas` | closed |
| T-02-20 | Tampering | concurrent staff actions on one target | high | mitigate | A per-target `SET NX` lock held for the whole run and released by compare-and-delete. Test: `TestStaffActionTargetLock` | closed |
| T-02-21 | Elevation of privilege | links changed between card and Confirm | medium | mitigate | Confirm compares the count and a sorted-ID signature and aborts on any change. Test: `TestStaffActionLinksChangedAborts` | closed |
| T-02-22 | Denial of service | target lock never released after a crash | low | mitigate | 30-minute safety TTL, released by a deferred call in the run goroutine. Test: `TestStaffActionTargetLockReleased` | closed |
| T-02-23 | Tampering | stale pending card after a restart | low | mitigate | Lazy expiry inside the compare-and-set on any tap, plus the hash TTL. Test: `TestStaffActionCardExpiryBoundary` | closed |
| T-02-24 | Denial of service | fan-out amplification | high | mitigate | One Redis slot reservation per fan-out call, 4 workers, a Confirm tap per action and one run per target. Tests: `TestTelegramPacerFleetSpacing`, `TestStaffActionWorkersBounded` | closed |
| T-02-25 | Denial of service | a 429 on one replica ignored by others | medium | mitigate | A shared block key is extended from every retry_after and honoured by every reservation. Test: `TestTelegramPacerSharedBlock` | closed |
| T-02-26 | Repudiation | a misclassified failure hides its cause | low | mitigate | A live, paced probe of the bot's rights on 400 or 403; otherwise the Telegram description is shown. Test: `TestStaffActionFailureClassification` | closed |
| T-02-27 | Denial of service | unbounded retry_after waits stall a run | medium | mitigate | Each wait is capped at 60 s, with 3 retries, after which the run fails as "rate limited" but still sets the block. Test: `TestTelegramPacerRetryAfterCap` | closed |
| T-02-28 | Repudiation | collapse and overflow | medium | mitigate | Only done lines collapse; skipped and failed lines are always listed; overflow goes to continuation messages. Tests: `TestStaffActionSummaryCollapse`, `TestStaffActionSummaryOverflow`, `TestStaffActionOverflowContinuation` | closed |
| T-02-29 | Denial of service | edit rate limit on the card | medium | mitigate | One writer edits at most every 2.5 s, the final edit retries on 429, and a new message is posted if the edit fails. Tests: `TestStaffActionFinalEditRetriesOn429`, `TestStaffActionFinalFallsBackToNewMessage` | closed |
| T-02-30 | Repudiation | shutdown mid-run | medium | mitigate | `StopStaffActions` marks unfinished groups "interrupted by restart" and delivers within 30 s. It is registered in `main.go:218`, after the DB-close handler, so it runs first (LIFO). A hard crash is accepted (AR-02-04). Test: `TestStopStaffActionsFinalizes` | closed |
| T-02-31 | Repudiation | a worker panic drops a group | medium | mitigate | Slots are pre-filled, every goroutine recovers, and a post-wait sweep marks leftovers "internal error". Test: `TestStaffActionWorkerPanicReported`, plus the coordinator sweep | closed |
| T-02-SC | Tampering | npm/pip/cargo installs | low | accept | See AR-02-05 | closed |

*Status: open · closed · open — below high threshold (non-blocking)*
*Severity: critical > high > medium > low — only open threats at or above workflow.security_block_on count toward threats_open*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-02-01 | T-02-11 | A staff `/unban` or `/unmute` can reverse a ban or mute placed by a group's own admins. It does so only in groups where the issuer is, live, an admin with the restrict right, which is the same authority a local `/unban` has there | Plan 02-02 threat model | 2026-10-05 |
| AR-02-02 | T-02-12 | A staff `/kick` on a muted member drops the restriction, so the user can rejoin unmuted. This matches per-group `/kick` and is flagged for owner review at verification | Plan 02-02 threat model | 2026-10-05 |
| AR-02-03 | T-02-16 | The ambiguity list shows only users who used the exact username typed. It appears only inside the Staff Group, to trusted staff, and is needed to pick the right ID (D-04). Names are HTML-escaped and capped at 64 runes | Plan 02-04 threat model | 2026-10-05 |
| AR-02-04 | T-02-30 (hard crash) | A SIGKILL or OOM in the middle of a run can leave ⏳ lines on the card. Re-running the same command is safe, because the decision table never shortens or lifts anything. Phase 3's durable audit record could back a resume (OQ4) | Plan 02-07 threat model | 2026-10-05 |
| AR-02-05 | T-02-SC | No packages were installed: `go.mod` and `go.sum` are unchanged from 039c915 (phase start) to the phase head. errgroup, miniredis and go-redis were already required | Plan threat models 02-01 to 02-07 | 2026-10-05 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-10-05 | 32 | 32 | 0 | /gsd-secure-phase (orchestrator, L1 grep depth; the auditor was skipped under the threats_open 0 / plan-time register / ASVS 1 short-circuit) |

## Security Audit 2026-10-05

| Metric | Count |
|--------|-------|
| Threats found | 32 |
| Closed | 32 |
| Open | 0 |

Evidence at L1 depth:
- All 32 named mitigation tests exist.
- `make test` passed after every wave merge.
- Every Per-Task Verification Map command in `02-VALIDATION.md` passed under `-race` on the phase head.
- A non-comment grep of `alita/modules/staff_action*.go` finds no admin-cache predicate.
- `crypto/rand` is imported in `staff_action_card.go`.
- `git diff 039c915 HEAD -- go.mod go.sum` is empty.

Behaviour against the real Telegram Bot API (ban preservation under mute, flood control) is outside L1. It is tracked as manual-only in `02-VALIDATION.md`.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-10-05
