# 261115-pvl Task 2: README.uk.md notes

Commit: `563dba0` (`docs: add original Ukrainian README`) on `claude/ecstatic-goldberg-qsss05`, one commit after `6733c61`. `git show --stat` lists exactly `README.uk.md` (1052 insertions, no deletions). Nothing pushed, no branch created, nothing under `.planning/` committed.

README.uk.md is 1052 lines, the same count as README.md.

## Commands run and results

- Guard: branch not protected, SEQUENTIAL=true, plan ledger `.git/gsd-plan-head-before-261115-pvl-02`.
- Build method: the prose was written in a template in the session scratchpad with `@@B1@@` to `@@B9@@` placeholders; a throwaway script (scratchpad only, nothing in the repo) replaced each placeholder with the exact line range of the matching fenced block of README.md (lines 640-673, 737-742, 750-754, 760-767, 776-782, 784-786, 792-801, 989-1015, 1034-1038). Result: all 9 fenced blocks are byte-identical (verified with a regex comparison, `True 9`).
- Inline code spans: the multiset of backticked spans of README.md and README.uk.md is identical (no spans only in one file).
- `check_readme.py README.md README.uk.md`: `SUMMARY fails=0 reviews=4`, exit 0, with the sibling link test running without a skip. All 231 registry commands appear in the Commands section of both files.
- `check_readme.py README.uk.md` alone: exit 0.
- `check_readme.py --parity README.md README.uk.md`: 40 headings with the same levels, 68 env names in both, `prose characters 42373 vs 44255 (ratio 104%)`, `SUMMARY fails=0 reviews=0`, exit 0.
- First run had one FAIL: the calque regex `являєть?ся` matched inside `з'являється` (table row of `/addreaction`). Reworded to "Реагувати емодзі на ключове слово". Also fixed typos before the commit (a Russian-looking plural in two places, "зі наскрізним", "могти").

## Length ratio

104 percent (44255 vs 42373 prose characters, checker's own measure), under the 115 percent cap.

## Flavour lines written differently from the English, and why

- Intro: "Мене створено в Jaya Abadi, щоб керувати цим об'єктом. Об'єкт — це група в Telegram." (gender-free passive instead of "Jaya Abadi built me"); "Чергувати вночі біля групи нікому не треба: я не сплю." (own shape for "nobody has to stay up"); "Без паніки. Я впораюся." (the brief's UK rendering of "No sweat"; English tagline also "No sweat"). "версія 72 з 74" once.
- Moderation: "Порядок передусім. Перевіряю права щоразу, навіть власні." Protection: "Спам обробляється так само, як усе інше: за процедурою, без емоцій."
- Content: "тут я привітна. Сьогодні в пайку темпе." (feminine adjective, own rhythm); Greetings: "Меню без змін: темпе." (a different second tempe line, not a repeat).
- Command lines: Bans "Увага. Право обмежувати учасників потрібне і адміну, і мені. Перевіряю щоразу."; Mutes "Увага. Права ті самі, що для банів."; Warns "Облік попереджень веду я."; Purges "Процедура очищення."; Admin "Доступ наглядача видається за процедурою." (glossary word used once); Approvals "Список веду я."; Antiflood "Повідомлення рахую я, вам не доведеться."; Blacklists "Слова, які я видаляю одразу. Які саме, вирішуєте ви."; Notes "Пам'ятаю я, щоб група не мусила."; Filters "Бачу тригер — відповідаю."; Rules "Регламент. Він є на кожному об'єкті."; Pins "Закріплені оголошення лишаються нагорі."; Reactions "Ключове слово я відзначаю емодзі."; Log channels "Кожну мою дію занесено до журналу."; Reports "Скарга доходить до адмінів. Адміни не можуть її викликати."; Misc "Дрібні запити теж обробляються."; Dev "Решті запит відхилено." All avoid gendered forms for the bot except "привітна" (feminine, consistent) and "Я написана".
- Everything in Staff Group, Lockdown, member requirements, deletion, settings, captcha, forward blocklist, Getting started, Configuration, Endpoints, Structure and Development is plain and joke-free.
- Product terms reused from `locales/uk.yml`: «Staff-група», локдаун, капча, антифлуд, «Чорний список», «Блок-джерела», «Вимоги», «Видалення», «Команди», «Відредаговані», «Самовидалення», «Канал логів», «Анонімні адміни», button names «Зняти локдаун», «Заблокувати нових учасників», «Відкликати посилання», «Відв'язати», «Останні дії», «Виправлено», «Перевіряти вхід», «Видаляти повідомлення», «Один раз».

## REVIEW judgements

- `[figures]` spelled-out numbers on lines 68, 102, 104, 108, 251, 628, 756, 758, 812, 834, 880, 914: each mirrors a spelled number of the English file and is a verified count: three captcha modes, all three raid triggers, three alert buttons, eight requirement rules, three `/setgdesc` aliases, three retired commands, two facts about the compose stack, five passed compose variables (twice), four required Turnstile settings (twice), two monitoring defaults switched off by `DEBUG`. All accepted. (README.md has two more, lines 150 and 922, which the Ukrainian file words differently or omits.)
- `[ukrainian]` line 31 "я перевіряю в": present tense, no gender. Line 186 "я знаю в": present tense, no gender. Both false positives of the regex.
- Informal pronouns: none flagged. Never-joke stems: none flagged. A manual re-read against the brief's never-joke list found no hunger, war, death, gore, cannibalism or gloating line; the ban, warn and lockdown texts are exact statements only.
- Series references per H2 (checker INFO): Можливості 2 (one sentence: пайок and темпе), Команди 2 (Greetings: темпе; Rules: об'єкт) plus one "наглядача" in Admin; all others 0. Within each sub-section there is at most one reference; the two in "Команди" mirror the English file's layout (tempe in Greetings, facility in Rules). If Task 3 reads "one per section" as one per H2, drop the Rules line or the Greetings line in both files together.

## Choices and deviations

- Code blocks are byte-identical, including English code comments in the structure tree, the bash/ini blocks and the mermaid labels. The plan text allows translating code comments, but the Task 2 instruction says byte-identical, so I kept the English comments. Task 3 can decide whether to translate them (in both files at once) if wanted.
- Image alt texts of the badges are translated (Ліцензія: MIT, Збудовано на gotgbot v2). The nav link to `README.md` is labelled "English".
- H2 headings: Зміст, Можливості, Команди, Архітектура, Стек технологій, Початок роботи, Конфігурація (not "Налаштування", to avoid a clash with the settings menu), Ендпоінти й моніторинг, Структура проєкту, Розробка, Документація, Ліцензія. TOC anchors are the Cyrillic slugs and the checker confirms them.
- `Xenobot-72` is never declined; sentences were built around the nominative. No Russian-only letters in prose; `ы ъ э ё` appears only in a code span.
- Retired-section heading: «Виведено з роботи».

## Errors found in README.md

- None blocking. Observations only: README.md line 15 label "Українська" and line 3 `<h1>` are fine; the checker reports its own REVIEW for spelled numbers (judged in FACTS H5). No factual error spotted while mirroring it.
