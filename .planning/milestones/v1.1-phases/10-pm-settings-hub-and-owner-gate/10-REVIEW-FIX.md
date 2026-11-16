---
phase: 10-pm-settings-hub-and-owner-gate
fixed_at: 2026-11-04
review_path: .planning/phases/10-pm-settings-hub-and-owner-gate/10-REVIEW.md
iteration: 1
findings_in_scope: 3
fixed: 3
skipped: 0
status: fixed
---

# Phase 10: Code Review Fix Report

Scope was limited to WR-03, WR-01 and IN-03. WR-02 (a Phase 9 decision, deferred) and IN-01, IN-02, IN-04 were left alone on purpose.

| Finding | Action | Commit |
|---------|--------|--------|
| WR-03 | Code fix, tests first. `cache.IsBotGoneError` (replaces `isBotLookupClientError`) accepts only Bot API codes 400 and 403. The admin list observer and `refreshChatAdminIndex` use it, so a 401, 404, 409, 429, 5xx, transport error or timeout leaves `chat_admin_index` rows alone and a 429 keeps its `retry_after`. `isTelegramClientError` (settings doors) is unchanged. AGENTS.md updated. | 0be0f65 |
| WR-01 | Documentation only. Comment on `settingsCreatorGone`, the roster feeder comment and AGENTS.md now state the name-only detection limit, its fail direction (administrators allowed in, the owner never locked out) and the owner's fix, and point at check A1 in `10-SPIKE.md`. No behavior change. | 0bdbfac |
| IN-03 | Comment only. `main.go` comment says `StopAdminIndexSweeper` cancels the context and waits on the WaitGroup. | 420dc69 |

## Tests

- `TestAdminListObserver` (`fuku/utils/cache`): 400 and 403 (also wrapped) are a gone event; 401, 404, 409, 429, 500 and a transport error report nothing.
- `TestAdminIndexKeepsRowsOnFailure`, `TestAdminIndexBotLookupRefused` (new), `TestAdminIndexRefresh` (`fuku/modules`): persisted rows survive a 401, 404 or 409 during an admin-list load and during `refreshChatAdminIndex`; 400 and 403 still clear them; the 429 `retry_after` test is unchanged.

## Verification

Run in the main checkout (no worktree), on branch `claude/ecstatic-goldberg-qsss05`:

- `CGO_ENABLED=0 go build ./...` passes.
- `go vet -tags testtools ./fuku/modules ./fuku/utils/cache` passes.
- `go test -tags testtools -race -count=1 ./fuku/utils/cache` passes.
- `go test -tags testtools -race -count=1 -run '^(TestAdminIndex|TestAdminListObserver|TestSettingsGate|TestSettingsOwnerState)' ./fuku/modules` passes.
- `make lint` could not run (the installed golangci-lint refuses Go 1.26.0). `gofmt -l` is clean on every touched file except `main.go`, which already had an unrelated alignment issue at line 508 before these changes.

---

_Fixed: 2026-11-04_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
