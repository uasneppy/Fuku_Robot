//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
)

// settingsDelEnv is the toolkit of the Deletion page tests: the settings environment with the
// deletion pacer made fast, the bot's own getChatMember scriptable per chat, the deletion rows,
// greetings rows and staff rows of its chat cleaned up, and an owner (the live creator of the group)
// who opens the menu. Every ID is fresh, so a run in the shared CI database leaves nothing behind.
type settingsDelEnv struct {
	*settingsEnv
	script *deletionScriptClient
	owner  gotgbot.User
}

// newSettingsDelEnv builds the environment; the bot is an administrator with the delete right.
func newSettingsDelEnv(t *testing.T) *settingsDelEnv {
	t.Helper()
	env := newSettingsEnv(t)
	withFastDeletionPacer(t)
	deletionCleanup(t, env.chat.Id)
	staffCleanup(t, env.chat.Id)
	LoadGreetings(env.dispatcher)
	script := &deletionScriptClient{BotClient: env.fake, fake: env.fake}
	env.bot.BotClient = script
	env.fake.setBotMember(env.chat.Id, lockdownBotAdminJSON(false, true, false))
	owner := env.newOwner(env.chat)
	t.Cleanup(func() {
		ids := []int64{owner.Id, env.admin.Id}
		db.DB.Where("user_id IN ?", ids).Delete(&models.User{})
		for _, id := range ids {
			dbcache.DeleteCache(dbcache.CacheKey("user", id))
		}
		dbcache.ResetLocalForTest()
	})
	return &settingsDelEnv{settingsEnv: env, script: script, owner: owner}
}

// settingsDelMenu is one open settings message of the env's group: a group menu, or a private one.
type settingsDelMenu struct {
	env     *settingsDelEnv
	msgID   int64
	chat    gotgbot.Chat
	private bool
	// actor is who presses: the owner unless the menu was opened by someone else.
	actor gotgbot.User
	// mainText and mainKeyboard are the Main page the menu opened with.
	mainText     string
	mainKeyboard [][]gotgbot.InlineKeyboardButton
}

// groupMenu opens /settings in the group as the owner and returns the Main message.
func (e *settingsDelEnv) groupMenu() *settingsDelMenu {
	e.t.Helper()
	msgID := e.openMenu(e.owner)
	text, keyboard := e.lastEdit(e.chat.Id, msgID)
	return &settingsDelMenu{env: e, msgID: msgID, chat: e.chat, actor: e.owner, mainText: text, mainKeyboard: keyboard}
}

// groupMenuOf opens /settings in the group as user and returns the Main message; user presses it.
func (e *settingsDelEnv) groupMenuOf(user gotgbot.User) *settingsDelMenu {
	e.t.Helper()
	msgID := e.openMenu(user)
	text, keyboard := e.lastEdit(e.chat.Id, msgID)
	return &settingsDelMenu{env: e, msgID: msgID, chat: e.chat, actor: user, mainText: text, mainKeyboard: keyboard}
}

// privateMenu opens the settings of the group in the owner's private chat and returns the Main message.
func (e *settingsDelEnv) privateMenu() *settingsDelMenu {
	e.t.Helper()
	e.sendPrivate(e.owner, fmt.Sprintf("/start settings_%d", e.chat.Id))
	sent := settingsPrivateOnlySent(e.t, e.settingsEnv, e.owner)
	return &settingsDelMenu{
		env: e, msgID: sent.MessageID, chat: settingsPrivateChat(e.owner), private: true, actor: e.owner,
		mainText: fmt.Sprint(sent.Params["text"]), mainKeyboard: staffKeyboardOf(sent.Params["reply_markup"]),
	}
}

// settingsDelMenus runs a case in a group menu and in a private menu, each in its own environment.
func settingsDelMenus(t *testing.T, run func(t *testing.T, m *settingsDelMenu)) {
	t.Helper()
	t.Run("group menu", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		run(t, env.groupMenu())
	})
	t.Run("private menu", func(t *testing.T) {
		env := newSettingsDelEnv(t)
		run(t, env.privateMenu())
	})
}

// fields are the callback fields of a button of this menu: the owner field plus the pairs.
func (m *settingsDelMenu) fields(kv ...string) map[string]string {
	if m.private {
		return m.env.privateFields(m.env.chat.Id, kv...)
	}
	return m.env.groupFields(m.actor, kv...)
}

// press presses a button of this menu built from the pairs.
func (m *settingsDelMenu) press(kv ...string) {
	m.env.t.Helper()
	data := encodeCallbackData(settingsCallbackNamespace, m.fields(kv...))
	if data == "" {
		m.env.t.Fatalf("callback data %v did not encode", kv)
	}
	m.env.pressRaw(m.chat, m.actor, m.msgID, data)
}

// pressButton presses an existing button of the menu, with the data it carries.
func (m *settingsDelMenu) pressButton(button gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.env.pressRaw(m.chat, m.actor, m.msgID, button.CallbackData)
}

// last is the text and keyboard of the menu message after its last edit.
func (m *settingsDelMenu) last() (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	return m.env.lastEdit(m.chat.Id, m.msgID)
}

// editCount is how many times the menu message was edited so far.
func (m *settingsDelMenu) editCount() int {
	return len(m.env.edits(m.chat.Id, m.msgID))
}

// open presses a page action (dl or dr) with its t field.
func (m *settingsDelMenu) open(action, code string) (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.press("a", action, "t", code)
	return m.last()
}

// openHub presses the Deletion cell of Main.
func (m *settingsDelMenu) openHub() (string, [][]gotgbot.InlineKeyboardButton) {
	m.env.t.Helper()
	m.press("a", "op", "p", "d")
	return m.last()
}

// choose presses the choice v of the row code on its screen.
func (m *settingsDelMenu) choose(code, value string) {
	m.env.t.Helper()
	m.press("a", "st", "p", "d", "t", code, "v", value)
}

// botReads counts the getChatMember calls about the bot itself in chatID.
func (e *settingsDelEnv) botReads(chatID int64) int {
	count := 0
	for _, call := range callsToChat(e.fake.staffBotClient, "getChatMember", chatID) {
		if staffParamInt(call.Params, "user_id") == staffTestBotID {
			count++
		}
	}
	return count
}

// setBot scripts the bot's own record in the env's chat.
func (e *settingsDelEnv) setBot(raw string) {
	e.t.Helper()
	e.fake.setBotMember(e.chat.Id, raw)
}

// stored reads the stored delay of a kind straight from the database.
func (e *settingsDelEnv) stored(kind string) (delay int, present bool) {
	e.t.Helper()
	row, ok := deletionStoredRow(e.t, e.chat.Id, kind)
	return row.DelaySecs, ok
}

// setRule stores a rule through the real apply function (a service or bot row, not joined or left).
func (e *settingsDelEnv) setRule(kind string, delay int) {
	e.t.Helper()
	if res := applyDeleteRule(context.Background(), e.chat.Id, kind, deletionSet(delay)); res.Outcome != deletionApplied {
		e.t.Fatalf("applyDeleteRule(%s, %d) = %+v", kind, delay, res)
	}
}

// setJoinLeft stores the joined or left rule through the real shared apply function.
func (e *settingsDelEnv) setJoinLeft(kind string, state deletion.JoinLeftState) {
	e.t.Helper()
	if res := applyCleanServiceRule(context.Background(), e.chat.Id, kind, state); res.Outcome == deletionRefused || res.Outcome == deletionSaveFailed {
		e.t.Fatalf("applyCleanServiceRule(%s, %+v) = %+v", kind, state, res)
	}
}

// makeStaff turns the env's chat into a Staff Group.
func (e *settingsDelEnv) makeStaff() {
	e.t.Helper()
	if _, err := staff.CreateStaffGroup(e.chat.Id, e.owner.Id, "Staff"); err != nil {
		e.t.Fatalf("CreateStaffGroup(%d) error = %v", e.chat.Id, err)
	}
}

// settingsDelWantLine fails the test unless the screen has the state line of nameKey with the icon and
// the word marker (or text) in it.
func settingsDelWantLine(t *testing.T, text, nameKey, icon string, words ...string) {
	t.Helper()
	line, ok := settingsMainStateLine(text, nameKey)
	if !ok {
		t.Errorf("no state line for %s in %q", nameKey, text)
		return
	}
	if !strings.Contains(line, icon) {
		t.Errorf("line %q of %s lacks the icon %s", line, nameKey, icon)
	}
	for _, word := range words {
		if !strings.Contains(line, word) {
			t.Errorf("line %q of %s lacks %q", line, nameKey, word)
		}
	}
}

// settingsDelWidths is the width of each row of a keyboard.
func settingsDelWidths(keyboard [][]gotgbot.InlineKeyboardButton) []int {
	widths := make([]int, 0, len(keyboard))
	for _, row := range keyboard {
		widths = append(widths, len(row))
	}
	return widths
}
