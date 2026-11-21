---
phase: 21-pm-copies
plan: 02
subsystem: event-messages
status: complete
tags: [pm-copies, pacer, job-pool, moderation, i18n]
requires: [22-01]
provides:
  - eventPMSpec, eventPMSpecs, registerEventPM, eventPMSpecFor (seven manual specs)
  - pmCopyPacerOptions, pmCopyPacer, pmCopyCallTimeout, pmCopyPaced (fuku:pmcopy:pace:*)
  - pmCopyJobs, pmCopyMaxInFlight (16), pmCopyJobTimeout (60 s)
  - eventPMOffer, offerEventPM, runEventPM
  - composeEventPM, eventPMDefaultText
  - eventReply.PMNote; replyEventGroup, offerReplyPM; sendSuccess (replySuccess wraps it)
  - nine eventmsg_pm_ locale keys; TestEventMsgLocaleKeys and eventMsgLocaleLimits
affects: [22-03, 22-04, 22-05, 22-06, 22-07, 22-08, 22-09, 22, 23]
tech-stack:
  added: []
  patterns:
    - "an engine split: the unchanged group step returns what it posted, then a non-blocking offer reads the same cached result"
    - "a bounded in-flight job pool on raidPostTracker.startBelow with no drain, its own fleet-wide pacer keys"
    - "a test client wrapper that rewrites only the answers to one chat ID and keeps every request in the site's recorder"
key-files:
  created:
    - fuku/modules/event_message_pm.go
    - fuku/modules/event_message_pm_env_test.go
    - fuku/modules/event_message_pm_test.go
    - fuku/i18n/eventmsg_locale_test.go
  modified:
    - fuku/modules/event_message.go
    - fuku/modules/moderation.go
    - fuku/modules/mute.go
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
  - "The copy is composed in the handler (offerReplyPM) and the job only sends: the composer needs the moderation context's translator, actor and target, and none of it may be touched from a goroutine."
  - "eventPMOffer carries no translator yet: nothing in this plan reads one in the job. Plan 22-03 adds Custom and Fallback (the default text built in the handler), which is what the fallback needs."
  - "The reason line of the default private text is `<b>Reason:</b> {reason}` (uk `<b>Причина:</b> {reason}`), the labelled-line form of the style guide and of the group's own reason line, not the plan's bare `Reason: {reason}` draft."
  - "runEventPM logs a Telegram error by its code through memberReqErrText, the existing IDs-only classifier."
  - "AGENTS.md was edited in the two task commits (same-commit rule), so the plan's Task 3 has no commit of its own."
metrics:
  duration: ~50 min
  completed: 2026-11-19
actuals:
  tokens: 12900
  tasks: 3
  commits: 2
plan_head_before: 9f7efd88eeea7c58484c6cc8ad253d57e6b17875
plan_head_after: 4091fa739f4c4359c5f96f2f3bd5aeff36fbe677
---

# Phase 22 Plan 02: Private copy engine Summary

`replyEvent` now runs Phase 21's group step unchanged (`replyEventGroup`, which returns the message it posted) and then
offers the event's private copy (`offerReplyPM`): with a registered spec and `Resolve(event, pm)` not Silent, the copy is
composed in the handler (`composeEventPM`: the stored text through the group renderer, or the event's default private text
in the group's language, the stored buttons and `{rules}` button only, the stored media) and sent by a bounded job through
its own fleet-wide pacer on `fuku:pmcopy:pace:*`, to the punished member's private chat, after the punishment and the group
reply, with no reply target. All seven manual events have a spec and a default text in 8 languages.

## What later plans use

| Kind | Name |
|------|------|
| Spec table | `eventPMSpec{DefaultKey string; Automatic bool}`, `eventPMSpecs`, `registerEventPM(event, spec)` (panics on an unknown event, an empty key, a duplicate), `eventPMSpecFor(event)`; the seven manual rows in `event_message_pm.go`'s `init`, `Automatic: false` |
| Pacer | `pmCopyPacerOptions` (keys `fuku:pmcopy:pace:next` / `:block`, 200 ms, 1 retry, 15 s), `pmCopyPacer` (a variable), `pmCopyCallTimeout` 10 s, `pmCopyPaced(ctx, call)` |
| Pool | `pmCopyJobs raidPostTracker`, `pmCopyMaxInFlight` 16, `pmCopyJobTimeout` 60 s (a variable); no drain, no flag |
| Offer | `eventPMOffer{Bot, Chat gotgbot.Chat, Event, UserID, Message media.GreetingMessage}`, `offerEventPM(o)` (sheds with one Debug line when 16 run), `runEventPM(ctx, o)` (one paced `media.SendGreetingMessage` to `o.UserID`, Debug outcome log) |
| Composer | `composeEventPM(b, chat, tr, spec, r, row, actor) media.GreetingMessage`, `eventPMDefaultText(tr, spec, chat, values, note) string` |
| Engine split | `eventReply.PMNote`; `(c *moderationCtx) replyEventGroup(r, msgs) (*gotgbot.Message, error)`; `(c *moderationCtx) offerReplyPM(r, msgs)`; `(c *moderationCtx) sendSuccess(text, opts) (*gotgbot.Message, error)` |
| Locale | `eventmsg_pm_ban`, `_tban`, `_kick`, `_mute`, `_tmute`, `_unban`, `_unmute` (each names `<b>{group}</b>`; the timed two `{until}`), `eventmsg_pm_reason` (`{reason}`), `eventmsg_pm_unmute_lockdown` |
| Test toolkit | `newEventPMSite`, `eventPMTestClient`, `withFastPMCopyPacer`, `waitEventPM`, `storeEventPM(t, chat, event, text, on)`, `eventPMCleanup`, `eventPMCalls`, `eventPMOnly`, `eventPMNone`, `eventPMMember` (42), `eventPMDefault` |

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 (tracer) | the ban's private copy end to end: replyEvent split, engine file, ban spec, two locale keys, `TestEventPMEndToEnd`, AGENTS.md | c363f25 |
| 2 | the other six specs, the `/unmute` PMNote, seven locale keys, `TestEventMsgLocaleKeys`, `TestEventPMSpecs`, `TestEventPMDefaultTexts`, `TestEventPMComposition`, AGENTS.md | 4091fa7 |
| 3 | AGENTS.md (PM copies bullet, pacer Data bullet, shutdown sentence, Event messages bullet) | folded into c363f25 and 4091fa7 |

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestEventPMEndToEnd|TestLegacyModerationRequests|TestEventMessageBanEndToEnd|TestCommandRepliesStayInsideTheHandler)$' -v ./fuku/modules`: PASS (task 1 gate; 6 + 17 + 6 subtests, no SKIP).
- `go test -tags testtools -race -count=1 -run '^(TestEventPMSpecs|TestEventPMDefaultTexts|TestEventPMComposition|TestEventPMEndToEnd|TestLegacyModerationRequests|TestEventMessageSites)$' ./fuku/modules`: ok (task 2 gate).
- `go test -tags testtools -race -count=1 -run 'TestEventMessage.*|TestLegacyModerationRequests|TestMessagesEndToEnd|TestSettingsMessages.*|TestBotManual.*|TestEventPM.*|TestCommandRepliesStayInsideTheHandler' ./fuku/modules`: ok (40 s).
- `go test -tags testtools -race -count=1 ./fuku/i18n`: ok (55 s), including `TestEventMsgLocaleKeys`, `TestLocaleStyle`, `TestLocalePlaceholdersMatchOtherLanguages`.
- `go test -tags testtools -race -count=1 ./fuku/db/eventmsg`: ok.
- golangci-lint v2.13.1, `CGO_ENABLED=0`: `run --new-from-rev=9940d97 ./...` 0 issues; with `--build-tags=notesttools` 0 issues; `--default=none --enable dupl` 0 issues.
- `CGO_ENABLED=0 go build ./...` ok; `gofmt -l` empty on every touched Go file; `make check-translations` all present; `make check-docs` no drift.
- Acceptance greps: `fuku:pmcopy:pace:next` once in `event_message_pm.go`; `registerEventPM(eventmsg.` 7; `eventmsg_pm_unmute_lockdown` once in `mute.go`; 9 `eventmsg_pm_` keys in each of the 8 language files; `PM copies (` and `offerReplyPM` in AGENTS.md.
- `git diff --exit-code` of the moderation golden since its own commit, and `git diff --exit-code 9940d97 -- fuku/modules/bans.go fuku/modules/command_replies_guard_test.go main.go go.mod go.sum fuku/modules/settings_deletion_golden_test.go`: no change.
- Mutations (all reverted) each failed the tests that guard them: dropping the `Silent` check of `offerReplyPM` (`off by default`, `no pm row`); the raw title in `eventPMDefaultText` (every default text subtest); `PMNote` not passed by `/unmute` (both lockdown subtests); the bot's own rows added to the copy's keyboard (`custom copy`, the photo composition and the default texts with a reason).

## Deviations from Plan

### Auto-fixed Issues

None in production behaviour.

**1. [Ordering] AGENTS.md in the task commits, no Task 3 commit**
- The repo rule puts an AGENTS.md change in the commit that makes the rule true: the PM copies bullet, the pacer Data bullet and the shutdown sentence went into c363f25 with the engine, and the seven specs, `PMNote` and the tests into 4091fa7. The plan's Task 3 commit subject (`feat(22-02): send a group's private copy after a manual punishment`) was not used.

**2. [Ordering] Two locale keys in Task 1**
- `eventmsg_pm_ban` and `eventmsg_pm_reason` went into the tracer commit so the ban spec it registers names a key that exists in every file; the other seven keys came with Task 2.

**3. [Scope] `eventPMOffer` has no translator field**
- The plan's artifact table lists the group translator on the offer; nothing in the job reads it in this plan, and an unread field is a lint and review smell. Plan 22-03 adds `Custom` and `Fallback`, built in the handler, which is where the translator is.

**4. [Wording] The reason line is labelled**
- `<b>Reason:</b> {reason}` in every language (the label each language's `bans_ban_ban_reason` uses), per the style guide's labelled detail lines; the limits test allows `<b>` and nothing else.

**5. [Test] The toolkit reads the private copy from the site's recorder**
- `eventPMTestClient` rewrites only the answers to chat 42 (a private-chat message) and does not keep its own list or a refusal switch: plan 22-03's `eventPMRefuse` can add the refusal to it. `eventPMCalls` filters sends only, because the handler's own `getChat` of the member is also a request to chat 42.

## Not verified

- The whole `fuku/modules` package was not run (targeted runs only, as instructed); `make test-shard-check` was not run (the new top-level tests are found by `go test -list` like any other).
- No PostgreSQL run: nothing of this plan rests on a constraint.
- Live Telegram: a copy to a person who never started the bot answers 403, which this plan only logs at Debug; the classification is plan 22-03.

## Notes for plan 22-03

- `runEventPM` today logs the outcome with `memberReqErrText` and does nothing else; add `pmCopyClassify` there. A refused custom copy's fallback needs the default text: build it in `offerReplyPM` with `eventPMDefaultText(c.Tr, spec, c.Chat, r.Values, r.PMNote)` and put it on the offer (`Fallback`), and set `Custom` when `row.HasContent()`.
- The target guard (`pmCopySkipTarget`) belongs in `offerEventPM` (or `offerReplyPM` before composing, to save the render); `r.Target.Id` is the member's ID from the site's `getChat`.
- The shed path of `offerEventPM` exists but is untested; a test can fill `pmCopyJobs` with `startBelow` calls (and `finish` them in `t.Cleanup`) to force it.
- `eventPMTestClient` is a value type wrapping `s.bot.BotClient`; give it a pointer field for scripted refusals of sends to chat 42, as `eventMsgFailingClient` does for the group.
- `newEventPMSite` registers `pmCopyJobs.wait` as its last cleanup, so the pacer swap and miniredis outlive every job; keep that order in new helpers.

## Known Stubs

None.

## Threat Flags

None: no surface beyond the plan's threat model. T-22-05 (`TestEventPMEndToEnd` custom copy, `TestEventPMDefaultTexts` escaped title and reason), T-22-06 (method order in `eventPMOnly`), T-22-07 and T-22-08 (`TestEventPMComposition`: anonymous admin, no Unban row) and T-22-09 (`TestCommandRepliesStayInsideTheHandler`, empty allow-list) are mitigated and tested.

## Self-Check: PASSED

- Files: the 4 created files exist; the 12 modified files carry their edits.
- Commits: c363f25 and 4091fa7 are ancestors of HEAD.
