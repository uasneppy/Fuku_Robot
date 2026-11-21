//go:build testtools

package modules

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// settingsMsgValidPresses are one valid press of each action code of the Messages page, its fields besides the private owner
// field c, keyed by a name (the code, or the code and d=p for the private copy's form of an editor code, Phase 22). The
// action code is the second item.
var settingsMsgValidPresses = map[string][]string{
	settingsActMsgFamily:  {"a", settingsActMsgFamily, "t", "m"},
	settingsActMsgEvent:   {"a", settingsActMsgEvent, settingsMsgEventField, "bn"},
	settingsActMsgSet:     {"a", settingsActMsgSet, "t", settingsEditPartText, settingsMsgEventField, "bn"},
	settingsActMsgRemove:  {"a", settingsActMsgRemove, "t", settingsEditPartText, settingsMsgEventField, "bn"},
	settingsActMsgCancel:  {"a", settingsActMsgCancel, "t", settingsEditPartText, settingsMsgEventField, "bn"},
	settingsActMsgPreview: {"a", settingsActMsgPreview, "t", settingsEditPartText, settingsMsgEventField, "bn"},
	settingsActMsgFull:    {"a", settingsActMsgFull, settingsMsgEventField, "bn"},
	settingsActMsgSilent:  {"a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1"},
	// Phase 22: the private copy's switch, the hub's option toggle and the d=p form of every event and editor code.
	settingsActMsgPM:               {"a", settingsActMsgPM, settingsMsgEventField, "bn", "v", "1"},
	settingsActMsgPMOption:         {"a", settingsActMsgPMOption, "t", "d", "v", "1"},
	settingsActMsgEvent + " d=p":   {"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	settingsActMsgSet + " d=p":     {"a", settingsActMsgSet, "t", settingsEditPartText, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	settingsActMsgRemove + " d=p":  {"a", settingsActMsgRemove, "t", settingsEditPartText, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	settingsActMsgCancel + " d=p":  {"a", settingsActMsgCancel, "t", settingsEditPartText, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	settingsActMsgPreview + " d=p": {"a", settingsActMsgPreview, "t", settingsEditPartText, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	settingsActMsgFull + " d=p":    {"a", settingsActMsgFull, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
}

// settingsMsgForgedPress is one forged press: in a group menu (the opener u in place of c) or in a private menu, with the
// fields given (the private ones get c added unless the fields carry u).
type settingsMsgForgedPress struct {
	name  string
	group bool
	kv    []string
}

// settingsMsgWith is kv with the value of key replaced, or the pair appended when kv has no such key.
func settingsMsgWith(kv []string, key, value string) []string {
	out := append([]string{}, kv...)
	for i := 0; i+1 < len(out); i += 2 {
		if out[i] == key {
			out[i+1] = value
			return out
		}
	}
	return append(out, key, value)
}

// settingsMsgForgedPresses are the forged presses of every valid press: a group menu's press, a private press carrying u in
// place of c, an unknown event (or family for hl, option for ho), an unknown part for the part actions, a target other than 0
// or 1 for the switches, an extra field and, for the d=p forms, a destination other than p (Phase 22, D-26).
func settingsMsgForgedPresses() []settingsMsgForgedPress {
	var forged []settingsMsgForgedPress
	for name, kv := range settingsMsgValidPresses {
		code := kv[1]
		forged = append(forged,
			settingsMsgForgedPress{name: name + " from a group menu", group: true, kv: kv},
			settingsMsgForgedPress{name: name + " carrying u in a private menu", kv: append(append([]string{}, kv...), "u", "1234567890")},
			settingsMsgForgedPress{name: name + " with an extra field", kv: append(append([]string{}, kv...), "e", "1")},
		)
		switch code {
		case settingsActMsgFamily:
			forged = append(forged, settingsMsgForgedPress{name: name + " of an unknown family", kv: settingsMsgWith(kv, "t", "z")})
		case settingsActMsgPMOption:
			forged = append(forged, settingsMsgForgedPress{name: name + " of an unknown option", kv: settingsMsgWith(kv, "t", "x")})
		default:
			forged = append(forged, settingsMsgForgedPress{name: name + " of an unknown event", kv: settingsMsgWith(kv, settingsMsgEventField, "zz")})
		}
		switch code {
		case settingsActMsgSet, settingsActMsgRemove, settingsActMsgCancel, settingsActMsgPreview:
			forged = append(forged, settingsMsgForgedPress{name: name + " of an unknown part", kv: settingsMsgWith(kv, "t", "q")})
		case settingsActMsgSilent, settingsActMsgPM, settingsActMsgPMOption:
			forged = append(forged, settingsMsgForgedPress{name: name + " with v=2", kv: settingsMsgWith(kv, "v", "2")})
		}
		if slicesContainsKey(kv, settingsMsgDestField) {
			for _, d := range []string{"x", settingsMsgDestGroupCode} {
				forged = append(forged, settingsMsgForgedPress{name: name + " with d=" + d, kv: settingsMsgWith(kv, settingsMsgDestField, d)})
			}
		}
	}
	return forged
}

// pressForged makes one forged press of chat as user on message msgID (of the group chat for a group press, of the user's
// private chat otherwise).
func (e *settingsMsgEnv) pressForged(chat gotgbot.Chat, user gotgbot.User, msgID int64, f settingsMsgForgedPress) {
	e.t.Helper()
	switch {
	case f.group:
		e.pressRaw(chat, user, msgID, encodeCallbackData(settingsCallbackNamespace, e.groupFields(user, f.kv...)))
	case slicesContainsKey(f.kv, "u"):
		fields := map[string]string{}
		for i := 0; i+1 < len(f.kv); i += 2 {
			fields[f.kv[i]] = f.kv[i+1]
		}
		e.pressPrivate(user, msgID, fields)
	default:
		e.pressPrivate(user, msgID, e.privateFields(chat.Id, f.kv...))
	}
}

// slicesContainsKey reports whether the key, value pairs kv carry key.
func slicesContainsKey(kv []string, key string) bool {
	for i := 0; i < len(kv); i += 2 {
		if kv[i] == key {
			return true
		}
	}
	return false
}

func TestSettingsMessagesForged(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	admin := env.admin
	groupMsg := env.openMenu(admin)
	const privateMsg = 8101

	for _, f := range settingsMsgForgedPresses() {
		msgID, editChat := int64(privateMsg), admin.Id
		if f.group {
			msgID, editChat = groupMsg, env.chat.Id
		}
		members, chats, answers := len(env.fake.callsFor("getChatMember")), len(env.fake.callsFor("getChat")), env.answerCount()
		edits := len(env.edits(editChat, msgID))
		env.pressForged(env.chat, admin, msgID, f)
		if got := env.answerCount() - answers; got != 1 {
			t.Errorf("%s was answered %d times, want once", f.name, got)
		}
		if text, alert := env.lastAnswer(); !containsMarker(text, "common_callback_invalid_request") || !alert {
			t.Errorf("%s answered %q (alert %v), want the invalid request alert", f.name, text, alert)
		}
		if got := len(env.fake.callsFor("getChatMember")) - members; got != 0 {
			t.Errorf("%s made %d getChatMember calls, want none", f.name, got)
		}
		if got := len(env.fake.callsFor("getChat")) - chats; got != 0 {
			t.Errorf("%s made %d getChat calls, want none", f.name, got)
		}
		if got := len(env.edits(editChat, msgID)); got != edits {
			t.Errorf("%s edited the menu", f.name)
		}
	}
	if n := settingsMsgRows(t, env.chat.Id); n != 0 {
		t.Errorf("the forged presses left %d event message rows, want none", n)
	}
	if env.redis.Exists(settingsInputKey(admin.Id)) {
		t.Errorf("the forged presses left a wait")
	}

	// A valid press of each code passes step 1: it reaches the live lookup of the presser and is not the invalid request.
	for name, kv := range settingsMsgValidPresses {
		lookups := env.memberLookupsOf(env.chat.Id, admin.Id)
		env.pressPrivate(admin, env.updateID(), env.privateFields(env.chat.Id, kv...))
		if got := env.memberLookupsOf(env.chat.Id, admin.Id) - lookups; got != 1 {
			t.Errorf("a valid %s press made %d lookups of the presser, want 1", name, got)
		}
		if text, _ := env.lastAnswer(); containsMarker(text, "common_callback_invalid_request") {
			t.Errorf("a valid %s press was answered as an invalid request", name)
		}
		env.redis.Del(settingsInputKey(admin.Id))
		env.redis.Del(settingsInputDoneKey(admin.Id))
	}
}

// settingsMsgSafetyCounts are the rows a draw of the Messages page could create for chatID: its event messages, its chats row
// and its rules row.
func settingsMsgSafetyCounts(t *testing.T, chatID int64) (events, chats, rules int64) {
	t.Helper()
	for _, c := range []struct {
		model any
		n     *int64
	}{{&models.ChatEventMessage{}, &events}, {&models.Chat{}, &chats}, {&models.RulesSettings{}, &rules}} {
		if err := db.DB.Model(c.model).Where("chat_id = ?", chatID).Count(c.n).Error; err != nil {
			t.Fatalf("count the rows of chat %d: %v", chatID, err)
		}
	}
	return events, chats, rules
}

func TestSettingsMessagesReadsCreateNothing(t *testing.T) {
	env := newSettingsMsgEnv(t)
	for _, kind := range []string{"group menu", "private menu"} {
		t.Run(kind, func(t *testing.T) {
			private := kind == "private menu"
			chat := env.addChat()
			events, chats, rules := settingsMsgSafetyCounts(t, chat.Id)
			if events != 0 || rules != 0 {
				t.Fatalf("the fresh chat %d already has %d event message rows and %d rules rows", chat.Id, events, rules)
			}
			const msgID = 8102
			open := func(kv ...string) { settingsPhase18Open(env.settingsEnv, chat, msgID, private, kv...) }
			open("a", settingsActBack)
			open("a", settingsActOpen, "p", settingsPageMessages)
			if private {
				open("a", settingsActMsgFamily, "t", "m")
				for _, ev := range settingsMsgManualEvents {
					open("a", settingsActMsgEvent, settingsMsgEventField, ev.Code)
					// Phase 22: the event's private copy screen.
					open("a", settingsActMsgEvent, settingsMsgEventField, ev.Code, settingsMsgDestField, settingsMsgDestPMCode)
				}
				if text, _ := env.lastEdit(env.admin.Id, msgID); !containsMarker(text, "settings_msg_pm_intro") {
					t.Errorf("the run's last edit is %q, want the last event's private copy screen", text)
				}
				// A private copy's Full preview reads the default private text and sends it to the presser alone.
				sent := len(env.fake.sentTo(env.admin.Id))
				open("a", settingsActMsgFull, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode)
				if got := len(env.fake.sentTo(env.admin.Id)) - sent; got < 2 {
					t.Errorf("the private copy's Full preview sent %d messages to the presser, want the header and the copy", got)
				}
				open("a", settingsActOpen, "p", settingsPageMessages)
			} else if text, _ := env.lastEdit(chat.Id, msgID); !containsMarker(text, "settings_msg_group_note") {
				t.Errorf("the run's last edit is %q, want the group note", text)
			}
			for _, f := range settingsMsgForgedPresses() {
				env.pressForged(chat, env.admin, env.updateID(), f)
			}
			if gotEvents, gotChats, gotRules := settingsMsgSafetyCounts(t, chat.Id); gotEvents != 0 || gotChats != chats || gotRules != 0 {
				t.Errorf("drawing the page left %d event message rows and %d rules rows and changed the chat rows from %d to %d",
					gotEvents, gotRules, chats, gotChats)
			}
			var options int64
			if err := db.DB.Model(&models.ChatEventMessageOptions{}).Where("chat_id = ?", chat.Id).Count(&options).Error; err != nil {
				t.Fatalf("count the options rows of chat %d: %v", chat.Id, err)
			}
			if options != 0 {
				t.Errorf("drawing the page left %d private copy options rows, want none", options)
			}
			if key := eventmsg.CacheKey(chat.Id); env.redis.Exists(key) {
				t.Errorf("drawing the page filled %s", key)
			}
		})
	}

	t.Run("a failing read is the cannot-open edit on every screen and the unknown mark on Main", func(t *testing.T) {
		mreqSwap(t, &settingsMsgRead, func(context.Context, int64) (*eventmsg.Messages, error) {
			return nil, errors.New("event messages are unreadable")
		})
		for name, kv := range map[string][]string{
			"the hub":         {"a", settingsActOpen, "p", settingsPageMessages},
			"the family list": {"a", settingsActMsgFamily, "t", "m"},
			"an event screen": {"a", settingsActMsgEvent, settingsMsgEventField, "tm"},
		} {
			msgID := env.updateID()
			env.pressPrivate(env.admin, msgID, env.privateFields(env.chat.Id, kv...))
			text, keyboard := env.lastEdit(env.admin.Id, msgID)
			if !containsMarker(text, "settings_cannot_open") || len(keyboard) != 0 {
				t.Errorf("%s with a failing read drew %q with %d rows, want the cannot-open edit", name, text, len(keyboard))
			}
		}
		if state, ok := settingsMsgMainState(env.bot, i18n.MustNewTranslator("en"), &env.chat); ok || state != "" {
			t.Errorf("Main's Messages state with a failing read = %q (known %v), want unknown", state, ok)
		}
	})
}

// settingsMsgCountReads wraps settingsMsgRead with a counter for the test.
func settingsMsgCountReads(t *testing.T) *atomic.Int64 {
	t.Helper()
	var reads atomic.Int64
	read := settingsMsgRead
	mreqSwap(t, &settingsMsgRead, func(ctx context.Context, chatID int64) (*eventmsg.Messages, error) {
		reads.Add(1)
		return read(ctx, chatID)
	})
	return &reads
}

func TestSettingsMessagesCost(t *testing.T) {
	env := newSettingsMsgEnv(t)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	admin := env.admin
	settingsMsgSeed(t, env.chat.Id, eventmsg.EventBan, "Banned {mention}", false)
	reads := settingsMsgCountReads(t)
	const msgID = 8103
	press := func(kv ...string) map[string]int {
		before := settingsPhase18Calls(env.settingsEnv)
		env.pressPrivate(admin, msgID, env.privateFields(env.chat.Id, kv...))
		return settingsPhase18CallsSince(env.settingsEnv, before)
	}
	draws := map[string][]string{
		"the hub":         {"a", settingsActOpen, "p", settingsPageMessages},
		"the family list": {"a", settingsActMsgFamily, "t", "m"},
		"an event screen": {"a", settingsActMsgEvent, settingsMsgEventField, "bn"},
		// Phase 22: a private copy screen.
		"a private copy screen": {"a", settingsActMsgEvent, settingsMsgEventField, "bn", settingsMsgDestField, settingsMsgDestPMCode},
	}
	pipeline := map[string]int{"getChatMember": 1, "getChat": 1, "editMessageText": 1, "answerCallbackQuery": 1}

	t.Run("Main's Messages line is one read and no Telegram call, and a group Main reads nothing", func(t *testing.T) {
		before, r := settingsPhase18Calls(env.settingsEnv), reads.Load()
		if _, ok := settingsMsgMainState(env.bot, i18n.MustNewTranslator("en"), &env.chat); !ok {
			t.Fatalf("Main's Messages state is unknown")
		}
		if got := reads.Load() - r; got != 1 {
			t.Errorf("Main's Messages line made %d reads, want 1", got)
		}
		settingsMainEntries(env.bot, i18n.MustNewTranslator("en"), &env.chat, true, false)
		if got := reads.Load() - r; got != 1 {
			t.Errorf("a group Main raised the reads to %d, want no read of its own", got)
		}
		if delta := settingsPhase18CallsSince(env.settingsEnv, before); len(delta) != 0 {
			t.Errorf("the Messages line made Telegram calls %v, want none", delta)
		}
	})

	t.Run("a draw is one read and the pipeline's calls", func(t *testing.T) {
		for name, kv := range draws {
			r := reads.Load()
			if delta := press(kv...); !maps.Equal(delta, pipeline) {
				t.Errorf("%s made %v, want %v", name, delta, pipeline)
			}
			if got := reads.Load() - r; got != 1 {
				t.Errorf("%s made %d reads, want 1", name, got)
			}
		}
	})

	t.Run("a press refused in step 1 costs its answer and nothing else", func(t *testing.T) {
		r := reads.Load()
		for _, f := range settingsMsgForgedPresses() {
			if f.group {
				continue
			}
			before := settingsPhase18Calls(env.settingsEnv)
			env.pressForged(env.chat, admin, msgID, f)
			if delta, want := settingsPhase18CallsSince(env.settingsEnv, before), (map[string]int{"answerCallbackQuery": 1}); !maps.Equal(delta, want) {
				t.Errorf("%s made %v, want %v", f.name, delta, want)
			}
		}
		if got := reads.Load() - r; got != 0 {
			t.Errorf("refused presses made %d reads, want none", got)
		}
	})

	t.Run("with Redis down every screen draws, Silent and Remove write and Set is the alert", func(t *testing.T) {
		up := make(map[string]map[string]int)
		for name, kv := range draws {
			up[name] = press(kv...)
		}
		restore := cache.DisableRedisForTest()
		defer restore()
		for name, kv := range draws {
			if got := press(kv...); !maps.Equal(got, up[name]) {
				t.Errorf("with Redis down %s made %v, with Redis %v", name, got, up[name])
			}
		}
		press("a", settingsActMsgSilent, settingsMsgEventField, "kk", "v", "1")
		if row, ok := settingsMsgStored(t, env.chat.Id, eventmsg.EventKick); !ok || !row.Silent {
			t.Errorf("Silent with Redis down stored %+v (row %v), want the kick Silent", row, ok)
		}
		press("a", settingsActMsgRemove, "t", settingsEditPartText, settingsMsgEventField, "bn")
		if row, _ := settingsMsgStored(t, env.chat.Id, eventmsg.EventBan); row.Text != "" {
			t.Errorf("Remove with Redis down left the ban text %q", row.Text)
		}
		edits := len(env.edits(admin.Id, msgID))
		press("a", settingsActMsgSet, "t", settingsEditPartText, settingsMsgEventField, "bn")
		if text, alert := env.lastAnswer(); !alert || !containsMarker(text, "settings_edit_unavailable") {
			t.Errorf("Set with Redis down answered %q (alert %v), want the unavailable alert", text, alert)
		}
		if got := len(env.edits(admin.Id, msgID)); got != edits {
			t.Errorf("Set with Redis down edited the menu")
		}
	})
}

// The sentinels of the log sweep: a stored text, a file ID, a caption, a button name and a button link of an event
// message. They carry no digit, so a search for one is unambiguous.
const (
	settingsMsgSentinelText    = "zzmsgsentineltext"
	settingsMsgSentinelFile    = "zzmsgsentinelfile"
	settingsMsgSentinelCaption = "zzmsgsentinelcaption"
	settingsMsgSentinelButton  = "zzmsgsentinelbutton"
	settingsMsgSentinelLink    = "https://zzmsgsentinel.example/link"
)

func TestSettingsMessagesLogsCarryIDsOnly(t *testing.T) {
	logged := newSettingsListLogged(t)
	start := logged.mark()
	env := newSettingsMsgEnv(t)
	settingsEditFakeSends(env.settingsEnv)
	listEditAdmin(env.settingsEnv, env.chat, env.admin, true)
	admin, chat := env.admin, env.chat
	ctx := context.Background()
	if err := eventmsg.SetText(ctx, chat.Id, eventmsg.EventBan, eventmsg.DestGroup, settingsMsgSentinelText, 1); err != nil {
		t.Fatalf("seed the ban text: %v", err)
	}
	if err := eventmsg.SetMedia(ctx, chat.Id, eventmsg.EventBan, eventmsg.DestGroup, settingsMsgSentinelFile, db.PHOTO, 1); err != nil {
		t.Fatalf("seed the ban media: %v", err)
	}
	if err := eventmsg.SetButtons(ctx, chat.Id, eventmsg.EventBan, eventmsg.DestGroup,
		[]db.Button{{Name: settingsMsgSentinelButton, Url: settingsMsgSentinelLink}}, 1); err != nil {
		t.Fatalf("seed the ban buttons: %v", err)
	}
	broken := errors.New("the database is down")
	press := func(kv ...string) { env.pressPrivate(admin, env.updateID(), env.privateFields(chat.Id, kv...)) }

	t.Run("a read that fails on every screen", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &settingsMsgRead, func(context.Context, int64) (*eventmsg.Messages, error) { return nil, broken })
		press("a", settingsActOpen, "p", settingsPageMessages)
		press("a", settingsActMsgFamily, "t", "m")
		press("a", settingsActMsgEvent, settingsMsgEventField, "bn")
		logged.wantChat(t, "a failing read", mark, chat.Id)
	})

	t.Run("a Silent switch whose write fails", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &eventMsgWrites.Silent, func(context.Context, int64, string, string, bool, int64) error { return broken })
		press("a", settingsActMsgSilent, settingsMsgEventField, "bn", "v", "1")
		logged.wantChat(t, "a failing Silent write", mark, chat.Id)
	})

	t.Run("a Remove of each part whose write fails", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &eventMsgWrites.Text, func(context.Context, int64, string, string, string, int64) error { return broken })
		mreqSwap(t, &eventMsgWrites.Media, func(context.Context, int64, string, string, string, int, int64) error { return broken })
		mreqSwap(t, &eventMsgWrites.Buttons, func(context.Context, int64, string, string, []db.Button, int64) error { return broken })
		for _, part := range []string{settingsEditPartText, settingsEditPartMedia, settingsEditPartButtons} {
			press("a", settingsActMsgRemove, "t", part, settingsMsgEventField, "bn")
		}
		logged.wantChat(t, "a failing Remove", mark, chat.Id)
		if row, _ := settingsMsgStored(t, chat.Id, eventmsg.EventBan); row.Text != settingsMsgSentinelText || row.FileID != settingsMsgSentinelFile {
			t.Errorf("a failing Remove changed the ban row: %+v", row)
		}
	})

	t.Run("a typed text, media and buttons whose saves fail", func(t *testing.T) {
		mark := logged.mark()
		mreqSwap(t, &eventMsgWrites.Text, func(context.Context, int64, string, string, string, int64) error { return broken })
		mreqSwap(t, &eventMsgWrites.Media, func(context.Context, int64, string, string, string, int, int64) error { return broken })
		mreqSwap(t, &eventMsgWrites.Buttons, func(context.Context, int64, string, string, []db.Button, int64) error { return broken })
		photo := settingsMediaItemID("photo", settingsMsgSentinelFile)
		photo.Caption = settingsMsgSentinelCaption
		for part, msg := range map[string]gotgbot.Message{
			settingsEditPartText:    {Text: settingsMsgSentinelText + " {first}"},
			settingsEditPartMedia:   photo,
			settingsEditPartButtons: {Text: "[" + settingsMsgSentinelButton + "](buttonurl://" + settingsMsgSentinelLink + ")"},
		} {
			settingsMsgSetWait(t, env, admin, "kk", part)
			before := settingsEditSentCount(env.settingsEnv, admin)
			sendPrivateMessage(env.settingsEnv, admin, msg)
			settingsEditWantReply(t, env.settingsEnv, admin, before, "common_settings_save_failed")
			env.redis.Del(settingsInputKey(admin.Id))
			env.redis.Del(settingsInputDoneKey(admin.Id))
		}
		logged.wantChat(t, "a failing typed save", mark, chat.Id)
		if _, ok := settingsMsgStored(t, chat.Id, eventmsg.EventKick); ok {
			t.Errorf("a failing typed save stored a kick row")
		}
	})

	// A 403 (the presser never started the bot) is the presser's own doing and answered with the need-start alert, with no
	// log line by design; any other refused preview is logged, by user and chat, with Telegram's error and never the content.
	t.Run("a preview the presser's private chat refuses", func(t *testing.T) {
		env.fake.script("sendMessage", admin.Id, staffFakeError(403, "Forbidden: bot can't initiate conversation with a user"))
		press("a", settingsActMsgFull, settingsMsgEventField, "bn")
		if text, alert := env.lastAnswer(); !alert || !containsMarker(text, "settings_edit_preview_need_start") {
			t.Errorf("a refused preview answered %q (alert %v), want the need-start alert", text, alert)
		}
		mark := logged.mark()
		env.fake.script("sendPhoto", admin.Id, staffFakeError(400, "Bad Request: wrong file identifier/HTTP URL specified"))
		press("a", settingsActMsgFull, settingsMsgEventField, "bn")
		if text, alert := env.lastAnswer(); !alert || !containsMarker(text, "settings_edit_preview_failed") {
			t.Errorf("a preview whose content was refused answered %q (alert %v), want the failed alert", text, alert)
		}
		logged.wantChat(t, "a preview whose content was refused", mark, chat.Id)
	})

	for _, line := range logged.since(start) {
		for _, sentinel := range []string{
			settingsMsgSentinelText, settingsMsgSentinelFile, settingsMsgSentinelCaption, settingsMsgSentinelButton, "zzmsgsentinel.example",
		} {
			if strings.Contains(line, sentinel) {
				t.Errorf("a log line carries the stored or typed %q: %q", sentinel, line)
			}
		}
	}
}
