---
phase: 20-message-engine-messages-page-and-manual-punishments
plan: 03
subsystem: settings menu / message editor
status: complete
tags: [settings, message-editor, typed-reply-door, hooks, MSG-06]
requires: []
provides:
  - settingsEditEvent kind and settingsEditHooks (Load, Remove, Write, Draw, Preview, PreviewHeaderKey)
  - settingsEditPageDef.Fields / ReservedButtons / Hooks, settingsEditState.Silent
  - registerSettingsEditDef and settingsEditExtraDefs (searched by settingsEditPageByKey after the three pages)
  - settingsEditFields, settingsEditDraw
  - settingsEditResolvedActions (one action set for many messages)
  - renderSettingsInputRefusalWith; the door writes and draws a hooked definition through its hooks
affects: [21-06, 21-07, 21-08, 21-09, 21-11]
tech-stack:
  added: []
  patterns:
    - "hooked editor definitions: a page defines a message through function hooks instead of a new editor kind branch"
    - "extra callback fields on editor action buttons only (never Back, Close or the navigation row)"
key-files:
  created:
    - fuku/modules/settings_edit_resolved_test.go
  modified:
    - fuku/modules/settings_edit.go
    - fuku/modules/settings_edit_actions.go
    - fuku/modules/settings_edit_check.go
    - fuku/modules/settings_edit_preview.go
    - fuku/modules/settings_input_door.go
decisions:
  - "The built-in pages' settingsEditActions now build through settingsEditResolvedActions (no extra field, a resolve that answers the page itself), which gives that function a production caller for golangci-lint's unused check; the fields, rights and Validate checks are the ones the pages had."
  - "registerSettingsEditDef has no natural production caller before plan 21-06 (it refuses the built-in keys), so it carries //nolint:unused until then."
metrics:
  duration: "about 1 h over two executors (cut off by a container restart after Task 2)"
  completed: 2026-11-19
actuals:
  tokens: 12300
  tasks: 3
  commits: 4
plan_head_before: 429f99d
plan_head_after: 7b6c645
---

# Phase 21 Plan 03: Message editor hooks Summary

The v1.1 message editor can now edit messages another page defines: a hooked definition (kind
`settingsEditEvent`) registered with `registerSettingsEditDef` is found by the wait store and the
typed-reply door like Welcome, Goodbye and Rules, runs Set, Remove, Cancel, the typed reply, the refusal
and the saved page through its own Load, Remove, Write, Draw and Preview hooks, carries its extra callback
fields (`k`) on every action button, and has the bot's own reserved buttons counted by the button check;
`settingsEditResolvedActions` serves one action-code set for many messages. Welcome, Goodbye and Rules are
unchanged: every existing editor and door test passes unedited.

## What was done

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Hooks, the extra registry and the extra callback fields in the editor | f1c0b27 | settings_edit.go, settings_edit_actions.go, settings_edit_check.go, settings_edit_preview.go, settings_edit_resolved_test.go (TestSettingsEditReservedButtons) |
| 2 | The door writes and draws a hooked definition; settingsEditResolvedActions | a98ca44 | settings_input_door.go, settings_edit_actions.go |
| - | Lint: production caller for settingsEditResolvedActions, nolint on registerSettingsEditDef | fbdd031 | settings_edit.go, settings_edit_actions.go |
| 3 | Test-only page with two hooked definitions end to end | 7b6c645 | settings_edit_resolved_test.go |

Tasks 1 and 2 were committed by a previous executor that was cut off by a container restart; this
executor verified both against every acceptance criterion, added the lint fix and Task 3, and ran every
verify command of the plan.

## Hooks for later plans (21-04, 21-06, 21-08)

```go
const settingsEditEvent settingsEditKind // after settingsEditRules

type settingsEditHooks struct {
	Load    func(ctx context.Context, chatID int64) (settingsEditState, error)                                   // required
	Remove  func(ctx context.Context, chatID int64, part string, by int64) (greetingApplyOutcome, *settingsEditState) // nil: Remove is a failed save
	Write   func(ctx context.Context, chatID int64, part string, verdict settingsEditVerdict, by int64) bool        // nil: a typed reply is a failed save
	Draw    func(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen // required
	Preview func(p *settingsPress, def settingsEditPageDef, st settingsEditState, part string) error              // part "" is Full preview; nil: settings_edit_preview_failed, nothing sent
	PreviewHeaderKey string // empty keeps settings_edit_preview_header
}

// settingsEditPageDef gains:
Fields          map[string]string // extra callback fields on Set, Remove, Preview, Full preview, the prompt's and a refusal's Cancel
ReservedButtons int               // buttons the bot adds itself (the Unban row), counted against 100
Hooks           *settingsEditHooks

// settingsEditState gains:
Silent bool // an event message's Silent switch, read by an event definition's Load and Draw only

func registerSettingsEditDef(def settingsEditPageDef) // panics on empty key, built-in key, duplicate, nil Hooks/Load/Draw
var settingsEditExtraDefs map[string]settingsEditPageDef
func settingsEditFields(def settingsEditPageDef, base map[string]string) map[string]string // base wins on a clash
func settingsEditDraw(v settingsView, def settingsEditPageDef, st settingsEditState, opts settingsEditScreenOpts) settingsScreen
func settingsEditResolvedActions(codes settingsEditPageDef, fields []string,
	resolve func(map[string]string) (settingsEditPageDef, bool)) map[string]settingsPageAction
func renderSettingsInputRefusalWith(v settingsView, cancel map[string]string, reason string) settingsScreen
```

- An event definition's `Load` must set `Kind = settingsEditEvent` and `TextIsDefault = Text == ""`.
- The definition's key is the wait's `p` (`h` + event code + destination code, e.g. `hbng`); `t` stays the part code.
- `resolve` is where the page requires its owner field (`settingsListPrivateField` for the Messages page), so a group-menu press is refused in step 1 before any I/O.
- `settingsEditApplyRemove` now takes a final `by int64`.
- `isGreeting()` is now `Kind == settingsEditWelcome || Kind == settingsEditGoodbye`.

## Verification

- Task 1 verify: `go test -tags testtools -race -count=1 -run '^(TestSettingsEditReservedButtons|TestSettingsEditCallbackFields|TestSettingsEditCallbackBytes|TestSettingsEditPages|TestSettingsEditRegistry|TestSettingsEditCheck.*|TestSettingsEditTextRulesButtonLimit|TestSettingsEditButtonRefusals|TestSettingsEditPromptMax)$' ./fuku/modules` ok (13 tests), `CGO_ENABLED=0 go build ./...` ok.
- Task 2 verify: the door, refusal, Cancel, list door and timezone families: ok (25 tests).
- Task 3 verify: `TestSettingsEditResolvedDefs` (8 subtests) and `TestSettingsEditReservedButtons` (5 subtests) pass with `-v`; the broad family `^(TestSettingsEdit|TestSettingsInput|TestSettingsRules|TestSettingsParityGreeting|TestSettingsParityMediaBelow|TestSettingsAuthorityParityEdit|TestSettingsAuthorityParityGreetings|TestSettingsCancel|TestSettingsGateAllDoors)` passes (107 tests, 332 s under -race). No FAIL, no SKIP, no "no tests to run".
- Because the lint fix routes every built-in editor action through `settingsEditResolvedActions`, also ran `TestSettingsPageActionValidate`, `TestSettingsControlActionAllowList`, `TestSettingsPhase18Codes`, `TestSettingsPhase19Codes`, `TestSettingsMainGrid` and `TestSettingsListForged`: ok.
- Mutation check: making the editor refusal's Cancel drop the extra fields fails `TestSettingsEditResolvedDefs` (restored).
- Acceptance greps: all counts as the plan states (T1 1/1/2/2 and 0 built-in lines changed; T2 1/1/1/1; T3 1/1); `git diff --exit-code c491b48 --` over the 13 existing editor and door test files exits 0; no `TestLegacy*Requests` golden touched.
- `make lint` could not run here (the installed golangci-lint was built with Go 1.25 and refuses Go 1.26). Ran `go vet -tags testtools ./fuku/modules` (clean) and `gofmt -l` on the changed files (clean) instead.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] CI golangci-lint `unused` findings on the two earlier 21-03 commits**
- **Found during:** finishing the plan (reported by the coordinator from CI, golangci-lint v2.13.1)
- **Issue:** `registerSettingsEditDef` and `settingsEditResolvedActions` had test callers only.
- **Fix:** `settingsEditActions` (the built-in pages' actions) now returns `settingsEditResolvedActions(def, nil, func(map[string]string) (settingsEditPageDef, bool) { return def, true })`: same codes, `Fields` (`t` for the four part actions, nil for Full preview), rights and part checks as before; `settingsEditValidatePart`, left without a caller, was removed. `registerSettingsEditDef` has no natural caller before plan 21-06 (it refuses the built-in keys), so it carries `//nolint:unused // first production caller arrives in plan 21-06`. **Plan 21-06, which registers the first event definitions, must remove that directive.**
- **Other symbols:** every other function, type and constant this plan added has a production reader. `settingsEditState.Silent` and the `settingsEditEvent` constant have no production reader yet either; CI did not report them, but if a later lint run does, plan 21-06 (whose event definitions read both) is where they get their caller.
- **Files modified:** fuku/modules/settings_edit.go, fuku/modules/settings_edit_actions.go
- **Commit:** fbdd031

### Other notes

- Task 3's test checks one more case than the plan's list: the same Set press of a group menu (`u` instead of `c`) is refused in step 1, and Preview through a definition's hook sends the header and the hook's line.
- `actuals.commits` counts the four 21-03 commits between `plan_head_before` and `plan_head_after`; two 21-02 commits (4c1e8bf, c9dc86a) are interleaved in that range because the plans ran in the same wave.
- AGENTS.md is not edited here, as the plan says: plan 21-06 documents the hooks in the message editor bullet when it registers the first definitions.

## Self-Check: PASSED

- FOUND: fuku/modules/settings_edit_resolved_test.go
- FOUND commits: f1c0b27, a98ca44, fbdd031, 7b6c645 (all ancestors of HEAD)
