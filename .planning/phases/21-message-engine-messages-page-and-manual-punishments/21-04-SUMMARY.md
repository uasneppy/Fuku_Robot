---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 04
subsystem: moderation
status: complete
tags: [event-messages, tracer, ban, MSG-01, MSG-02, MSG-03, MSG-04, MSG-09, EVT-01]
requires: [21-01, 21-02]
provides:
  - fuku/modules/event_message.go (the event message engine)
  - banDefaultReply and the /ban, /dban reply through replyEvent
  - eventmsg_admin_anon locale key (8 files)
  - test toolkit event_message_env_test.go
affects: [21-05, 21-06, 21-07, 21-08, 21-11, 21, 22, 23, 24]
tech-stack:
  added: []
  patterns: ["sentinel pass before FormattingReplacer, escaped values after", "default built at the site, handed to one engine", "definite-refusal fallback to the default"]
key-files:
  created:
    - fuku/modules/event_message.go
    - fuku/modules/event_message_env_test.go
    - fuku/modules/event_message_test.go
  modified:
    - fuku/modules/moderation.go
    - fuku/modules/bans.go
    - locales/en.yml
    - locales/es.yml
    - locales/fr.yml
    - locales/hi.yml
    - locales/id.yml
    - locales/pt.yml
    - locales/ru.yml
    - locales/uk.yml
    - AGENTS.md
decisions:
  - "The read error Warn carries no error text: only the chat ID and the event, as every engine log line."
  - "composeEventMessage copies the bot rows into a fresh slice before appending the custom rows, so the default's options reused by the refusal fallback are never changed."
  - "A row whose file ID is empty or whose media type is 0 is sent as text (db.TEXT)."
  - "eventDefaultReply gets a compile-time production use (var _ eventDefaultReply = banDefaultReply) rather than a nolint."
metrics:
  duration: ~10 min
  completed: 2026-11-19
actuals:
  tokens: 13900
  tasks: 3
  commits: 3
plan_head_before: 4ee4fbf523b2771eb313a1e309cf97cd587a4c43
plan_head_after: abb4fd250110862444ca081e37d9bf36082a75f3
---

# Phase 21 Plan 04: Event message engine and the ban event Summary

A group's stored ban message is what the next `/ban` or `/dban` posts there: read after the ban,
filled with the Welcome and event placeholders (escaped), sent as a reply with the Unban row first,
queued under `bot:punish_manual`, replaced by the default on a definite refusal, and nothing at all
when Silent; a group with nothing stored sends the golden reply byte for byte.

## What later plans use (names and signatures)

| Kind | Name |
|------|------|
| Seam | `var eventMessagesLookup = eventmsg.Get` (`func(context.Context, int64) (*eventmsg.Messages, error)`), `var eventMessageReadTimeout = 2 * time.Second` |
| Types | `eventValues{Reason, Duration string; Until time.Time}`, `eventActor{User *gotgbot.User; Anonymous bool}`, `eventDefaultInput{Target gotgbot.User; Reason, TimeVal string}`, `type eventDefaultReply func(tr *i18n.Translator, in eventDefaultInput) (string, *gotgbot.SendMessageOpts)`, `eventReply{Event string; Target gotgbot.User; Values eventValues; Default string; Opts *gotgbot.SendMessageOpts; Note string}` |
| Renderer | `eventTokenNames` (`reason admin duration until action count limit window`), `eventTokenSentinel(name string) string`, `eventPlaceholderValues(tr *i18n.Translator, chatID int64, actor eventActor, v eventValues) map[string]string`, `renderEventText(b *gotgbot.Bot, chat *gotgbot.Chat, target *gotgbot.User, stored string, buttons []db.Button, values map[string]string) (string, []db.Button)`, `eventAdminName(tr, actor) string` |
| Composer, send | `eventBotRows(opts *gotgbot.SendMessageOpts) [][]gotgbot.InlineKeyboardButton`, `composeEventMessage(b, chat, tr, r eventReply, row eventmsg.Message, actor eventActor) media.GreetingMessage`, `eventMessageRefused(err error) bool`, `(c *moderationCtx) replyEvent(r eventReply) error`, `eventTargetOf(info *gotgbot.ChatFullInfo) gotgbot.User` |
| Anonymous mark | `const eventAnonActorKey`, `markAnonymousActor(ctx *ext.Context)`, `actorIsAnonymous(ctx *ext.Context) bool` |
| Default builders | `banDefaultReply(tr, in eventDefaultInput)` in `bans.go`; `moderationReasonLine(tr *i18n.Translator, key, reason string) string` and `target.untilDate int64` (set by `extractTemporalTarget`) in `moderation.go` |
| Locale | `eventmsg_admin_anon` (en `an admin`, uk `адмін`) |
| Test toolkit | `eventMsgCleanup(t, chatIDs...)`, `eventMsgStore(t, chatID, event, text)`, `eventMsgStoreRow(t, chatID, event, text, fileID, mediaType, buttons)`, `eventMsgFailingClient` with `eventMsgRefuseOnce(s *botSite, method string, err error)`, `eventMsgMethods(client)`, `eventMsgIndex(methods, method)`; in the test file `eventMsgWantGolden`, `eventMsgWantCustom`, `eventMsgAnonCtx` |

Plan 21-05 routes the six other events: build the site's default (reuse the `eventDefaultReply` shape),
then `c.replyEvent(eventReply{Event: ..., Target: eventTargetOf(info), Values: eventValues{Reason, Duration: t.timeVal, Until: time.Unix(t.untilDate, 0)}, Default: text, Opts: opts, Note: note})`.
For `/unmute` the default already contains the lockdown note; pass the note in `Note` too, so a custom
text gets it. The proof tap must call `markAnonymousActor(ctx)` on the re-entered update.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 (tracer) | engine, ban send site, tracer test | 3b0cfcb |
| 2 | media, buttons, rules button, refusal fallback, anonymous admin, locale key, five tests | 37a8fd1 |
| 3 | AGENTS.md | abb4fd2 |

Tracer gate: the tracer's automated verify was re-run end to end after Task 1 and passed before
Task 2 started.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestEventMessageBanEndToEnd|TestLegacyModerationRequests|TestBotManualPunishmentReplies)$' -v ./fuku/modules`: PASS (6 + 17 + 42 subtests).
- `go test -tags testtools -race -count=1 -run '^(TestRenderEventText|TestEventMessageCompose|TestEventMessageFallback|TestEventMessageAnonymousAdmin|TestEventMessageLogsCarryIDsOnly|TestEventMessageBanEndToEnd|TestLegacyModerationRequests)$' -v ./fuku/modules`: PASS.
- `go test -tags testtools -race -count=1 -run '^(TestLocaleStyle|TestLocalePlaceholdersMatchOtherLanguages)$' ./fuku/i18n`: ok. `make check-translations`: all present.
- `TestLegacyModerationRequests` passes unedited; `git diff --exit-code 31e7e18 -- fuku/modules/moderation_legacy_request_test.go` exits 0. `git diff --exit-code c491b48 -- go.mod go.sum fuku/modules/mute.go` exits 0.
- Acceptance greps: `replyEvent(` 1, `banDefaultReply(` 1, `replyEvent(eventReply{` in bans.go 1, `c.replySuccess(` in bans.go 4, the five test functions 1 each, `eventmsg_admin_anon` 1 per language file, `eventMessageRefused` 1, `media.ErrNoPermission` 1; AGENTS.md `Event messages` 1, `eventMessagesLookup` 1, `replyEvent` 3.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/modules` clean; `gofmt -l` clean on every touched Go file.
- `make lint` could not run: the installed golangci-lint was built with Go 1.25 and refuses the module's Go 1.26.0 target. `go vet` and `gofmt` were run instead.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The fallback's options must not be changed by the custom keyboard**
- **Found during:** Task 2
- **Issue:** appending the custom rows to `eventBotRows(r.Opts)` could write into the default options' keyboard slice, which the refusal fallback then sends.
- **Fix:** the rows are copied into a fresh slice before the custom rows are appended.
- **Files modified:** fuku/modules/event_message.go
- **Commit:** 37a8fd1

**2. [Rule 2 - Test strength] the toolkit gained `eventMsgStoreRow`, `eventMsgMethods` and `eventMsgIndex`**
- To store media and buttons in one call and to assert the ban is called before the send.

Nothing else: plan executed as written.

## nolint:unused added

- `markAnonymousActor` in `fuku/modules/event_message.go`: `//nolint:unused // first production caller arrives in plan 21-05 (the anonymous admin's proof tap)`. Plan 21-05 removes it when the proof tap calls it.

## Notes

- Each of the six new top-level tests installs the real locales once (about 1.5 s under `-race`), so the file adds about 9 s to the package run; every subtest is a few milliseconds.
- `eventmsg_admin_anon` has no limits-table row: only `settings_` and `mreq_` keys have limits tables.

## Known Stubs

None.

## Threat Flags

None: no new endpoint or trust boundary beyond the plan's threat model (T-21-09 to T-21-13 mitigated as planned).

## Self-Check: PASSED

- FOUND: fuku/modules/event_message.go, fuku/modules/event_message_env_test.go, fuku/modules/event_message_test.go
- FOUND: 3b0cfcb, 37a8fd1, abb4fd2 (ancestors of HEAD)
