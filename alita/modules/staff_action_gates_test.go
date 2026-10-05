//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

// shortenCardLifetime makes unconfirmed cards expire after d for the test.
func shortenCardLifetime(t *testing.T, d time.Duration) {
	t.Helper()
	previous := staffActionCardLifetime
	staffActionCardLifetime = d
	t.Cleanup(func() { staffActionCardLifetime = previous })
}

// cardActions lists the callback actions on the buttons of the last card.
func (e *staffActionEnv) cardActions() []string {
	e.t.Helper()
	var actions []string
	sent := e.fake.sentTo(e.staffChat)
	if len(sent) == 0 {
		return nil
	}
	for _, row := range staffKeyboardOf(sent[len(sent)-1].Params["reply_markup"]) {
		for _, button := range row {
			if decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace); ok {
				actions = append(actions, decoded.Fields["a"])
			}
		}
	}
	return actions
}

// wantNoCard fails when a keyboard was sent to the Staff Group.
func (e *staffActionEnv) wantNoCard() {
	e.t.Helper()
	for _, sent := range e.fake.sentTo(e.staffChat) {
		if len(staffKeyboardOf(sent.Params["reply_markup"])) != 0 {
			e.t.Fatalf("a confirm card was sent to the Staff Group: %v", sent.Params["text"])
		}
	}
}

// wantReplyMarker expects exactly one message to the Staff Group and no card.
func (e *staffActionEnv) wantReplyMarker(key string) {
	e.t.Helper()
	texts := textsToChat(e.fake.staffBotClient, e.staffChat)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker(key)) {
		e.t.Fatalf("messages to the Staff Group = %q, want exactly one with %s", texts, key)
	}
	e.wantNoCard()
}

// prepareCard has the issuer ask for a ban of staffTestTarget and returns the
// card.
func (e *staffActionEnv) prepareCard(command string) (token string, msgID int64) {
	e.t.Helper()
	e.send(e.issuer, command)
	return e.card()
}

func TestStaffActionCancel(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

	token, msgID := env.prepareCard("/ban 4242 spamming")
	if actions := env.cardActions(); len(actions) != 2 || actions[0] != staffActRunConfirm || actions[1] != staffActRunCancel {
		t.Fatalf("card button actions = %v, want [%s %s]", actions, staffActRunConfirm, staffActRunCancel)
	}

	env.tap(env.issuer, staffActRunCancel, token, msgID)

	text := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(text, staffMarker("staff_act_card_cancelled")) || !strings.Contains(text, "Iss&lt;i&gt;uer") {
		t.Fatalf("cancelled card = %q, want the cancelled marker and the escaped issuer name", text)
	}
	edits := env.edits(env.staffChat, msgID)
	wantNoKeyboard(t, edits[len(edits)-1])
	if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
		t.Fatalf("banChatMember calls after Cancel = %d, want 0", len(bans))
	}
	if deletes := env.fake.callsFor("deleteMessage"); len(deletes) != 0 {
		t.Fatalf("deleteMessage calls = %d, want the card to stay", len(deletes))
	}
	if state := cardState(t, token); state != staffCardCancelled {
		t.Fatalf("card state = %q, want %q", state, staffCardCancelled)
	}

	editCount := len(edits)
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	answer, _ := env.lastAnswer()
	if !strings.Contains(answer, staffMarker("staff_act_card_handled")) {
		t.Fatalf("answer to a Confirm after Cancel = %q, want the handled toast", answer)
	}
	if got := len(env.edits(env.staffChat, msgID)); got != editCount {
		t.Fatalf("edits after the late Confirm = %d, want %d", got, editCount)
	}
	if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
		t.Fatalf("banChatMember calls after the late Confirm = %d, want 0", len(bans))
	}
}

func TestStaffActionCardIssuerOnly(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	g := env.groups[0]
	env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
	stranger := gotgbot.User{Id: env.issuer.Id + 1, FirstName: "Stranger"}

	token, msgID := env.prepareCard("/ban 4242")
	for _, action := range []string{staffActRunConfirm, staffActRunCancel} {
		env.tap(stranger, action, token, msgID)
		answer, alert := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_act_card_issuer_only")) || !alert {
			t.Fatalf("answer to %s from another member = (%q, alert %t), want the issuer-only alert", action, answer, alert)
		}
	}
	if edits := env.edits(env.staffChat, msgID); len(edits) != 0 {
		t.Fatalf("edits after taps by another member = %d, want 0", len(edits))
	}
	if state := cardState(t, token); state != staffCardPending {
		t.Fatalf("card state = %q, want it to stay %q", state, staffCardPending)
	}
	if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
		t.Fatalf("banChatMember calls = %d, want 0", len(bans))
	}

	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	wantBanned(t, env.fake, g, staffTestTarget)
}

func TestStaffActionCardTapAnswers(t *testing.T) {
	t.Run("malformed token", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunConfirm, "t": "xyz"})
		env.tapData(env.issuer, env.staffChatObj(), 77, data)
		answer, alert := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_cb_expired")) || alert {
			t.Fatalf("answer = (%q, alert %t), want the expired toast", answer, alert)
		}
	})

	t.Run("unknown token", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunConfirm, "t": "0123456789abcdef"})
		env.tapData(env.issuer, env.staffChatObj(), 77, data)
		answer, _ := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_act_card_expired")) {
			t.Fatalf("answer = %q, want the card-expired toast", answer)
		}
		if text := env.lastEditText(env.staffChat, 77); !strings.Contains(text, staffMarker("staff_act_card_expired_text")) {
			t.Fatalf("edit = %q, want the expired text", text)
		}
	})

	t.Run("other chat", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		token, msgID := env.prepareCard("/ban 4242")
		data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": staffActRunConfirm, "t": token})
		other := gotgbot.Chat{Id: env.groups[0], Type: "supergroup", Title: "Elsewhere"}
		env.tapData(env.issuer, other, msgID, data)
		answer, alert := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_cb_denied")) || !alert {
			t.Fatalf("answer = (%q, alert %t), want the denied alert", answer, alert)
		}
		if state := cardState(t, token); state != staffCardPending {
			t.Fatalf("card state = %q, want it untouched", state)
		}
	})

	t.Run("second Confirm", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		token, msgID := env.prepareCard("/ban 4242")
		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		env.waitRuns()
		editCount := len(env.edits(env.staffChat, msgID))

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		answer, _ := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_act_card_handled")) {
			t.Fatalf("answer to a repeated Confirm = %q, want the handled toast", answer)
		}
		if got := len(env.edits(env.staffChat, msgID)); got != editCount {
			t.Fatalf("edits after the repeated Confirm = %d, want %d", got, editCount)
		}
		if bans := env.callsTo("banChatMember", env.groups[0]); len(bans) != 1 {
			t.Fatalf("banChatMember calls = %d, want exactly one", len(bans))
		}
	})

	t.Run("expired card", func(t *testing.T) {
		shortenCardLifetime(t, time.Millisecond)
		env := newStaffActionEnv(t, 1)
		env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		token, msgID := env.prepareCard("/ban 4242")
		time.Sleep(20 * time.Millisecond)

		env.tap(env.issuer, staffActRunConfirm, token, msgID)
		answer, _ := env.lastAnswer()
		if !strings.Contains(answer, staffMarker("staff_act_card_expired")) {
			t.Fatalf("answer = %q, want the card-expired toast", answer)
		}
		edits := env.edits(env.staffChat, msgID)
		if len(edits) != 1 {
			t.Fatalf("edits = %d, want one", len(edits))
		}
		text := env.lastEditText(env.staffChat, msgID)
		if !strings.Contains(text, staffMarker("staff_act_card_expired_text")) || !strings.Contains(text, staffMarker("staff_act_name_ban")) {
			t.Fatalf("edit = %q, want the header and the expired text", text)
		}
		wantNoKeyboard(t, edits[0])
		if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
			t.Fatalf("banChatMember calls = %d, want 0", len(bans))
		}
		if state := cardState(t, token); state != staffCardExpired {
			t.Fatalf("card state = %q, want %q", state, staffCardExpired)
		}
	})
}

func TestStaffActionConfirmAborts(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(env *staffActionEnv)
		want    string
	}{
		{
			name: "staff status removed",
			breakIt: func(env *staffActionEnv) {
				if _, _, err := staff.DeleteStaffGroupWithLinks(env.staffChat); err != nil {
					t.Fatalf("delete Staff Group: %v", err)
				}
			},
			want: "staff_act_abort_not_staff",
		},
		{
			name: "issuer left the Staff Group",
			breakIt: func(env *staffActionEnv) {
				env.fake.setMember(env.staffChat, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
			},
			want: "staff_act_abort_issuer_left",
		},
		{
			name: "every link removed",
			breakIt: func(env *staffActionEnv) {
				if err := db.DB.Where("staff_chat_id = ?", env.staffChat).Delete(&models.StaffGroupLink{}).Error; err != nil {
					t.Fatalf("delete links: %v", err)
				}
			},
			want: "staff_act_no_links",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.fake.setMember(env.groups[0], staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			token, msgID := env.prepareCard("/ban 4242")
			tc.breakIt(env)

			env.tap(env.issuer, staffActRunConfirm, token, msgID)
			env.waitRuns()

			text := env.lastEditText(env.staffChat, msgID)
			if !strings.Contains(text, staffMarker(tc.want)) {
				t.Fatalf("card = %q, want %s", text, tc.want)
			}
			if bans := env.fake.callsFor("banChatMember"); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %d, want 0", len(bans))
			}
			if state := cardState(t, token); state != staffCardAborted {
				t.Fatalf("card state = %q, want %q", state, staffCardAborted)
			}
		})
	}
}

func TestStaffActionPerGroupGates(t *testing.T) {
	cases := []struct {
		name string
		// command defaults to banning staffTestTarget.
		command string
		// setup runs after the default target (a member) and issuer (an admin with
		// the restrict right) were stored.
		setup      func(env *staffActionEnv, g int64)
		wantMarker string
		wantWrites int
		check      func(t *testing.T, env *staffActionEnv, g int64, summary string)
	}{
		{
			name: "issuer admin without the restrict right",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator})
			},
			wantMarker: "staff_act_skip_issuer_no_right",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				if n := env.memberLookups(g, staffTestTarget); n != 0 {
					t.Fatalf("target lookups = %d, want 0", n)
				}
			},
		},
		{
			name: "issuer is the live creator",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, env.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
			},
			wantWrites: 1,
		},
		{
			name: "target is an administrator",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true})
			},
			wantMarker: "staff_act_skip_target_admin",
		},
		{
			name: "target is the creator",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
			},
			wantMarker: "staff_act_skip_target_admin",
		},
		{
			name:       "target is the bot",
			command:    "/ban 999",
			wantMarker: "staff_act_skip_target_bot",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				if n := env.memberLookups(g, staffTestBotID); n != 0 {
					t.Fatalf("lookups of the bot = %d, want 0", n)
				}
			},
		},
		{
			name:       "target is the Telegram service account",
			command:    "/ban 777000",
			wantMarker: "staff_act_skip_target_service",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				if n := env.memberLookups(g, 777000); n != 0 {
					t.Fatalf("lookups of the service account = %d, want 0", n)
				}
			},
		},
		{
			name: "group creator changed",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setCreator(g, env.owner+1000)
			},
			wantMarker: "staff_act_skip_link_removed",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				link, err := staff.GetLinkOfGroupFresh(g)
				if err != nil || link != nil {
					t.Fatalf("link of the group = (%+v, %v), want it removed", link, err)
				}
				notices := 0
				for _, text := range textsToChat(env.fake.staffBotClient, env.staffChat) {
					if strings.Contains(text, staffMarker("staff_notice_unlinked_group_owner_changed")) {
						notices++
					}
				}
				if notices != 1 {
					t.Fatalf("owner-changed notices in the Staff Group = %d, want 1", notices)
				}
				if n := env.memberLookups(g, env.issuer.Id); n != 0 {
					t.Fatalf("issuer lookups = %d, want 0", n)
				}
			},
		},
		{
			name: "owner lookup fails",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setFailure("getChatAdministrators", g, errors.New("boom"))
			},
			wantMarker: "staff_act_fail_owner_unknown",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				link, err := staff.GetLinkOfGroupFresh(g)
				if err != nil || link == nil {
					t.Fatalf("link of the group = (%+v, %v), want it kept", link, err)
				}
			},
		},
		{
			name: "issuer lookup fails",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.script("getChatMember", g, staffFakeError(500, "Internal Server Error"))
			},
			wantMarker: "staff_act_fail_lookup",
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				if n := env.memberLookups(g, staffTestTarget); n != 0 {
					t.Fatalf("target lookups = %d, want 0", n)
				}
			},
		},
		{
			name: "target lookup fails",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.script("getChatMember", g, nil, staffFakeError(500, "Internal Server Error"))
			},
			wantMarker: "staff_act_fail_lookup",
		},
		{
			name: "target already banned for good",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
			},
			wantMarker: "staff_act_skip_already_banned",
		},
		{
			name: "temporary ban is upgraded",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, staffTestTarget, staffFakeMember{
					Status:    gotgbot.ChatMemberStatusKicked,
					UntilDate: time.Now().Add(time.Hour).Unix(),
				})
			},
			wantWrites: 1,
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				bans := env.callsTo("banChatMember", g)
				if len(bans) != 1 || staffParamInt(bans[0].Params, "until_date") != 0 {
					t.Fatalf("ban calls = %+v, want one without an end date", bans)
				}
				if m := env.fake.member(g, staffTestTarget); m == nil || m.UntilDate != 0 {
					t.Fatalf("target after the upgrade = %+v, want a permanent ban", m)
				}
			},
		},
		{
			name: "telegram rejects the ban",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.script("banChatMember", g, staffFakeError(400, "Bad Request: not enough rights <b>"))
			},
			wantMarker: "staff_act_fail_telegram",
			wantWrites: 1,
			check: func(t *testing.T, env *staffActionEnv, g int64, summary string) {
				if !strings.Contains(summary, "not enough rights &lt;b&gt;") {
					t.Fatalf("summary = %q, want Telegram's escaped description", summary)
				}
			},
		},
		{
			name: "target left the group",
			setup: func(env *staffActionEnv, g int64) {
				env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
			},
			wantMarker: "staff_act_banned_not_in_group",
			wantWrites: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			g := env.groups[0]
			env.fake.setMember(g, staffTestTarget, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			if tc.setup != nil {
				tc.setup(env, g)
			}
			command := tc.command
			if command == "" {
				command = "/ban 4242"
			}
			token, msgID := env.prepareCard(command)
			env.tap(env.issuer, staffActRunConfirm, token, msgID)
			env.waitRuns()

			summary := env.lastEditText(env.staffChat, msgID)
			if !strings.Contains(summary, "Group A") {
				t.Fatalf("summary = %q, want the group's line", summary)
			}
			if tc.wantMarker != "" && !strings.Contains(summary, staffMarker(tc.wantMarker)) {
				t.Fatalf("summary = %q, want %s", summary, tc.wantMarker)
			}
			if writes := env.writes(g); len(writes) != tc.wantWrites {
				t.Fatalf("write calls in the group = %+v, want %d", writes, tc.wantWrites)
			}
			if sent := env.callsTo("sendMessage", g); len(sent) != 0 {
				t.Fatalf("messages sent into the linked group = %d, want 0", len(sent))
			}
			if writes := env.writes(env.staffChat); len(writes) != 0 {
				t.Fatalf("write calls to the Staff Group = %+v, want 0", writes)
			}
			if tc.check != nil {
				tc.check(t, env, g, summary)
			}
		})
	}
}

func TestStaffActionStaffChatNeverActedOn(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	env.fake.mu.Lock()
	before := len(env.fake.calls)
	env.fake.mu.Unlock()

	card := &staffActionCard{Issuer: env.issuer.Id, StaffChat: env.staffChat, Kind: staffKindBan, Target: staffTestTarget}
	link := models.StaffGroupLink{GroupChatID: env.staffChat, StaffChatID: env.staffChat, OwnerUserID: env.owner, GroupTitle: "Itself"}
	result := runStaffActionInGroup(context.Background(), env.bot, card, link, 0, newStaffOwnerPass())

	if result.Outcome != staffOutcomeSkipped || result.Reason != staffReasonSkipStaffGroup {
		t.Fatalf("result = %+v, want a skip because it is the Staff Group", result)
	}
	env.fake.mu.Lock()
	after := len(env.fake.calls)
	env.fake.mu.Unlock()
	if after != before {
		t.Fatalf("Telegram calls made = %d, want none", after-before)
	}
}

func TestStaffActionRedisRequired(t *testing.T) {
	env := newStaffActionEnv(t, 1)
	cache.SetRedisClientForTest(t, nil)

	env.send(env.issuer, "/ban 4242")

	env.wantReplyMarker("staff_act_redis_unavailable")
}

func TestStaffActionNoLinks(t *testing.T) {
	env := newStaffActionEnv(t, 0)

	env.send(env.issuer, "/ban 4242")

	env.wantReplyMarker("staff_act_no_links")
}

func TestStaffActionParseHints(t *testing.T) {
	hints := []struct {
		command string
		want    string
	}{
		{"/ban", "staff_act_hint_need_target"},
		{"/ban spam 123", "staff_act_hint_bad_target"},
		{"/ban -1001234", "staff_act_hint_bad_target"},
		{"/ban 0", "staff_act_hint_bad_target"},
	}
	for _, tc := range hints {
		t.Run(tc.command, func(t *testing.T) {
			env := newStaffActionEnv(t, 1)
			env.send(env.issuer, tc.command)
			env.wantReplyMarker(tc.want)
		})
	}

	t.Run("reason of 200 runes is kept whole", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		reason := strings.Repeat("😀", 200)
		env.send(env.issuer, "/ban 4242 "+reason)
		texts := textsToChat(env.fake.staffBotClient, env.staffChat)
		if len(texts) != 1 || !strings.Contains(texts[0], reason) || strings.Contains(texts[0], "…") {
			t.Fatalf("card = %q, want the whole 200-rune reason", texts)
		}
	})

	t.Run("reason of 201 runes is cut", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.send(env.issuer, "/ban 4242 "+strings.Repeat("😀", 201))
		texts := textsToChat(env.fake.staffBotClient, env.staffChat)
		if len(texts) != 1 || !strings.Contains(texts[0], strings.Repeat("😀", 200)+"…") || strings.Contains(texts[0], strings.Repeat("😀", 201)) {
			t.Fatalf("card = %q, want 200 runes and an ellipsis", texts)
		}
	})

	t.Run("no reason shows the placeholder", func(t *testing.T) {
		env := newStaffActionEnv(t, 1)
		env.send(env.issuer, "/ban 4242")
		texts := textsToChat(env.fake.staffBotClient, env.staffChat)
		if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_act_no_reason")) ||
			!strings.Contains(texts[0], staffMarker("staff_act_target_unknown_name")) {
			t.Fatalf("card = %q, want the no-reason and unknown-name placeholders", texts)
		}
	})
}
