---
phase: "13"
status: all_fixed
findings_in_scope: 2
fixed: 2
skipped: 0
source: 13-SECURITY.md (New Threats table, NEW-01 and NEW-02)
fixed_at: "2026-11-08"
---

# Phase 13 Security Fix Pass

Both low-severity findings of the security audit are fixed test-first. Neither was a blocking threat, so
`threats_open` stays 0 and `13-SECURITY.md` stays `verified`.

| Finding | RED | GREEN |
|---|---|---|
| NEW-01 | `ac75ae7` test(13) | `44a20b2` fix(13) |
| NEW-02 | `ac75ae7` test(13) | `44a20b2` fix(13) |

One RED commit and one GREEN commit cover both findings, because the two fixes are small and independent files.

## NEW-01: a bidi control inside a name closed its own isolate

**Changed.** `memberReqStripBidi` (new, `fuku/modules/memberreq_rules.go`) drops U+061C, U+200E, U+200F, U+202A to
U+202E and U+2066 to U+2069 and keeps every other character (zero-width joiners and Arabic letters stay).
`memberReqMention` strips the chosen name before the 64-rune cut and before the U+2068 and U+2069 wrapper; a name
that is empty after the strip uses the fallback, which is stripped as well. The name rules (`memberReqNames`,
`memberReqBroken`) still judge the original name; only the display changed.

**Every name splice uses the helper.** `grep` over `fuku/modules/memberreq*.go` shows two places that put a member's
name into an mreq text: `memberReqNoticeText` (`memberreq_notice.go`, every punishment notice) and the "I fixed it"
edit (`memberreq_fixed.go:223`). Both call `memberReqMention`, so none bypassed it and nothing else had to change.
The welcome paths (`SendWelcomeMessage(... user.FirstName)`) are the greetings module's text, not a requirements
notice, and are outside this finding.

**Files.** `fuku/modules/memberreq_rules.go`, `fuku/modules/memberreq_rules_test.go`.

**Tests (all in `TestMemberReqMention`).**
- `bidirectional controls in a name are stripped (NEW-01)`: each of the 12 controls alone.
- `a name cannot close its own isolate and reorder the line (NEW-01)`: `x` U+2069 U+202E `evil` U+2066 U+2067 `z`
  renders exactly one U+2068 and one U+2069 (the wrapper's) and no U+202x, U+2066 or U+2067.
- `the controls go before the cut to 64 runes (NEW-01)`: 30 controls plus 70 letters gives 64 letters.
- `escaping survives the strip (NEW-01)`: `<b>` and `&` stay escaped.
- `a name made only of controls falls back (NEW-01)`, including a fallback that itself carries a control.
- `an ordinary Arabic name is unchanged (NEW-01)`: a joiner and Arabic letters are kept.

RED failed for the stated reason: the link text still held U+061C, U+200E and the rest, and the isolate-closing name
kept its second U+2069 and U+202E.

## NEW-02: the undelivered-notice lift did not re-check the marker

**Changed.** `memberReqUndeliveredMute` (`fuku/modules/memberreq_notice.go`) makes one paced live `getChatMember`
(`memberReqLiveMember`) before the lift and lifts only while `isMemberReqMute(live, hold.MuteUntil)` holds. The
shared revert step moved into `memberReqRevertUndelivered`.

**Decisions (binding, as given).**
- (a) When the read succeeds and the record does not carry the marker (an admin replaced the restriction, the member
  is a plain member again, left, or was banned): no `restrictChatMember` and no `getChat`, the hold still goes back to
  pending through `memberreq.RevertHoldToPending`, and one Warn names the chat and user IDs only.
- (b) When the read itself fails, the lift goes ahead anyway: a failed read on top of a failed notice is a double
  fault, and a member left under a 330 day mute with no button is the worse outcome. This fail-toward-lifting choice
  is written in the function comment and in AGENTS.md.

**Files.** `fuku/modules/memberreq_notice.go`, `fuku/modules/memberreq_failure_test.go`, `AGENTS.md` (the Subsystem
traps executor/notice bullet now says the display name is stripped of bidi controls and that the undeliverable-notice
lift checks the marker first, with the fail-toward-lifting rule; nothing else was changed).

**Tests (`TestMemberReqUndeliveredMuteChecksMarker`, with the new helper `mreqDeliveredMute`).**
- `the marker still live: the mute is lifted after one paced live read` (exactly one extra `getChatMember`, two
  `restrictChatMember` calls, hold pending).
- `the record is left alone when ...` for an admin restriction with no end date, an admin restriction with an end
  date of their own, a plain member, a member who left and a banned member: one `restrictChatMember` call (the
  original mute only), the record unchanged, the hold pending, exactly one Warn with the IDs and no name.
- `a live read that fails still lifts the mute`.

The existing `TestMemberReqMuteNoticeUndeliverable` (marker live, refused lift, unreadable group, lockdown, StaleClaim
retry) is unchanged and green.

RED failed for the stated reason: no live read happened (`getChatMember calls = 0`) and an admin's restriction was
overwritten (`restrictChatMember calls = 2`, record replaced by the group's defaults).

## Verification (real results, at `44a20b2` plus the docs commit)

| Check | Result |
|---|---|
| `gofmt -l` on the four touched Go files | no output |
| `CGO_ENABLED=0 go build ./...` | exit 0 |
| `go vet -tags testtools ./fuku/modules` | exit 0 |
| `python3 .planning/phases/13-member-requirements/handoff/lint-local.py 8c0c827` (exit read directly) | exit 0, "no dupl, gocyclo or godox findings on lines added since 8c0c827" |
| `go test -tags testtools -race -count=20 -run '^(TestMemberReqMention\|TestMemberReqUndeliveredMuteChecksMarker\|TestMemberReqMuteNoticeUndeliverable\|TestMemberReqNoticeLanguage)$' ./fuku/modules` | ok, 95.7 s |
| `go test -tags testtools -race -count=1 -timeout 30m ./fuku/modules` (whole package, background, output in the session scratchpad `modules-full.out`) | `ok ... 709.511s`, `exit=0`; no test failed, none flaked (the rejoin once-tests included) |
| `make check-translations` | exit 0, all translations present |
| `make check-docs` | exit 0, no drift |
| `go.mod` and `go.sum` | untouched |

No locale key, migration, model or handler was added, so no locale, docs or migration step applies.

## Deviations

None. No existing test was edited. The RED and GREEN commits are one pair for both findings rather than one pair each.

## Not run

`make test` (the plan-13-14 gate command), `make lint` (cannot run in this sandbox; the stand-in ran instead), the
PostgreSQL integrity run (nothing here rests on PostgreSQL) and `node --test scripts/captcha-page/*.test.mjs` (the
page did not change).

## New concerns

None found. One observation, not a defect: the new live read adds one paced `getChatMember` to the already rare path of
an undeliverable mute notice (a mute whose notice failed twice or waited out a 5 minute rate limit), so it costs one
unit of the requirement pacer in a case that is already degraded.
