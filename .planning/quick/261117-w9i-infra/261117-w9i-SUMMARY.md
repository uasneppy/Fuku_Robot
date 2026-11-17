# 261117-w9i: Wave 9 summary (deletion and infrastructure)

Branch `wave9-infra`, base `79a7e52`. I11 and I12 were done before this wave and were skipped.

## Fixed

| Item | Commit | Fix | Test (failed before the fix) |
|------|--------|-----|------------------------------|
| I1 | `2f8a478` | `fuku/i18n/loader.go` builds embed paths with `path.Join` (and `path.Ext`), not `filepath.Join` | `TestLocaleLoaderUsesSlashPaths` (go/parser guard: no `filepath.Join` in the package; a Windows build cannot run here) |
| I2 | `90bc5b1` | a pin drops every pending row of the pinned message, whatever its kind (`deletion.DropPendingMessageAnyKind`); the fast path is gated on any stored delete rule (`memberDeleteRuleStored`). AGENTS.md D-11 text updated | `TestDropPendingMessageAnyKind`, `TestMemberDeletePinnedAnyKind` |
| I3 | `871f376` | the tracing processor detects nesting with its own marker (`processingDataKey`), so a webhook update (which pre-fills `ContextDataKey`) is counted and gets its per-update memo; the webhook's context is kept | `TestTracingProcessorProcessUpdateCountsTopLevelUpdatesOnly` (webhook case added) |
| I4 | `055a002` | `interpolateParams` fills `{name}` and the legacy verbs in one pass over the raw template, verbs counted on the template only | `TestInterpolateParamsIgnoresVerbsInValues`; `TestRawPlaceholderKeysRender` and every `TestLegacy*Requests` golden still pass |
| I5 | `d0b74fd` | the runner no longer returns early when the row count equals the file count: every checksum is verified on every run, a recorded version with no file is warned about; `scripts/migrate_psql.sh` reports the same orphan rows (it already verified every file). AGENTS.md Migrations updated | `TestRunMigrationFilesAlwaysVerifies` (SQLite: a modified file is refused; an orphan row no longer hides a pending file) |
| I6 | `5bc52fa` | `Wrap` and `Wrapf` share `wrapAt` with the right caller skip | `TestWrapfReportsCaller` |
| I7 | `91c775b` | `/debug/pprof/` is behind `requireMetricsAuth` (warns when no token is set) | `TestPPROFRequiresToken` |
| I8 | `2b893f3` | `/health` sends `PING` (no shared key); with `DISABLE_CACHE=true` a missing Redis is not unhealthy | `TestCheckRedisPings`, `TestHealthWithCacheDisabledAndNoRedis` |
| I10 | `810521d` | `StringTable` reads each language from its own locale with no English fallback; a missing or empty key, or a language with no locale, is an error. `trS` comment corrected | `TestStringTable` (fallback subtests now expect errors) |

## Not reproduced

None.

## Owner decisions

- **I9** cross-replica stale cache write: not implemented. A Redis-side generation with a Lua compare-and-set does not fit
  cleanly: the cache write goes through gocache's marshaler and store (a memory store in tests), so the script would bypass
  that layer, duplicate its encoding and add a round trip to every load and every `DeleteCache`. Recorded as an accepted
  limit in AGENTS.md (Data section), commit `d184a57`.

## Existing tests changed

- `TestTracingProcessorProcessUpdateCountsTopLevelUpdatesOnly`: the nested case now relies on the dedicated marker (a
  pre-filled `ContextDataKey` alone used to count as nested); a webhook case was added.
- `TestCheckRedisWithNilAndHealthyManagers` replaced by `TestCheckRedisPings` (it pinned the shared-key probe through a
  memory manager); the unused memory store fixture of `server_test.go` was removed.
- `TestStringTable`: the "English fallback" and "unknown language" subtests pinned the fallback I10 removes; they now
  expect an error ("missing in another language", "empty in another language", "unknown language").
- `TestMemberDeletePinnedNeedsRule`, `TestMemberSelfDestructPinRace`: the `memberDeleteDropPinned` seam lost its kind
  argument (closure signatures only, assertions unchanged).

No `TestLegacy*Requests` golden was touched.

## User-visible behaviour changes

- A pinned bot message (welcome, goodbye, rules, a punishment reply) whose kind has a delete timer is no longer deleted
  when its time comes; a pin in a chat with any delete rule drops the message's pending row of any kind.
- Webhook mode: `fuku_updates_processed` and the update duration metric now count updates, and the `*ForUpdate`
  predicates are memoised per update as in polling mode.
- A translated text whose filled-in value contains `%s`, `%d` or `{name}` is printed as typed, and the template's own
  verbs are still filled.
- Startup with `AUTO_MIGRATE=true` now fails on a modified applied migration even when nothing is pending, and applies
  a pending file that a recorded row with no file used to hide; such an orphan row is logged as a warning.
- Error locations from `errors.Wrapf` name the real caller.
- With `METRICS_AUTH_TOKEN` set, `/debug/pprof/` needs it as a bearer token.
- `/health` no longer writes a key; with `DISABLE_CACHE=true` and no Redis it answers 200 (the `redis` check still reads
  false).
- Startup with the web captcha configured fails if any page language lacks a page key, instead of showing English.

## Checks

- Per item: the listed tests with `-tags testtools -race`, `CGO_ENABLED=0 go build ./...`.
- `make check-translations`: all present. `golangci-lint run ./...`: 0 issues.
- Not run here (by instruction): `make test`, the whole `fuku/modules` package, `make test-postgres-integrity`.
  `scripts/migrate_psql.sh` was checked with `bash -n` only (no PostgreSQL here).

## Left unfinished

Nothing.
