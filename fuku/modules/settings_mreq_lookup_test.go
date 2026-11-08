//go:build testtools

package modules

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// settingsMreqHereEnv is a requirements environment whose bot answers the lookup half of the
// fake (the linked channel and the bot's standing in it), with an owner who opens the page.
type settingsMreqHereEnv struct {
	*memberReqEnv
	state *mreqLookupState
	owner gotgbot.User
	// basic makes a group menu's messages sit in a chat of type group (a basic group).
	basic bool
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
	e.press0(private, msgID, kv...)
	if private {
		return e.lastEdit(e.owner.Id, msgID)
	}
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

// settingsMreqRowWidths is the number of buttons of each row of a keyboard.
func settingsMreqRowWidths(keyboard [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, 0, len(keyboard))
	for _, row := range keyboard {
		widths = append(widths, len(row))
	}
	return widths
}

// settingsMreqSameInts reports whether two int slices are equal.
func settingsMreqSameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// settingsMreqChannelNow is the Punishment line of the channel rule with the given action.
func settingsMreqChannelNow(action int) string {
	return staffMarker("settings_mreq_now") + " " + settingsMreqIcons[action] + " " + staffMarker(settingsMreqWordKeys[action])
}

// settingsMreqWantChannelScreen fails the test unless text is the channel rule's screen with the
// status slot `slot` (empty for none) and, when offOnly, without the two choice hints.
func settingsMreqWantChannelScreen(t *testing.T, text, slot string, action int, offOnly bool) {
	t.Helper()
	desc := staffMarker("settings_mreq_desc_channel")
	head := desc + "\n"
	if slot != "" {
		head += slot + "\n"
	}
	head += "\n" + settingsMreqChannelNow(action)
	want := head + "\n\n" + staffMarker("settings_mreq_choose_hint") + "\n" + staffMarker("settings_mreq_actions_hint")
	if offOnly {
		want = head
	}
	if !strings.Contains(text, want) {
		t.Errorf("the channel screen %q lacks %q", text, want)
	}
	hasHints := strings.Contains(text, staffMarker("settings_mreq_choose_hint")) || strings.Contains(text, staffMarker("settings_mreq_actions_hint"))
	if offOnly && hasHints {
		t.Errorf("the Off-only channel screen %q still has the choice hints", text)
	}
	if strings.Contains(text, "⚠️") || strings.Contains(text, staffMarker("settings_cannot_open")) {
		t.Errorf("the channel screen %q carries an error or notice", text)
	}
}

func TestSettingsMreqChannelStatusScreen(t *testing.T) {
	type channelCase struct {
		name string
		// setup arranges the fake; private says which menu kind the case runs in.
		setup func(t *testing.T, e *settingsMreqHereEnv, private bool)
		// slot is the key of the slot line, icon its prefix; empty for no line.
		icon, slot string
		offOnly    bool
		action     int
	}
	linked := func(status string) func(*testing.T, *settingsMreqHereEnv, bool) {
		return func(t *testing.T, e *settingsMreqHereEnv, _ bool) { mreqChannel(t, e.state, e.chat.Id, status) }
	}
	botLookup := func(err error, times int) func(*testing.T, *settingsMreqHereEnv, bool) {
		return func(t *testing.T, e *settingsMreqHereEnv, _ bool) {
			channel := mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			for i := 0; i < times; i++ {
				e.state.scriptMember(channel, staffTestBotID, err)
			}
		}
	}
	botAnswer := func(answer mreqChannelMember) func(*testing.T, *settingsMreqHereEnv, bool) {
		return func(t *testing.T, e *settingsMreqHereEnv, _ bool) {
			channel := mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			e.state.setMember(channel, staffTestBotID, answer)
		}
	}
	const notAdmin, noLink, unknown = "settings_mreq_here_notadmin", "settings_mreq_here_nolink", "settings_mreq_here_unknown"
	cases := []channelCase{
		{name: "linked and an administrator bot has no slot line", setup: linked(gotgbot.ChatMemberStatusAdministrator), action: models.RequirementActionWarn},
		{name: "the bot a plain member", setup: linked(gotgbot.ChatMemberStatusMember), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{name: "the bot left", setup: linked(gotgbot.ChatMemberStatusLeft), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{name: "the bot kicked", setup: linked(gotgbot.ChatMemberStatusKicked), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{name: "the bot restricted", setup: linked(gotgbot.ChatMemberStatusRestricted), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{name: "a 400 from the bot's lookup", setup: botLookup(staffFakeError(400, "Bad Request: chat not found"), 1), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{name: "a 403 from the bot's lookup", setup: botLookup(staffFakeError(403, "Forbidden: bot was kicked"), 1), icon: "🚫 ", slot: notAdmin, action: models.RequirementActionWarn},
		{
			name: "no linked channel with a stored Ban is Off alone and the Ban stays visible",
			setup: func(t *testing.T, e *settingsMreqHereEnv, _ bool) {
				e.state.link(e.chat.Id, 0)
			},
			icon: "🚫 ", slot: noLink, offOnly: true, action: models.RequirementActionBan,
		},
		{name: "a 500 from the bot's lookup is unknown", setup: botLookup(staffFakeError(500, "Internal Server Error"), 1), icon: "❔ ", slot: unknown, action: models.RequirementActionWarn},
		{name: "a 429 from the bot's lookup is unknown, never not an administrator", setup: botLookup(staffFake429(1), 2), icon: "❔ ", slot: unknown, action: models.RequirementActionWarn},
		{name: "a null answer from the bot's lookup is unknown", setup: botAnswer(mreqChannelMember{Null: true}), icon: "❔ ", slot: unknown, action: models.RequirementActionWarn},
		{name: "an unrecognised status from the bot's lookup is unknown", setup: botAnswer(mreqChannelMember{Status: "mystery"}), icon: "❔ ", slot: unknown, action: models.RequirementActionWarn},
		{
			name: "a lookup pacer that sheds is unknown",
			setup: func(t *testing.T, e *settingsMreqHereEnv, _ bool) {
				mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				blockKey := memberReqLookupPacerOptions.BlockKey
				mreqSetKey(t, e.memberReqEnv, blockKey, 30*time.Second)
				t.Cleanup(func() { e.redis.Del(blockKey) })
			},
			icon: "❔ ", slot: unknown, action: models.RequirementActionWarn,
		},
		{
			name: "a slow bot lookup times out as unknown",
			setup: func(t *testing.T, e *settingsMreqHereEnv, _ bool) {
				channel := mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				release := e.state.holdKey(mreqChannelKey("getChatMember", channel, staffTestBotID))
				t.Cleanup(release)
				mreqSwap(t, &settingsMreqHereTimeout, 200*time.Millisecond)
			},
			icon: "❔ ", slot: unknown, action: models.RequirementActionWarn,
		},
		{
			name: "a getChat of the group that fails with a 500 is unknown",
			setup: func(t *testing.T, e *settingsMreqHereEnv, private bool) {
				mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				if private {
					t.Skip("a private press's own getChat would meet the scripted error first")
				}
				e.state.scriptChat(e.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
			icon: "❔ ", slot: unknown, action: models.RequirementActionWarn,
		},
		{
			name: "a null getChat of the group is unknown",
			setup: func(t *testing.T, e *settingsMreqHereEnv, private bool) {
				mreqChannel(t, e.state, e.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				if private {
					t.Skip("a private press's own getChat would meet the null answer first")
				}
				e.state.nullGetChat(e.chat.Id)
			},
			icon: "❔ ", slot: unknown, action: models.RequirementActionWarn,
		},
	}
	for _, private := range []bool{false, true} {
		menu := map[bool]string{false: "group menu", true: "private menu"}[private]
		for _, c := range cases {
			t.Run(menu+": "+c.name, func(t *testing.T) {
				env := newSettingsMreqHereEnv(t)
				c.setup(t, env, private)
				actions := settingsMreqChannelOn()
				actions[6] = c.action
				env.seed(actions, false, false)
				msgID := env.openMenuOf(private)
				answers := env.answerCount()
				text, keyboard := env.press(private, msgID, "a", "mo", "t", "ch")

				slot := ""
				if c.slot != "" {
					slot = settingsMreqHereLine(c.icon, c.slot)
				}
				settingsMreqWantChannelScreen(t, text, slot, c.action, c.offOnly)
				wantRows := []int{2, 3, 2}
				if c.offOnly {
					wantRows = []int{1, 2}
				}
				if got := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(got, wantRows) {
					t.Errorf("rows = %v, want %v", got, wantRows)
				}
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press was answered %d times, want once", got)
				}
				if answer, alert := env.lastAnswer(); answer != "" || alert {
					t.Errorf("the press was answered %q (alert %v), want empty", answer, alert)
				}
				if strings.Contains(text, strconv.FormatInt(env.state.links[env.chat.Id], 10)) && env.state.links[env.chat.Id] != 0 {
					t.Errorf("the screen %q names the linked channel's ID", text)
				}
			})
		}
	}
}

// settingsMreqLinkChannel makes a plain settings environment's group linked to a channel whose
// administrator is the bot, so the channel rule's screen keeps its three-row shape (without a
// link it is Off alone, D-05), and returns the channel's ID. The environment's bot answers the
// lookup half of the fake from then on.
func settingsMreqLinkChannel(t *testing.T, env *settingsEnv) int64 {
	t.Helper()
	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(&memberReqEnv{settingsEnv: env}, state)
	return mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
}

// settingsMreqCostEnv builds the environment of the call-count tests.
func settingsMreqCostEnv(t *testing.T) (*settingsMreqHereEnv, int64) {
	t.Helper()
	env := newSettingsMreqHereEnv(t)
	channel := mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
	return env, channel
}

// setRedisValue stores value under key with a TTL in the environment's Redis.
func (e *settingsMreqHereEnv) setRedisValue(key, value string) {
	e.t.Helper()
	if err := e.redis.Set(key, value); err != nil {
		e.t.Fatalf("set %s: %v", key, err)
	}
	e.redis.SetTTL(key, 10*time.Minute)
}

func TestSettingsMreqOpenCost(t *testing.T) {
	// Counts are of the lookup requests (getChat of the group, getChatMember of the bot in the
	// channel) a press made beyond the press pipeline's own: a group menu's pipeline makes neither,
	// a private menu's makes one getChat of the group.
	type draw struct {
		name string
		// prepare arranges the stored settings and the caches before the press.
		prepare func(e *settingsMreqHereEnv, channel int64)
		fields  []string
		chats   int
		members int
	}
	cases := []draw{
		{name: "a page with the channel rule Off", prepare: func(e *settingsMreqHereEnv, _ int64) {
			e.seed([7]int{models.RequirementActionWarn, 0, 0, 0, 0, models.RequirementActionMute, 0}, false, false)
		}, fields: []string{"a", "op", "p", settingsPageMreq}},
		{name: "the photo screen with the channel rule on", prepare: func(e *settingsMreqHereEnv, _ int64) {
			e.seed([7]int{0, 0, 0, 0, 0, models.RequirementActionMute, models.RequirementActionWarn}, false, false)
		}, fields: []string{"a", "mo", "t", "ph"}},
		{name: "a page with a cached none", prepare: func(e *settingsMreqHereEnv, _ int64) {
			e.seed(settingsMreqChannelOn(), false, false)
			e.setRedisValue(memberReqLinkedKey(e.chat.Id), "0")
		}, fields: []string{"a", "op", "p", settingsPageMreq}},
		{name: "a page with a cached link makes the one member lookup", prepare: func(e *settingsMreqHereEnv, channel int64) {
			e.seed(settingsMreqChannelOn(), false, false)
			e.setRedisValue(memberReqLinkedKey(e.chat.Id), strconv.FormatInt(channel, 10))
		}, fields: []string{"a", "op", "p", settingsPageMreq}, members: 1},
		{name: "a page with a cached link and a cached administrator makes no call", prepare: func(e *settingsMreqHereEnv, channel int64) {
			e.seed(settingsMreqChannelOn(), false, false)
			e.setRedisValue(memberReqLinkedKey(e.chat.Id), strconv.FormatInt(channel, 10))
			e.setRedisValue(memberReqBotAdminKey(channel), "1")
		}, fields: []string{"a", "op", "p", settingsPageMreq}},
		{name: "a page on a cache miss makes at most two calls", prepare: func(e *settingsMreqHereEnv, _ int64) {
			e.seed(settingsMreqChannelOn(), false, false)
		}, fields: []string{"a", "op", "p", settingsPageMreq}, chats: 1, members: 1},
		{name: "opening the channel screen reads fresh past a stale cache", prepare: func(e *settingsMreqHereEnv, channel int64) {
			e.seed(settingsMreqChannelOn(), false, false)
			e.setRedisValue(memberReqLinkedKey(e.chat.Id), "0")
			e.setRedisValue(memberReqBotAdminKey(channel), "1")
		}, fields: []string{"a", "mo", "t", "ch"}, chats: 1, members: 1},
	}
	for _, private := range []bool{false, true} {
		menu := map[bool]string{false: "group menu", true: "private menu"}[private]
		for _, c := range cases {
			t.Run(menu+": "+c.name, func(t *testing.T) {
				env, channel := settingsMreqCostEnv(t)
				c.prepare(env, channel)
				msgID := env.openMenuOf(private)
				chats, members := mreqChatCalls(env.memberReqEnv, env.chat.Id), mreqMemberCalls(env.memberReqEnv, channel, staffTestBotID)
				answers := env.answerCount()
				env.press(private, msgID, c.fields...)
				wantChats := c.chats
				if private {
					wantChats++
				}
				if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != wantChats {
					t.Errorf("getChat requests = %d, want %d", got, wantChats)
				}
				if got := mreqMemberCalls(env.memberReqEnv, channel, staffTestBotID) - members; got != c.members {
					t.Errorf("getChatMember requests of the bot = %d, want %d", got, c.members)
				}
				if got := env.answerCount() - answers; got != 1 {
					t.Errorf("the press was answered %d times, want once", got)
				}
				for _, key := range []string{"fuku:memberreq:pace:next", "fuku:memberreq:pace:block"} {
					if env.redis.Exists(key) {
						t.Errorf("a draw wrote the Phase 13 pacer key %s", key)
					}
				}
			})
		}
	}

	t.Run("a fresh channel screen refreshes the link and the administrator caches", func(t *testing.T) {
		env, channel := settingsMreqCostEnv(t)
		env.seed(settingsMreqChannelOn(), false, false)
		env.setRedisValue(memberReqLinkedKey(env.chat.Id), "0")
		env.setRedisValue(memberReqBotAdminKey(channel), "1")
		// The bot was demoted and the group linked since both were cached.
		env.state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
		msgID := env.openMenuOf(false)
		text, _ := env.press(false, msgID, "a", "mo", "t", "ch")
		if !strings.Contains(text, settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin")) {
			t.Errorf("the screen %q does not show the demoted bot", text)
		}
		if got, err := env.redis.Get(memberReqLinkedKey(env.chat.Id)); err != nil || got != strconv.FormatInt(channel, 10) {
			t.Errorf("the link cache = %q (%v), want the refreshed channel", got, err)
		}
		if env.redis.Exists(memberReqBotAdminKey(channel)) {
			t.Error("the stale administrator cache survived a fresh not-administrator answer")
		}
	})

	t.Run("a slow lookup returns about the status bound, drawing the unknown mark", func(t *testing.T) {
		env, channel := settingsMreqCostEnv(t)
		env.seed(settingsMreqChannelOn(), false, false)
		release := env.state.holdKey(mreqChannelKey("getChat", env.chat.Id, 0))
		t.Cleanup(release)
		mreqSwap(t, &settingsMreqHereTimeout, 300*time.Millisecond)
		msgID := env.openMenuOf(false)
		began := time.Now()
		text, keyboard := env.press(false, msgID, "a", "op", "p", settingsPageMreq)
		if took := time.Since(began); took > 3*time.Second {
			t.Errorf("the draw took %s, want about the 300 ms bound", took)
		}
		if !strings.Contains(text, settingsMreqHereLine("❔ ", "settings_mreq_here_unknown")) || len(keyboard) != 6 {
			t.Errorf("the page %q (%d rows) does not draw the unknown line on the full page", text, len(keyboard))
		}
		if mreqMemberCalls(env.memberReqEnv, channel, staffTestBotID) != 0 {
			t.Error("a timed out link read still asked for the bot's status")
		}
	})

	t.Run("Main makes no lookup request", func(t *testing.T) {
		env, channel := settingsMreqCostEnv(t)
		env.seed(settingsMreqChannelOn(), false, false)
		msgID := env.openMenuOf(false)
		before := env.lookupCalls(channel)
		text, _ := env.press(false, msgID, "a", "bk")
		if got := env.lookupCalls(channel); got != before {
			t.Errorf("Main made %d lookup requests, want none", got-before)
		}
		if strings.Contains(text, "🚫") {
			t.Errorf("Main %q carries a cannot-work flag", text)
		}
	})
}

// settingsMreqBasicEnv builds the environment of a basic group: no linked channel in the fake,
// and either a private menu over a getChat that says "group" or a group menu in a chat of type group.
func settingsMreqBasicEnv(t *testing.T, private bool) *settingsMreqHereEnv {
	t.Helper()
	env := newSettingsMreqHereEnv(t)
	if private {
		env.fake.setChatType(env.chat.Id, "group")
	} else {
		env.basic = true
	}
	return env
}

func TestSettingsMreqBasicGroup(t *testing.T) {
	basicLine := "🚫 " + staffMarker("settings_mreq_here_basic")
	for _, private := range []bool{false, true} {
		menu := map[bool]string{false: "group menu", true: "private menu"}[private]
		t.Run(menu+": the page shows the single basic line only while a rule is on", func(t *testing.T) {
			env := settingsMreqBasicEnv(t, private)
			env.seed([7]int{0, 0, 0, 0, 0, models.RequirementActionMute, models.RequirementActionWarn}, true, false)
			msgID := env.openMenuOf(private)
			chats := mreqChatCalls(env.memberReqEnv, env.chat.Id)
			text, keyboard := env.press(private, msgID, "a", "op", "p", settingsPageMreq)

			want := settingsMreqSwitchLine("settings_mreq_line_delete", false) + "\n\n" + basicLine + "\n\n" + staffMarker("settings_mreq_hint_letters")
			if !strings.Contains(text, want) {
				t.Errorf("the page %q lacks %q", text, want)
			}
			if n := strings.Count(text, staffMarker("settings_mreq_here_basic")); n != 1 {
				t.Errorf("the basic line appears %d times, want once (never one per rule)", n)
			}
			if strings.Contains(text, staffMarker("settings_mreq_here_nolink")) {
				t.Errorf("a basic group shows the channel line: %q", text)
			}
			if got := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(got, []int{2, 2, 2, 1, 2, 2}) {
				t.Errorf("rows = %v, want the six rows", got)
			}
			wantChats := 0
			if private {
				wantChats = 1
			}
			if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != wantChats {
				t.Errorf("getChat requests = %d, want %d (the pipeline's own only)", got, wantChats)
			}

			env.seed([7]int{}, false, false)
			text, _ = env.press(private, msgID, "a", "op", "p", settingsPageMreq)
			if !strings.Contains(text, staffMarker("settings_mreq_all_off")) || strings.Contains(text, "🚫") {
				t.Errorf("with every rule Off the page %q must show the all-off line and no basic line", text)
			}
		})

		t.Run(menu+": every rule screen says so and makes no lookup", func(t *testing.T) {
			env := settingsMreqBasicEnv(t, private)
			env.seed([7]int{0, 0, 0, 0, 0, models.RequirementActionMute, models.RequirementActionBan}, false, false)
			msgID := env.openMenuOf(private)
			asked := len(env.fake.callsFor("getChatMember"))
			for i, code := range settingsMreqRuleCodes {
				chats := mreqChatCalls(env.memberReqEnv, env.chat.Id)
				text, keyboard := env.press(private, msgID, "a", "mo", "t", code)
				if !strings.Contains(text, basicLine+"\n\n"+staffMarker("settings_mreq_now")) {
					t.Errorf("rule %s: the screen %q lacks the basic line in its slot", code, text)
				}
				wantRows, wantChats := []int{2, 3, 2}, 0
				if code == "ch" {
					wantRows = []int{1, 2}
				}
				if private {
					wantChats = 1
				}
				if got := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(got, wantRows) {
					t.Errorf("rule %s: rows = %v, want %v", code, got, wantRows)
				}
				hasChoose := strings.Contains(text, staffMarker("settings_mreq_choose_hint"))
				if hasChoose == (code == "ch") {
					t.Errorf("rule %s: choose hint present = %v", code, hasChoose)
				}
				if code == "ph" && !strings.Contains(text, staffMarker("settings_mreq_hint_photo")+"\n"+basicLine) {
					t.Errorf("the photo screen %q lacks its hint before the basic line", text)
				}
				if code == "ch" && !strings.Contains(text, settingsMreqChannelNow(models.RequirementActionBan)) {
					t.Errorf("the stored Ban is not visible on the channel screen %q", text)
				}
				if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != wantChats {
					t.Errorf("rule %s (%d): getChat requests = %d, want %d", code, i, got, wantChats)
				}
			}
			for _, call := range env.fake.callsFor("getChatMember")[asked:] {
				if user := staffParamInt(call.Params, "user_id"); user == staffTestBotID {
					t.Errorf("a basic group's screens asked about the bot in chat %d", staffParamInt(call.Params, "chat_id"))
				}
			}
		})
	}
}

func TestSettingsMreqRedrawKeepsStatus(t *testing.T) {
	notAdminLine := settingsMreqHereLine("🚫 ", "settings_mreq_here_notadmin")
	setup := func(t *testing.T) (*settingsMreqHereEnv, int64) {
		env := newSettingsMreqHereEnv(t)
		mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusMember)
		env.seed(settingsMreqChannelOn(), false, false)
		msgID := env.openMenuOf(false)
		return env, msgID
	}
	// press runs one press and fails unless it is answered once, empty, before the last status
	// request, and the edit is not an error.
	press := func(t *testing.T, env *settingsMreqHereEnv, msgID int64, kv ...string) (string, [][]gotgbot.InlineKeyboardButton) {
		t.Helper()
		var mu sync.Mutex
		atRead := -1
		env.state.setOnChannel(func(string, int64, int64) {
			mu.Lock()
			defer mu.Unlock()
			atRead = env.answerCount()
		})
		before := env.answerCount()
		text, keyboard := env.press(false, msgID, kv...)
		if got := env.answerCount() - before; got != 1 {
			t.Errorf("%v: answered %d times, want once", kv, got)
		}
		mu.Lock()
		defer mu.Unlock()
		if atRead >= 0 && atRead-before < 1 {
			t.Errorf("%v: the status was read before the press was answered", kv)
		}
		return text, keyboard
	}

	t.Run("a switch press keeps the status line", func(t *testing.T) {
		env, msgID := setup(t)
		press(t, env, msgID, "a", "op", "p", settingsPageMreq)
		text, _ := press(t, env, msgID, "a", "tg", "p", settingsPageMreq, "t", "j", "v", "1")
		if !strings.Contains(text, notAdminLine) {
			t.Errorf("the redraw after a switch %q dropped the status line", text)
		}
		if stored, _ := settingsMreqStored(t, env.chat.Id); !stored.CheckOnJoin {
			t.Error("the switch was not written")
		}
	})

	t.Run("a choice, a card Yes and an expired Yes keep the rule's status slot", func(t *testing.T) {
		env, msgID := setup(t)
		text, _ := press(t, env, msgID, "a", "mo", "t", "ch")
		if !strings.Contains(text, notAdminLine) {
			t.Fatalf("the channel screen %q lacks the status line", text)
		}
		// Off writes at once and redraws with the status.
		text, _ = press(t, env, msgID, "a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")
		if !strings.Contains(text, notAdminLine) || !strings.Contains(text, settingsMreqChannelNow(models.RequirementActionOff)) {
			t.Errorf("the redraw after Off %q lost the status slot or the stored value", text)
		}
		// Ban asks first; the card's Yes writes and redraws the screen with the status.
		_, keyboard := press(t, env, msgID, "a", "st", "p", settingsPageMreq, "t", "ch", "v", "3")
		yes := settingsButtonFields(t, keyboard[0][0])
		text, _ = press(t, env, msgID, "a", "my", "t", "ch", "v", "3", "e", yes["e"])
		if !strings.Contains(text, notAdminLine) || !strings.Contains(text, settingsMreqChannelNow(models.RequirementActionBan)) {
			t.Errorf("the redraw after Yes %q lost the status slot or the stored value", text)
		}
		// An expired Yes writes nothing and redraws with the note and the status.
		text, _ = press(t, env, msgID, "a", "my", "t", "ch", "v", "2", "e", "1")
		if !strings.Contains(text, notAdminLine) || !strings.Contains(text, settingsNoticeWarnMarker("settings_card_expired")) {
			t.Errorf("the expired Yes redraw %q lost the status slot or the note", text)
		}
		if stored, _ := settingsMreqStored(t, env.chat.Id); stored.ChannelAction != models.RequirementActionBan {
			t.Errorf("the expired Yes wrote: stored channel action %d, want Ban", stored.ChannelAction)
		}
	})

	t.Run("a failed write redraws with the save-failed notice and the status", func(t *testing.T) {
		env, msgID := setup(t)
		settingsMreqSeams(t)
		memberReqWrites.Action = func(context.Context, int64, memberreq.Rule, int) error { return errors.New("write failed") }
		text, _ := press(t, env, msgID, "a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")
		if !strings.Contains(text, notAdminLine) || !strings.Contains(text, settingsNoticeWarnMarker("common_settings_save_failed")) {
			t.Errorf("the failed-write redraw %q lacks the status line or the notice", text)
		}
		if stored, _ := settingsMreqStored(t, env.chat.Id); stored.ChannelAction != models.RequirementActionWarn {
			t.Errorf("a failed write changed the stored value to %d", stored.ChannelAction)
		}
	})

	t.Run("a redraw reads from the caches and never makes more than two requests", func(t *testing.T) {
		env, msgID := setup(t)
		channel := env.state.links[env.chat.Id]
		press(t, env, msgID, "a", "mo", "t", "ch")
		before := env.lookupCalls(channel)
		press(t, env, msgID, "a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")
		if got := env.lookupCalls(channel) - before; got > 2 {
			t.Errorf("a redraw made %d lookup requests, want at most 2 (the link comes from the cache)", got)
		}
		if got := mreqChatCalls(env.memberReqEnv, env.chat.Id); got != 1 {
			t.Errorf("getChat requests in all = %d, want the one fresh read of the opening", got)
		}
	})
}

// settingsMreqRefusalMenus runs body once in a group menu and once in a private one.
func settingsMreqRefusalMenus(t *testing.T, body func(t *testing.T, private bool)) {
	t.Helper()
	for _, private := range []bool{false, true} {
		name := map[bool]string{false: "group menu", true: "private menu"}[private]
		t.Run(name, func(t *testing.T) { body(t, private) })
	}
}

// settingsMreqPressed is what one press of a refusal test left behind: the answers and edits it
// made and the last answer.
type settingsMreqPressed struct {
	Answers, Edits int
	Answer         string
	Alert          bool
}

// strictPress presses a button of the opened menu and reports what it answered and edited. The
// edits are counted on the message the press was made on.
func (e *settingsMreqHereEnv) strictPress(private bool, msgID int64, kv ...string) settingsMreqPressed {
	e.t.Helper()
	return e.measure(private, msgID, func() { e.press0(private, msgID, kv...) })
}

// measure runs one press and reports the answers and edits it made.
func (e *settingsMreqHereEnv) measure(private bool, msgID int64, press func()) settingsMreqPressed {
	e.t.Helper()
	chatID := e.chat.Id
	if private {
		chatID = e.owner.Id
	}
	answers, edits := e.answerCount(), len(e.edits(chatID, msgID))
	press()
	out := settingsMreqPressed{Answers: e.answerCount() - answers, Edits: len(e.edits(chatID, msgID)) - edits}
	if out.Answers > 0 {
		out.Answer, out.Alert = e.lastAnswer()
	}
	return out
}

// press0 presses like press but does not read the edit back, so a press that edits nothing is
// not a test failure.
func (e *settingsMreqHereEnv) press0(private bool, msgID int64, kv ...string) {
	e.t.Helper()
	var fields map[string]string
	if private {
		fields = e.privateFields(e.chat.Id, kv...)
	} else {
		fields = e.groupFields(e.owner, kv...)
	}
	data := encodeCallbackData(settingsCallbackNamespace, fields)
	if data == "" {
		e.t.Fatalf("settings callback data %v did not encode", kv)
	}
	e.pressData(private, msgID, data)
}

// pressData presses a button carrying data, as the owner, in the menu of the given kind.
func (e *settingsMreqHereEnv) pressData(private bool, msgID int64, data string) {
	e.t.Helper()
	chat := e.chat
	if e.basic {
		chat.Type = "group"
	}
	if private {
		chat = gotgbot.Chat{Id: e.owner.Id, Type: "private", FirstName: e.owner.FirstName}
	}
	e.pressRaw(chat, e.owner, msgID, data)
}

// strictPressData is strictPress for a button whose data is already encoded (a card's Yes).
func (e *settingsMreqHereEnv) strictPressData(private bool, msgID int64, data string) settingsMreqPressed {
	e.t.Helper()
	return e.measure(private, msgID, func() { e.pressData(private, msgID, data) })
}

// settingsMreqChannelStrictKV is the st press of the channel rule with the absolute value v.
func settingsMreqChannelStrictKV(v int) []string {
	return []string{"a", "st", "p", settingsPageMreq, "t", "ch", "v", strconv.Itoa(v)}
}

// settingsMreqWantRefusedNoLink fails the test unless the press was refused as "no linked channel":
// one alert with the status line's own key, one edit that is the Off-only channel screen, and
// nothing written.
func settingsMreqWantRefusedNoLink(t *testing.T, env *settingsMreqHereEnv, private bool, msgID int64, got settingsMreqPressed, stored int) {
	t.Helper()
	if got.Answers != 1 || !got.Alert || got.Answer != settingsMreqHereLine("", "settings_mreq_here_nolink") {
		t.Errorf("answers = %d, last = %q (alert %v), want one alert of settings_mreq_here_nolink", got.Answers, got.Answer, got.Alert)
	}
	if got.Edits != 1 {
		t.Fatalf("the press edited the message %d times, want once (the Off-only screen)", got.Edits)
	}
	chatID := env.chat.Id
	if private {
		chatID = env.owner.Id
	}
	text, keyboard := env.lastEdit(chatID, msgID)
	settingsMreqWantChannelScreen(t, text, settingsMreqHereLine("🚫 ", "settings_mreq_here_nolink"), stored, true)
	if strings.Contains(text, staffMarker("settings_mreq_confirm")) {
		t.Errorf("the refusal drew a card: %q", text)
	}
	if rows := settingsMreqRowWidths(keyboard); !settingsMreqSameInts(rows, []int{1, 2}) {
		t.Errorf("rows = %v, want the Off-only [1 2]", rows)
	}
}

// settingsMreqWantRefusedUnknown fails the test unless the press was refused as "could not
// check": one alert of mreq_fixed_check_failed and no edit.
func settingsMreqWantRefusedUnknown(t *testing.T, got settingsMreqPressed) {
	t.Helper()
	if got.Answers != 1 || !got.Alert || got.Answer != staffMarker("mreq_fixed_check_failed") {
		t.Errorf("answers = %d, last = %q (alert %v), want one alert of mreq_fixed_check_failed", got.Answers, got.Answer, got.Alert)
	}
	if got.Edits != 0 {
		t.Errorf("the refused press edited the message %d times, want none", got.Edits)
	}
}

func TestSettingsMreqChannelRefusal(t *testing.T) {
	settingsMreqRefusalMenus(t, func(t *testing.T, private bool) {
		t.Run("Warn with no linked channel is refused and the Off-only screen is redrawn", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			env.state.link(env.chat.Id, 0)
			env.seed([7]int{}, false, false)
			msgID := env.openMenuOf(private)
			got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
			settingsMreqWantRefusedNoLink(t, env, private, msgID, got, 0)
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
				t.Errorf("a refused press wrote %v", stored)
			}
		})

		t.Run("Warn with a linked channel writes", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			env.seed([7]int{}, false, false)
			msgID := env.openMenuOf(private)
			got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
			if got.Answers != 1 || got.Edits != 1 {
				t.Fatalf("answers = %d, edits = %d, want one each", got.Answers, got.Edits)
			}
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{6: models.RequirementActionWarn} {
				t.Errorf("stored %v, want only the channel rule on Warn", stored)
			}
			chatID := env.chat.Id
			if private {
				chatID = env.owner.Id
			}
			text, _ := env.lastEdit(chatID, msgID)
			settingsMreqWantChannelScreen(t, text, "", models.RequirementActionWarn, false)
		})

		t.Run("Warn with an unreadable link answers could-not-check and changes nothing", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			// A shed pacer is the unreadable answer both menu kinds can script: a private press's own
			// getChat is not behind the lookup pacer, so it still passes.
			blockKey := memberReqLookupPacerOptions.BlockKey
			mreqSetKey(t, env.memberReqEnv, blockKey, 30*time.Second)
			t.Cleanup(func() { env.redis.Del(blockKey) })
			env.seed([7]int{}, false, false)
			msgID := env.openMenuOf(private)
			got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
			settingsMreqWantRefusedUnknown(t, got)
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
				t.Errorf("an unreadable link wrote %v", stored)
			}
		})

		t.Run("Kick, Ban and Mute with no linked channel are refused with no card", func(t *testing.T) {
			for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
				env := newSettingsMreqHereEnv(t)
				env.state.link(env.chat.Id, 0)
				env.seed([7]int{}, false, false)
				msgID := env.openMenuOf(private)
				got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(action)...)
				settingsMreqWantRefusedNoLink(t, env, private, msgID, got, 0)
				if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
					t.Errorf("a refused %s wrote %v", settingsMreqWordKeys[action], stored)
				}
			}
		})

		for _, botStatus := range []string{gotgbot.ChatMemberStatusAdministrator, gotgbot.ChatMemberStatusMember} {
			t.Run("with a linked channel and the bot "+botStatus+" Kick, Ban and Mute ask first and Yes writes", func(t *testing.T) {
				for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
					env := newSettingsMreqHereEnv(t)
					mreqChannel(t, env.state, env.chat.Id, botStatus)
					env.seed([7]int{}, false, false)
					msgID := env.openMenuOf(private)
					got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(action)...)
					if got.Answers != 1 || got.Edits != 1 || got.Alert {
						t.Fatalf("%s: %+v, want one empty answer and one edit", settingsMreqWordKeys[action], got)
					}
					text, keyboard := env.lastEdit(settingsMreqMenuChat(env, private), msgID)
					yes, _, _ := settingsMreqWantCard(t, text, keyboard, 6, action)
					if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
						t.Fatalf("drawing the card wrote %v", stored)
					}
					got = env.strictPressData(private, msgID, yes.CallbackData)
					if got.Answers != 1 || got.Alert || got.Edits != 1 {
						t.Errorf("Yes: %+v, want one empty answer and one edit", got)
					}
					if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{6: action} {
						t.Errorf("after Yes stored %v, want the channel rule on %s", stored, settingsMreqWordKeys[action])
					}
				}
			})
		}

		t.Run("the card's Yes checks the link again", func(t *testing.T) {
			for name, lose := range map[string]func(env *settingsMreqHereEnv){
				"the link was lost": func(env *settingsMreqHereEnv) { env.state.link(env.chat.Id, 0) },
				"the pacer sheds": func(env *settingsMreqHereEnv) {
					blockKey := memberReqLookupPacerOptions.BlockKey
					mreqSetKey(env.t, env.memberReqEnv, blockKey, 30*time.Second)
					env.t.Cleanup(func() { env.redis.Del(blockKey) })
				},
			} {
				env := newSettingsMreqHereEnv(t)
				mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				env.seed([7]int{}, false, false)
				msgID := env.openMenuOf(private)
				env.press0(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionBan)...)
				text, keyboard := env.lastEdit(settingsMreqMenuChat(env, private), msgID)
				yes, _, _ := settingsMreqWantCard(t, text, keyboard, 6, models.RequirementActionBan)

				lose(env)
				got := env.strictPressData(private, msgID, yes.CallbackData)
				if name == "the link was lost" {
					settingsMreqWantRefusedNoLink(t, env, private, msgID, got, 0)
				} else {
					settingsMreqWantRefusedUnknown(t, got)
				}
				if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
					t.Errorf("%s: a refused Yes wrote %v", name, stored)
				}
			}
		})

		t.Run("an unreadable link refuses Kick, Ban and Mute and edits nothing", func(t *testing.T) {
			kinds := map[string]func(env *settingsMreqHereEnv){
				"a shed pacer": func(env *settingsMreqHereEnv) {
					blockKey := memberReqLookupPacerOptions.BlockKey
					mreqSetKey(env.t, env.memberReqEnv, blockKey, 30*time.Second)
					env.t.Cleanup(func() { env.redis.Del(blockKey) })
				},
			}
			if !private {
				// A private press's own getChat of the group would meet these first.
				kinds["a 500"] = func(env *settingsMreqHereEnv) {
					env.state.scriptChat(env.chat.Id, staffFakeError(500, "Internal Server Error"))
				}
				kinds["a null answer"] = func(env *settingsMreqHereEnv) { env.state.nullGetChat(env.chat.Id) }
				kinds["a timeout"] = func(env *settingsMreqHereEnv) {
					release := env.state.holdKey(mreqChannelKey("getChat", env.chat.Id, 0))
					env.t.Cleanup(release)
					mreqSwap(env.t, &settingsMreqHereTimeout, 200*time.Millisecond)
				}
			}
			for name, arrange := range kinds {
				for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
					env := newSettingsMreqHereEnv(t)
					mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
					env.seed([7]int{}, false, false)
					msgID := env.openMenuOf(private)
					arrange(env)
					got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(action)...)
					settingsMreqWantRefusedUnknown(t, got)
					if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
						t.Errorf("%s, %s: wrote %v", name, settingsMreqWordKeys[action], stored)
					}
				}
			}
		})

		t.Run("Off never checks the link", func(t *testing.T) {
			for name, arrange := range map[string]func(env *settingsMreqHereEnv){
				"no linked channel": func(env *settingsMreqHereEnv) { env.state.link(env.chat.Id, 0) },
				"a shed pacer": func(env *settingsMreqHereEnv) {
					blockKey := memberReqLookupPacerOptions.BlockKey
					mreqSetKey(env.t, env.memberReqEnv, blockKey, 30*time.Second)
					env.t.Cleanup(func() { env.redis.Del(blockKey) })
				},
			} {
				env := newSettingsMreqHereEnv(t)
				mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
				env.seed([7]int{6: models.RequirementActionBan}, false, false)
				msgID := env.openMenuOf(private)
				arrange(env)
				got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionOff)...)
				if got.Answers != 1 || got.Alert || got.Answer != "" || got.Edits != 1 {
					t.Errorf("%s: Off answered %+v, want one empty answer and one edit", name, got)
				}
				if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
					t.Errorf("%s: Off left %v stored, want every rule off", name, stored)
				}
			}
		})

		t.Run("a press of the stored value makes no link check", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			env.state.link(env.chat.Id, 0)
			env.seed([7]int{6: models.RequirementActionBan}, false, false)
			msgID := env.openMenuOf(private)
			got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionBan)...)
			if got.Answers != 1 || got.Alert || got.Answer != "" || got.Edits != 1 {
				t.Errorf("a press of the stored Ban answered %+v, want one empty answer and one edit (no refusal alert)", got)
			}
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{6: models.RequirementActionBan} {
				t.Errorf("stored %v, want the Ban untouched", stored)
			}
		})

		t.Run("an expired Yes redraws with the expiry note and makes no link check", func(t *testing.T) {
			env := newSettingsMreqHereEnv(t)
			mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
			env.seed([7]int{}, false, false)
			msgID := env.openMenuOf(private)
			saved := settingsCardLifetime
			settingsCardLifetime = -2 * time.Minute
			t.Cleanup(func() { settingsCardLifetime = saved })
			env.press0(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionMute)...)
			text, keyboard := env.lastEdit(settingsMreqMenuChat(env, private), msgID)
			yes, _, _ := settingsMreqWantCard(t, text, keyboard, 6, models.RequirementActionMute)
			settingsCardLifetime = saved

			// Were the link check made, a lost link would turn the expiry note into a refusal.
			env.state.link(env.chat.Id, 0)
			got := env.strictPressData(private, msgID, yes.CallbackData)
			if got.Answers != 1 || got.Alert || got.Answer != "" || got.Edits != 1 {
				t.Errorf("an expired Yes answered %+v, want one empty answer and one edit", got)
			}
			text, _ = env.lastEdit(settingsMreqMenuChat(env, private), msgID)
			if want := settingsNoticeWarnMarker("settings_card_expired"); !strings.Contains(text, want) {
				t.Errorf("the expired Yes redraw %q lacks the notice %q", text, want)
			}
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
				t.Errorf("an expired Yes wrote %v", stored)
			}
		})
	})

	t.Run("a presser demoted before the Yes is refused before the link check", func(t *testing.T) {
		env := newSettingsMreqHereEnv(t)
		mreqChannel(t, env.state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
		env.seed([7]int{}, false, false)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		msgID := env.openMenu(helper)
		env.memberReqEnv.settingsEnv.press(helper, msgID, env.groupFields(helper, settingsMreqChannelStrictKV(models.RequirementActionBan)...))
		text, keyboard := env.lastEdit(env.chat.Id, msgID)
		yes, _, _ := settingsMreqWantCard(t, text, keyboard, 6, models.RequirementActionBan)

		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: false})
		chats, answers, edits := mreqChatCalls(env.memberReqEnv, env.chat.Id), env.answerCount(), len(env.edits(env.chat.Id, msgID))
		env.pressRaw(env.chat, helper, msgID, yes.CallbackData)
		if answer, alert := env.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("settings_no_restrict")) {
			t.Errorf("answer = %q (alert %v), want the settings_no_restrict alert", answer, alert)
		}
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("Yes answered %d times, want once", got)
		}
		if got := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; got != 0 {
			t.Errorf("a refused Yes made %d getChat requests of the group, want none (the right comes first)", got)
		}
		if got := len(env.edits(env.chat.Id, msgID)); got != edits {
			t.Error("a refused Yes edited the card")
		}
		if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
			t.Errorf("a refused Yes wrote %v", stored)
		}
	})

	t.Run("a basic group is refused as having no linked channel with no lookup", func(t *testing.T) {
		for _, private := range []bool{false, true} {
			env := settingsMreqBasicEnv(t, private)
			env.seed([7]int{}, false, false)
			msgID := env.openMenuOf(private)
			chats := mreqChatCalls(env.memberReqEnv, env.chat.Id)
			got := env.strictPress(private, msgID, settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
			if got.Answers != 1 || !got.Alert || got.Answer != settingsMreqHereLine("", "settings_mreq_here_nolink") || got.Edits != 1 {
				t.Errorf("private=%v: %+v, want one nolink alert and one edit", private, got)
			}
			wantChats := 0
			if private {
				wantChats = 1
			}
			if n := mreqChatCalls(env.memberReqEnv, env.chat.Id) - chats; n != wantChats {
				t.Errorf("private=%v: %d getChat requests, want %d (the pipeline's own only)", private, n, wantChats)
			}
			text, keyboard := env.lastEdit(settingsMreqMenuChat(env, private), msgID)
			if !strings.Contains(text, settingsMreqBasicLineMarker()) || settingsMreqRowWidths(keyboard)[0] != 1 {
				t.Errorf("private=%v: the redraw %q is not the Off-only basic screen", private, text)
			}
			if stored := settingsMreqStoredActions(t, env.chat.Id); stored != [7]int{} {
				t.Errorf("private=%v: wrote %v", private, stored)
			}
		}
	})
}

// settingsMreqMenuChat is the chat the menu message of the given kind sits in.
func settingsMreqMenuChat(env *settingsMreqHereEnv, private bool) int64 {
	if private {
		return env.owner.Id
	}
	return env.chat.Id
}

// settingsMreqBasicLineMarker is the basic-group status line of the marker locale.
func settingsMreqBasicLineMarker() string {
	return "🚫 " + staffMarker("settings_mreq_here_basic")
}
