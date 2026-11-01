---
quick_id: 261101-hlf
status: complete
---

# Summary: Ukrainian translation tool (plan Tasks 1-9)

Implemented `scripts/translate_uk` (locale IO, validators, glossary, progress store, project core, CLI, loopback
server, editor page, README) and generated the draft `locales/uk.yml`, inline with TDD, on branch
`claude/charming-babbage-356bx3` (PR uasneppy/Fuku_Robot#4). Task 10 (registering `uk`) is gated on the owner
finishing the translation and was not started.

Deviations, each ruled in the SDD ledger: `KeyState.Tokens`, `Glossary.FlagRussian` (the plan's method name clashed
with the field), `Project.Preview` and `POST /api/check` (live problems without duplicating the validators in JS),
`Progress.Has`, and pseudo-tag names (`<userhandle>`) are translatable with only their count compared.

Verification: `go test -tags testtools -race ./scripts/translate_uk/` green; headless-Chromium runs against the real
1,418 keys; `make check-translations` lists `uk`. Full `make test` exits non-zero only on `go: no such tool "covdata"`
for `alita/utils/actionlog` in this container; it reproduces on a clean `origin/main`.

A fresh whole-branch reviewer found a Critical bug (a leading newline was lost on every write, 11 real keys and the
first draft) plus three Important issues and a spec deviation (same-origin Origin check); all fixed test-first and the
draft regenerated. Six Minor findings are deferred.
