//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsFwdEnv is the toolkit of the Blocked sources page's tests: the list environment whose chat is a Staff Group
// made by a live creator, with the Staff commands loaded on its dispatcher (so a parity test can run /addfwdblock and
// the others), the live source resolver scripted and the env's administrator a plain administrator of the Staff Group.
type settingsFwdEnv struct {
	*settingsListEnv
	owner    gotgbot.User
	resolver *fwdResolver
}

// newSettingsFwdEnv builds the environment. The default chat of the settings environment is the Staff Group.
func newSettingsFwdEnv(t *testing.T) *settingsFwdEnv {
	t.Helper()
	env := newSettingsListEnv(t)
	owner := env.newOwner(env.chat)
	e := &settingsFwdEnv{settingsListEnv: env, owner: owner, resolver: &fwdResolver{answers: map[any]forwardBlockChat{
		"@goodchan":           {ID: -1001000000011, Type: "channel", Title: "Good <Channel>"},
		"@aprivate":           {ID: -1001000000013, Type: "private", Title: "Someone"},
		int64(-1001000000012): {ID: -1001000000012, Type: "supergroup", Title: "Numeric Group"},
	}, errs: map[any]error{
		"@limited": &gotgbot.TelegramError{Method: "getChat", Code: 429, Description: "Too Many Requests"},
	}}}
	e.resolver.install(t)
	e.makeStaff(env.chat, owner)
	return e
}

// makeStaff turns chat into a Staff Group of owner and cleans its rows, keys and audit rows with the test.
func (e *settingsFwdEnv) makeStaff(chat gotgbot.Chat, owner gotgbot.User) {
	e.t.Helper()
	staffCleanup(e.t, chat.Id)
	if _, err := staff.CreateStaffGroup(chat.Id, owner.Id, "Staff HQ"); err != nil {
		e.t.Fatalf("CreateStaffGroup(%d) error = %v", chat.Id, err)
	}
}

// otherStaffGroup makes a second Staff Group of the same owner, listing the env's administrator as an administrator.
func (e *settingsFwdEnv) otherStaffGroup() gotgbot.Chat {
	e.t.Helper()
	chat := e.addChat()
	e.fake.setMember(chat.Id, e.owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	e.fake.setCreator(chat.Id, e.owner.Id)
	e.fake.listAdministrator(chat.Id, e.admin.Id)
	settingsListCleanup(e.t, chat.Id)
	e.makeStaff(chat, e.owner)
	return chat
}

// seed lists sources of the Staff Group through the repository, each titled "T<last digits>".
func (e *settingsFwdEnv) seed(chat gotgbot.Chat, sources ...int64) {
	e.t.Helper()
	for _, source := range sources {
		if _, err := staff.AddForwardBlock(context.Background(), chat.Id, source, fmt.Sprintf("T%d", -source%1000), e.owner.Id); err != nil {
			e.t.Fatalf("seed source %d: %v", source, err)
		}
	}
}

// stored is the Staff Group's listed sources with their titles.
func (e *settingsFwdEnv) stored(chat gotgbot.Chat) map[int64]string {
	e.t.Helper()
	rows, err := staff.ListForwardBlocksFresh(context.Background(), chat.Id)
	if err != nil {
		e.t.Fatalf("list the sources: %v", err)
	}
	got := make(map[int64]string, len(rows))
	for _, row := range rows {
		got[row.SourceChatID] = row.SourceTitle
	}
	return got
}

// idOf is the row ID of a listed source.
func (e *settingsFwdEnv) idOf(chat gotgbot.Chat, source int64) int64 {
	e.t.Helper()
	rows, err := staff.ListForwardBlocksFresh(context.Background(), chat.Id)
	if err != nil {
		e.t.Fatalf("list the sources: %v", err)
	}
	for _, row := range rows {
		if row.SourceChatID == source {
			return row.ID
		}
	}
	e.t.Fatalf("source %d is not listed in chat %d", source, chat.Id)
	return 0
}

// action is the Staff Group's stored action.
func (e *settingsFwdEnv) action(chat gotgbot.Chat) string {
	e.t.Helper()
	rules, err := staff.GetForwardBlockRulesFresh(context.Background(), chat.Id)
	if err != nil {
		e.t.Fatalf("read the action: %v", err)
	}
	return rules.Action
}

// open opens the Blocked sources page of the Staff Group in a fresh private menu of user.
func (e *settingsFwdEnv) open(chat gotgbot.Chat, user gotgbot.User) *settingsBlacklistMenu {
	e.t.Helper()
	return settingsListOpenMenuPage(e.settingsListEnv, chat, user, settingsPageFwdBlock)
}

// begin opens the page as user and presses Add, so user has a wait for the source part. It returns the menu.
func (e *settingsFwdEnv) begin(chat gotgbot.Chat, user gotgbot.User) *settingsBlacklistMenu {
	e.t.Helper()
	m := e.open(chat, user)
	add, ok := m.markedButton("settings_btn_fwd_add")
	if !ok {
		e.t.Fatalf("the Blocked sources page has no Add button")
	}
	m.tap(add)
	if !e.redis.Exists(settingsInputKey(user.Id)) {
		e.t.Fatalf("pressing Add left user %d with no wait", user.Id)
	}
	return m
}

// endWait removes whatever wait and marker the previous case left.
func (e *settingsFwdEnv) endWait(user gotgbot.User) {
	e.redis.Del(settingsInputKey(user.Id))
	e.redis.Del(settingsInputDoneKey(user.Id))
}

// command runs a text command in the Staff Group as from and returns the text of the reply it sent ("" when none).
func (e *settingsFwdEnv) command(chat gotgbot.Chat, from gotgbot.User, text string) string {
	e.t.Helper()
	before := len(e.fake.sentTo(chat.Id))
	e.sendIn(chat, from, text)
	sent := e.fake.sentTo(chat.Id)
	if len(sent) == before {
		return ""
	}
	return fmt.Sprint(sent[len(sent)-1].Params["text"])
}

// wantAlert presses a button of the page with the given fields as user and fails the test unless the press was
// answered with exactly one alert of key and edited nothing.
func (e *settingsFwdEnv) wantAlert(m *settingsBlacklistMenu, name, key string, fields ...string) {
	e.t.Helper()
	answers, edits := e.answerCount(), len(e.edits(m.user.Id, m.msgID))
	m.press(fields...)
	if got := e.answerCount() - answers; got != 1 {
		e.t.Errorf("%s: the press made %d answers, want 1", name, got)
		return
	}
	text, alert := e.lastAnswer()
	if !alert || !strings.Contains(text, staffMarker(key)) {
		e.t.Errorf("%s: answered %q (alert %v), want the alert %s", name, text, alert, key)
	}
	if got := len(e.edits(m.user.Id, m.msgID)); got != edits {
		e.t.Errorf("%s: the press edited the menu %d times, want none", name, got-edits)
	}
}

// settingsFwdCodes are the seven action codes of the page with the fields of a valid press.
func settingsFwdCodes(row string) [][]string {
	minute := settingsCardExpiry(time.Now())
	return [][]string{
		{"a", "vg", "v", "1"}, {"a", "vx", "r", row, "v", "1"}, {"a", "vy", "r", row, "e", minute},
		{"a", "va"}, {"a", "vc", "t", "s"}, {"a", "ve"}, {"a", "vs", "v", "ban"},
	}
}

// settingsFwdCountSeams replaces the page's read and count seams with counting wrappers and restores them.
func settingsFwdCountSeams(t *testing.T) (reads, counts *atomic.Int64) {
	t.Helper()
	reads, counts = new(atomic.Int64), new(atomic.Int64)
	read, count := settingsFwdBlockRead, settingsFwdBlockCount
	mreqSwap(t, &settingsFwdBlockRead, func(ctx context.Context, chatID int64) ([]settingsListEntry, error) {
		reads.Add(1)
		return read(ctx, chatID)
	})
	mreqSwap(t, &settingsFwdBlockCount, func(ctx context.Context, chatID int64) (int, error) {
		counts.Add(1)
		return count(ctx, chatID)
	})
	return reads, counts
}

func TestSettingsFwdBlockOpens(t *testing.T) {
	t.Run("the Staff Group's private menu lists the sources in order with the intro, the count and the one action", func(t *testing.T) {
		e := newSettingsFwdEnv(t)
		e.seed(e.chat, -1001000000002, -1001000000001)

		mainMsg := e.updateID()
		e.pressPrivate(e.owner, mainMsg, e.privateFields(e.chat.Id, "a", settingsActBack))
		mainText, mainKeyboard := e.lastEdit(e.owner.Id, mainMsg)
		line, ok := settingsMainStateLine(mainText, "settings_page_fwdblock")
		if !ok || !strings.HasSuffix(line, " 2/200") {
			t.Errorf("Main's Blocked sources line = %q, want it to end in 2/200:\n%s", line, mainText)
		}
		if _, ok := settingsButtonByMarker(mainKeyboard, "settings_page_fwdblock"); !ok {
			t.Errorf("Main has no Blocked sources cell")
		}

		_, text, keyboard := settingsListOpenPrivate(e.settingsListEnv, e.owner, settingsPageFwdBlock)
		for _, want := range []string{
			staffMarker("settings_header"), staffMarker("settings_page_fwdblock"), staffMarker("settings_fwd_intro"),
			staffMarker("settings_fwd_count") + " 2 200", staffMarker("settings_bl_action_line") + " " + staffMarker("settings_bl_opt_none"),
			"1. <code>-1001000000001 T1</code>", "2. <code>-1001000000002 T2</code>",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("the page lacks %q:\n%s", want, text)
			}
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2, 2, 2}) {
			t.Errorf("row widths = %v, want the delete row, [Add | Action] and the navigation row", got)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_fwd_add"); !ok {
			t.Errorf("the page has no Add button: %+v", keyboard)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_btn_bl_action"); !ok {
			t.Errorf("the page has no Action button: %+v", keyboard)
		}
	})

	t.Run("an empty list shows the intro and the empty line with Add and Action", func(t *testing.T) {
		e := newSettingsFwdEnv(t)
		_, text, keyboard := settingsListOpenPrivate(e.settingsListEnv, e.owner, settingsPageFwdBlock)
		if !strings.Contains(text, staffMarker("settings_fwd_intro")) || !strings.Contains(text, staffMarker("settings_fwd_empty")) {
			t.Errorf("the empty page lacks the intro or the empty line:\n%s", text)
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{2, 2}) {
			t.Errorf("empty list rows = %v, want [Add | Action] and the navigation row", got)
		}
	})

	t.Run("the page is hidden and every code refused in a chat that is not a Staff Group", func(t *testing.T) {
		e := newSettingsFwdEnv(t)
		chat := e.addChat()
		person := e.newOwner(chat)
		settingsListCleanup(t, chat.Id)
		reads, counts := settingsFwdCountSeams(t)

		mainMsg := e.updateID()
		e.pressPrivate(person, mainMsg, e.privateFields(chat.Id, "a", settingsActBack))
		text, keyboard := e.lastEdit(person.Id, mainMsg)
		if _, ok := settingsMainStateLine(text, "settings_page_fwdblock"); ok {
			t.Errorf("a plain group's Main has a Blocked sources line:\n%s", text)
		}
		if _, ok := settingsButtonByMarker(keyboard, "settings_page_fwdblock"); ok {
			t.Errorf("a plain group's Main has a Blocked sources cell")
		}

		m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: chat, user: person, msgID: e.updateID()}
		e.wantAlert(m, "open", "common_callback_invalid_request", "a", settingsActOpen, "p", settingsPageFwdBlock)
		for _, fields := range settingsFwdCodes("5") {
			e.wantAlert(m, "code "+fields[1], "common_callback_invalid_request", fields...)
		}
		if reads.Load() != 0 || counts.Load() != 0 {
			t.Errorf("a plain group read the list %d times and counted it %d times, want none", reads.Load(), counts.Load())
		}
		if n := fwdRowCount(t, chat.Id); n != 0 {
			t.Errorf("the forged presses left %d rows", n)
		}
	})

	t.Run("a Staff Group whose cached gate says no refuses the page like any other group", func(t *testing.T) {
		e := newSettingsFwdEnv(t)
		mreqSwap(t, &settingsFwdBlockStaffGate, func(int64) bool { return false })
		m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: e.chat, user: e.owner, msgID: e.updateID()}
		e.wantAlert(m, "open", "common_callback_invalid_request", "a", settingsActOpen, "p", settingsPageFwdBlock)
		e.wantAlert(m, "an Add press", "common_callback_invalid_request", "a", "va")
		e.wantAlert(m, "an action press", "common_callback_invalid_request", "a", "vs", "v", "ban")
		if e.action(e.chat) != "none" {
			t.Errorf("the refused action press changed the stored action to %q", e.action(e.chat))
		}
	})

	t.Run("the Staff Group's own group menu shows only the private-only note and reads nothing", func(t *testing.T) {
		setCachedBotUsername(t, "FukuTestBot")
		e := newSettingsFwdEnv(t)
		const sentinel = "ZZSentinelSource"
		if _, err := staff.AddForwardBlock(context.Background(), e.chat.Id, -1001000000001, sentinel, e.owner.Id); err != nil {
			t.Fatal(err)
		}
		reads, counts := settingsFwdCountSeams(t)

		msgID := e.openMenu(e.owner)
		mainText, _ := e.lastEdit(e.chat.Id, msgID)
		line, ok := settingsMainStateLine(mainText, "settings_page_fwdblock")
		if !ok || !strings.HasSuffix(line, staffMarker("settings_state_private_only")) {
			t.Errorf("a group Main's Blocked sources line = %q, want the private-only word", line)
		}
		e.press(e.owner, msgID, e.groupFields(e.owner, "a", settingsActOpen, "p", settingsPageFwdBlock))
		text, _ := e.lastEdit(e.chat.Id, msgID)
		if !strings.Contains(text, staffMarker("settings_entries_group_note")) {
			t.Errorf("the group menu's page is not the bare note:\n%s", text)
		}
		for _, fields := range settingsFwdCodes("5") {
			e.press(e.owner, msgID, e.groupFields(e.owner, fields...))
			if answer, alert := e.lastAnswer(); !alert || !strings.Contains(answer, staffMarker("common_callback_invalid_request")) {
				t.Errorf("code %s of a group menu answered %q (alert %v), want an invalid request", fields[1], answer, alert)
			}
		}
		if reads.Load() != 0 || counts.Load() != 0 {
			t.Errorf("a group menu read the list %d times and counted it %d times, want none", reads.Load(), counts.Load())
		}
		// Nothing the bot edited, sent or answered in the group menu's run names a source, its title or its ID (T-tjj-05).
		if everything := settingsBlacklistTexts(e.settingsListEnv); strings.Contains(everything, sentinel) || strings.Contains(everything, "1001000000001") {
			t.Errorf("the group menu's run named a source:\n%s", everything)
		}
	})
}

func TestSettingsFwdBlockMainShape(t *testing.T) {
	// A Staff Group hides Deletion and shows Blocked sources, so its Main has the same eighteen cells as any other chat's:
	// the grid pairs Blocked sources with Staff, and Access and the navigation follow.
	const (
		groupOwner   = "s|v / l|c / f|n / k|b / i|w / g|r / e|m / t|u / o|y / a / close"
		privateOwner = "s|v / l|c / f|n / k|b / i|w / g|r / e|m / t|u / o|y / a / groups|close"
		groupAdmin   = "s|v / l|c / f|n / k|b / i|w / g|r / e|m / t|u / o|y / close"
		privateAdmin = "s|v / l|c / f|n / k|b / i|w / g|r / e|m / t|u / o|y / groups|close"
	)
	e := newSettingsFwdEnv(t)
	check := func(name string, keyboard [][]gotgbot.InlineKeyboardButton, want string, rows int) {
		t.Helper()
		if got := settingsMainShape(t, keyboard); got != want {
			t.Errorf("%s Main = %q, want %q", name, got, want)
		}
		if len(keyboard) != rows {
			t.Errorf("%s Main has %d rows, want %d", name, len(keyboard), rows)
		}
	}

	groupMsg := e.openMenu(e.owner)
	_, keyboard := e.lastEdit(e.chat.Id, groupMsg)
	check("the owner's group", keyboard, groupOwner, settingsMainOwnerRows)
	privateMsg := e.updateID()
	e.pressPrivate(e.owner, privateMsg, e.privateFields(e.chat.Id, "a", settingsActBack))
	_, keyboard = e.lastEdit(e.owner.Id, privateMsg)
	check("the owner's private", keyboard, privateOwner, settingsMainOwnerRows)

	// An administrator the owner gate admits has no Access row.
	e.setAdminsAllowed(e.chat.Id, true)
	adminGroupMsg := e.openMenu(e.admin)
	_, keyboard = e.lastEdit(e.chat.Id, adminGroupMsg)
	check("the administrator's group", keyboard, groupAdmin, settingsMainOwnerRows-1)
	adminPrivateMsg := e.updateID()
	e.pressPrivate(e.admin, adminPrivateMsg, e.privateFields(e.chat.Id, "a", settingsActBack))
	adminText, keyboard := e.lastEdit(e.admin.Id, adminPrivateMsg)
	check("the administrator's private", keyboard, privateAdmin, settingsMainOwnerRows-1)
	if _, ok := settingsMainStateLine(adminText, "settings_page_fwdblock"); !ok {
		t.Errorf("the administrator's Main has no Blocked sources line:\n%s", adminText)
	}
}

func TestSettingsFwdBlockAdd(t *testing.T) {
	e := newSettingsFwdEnv(t)
	cacheKey := dbcache.CacheKey("staff_forward_block", e.chat.Id)
	sayAndReply := func(text string, wantKeys ...string) string {
		t.Helper()
		before := settingsEditSentCount(e.settingsEnv, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, text)
		reply := settingsListDoorReply(t, e.settingsListEnv, e.owner, before, wantKeys...)
		return fmt.Sprint(reply.Params["text"])
	}

	t.Run("an @username is resolved, stored with its title, the cache dropped and the saved page sent", func(t *testing.T) {
		if _, err := staff.GetForwardBlockRules(context.Background(), e.chat.Id); err != nil {
			t.Fatal(err)
		}
		if !e.redis.Exists(cacheKey) {
			t.Fatalf("the watcher's cache key %s was not primed", cacheKey)
		}
		m := e.begin(e.chat, e.owner)
		prompt, keyboard := m.last()
		for _, want := range []string{
			staffMarker("settings_header") + " " + staffMarker("settings_page_fwdblock"), staffMarker("settings_fwd_wait"),
			staffMarker("settings_fwd_wait_rules") + " 200", staffMarker("settings_edit_wait_footer"),
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("the prompt lacks %q:\n%s", want, prompt)
			}
		}
		if len(keyboard) != 1 || len(keyboard[0]) != 1 {
			t.Fatalf("the prompt keyboard = %+v, want one row with Cancel", keyboard)
		}
		if fields := settingsButtonFields(t, keyboard[0][0]); fields["a"] != "vc" || fields["t"] != "s" || len(fields) != 3 {
			t.Errorf("Cancel carries %v, want vc, t=s and c", fields)
		}
		if got := e.redis.HGet(settingsInputKey(e.owner.Id), "p") + "/" + e.redis.HGet(settingsInputKey(e.owner.Id), "t"); got != "v/s" {
			t.Errorf("the wait reads page/part %q, want v/s", got)
		}

		before := settingsEditSentCount(e.settingsEnv, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, "@goodchan")
		if got := e.stored(e.chat); len(got) != 1 || got[-1001000000011] != "Good <Channel>" {
			t.Errorf("stored %v, want the resolved channel with its title", got)
		}
		var createdBy int64
		db.DB.Model(&models.StaffForwardBlock{}).Where("staff_chat_id = ?", e.chat.Id).Select("created_by").Scan(&createdBy)
		if createdBy != e.owner.Id {
			t.Errorf("created_by = %d, want the sender %d", createdBy, e.owner.Id)
		}
		if e.redis.Exists(cacheKey) {
			t.Errorf("the watcher's cache key %s survived the add", cacheKey)
		}
		page := settingsListDoorReply(t, e.settingsListEnv, e.owner, before, "settings_page_fwdblock", "settings_fwd_saved")
		if text := fmt.Sprint(page.Params["text"]); !strings.Contains(text, "<code>-1001000000011 Good &lt;Channel&gt;</code>") {
			t.Errorf("the saved page lacks the escaped entry:\n%s", text)
		}
		if closed, closedKeyboard := m.last(); !strings.Contains(closed, staffMarker("settings_edit_prompt_closed")) || len(closedKeyboard) != 0 {
			t.Errorf("the prompt was edited to %q with keyboard %+v, want the closed line and none", closed, closedKeyboard)
		}
		settingsEditWaitGone(t, e.settingsEnv, e.owner)
	})

	t.Run("a numeric ID is stored as typed after its live lookup", func(t *testing.T) {
		e.begin(e.chat, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, "-1001000000012")
		if got := e.stored(e.chat); got[-1001000000012] != "Numeric Group" || len(got) != 2 {
			t.Errorf("stored %v, want the numeric group added beside the first source", got)
		}
	})

	t.Run("a message that is not one token or has no text is refused with the wait kept and nothing stored", func(t *testing.T) {
		e.endWait(e.owner)
		e.begin(e.chat, e.owner)
		calls := len(e.resolver.calls)
		sayAndReply("@goodchan @other", "settings_fwd_refuse_one", "settings_edit_still_waiting")
		settingsEditWaitKept(t, e.settingsEnv, e.owner)
		before := settingsEditSentCount(e.settingsEnv, e.owner)
		sendPrivateMessage(e.settingsEnv, e.owner, settingsMediaItem("sticker"))
		settingsListDoorReply(t, e.settingsListEnv, e.owner, before, "settings_edit_refuse_need_text", "settings_edit_still_waiting")
		settingsEditWaitKept(t, e.settingsEnv, e.owner)
		if len(e.resolver.calls) != calls || len(e.stored(e.chat)) != 2 {
			t.Errorf("a refused message made %d lookups and left %v", len(e.resolver.calls)-calls, e.stored(e.chat))
		}
	})

	t.Run("each refusal says its own words, keeps the wait and stores nothing", func(t *testing.T) {
		for _, tc := range []struct{ name, text, key string }{
			{"an unknown name", "@nobody", "staff_fwdblock_not_found"},
			{"a name that is a person", "@aprivate", "staff_fwdblock_not_source"},
			{"something that is not a ref", "hello", "staff_fwdblock_usage_add"},
			{"a source that is already listed", "@goodchan", "staff_fwdblock_already"},
		} {
			e.endWait(e.owner)
			e.begin(e.chat, e.owner)
			text := sayAndReply(tc.text, tc.key, "settings_edit_still_waiting")
			if tc.key == "staff_fwdblock_already" && !strings.Contains(text, "-1001000000011 Good &lt;Channel&gt;") {
				t.Errorf("%s: the refusal does not name the source: %q", tc.name, text)
			}
			settingsEditWaitKept(t, e.settingsEnv, e.owner)
			if got := e.stored(e.chat); len(got) != 2 {
				t.Errorf("%s: the refusal stored %v", tc.name, got)
			}
		}
	})

	t.Run("a lookup Telegram could not answer keeps the wait with the door's own plain reply", func(t *testing.T) {
		e.endWait(e.owner)
		e.begin(e.chat, e.owner)
		text := sayAndReply("@limited", "settings_check_failed")
		if strings.Contains(text, staffMarker("settings_edit_still_waiting")) {
			t.Errorf("the reply %q carries the still-waiting line", text)
		}
		settingsEditWaitKept(t, e.settingsEnv, e.owner)
		if got := e.stored(e.chat); len(got) != 2 {
			t.Errorf("a failed lookup stored %v", got)
		}
	})

	t.Run("a full list and a failed save are refused with the shared words", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			err  error
			key  string
		}{
			{"a full list", staff.ErrForwardBlockFull, "staff_fwdblock_full"},
			{"a failed save", errors.New("disk full"), "common_settings_save_failed"},
		} {
			writes := forwardBlockWrites
			mreqSwap(t, &forwardBlockWrites, forwardBlockWriteSeams{
				Add:            func(context.Context, int64, int64, string, int64) (bool, error) { return false, tc.err },
				RemoveByID:     writes.RemoveByID,
				RemoveBySource: writes.RemoveBySource,
				SetAction:      writes.SetAction,
			})
			e.endWait(e.owner)
			e.begin(e.chat, e.owner)
			sayAndReply("@goodchan", tc.key, "settings_edit_still_waiting")
			settingsEditWaitKept(t, e.settingsEnv, e.owner)
			if got := e.stored(e.chat); len(got) != 2 {
				t.Errorf("%s: stored %v", tc.name, got)
			}
			forwardBlockWrites = writes
		}
	})

	t.Run("a Staff Group unset between the Add press and the message ends the wait", func(t *testing.T) {
		chat := e.otherStaffGroup()
		e.begin(chat, e.owner)
		if _, _, err := staff.DeleteStaffGroupWithLinks(chat.Id); err != nil {
			t.Fatal(err)
		}
		before := settingsEditSentCount(e.settingsEnv, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, "@goodchan")
		text := fmt.Sprint(settingsListDoorReply(t, e.settingsListEnv, e.owner, before, "staff_unset_not_staff", "settings_edit_wait_stopped").Params["text"])
		if !strings.HasSuffix(text, staffMarker("settings_edit_wait_stopped")) {
			t.Errorf("the reply %q does not end with the wait-stopped line", text)
		}
		settingsEditWaitGone(t, e.settingsEnv, e.owner)
		if n := fwdRowCount(t, chat.Id); n != 0 {
			t.Errorf("the add stored %d rows for a chat that is no Staff Group", n)
		}
	})

	t.Run("without Redis Add is the alert and the list, the card and the action keep working", func(t *testing.T) {
		e.endWait(e.owner)
		m := e.open(e.chat, e.owner)
		add, _ := m.markedButton("settings_btn_fwd_add")
		edits := len(e.edits(e.owner.Id, m.msgID))
		id := strconv.FormatInt(e.idOf(e.chat, -1001000000012), 10)

		restore := cache.DisableRedisForTest()
		answers := e.answerCount()
		m.tap(add)
		text, alert := e.lastAnswer()
		if got := e.answerCount() - answers; got != 1 || !alert || !strings.Contains(text, staffMarker("settings_entries_add_unavailable")) {
			t.Errorf("Add without Redis answered %q (alert %v, %d answers), want one alert settings_entries_add_unavailable", text, alert, got)
		}
		if got := len(e.edits(e.owner.Id, m.msgID)); got != edits {
			t.Errorf("Add without Redis edited the menu %d times", got-edits)
		}
		m.press("a", "vg", "v", "1")
		m.press("a", "vx", "r", id, "v", "1")
		if card, _ := m.last(); !strings.Contains(card, staffMarker("settings_fwd_delete_ask")) {
			t.Errorf("the delete card did not draw without Redis:\n%s", card)
		}
		m.press("a", "vs", "v", "kick")
		m.press("a", "vy", "r", id, "e", settingsCardExpiry(time.Now()))
		restore()
		if got := e.stored(e.chat); len(got) != 1 {
			t.Errorf("the delete without Redis left %v", got)
		}
		if got := e.action(e.chat); got != "kick" {
			t.Errorf("the action without Redis is %q, want kick", got)
		}
		if keys := settingsEditInputKeys(e.settingsEnv); len(keys) != 0 {
			t.Errorf("Add without Redis left %v", keys)
		}
	})
}

func TestSettingsFwdBlockDelete(t *testing.T) {
	e := newSettingsFwdEnv(t)
	e.seed(e.chat, -1001000000001, -1001000000002, -1001000000003)
	cacheKey := dbcache.CacheKey("staff_forward_block", e.chat.Id)
	prime := func() {
		t.Helper()
		if _, err := staff.GetForwardBlockRules(context.Background(), e.chat.Id); err != nil {
			t.Fatal(err)
		}
		if !e.redis.Exists(cacheKey) {
			t.Fatalf("the watcher's cache key %s was not primed", cacheKey)
		}
	}
	id := func(source int64) string { return strconv.FormatInt(e.idOf(e.chat, source), 10) }

	t.Run("the card names the source, Yes deletes that row only and drops the cache, No returns to the page", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		prime()
		m.press("a", "vx", "r", id(-1001000000002), "v", "1")
		card, keyboard := m.last()
		if !strings.Contains(card, staffMarker("settings_fwd_delete_ask")+" -1001000000002 T2") {
			t.Fatalf("the card does not name the source:\n%s", card)
		}
		if got := e.stored(e.chat); len(got) != 3 {
			t.Fatalf("the ask changed the list: %v", got)
		}
		no, ok := settingsButtonByMarker(keyboard, "settings_btn_no")
		if !ok {
			t.Fatalf("the card has no No button: %+v", keyboard)
		}
		m.tap(no)
		if page, _ := m.last(); !strings.Contains(page, "2. <code>-1001000000002 T2</code>") {
			t.Errorf("No did not return to the page:\n%s", page)
		}

		m.press("a", "vx", "r", id(-1001000000002), "v", "1")
		_, keyboard = m.last()
		yes, _ := settingsButtonByMarker(keyboard, "settings_btn_yes")
		m.tap(yes)
		if got := e.stored(e.chat); len(got) != 2 || got[-1001000000001] == "" || got[-1001000000003] == "" {
			t.Errorf("Yes left %v, want only the second source gone", got)
		}
		if e.redis.Exists(cacheKey) {
			t.Errorf("the watcher's cache key %s survived the delete", cacheKey)
		}
		page, _ := m.last()
		if !strings.Contains(page, settingsNoticeOKIcon+staffMarker("settings_fwd_removed")+" -1001000000002 T2") || strings.Contains(page, "T2</code>") {
			t.Errorf("the redrawn page is wrong:\n%s", page)
		}
	})

	t.Run("an expired Yes deletes nothing", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		m.press("a", "vy", "r", id(-1001000000003), "e", settingsCardExpiry(time.Now().Add(-2*settingsCardLifetime)))
		if page, _ := m.last(); !strings.Contains(page, staffMarker("settings_card_expired")) {
			t.Errorf("the expired Yes did not say so:\n%s", page)
		}
		if got := e.stored(e.chat); len(got) != 2 {
			t.Errorf("an expired Yes left %v", got)
		}
	})

	t.Run("a foreign row and a gone row answer alike and delete nothing", func(t *testing.T) {
		other := e.otherStaffGroup()
		e.seed(other, -1001000000099)
		foreign := strconv.FormatInt(e.idOf(other, -1001000000099), 10)
		for _, row := range []string{foreign, "999999999"} {
			m := e.open(e.chat, e.owner)
			m.press("a", "vx", "r", row, "v", "1")
			page, _ := m.last()
			if !strings.Contains(page, staffMarker("settings_entries_already_removed")) || strings.Contains(page, staffMarker("settings_fwd_delete_ask")) {
				t.Errorf("row %s: the ask drew %q, want the already-removed notice and no card", row, page)
			}
			m.press("a", "vy", "r", row, "e", settingsCardExpiry(time.Now()))
			if page, _ := m.last(); !strings.Contains(page, staffMarker("settings_entries_already_removed")) {
				t.Errorf("row %s: the Yes drew %q, want the already-removed notice", row, page)
			}
		}
		if got := e.stored(other); len(got) != 1 {
			t.Errorf("the other Staff Group's list became %v", got)
		}
		if got := e.stored(e.chat); len(got) != 2 {
			t.Errorf("this list became %v", got)
		}
	})

	t.Run("a presser demoted between the two taps is refused and deletes nothing", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		m.press("a", "vx", "r", id(-1001000000003), "v", "1")
		_, keyboard := m.last()
		yes, _ := settingsButtonByMarker(keyboard, "settings_btn_yes")
		e.fake.setMember(e.chat.Id, e.owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		answers := e.answerCount()
		m.tap(yes)
		if text, alert := e.lastAnswer(); e.answerCount()-answers != 1 || !alert || !strings.Contains(text, staffMarker("staff_fwdblock_owner_only")) {
			t.Errorf("the demoted Yes answered %q (alert %v), want the owner-only alert", text, alert)
		}
		if got := e.stored(e.chat); len(got) != 2 {
			t.Errorf("a demoted Yes left %v", got)
		}
	})
}

func TestSettingsFwdBlockAction(t *testing.T) {
	e := newSettingsFwdEnv(t)
	cacheKey := dbcache.CacheKey("staff_forward_block", e.chat.Id)

	t.Run("the action screen is drawn for an empty list and every value is stored, marked and drops the cache", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		m.press("a", "ve")
		text, keyboard := m.last()
		for _, want := range []string{staffMarker("settings_page_fwdblock"), staffMarker("settings_bl_action_intro"), staffMarker("settings_fwd_action_hint")} {
			if !strings.Contains(text, want) {
				t.Errorf("the empty action screen lacks %q:\n%s", want, text)
			}
		}
		if got := settingsListRowWidths(keyboard); fmt.Sprint(got) != fmt.Sprint([]int{3, 2, 2}) {
			t.Errorf("action screen rows = %v, want the choices and the navigation row", got)
		}
		for _, action := range []string{"warn", "mute", "kick", "ban", "none"} {
			if _, err := staff.GetForwardBlockRules(context.Background(), e.chat.Id); err != nil {
				t.Fatal(err)
			}
			m.press("a", "vs", "v", action)
			if got := e.action(e.chat); got != action {
				t.Fatalf("after %s the stored action is %q", action, got)
			}
			if e.redis.Exists(cacheKey) {
				t.Errorf("%s: the watcher's cache key survived the write", action)
			}
			text, keyboard := m.last()
			word := map[string]string{"warn": "settings_opt_warn", "mute": "settings_opt_mute", "kick": "settings_opt_kick", "ban": "settings_opt_ban", "none": "settings_bl_opt_none"}[action]
			marked := settingsSelectedPrefix + staffMarker(word)
			count := 0
			for _, button := range settingsAllButtons(keyboard) {
				if strings.Contains(button.Text, marked) {
					count++
				}
			}
			if count != 1 || !strings.Contains(text, staffMarker("settings_bl_action_line")+" "+staffMarker(word)) {
				t.Errorf("%s: the screen marks %d buttons or lacks the action line:\n%s", action, count, text)
			}
			if strings.Contains(text, settingsNoticeWarnIcon) {
				t.Errorf("%s: the screen carries a warning notice:\n%s", action, text)
			}
		}
	})

	t.Run("a list with sources shows the action on its page and a press of the stored value changes nothing", func(t *testing.T) {
		e.seed(e.chat, -1001000000001)
		m := e.open(e.chat, e.owner)
		m.press("a", "vs", "v", "ban")
		m.press("a", "vs", "v", "ban")
		m.press("a", settingsActOpen, "p", settingsPageFwdBlock)
		if page, _ := m.last(); !strings.Contains(page, staffMarker("settings_bl_action_line")+" "+staffMarker("settings_opt_ban")) {
			t.Errorf("the page does not show the stored action:\n%s", page)
		}
	})

	t.Run("a forged or stale value is refused in step 1 and writes nothing", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		before := e.action(e.chat)
		for _, fields := range [][]string{
			{"a", "vs", "v", "tban"}, {"a", "vs", "v", "BAN"}, {"a", "vs", "v", ""}, {"a", "vs", "v", "ban ", "x", "1"}, {"a", "vs"}, {"a", "vs", "v", "delete"},
		} {
			e.wantAlert(m, fmt.Sprint(fields), "common_callback_invalid_request", fields...)
		}
		// A press with the group menu's owner field instead of c.
		msgID := e.openMenu(e.owner)
		e.press(e.owner, msgID, e.groupFields(e.owner, "a", "vs", "v", "kick"))
		if text, alert := e.lastAnswer(); !alert || !strings.Contains(text, staffMarker("common_callback_invalid_request")) {
			t.Errorf("a group menu's action press answered %q (alert %v)", text, alert)
		}
		if got := e.action(e.chat); got != before {
			t.Errorf("the forged presses changed the action from %q to %q", before, got)
		}
	})

	t.Run("a failed save redraws the stored state with the save-failed notice", func(t *testing.T) {
		m := e.open(e.chat, e.owner)
		before := e.action(e.chat)
		writes := forwardBlockWrites
		mreqSwap(t, &forwardBlockWrites, forwardBlockWriteSeams{
			Add:            writes.Add,
			RemoveByID:     writes.RemoveByID,
			RemoveBySource: writes.RemoveBySource,
			SetAction:      func(context.Context, int64, int64, string) error { return errors.New("write failed") },
		})
		m.press("a", "vs", "v", "kick")
		if text, _ := m.last(); !strings.Contains(text, settingsNoticeWarnIcon+staffMarker("common_settings_save_failed")) {
			t.Errorf("a failed save drew %q, want the save-failed notice", text)
		}
		if got := e.action(e.chat); got != before {
			t.Errorf("a failed save changed the action to %q", got)
		}
	})
}

// TestSettingsAuthorityParityFwdBlock is the authority-parity test of the page: the menu refuses what the commands refuse,
// with the right the controls declare. The Staff Group's live creator writes by page and command alike, a live
// administrator who is not the creator is refused by both with nothing written and views and pages freely, and an
// administrator the owner gate refuses is refused by the menu only, the documented carve-out.
func TestSettingsAuthorityParityFwdBlock(t *testing.T) {
	e := newSettingsFwdEnv(t)
	e.seed(e.chat, -1001000000001)
	row := strconv.FormatInt(e.idOf(e.chat, -1001000000001), 10)
	e.setAdminsAllowed(e.chat.Id, true)
	const ownerOnly = "staff_fwdblock_owner_only"
	snapshot := func() string { return fmt.Sprint(e.stored(e.chat), e.action(e.chat)) }
	before := snapshot()

	t.Run("an administrator who is not the creator views and pages by menu and command and is refused every write by both", func(t *testing.T) {
		m := e.open(e.chat, e.admin)
		if page, _ := m.last(); !strings.Contains(page, "1. <code>-1001000000001 T1</code>") {
			t.Errorf("the administrator cannot view the page:\n%s", page)
		}
		m.press("a", "vg", "v", "1")
		m.press("a", "ve")
		if text, _ := m.last(); !strings.Contains(text, staffMarker("settings_fwd_action_hint")) {
			t.Errorf("the administrator cannot view the action screen:\n%s", text)
		}
		for _, fields := range [][]string{
			{"a", "va"}, {"a", "vx", "r", row, "v", "1"}, {"a", "vy", "r", row, "e", settingsCardExpiry(time.Now())}, {"a", "vs", "v", "ban"},
		} {
			e.wantAlert(m, "page "+fields[1], ownerOnly, fields...)
		}
		if got := snapshot(); got != before {
			t.Errorf("the menu's refusals changed the list: %s, was %s", got, before)
		}

		if reply := e.command(e.chat, e.admin, "/fwdblocklist"); !strings.Contains(reply, staffMarker("staff_fwdblock_list_header")) || !strings.Contains(reply, "-1001000000001") {
			t.Errorf("the administrator's /fwdblocklist answered %q", reply)
		}
		if reply := e.command(e.chat, e.admin, "/fwdblockaction"); !strings.Contains(reply, staffMarker("staff_fwdblock_action_current")) {
			t.Errorf("the administrator's /fwdblockaction answered %q", reply)
		}
		for _, text := range []string{"/addfwdblock @goodchan", "/rmfwdblock -1001000000001", "/fwdblockaction ban"} {
			if reply := e.command(e.chat, e.admin, text); !strings.Contains(reply, staffMarker(ownerOnly)) {
				t.Errorf("the administrator's %q answered %q, want the owner-only text", text, reply)
			}
		}
		if got := snapshot(); got != before {
			t.Errorf("the commands' refusals changed the list: %s, was %s", got, before)
		}
	})

	t.Run("a creator demoted between the Add press and the typed message is refused and the wait ends", func(t *testing.T) {
		e.begin(e.chat, e.owner)
		e.fake.setMember(e.chat.Id, e.owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
		sent := settingsEditSentCount(e.settingsEnv, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, "@goodchan")
		reply := settingsListDoorReply(t, e.settingsListEnv, e.owner, sent, ownerOnly, "settings_edit_wait_stopped")
		if text := fmt.Sprint(reply.Params["text"]); !strings.HasSuffix(text, staffMarker("settings_edit_wait_stopped")) {
			t.Errorf("the refusal %q does not end with the wait-stopped line", text)
		}
		settingsEditWaitGone(t, e.settingsEnv, e.owner)
		if got := snapshot(); got != before {
			t.Errorf("the refused typed reply changed the list: %s, was %s", got, before)
		}
		e.fake.setMember(e.chat.Id, e.owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
	})

	t.Run("a plain member's presses and commands change nothing", func(t *testing.T) {
		member := gotgbot.User{Id: uniqueLinkOwnerID() + 8_500_000, FirstName: "Plain"}
		e.fake.setMember(e.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: e.chat, user: member, msgID: e.updateID()}
		e.wantAlert(m, "a plain member's Add", "settings_not_admin", "a", "va")
		if reply := e.command(e.chat, member, "/addfwdblock @goodchan"); strings.Contains(reply, staffMarker("staff_fwdblock_added")) {
			t.Errorf("a plain member's /addfwdblock answered %q", reply)
		}
		if got := snapshot(); got != before {
			t.Errorf("a plain member changed the list: %s, was %s", got, before)
		}
	})

	t.Run("the creator writes by menu and command alike", func(t *testing.T) {
		m := e.begin(e.chat, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, "@goodchan")
		m.press("a", "vs", "v", "mute")
		if got := e.stored(e.chat); len(got) != 2 || e.action(e.chat) != "mute" {
			t.Errorf("the menu's writes left %v and action %q", got, e.action(e.chat))
		}
		if reply := e.command(e.chat, e.owner, "/addfwdblock -1001000000012"); !strings.Contains(reply, staffMarker("staff_fwdblock_added")) {
			t.Errorf("the creator's /addfwdblock answered %q", reply)
		}
		if reply := e.command(e.chat, e.owner, "/fwdblockaction ban"); !strings.Contains(reply, staffMarker("staff_fwdblock_action_changed")) {
			t.Errorf("the creator's /fwdblockaction answered %q", reply)
		}
		if got := e.stored(e.chat); len(got) != 3 || e.action(e.chat) != "ban" {
			t.Errorf("the commands' writes left %v and action %q", got, e.action(e.chat))
		}
	})

	t.Run("an administrator the owner gate refuses is refused by the menu and not by the command", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		e.setAdminsAllowed(e.chat.Id, false)
		m := &settingsBlacklistMenu{env: e.settingsListEnv, chat: e.chat, user: e.admin, msgID: e.updateID()}
		e.wantAlert(m, "the gated administrator's page", "settings_gate_owner_only", "a", settingsActOpen, "p", settingsPageFwdBlock)
		e.wantAlert(m, "the gated administrator's pager", "settings_gate_owner_only", "a", "vg", "v", "1")
		if reply := e.command(e.chat, e.admin, "/fwdblocklist"); !strings.Contains(reply, staffMarker("staff_fwdblock_list_header")) {
			t.Errorf("the gated administrator's /fwdblocklist answered %q, want the list", reply)
		}
	})
}

// TestSettingsParityFwdBlock proves that the page and the commands leave the same row and the same cache state for the
// same input, through the shared apply functions: Staff Group A is worked by the commands and Staff Group B by the menu.
func TestSettingsParityFwdBlock(t *testing.T) {
	e := newSettingsFwdEnv(t)
	a, b := e.chat, e.otherStaffGroup()
	rowsOf := func(chat gotgbot.Chat) string {
		t.Helper()
		rows, err := staff.ListForwardBlocksFresh(context.Background(), chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		for _, row := range rows {
			fmt.Fprintf(&sb, "%d|%s|%d;", row.SourceChatID, row.SourceTitle, row.CreatedBy)
		}
		rules, err := staff.GetForwardBlockRules(context.Background(), chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sb, "action=%s;sources=%v;key=%v", rules.Action, rules.Sources, e.redis.Exists(dbcache.CacheKey("staff_forward_block", chat.Id)))
		return sb.String()
	}

	// Add by @username and by numeric ID: the command in A, the typed reply in B.
	for _, ref := range []string{"@goodchan", "-1001000000012"} {
		if reply := e.command(a, e.owner, "/addfwdblock "+ref); !strings.Contains(reply, staffMarker("staff_fwdblock_added")) {
			t.Fatalf("/addfwdblock %s answered %q", ref, reply)
		}
		e.begin(b, e.owner)
		settingsListDoorSay(e.settingsListEnv, e.owner, ref)
	}
	if got, want := rowsOf(b), rowsOf(a); got != want {
		t.Errorf("the page left %q, the command left %q", got, want)
	}

	// The action: /fwdblockaction in A, the choice button in B.
	e.command(a, e.owner, "/fwdblockaction kick")
	mb := e.open(b, e.owner)
	mb.press("a", "vs", "v", "kick")
	if got, want := rowsOf(b), rowsOf(a); got != want {
		t.Errorf("after the action the page left %q, the command left %q", got, want)
	}

	// Remove: /rmfwdblock by source in A, the delete card by row ID in B.
	if reply := e.command(a, e.owner, "/rmfwdblock -1001000000011"); !strings.Contains(reply, staffMarker("staff_fwdblock_removed")) {
		t.Fatalf("/rmfwdblock answered %q", reply)
	}
	mb.press("a", "vx", "r", strconv.FormatInt(e.idOf(b, -1001000000011), 10), "v", "1")
	_, keyboard := mb.last()
	yes, _ := settingsButtonByMarker(keyboard, "settings_btn_yes")
	mb.tap(yes)
	if got, want := rowsOf(b), rowsOf(a); got != want {
		t.Errorf("after the removal the page left %q, the command left %q", got, want)
	}
	if got := e.stored(b); len(got) != 1 || got[-1001000000012] == "" {
		t.Errorf("the page's removal left %v, want only the numeric group", got)
	}
}

// TestSettingsDriftFwdBlock proves that the page draws what the commands stored and what the watcher reads: a source
// added by command is listed and blocked, the action set by command is the one on the page and the one the watcher
// reads, and a removal by command empties both.
func TestSettingsDriftFwdBlock(t *testing.T) {
	e := newSettingsFwdEnv(t)
	rules := func() staff.ForwardBlockRules {
		t.Helper()
		got, err := staff.GetForwardBlockRules(context.Background(), e.chat.Id)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	page := func() string {
		t.Helper()
		_, text, _ := settingsListOpenPrivate(e.settingsListEnv, e.owner, settingsPageFwdBlock)
		return text
	}

	e.command(e.chat, e.owner, "/addfwdblock @goodchan")
	e.command(e.chat, e.owner, "/fwdblockaction ban")
	text := page()
	if !strings.Contains(text, "1. <code>-1001000000011 Good &lt;Channel&gt;</code>") || !strings.Contains(text, staffMarker("settings_bl_action_line")+" "+staffMarker("settings_opt_ban")) {
		t.Errorf("the page does not draw what the commands stored:\n%s", text)
	}
	if got := rules(); !got.IsBlocked(-1001000000011) || got.Action != "ban" {
		t.Errorf("the watcher reads %+v, want the listed source with the action ban", got)
	}
	// What the page writes is what the commands list and the watcher reads.
	m := e.open(e.chat, e.owner)
	m.press("a", "vs", "v", "warn")
	if reply := e.command(e.chat, e.owner, "/fwdblocklist"); !strings.Contains(reply, "-1001000000011") || !strings.Contains(reply, staffMarker("settings_opt_warn")) {
		t.Errorf("/fwdblocklist answered %q, want the source and the action the page set", reply)
	}
	if got := rules(); got.Action != "warn" {
		t.Errorf("the watcher reads the action %q, want warn", got.Action)
	}
	e.command(e.chat, e.owner, "/rmfwdblock -1001000000011")
	if text := page(); !strings.Contains(text, staffMarker("settings_fwd_empty")) || strings.Contains(text, "1001000000011") {
		t.Errorf("the page still draws the removed source:\n%s", text)
	}
	if got := rules(); got.IsBlocked(-1001000000011) {
		t.Errorf("the watcher still blocks the removed source: %+v", got)
	}
}
