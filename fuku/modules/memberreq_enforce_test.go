//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mreqCallCount counts the requests of the named methods the fake has recorded for the group.
func (e *memberReqEnv) mreqCallCount(methods ...string) int {
	e.t.Helper()
	count := 0
	for _, method := range methods {
		count += len(e.calls(method))
	}
	return count
}

// mreqText is the text of a sendMessage the fake recorded.
func mreqText(sent staffSentMessage) string {
	return fmt.Sprint(sent.Params["text"])
}

// mreqKeyboardData returns the callback data of every button of a recorded message.
func mreqKeyboardData(sent staffSentMessage) []string {
	var data []string
	for _, row := range staffKeyboardOf(sent.Params["reply_markup"]) {
		for _, button := range row {
			data = append(data, button.CallbackData)
		}
	}
	return data
}

func TestMemberReqEnforceWarnOnce(t *testing.T) {
	env := newMemberReqEnv(t)
	env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
	member := env.mreqMember("Alex", "", "alex")

	got := env.mreqEnforce(env.mreqCase(member))
	if got.Outcome != memberReqWarned || got.Hold == nil {
		t.Fatalf("first enforcement = %+v, want Warned with the hold", got)
	}

	row := env.mreqWarnRow(member.Id)
	if row == nil || row.NumWarns != 1 || len(row.Reasons) != 1 {
		t.Fatalf("warns row = %+v, want one warn with one reason", row)
	}
	if reason := row.Reasons[0]; !strings.Contains(reason, staffMarker("mreq_warn_reason")) ||
		!strings.Contains(reason, staffMarker("mreq_rule_lastname")) {
		t.Fatalf("stored warn reason = %q, want mreq_warn_reason naming the last name rule", reason)
	}

	notices := env.mreqNotices()
	if len(notices) != 1 {
		t.Fatalf("messages to the group = %d, want exactly one notice", len(notices))
	}
	text := mreqText(notices[0])
	for _, want := range []string{
		staffMarker("mreq_notice_warn"),
		fmt.Sprintf("tg://user?id=%d", member.Id),
		staffMarker("mreq_rule_lastname"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("notice %q does not contain %q", text, want)
		}
	}
	if !strings.HasSuffix(text, " 1 3") {
		t.Errorf("notice %q should end with the count 1 and the limit 3", text)
	}
	if _, set := notices[0].Params["reply_parameters"]; set {
		t.Errorf("notice carries reply_parameters %v, want a new message", notices[0].Params["reply_parameters"])
	}
	if opts, ok := notices[0].Params["link_preview_options"].(*gotgbot.LinkPreviewOptions); !ok || !opts.IsDisabled {
		t.Errorf("link_preview_options = %v, want previews disabled", notices[0].Params["link_preview_options"])
	}
	if data := mreqKeyboardData(notices[0]); len(data) != 0 {
		t.Errorf("warn notice has buttons %v, want none", data)
	}

	hold := env.mreqHold(member.Id)
	if hold == nil || hold.Action != models.RequirementActionWarn || hold.Rules != 1 ||
		hold.PunishedAt == nil || hold.NoticeMsgID != notices[0].MessageID {
		t.Fatalf("hold = %+v, want warn action, rules 1, punished, notice %d", hold, notices[0].MessageID)
	}

	writesBefore := env.mreqCallCount("sendMessage", "banChatMember", "restrictChatMember", "unbanChatMember")
	again := env.mreqEnforce(env.mreqCase(member))
	if again.Outcome != memberReqHeld {
		t.Fatalf("second enforcement = %+v, want Held", again)
	}
	if after := env.mreqCallCount("sendMessage", "banChatMember", "restrictChatMember", "unbanChatMember"); after != writesBefore {
		t.Errorf("a held member made %d new writes or messages, want none", after-writesBefore)
	}
	if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
		t.Errorf("warns after the held enforcement = %+v, want still 1", row)
	}

	for _, sent := range env.mreqNotices() {
		if strings.Contains(mreqText(sent), staffMarker("warns_user_warning")) {
			t.Errorf("the warn module's reply was sent: %q", mreqText(sent))
		}
		for _, data := range mreqKeyboardData(sent) {
			if strings.HasPrefix(data, "rmWarn|") {
				t.Errorf("the Remove warn button was sent: %q", data)
			}
		}
	}
}

func TestMemberReqWarnMarkInsideLock(t *testing.T) {
	shortenStale := func(t *testing.T) {
		t.Helper()
		previous := memberreq.StaleClaim
		memberreq.StaleClaim = time.Millisecond
		t.Cleanup(func() { memberreq.StaleClaim = previous })
	}
	swapApplyWarn := func(t *testing.T, replacement func(context.Context, *gotgbot.Bot, *gotgbot.Chat, int64, string, func()) (warnApplyResult, error)) {
		t.Helper()
		previous := memberReqApplyWarn
		memberReqApplyWarn = replacement
		t.Cleanup(func() { memberReqApplyWarn = previous })
	}

	t.Run("a recorded warn is a punished hold before the warn lock is released", func(t *testing.T) {
		shortenStale(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := env.mreqMember("Alex", "", "alex")

		var lockHeld, punishedInside bool
		swapApplyWarn(t, func(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64, reason string, afterRecord func()) (warnApplyResult, error) {
			return applyWarn(ctx, b, chat, userID, reason, func() {
				if afterRecord != nil {
					afterRecord()
				}
				if mu := warnPunishLock(chat.Id, userID); mu.TryLock() {
					mu.Unlock()
				} else {
					lockHeld = true
				}
				hold, err := memberreq.GetHoldFresh(context.Background(), chat.Id, userID)
				punishedInside = err == nil && hold != nil && hold.PunishedAt != nil
			})
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("enforcement = %+v, want Warned", got)
		}
		if !lockHeld {
			t.Error("afterRecord ran outside the warn lock")
		}
		if !punishedInside {
			t.Error("the hold was not marked punished by the time the warn lock was released")
		}

		time.Sleep(10 * time.Millisecond)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqHeld {
			t.Fatalf("enforcement after the stale age = %+v, want Held (a punished hold is never reclaimed)", got)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns = %+v, want exactly 1 after a reclaim attempt", row)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want 1", n)
		}
	})

	t.Run("a warn that cannot be recorded leaves a pending hold that one reclaim warns once", func(t *testing.T) {
		shortenStale(t)
		env := newMemberReqEnv(t)
		env.mreqRules(map[memberreq.Rule]int{memberreq.RuleLastName: models.RequirementActionWarn})
		member := env.mreqMember("Alex", "", "alex")

		calls := 0
		swapApplyWarn(t, func(ctx context.Context, b *gotgbot.Bot, chat *gotgbot.Chat, userID int64, reason string, afterRecord func()) (warnApplyResult, error) {
			calls++
			if calls > 1 {
				return applyWarn(ctx, b, chat, userID, reason, afterRecord)
			}
			restore := captchaApplyBrokenDB(t)
			defer restore()
			return applyWarn(ctx, b, chat, userID, reason, afterRecord)
		})

		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqNotApplied {
			t.Fatalf("enforcement with a failing warn write = %+v, want NotApplied", got)
		}
		hold := env.mreqHold(member.Id)
		if hold == nil || hold.PunishedAt != nil {
			t.Fatalf("hold after the failed warn = %+v, want a pending hold", hold)
		}
		if row := env.mreqWarnRow(member.Id); row != nil {
			t.Errorf("warns row = %+v, want none", row)
		}
		if n := len(env.mreqNotices()); n != 0 {
			t.Errorf("notices after the failed warn = %d, want none", n)
		}

		time.Sleep(10 * time.Millisecond)
		if got := env.mreqEnforce(env.mreqCase(member)); got.Outcome != memberReqWarned {
			t.Fatalf("enforcement after the stale age = %+v, want Warned by the reclaim", got)
		}
		if row := env.mreqWarnRow(member.Id); row == nil || row.NumWarns != 1 {
			t.Errorf("warns = %+v, want exactly one", row)
		}
		if hold := env.mreqHold(member.Id); hold == nil || hold.PunishedAt == nil {
			t.Errorf("hold after the reclaim = %+v, want punished", hold)
		}
		if n := len(env.mreqNotices()); n != 1 {
			t.Errorf("notices = %d, want exactly one", n)
		}
	})
}
