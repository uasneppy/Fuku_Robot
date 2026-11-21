---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 02
subsystem: database
status: complete
tags: [migration, repository, cache, skipLocal, backup]
requires: []
provides:
  - chat_event_messages table (migration 20261120110000)
  - models.ChatEventMessage
  - fuku/db/eventmsg (events table, Get, GetFresh, SetText, SetMedia, SetButtons, SetSilent, Messages, Message, CacheKey)
  - event_messages:<chat> in skipLocal
affects: [21-04, 21-05, 21-06, 21-07, 21-08, 21-11, 21, 22, 23, 24, 29]
tech-stack:
  added: []
  patterns: ["one cached struct per chat (deletion.Rules style)", "column-targeted upsert per part (timezone.Set style)", "Go-side key validation, no CHECK"]
key-files:
  created:
    - migrations/20261119120000_add_chat_event_messages.sql
    - fuku/db/models/event_message.go
    - fuku/db/migrations/event_messages_migration_test.go
    - fuku/db/eventmsg/events.go
    - fuku/db/eventmsg/repository.go
    - fuku/db/eventmsg/repository_test.go
    - fuku/db/eventmsg/testmain_test.go
    - fuku/db/backup/event_messages_backup_test.go
  modified:
    - fuku/db/cache/local.go
    - fuku/db/cache/local_test.go
    - fuku/db/backup/testmain_test.go
    - fuku/modules/test_harness_test.go
    - Makefile
    - AGENTS.md
decisions:
  - "Each Set* writer drops event_messages:<chat> itself after a shared validating upsert helper, so the key drop is visible per writer"
  - "Get normalises a nil set or nil map to an empty non-nil map, so a cache round trip never hands a caller a nil map"
  - "Rows are kept when every part is removed; an empty row reads as the default (Message.HasContent false, Silent false)"
metrics:
  duration: ~25 min
  completed: 2026-11-18
actuals:
  tokens: 12700
  tasks: 3
  commits: 4
plan_head_before: d90bbf17e313f0692e1330e4942c000fb11d7c0c
plan_head_after: 4c1e8bf
---

# Phase 21 Plan 02: Event message storage Summary

A group's custom event messages are stored in `chat_event_messages` (one row per chat, event and destination), read as one cached `eventmsg.Messages` per chat under `event_messages:<chat>` (in `skipLocal`), written by four column-targeted upserts that drop the key, with the event and destination checked in Go and the table kept out of every backup path.

## What later plans use

| Kind | Name |
|------|------|
| Table | `chat_event_messages`, PK `pk_chat_event_messages (chat_id, event, dest)`; columns `text`, `file_id`, `media_type`, `buttons` (jsonb `'[]'`), `silent`, `updated_by`, `updated_at` |
| Model | `models.ChatEventMessage` (`ChatID`, `Event`, `Dest`, `Text`, `FileID`, `MediaType`, `Buttons models.ButtonArray`, `Silent`, `UpdatedBy`, `UpdatedAt`) |
| Events | `eventmsg.EventBan` `ban`, `EventTimedBan` `tban`, `EventKick` `kick`, `EventMute` `mute`, `EventTimedMute` `tmute`, `EventUnban` `unban`, `EventUnmute` `unmute`; `eventmsg.DestGroup` `group`; `Events`, `Dests`, `ValidEvent`, `ValidDest`, `ErrUnknownEvent`, `ErrUnknownDest` |
| Read | `eventmsg.Get(ctx, chatID) (*Messages, error)` (cached), `eventmsg.GetFresh(ctx, chatID)` (no cache); `(*Messages).Lookup(event, dest) (Message, bool)` (nil-safe); `Message.HasContent()` |
| Write | `SetText(ctx, chatID, event, dest, text, by)`, `SetMedia(ctx, chatID, event, dest, fileID, mediaType, by)`, `SetButtons(ctx, chatID, event, dest, []models.Button, by)`, `SetSilent(ctx, chatID, event, dest, silent, by)` |
| Cache key | `eventmsg.CacheKey(chatID)` = `cache.CacheKey("event_messages", chatID)`; prefix `eventMessagesPrefix` in `skipLocal` |

`Message.Buttons` is `[]models.Button` (converted from the model's `ButtonArray`).

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | migration, model, migration tests | 8c3d8aa |
| 2 | eventmsg repository, event table, skipLocal | 429f99d |
| 3 | backup exclusion proof, harnesses, Makefile, AGENTS.md | 4c1e8bf |

The measured `commits: 4` (rev-list from `plan_head_before` to `plan_head_after`) includes one commit of plan 21-03 (`f1c0b27`), which ran in the same working tree at the same time; this plan made three task commits plus this summary.

## Verification

- `go test -tags testtools -race -count=1 -run '^(TestChatEventMessagesMigrationStatements|TestChatEventMessagesMigrationDefaults|TestMigrationRunner.*)$' -v ./fuku/db/migrations`: Statements PASS, Defaults SKIP (PostgreSQL only, the allowed skip); whole package ok.
- `go test -tags testtools -race -count=1 -run '^(TestEventMessages|TestEventMessagesUpsert)$' -v ./fuku/db/eventmsg`: PASS (6 subtests, including one that proves with an in-memory cache marshaler that each of the four writers drops the cached read).
- `go test -tags testtools -race -count=1 ./fuku/db/cache`: ok.
- `go test -tags testtools -race -count=1 -run '^TestEventMessagesStayOutOfBackup$' -v ./fuku/db/backup`: PASS.
- `CGO_ENABLED=0 go build ./...` ok; `go vet -tags testtools ./fuku/db/eventmsg ./fuku/db/backup ./fuku/modules` ok; one `fuku/modules` test run to confirm the harness migrates the new model.
- `git diff --exit-code c491b48 -- go.mod go.sum fuku/db/backup/backup.go fuku/db/backup/types.go` exits 0.
- `gofmt -l` clean on every touched Go file.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Test strength] Cache drop proved against a live cache**
- **Found during:** Task 2
- **Issue:** the eventmsg package tests run with no cache marshaler, so "a value cached before a write is gone after it" would pass trivially.
- **Fix:** the subtests install `utilscache.SetupTestMemoryMarshaler(t)`, and a dedicated subtest shows a change made behind the repository's back is hidden by the cached read until each of the four writers drops the key.
- **Commit:** 429f99d

### Not run

- `make lint` could not run: the installed golangci-lint was built with Go 1.25 and refuses the module's Go 1.26.0 target ("can't load config"). `go vet` and `gofmt` were run instead.

## Known Stubs

None.

## Threat Flags

None: no new endpoint or trust boundary beyond the plan's threat model (T-21-02 to T-21-05 mitigated as planned).

## Self-Check: PASSED

- Files: all 8 created files exist; the 6 modified files carry their edits.
- Commits: 8c3d8aa, 429f99d, 4c1e8bf are ancestors of HEAD.
