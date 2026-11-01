# Ukrainian translation tool — design

Date: 2026-11-01 · Status: awaiting review · Scope: design only, nothing is implemented yet

## Goal

Add Ukrainian (`uk`) as the 8th locale of Fuku Robot, with every string reviewed and written by the owner by hand
(no machine translation). The owner needs a comfortable way to do it: about 1,415 keys and 3,100 lines of
`locales/en.yml`, full of `%s` / `{name}` placeholders and Telegram-HTML tags that must not break.

Success means:

- `locales/uk.yml` has every key of `en.yml`, and every string has been explicitly confirmed by the owner.
- Placeholders and HTML tags in each Ukrainian string match the English one, so no message can be rejected by
  Telegram or print a raw `%s`.
- `make check-translations` and `make test` pass with `uk` registered, and a group admin can pick Ukrainian in `/lang`.

## Decisions already made with the owner

- Approach: a small local translation tool (not a spreadsheet, not hand-editing the YAML).
- Starting point: `uk.yml` is pre-filled from `ru.yml`, because Ukrainian is close to Russian and correcting is much
  faster than writing from nothing. The risk is Russian leaking through, so nothing counts as done until the owner
  presses Confirm on it.

## 1. The tool and where it lives

`scripts/translate_uk/`, one Go program in the main module, run on the owner's own machine with
`go run ./scripts/translate_uk` and opened at `http://127.0.0.1:8080` (loopback only, no auth needed). It is not run
in the cloud session.

- **Imports:** standard library and `gopkg.in/yaml.v3` (already a dependency) only. It must not import `alita/config`
  or `alita/db`, which connect to PostgreSQL before `main`; `scripts/claude-spike` follows the same rule.
- **Web page:** one HTML file embedded with `go:embed`. No CDN, no Node, works offline.
- **Modes:**
  - default: serve the editor.
  - `-init`: create the draft `locales/uk.yml` and refuse to overwrite an existing one unless `-force` is given.
  - `-check`: print per-module progress and exit non-zero while any key is unconfirmed or invalid.
- **Draft contents:** keys in `en.yml` order, values copied from `ru.yml`. Only `lang_sample` (`Українська`),
  `language_flag` (`🇺🇦`) and `language_name` (`Ukrainian`) are set to Ukrainian from the start.
- **Progress file:** `scripts/translate_uk/confirmed.txt`, committed to git. One line per confirmed key:
  `key<TAB>first 8 hex chars of sha256(English string)`. This is a refinement of the "list of keys" idea: when a later
  change edits the English text of a key, its stored hash no longer matches and the key goes back to unconfirmed, so
  the owner re-checks it instead of silently keeping a stale translation. The file lives under `scripts/`, not
  `locales/`, because `locales/` is embedded into the binary and every file there is read by the locale loader and by
  `scripts/check_translations`.
- **Writing `uk.yml`:** the whole file is rewritten atomically (temp file + rename) on each save; 1,415 keys is small.
  Emit style does not have to match the other locales; the requirement is an exact value round-trip
  (`parse(write(x)) == x`), covered by a test.

### Why a draft `uk.yml` in `locales/` is safe before it is finished

A user can only reach a language that passes both of these, and neither lists `uk` until the finishing step:

1. the allow-list in `alita/modules/language.go:107`, which rejects any other code with `language_invalid_selection`;
2. the `/lang` keyboard, which is built from the `ENABLED_LOCALES` environment variable (default `en`, see
   `alita/config/config.go:493` and `alita/utils/keyboard/keyboard.go:40`), not from the files in `locales/`.

The loader does read every file in `locales/`, so `uk` will appear in the "Locale manager initialized with N languages"
log line and be checked by `make check-translations` (it will pass, since every key exists). The draft is a copy of
`ru.yml` apart from the three `lang_*` keys, so it also passes the existing per-locale length tests.

## 2. The editor

- **Left column:** modules, grouped by the part of the key before the first underscore (`admin`, `staff`, `settings`,
  `lockdown`, `captcha_web`, ...), each with `confirmed / total`, plus a text filter and an "unconfirmed only" toggle.
- **Main area:** per key, the English string on the left (read-only) and an editable Ukrainian box on the right,
  marked unconfirmed until Confirm. `Ctrl+Enter` saves, confirms and jumps to the next unconfirmed key, so a string
  that needs no change costs one keystroke.
- **Blocking checks (save refused, message shown):**
  - the multiset of placeholders must equal the English one: printf verbs (`%s`, `%d`, `%v`, `%[1]s`, `%%`) and
    `{name}` tokens;
  - HTML tags must match: same tags, same count, balanced and properly nested; tags with attributes (`<a href="...">`)
    must match exactly, so a link target cannot be altered.
- **Warnings (shown, never block):**
  - letters that do not exist in Ukrainian (`ы э ъ ё`, either case);
  - a short, editable list of common Russian function words (`что`, `это`, `или`, `как`, `только`, `если`, `чтобы`,
    `когда`), which catches pre-fill leftovers the letter check cannot see;
  - Ukrainian longer than 1.4x the English length in characters. This is only a heuristic; the authoritative limits are
    the existing tests (for example `TestSettingsHelpFitsOneMessage` and the `settings_` limits table in
    `alita/i18n/settings_locale_test.go`);
  - more than one apostrophe character in use (`'`, `’`, `ʼ`) across the file.
- **Glossary panel:** `scripts/translate_uk/glossary.yml` maps a term (ban, mute, kick, warn, captcha, lockdown, staff
  group, approved user, ...) to the one Ukrainian rendering the owner picks, with optional forbidden variants. When the
  English text contains a term and the Ukrainian text does not use the chosen rendering, the key gets a warning. The
  file starts with the term list and no translations; choosing them is the owner's first step, and they are not decided
  in this spec.
- **Confirmation rules:** pressing Confirm is the only way a key becomes confirmed; saving text alone does not confirm
  it. Editing a confirmed key's text removes its confirmation until it is confirmed again.

## 3. Finishing: registering `uk`

Only after `go run ./scripts/translate_uk -check` exits 0, in one small change:

- `alita/modules/language.go:107` — add `uk` to the allow-list.
- `alita/utils/captchaweb/page.go:52` — add `uk` to `PageLanguages`.
- `alita/utils/captchaweb/page/captcha.html:236` — add `uk` to the `LANGS` array
  (`TestPageLanguagesMatchScript` keeps these two in step).
- Tests that list the locales explicitly. A first search found `"ru"` in: `alita/modules/settings_*_test.go`,
  `staff_panel_render_test.go`, `captcha_mode_turnstile_test.go`, `alita/utils/captchaweb/initdata_test.go`,
  `alita/i18n/staff_locale_test.go`, `alita/i18n/table_test.go`. The plan will go through each and add `uk` where a
  list means "every supported language".
- Documentation of the locale list: `README.md:772`, `docs/src/content/docs/api-reference/environment.md`
  (regenerate with `make generate-docs` if generated), and a comment in `sample.env`.
- Rules that count languages: `AGENTS.md` and `.claude/CLAUDE.md` say "7 locale files" and "7 languages" in several
  places and must say 8, in the same commit (`AGENTS.md` requires that).
- Operators enable it by adding `uk` to `ENABLED_LOCALES`; the code default stays `en`.
- Then run `make check-translations` and `make test` (and `node --test scripts/captcha-page/*.test.mjs`, since the
  captcha page changes).

## 4. Testing the tool

Go tests under `scripts/translate_uk/`, run by `make test`, using real fixtures and no mock libraries:

- placeholder and tag validation, table-driven, including the failure cases (missing `%s`, extra tag, altered link);
- YAML round trip for multi-line strings, quotes, `%`, `{}` and emoji;
- `-init` produces English key order and refuses to overwrite;
- a changed English string makes a confirmed key unconfirmed again;
- `-check` exit code with and without unconfirmed keys.

Per the repo rule, each test is checked to fail when the behaviour it guards is broken.

## 5. Rollout order

1. Build the tool, run `-init`, commit the draft `uk.yml` (unreachable by users, see above).
2. The owner translates on their machine and commits `uk.yml` and `confirmed.txt` as they go; git is the backup.
3. When `-check` passes, make the registration change from section 3.

## Out of scope

Machine translation, other languages, Crowdin integration, multi-user editing, hosting the tool, and translating the
docs site or README. Two things outside the locale files could matter for "fully Ukrainian" and are not decided here:
English fallbacks hard-coded in Go (for example `"Admin commands"` in `alita/utils/keyboard/keyboard.go`), and
`locales/config.yml` (the help alt-name pseudo-locale, not a language). The plan phase will grep for the hard-coded
strings and bring the list back for a decision.

## Spec self-review

- Placeholders: no TBD or TODO left. The glossary terms and Ukrainian renderings are intentionally empty; section 2
  says so and assigns the choice to the owner.
- Consistency: progress is stored as `key + hash` everywhere (section 1 and the confirmation rules); the earlier
  "list of confirmed keys" wording is superseded by that.
- Safety claim checked against code: the picker uses `ENABLED_LOCALES` and the callback uses the allow-list, so an
  unregistered draft is not user-reachable; the loader does load it, which is stated.
- Not verified yet: that every existing test passes with the draft file present. That is the first check in the plan.
