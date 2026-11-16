---
quick_id: 261116-srv
status: complete
---

# Summary

- Applied the string review to `locales/en.yml` (364 values changed) and `locales/uk.yml` (522 values changed), as
  global passes per rule family, then the per-string findings file by file. No key added, removed or renamed; en and
  uk placeholders and tags still match each other.
- The six other locales got only the wrong-fact fixes (19 or 20 values each, in their existing plain wording).
- Code checks found reply strings Telegram would reject as HTML (raw `<keyword>`-style hints in /addreaction,
  /removereaction, /log, /github, /wiki, /ud, /reports and /connect replies); they are escaped or rewritten. The ban,
  kick and promote-title extra lines were glued to the sentence before them; their keys now start with a line break.
- Stored replies in twelve legacy request goldens and the Phase 16 deletion screen digests were updated to the new
  wording only.

## Verification

See the hand-off report of this task for the command results.
