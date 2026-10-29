---
phase: 07-ai-assisted-raid-detection
plan: 13
subsystem: testing
tags: [raid-ai, integration-test, fail-open, gemini, typesafe, httptest, miniredis]
status: complete

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "counter, watcher, pool, image path, budgets and breakers, settings, /autolockdown ai, StopRaidAI (plans 07-01 to 07-12, 07-14)"
provides:
  - "TestRaidAIImageBurst: spam photos from three new accounts lock a group with ai_burst end to end through the real dispatcher"
  - "TestRaidAINeverActs: the raid AI path makes only Telegram reads and never locks below the rule"
  - "TestRaidAIFailOpen: with TypeSafe and Gemini down in seven ways, updates return within 500 ms, no verdict counts and the join surge and flood triggers still lock"
  - "The green phase gate and the UAT human-check list"
affects: [phase-07-verification, 07-UAT]

actuals:
  tokens: 4900
  tasks: 2
  commits: 2
plan_head_before: 6c1ee23abb1fb0848a594eb8fe7b8567a4238f81
plan_head_after: 78fd1388352939e742b278c6c51dfd9bb72505d9

tech-stack:
  added: []
  patterns:
    - "Failure modes as a table of providers applied to a fresh env per subtest, with an idempotent release registered on the env's cleanup so held requests end before the servers close"
    - "A Telegram-write proof that records the fake's call log after the setup and requires every later method to start with get"

key-files:
  created:
    - alita/modules/raid_ai_integration_test.go
  modified: []

key-decisions:
  - "No production code changed: no proof found a defect in the plans before it"
  - "TestRaidAINeverActs asserts that every Telegram method called after the setup starts with get, which is stronger than the plan's list of forbidden writes"
  - "The photo cases use a Gemini fake that judges by the downloaded file name, so spam and kept photos can be mixed in one burst"

requirements-completed: [RAID-03, RAID-04, RAID-05, PLAT-02]

coverage:
  - id: D1
    description: "Five spam photos from three new accounts lock the group with ai_burst, and the public notice, Staff Group alert, sample, Ban button and /lockdownstatus name the AI trigger"
    requirement: "RAID-03"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -run '^TestRaidAIImageBurst$' ./alita/modules"
        status: pass
  - id: D2
    description: "The raid AI path never writes to Telegram and one verdict, four verdicts from three accounts and five from two never lock, over text, photo and mixed paths"
    requirement: "RAID-05"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -run '^TestRaidAINeverActs$' ./alita/modules"
        status: pass
  - id: D3
    description: "With the AI providers hung, failing, rate limited, unreadable, unconfigured, paused or shed, every update returns within 500 ms and the join surge and flood triggers still lock"
    requirement: "RAID-05"
    verification:
      - kind: integration
        ref: "go test -tags testtools -race -run '^TestRaidAIFailOpen$' ./alita/modules"
        status: pass
  - id: D4
    description: "The full phase gate: make test, check-translations, check-docs, build, vet, gofmt, go.mod unchanged"
    requirement: "PLAT-02"
    verification:
      - kind: other
        ref: "make test (exit=0, 63 ok packages); make check-translations; make check-docs; CGO_ENABLED=0 go build ./...; go vet -tags testtools ./...; gofmt -l alita main.go scripts"
        status: pass
---

# Phase 7 Plan 13: Prove the AI raid detection end to end Summary

**Three integration tests through the real dispatcher show spam photos from new accounts lock a group with the AI trigger, the AI path only ever reads from Telegram, and a hung, failing or unconfigured provider changes nothing about the join surge and flood triggers.**

## Accomplishments

- `TestRaidAIImageBurst`: three seeded new members (users rows with markup in their names) post 2, 2 and 1 photos to a Gemini fake answering 0.92. After four photos nothing is locked. After the fifth there is one active lockdown, kind `ai_burst`, count 5, window 120, percent 0, started by the bot, with one public notice (`lockdown_auto_started_ai_burst 5 120`). After one worker cycle the Staff Group alert carries `lockdown_alert_trigger_ai_burst 5 120`, `lockdown_alert_counts_ai`, the three escaped names and IDs, three sample lines and a Ban button counting 3. `/lockdownstatus` answers `lockdown_status_active_auto` and `lockdown_trigger_ai_burst`, and only the one lock call was made.
- `TestRaidAINeverActs`: twelve subtests (text, photo and mixed paths against keep, one spam, four spam from three accounts, five spam from two accounts) in a group without `/aispam`. Each proves the provider was asked once per message, the verdict items and accounts match, no lockdown row exists, and every Telegram method called after the setup starts with `get` (observed: `getChat`, `getChatAdministrators`, `getChatMember`, plus the image path's `getFile`). The plan's named write methods are also checked through `wantNoActions`.
- `TestRaidAIFailOpen`: seven modes (hung, 500, 429 with a 30 s retry delay, unreadable JSON, keys missing, breakers open after real failures, queue full behind gated workers), each with a join-surge subtest and a flood subtest in its own env. Every new member's link text and photo, and each of the 30 flood items, returns from `ProcessUpdate` within 500 ms; no `:av` item is ever written, also not after the held requests are released; 15 self-joins lock with `join_surge` and 30 items from four recent joiners lock with `flood`. The modes that must make no provider request (keys missing, breakers open, queue full) assert zero new requests to both servers; the others assert a provider was asked, so the case proves something.

## Task Commits

1. Task 1: `e81f26c` test(07-13): prove an image burst locks a group end to end and the AI path never acts
2. Task 2: `78fd138` test(07-13): prove the AI path fails open and close the phase gate

## Files Created/Modified

- `alita/modules/raid_ai_integration_test.go` (created): the three tests and their helpers (`aiPhoto`, `serveGeminiByFileName`, `callsSince`, `failOpenModes`, `dispatchWithin`, `probeFailOpen`). No production file changed, `go.mod` and `go.sum` unchanged.

## Phase Gate

| Check | Result |
|-------|--------|
| `make test` | `exit=0`, 63 `ok` packages, no FAIL line. The skip gate accepted two skips that were already there (`TestUpdateChat`, `TestRaidDetectionCarryOver`, PostgreSQL-only) |
| `make check-translations` | all translations present |
| `make check-docs` | no drift |
| `CGO_ENABLED=0 go build ./...` | pass |
| `go vet -tags testtools ./...` | pass |
| `gofmt -l alita main.go scripts` | prints nothing |
| `git diff 6c1ee23 -- go.mod go.sum` | empty |
| `make lint` | not run: golangci-lint is built with Go 1.25 and cannot lint the module's Go 1.26. `go vet` and `gofmt` stand in |
| PostgreSQL (migration 20261029120000 chain, `make test-postgres-integrity`: `TestRaidDetectionSettings`, `TestRaidDetectionCarryOver`) | not run here by rule; runs in CI |

## Decisions Made

- The never-acts proof uses "every call after setup is a `get...` read" instead of only the plan's list of nine write methods, so a write method added later is caught without editing the list.
- The photo cases need spam and kept photos in one run, so the Gemini fake in `serveGeminiByFileName` decodes the inline image and judges by the file name the file server embedded in the bytes. It replaces the env's Gemini server through the production seam `raidAIGeminiEndpoint` and restores it at cleanup.
- Fail-open modes release what they hold from a cleanup registered on the env after its servers, so cleanups run in the order that lets held requests end before the servers close.
- Join surge and flood are separate subtests, each with its own fresh env, instead of one env carrying both locks.

## Deviations from Plan

None - plan executed as written. No proof found a defect, so no `fix(07-13)` commit exists.

The mutation check was made once on `keys missing` (leaving the keys configured): the test failed with provider requests, counted verdicts and an `ai_burst` lockdown, then was reverted, so the zero-request assertions have teeth.

## Known Stubs

None.

## Threat Flags

None. Every provider in the tests is an `httptest` server set through the production seams, no key is real, and no test skips on a missing key (T-07-43, T-07-44, T-07-45 mitigated).

## UAT human-check list

1. With `GEMINI_API_KEY` and `TYPESAFE_API_KEY` set, have three or more accounts that joined in the last 24 hours post five spam photos or link texts within two minutes in a test supergroup: one lockdown, one notice and one alert naming "AI spam" with the count, and Ban recent joiners lists them.
2. One account posting ten spam messages locks nothing.
3. With no `GEMINI_API_KEY`, `/autolockdown` shows the AI line, with no TypeSafe key either it shows "AI checks are not set up", and no image leaves the server (no outbound call to `generativelanguage.googleapis.com`).
4. `/autolockdown ai 8` and `ai off` by an owner work, are refused for a plain member, and an anonymous admin gets the proof first.
5. A native-speaker read of the new lockdown and autolockdown strings in es, fr, hi, id, pt and ru.
6. The owner runs the spike kit of `07-SPIKE.md` with a paid-tier key and decides the 0.85 bar, the thumbnails and the model.
7. CI: the migration chain applies 20261029120000 on PostgreSQL and `make test-postgres-integrity` passes `TestRaidDetectionSettings` and `TestRaidDetectionCarryOver`.
8. Live join surge and live message flood in a test supergroup still lock the group with the providers unreachable.

## Self-Check: PASSED

- `alita/modules/raid_ai_integration_test.go`: found
- Commits `e81f26c` and `78fd138`: found
