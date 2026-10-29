---
phase: 07-ai-assisted-raid-detection
plan: 08
subsystem: raid-detection
tags: [gemini, telegram-files, image-moderation, secrets, raid-ai-pool]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "gemini client (07-02), GEMINI_API_KEY/GEMINI_MODEL config (07-03), candidate watcher and pool (07-06), raidAIContext (07-14), verdict counter (07-01)"
provides:
  - "raidAIMediaRef: the one frame (photo, sticker, GIF/video/animated-sticker thumbnail) of a message the image check sends"
  - "raidAIDownload: getFile plus a GET that cannot leak the bot token"
  - "raidAIProcessImage: Gemini verdict at 0.85 or more feeds the AI burst counter; blocked and failed counted apart"
  - "RaidAIStats.Blocked and the once-per-process deprecated-model warning"
affects: [07-09 album gate and Gemini breaker, 07-10 and later live checks]

actuals:
  tokens: 15000
  tasks: 3
  commits: 5
plan_head_before: 5dee09e041a4fc567c1cccc4827310cf23500d7e
plan_head_after: b566107933d0582df371623cf615c0bb00cdcd40
commits: 5

tech-stack:
  added: []
  patterns:
    - "Download errors rebuilt from url.Error.Err, status-only errors, no redirects; a last-line scrub of the bot token and Gemini key before anything is logged"
    - "Test wrapper of the lockdown fake whose FileURL points at an httptest file server and keeps the bot<token> segment"

key-files:
  created:
    - alita/modules/raid_ai_media.go
    - alita/modules/raid_ai_image.go
    - alita/modules/raid_ai_media_test.go
  modified:
    - alita/modules/raid_ai.go
    - alita/modules/raid_ai_text_test.go
    - AGENTS.md

key-decisions:
  - "The group 2 watcher was split: raidAIWatch checks chat type and sender once, then sends a message with Text to the unchanged text rules and a message with a media frame to the new image rules; a caption is never judged on the raid path"
  - "Image candidates reuse the candidate script with an empty hash, so only 'is the sender new' is asked and nothing is recorded"
  - "raidAIScrub removes the bot token and the Gemini key from any error text before it is logged, on top of the rebuilt errors"
  - "raidAIBlocked lives in raid_ai_image.go (as the plan's artifact list says) and is read by GetRaidAIStats"

requirements-completed: [RAID-04, PLAT-02, RAID-05]

duration: 45min
completed: 2026-10-29
status: complete
---

# Phase 07 Plan 08: Gemini image checks for the AI burst Summary

**A new member's photo, GIF, sticker or video is fetched off the update path with a download that cannot leak the bot token, judged by Gemini, and counted toward the AI burst at a spam probability of 0.85 or more; nothing leaves the server without GEMINI_API_KEY.**

## Performance

- **Duration:** about 45 min
- **Tasks:** 3 of 3
- **Files:** 3 created, 3 modified

## Accomplishments

- Image route in the group 2 watcher: only a new member's media, only when `GEMINI_API_KEY` is set, after Redis, the chat's `ai_enabled` and `aispamCarvedOut`; the job joins the shared pool (D-18) and carries no caption, name or text.
- `raidAIMediaRef` picks the photo size nearest 1024 px, a regular static sticker's WebP, or Telegram's thumbnail for a GIF (animation checked before the document), video, animated or video sticker; custom emoji, masks, video notes, documents, voice notes, missing thumbnails and declared sizes over 5 MiB give nothing.
- `raidAIDownload` refuses empty and absolute file paths, redirects, bad statuses, bodies over 5 MiB and anything `http.DetectContentType` does not call JPEG, PNG or WebP; every error is rebuilt, so neither the token nor the URL is in it.
- `raidAIProcessImage`: ErrBlocked counts `RaidAIStats.Blocked`, any other error counts failed, a verdict counts checked and then kept or spam (`raidAIImageBar = 0.85`, inclusive); a shutdown-cut check is shed, not failed. A LEGACY, DEPRECATED or RETIRED model stage logs one warning per process.
- AGENTS.md: image-check bullet, the download rule and the gemini package rule (stdlib only, key only in `x-goog-api-key`).

## Task Commits

1. Task 1 RED `5eb1938` test(07-08): show new members' spam photos are never judged
2. Task 1 GREEN `bc4805e` feat(07-08): judge new members' photos with Gemini for the AI burst
3. Task 2 RED `705ddc2` test(07-08): show stickers, GIFs and videos are not judged
4. Task 2 GREEN `21ce367` feat(07-08): pick the frame Gemini judges for every media kind
5. Task 3 `b566107` test(07-08): prove the image checks keep the Gemini key and the bot token out of logs

## Verification

- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run '^(TestRaidAISecretsNeverLogged|TestRaidAIMedia)$' -v ./alita/modules`: PASS (all subtests).
- `CGO_ENABLED=1 go test -tags testtools -race -count=1 -run 'AISpam|Raid|Lockdown|Backup' ./alita/modules`: ok (151 s).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./alita/modules ./alita/utils/gemini`, `gofmt -l alita` clean, `git diff --exit-code go.mod go.sum` clean.
- `make lint` was not run: golangci-lint is built with Go 1.25 against the module's Go 1.26, so `go vet` and `gofmt -l` stood in for it. The orchestrator runs the full `make test` after the wave.
- Mutation check: wrapping the raw request error instead of `ue.Err` in `raidAIDownload` made TestRaidAISecretsNeverLogged fail on the returned error and on the log; reverted.
- Acceptance greps: `raidAIImageBar = 0.85` 1, `http.ErrUseLastResponse` 1, `http.DetectContentType` 1, `raidAIProcessImage(` in raid_ai.go 1, `x-goog-api-key` and `alita/utils/gemini` in AGENTS.md present, `ue.Err` in raid_ai_media.go 2, `func TestRaidAISecretsNeverLogged(` 1.

## Deviations from Plan

None - plan executed exactly as written. Two small notes:

- TDD RED for Task 1 was a compile failure (symbols not yet defined), and Task 3's test passed against the Task 1 and 2 code, so it needed no production change; its commit is `test(...)` as the plan allows.
- `resetRaidAIStats` in `raid_ai_text_test.go` also zeroes the new Blocked counter (one line).
- The per-plan commit ledger and spawn sentinel files under the worktree git directory could not be written (the sandbox refuses git-directory paths), so `commits:` is measured with `git rev-list --count 5dee09e..HEAD` against the base the orchestrator named.

## Known Stubs

None.

## Threat Flags

None. The mitigations T-07-28 to T-07-32 are implemented and tested; no new network surface beyond the planned Gemini and Telegram file calls.

## Pending human verification

- A live run with a real `GEMINI_API_KEY`: a spam picture from a fresh account is judged and, in a burst of 5 items from 3 accounts, locks the group; 0.85 may need tuning (07-SPIKE.md names it first).
- Confirm against the real Bot API that `getFile` for a GIF/video thumbnail and for a static sticker returns a downloadable JPEG/WebP path.
- With a local Bot API server (absolute `file_path`) confirm the refusal behaves as intended for the deployment in use.

## Self-Check: PASSED

- Created files exist: raid_ai_media.go, raid_ai_image.go, raid_ai_media_test.go.
- Commits 5eb1938, bc4805e, 705ddc2, 21ce367, b566107 are on the branch.
