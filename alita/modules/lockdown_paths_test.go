//go:build testtools

package modules

import (
	"errors"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/greetings"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// deletesOf returns the deleteMessage calls for message msgID in the env's chat.
func (e *lockdownEnv) deletesOf(msgID int64) []moduleBotCall {
	var matched []moduleBotCall
	for _, call := range e.calls("deleteMessage") {
		if staffParamInt(call.Params, "message_id") == msgID {
			matched = append(matched, call)
		}
	}
	return matched
}

// callPosition is the index in the fake's call order of the first recorded call of
// method whose key parameter equals want, or -1.
func (e *lockdownEnv) callPosition(method, key string, want int64) int {
	for i, call := range e.callLog() {
		if call.Method == method && staffParamInt(call.Params, "chat_id") == e.chat.Id &&
			staffParamInt(call.Params, key) == want {
			return i
		}
	}
	return -1
}

// wantNoJoinWelcome fails when the bot posted anything a welcome or a captcha
// challenge would use since the given counts were taken.
func (e *lockdownEnv) wantNoJoinWelcome(messages, photos, restricts int) {
	e.t.Helper()
	messagesAfter, photosAfter, restrictsAfter := e.chatSends()
	if messagesAfter != messages || photosAfter != photos || restrictsAfter != restricts {
		e.t.Errorf("the bot sent %d message(s), %d photo(s), %d restriction(s) since the join, want none: no welcome and no captcha",
			messagesAfter-messages, photosAfter-photos, restrictsAfter-restricts)
	}
}

func TestLockdownGuardDedupesTwoPaths(t *testing.T) {
	t.Run("chat_member then service", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(true)
		env.lockAsAdmin("raid")
		messages, photos, restricts := env.chatSends()

		raider := env.newJoiner("Raider")
		env.join(raider, raider, "https://t.me/+abc")
		msgID := env.serviceJoin(raider, nil, raider)

		row := env.joinerRow(raider.Id)
		if row.JoinPath != models.JoinPathMember || row.JoinMsgID != msgID {
			t.Errorf("row = %+v, want path member and join_msg_id %d: the second delivery adds its message to the same row", row, msgID)
		}
		env.cycle()
		env.cycle()

		if bans := env.bansOf(raider.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want exactly 1", len(bans))
		}
		if deletes := env.deletesOf(msgID); len(deletes) != 1 {
			t.Errorf("deleteMessage calls for the join message = %d, want exactly 1", len(deletes))
		}
		if cleared := env.joinerRow(raider.Id); cleared.JoinMsgID != 0 || cleared.State != models.JoinerStateBanned {
			t.Errorf("row after the cycles = %+v, want banned with the message ID cleared", cleared)
		}
		env.wantNoJoinWelcome(messages, photos, restricts)
	})

	t.Run("service then chat_member", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(true)
		env.lockAsAdmin("raid")
		messages, photos, restricts := env.chatSends()

		raider := env.newJoiner("Raider")
		msgID := env.serviceJoin(raider, nil, raider)
		env.join(raider, raider, "https://t.me/+abc")

		row := env.joinerRow(raider.Id)
		if row.JoinPath != models.JoinPathService || row.JoinMsgID != msgID {
			t.Errorf("row = %+v, want path service and join_msg_id %d: the first delivery decides", row, msgID)
		}
		env.cycle()
		env.cycle()

		if bans := env.bansOf(raider.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want exactly 1", len(bans))
		}
		if deletes := env.deletesOf(msgID); len(deletes) != 1 {
			t.Errorf("deleteMessage calls for the join message = %d, want exactly 1", len(deletes))
		}
		env.wantNoJoinWelcome(messages, photos, restricts)
	})

	t.Run("service only", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)

		// Control: before any lockdown a join service message gets a welcome, so the
		// silence below is the guard's doing.
		control := env.newJoiner("Control")
		env.serviceJoin(control, nil, control)
		if messages, photos, _ := env.chatSends(); messages+photos == 0 {
			t.Fatal("control: a join message before the lockdown sent no welcome, so the test proves nothing")
		}

		env.lockAsAdmin("raid")
		messages, photos, restricts := env.chatSends()
		raider := env.newJoiner("Raider")
		msgID := env.serviceJoin(raider, nil, raider)

		row := env.joinerRow(raider.Id)
		if row.State != models.JoinerStatePending || row.JoinPath != models.JoinPathService || row.PerformerID != raider.Id {
			t.Errorf("row = %+v, want pending, service path, performer = joiner", row)
		}
		if bans := env.bansOf(raider.Id); len(bans) != 0 {
			t.Errorf("the guard made %d ban call(s), want none: the worker bans, never the handler", len(bans))
		}
		env.cycle()

		ban := env.callPosition("banChatMember", "user_id", raider.Id)
		del := env.callPosition("deleteMessage", "message_id", msgID)
		if ban < 0 || del < 0 || del < ban {
			t.Errorf("ban at call %d, delete at call %d, want both made and the delete after the ban", ban, del)
		}
		if deletes := env.deletesOf(msgID); len(deletes) != 1 {
			t.Errorf("deleteMessage calls for the join message = %d, want exactly 1", len(deletes))
		}
		env.wantNoJoinWelcome(messages, photos, restricts)
	})
}

func TestLockdownMixedServiceMessage(t *testing.T) {
	t.Run("later handlers see only the exempt user", func(t *testing.T) {
		env := newLockdownEnv(t)
		var seen []int64
		env.dispatcher.AddHandlerToGroup(
			handlers.NewMessage(func(m *gotgbot.Message) bool { return m.NewChatMembers != nil },
				func(_ *gotgbot.Bot, ctx *ext.Context) error {
					for _, u := range ctx.EffectiveMessage.NewChatMembers {
						seen = append(seen, u.Id)
					}
					return ext.EndGroups
				}), 0)
		env.lockAsAdmin("raid")

		human := env.newJoiner("Human")
		bot := gotgbot.User{Id: env.newJoiner("").Id, IsBot: true, FirstName: "Helper", Username: "helper_bot"}
		msgID := env.serviceJoin(env.admin, nil, human, bot)

		if len(seen) != 1 || seen[0] != human.Id {
			t.Errorf("a handler at group 0 saw new members %v, want only [%d]", seen, human.Id)
		}
		if row := env.joinerRow(human.Id); row.State != models.JoinerStateExempt {
			t.Errorf("human row = %+v, want exempt: a live administrator added them", row)
		}
		botRow := env.joinerRow(bot.Id)
		if botRow.State != models.JoinerStatePending || botRow.JoinMsgID != 0 {
			t.Errorf("bot row = %+v, want pending with no join message: the message holds an exempt user", botRow)
		}

		env.cycle()
		env.cycle()

		if bans := env.bansOf(human.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls for the exempt human = %d, want none", len(bans))
		}
		if bans := env.bansOf(bot.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls for the bot = %d, want 1", len(bans))
		}
		if deletes := env.deletesOf(msgID); len(deletes) != 0 {
			t.Errorf("deleteMessage calls for the join message = %d, want none: it still announces the exempt user", len(deletes))
		}
	})

	t.Run("greetings welcome only the exempt user", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		env.lockAsAdmin("raid")
		messages, photos, _ := env.chatSends()

		human := env.newJoiner("Human")
		bot := gotgbot.User{Id: env.newJoiner("").Id, IsBot: true, FirstName: "Helper", Username: "helper_bot"}
		env.serviceJoin(env.admin, nil, human, bot)

		messagesAfter, photosAfter, _ := env.chatSends()
		if got := (messagesAfter - messages) + (photosAfter - photos); got != 1 {
			t.Errorf("welcomes sent = %d, want exactly 1: one for the exempt user and none for the banned bot", got)
		}
	})
}

func TestLockdownJoinAfterDeclinedRequest(t *testing.T) {
	env := newLockdownEnv(t)
	active := env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	seeded := seedLockdownJoiner(t, active.ID, env.chat.Id, raider.Id, models.JoinerStateDeclined)
	if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", seeded.ID).
		UpdateColumn("join_path", models.JoinPathRequest).Error; err != nil {
		t.Fatalf("set join path: %v", err)
	}
	ageJoiner(t, seeded.ID, 5*time.Second)

	env.join(raider, raider, "https://t.me/+abc")

	row := env.joinerRow(raider.Id)
	if row.ID != seeded.ID || row.State != models.JoinerStatePending || row.JoinPath != models.JoinPathMember ||
		row.InviteLink != "https://t.me/+abc" || row.BanUntil == 0 {
		t.Errorf("row = %+v, want the declined request row reclaimed: pending, member path, the link, a ban_until", row)
	}
	env.cycle()

	if bans := env.bansOf(raider.Id); len(bans) != 1 {
		t.Errorf("banChatMember calls = %d, want 1: a request that was declined is a different event from a link join", len(bans))
	}
}

func TestLockdownRejoinAfterUnban(t *testing.T) {
	env := newLockdownEnv(t)
	env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.cycle()
	first := env.joinerRow(raider.Id)
	if first.State != models.JoinerStateBanned {
		t.Fatalf("setup: state = %q, want banned", first.State)
	}

	// An admin unbanned the raider by hand. The clock cannot be advanced, so the row
	// is moved 30 s into the past, ban_until included.
	env.fake.setMember(env.chat.Id, raider.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
	ageJoiner(t, first.ID, 30*time.Second)
	if err := db.DB.Model(&models.LockdownJoiner{}).Where("id = ?", first.ID).
		UpdateColumn("ban_until", first.BanUntil-30).Error; err != nil {
		t.Fatalf("shift ban_until: %v", err)
	}
	earlierUntil := first.BanUntil - 30

	env.join(raider, raider, "https://t.me/+again")

	again := env.joinerRow(raider.Id)
	if again.ID != first.ID || again.State != models.JoinerStatePending || again.BanUntil <= earlierUntil {
		t.Fatalf("row = %+v, want the same row pending again with a ban_until later than %d", again, earlierUntil)
	}
	env.cycle()

	bans := env.bansOf(raider.Id)
	if len(bans) != 2 {
		t.Fatalf("banChatMember calls = %d, want 2: the first join and the re-join", len(bans))
	}
	if got := staffParamInt(bans[1].Params, "until_date"); got != again.BanUntil {
		t.Errorf("until_date of the re-join ban = %d, want the row's new ban_until %d", got, again.BanUntil)
	}

	// A second delivery of the same re-join, inside the window, changes nothing.
	env.serviceJoin(raider, nil, raider)
	env.join(raider, raider, "https://t.me/+again")
	env.cycle()
	if bans := env.bansOf(raider.Id); len(bans) != 2 {
		t.Errorf("banChatMember calls after a repeated delivery = %d, want still 2", len(bans))
	}
}

func TestLockdownApprovalRace(t *testing.T) {
	approve := func(env *lockdownEnv, user gotgbot.User) {
		env.memberUpdate(env.admin, gotgbot.ChatMemberLeft{User: user}, gotgbot.ChatMemberMember{User: user}, true)
	}

	t.Run("approval wins", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		env.lockAsAdmin("raid")
		messages, photos, _ := env.chatSends()

		applicant := env.newJoiner("Applicant")
		env.serviceJoin(applicant, nil, applicant)
		if row := env.joinerRow(applicant.Id); row.State != models.JoinerStatePending {
			t.Fatalf("setup: state = %q, want pending after the service message", row.State)
		}

		approve(env, applicant)

		if row := env.joinerRow(applicant.Id); row.State != models.JoinerStateExempt {
			t.Errorf("row = %+v, want exempt: the chat_member update shows an admin approved the request", row)
		}
		messagesAfter, photosAfter, _ := env.chatSends()
		if (messagesAfter-messages)+(photosAfter-photos) != 1 {
			t.Error("no welcome followed the approval: greetings should run for the let-in user")
		}
		env.cycle()
		if bans := env.bansOf(applicant.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none for an approved applicant", len(bans))
		}
	})

	t.Run("ban already made", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		env.lockAsAdmin("raid")

		applicant := env.newJoiner("Applicant")
		env.serviceJoin(applicant, nil, applicant)
		env.cycle()
		if row := env.joinerRow(applicant.Id); row.State != models.JoinerStateBanned {
			t.Fatalf("setup: state = %q, want banned", row.State)
		}
		messages, photos, restricts := env.chatSends()

		approve(env, applicant)

		if row := env.joinerRow(applicant.Id); row.State != models.JoinerStateBanned {
			t.Errorf("row = %+v, want it to stay banned until the lift (D-24)", row)
		}
		env.wantNoJoinWelcome(messages, photos, restricts)
		env.cycle()
		if bans := env.bansOf(applicant.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want exactly the one already made", len(bans))
		}
	})
}

func TestLockdownGuardServiceMessageVariants(t *testing.T) {
	t.Run("a service message sent by a bot account reaches the guard", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")

		botAccount := gotgbot.User{Id: env.newJoiner("").Id, IsBot: true, FirstName: "Spammer", Username: "spam_bot"}
		env.serviceJoin(botAccount, nil, botAccount)

		row := env.joinerRow(botAccount.Id)
		if row.State != models.JoinerStatePending || !row.IsBot {
			t.Fatalf("row = %+v, want a pending bot row", row)
		}
		env.cycle()
		if bans := env.bansOf(botAccount.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want 1", len(bans))
		}
	})

	t.Run("a service message from an anonymous admin lets the added human in", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		anonymous := gotgbot.User{Id: lockdownGroupAnonymousBot, IsBot: true, FirstName: "Group"}
		chat := env.chat
		lookups := len(env.callLog())

		guest := env.newJoiner("Guest")
		env.serviceJoin(anonymous, &chat, guest)

		row := env.joinerRow(guest.Id)
		if row.State != models.JoinerStateExempt {
			t.Fatalf("row = %+v, want exempt: an anonymous administrator added them", row)
		}
		for _, call := range env.callLog()[lookups:] {
			if call.Method == "getChatMember" {
				t.Errorf("getChatMember was called for an anonymous performer: %+v", call.Params)
			}
		}
		env.cycle()
		if bans := env.bansOf(guest.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none", len(bans))
		}
	})
}

// performerLookups counts the getChatMember calls about userID in the env's chat.
func (e *lockdownEnv) performerLookups(userID int64) int {
	count := 0
	for _, call := range e.calls("getChatMember") {
		if staffParamInt(call.Params, "user_id") == userID {
			count++
		}
	}
	return count
}

func TestLockdownAdminAddedJoinerExempt(t *testing.T) {
	t.Run("a live administrator adds a human", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		env.lockAsAdmin("raid")
		messages, photos, _ := env.chatSends()

		guest := env.newJoiner("Guest")
		env.join(guest, env.admin, "")

		if row := env.joinerRow(guest.Id); row.State != models.JoinerStateExempt || row.PerformerID != env.admin.Id {
			t.Errorf("row = %+v, want exempt, performed by the administrator", row)
		}
		messagesAfter, photosAfter, _ := env.chatSends()
		if (messagesAfter-messages)+(photosAfter-photos) != 1 {
			t.Error("no welcome followed: an exempt joiner is let through to the greetings")
		}
		env.cycle()
		if bans := env.bansOf(guest.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none", len(bans))
		}
	})

	t.Run("a live administrator adds a bot", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.loadJoinModules()
		env.enableJoinWelcome(false)
		env.lockAsAdmin("raid")
		messages, photos, restricts := env.chatSends()

		helper := gotgbot.User{Id: env.newJoiner("").Id, IsBot: true, FirstName: "Helper", Username: "helper_bot"}
		env.join(helper, env.admin, "")

		if row := env.joinerRow(helper.Id); row.State != models.JoinerStatePending || !row.IsBot {
			t.Errorf("row = %+v, want a pending bot row: a bot never gets in during a lockdown", row)
		}
		env.wantNoJoinWelcome(messages, photos, restricts)
		env.cycle()
		if bans := env.bansOf(helper.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want 1", len(bans))
		}
	})

	t.Run("a plain member adds a human", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")

		plain := env.newJoiner("Plain")
		env.fake.setMember(env.chat.Id, plain.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		guest := env.newJoiner("Guest")
		env.join(guest, plain, "")

		if row := env.joinerRow(guest.Id); row.State != models.JoinerStatePending {
			t.Errorf("row = %+v, want pending: only an administrator's invite exempts", row)
		}
		env.cycle()
		if bans := env.bansOf(guest.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want 1", len(bans))
		}
	})

	t.Run("a performer whose live lookup fails", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		lookupsBefore := env.performerLookups(env.admin.Id)
		env.fake.script("getChatMember", env.chat.Id, errors.New("telegram is down"))

		guest := env.newJoiner("Guest")
		env.join(guest, env.admin, "")

		if got := env.performerLookups(env.admin.Id) - lookupsBefore; got != 1 {
			t.Errorf("live lookups of the performer = %d, want exactly 1", got)
		}
		if row := env.joinerRow(guest.Id); row.State != models.JoinerStatePending {
			t.Errorf("row = %+v, want pending: a performer who cannot be checked never exempts", row)
		}
		env.cycle()
		if bans := env.bansOf(guest.Id); len(bans) != 1 {
			t.Errorf("banChatMember calls = %d, want 1", len(bans))
		}
	})

	t.Run("the anonymous admin identity adds a human", func(t *testing.T) {
		env := newLockdownEnv(t)
		env.lockAsAdmin("raid")
		anonymous := gotgbot.User{Id: lockdownGroupAnonymousBot, IsBot: true, FirstName: "Group"}
		lookupsBefore := env.performerLookups(anonymous.Id)

		guest := env.newJoiner("Guest")
		env.join(guest, anonymous, "")

		if row := env.joinerRow(guest.Id); row.State != models.JoinerStateExempt {
			t.Errorf("row = %+v, want exempt: an anonymous administrator added them", row)
		}
		if got := env.performerLookups(anonymous.Id) - lookupsBefore; got != 0 {
			t.Errorf("live lookups of the anonymous identity = %d, want none", got)
		}
		env.cycle()
		if bans := env.bansOf(guest.Id); len(bans) != 0 {
			t.Errorf("banChatMember calls = %d, want none", len(bans))
		}
	})
}

func TestLockdownSuppressesGoodbye(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	if err := greetings.SetGoodbyeToggle(env.chat.Id, true); err != nil {
		t.Fatalf("enable goodbye: %v", err)
	}
	env.lockAsAdmin("raid")

	raider := env.newJoiner("Raider")
	env.join(raider, raider, "https://t.me/+abc")
	env.cycle()
	row := env.joinerRow(raider.Id)
	if row.State != models.JoinerStateBanned {
		t.Fatalf("setup: state = %q, want banned", row.State)
	}
	botUser := gotgbot.User{Id: staffTestBotID, IsBot: true, FirstName: "Alita"}
	kick := func(user, from gotgbot.User, until int64) {
		env.memberUpdate(from, gotgbot.ChatMemberMember{User: user}, gotgbot.ChatMemberBanned{User: user, UntilDate: until}, false)
	}

	sent := len(env.replies())
	kick(raider, botUser, row.BanUntil)
	if got := len(env.replies()); got != sent {
		t.Errorf("messages sent after the lockdown's own ban = %d, want none: no goodbye for a removed joiner", got-sent)
	}

	// Control: a ban that is not the lockdown's own still gets the group's goodbye.
	other := env.newJoiner("Other")
	kick(other, env.admin, 0)
	if got := len(env.replies()); got != sent+1 {
		t.Fatalf("messages sent after a deliberate ban = %d, want 1 goodbye", got-sent)
	}

	// A joiner whose ban was replaced by one of another length is a deliberate ban too.
	replaced := env.newJoiner("Replaced")
	env.join(replaced, replaced, "https://t.me/+abc")
	env.cycle()
	replacedRow := env.joinerRow(replaced.Id)
	kick(replaced, env.admin, replacedRow.BanUntil+3600)
	if got := len(env.replies()); got != sent+2 {
		t.Errorf("messages sent after a replaced ban = %d, want a second goodbye", got-sent-1)
	}
}
