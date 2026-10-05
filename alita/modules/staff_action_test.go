//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

const staffTestTarget int64 = 4242

// wantBanned fails unless the fake shows userID as banned in chatID.
func wantBanned(t *testing.T, fake *staffActionFake, chatID, userID int64) {
	t.Helper()
	m := fake.member(chatID, userID)
	if m == nil || m.Status != gotgbot.ChatMemberStatusKicked {
		t.Fatalf("member %d of chat %d = %+v, want kicked", userID, chatID, m)
	}
}

// cardState reads the state field of a card straight from Redis.
func cardState(t *testing.T, token string) string {
	t.Helper()
	client := cache.GetRedisClient()
	if client == nil {
		t.Fatal("no redis client")
	}
	state, err := client.HGet(cache.Context, staffCardKey(token), "state").Result()
	if err != nil {
		t.Fatalf("read state of card %s: %v", token, err)
	}
	return state
}

func TestStaffActionTracer(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})

	env.send(env.issuer, "/ban 4242 spamming")

	texts := textsToChat(env.fake.staffBotClient, env.staffChat)
	if len(texts) != 1 {
		t.Fatalf("messages to the Staff Group = %q, want exactly one card", texts)
	}
	if !strings.Contains(texts[0], staffMarker("staff_act_card_applies")) || !strings.Contains(texts[0], "spamming") {
		t.Fatalf("card text = %q, want the applies marker and the reason", texts[0])
	}
	token, cardMsgID := env.card()
	if _, ok := parseStaffActionToken(token); !ok {
		t.Fatalf("card token %q is not 16 lowercase hex characters", token)
	}
	for _, row := range staffKeyboardOf(env.fake.sentTo(env.staffChat)[0].Params["reply_markup"]) {
		for _, button := range row {
			if len(button.CallbackData) > 32 {
				t.Fatalf("button data %q is %d bytes, want 32 at most", button.CallbackData, len(button.CallbackData))
			}
		}
	}
	if calls := env.fake.callsFor("banChatMember"); len(calls) != 0 {
		t.Fatalf("banChatMember calls before Confirm = %d, want 0", len(calls))
	}

	env.tap(env.issuer, staffActRunConfirm, token, cardMsgID)
	env.waitRuns()

	for _, group := range []int64{g1, g2} {
		bans := env.callsTo("banChatMember", group)
		if len(bans) != 1 || staffParamInt(bans[0].Params, "user_id") != staffTestTarget {
			t.Fatalf("banChatMember calls in group %d = %+v, want one for user %d", group, bans, staffTestTarget)
		}
		if until := staffParamInt(bans[0].Params, "until_date"); until != 0 {
			t.Fatalf("until_date in group %d = %d, want a permanent ban", group, until)
		}
		wantBanned(t, env.fake, group, staffTestTarget)
	}
	if writes := env.writes(env.staffChat); len(writes) != 0 {
		t.Fatalf("write calls to the Staff Group = %+v, want none", writes)
	}

	edits := env.edits(env.staffChat, cardMsgID)
	last := edits[len(edits)-1]
	wantNoKeyboard(t, last)
	text := env.lastEditText(env.staffChat, cardMsgID)
	for _, want := range []string{"Group A", "Group B", staffMarker("staff_act_banned_not_in_group")} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary = %q, want it to contain %q", text, want)
		}
	}
	if state := cardState(t, token); state != staffCardDone {
		t.Fatalf("card state = %q, want %q", state, staffCardDone)
	}
}

func TestStaffActionSkipsWhereIssuerNotAdmin(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	g1, g2 := env.groups[0], env.groups[1]
	env.fake.setMember(g1, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g2, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	env.fake.setMember(g2, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	env.send(env.issuer, "/ban 4242")
	token, cardMsgID := env.card()
	env.tap(env.issuer, staffActRunConfirm, token, cardMsgID)
	env.waitRuns()

	wantBanned(t, env.fake, g1, staffTestTarget)
	if m := env.fake.member(g2, staffTestTarget); m == nil || m.Status != gotgbot.ChatMemberStatusMember {
		t.Fatalf("target in group B = %+v, want still a member", m)
	}
	text := env.lastEditText(env.staffChat, cardMsgID)
	if !strings.Contains(text, staffMarker("staff_act_skip_issuer_not_admin")) {
		t.Fatalf("summary = %q, want the issuer-not-admin skip", text)
	}
	if lookups := env.memberLookups(g2, staffTestTarget); lookups != 0 {
		t.Fatalf("getChatMember lookups of the target in the skipped group = %d, want 0", lookups)
	}
	if writes := env.writes(g2); len(writes) != 0 {
		t.Fatalf("write calls in the skipped group = %+v, want none", writes)
	}
}
