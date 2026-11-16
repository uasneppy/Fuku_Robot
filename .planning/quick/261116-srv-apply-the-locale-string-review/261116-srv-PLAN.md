---
quick_id: 261116-srv
description: Apply the full review of the bot's user-facing strings (47 global wording rules, wrong facts, per-string findings) to locales/en.yml and locales/uk.yml, with the fact fixes in the six other locales
---

# Plan

## Task 1: code checks first

Read the callers of the keys the review flagged (antiflood action words, content_need_content, the HTML replies with
raw angle-bracket hints, the Markdown help pages, random content, /cleangoodbye, /resetrules, the time parser, the
extraction errors, /runs, the reason lines appended after ban, kick and tban replies, the report buttons, quoted button
names) and note what each string has to say.

## Task 2: global rules, one commit per rule family

Ukrainian vocabulary (mute, ban, locks, lockdown, admin, member, approved, requirements, username, private chat, join,
staff link, undo, owner, log channel, failure, invalid input, retry, who did it, impersonal plurals, exempt, gender,
captcha); English house style (contractions, please, Request denied., warning counts, IDs, units, ellipsis, British
spelling); both (first person, no emoji after the name, full stops, × bullets, one on/off refusal).

## Task 3: wrong facts

Fix them in en and uk, and the same facts in es, fr, hi, id, pt, ru in their existing plain wording.

## Task 4: per-string findings, file by file

Apply the five findings files, adjusting each proposal to the global rules, with voice only in ordinary modules.

## Constraints

Values only; placeholders and markup unchanged except the deliberate markup fixes; length tests stay green by
shortening strings; stored replies in tests updated to the new wording and nothing else.

## Verify

`CGO_ENABLED=1 go test -tags testtools -count=1 ./...`, `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./fuku/...`,
`make check-translations`, `make generate-docs` + `make check-docs`.
