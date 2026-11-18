package modules

import (
	"fmt"
	"html"
	"slices"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
)

func listModulesFrom(registry *moduleStruct) []string {
	var modules []string
	for module, enabled := range registry.AbleMap {
		if enabled {
			modules = append(modules, module)
		}
	}
	slices.Sort(modules)
	return modules
}

// helpModuleNameKey is the locale key of a help module's shown name.
func helpModuleNameKey(module string) string {
	return "help_module_name_" + strings.ToLower(module)
}

// helpModuleLabel is the name of a help module in the reader's language: the
// module's help_module_name_ key, or the internal name when the key is missing.
// Callback data and deep links keep the internal name.
func helpModuleLabel(tr *i18n.Translator, module string) string {
	if tr != nil {
		if label, err := tr.GetString(helpModuleNameKey(module)); err == nil && strings.TrimSpace(label) != "" {
			return label
		}
	}
	return module
}

// helpTranslator returns the translator of lang, Ukrainian for an empty code.
func helpTranslator(langCode string) *i18n.Translator {
	if langCode == "" {
		langCode = "uk"
	}
	return i18n.MustNewTranslator(langCode)
}

// initHelpButtonsFrom builds the /help module keyboard in langCode: one button per
// enabled module, labelled with its translated name and sorted by that label in the
// language's own order, three to a row, then Back. The callback data carries the
// internal module name, so a button posted before the labels were translated still works.
func initHelpButtonsFrom(registry *moduleStruct, langCode string) gotgbot.InlineKeyboardMarkup {
	tr := helpTranslator(langCode)
	type entry struct{ module, label string }
	var entries []entry
	for _, module := range listModulesFrom(registry) {
		entries = append(entries, entry{module: module, label: helpModuleLabel(tr, module)})
	}
	col := collate.New(helpCollationTag(langCode), collate.IgnoreCase)
	slices.SortStableFunc(entries, func(a, b entry) int {
		if c := col.CompareString(a.label, b.label); c != 0 {
			return c
		}
		return strings.Compare(a.module, b.module)
	})

	kb := make([]gotgbot.InlineKeyboardButton, 0, len(entries))
	for _, e := range entries {
		kb = append(kb, gotgbot.InlineKeyboardButton{
			Text:         e.label,
			CallbackData: encodeCallbackData("helpq", map[string]string{"m": e.module}),
		})
	}
	zb := slices.Collect(slices.Chunk(kb, 3))
	backText, _ := tr.GetString("helpers_back_button")
	zb = append(zb, []gotgbot.InlineKeyboardButton{{
		Text:         backText,
		CallbackData: encodeCallbackData("helpq", map[string]string{"m": "BackStart"}),
	}})
	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: zb}
}

// helpCollationTag is the collation language of a locale code; an unknown code sorts
// by the root order.
func helpCollationTag(langCode string) language.Tag {
	tag, err := language.Parse(langCode)
	if err != nil {
		return language.Und
	}
	return tag
}

// helpMenuMarkup is the main /help keyboard in the language of the update.
func helpMenuMarkup(ctx *ext.Context) gotgbot.InlineKeyboardMarkup {
	return initHelpButtonsFrom(DefaultHelpRegistry(), lang.GetLanguage(ctx))
}

func getModuleHelpAndKb(module, lang string, registry *moduleStruct) (helpText string, replyMarkup gotgbot.InlineKeyboardMarkup) {
	ModName := module
	if ModName != "" {
		ModName = strings.ToUpper(ModName[:1]) + ModName[1:]
	}
	tr := helpTranslator(lang)
	helpMsg, _ := tr.GetString(fmt.Sprintf("%s_help_msg", strings.ToLower(ModName)))
	headerTemplate, _ := tr.GetString("helpers_module_help_header")
	helpText = renderModuleHelp(headerTemplate, helpModuleTitle(tr, ModName), helpMsg)

	backText, _ := tr.GetString("common_back_arrow")
	homeText, _ := tr.GetString("common_home")
	backBtnSuffix := []gotgbot.InlineKeyboardButton{
		{
			Text:         backText,
			CallbackData: encodeCallbackData("helpq", map[string]string{"m": "Help"}),
		},
		{
			Text:         homeText,
			CallbackData: encodeCallbackData("helpq", map[string]string{"m": "BackStart"}),
		},
	}

	kb := registry.moduleHelpKb(ModName, tr)
	replyMarkup = gotgbot.InlineKeyboardMarkup{
		InlineKeyboard: append(kb, backBtnSuffix),
	}
	return
}

// moduleHelpKb is the module's own help keyboard in the reader's language: the
// builder a module registered with SetModuleHelpFunc, else its static rows.
func (registry *moduleStruct) moduleHelpKb(module string, tr *i18n.Translator) [][]gotgbot.InlineKeyboardButton {
	ableMapMu.RLock()
	build, hasBuild := registry.helpableKbFn[module]
	static := registry.helpableKb[module]
	ableMapMu.RUnlock()
	if hasBuild && build != nil {
		return build(tr)
	}
	return slices.Clone(static)
}

// helpModuleIcons is the icon of each help module, shown before its translated name in the page title.
var helpModuleIcons = map[string]string{
	"Admin": "👮", "AISpam": "🤖", "Antiflood": "🌊", "AntiRaid": "🛡", "Approvals": "👤", "Backup": "📦",
	"Bans": "🔨", "Blacklists": "🔤", "Captcha": "🧩", "Connections": "🔗", "Disabling": "🚫", "Federations": "🏛",
	"Filters": "🔁", "Formatting": "✍️", "Greetings": "🎉", "Languages": "🌐", "Lockdown": "🔒", "Locks": "🔐",
	"LogChannels": "🧾", "Misc": "🧰", "Mutes": "🔇", "Notes": "📝", "Pins": "📌", "Purges": "🧹",
	"Reactions": "👍", "Reports": "📣", "Rules": "📜", "Settings": "⚙️", "Staff": "👥", "Warns": "🚩",
}

// helpModuleTitle is a module page's title text: its icon, when it has one, and its translated name.
func helpModuleTitle(tr *i18n.Translator, module string) string {
	label := helpModuleLabel(tr, module)
	if icon := helpModuleIcons[module]; icon != "" {
		return icon + " " + label
	}
	return label
}

// helpHasOwnTitle reports whether a help text opens with its own title line: one bold line followed by a
// blank line. Such a page gets no generic header, so the title is never shown twice.
func helpHasOwnTitle(helpMsg string) bool {
	first, rest, found := strings.Cut(helpMsg, "\n")
	return found && strings.HasPrefix(first, "<b>") && strings.HasSuffix(first, "</b>") &&
		strings.Count(first, "<b>") == 1 && strings.HasPrefix(rest, "\n")
}

func renderModuleHelp(headerTemplate, title, helpMsg string) string {
	if helpHasOwnTitle(helpMsg) {
		return formatting.ToTelegramHTML(helpMsg)
	}
	return formatting.ToTelegramHTML(fmt.Sprintf(headerTemplate, title)) +
		formatting.ToTelegramHTML(helpMsg)
}

func sendHelpkb(b *gotgbot.Bot, ctx *ext.Context, module string, registry *moduleStruct) (msg *gotgbot.Message, err error) {
	module = strings.ToLower(module)
	if module == "help" {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		helpText := getMainHelp(tr, html.EscapeString(ctx.EffectiveMessage.From.FirstName))
		kb := initHelpButtonsFrom(registry, lang.GetLanguage(ctx))
		_, err = b.SendMessage(
			ctx.EffectiveMessage.Chat.Id,
			helpText,
			&gotgbot.SendMessageOpts{
				ParseMode:   formatting.HTML,
				ReplyMarkup: &kb,
			},
		)
		return
	}
	helpText, replyMarkup, _parsemode := getHelpTextAndMarkup(ctx, module, registry)

	msg, err = b.SendMessage(
		ctx.EffectiveChat.Id,
		helpText,
		&gotgbot.SendMessageOpts{
			ParseMode:   _parsemode,
			ReplyMarkup: replyMarkup,
		},
	)
	return
}

func getModuleNameFromAltName(altName string, registry *moduleStruct) string {
	return findHelpModule(altName, "", registry)
}

// findHelpModule resolves what a user typed after /help to an enabled module: an
// alt name from locales/config.yml, the internal name, or the module's translated
// name in langCode, compared lower-cased and with or without its spaces.
func findHelpModule(typed, langCode string, registry *moduleStruct) string {
	typed = strings.ToLower(strings.TrimSpace(typed))
	if typed == "" {
		return ""
	}
	modules := listModulesFrom(registry)
	for _, modName := range modules {
		if slices.Contains(getAltNamesOfModule(modName), typed) {
			return modName
		}
	}
	if langCode == "" {
		return ""
	}
	tr := helpTranslator(langCode)
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	for _, modName := range modules {
		label := strings.ToLower(helpModuleLabel(tr, modName))
		if label == typed || squash(label) == squash(typed) {
			return modName
		}
	}
	return ""
}

func getAltNamesOfModule(moduleName string) []string {
	tr := i18n.MustNewTranslator("config")
	altNamesFromConfig, _ := tr.GetStringSlice(fmt.Sprintf("alt_names.%s", moduleName))
	return append(altNamesFromConfig, strings.ToLower(moduleName))
}

func getHelpTextAndMarkup(ctx *ext.Context, module string, registry *moduleStruct) (helpText string, kbmarkup gotgbot.InlineKeyboardMarkup, _parsemode string) {
	userOrGroupLanguage := lang.GetLanguage(ctx)
	moduleName := findHelpModule(module, userOrGroupLanguage, registry)

	_parsemode = formatting.HTML
	if moduleName != "" {
		helpText, kbmarkup = getModuleHelpAndKb(moduleName, userOrGroupLanguage, registry)
	} else {
		tr := i18n.MustNewTranslator(userOrGroupLanguage)
		helpText = getMainHelp(tr, html.EscapeString(ctx.EffectiveUser.FirstName))
		kbmarkup = initHelpButtonsFrom(registry, userOrGroupLanguage)
	}

	return
}
