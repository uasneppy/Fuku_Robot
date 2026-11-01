# translate_uk: write the Ukrainian locale by hand

A small local editor for `locales/uk.yml`. It shows each English string next to a box for the Ukrainian one,
refuses a text that would break a message (a lost `%s`, a dropped `<b>`), and keeps count of what you have
confirmed. Design: `docs/superpowers/specs/2026-11-01-uk-translation-tool-design.md`.

The tool runs on your own computer. It needs Go (the version in `go.mod`) and nothing else: no Node, no network.

## Start

From the repository root, on the branch that has the tool:

```bash
go run ./scripts/translate_uk
```

Open the address it prints (`http://127.0.0.1:8080`). Stop it with Ctrl+C. It only listens on your own machine;
use `-addr 127.0.0.1:9000` if the port is taken.

The draft `locales/uk.yml` is already in the repository. If it is missing, create it once:

```bash
go run ./scripts/translate_uk -init
```

`-init` starts from the Russian text, so every string needs your eyes. Nothing counts as done until you press
**Confirm**.

## The daily flow

1. Open `glossary.yml` first and fill in `uk` for the terms you want fixed (ban, mute, lockdown...). Write a stem:
   `бан` matches бан, бану, забанити. The editor warns when a text containing the English term does not use it.
2. Pick a module on the left, then a key. Correct the Ukrainian on the right.
3. Press **Ctrl+Enter** (or Cmd+Enter) to confirm and jump to the next unconfirmed key. **Save** keeps your text
   without confirming it.
4. Commit `locales/uk.yml` and `scripts/translate_uk/confirmed.txt` often. Git is your backup.

What you will see under the box:

- **Cannot save** (red): the text would break something. A `%s` or `{name}` is missing or extra, a tag does not
  match, a stray `<` or `>` was added, or a list has a different number of items. Fix it to continue.
- **Check** (amber): worth a look, never blocks. Letters that do not exist in Ukrainian (`ы э ъ ё`) and Russian
  function words mark a leftover from the Russian draft; a much longer text may not fit Telegram's limits; a
  glossary term is rendered differently from your choice.
- The chips under the box are the placeholders and tags of the English text. Red means the Ukrainian does not
  contain it yet.
- `<userhandle>`, `<keyword>` and similar are argument names in usage lines, not Telegram tags: you may translate
  the name, but keep the angle brackets and do not drop or add one.

Texts you have not saved are kept while you move between keys and the page warns before you close it.

## Other commands

| Command | What it does |
| --- | --- |
| `-check` | Prints progress per module and exits 1 until every key is confirmed and valid. |
| `-sync` | Appends keys that `en.yml` has and `uk.yml` lacks (Russian or English text, unconfirmed). Run it after anyone adds a locale key; `make check-translations` needs every key in every locale. |
| `-init -force` | Starts over: rewrites `uk.yml` from Russian and **empties `confirmed.txt`**. |

If the English text of a key you confirmed changes later, the key shows "English changed, re-check" and counts as
unconfirmed again, because `confirmed.txt` stores a hash of the English each confirmation was made against.

## When you are done

When `go run ./scripts/translate_uk -check` exits 0, tell Claude. Registering `uk` in the bot (the language
picker, the captcha page, the tests and the docs) is a separate, gated step. Until then no user can select the
language, even though the file is in the repository.
