---
phase: 07-ai-assisted-raid-detection
plan: 05
subsystem: ai-raid-detection
tags: [gemini, spike, cli-tool, accuracy, cost, probe, stdlib]

requires:
  - phase: 07-ai-assisted-raid-detection
    provides: "alita/utils/gemini client (Classify, GetModel, Options, HTTPError, ErrBlocked) from plan 07-02"
provides:
  - "scripts/gemini-spike: standalone tool with a labelled accuracy, latency, token and cost run, -probe and -models-get"
  - "07-SPIKE.md: the owner's guide (samples, run order, reading, decision table, constants to edit, live-only checks)"
affects: [07-08 image path (raidAIImageBar), 07-09 budgets, 07-11 docs]

actuals:
  tokens: 15150
  tasks: 2
  commits: 4

plan_head_before: 64b384ed107868efc0042c2a1f8ae75120812bb6
plan_head_after: f1919f98220395bc0dbdc0f8f437b267ca253b9a
commits: 4

tech-stack:
  added: []
  patterns:
    - "package main as a thin wrapper over run(args, getenv, stdout, stderr, transport) int, so tests drive every mode with a counting RoundTripper and never touch the network"
    - "Key only from GEMINI_API_KEY, never a flag; refused before any client is built; scrubbed from every printed line"
    - "Pure report functions (labelOf, summarize, sweep, percentile, estimateCost, writeCSV) tested with fixed inputs"

key-files:
  created:
    - scripts/gemini-spike/main.go
    - scripts/gemini-spike/report.go
    - scripts/gemini-spike/probe.go
    - scripts/gemini-spike/report_test.go
    - scripts/gemini-spike/main_test.go
    - .planning/phases/07-ai-assisted-raid-detection/07-SPIKE.md
  modified: []

key-decisions:
  - "A file counts as labelled only if it has an image extension and a spam/ or ham/ parent folder (else a spam_ or ham_ prefix); one labelOf function covers both rules"
  - "The tool refuses to run (exit 2) before building any client when GEMINI_API_KEY is empty; -fake-key substitutes the built-in fake key only together with -probe"
  - "-probe exits 1 when the API rejected a field name ('Invalid JSON payload'), because that means a request spelling in alita/utils/gemini needs a fix"
  - "CSV goes to standard output (or -csv file) followed by the summary on standard output; the scan line and skipped counts go to standard error"
  - "Bars are flagged when probability >= bar; blocked and failed images are counted apart and excluded from the matrix and every rate"
  - "Percentiles use nearest rank; cost = (prompt*in + (candidates+thoughts)*out)/1e6, with prices as flags defaulting to 0.30 and 2.50 and a printed 'verify at ai.google.dev/gemini-api/docs/pricing' note"
  - "The 1x1 WebP is a base64 literal (26 bytes) because the standard library has no WebP encoder"

requirements-completed: [RAID-04]

coverage:
  - id: D1
    description: "Labelled run prints a per-image CSV and a summary: accuracy, precision, recall, confusion matrix at the 0.85 bar, a sweep of bars 0.50 to 0.95, blocked and failed counts, latency percentiles, tokens and estimated cost"
    requirement: "RAID-04"
    verification:
      - kind: unit
        ref: "scripts/gemini-spike/report_test.go#TestSpikeSummary"
        status: pass
      - kind: unit
        ref: "scripts/gemini-spike/report_test.go#TestSpikeCost"
        status: pass
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeNoKeyMakesNoRequest"
        status: pass
    human_judgment: false
  - id: D2
    description: "Without GEMINI_API_KEY the tool prints usage, exits non-zero and sends no request; -probe sends only with the key or -fake-key; the key never reaches stdout or stderr"
    requirement: "RAID-04"
    verification:
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeNoKeyMakesNoRequest"
        status: pass
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeProbeRefusesWithoutAnyKey"
        status: pass
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeRefusesBadFlags"
        status: pass
    human_judgment: false
  - id: D3
    description: "Labels come from a spam/ or ham/ folder or a spam_ or ham_ prefix; other files are skipped; only jpg, jpeg, png and webp are read"
    verification:
      - kind: unit
        ref: "scripts/gemini-spike/report_test.go#TestSpikeLabels"
        status: pass
    human_judgment: false
  - id: D4
    description: "-probe sends one request per image type (JPEG, PNG, WebP) and per option variant and reads each answer; -models-get prints name, token limits and generation methods"
    requirement: "RAID-04"
    verification:
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeProbeVerdicts"
        status: pass
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeModelsGet"
        status: pass
    human_judgment: false
  - id: D5
    description: "The tool's flags reach the request (media resolution, temperature, thinking, store, safety)"
    verification:
      - kind: unit
        ref: "scripts/gemini-spike/main_test.go#TestSpikeOptionsReachTheRequest"
        status: pass
    human_judgment: false
  - id: D6
    description: "07-SPIKE.md gives the owner a usable order, decision table and list of constants to edit"
    verification: []
    human_judgment: true
    rationale: "Prose guidance for a person; no test can say whether it is clear enough. The facts it states (flag names, constant names) were checked against the code by reading."
  - id: D7
    description: "The tool works against the real Gemini API with the owner's key"
    verification: []
    human_judgment: true
    rationale: "No key is available in this environment; only a live run proves the request shape, accuracy, latency and cost (see Pending Human Verification)"

duration: 6min
completed: 2026-10-29
status: complete
---

# Phase 7 Plan 05: Gemini spike kit Summary

**A standalone `scripts/gemini-spike` tool that sends a folder of labelled images through the bot's own Gemini client and prints accuracy, precision, recall, a sweep of bars, latency percentiles, tokens and estimated cost, with `-probe` and `-models-get` modes and a 07-SPIKE.md guide telling the owner which constants to change.**

## Performance

- **Duration:** about 6 min (18:06 to 18:13 UTC)
- **Tasks:** 2 (both TDD)
- **Files:** 6 created (3 source, 2 test, 1 guide), 0 modified; `go.mod` and `go.sum` unchanged

## Accomplishments

- **Labelled run.** `go run ./scripts/gemini-spike -dir <folder>` walks the folder, labels files (`spam/` or `ham/` parent folder, else a `spam_` or `ham_` prefix), checks the type from the file's bytes and the 5 MiB cap, classifies with `-concurrency` workers (default 2, at most 8) and `-limit`, and prints a CSV row per image (`file,label,probability,category,finish,ms,prompt_tokens,output_tokens,thought_tokens`) then the summary. The summary has the confusion matrix at `-bar` (default 0.85), a sweep over 0.50 to 0.95, blocked and failed counts kept out of the matrix, p50, p95 and max latency, token totals and the cost with a cost per 1000 images. A blocked answer is counted as blocked; any other failure shows only its HTTP status and code.
- **No key, no request.** The key is read only from `GEMINI_API_KEY`. With none, `run` prints the usage (which names the variable, the three modes and the warning that images go to Google) and returns 2 before any client is built. Bad flags (model name, bar, concurrency, temperature, store, schema field) also return 2 with nothing sent. The key is scrubbed from every printed line and tests assert it never appears in stdout or stderr, even when the server echoes it.
- **`-probe`.** Sends one tiny request per variant: JPEG, PNG and WebP images, each of the three structured-output field spellings, thinking MINIMAL and off, the three media resolutions, store true and false and a safety threshold (14 requests). Each answer is read by `probeVerdict`: `Invalid JSON payload` is "field rejected", `API_KEY_INVALID` is "shape accepted, key refused", a verdict is "accepted and answered", anything else shows status, code and the start of the message. With no key it needs the explicit `-fake-key`, which uses a built-in fake key and no real image. Exit code 1 when any field was rejected.
- **`-models-get`.** Prints the model's name, display name, input and output token limits, generation methods and whether it takes `generateContent`; a failure shows the status without the key.
- **07-SPIKE.md.** Why the spike is deferred (D-17) and that it replaces the AI-SPEC evaluation (D-22); a paid-tier key and the fact that images go to Google (D-21); how to collect samples per media type (photo, static sticker, saved JPEG thumbnails of a GIF, a video sticker and an animated TGS sticker); the run order; how to read each number; a decision table; and the exact constants to edit afterwards (`raidAIImageBar`, `defaultMediaRes`, thinking level, temperature, `raidAIChatHourlyBudget`, `raidAIGlobalHourlyBudget`, the media rules, `GEMINI_MODEL`); plus the live-only checks from 07-VALIDATION.md.

## Task Commits

1. **Task 1 RED:** `28b7811` test(07-05): show the Gemini spike tool does not exist
2. **Task 1 GREEN:** `d0d79fc` feat(07-05): add the Gemini spike tool for a labelled accuracy and cost run
3. **Task 2 RED:** `c200c59` test(07-05): show the spike tool has no probe or models-get mode
4. **Task 2 GREEN:** `f1919f9` feat(07-05): add the probe and model check to the spike tool, with the owner's guide

**Plan metadata:** docs(07-05) commit following this file (SUMMARY only; STATE.md and ROADMAP.md are owned by the orchestrator).

## Files Created

- `scripts/gemini-spike/main.go` - flags, `run`, key handling, scan, worker pool, `runProbe`, `runModelsGet`
- `scripts/gemini-spike/report.go` - `labelOf`, `result`, `writeCSV`, `summary`/`summarize`, `defaultBars`, `sweep`, `percentile`, `estimateCost`, `costPer1000`, `totals`/`totalsOf`, `writeSummary`
- `scripts/gemini-spike/probe.go` - `probeVariant`, `probeVariants`, `probeVerdict`, the tiny images
- `scripts/gemini-spike/report_test.go`, `scripts/gemini-spike/main_test.go` - nine top-level tests, all network-free
- `.planning/phases/07-ai-assisted-raid-detection/07-SPIKE.md` - the owner's guide

## Decisions Made

See `key-decisions`. The plan's Decisions section was followed as written (file layout, `run` signature, key only from the environment, label rules, price flags, tiny images). The `result` struct carries the finish text; `totals` was added so `writeSummary` gets one value for latency, tokens and cost.

## Deviations from Plan

None - plan executed exactly as written. Small additions inside the plan's own threat mitigations (T-07-17, T-07-19): `TestSpikeRefusesBadFlags` and `TestSpikeOptionsReachTheRequest` (beyond the five tests named) prove bad flags send nothing and the flags reach the request; `-concurrency` is capped at 8; free-text flags (`-media-res`, `-safety`, `-thinking`) accept letters, digits and underscores only.

## Verification

- `CGO_ENABLED=1 go test -race -count=1 -v ./scripts/gemini-spike`: all nine tests PASS (`TestSpikeLabels`, `TestSpikeSummary`, `TestSpikeCost`, `TestSpikeNoKeyMakesNoRequest`, `TestSpikeRefusesBadFlags`, `TestSpikeOptionsReachTheRequest`, `TestSpikeProbeVerdicts`, `TestSpikeProbeRefusesWithoutAnyKey`, `TestSpikeModelsGet`). Also green with `-tags testtools`, together with `./alita/utils/gemini`.
- `CGO_ENABLED=0 go build ./...` clean; `go vet ./scripts/gemini-spike` and `go vet -tags testtools ./scripts/gemini-spike` clean; `gofmt -l scripts/gemini-spike` prints nothing.
- Acceptance: imports are standard library plus `github.com/divkix/Alita_Robot/alita/utils/gemini` only (checked with `go list -f '{{.Imports}}'`); `GEMINI_API_KEY` in `main.go` (3), `func run(` (1), `func summarize(` (1), `func probeVariants(` (1), `fake-key` and `models-get` present; `07-SPIKE.md` contains `D-22`, `raidAIImageBar` and `paid`; `git diff --exit-code go.mod go.sum` exits 0.
- `make lint` was NOT run: golangci-lint is built with Go 1.25 against the module's Go 1.26 here. `gofmt -l` and `go vet` stood in for it. The orchestrator runs the full `make test` after the wave (it compiles and runs `./scripts/gemini-spike` through `./...`).
- No test calls a real host, none uses `t.Skip`, no mock library, no PostgreSQL was started.

## Pending Human Verification (live provider, cannot run here)

These need the owner's real key (steps are in `07-SPIKE.md`):

1. `export` a paid-tier `GEMINI_API_KEY` (typed with `read -rs`), then `go run ./scripts/gemini-spike -probe -fake-key`: expect `shape accepted, key refused` on all 14 lines and exit 0 (no key needed).
2. `go run ./scripts/gemini-spike -probe`: expect no `field rejected` line. Any `rejected: HTTP 400 ...` line names a setting the model does not accept.
3. `go run ./scripts/gemini-spike -models-get`: expect `Takes generateContent: yes`; a 404 means `GEMINI_MODEL` (default `gemini-3.5-flash-lite`, research assumption A1) is wrong.
4. `go run ./scripts/gemini-spike -dir <samples> -csv base.csv` on 30+ spam and 30+ ordinary images per type: read accuracy, precision and recall at 0.85, the sweep, blocked rate, p95 latency against the 8 s timeout, and the cost per 1000 images (check prices at ai.google.dev, assumption A2).
5. Variant runs (`-temperature default`, `-thinking LOW`, `-media-res MEDIA_RESOLUTION_LOW`, `-safety BLOCK_NONE`, `-model <other>`) and then apply the decision table to `raidAIImageBar` and the other constants. Note `raidAIImageBar` and the budgets are added by plans 07-08 and 07-09 of this phase; they do not exist on this branch yet.
6. The `-schema-field responseFormat` spelling is unverified against a live key (spike-only, from the 07-02 summary).

## Known Stubs

None.

## Threat Flags

None - the tool adds no endpoint to the bot. Its one outbound call (to Google, with the owner's key and sample images) is covered by T-07-17, T-07-18 and T-07-19.

## Self-Check: PASSED

- Files verified on disk: `scripts/gemini-spike/main.go`, `report.go`, `probe.go`, `report_test.go`, `main_test.go` and `.planning/phases/07-ai-assisted-raid-detection/07-SPIKE.md`.
- Commits verified: 28b7811, d0d79fc, c200c59, f1919f9 (4 commits since base 64b384ed, measured with `git rev-list --count`).
