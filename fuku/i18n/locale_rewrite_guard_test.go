//go:build testtools

package i18n

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The rewrite guards of the formatting cleanup (plan of 2026-11-18, Part 4 step 3). The Ukrainian and English texts
// are reworded; these tests hold the facts they carry. TestLocalePlaceholdersMatchOtherLanguages compares the
// placeholders of every Ukrainian and English text with the same key in the six languages the cleanup does not
// reword, and TestHelpPagesKeepTheirCommands holds every command a help page named before the cleanup.

var (
	// localeVerb finds a fmt verb (%s, %d, %[2]s); %% is a literal percent sign.
	localeVerb = regexp.MustCompile(`%%|%(\[[0-9]+\])?[-+#0-9.]*[sdvqfxXt]`)
	// localeNamed finds a named placeholder ({name}).
	localeNamed = regexp.MustCompile(`\{[a-zA-Z_][a-zA-Z0-9_]*\}`)
	// helpCommand finds a /command named in a text.
	helpCommand = regexp.MustCompile(`(^|[^A-Za-z0-9_/:.<])/([a-z][a-z0-9_]*)`)
)

// localePlaceholders is a text's fmt verbs (without their index, sorted) and its named placeholders (sorted).
func localePlaceholders(text string) string {
	var verbs []string
	for _, verb := range localeVerb.FindAllString(text, -1) {
		if verb == "%%" {
			continue
		}
		if i := strings.Index(verb, "]"); i > 0 {
			verb = "%" + verb[i+1:]
		}
		verbs = append(verbs, verb)
	}
	sort.Strings(verbs)
	names := localeNamed.FindAllString(text, -1)
	sort.Strings(names)
	names = slices.Compact(names)
	return strings.Join(verbs, " ") + " | " + strings.Join(names, " ")
}

func TestLocalePlaceholdersMatchOtherLanguages(t *testing.T) {
	others := []string{"es", "fr", "hi", "id", "pt", "ru"}
	data := map[string]map[string]any{}
	for _, lang := range append(slices.Clone(localeStyleLangs), others...) {
		data[lang] = loadStaffLocale(t, lang)
	}
	for _, lang := range localeStyleLangs {
		for key, value := range data[lang] {
			text, ok := value.(string)
			if !ok {
				continue
			}
			want := localePlaceholders(text)
			for _, other := range others {
				otherText, ok := data[other][key].(string)
				if !ok {
					continue
				}
				if got := localePlaceholders(otherText); got != want {
					t.Errorf("%s:%s has placeholders %q, %s has %q", lang, key, want, other, got)
				}
			}
		}
	}
}

func TestLocalePlaceholders(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"Plain.", " | "},
		{"%s and %d", "%d %s | "},
		{"%[2]s by %[1]s", "%s %s | "},
		{"60% of {chat} and 100%% {chat}", " | {chat}"},
	} {
		if got := localePlaceholders(tc.text); got != tc.want {
			t.Errorf("localePlaceholders(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

// helpPageCommands is every /command each Ukrainian and English help page named before the cleanup. A page may name
// more; it may never lose one.
var helpPageCommands = map[string][]string{
	"uk:admin_help_msg":       {"adminlist", "demote", "promote", "setgdesc", "setgpic", "setgtitle", "title"},
	"uk:aispam_help_msg":      {"aispam"},
	"uk:antiflood_help_msg":   {"delflood", "flood", "setflood", "setfloodmode"},
	"uk:antiraid_help_msg":    {"antiraid", "autoantiraid", "autolockdown", "lockdown", "raidactiontime", "raidtime", "unlockdown"},
	"uk:approvals_help_msg":   {"approval", "approve", "approved", "unapprove", "unapproveall"},
	"uk:backup_help_msg":      {"export", "import", "reset"},
	"uk:bans_help_msg":        {"ban", "banme", "dban", "kickme", "restrict", "sban", "skick", "tban", "unban", "unbanall", "unrestrict"},
	"uk:blacklists_help_msg":  {"addblacklist", "blacklistaction", "blacklists", "blaction", "remallbl", "rmblacklist"},
	"uk:captcha_help_msg":     {"captcha", "captchaaction", "captchamaxattempts", "captchamode", "captchatime"},
	"uk:connections_help_msg": {"allowconnect", "connect", "connection", "disconnect", "id", "reconnect"},
	"uk:devs_help_msg":        {"adddev", "addsudo", "chatinfo", "chatlist", "leavechat", "remdev", "remsudo", "stats", "teamusers"},
	"uk:disabling_help_msg":   {"connect", "disable", "disableable", "disabled", "disabledel", "enable"},
	"uk:federations_help_msg": {"chatfed", "delfed", "fban", "fbanlist", "fbanstat", "fedadmins", "feddemote", "fedinfo", "fednotif", "fedpromote", "fedreason", "fedstat", "fedsubs", "funban", "importfbans", "joinfed", "leavefed", "myfeds", "newfed", "quietfed", "renamefed", "setfedlog", "subfed", "unfban", "unsetfedlog", "unsubfed"},
	"uk:filters_help_msg":     {"filter", "filters", "stop", "stopall"},
	"uk:greetings_help_msg":   {"autoapprove", "cleangoodbye", "cleanservice", "cleanwelcome", "goodbye", "resetgoodbye", "resetwelcome", "setgoodbye", "setwelcome", "welcome"},
	"uk:help_help_msg":        {"about", "donate", "help", "start"},
	"uk:languages_help_msg":   {"lang"},
	"uk:lockdown_help_msg":    {"aispam", "autolockdown", "lockdown", "lockdownstatus", "unlockdown"},
	"uk:locks_help_msg":       {"lock", "locks", "locktypes", "unlock"},
	"uk:logchannels_help_msg": {"log", "logcategories", "logchannel", "nolog", "setlog", "unsetlog"},
	"uk:misc_help_msg":        {"github", "id", "info", "ping", "removebotkeyboard", "runs", "stat", "stickerid", "tr", "ud", "wiki", "zombies"},
	"uk:mutes_help_msg":       {"dmute", "mute", "smute", "tmute", "unmute"},
	"uk:notes_help_msg":       {"clear", "clearall", "get", "notes", "privatenotes", "save", "saved"},
	"uk:pins_help_msg":        {"antichannelpin", "cleanlinked", "permapin", "pin", "pinned", "unpin", "unpinall"},
	"uk:purges_help_msg":      {"del", "purge", "purgefrom", "purgeto"},
	"uk:reactions_help_msg":   {"addreaction", "reactions", "removereaction", "resetreactions", "settings"},
	"uk:reports_help_msg":     {"report", "reports"},
	"uk:rules_help_msg":       {"privaterules", "resetrules", "resetrulesbtn", "resetrulesbutton", "rules", "rulesbtn", "setrules"},
	"uk:settings_help_msg":    {"autolockdown", "captcha", "captchaaction", "captchamode", "captchatime", "delflood", "linkstaff", "setflood", "setfloodmode", "setgoodbye", "setrules", "settings", "setwelcome", "timezone", "unlinkstaff", "unlockdown"},
	"uk:staff_help_msg":       {"addfwdblock", "ban", "dban", "dkick", "dmute", "fwdblockaction", "fwdblocklist", "kick", "linkstaff", "lockdownstatus", "mute", "rmfwdblock", "sban", "setstaff", "skick", "smute", "staff", "tban", "tmute", "unban", "unlinkstaff", "unmute", "unsetstaff"},
	"uk:users_help_msg":       {"info"},
	"uk:warns_help_msg":       {"dwarn", "resetallwarns", "resetwarn", "rmwarn", "setwarnlimit", "setwarnmode", "swarn", "warn", "warnings", "warns"},
	"en:admin_help_msg":       {"adminlist", "demote", "promote", "setgdesc", "setgpic", "setgtitle", "title"},
	"en:aispam_help_msg":      {"aispam"},
	"en:antiflood_help_msg":   {"delflood", "flood", "setflood", "setfloodmode"},
	"en:antiraid_help_msg":    {"antiraid", "autoantiraid", "autolockdown", "lockdown", "raidactiontime", "raidtime", "unlockdown"},
	"en:approvals_help_msg":   {"approval", "approve", "approved", "unapprove", "unapproveall"},
	"en:backup_help_msg":      {"export", "import", "reset"},
	"en:bans_help_msg":        {"ban", "banme", "dban", "kickme", "restrict", "sban", "skick", "tban", "unban", "unbanall", "unrestrict"},
	"en:blacklists_help_msg":  {"addblacklist", "blacklistaction", "blacklists", "blaction", "remallbl", "rmblacklist"},
	"en:captcha_help_msg":     {"captcha", "captchaaction", "captchamaxattempts", "captchamode", "captchatime"},
	"en:connections_help_msg": {"allowconnect", "connect", "connection", "disconnect", "id", "reconnect"},
	"en:devs_help_msg":        {"adddev", "addsudo", "chatinfo", "chatlist", "leavechat", "remdev", "remsudo", "stats", "teamusers"},
	"en:disabling_help_msg":   {"connect", "disable", "disableable", "disabled", "disabledel", "enable"},
	"en:federations_help_msg": {"chatfed", "delfed", "fban", "fbanlist", "fbanstat", "fedadmins", "feddemote", "fedinfo", "fednotif", "fedpromote", "fedreason", "fedstat", "fedsubs", "funban", "importfbans", "joinfed", "leavefed", "myfeds", "newfed", "quietfed", "renamefed", "setfedlog", "subfed", "unfban", "unsetfedlog", "unsubfed"},
	"en:filters_help_msg":     {"filter", "filters", "stop", "stopall"},
	"en:greetings_help_msg":   {"autoapprove", "cleangoodbye", "cleanservice", "cleanwelcome", "goodbye", "resetgoodbye", "resetwelcome", "setgoodbye", "setwelcome", "welcome"},
	"en:help_help_msg":        {"about", "donate", "help", "start"},
	"en:languages_help_msg":   {"lang"},
	"en:lockdown_help_msg":    {"aispam", "autolockdown", "lockdown", "lockdownstatus", "unlockdown"},
	"en:locks_help_msg":       {"lock", "locks", "locktypes", "unlock"},
	"en:logchannels_help_msg": {"log", "logcategories", "logchannel", "nolog", "setlog", "unsetlog"},
	"en:misc_help_msg":        {"github", "id", "info", "ping", "removebotkeyboard", "runs", "stat", "stickerid", "tr", "ud", "wiki", "zombies"},
	"en:mutes_help_msg":       {"dmute", "mute", "smute", "tmute", "unmute"},
	"en:notes_help_msg":       {"clear", "clearall", "get", "notes", "privatenotes", "save", "saved"},
	"en:pins_help_msg":        {"antichannelpin", "cleanlinked", "permapin", "pin", "pinned", "unpin", "unpinall"},
	"en:purges_help_msg":      {"del", "purge", "purgefrom", "purgeto"},
	"en:reactions_help_msg":   {"addreaction", "reactions", "removereaction", "resetreactions", "settings"},
	"en:reports_help_msg":     {"report", "reports"},
	"en:rules_help_msg":       {"privaterules", "resetrules", "resetrulesbtn", "resetrulesbutton", "rules", "rulesbtn", "setrules"},
	"en:settings_help_msg":    {"autolockdown", "captcha", "captchaaction", "captchamode", "captchatime", "delflood", "linkstaff", "setflood", "setfloodmode", "setgoodbye", "setrules", "settings", "setwelcome", "timezone", "unlinkstaff", "unlockdown"},
	"en:staff_help_msg":       {"addfwdblock", "ban", "dban", "dkick", "dmute", "fwdblockaction", "fwdblocklist", "kick", "linkstaff", "lockdownstatus", "mute", "rmfwdblock", "sban", "setstaff", "skick", "smute", "staff", "tban", "tmute", "unban", "unlinkstaff", "unmute", "unsetstaff"},
	"en:users_help_msg":       {"info"},
	"en:warns_help_msg":       {"dwarn", "resetallwarns", "resetwarn", "rmwarn", "setwarnlimit", "setwarnmode", "swarn", "warn", "warnings", "warns"},
}

func TestHelpPagesKeepTheirCommands(t *testing.T) {
	for id, want := range helpPageCommands {
		lang, key, _ := strings.Cut(id, ":")
		text, ok := loadStaffLocale(t, lang)[key].(string)
		if !ok {
			t.Errorf("%s no longer exists", id)
			continue
		}
		named := map[string]bool{}
		for _, m := range helpCommand.FindAllStringSubmatch(text, -1) {
			named[m[2]] = true
		}
		for _, command := range want {
			if !named[command] {
				t.Errorf("%s no longer names /%s", id, command)
			}
		}
	}
}
