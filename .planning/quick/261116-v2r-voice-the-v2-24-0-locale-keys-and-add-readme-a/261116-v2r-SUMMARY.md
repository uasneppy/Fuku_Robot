---
quick_id: 261116-v2r
status: complete
---

# Summary

- Reviewed all 434 new keys in en and uk. English: 4 changed (three refusals with a "Request denied." /
  "Note name rejected." opening, one step confirmation), 430 left as they were (settings, staff and screening
  texts stay near plain wording). Ukrainian: 48 changed, 386 left: masculine forms for the bot removed, pronouns that
  assumed a member's or owner's gender rephrased, and the file's vocabulary used (мут/замучено, вигнати/вигнано,
  схвалені, канал журналу).
- The eight keys main changed are back in the voice with main's facts (/captchamaxattempts, no donation page,
  /about without links, the website line, the forward blocklist section of the staff help). English staff_help_msg
  needed no change.
- `settings_del_svc_gifts` (uk) stays "Розіграш, дарунки": the plural form is over the 18-character label ceiling.
- README.md and README.uk.md gained an Attributions section before License, in the table of contents; the old
  one-line credit moved into it.
- No stored test wording needed updating.

## Verification

- `CGO_ENABLED=1 go test -tags testtools -count=1 ./...`: exit 0, 69 packages ok (fuku/modules 346 s).
- `CGO_ENABLED=0 go build ./...`, `go vet -tags testtools ./fuku/...`: clean.
- `make check-translations`: all present. `make generate-docs`: no change. `make check-docs`: no drift.
