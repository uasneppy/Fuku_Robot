//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// seedStaffUsers inserts users rows for the test and removes them, and the local
// read-through layer that may have cached them, when it ends.
func seedStaffUsers(t *testing.T, rows ...models.User) {
	t.Helper()
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].UserId)
	}
	db.DB.Where("user_id IN ?", ids).Delete(&models.User{})
	t.Cleanup(func() {
		db.DB.Where("user_id IN ?", ids).Delete(&models.User{})
		dbcache.ResetLocalForTest()
	})
	for i := range rows {
		if err := db.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("seed user %d: %v", rows[i].UserId, err)
		}
	}
	dbcache.ResetLocalForTest()
}

// wantNoWrites fails when any ban, restrict or unban call reached any chat.
func (e *staffActionEnv) wantNoWrites() {
	e.t.Helper()
	for _, method := range []string{"banChatMember", "restrictChatMember", "unbanChatMember", "deleteMessage"} {
		if calls := e.fake.callsFor(method); len(calls) != 0 {
			e.t.Fatalf("%s calls = %d, want 0", method, len(calls))
		}
	}
}

// confirmLastCard taps Confirm on the last card as the issuer and waits for the run.
func (e *staffActionEnv) confirmLastCard() {
	e.t.Helper()
	token, msgID := e.card()
	e.tap(e.issuer, staffActRunConfirm, token, msgID)
	e.waitRuns()
}

// wantBanCall expects exactly one banChatMember in chatID, aimed at userID.
func (e *staffActionEnv) wantBanCall(chatID, userID int64) {
	e.t.Helper()
	calls := e.callsTo("banChatMember", chatID)
	if len(calls) != 1 || staffParamInt(calls[0].Params, "user_id") != userID {
		e.t.Fatalf("banChatMember calls in chat %d = %+v, want one aimed at %d", chatID, calls, userID)
	}
}

func TestStaffActionUsernameTarget(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	seedStaffUsers(t, models.User{UserId: staffTestTarget, UserName: "spambot1", Name: "Spam Bot", LastActivity: time.Now()})
	env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban @SPAMBOT1 spamming")

	env.wantCardOnly()
	card := env.lastCardText()
	if !strings.Contains(card, "Spam Bot") || !strings.Contains(card, "4242") || !strings.Contains(card, "spamming") {
		t.Fatalf("card = %q, want the resolved name, the numeric ID and the reason", card)
	}
	env.wantNoWrites()

	env.confirmLastCard()
	env.wantBanCall(g, staffTestTarget)
}

func TestStaffActionUsernameCaseInsensitive(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	seedStaffUsers(t, models.User{UserId: staffTestTarget, UserName: "MixedCase_Name", Name: "Mixed", LastActivity: time.Now()})

	for _, typed := range []string{"@mixedcase_name", "@MIXEDCASE_NAME", "@MixedCase_Name"} {
		env.send(env.issuer, "/kick "+typed)
		if card := env.lastCardText(); !strings.Contains(card, "Mixed") || !strings.Contains(card, "4242") {
			t.Fatalf("/kick %s card = %q, want the resolved user", typed, card)
		}
	}
}

func TestStaffActionUsernameUnknown(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban @nobody_here")

	env.wantReplyMarker("staff_act_username_unknown")
	if text := textsToChat(env.fake.staffBotClient, env.staffChat)[0]; !strings.Contains(text, "nobody_here") {
		t.Fatalf("reply = %q, want it to name the username", text)
	}
	env.wantNoWrites()
	if lookups := env.fake.callsFor("getChat"); len(lookups) != 0 {
		t.Fatalf("getChat calls = %d, want 0: a username is never resolved through live Telegram", len(lookups))
	}
}

func TestStaffActionUsernameNeverFallsBackToChannels(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	name := fmt.Sprintf("chan_only_%d", env.owner)
	if err := db.DB.Create(&models.ChannelSettings{ChatId: env.staffChat - 1, ChannelId: -1001234567890, ChannelName: "x", Username: name}).Error; err != nil {
		t.Fatalf("seed a channel with that username: %v", err)
	}
	t.Cleanup(func() { db.DB.Where("chat_id = ?", env.staffChat-1).Delete(&models.ChannelSettings{}) })

	env.send(env.issuer, "/ban @"+name)

	env.wantReplyMarker("staff_act_username_unknown")
	env.wantNoWrites()
}

func TestStaffActionUsernameAmbiguous(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	env.fake.setMember(env.groups[0], 910001, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	suffix := env.owner
	upper, lower := fmt.Sprintf("Dupe_%d", suffix), fmt.Sprintf("dupe_%d", suffix)
	older := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	seedStaffUsers(t,
		models.User{UserId: 910001, UserName: upper, Name: "<b>Evil</b> & Co", LastActivity: older},
		models.User{UserId: 910002, UserName: lower, Name: "Plain One", LastActivity: newer},
	)

	env.send(env.issuer, "/ban @"+strings.ToUpper(upper))

	env.wantReplyMarker("staff_act_username_ambiguous")
	text := textsToChat(env.fake.staffBotClient, env.staffChat)[0]
	for _, want := range []string{
		"910001", "910002",
		"&lt;b&gt;Evil&lt;/b&gt; &amp; Co", "Plain One",
		staffMarker("staff_act_username_last_seen"), "2026-03-01", "2026-04-02",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reply = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, "<b>Evil</b>") {
		t.Fatalf("reply = %q, a stored name must be HTML-escaped", text)
	}
	if strings.Index(text, "910002") > strings.Index(text, "910001") {
		t.Fatalf("reply = %q, want the most recently seen user listed first", text)
	}
	env.wantNoWrites()
}

func TestStaffActionUsernameAmbiguousSameCaseDuplicates(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	name := fmt.Sprintf("twin_%d", env.owner)
	seedStaffUsers(t,
		models.User{UserId: 910011, UserName: name, Name: "Twin A", LastActivity: time.Now().Add(-time.Hour)},
		models.User{UserId: 910012, UserName: name, Name: "Twin B", LastActivity: time.Now()},
	)

	env.send(env.issuer, "/mute @"+name+" 1h")

	env.wantReplyMarker("staff_act_username_ambiguous")
	env.wantNoWrites()
}

func TestStaffActionUsernameLookupError(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	previous := staffUserLookup
	t.Cleanup(func() { staffUserLookup = previous })
	staffUserLookup = func(string, int) ([]models.User, error) { return nil, errors.New("database is down") }

	env.send(env.issuer, "/ban @someone_real")

	env.wantReplyMarker("staff_act_abort_check_failed")
	for _, text := range textsToChat(env.fake.staffBotClient, env.staffChat) {
		if strings.Contains(text, staffMarker("staff_act_username_unknown")) {
			t.Fatalf("reply = %q, a database error must never read as \"never seen\"", text)
		}
	}
	env.wantNoWrites()
}

func TestStaffActionUsernameLookupAsksForMoreThanOne(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	var gotName string
	var gotLimit int
	previous := staffUserLookup
	t.Cleanup(func() { staffUserLookup = previous })
	staffUserLookup = func(name string, limit int) ([]models.User, error) {
		gotName, gotLimit = name, limit
		return nil, nil
	}

	env.send(env.issuer, "/ban @Some_Name")

	if gotName != "Some_Name" || gotLimit != staffUsernameMatchLimit || staffUsernameMatchLimit < 2 {
		t.Fatalf("lookup got (%q, %d), want the typed name without the @ and a limit of %d (more than one row, so an ambiguity can be seen)",
			gotName, gotLimit, staffUsernameMatchLimit)
	}
}

func TestStaffActionBadUsername(t *testing.T) {
	env := newStaffActionEnv(t, 1)

	for _, command := range []string{"/ban @ab spam", "/ban @bad-name", "/ban @"} {
		env.send(env.issuer, command)
	}

	texts := textsToChat(env.fake.staffBotClient, env.staffChat)
	if len(texts) != 3 {
		t.Fatalf("messages to the Staff Group = %q, want one hint per command", texts)
	}
	for _, text := range texts {
		if !strings.Contains(text, staffMarker("staff_act_hint_bad_username")) {
			t.Fatalf("reply = %q, want the bad username hint", text)
		}
	}
	env.wantNoCard()
	env.wantNoWrites()
}

func TestStaffActionTextMention(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	const mentioned int64 = 424242
	env.fake.setMember(g, mentioned, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	update := env.messageUpdate(env.staffChatObj(), &env.issuer, "/ban Zoë 🙂 <Smith> spamming now")
	update.Message.Entities = []gotgbot.MessageEntity{
		{Type: "bot_command", Offset: 0, Length: 4},
		{Type: "text_mention", Offset: 5, Length: 14, User: &gotgbot.User{Id: mentioned, FirstName: "Zoë 🙂", LastName: "<Smith>"}},
	}
	env.process(update)

	env.wantCardOnly()
	card := env.lastCardText()
	if !strings.Contains(card, "Zoë 🙂 &lt;Smith&gt;") || !strings.Contains(card, "424242") {
		t.Fatalf("card = %q, want the escaped mention name and the entity user ID", card)
	}
	if !strings.Contains(card, "spamming now") {
		t.Fatalf("card = %q, want the reason \"spamming now\" intact after the mention", card)
	}
	env.wantNoWrites()

	env.confirmLastCard()
	env.wantBanCall(g, mentioned)
}

func TestStaffActionBareReplyHint(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	colleague := gotgbot.User{Id: env.issuer.Id + 100, FirstName: "Colleague"}
	env.fake.setMember(g, colleague.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})

	update := env.messageUpdate(env.staffChatObj(), &env.issuer, "/ban")
	update.Message.ReplyToMessage = &gotgbot.Message{MessageId: 1, Date: 1, Chat: env.staffChatObj(), From: &colleague, Text: "forwarded spam"}
	env.process(update)

	env.wantReplyMarker("staff_act_hint_no_reply")
	env.wantNoWrites()
	if lookups := env.fake.callsFor("getChatMember"); len(lookups) != 0 {
		t.Fatalf("getChatMember calls = %d, want 0: the replied-to sender is never looked up", len(lookups))
	}
}

func TestStaffActionReplyWithExplicitTarget(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	colleague := gotgbot.User{Id: env.issuer.Id + 100, FirstName: "Colleague"}
	env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g, colleague.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	update := env.messageUpdate(env.staffChatObj(), &env.issuer, "/ban 4242 spam")
	update.Message.ReplyToMessage = &gotgbot.Message{MessageId: 1, Date: 1, Chat: env.staffChatObj(), From: &colleague, Text: "look"}
	env.process(update)

	env.wantCardOnly()
	card := env.lastCardText()
	if !strings.Contains(card, "4242") || strings.Contains(card, "Colleague") {
		t.Fatalf("card = %q, want the named target and never the replied-to sender", card)
	}

	env.confirmLastCard()
	env.wantBanCall(g, staffTestTarget)
	if m := env.fake.member(g, colleague.Id); m == nil || m.Status != gotgbot.ChatMemberStatusMember {
		t.Fatalf("replied-to member = %+v, want untouched", m)
	}
}

func TestStaffActionRefusedVariants(t *testing.T) {
	for _, command := range []string{"sban", "dban", "skick", "dkick", "smute", "dmute"} {
		t.Run(command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			// The per-group modules are loaded as well, so a variant that slipped past the
			// interceptor would act locally and this test would see it.
			full := staffDispatcher(t, true)

			update := env.messageUpdate(env.staffChatObj(), &env.issuer, "/"+command+" 4242")
			if err := full.ProcessUpdate(env.bot, update, nil); err != nil {
				t.Fatalf("ProcessUpdate error = %v", err)
			}

			env.wantReplyMarker("staff_act_hint_variant")
			if text := textsToChat(env.fake.staffBotClient, env.staffChat)[0]; !strings.Contains(text, command) {
				t.Fatalf("reply = %q, want it to name /%s", text, command)
			}
			env.wantNoWrites()
			if m := env.fake.member(env.groups[0], staffTestTarget); m == nil || m.Status != gotgbot.ChatMemberStatusMember {
				t.Fatalf("target = %+v, want untouched in the linked group", m)
			}
		})
	}
}

func TestStaffActionRefusedVariantsWithoutArguments(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	full := staffDispatcher(t, true)

	for _, command := range []string{"/dban", "/dmute", "/skick"} {
		update := env.messageUpdate(env.staffChatObj(), &env.issuer, command)
		update.Message.ReplyToMessage = &gotgbot.Message{MessageId: 1, Date: 1, Chat: env.staffChatObj(), From: &gotgbot.User{Id: 555009, FirstName: "Other"}, Text: "x"}
		if err := full.ProcessUpdate(env.bot, update, nil); err != nil {
			t.Fatalf("ProcessUpdate(%s) error = %v", command, err)
		}
	}

	for _, text := range textsToChat(env.fake.staffBotClient, env.staffChat) {
		if !strings.Contains(text, staffMarker("staff_act_hint_variant")) {
			t.Fatalf("reply = %q, want only the variant hint", text)
		}
	}
	env.wantNoCard()
	env.wantNoWrites()
}
