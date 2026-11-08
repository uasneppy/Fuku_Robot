//go:build testtools

package modules

import (
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMreqHereEnv is a requirements environment whose bot answers the lookup half of the
// fake (the linked channel and the bot's standing in it), with an owner who opens the page.
type settingsMreqHereEnv struct {
	*memberReqEnv
	state *mreqLookupState
	owner gotgbot.User
}

// newSettingsMreqHereEnv builds the environment and the owner of its group.
func newSettingsMreqHereEnv(t *testing.T) *settingsMreqHereEnv {
	t.Helper()
	env, state := mreqLookupEnv(t)
	return &settingsMreqHereEnv{memberReqEnv: env, state: state, owner: env.newOwner(env.chat)}
}

// seed stores the requirement settings of the group.
func (e *settingsMreqHereEnv) seed(actions [7]int, join, del bool) {
	e.t.Helper()
	settingsMreqSeed(e.t, e.chat.Id, actions, join, del)
}

// channelRuleOn is the stored actions with only the channel rule on, as Warn.
func settingsMreqChannelOn() [7]int {
	return [7]int{0, 0, 0, 0, 0, 0, models.RequirementActionWarn}
}

// press presses a button of the page or of a screen of the menu, in the group menu or the
// private one, and returns the message's text and keyboard afterwards.
func (e *settingsMreqHereEnv) press(private bool, msgID int64, kv ...string) (string, [][]gotgbot.InlineKeyboardButton) {
	e.t.Helper()
	if private {
		e.pressPrivate(e.owner, msgID, e.privateFields(e.chat.Id, kv...))
		return e.lastEdit(e.owner.Id, msgID)
	}
	e.settingsEnv.press(e.owner, msgID, e.groupFields(e.owner, kv...))
	return e.lastEdit(e.chat.Id, msgID)
}

// openMenuOf returns the message ID a press of the given menu kind is made on: the group
// menu's real message, or an arbitrary message of the owner's private chat.
func (e *settingsMreqHereEnv) openMenuOf(private bool) int64 {
	e.t.Helper()
	if private {
		return int64(e.updateID())
	}
	return e.openMenu(e.owner)
}

// settingsMreqOpened is a page that was opened: its message, its text and keyboard, and the
// callback answers the opening press made.
type settingsMreqOpened struct {
	MsgID    int64
	Text     string
	Keyboard [][]gotgbot.InlineKeyboardButton
	// AnswersBefore is the answer count just before the opening press, Answers what it made.
	AnswersBefore, Answers int
}

// openPageOf opens the Requirements page in the given menu kind.
func (e *settingsMreqHereEnv) openPageOf(private bool) settingsMreqOpened {
	e.t.Helper()
	opened := settingsMreqOpened{MsgID: e.openMenuOf(private)}
	opened.AnswersBefore = e.answerCount()
	opened.Text, opened.Keyboard = e.press(private, opened.MsgID, "a", "op", "p", settingsPageMreq)
	opened.Answers = e.answerCount() - opened.AnswersBefore
	return opened
}

// hereLine is the status line of the marker locale: the icon, the key's marker and the rule
// name the line carries.
func settingsMreqHereLine(icon, key string) string {
	return icon + staffMarker(key) + " " + staffMarker("settings_mreq_rule_channel")
}

// settingsMreqLookupCalls counts the lookup requests (getChat of the group and getChatMember
// of a channel's bot) the fake saw so far.
func (e *settingsMreqHereEnv) lookupCalls(channel int64) int {
	return mreqChatCalls(e.memberReqEnv, e.chat.Id) + mreqMemberCalls(e.memberReqEnv, channel, staffTestBotID)
}

func TestSettingsMreqChannelStatus(t *testing.T) {
	menus := []struct {
		name    string
		private bool
	}{{"group menu", false}, {"private menu", true}}

	for _, menu := range menus {
		private := menu.private
		t.Run(menu.name+": the bot is not an administrator of the linked channel", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			channel := mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusMember)
			env.seed(settingsMreqChannelOn(), true, false)

			// The press is answered empty before the status read makes its requests: the answers
			// counted at the LAST lookup request (a private press's own getChat comes before the
			// status read, so the first request is not the status read's).
			var mu sync.Mutex
			answeredAtRead := -1
			env.state.setOnChannel(func(string, int64, int64) {
				mu.Lock()
				defer mu.Unlock()
				answeredAtRead = env.answerCount()
			})
			opened := env.openPageOf(private)
			text, keyboard := opened.Text, opened.Keyboard

			block := settingsMreqSwitchLine("settings_mreq_line_delete", false) + "\n\n" +
				settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin") + "\n\n" +
				staffMarker("settings_mreq_hint_letters") + "\n\n" +
				staffMarker("settings_mreq_hint_blank") + "\n\n" +
				staffMarker("settings_mreq_hint_lookup") + "\n\n" +
				staffMarker("settings_mreq_hint_join")
			if !strings.Contains(text, block) {
				t.Errorf("the page %q lacks the block %q", text, block)
			}
			settingsMreqWantPage(t, text, settingsMreqChannelOn(), true, false)
			if rows := len(keyboard); rows != 6 {
				t.Errorf("the page has %d rows, want 6", rows)
			}
			if strings.Contains(text, strconv.FormatInt(channel, 10)) {
				t.Errorf("the page %q names the linked channel's ID", text)
			}
			mu.Lock()
			read := answeredAtRead
			mu.Unlock()
			if read < 0 {
				t.Fatal("the page made no status request, want one")
			}
			if read-opened.AnswersBefore < 1 {
				t.Errorf("the press was answered %d times before its last status request, want once", read-opened.AnswersBefore)
			}
			if opened.Answers != 1 {
				t.Errorf("the page press was answered %d times, want exactly once", opened.Answers)
			}
		})

		t.Run(menu.name+": an administrator bot draws no block and no blank line", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			env.seed(settingsMreqChannelOn(), true, false)
			text := env.openPageOf(private).Text

			want := settingsMreqSwitchLine("settings_mreq_line_delete", false) + "\n\n" + staffMarker("settings_mreq_hint_letters")
			if !strings.Contains(text, want) {
				t.Errorf("the page %q lacks %q (no block, one blank line)", text, want)
			}
			if strings.Contains(text, "🚫") || strings.Contains(text, "❔") {
				t.Errorf("the page %q has a status line with the bot an administrator", text)
			}
			if !strings.Contains(text, staffMarker("settings_mreq_hint_lookup")) {
				t.Errorf("the page %q lacks the lookup hint", text)
			}
		})

		t.Run(menu.name+": a channel rule that is Off makes no status read and shows no line", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			channel := mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusMember)
			env.seed([7]int{models.RequirementActionWarn, 0, 0, 0, 0, models.RequirementActionMute, 0}, true, true)
			msgID := env.openMenuOf(private)
			// The private press's own getChat of the group is the pipeline's, not the page's.
			base := env.lookupCalls(channel)
			text, _ := env.press(private, msgID, "a", "op", "p", settingsPageMreq)
			wantCalls := base
			if private {
				wantCalls++
			}
			if got := env.lookupCalls(channel); got != wantCalls {
				t.Errorf("lookup requests = %d, want %d (the pipeline's own only)", got, wantCalls)
			}
			if strings.Contains(text, "🚫") || strings.Contains(text, "❔") {
				t.Errorf("the page %q shows a status line with the channel rule Off", text)
			}
		})
	}
}
