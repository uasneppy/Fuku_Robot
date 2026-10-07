//go:build testtools

package modules

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

// callLog returns every call the fake recorded so far, in order.
func (e *lockdownEnv) callLog() []moduleBotCall {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	return append([]moduleBotCall(nil), e.fake.calls...)
}

// memberCallsSince lists the getChatMember, banChatMember and unbanChatMember calls
// about members of the lockdown chat made after the first n recorded calls, as
// "method:user", in order.
func (e *lockdownEnv) memberCallsSince(n int) []string {
	var out []string
	for _, call := range e.callLog()[n:] {
		switch call.Method {
		case "getChatMember", "banChatMember", "unbanChatMember":
		default:
			continue
		}
		if staffParamInt(call.Params, "chat_id") != e.chat.Id {
			continue
		}
		out = append(out, fmt.Sprintf("%s:%d", call.Method, staffParamInt(call.Params, "user_id")))
	}
	return out
}

// lockAndBan locks the chat, lets one user per name join through an invite link and
// runs a cycle, so every one of them is recorded and banned. It returns the lockdown
// and the users in the order they joined.
func (e *lockdownEnv) lockAndBan(names ...string) (*models.ChatLockdown, []gotgbot.User) {
	e.t.Helper()
	ld := e.lockAsAdmin("raid")
	users := make([]gotgbot.User, 0, len(names))
	for _, name := range names {
		user := e.newJoiner(name)
		e.join(user, user, "https://t.me/+abc")
		users = append(users, user)
	}
	e.cycle()
	for _, user := range users {
		if row := e.joinerRow(user.Id); row.State != models.JoinerStateBanned {
			e.t.Fatalf("setup: joiner %d is %q, want banned", user.Id, row.State)
		}
	}
	return ld, users
}

// messagesWith counts the messages the bot sent to the chat that contain want.
func (e *lockdownEnv) messagesWith(want string) int {
	return len(e.repliesWith(want))
}

func TestIsLockdownBan(t *testing.T) {
	const until int64 = 1_900_000_000
	kicked := func(at int64) gotgbot.MergedChatMember {
		return gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: at}
	}

	tests := []struct {
		name   string
		member gotgbot.MergedChatMember
		until  int64
		want   bool
	}{
		{"kicked on the exact end date", kicked(until), until, true},
		{"kicked 2 s earlier", kicked(until - 2), until, true},
		{"kicked 2 s later", kicked(until + 2), until, true},
		{"kicked 3 s earlier", kicked(until - 3), until, false},
		{"kicked 3 s later", kicked(until + 3), until, false},
		{"a permanent ban", kicked(0), until, false},
		{"a timed ban of another length", kicked(until + 3600), until, false},
		{"left", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusLeft, UntilDate: until}, until, false},
		{"member", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusMember, UntilDate: until}, until, false},
		{"restricted", gotgbot.MergedChatMember{Status: gotgbot.ChatMemberStatusRestricted, UntilDate: until}, until, false},
		{"no stored end date", kicked(until), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLockdownBan(tt.member, tt.until); got != tt.want {
				t.Errorf("isLockdownBan = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLockdownLiftUnbansJoiners(t *testing.T) {
	env := newLockdownEnv(t)
	env.loadJoinModules()
	env.enableJoinWelcome(false)
	ld, users := env.lockAndBan("Ann", "Bob", "Cid")
	a, b, c := users[0], users[1], users[2]

	var banOrder []string
	for _, call := range env.calls("banChatMember") {
		banOrder = append(banOrder, fmt.Sprint(staffParamInt(call.Params, "user_id")))
	}
	wantBans := []string{fmt.Sprint(a.Id), fmt.Sprint(b.Id), fmt.Sprint(c.Id)}
	if !slices.Equal(banOrder, wantBans) {
		t.Errorf("ban order = %v, want the order the joiners were recorded in: %v", banOrder, wantBans)
	}

	env.send(env.admin, "/unlockdown")
	env.wantReplyHas(staffMarker("lockdown_lifted"), staffMarker("lockdown_lifted_unbanning")+" 3")
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
		t.Fatalf("State after /unlockdown = %q, want lifting while three joiners wait to be unbanned", row.State)
	}

	mark := len(env.callLog())
	env.cycle()

	want := []string{
		fmt.Sprintf("getChatMember:%d", a.Id), fmt.Sprintf("unbanChatMember:%d", a.Id),
		fmt.Sprintf("getChatMember:%d", b.Id), fmt.Sprintf("unbanChatMember:%d", b.Id),
		fmt.Sprintf("getChatMember:%d", c.Id), fmt.Sprintf("unbanChatMember:%d", c.Id),
	}
	if got := env.memberCallsSince(mark); !slices.Equal(got, want) {
		t.Errorf("member calls of the lift = %v, want %v: look first, then unban, in the order recorded", got, want)
	}
	for _, call := range env.calls("unbanChatMember") {
		if !staffParamBool(call.Params, "only_if_banned") {
			t.Errorf("unbanChatMember %v was sent without only_if_banned", call.Params)
		}
	}
	for _, user := range users {
		if row := env.joinerRow(user.Id); row.State != models.JoinerStateUnbanned {
			t.Errorf("joiner %d state = %q, want unbanned", user.Id, row.State)
		}
		if member := env.fake.member(env.chat.Id, user.Id); member == nil || member.Status != gotgbot.ChatMemberStatusLeft {
			t.Errorf("joiner %d in the chat = %+v, want left (able to rejoin)", user.Id, member)
		}
	}
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
		t.Errorf("State after the lift cycle = %q, want lifted", row.State)
	}
	if got := env.messagesWith(staffMarker("lockdown_lift_tally") + " 3"); got != 1 {
		t.Fatalf("tally messages = %d, want exactly 1 saying 3 were unbanned", got)
	}

	sent := len(env.replies())
	env.cycle()
	if got := len(env.replies()); got != sent {
		t.Errorf("a further cycle posted %d message(s), want none: the tally is posted once", got-sent)
	}

	// Anyone can rejoin now, and the lockdown no longer sees them.
	env.join(a, a, "")
	if rows := env.joinerRows(a.Id); len(rows) != 1 {
		t.Errorf("joiner rows of the returning user = %d, want the 1 old row and no new one", len(rows))
	}
	if got := len(env.replies()); got <= sent {
		t.Error("the welcome did not run for a user who rejoined after the lift")
	}
}

func TestLockdownLiftKeepsDeliberateBan(t *testing.T) {
	env := newLockdownEnv(t)
	_, users := env.lockAndBan("Ann", "Bob", "Cid", "Dee")
	a, b, c, d := users[0], users[1], users[2], users[3]
	dRow := env.joinerRow(d.Id)

	// B was banned for good, C was unbanned by hand, D got a /tban of another length.
	env.fake.setMember(env.chat.Id, b.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked})
	env.fake.setMember(env.chat.Id, c.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusLeft})
	env.fake.setMember(env.chat.Id, d.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusKicked, UntilDate: dRow.BanUntil + 3600})

	env.send(env.admin, "/unlockdown")
	env.cycle()

	unbans := env.calls("unbanChatMember")
	if len(unbans) != 1 || staffParamInt(unbans[0].Params, "user_id") != a.Id {
		t.Fatalf("unbanChatMember calls = %v, want exactly one, for the lockdown's own ban (user %d)", unbans, a.Id)
	}
	wantStates := map[int64]string{
		a.Id: models.JoinerStateUnbanned,
		b.Id: models.JoinerStateKept,
		c.Id: models.JoinerStateKept,
		d.Id: models.JoinerStateKept,
	}
	for id, want := range wantStates {
		if row := env.joinerRow(id); row.State != want {
			t.Errorf("joiner %d state = %q, want %q", id, row.State, want)
		}
	}
	if member := env.fake.member(env.chat.Id, b.Id); member == nil || member.Status != gotgbot.ChatMemberStatusKicked {
		t.Errorf("B in the chat = %+v, want still kicked", member)
	}
	if member := env.fake.member(env.chat.Id, d.Id); member == nil || member.Status != gotgbot.ChatMemberStatusKicked {
		t.Errorf("D in the chat = %+v, want still kicked", member)
	}
	tally := env.repliesWith(staffMarker("lockdown_lift_tally") + " 1")
	if len(tally) != 1 {
		t.Fatalf("tally messages = %d, want 1 saying 1 was unbanned", len(tally))
	}
	if !strings.Contains(tally[0], staffMarker("lockdown_lift_tally_kept")+" 3") {
		t.Errorf("tally = %q, want it to say 3 stay banned on purpose", tally[0])
	}
}

func TestLockdownLiftNoJoiners(t *testing.T) {
	env := newLockdownEnv(t)
	ld := env.lockAsAdmin("raid")

	env.send(env.admin, "/unlockdown")

	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
		t.Fatalf("State = %q, want lifted by /unlockdown itself", row.State)
	}
	env.wantReplyHas(staffMarker("lockdown_lifted"))
	env.wantReplyLacks(staffMarker("lockdown_lifted_unbanning"))

	sent := len(env.replies())
	env.cycle()
	if got := len(env.replies()); got != sent {
		t.Errorf("a cycle posted %d message(s), want none: there is nothing to tally", got-sent)
	}
	if unbans := env.calls("unbanChatMember"); len(unbans) != 0 {
		t.Errorf("unbanChatMember calls = %d, want none", len(unbans))
	}
}

func TestLockdownLiftCancelsPending(t *testing.T) {
	env := newLockdownEnv(t)
	ld := env.lockAsAdmin("raid")
	raider := env.newJoiner("Raider")
	env.join(raider, raider, "")
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStatePending {
		t.Fatalf("setup: state = %q, want pending", row.State)
	}

	env.send(env.admin, "/unlockdown")
	env.wantReplyLacks(staffMarker("lockdown_lifted_unbanning"))
	env.cycle()

	if bans := env.bansOf(raider.Id); len(bans) != 0 {
		t.Errorf("banChatMember calls = %d, want none: a joiner still pending at the lift is never banned", len(bans))
	}
	if row := env.joinerRow(raider.Id); row.State != models.JoinerStateCancelled {
		t.Errorf("state = %q, want cancelled", row.State)
	}
	if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
		t.Errorf("lockdown State = %q, want lifted", row.State)
	}
}

func TestLockdownLiftUnbanFailureListed(t *testing.T) {
	t.Run("a failure is retried, then listed by name and ID", func(t *testing.T) {
		env := newLockdownEnv(t)
		ld, users := env.lockAndBan("E<b>", "Fay")
		e, f := users[0], users[1]
		// E's three unban calls fail; F's passes. Scripted answers are first in, first out.
		refusal := staffFakeError(400, "Bad Request: no <rights> to unban")
		env.fake.script("unbanChatMember", env.chat.Id, refusal, nil, refusal, refusal)

		env.send(env.admin, "/unlockdown")
		env.cycle()
		if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifting {
			t.Fatalf("State after one cycle = %q, want lifting: E still has tries left", row.State)
		}
		env.cycle()
		env.cycle()

		failed := env.joinerRow(e.Id)
		if failed.State != models.JoinerStateUnbanFailed || !strings.Contains(failed.Detail, "no &lt;rights&gt; to unban") {
			t.Errorf("E = %+v, want unban_failed with Telegram's escaped detail", failed)
		}
		if row := env.joinerRow(f.Id); row.State != models.JoinerStateUnbanned {
			t.Errorf("F state = %q, want unbanned: one failure never blocks the others", row.State)
		}
		if row := env.freshRow(ld.ID); row.State != models.LockdownStateLifted {
			t.Errorf("lockdown State = %q, want lifted once nothing is left to try", row.State)
		}

		tally := env.repliesWith(staffMarker("lockdown_lift_tally") + " 1")
		if len(tally) != 1 {
			t.Fatalf("tally messages = %d, want 1", len(tally))
		}
		for _, want := range []string{
			staffMarker("lockdown_lift_tally_failed") + " 1",
			"E&lt;b&gt;",
			fmt.Sprint(e.Id),
			"no &lt;rights&gt; to unban",
		} {
			if !strings.Contains(tally[0], want) {
				t.Errorf("tally %q does not contain %q", tally[0], want)
			}
		}
	})

	t.Run("a long failure list is cut and the rest counted", func(t *testing.T) {
		previous := lockdownTallyListMax
		lockdownTallyListMax = 1
		t.Cleanup(func() { lockdownTallyListMax = previous })

		env := newLockdownEnv(t)
		_, users := env.lockAndBan("Eve", "Fox")
		e, f := users[0], users[1]
		refusal := staffFakeError(400, "Bad Request: nope")
		env.fake.script("unbanChatMember", env.chat.Id, refusal, refusal, refusal, refusal, refusal, refusal)

		env.send(env.admin, "/unlockdown")
		env.cycle()
		env.cycle()
		env.cycle()

		tally := env.repliesWith(staffMarker("lockdown_lift_tally_failed") + " 2")
		if len(tally) != 1 {
			t.Fatalf("tally messages = %d, want 1 naming 2 failures", len(tally))
		}
		if !strings.Contains(tally[0], fmt.Sprint(e.Id)) || strings.Contains(tally[0], fmt.Sprint(f.Id)) {
			t.Errorf("tally %q, want only the first-recorded failure named", tally[0])
		}
		if !strings.Contains(tally[0], staffMarker("lockdown_lift_tally_more")+" 1") {
			t.Errorf("tally %q, want it to count the 1 not listed", tally[0])
		}
	})
}
