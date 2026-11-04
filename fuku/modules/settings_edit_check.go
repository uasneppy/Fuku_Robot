package modules

import (
	"strings"

	tgmd2html "github.com/PaulSonOfLars/gotg_md2html"
	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// This file is the pure check of a message the owner typed for one part of a Welcome, Goodbye
// or Rules message. It converts the message exactly as the commands convert theirs, and it
// refuses, with the reason, whatever Telegram would refuse when the message is sent at a join.
// It sends nothing, writes nothing and reads no database: the typed-reply door calls it between
// its authority checks and its claim, so a refusal never consumes the wait.

// settingsEditCheckInput is what the check judges: the page, the part the owner is setting, the
// message the owner sent, the stored message as the page shows it, the owner (the first of the
// three members the dry run fills the placeholders for), the group's title and the language of
// the commands' own refusal text.
type settingsEditCheckInput struct {
	Def        settingsEditPageDef
	Part       string
	Msg        *gotgbot.Message
	State      settingsEditState
	Owner      gotgbot.User
	GroupTitle string
	Language   string
}

// settingsEditVerdict is the answer of the check. When OK is true the fields of the judged part
// carry what to store: Text the stored HTML of a text part. When OK is false RefusalKey and
// RefusalParams name the locale line to show, except that RefusalText is the finished text of a
// refusal the commands already send (the over-long text), which wins when it is not empty.
type settingsEditVerdict struct {
	OK            bool
	Text          string
	RefusalKey    string
	RefusalParams i18n.TranslationParams
	RefusalText   string
}

// settingsEditRefusal is the verdict that refuses with a locale key and its parameters.
func settingsEditRefusal(key string, params i18n.TranslationParams) settingsEditVerdict {
	return settingsEditVerdict{RefusalKey: key, RefusalParams: params}
}

// settingsEditConvert runs one conversion of the owner's message and recovers from a panic of
// the converter (an entity whose offset is past the end of the text), answering ok false. It
// logs no text.
func settingsEditConvert(convert func() string) (out string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Warn("[SettingsEdit][Convert]: a typed message could not be converted")
			out, ok = "", false
		}
	}()
	return convert(), true
}

// settingsEditCheck judges the message of in for the part of in. It is pure: no Telegram call,
// no database read, no write.
func settingsEditCheck(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg == nil {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	if in.Part == settingsEditPartText {
		if in.Def.Kind == settingsEditRules {
			return settingsEditCheckRulesText(in)
		}
		return settingsEditCheckGreetingText(in)
	}
	return settingsEditRefusal("settings_edit_refuse_need_text", nil)
}

// settingsEditCheckGreetingText converts a Welcome or Goodbye text as /setwelcome and
// /setgoodbye do (the buttons parser over the message's markdown, trimmed as preFixes trims).
func settingsEditCheckGreetingText(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	stored, ok := settingsEditConvert(func() string {
		text, _ := tgmd2html.MD2HTMLButtonsV2(in.Msg.OriginalMDV2())
		return text
	})
	if !ok {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	return settingsEditVerdict{OK: true, Text: strings.Trim(stored, "\n\t\r ")}
}

// settingsEditCheckRulesText converts a Rules text as /setrules does.
func settingsEditCheckRulesText(in settingsEditCheckInput) settingsEditVerdict {
	if in.Msg.Text == "" {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	stored, ok := settingsEditConvert(func() string {
		return tgmd2html.MD2HTMLV2(in.Msg.OriginalMDV2())
	})
	if !ok {
		return settingsEditRefusal("settings_edit_refuse_need_text", nil)
	}
	return settingsEditVerdict{OK: true, Text: stored}
}
