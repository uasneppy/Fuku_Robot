package modules

import (
	"context"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"

	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// The old raid mode is retired (LOCK-15, D-16). It used to temp-ban every joiner while
// a raid was on, from a join handler at group -5 and an expiry poller; both are gone,
// replaced by /lockdown and the automatic triggers of /autolockdown. What stays is the
// AntiRaid module name and help page, commands that point admins to the new tools, and
// /autoantiraid, kept as an alias of the join-surge threshold.

var antiRaidModule = moduleStruct{moduleName: "AntiRaid"}

// antiraidCallbackAnswerMax is Telegram's limit for the text of a callback answer, in
// characters.
const antiraidCallbackAnswerMax = 200

// antiraidTagPattern matches an HTML tag. A callback answer is plain text, so the
// pointer's markup is removed before it is shown.
var antiraidTagPattern = regexp.MustCompile(`<[^>]*>`)

// antiraidRetiredDesc registers /antiraid and the two old timing commands as one
// pointer. It has no checks: the reply is static text that names no group data.
var antiraidRetiredDesc = helpers.CommandDescriptor{
	Name:        "antiraid",
	Aliases:     []string{"raidtime", "raidactiontime"},
	Disableable: true,
}

// autoantiraidDesc is /autoantiraid with exactly /autolockdown's checks: a live
// getChatMember of the sender, never the admin cache (D-10). The old command was open
// to any cached admin, so this is stricter on purpose.
var autoantiraidDesc = helpers.CommandDescriptor{
	Name:           "autoantiraid",
	RequiredChecks: []helpers.CheckFunc{requireLockdownGroup(), requireAutoLockdownAuthority()},
}

// antiraidRetired answers /antiraid, /raidtime and /raidactiontime with the pointer to
// /lockdown and /autolockdown, and changes nothing.
func (m moduleStruct) antiraidRetired(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if msg == nil {
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	replyLockdown(b, msg, lockdownText(tr, "lockdown_antiraid_retired"))
	return ext.EndGroups
}

// autoantiraid is the /autoantiraid alias. With words it runs /autolockdown's command
// body with "joins" in front, so "N" sets the join-surge threshold and turns it on,
// "off" turns the surge off and "on" turns it back on; alone it shows the triggers. The
// range and the grammar are /autolockdown's.
func (m moduleStruct) autoantiraid(b *gotgbot.Bot, ctx *ext.Context) error {
	args := lockdownCommandArgs(ctx)
	if len(args) > 0 {
		args = append([]string{autoTriggerJoins}, args...)
	}
	return autolockdownApply(b, ctx, args)
}

// antiraidRetiredCallback answers a press on an old antiraid button once with the
// pointer and changes nothing. An update with no callback query is not ours.
func (m moduleStruct) antiraidRetiredCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok || query == nil {
		return ext.ContinueGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text := antiraidTagPattern.ReplaceAllString(lockdownText(tr, "lockdown_antiraid_retired"), "")
	if utf8.RuneCountInString(text) > antiraidCallbackAnswerMax {
		text = string([]rune(text)[:antiraidCallbackAnswerMax])
	}
	_, _ = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: text, ShowAlert: true})
	return ext.EndGroups
}

// antiraidSweepOnce makes the startup sweep of the retired keys run once per process.
var antiraidSweepOnce sync.Once

// sweepRetiredAntiRaidKeys is compile scaffolding for its test: it removes nothing yet.
func sweepRetiredAntiRaidKeys(ctx context.Context) (int, error) {
	return 0, nil
}

// LoadAntiRaid registers the pointer commands, the /autoantiraid alias and the answer
// to an old antiraid button.
func LoadAntiRaid(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(antiRaidModule.moduleName, true)

	helpers.WrapCommand(dispatcher, antiraidRetiredDesc, pipelineHandler(antiRaidModule.antiraidRetired))
	helpers.WrapCommand(dispatcher, autoantiraidDesc, pipelineHandler(antiRaidModule.autoantiraid))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("antiraid"), antiRaidModule.antiraidRetiredCallback))
}

func init() {
	RegisterLegacyModule("AntiRaid", 230, LoadAntiRaid)
	// An anonymous admin proves who they are first; the proof re-enters here as the
	// person who tapped, and the same live check runs for them.
	RegisterAnonymousAdminHandler("autoantiraid", anonPipelineHandler(autoantiraidDesc, antiRaidModule.autoantiraid))
}
