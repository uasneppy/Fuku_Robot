---
title: Text style guide
description: How Xenobot-72 writes its help pages and replies in Ukrainian and English, and the glossary of terms.
---

<!-- MANUALLY MAINTAINED: do not regenerate -->

This guide covers the Ukrainian (`locales/uk.yml`) and English (`locales/en.yml`) texts of the bot: the help pages and
the replies after commands. The other six languages keep their wording; only mechanical markup changes reach them.

`TestLocaleStyle` in `fuku/i18n` checks the rules a program can check. Its allow-list is empty: every Ukrainian and
English text follows the guide, and a new one must too.

Status: the owner approved this guide on 2026-11-18, with the sample batch, and every Ukrainian and English text was
converted to it the same day. The decisions taken with the approval: long help pages keep their topic sections, a
reply joins the persona phrase to the fact with a colon on line 1, and `/warn` keeps the count on its own labelled
line.

## Voice

- The facility voice stays: the bot speaks as a unit that keeps order in "facilities" (об'єкти) with "residents"
  (мешканці). One short persona phrase per text at most, never in place of a fact.
- Plain words, short sentences. Say what happened, then what to do.

## Markup

- HTML only: `<b>`, `<i>`, `<code>`, `<a>`, and every message is sent with HTML parse mode. No markdown (`*bold*`, `` `code` ``, `_italic_`) in Ukrainian or English
  strings.
- Angle-bracket placeholders are written as `&lt;…&gt;` inside `<code>`: `<code>&lt;користувач&gt;</code>`.
- No leading or trailing whitespace, no double spaces, never three newlines in a row.
- A text that shows Markdown syntax as an example writes a backtick as `&#96;` and an asterisk as `&#42;`
  (`formatting_markdown`).

## Help pages

```
<b>🔨 Бани</b>

Одне-два речення про те, що робить модуль.

<b>Команди для всіх</b>
• /kickme — вилучає вас із чату.

<b>Команди адмінів</b>
• /ban <code>&lt;користувач&gt;</code> — банить користувача.
• /tban <code>&lt;користувач&gt; &lt;час&gt;</code> — банить на час: <code>30m</code>, <code>2h</code>, <code>7d</code>.

<b>Примітки</b>
Вкажіть користувача через @username чи ID або відповідайте на його повідомлення.

<b>Приклад</b>
<code>/tban @user 2h</code> — бан на дві години.
```

- One title line: the module's icon and its translated name (`help_module_name_<module>`), in bold. The page carries
  its own title, so no generic header is added above it. A page not yet converted gets the title from
  `helpers_module_help_header` (`<b>icon name</b>`).
- Order: title, intro, command sections, topic sections, notes, example.
- One blank line between sections, none inside a list, none at the end.
- Bullet `•`. The command, then its arguments in `<code>`, then an em dash with spaces (` — `), then the description,
  starting with a lower-case verb form and ending with a full stop.
- A command that is a whole example on its own (`<code>/lockdown [reason]</code>`) may sit in `<code>` with its
  arguments.
- Section headings come from one set, the same words on every page:

| Ukrainian | English |
|-----------|---------|
| Команди для всіх | Commands for everyone |
| Команди адмінів | Admin commands |
| Команди власника | Owner commands |
| Примітки | Notes |
| Приклад | Example |

- A long page may add topic sections between the command sections and the notes, each headed by a short noun phrase
  in bold, the same words in both languages' sense (Lockdown: «Автоматичний локдаун» / «Automatic lockdown»,
  «Кнопки сповіщення» / «Alert buttons»; Captcha: «Типи капчі» / «Captcha types»). The fixed headings above are still
  used for the sections they name.

## Replies after commands

- Line 1 says what happened, with the person as a mention. The persona phrase, where there is one, opens that line and
  is joined to the fact with a colon: `Видалення з об'єкта виконано: Олена забанено.`
- Details follow as labelled lines: `<b>Причина:</b> …`, `<b>Термін:</b> …`, `<b>Попереджень:</b> 2/3`. A count such as
  the warnings of `/warn` always has its own labelled line.
- A list in a reply puts each item on its own line after a bullet `•`; the code adds the bullets, so a translation
  carries only the line before the list.
- A text that is a fragment of a longer message carries no blank lines of its own at its start or end: the code joins
  the parts (a fragment may start with one newline when it is a detail line, as `bans_kick_kicked_reason` does).
- Values a user typed or a stored value spliced into a text are escaped for HTML at the send site.
- No blank line inside a short reply; one blank line before a hint.
- Errors: what went wrong, then what to do, in at most two sentences.

## Punctuation

- Ukrainian quotes «…»; English quotes "…".
- The dash is "—" with a space on each side.
- No space before punctuation; one space after it.

## Glossary

One spelling for each term across a file.

| Ukrainian | English | Notes |
|-----------|---------|-------|
| адмін, адміни | admin, admins | not «адміністратор» in short texts |
| власник групи | group owner | the creator |
| учасник | member | |
| мешканець | resident | persona only |
| об'єкт | facility | persona only |
| група | group | «чат» only for a private chat or where Telegram says chat |
| приватний чат | private chat | |
| бан, забанити, розбанити | ban, banned, unban | |
| вилучити (з чату) | kick | not «кікнути» |
| мут, видати мут, зняти мут | mute, muted, unmute | |
| попередження | warning | count: «Попереджень: 2/3» |
| замок, закрити на замок | lock, locked | |
| локдаун | lockdown | |
| реакція | reaction | |
| фільтр | filter | |
| нотатка | note | |
| чорний список | blacklist | |
| привітання, прощання | welcome, goodbye | |
| правила | rules | |
| капча | captcha | |
| флуд, антифлуд | flood, antiflood | |
| Staff-група | Staff Group | |
| канал журналу | log channel | |
| схвалений учасник | approved member | |
| анонімний адмін | anonymous admin | |
| часовий пояс | timezone | |
| налаштування, меню налаштувань | settings, settings menu | |
| посилання-запрошення | invite link | |
| причина | reason | label |
| термін | duration | label |
