//go:build testtools

package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/config"
	"github.com/divkix/Alita_Robot/alita/db"
	dbcache "github.com/divkix/Alita_Robot/alita/db/cache"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// autoRow reads the env's chat's chat_raid_detection row straight from the database and
// reports whether there is one.
func (e *lockdownEnv) autoRow() (models.ChatRaidDetection, bool) {
	e.t.Helper()
	var rows []models.ChatRaidDetection
	if err := db.DB.Where("chat_id = ?", e.chat.Id).Find(&rows).Error; err != nil {
		e.t.Fatalf("read the raid detection row: %v", err)
	}
	if len(rows) == 0 {
		return models.ChatRaidDetection{}, false
	}
	return rows[0], true
}

// wantAutoRow fails unless the env's chat has a row with exactly these columns.
func (e *lockdownEnv) wantAutoRow(joinOn bool, joinN int, floodOn bool, floodN int) {
	e.t.Helper()
	row, ok := e.autoRow()
	if !ok {
		e.t.Fatal("no chat_raid_detection row, want one")
	}
	if row.JoinEnabled != joinOn || row.JoinThreshold != joinN || row.FloodEnabled != floodOn || row.FloodThreshold != floodN {
		e.t.Errorf("row = joins %v/%d, flood %v/%d; want joins %v/%d, flood %v/%d",
			row.JoinEnabled, row.JoinThreshold, row.FloodEnabled, row.FloodThreshold, joinOn, joinN, floodOn, floodN)
	}
}

// wantAutoAIRow fails unless the env's chat has a row whose AI trigger columns are
// exactly these. It sits beside wantAutoRow, which keeps its signature for the join and
// flood columns.
func (e *lockdownEnv) wantAutoAIRow(aiOn bool, aiN int) {
	e.t.Helper()
	row, ok := e.autoRow()
	if !ok {
		e.t.Fatal("no chat_raid_detection row, want one")
	}
	if row.AIEnabled != aiOn || row.AIThreshold != aiN {
		e.t.Errorf("row = ai %v/%d; want ai %v/%d", row.AIEnabled, row.AIThreshold, aiOn, aiN)
	}
}

// wantNoAutoRow fails when the env's chat has a chat_raid_detection row: a refusal
// saves nothing.
func (e *lockdownEnv) wantNoAutoRow() {
	e.t.Helper()
	if row, ok := e.autoRow(); ok {
		e.t.Errorf("a chat_raid_detection row exists (%+v), want none", row)
	}
}

func TestAutoLockdown(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(
			staffMarker("lockdown_auto_title"),
			staffMarker("lockdown_auto_joins_on")+" 15",
			staffMarker("lockdown_auto_flood_on")+" 30",
		)
		env.wantNoAutoRow()
	})

	t.Run("joins 20 is followed by the next burst on any replica", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown joins 20")

		env.wantReplyHas(
			staffMarker("lockdown_auto_saved"),
			staffMarker("lockdown_auto_joins_on")+" 20",
			staffMarker("lockdown_auto_flood_on")+" 30",
		)
		env.wantAutoRow(true, 20, true, 30)

		// Another replica has nothing of this one's in-process cache.
		dbcache.ResetLocalForTest()

		env.selfJoins(19)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 19 joins with the threshold at 20, want none", row.TriggerKind)
		}
		twentieth := env.newJoiner("Twentieth")
		env.join(twentieth, twentieth, "")

		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after the 20th join in a minute")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge || row.TriggerCount != 20 {
			t.Errorf("trigger = %q with count %d, want a join surge of 20", row.TriggerKind, row.TriggerCount)
		}
	})

	t.Run("joins off then on keeps the threshold", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown joins off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_off")+" 15")
		env.wantAutoRow(false, 15, true, 30)

		env.selfJoins(25)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after 25 joins with the join surge off, want none", row.TriggerKind)
		}

		env.send(env.admin, "/autolockdown joins on")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_on")+" 15")
		env.wantAutoRow(true, 15, true, 30)

		next := env.newJoiner("Twenty Sixth")
		env.join(next, next, "")
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown on the next join after the join surge was switched on again")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
		}
	})

	t.Run("flood off leaves the join surge on", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown flood off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_flood_off")+" 30")
		env.wantAutoRow(true, 15, false, 30)

		recent := env.newMembers(4, 2*time.Hour)
		env.sendItems(recent, 30)
		if row := env.activeLockdown(); row != nil {
			t.Fatalf("a lockdown (kind %q) exists after a flood with the flood trigger off, want none", row.TriggerKind)
		}

		env.selfJoins(15)
		row := env.activeLockdown()
		if row == nil {
			t.Fatal("no active lockdown after 15 joins: switching the flood trigger off must leave the join surge on")
		}
		if row.TriggerKind != models.LockdownTriggerJoinSurge {
			t.Errorf("TriggerKind = %q, want %q", row.TriggerKind, models.LockdownTriggerJoinSurge)
		}
	})
}

// withAIKeys sets the two AI provider keys of the server for one test: TypeSafe (which
// also needs ENABLE_AISPAM) and the image provider. An empty string means not configured.
func withAIKeys(t *testing.T, typeSafe, image string) {
	t.Helper()
	previousKey, previousEnabled, previousImage := config.AppConfig.TypeSafeAPIKey, config.AppConfig.EnableAISpam, config.AppConfig.AnthropicAPIKey
	config.AppConfig.TypeSafeAPIKey = typeSafe
	config.AppConfig.EnableAISpam = true
	config.AppConfig.AnthropicAPIKey = image
	t.Cleanup(func() {
		config.AppConfig.TypeSafeAPIKey = previousKey
		config.AppConfig.EnableAISpam = previousEnabled
		config.AppConfig.AnthropicAPIKey = previousImage
	})
}

func TestAutolockdownAI(t *testing.T) {
	t.Run("the view lists the AI trigger", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "typesafe-test-key", "")

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(
			staffMarker("lockdown_auto_title"),
			staffMarker("lockdown_auto_ai_on")+" 5",
		)
		env.wantReplyLacks(staffMarker("lockdown_auto_ai_unavailable"))
		env.wantNoAutoRow()
	})

	t.Run("the view of a trigger that is off", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "typesafe-test-key", "")
		seedRaidSettings(t, models.ChatRaidDetection{
			ChatID: env.chat.Id, JoinEnabled: true, JoinThreshold: 15, FloodEnabled: true, FloodThreshold: 30,
			AIEnabled: false, AIThreshold: 9,
		})

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(staffMarker("lockdown_auto_ai_off") + " 9")
		env.wantReplyLacks(staffMarker("lockdown_auto_ai_on"))
	})

	t.Run("no AI keys at all says so after the AI line", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "", "")

		env.send(env.admin, "/autolockdown")

		reply := env.lastReply()
		line := strings.Index(reply, staffMarker("lockdown_auto_ai_on"))
		notice := strings.Index(reply, staffMarker("lockdown_auto_ai_unavailable"))
		if line < 0 || notice < 0 || notice < line {
			t.Errorf("reply %q: the AI line is at %d and the not-set-up notice at %d, want the notice after the line", reply, line, notice)
		}
	})

	t.Run("one provider is enough for no notice", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "", "anthropic-test-key")

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(staffMarker("lockdown_auto_ai_on") + " 5")
		env.wantReplyLacks(staffMarker("lockdown_auto_ai_unavailable"))
	})

	t.Run("the hint names the ai word", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown")

		env.wantReplyHas(staffMarker("lockdown_auto_hint"))
	})

	t.Run("ai 8 saves and keeps the other triggers", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "typesafe-test-key", "")

		env.send(env.admin, "/autolockdown ai 8")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_ai_on")+" 8")
		env.wantAutoAIRow(true, 8)
		env.wantAutoRow(true, 15, true, 30)
	})

	t.Run("ai off keeps the threshold", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		withAIKeys(t, "typesafe-test-key", "")

		env.send(env.admin, "/autolockdown ai 8")
		env.send(env.admin, "/autolockdown ai off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_ai_off")+" 8")
		env.wantAutoAIRow(false, 8)

		env.send(env.admin, "/autolockdown ai on")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_ai_on")+" 8")
		env.wantAutoAIRow(true, 8)
		env.wantAutoRow(true, 15, true, 30)
	})

	t.Run("the other triggers leave the AI trigger alone", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown ai 8")
		env.send(env.admin, "/autolockdown joins 20")

		env.wantAutoAIRow(true, 8)
		env.wantAutoRow(true, 20, true, 30)
	})

	t.Run("out of range writes nothing", func(t *testing.T) {
		for _, text := range []string{"/autolockdown ai 2", "/autolockdown ai 501"} {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)

			env.send(env.admin, text)

			env.wantReplyHas(staffMarker("lockdown_auto_range") + " 3 500")
			env.wantReplyLacks(staffMarker("lockdown_auto_saved"))
			env.wantNoAutoRow()
		}
	})

	t.Run("bad words write nothing", func(t *testing.T) {
		for _, text := range []string{"/autolockdown ai maybe", "/autolockdown ai", "/autolockdown ai 8 9"} {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)

			env.send(env.admin, text)

			env.wantReplyHas(staffMarker("lockdown_auto_usage"))
			env.wantNoAutoRow()
		}
	})

	t.Run("an administrator without can_restrict_members may view but not change", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})

		env.send(helper, "/autolockdown")
		env.wantReplyHas(staffMarker("lockdown_auto_ai_on") + " 5")

		env.send(helper, "/autolockdown ai 8")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()

		env.send(helper, "/autolockdown ai off")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("a plain member is refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		member := env.newOutsider()
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(member, "/autolockdown ai off")

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("an anonymous admin proves first", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		raidCleanup(t, env.chat.Id)

		env.sendAnonymous("/autolockdown ai 8")
		env.wantProofPrompt()
		env.wantNoAutoRow()

		env.tapProof(creator)
		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_ai_on")+" 8")
		env.wantAutoAIRow(true, 8)
	})

	t.Run("an anonymous admin who taps without the restrict right saves nothing", func(t *testing.T) {
		env, _ := newAnonLockdownEnv(t, true)
		raidCleanup(t, env.chat.Id)
		helper := gotgbot.User{Id: uniqueLinkOwnerID() + 43, FirstName: "Helper"}
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		env.fake.setCreator(env.chat.Id, helper.Id)

		env.sendAnonymous("/autolockdown ai off")
		env.wantProofPrompt()
		env.tapProof(helper)

		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("a basic group is refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.chat.Type = "group"

		env.send(env.admin, "/autolockdown ai 8")

		env.wantReplyHas(staffMarker("lockdown_basic_group"))
		env.wantReplyLacks(staffMarker("lockdown_auto_saved"))
		env.wantNoAutoRow()
	})

	t.Run("a failed save writes nothing", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		previous := autolockdownWrite
		autolockdownWrite = func(int64, autoLockdownChange) error { return errors.New("database is gone") }
		t.Cleanup(func() { autolockdownWrite = previous })

		env.send(env.admin, "/autolockdown ai 8")

		env.wantReplyHas(staffMarker("lockdown_auto_save_failed"))
		env.wantReplyLacks(staffMarker("lockdown_auto_saved"), "database is gone")
		env.wantNoAutoRow()
	})

	t.Run("the next burst follows the change on any replica", func(t *testing.T) {
		env := newAIBurstEnv(t)
		env.send(env.admin, "/autolockdown ai 3")
		env.wantReplyHas(staffMarker("lockdown_auto_saved"))
		env.wantAutoAIRow(true, 3)

		// Another replica has nothing of this one's in-process cache.
		dbcache.ResetLocalForTest()

		accounts := env.newMembers(3, 0)
		for _, account := range accounts {
			env.spamFrom(account, 1)
		}
		env.settleAI()

		row := env.activeLockdown()
		if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst {
			t.Fatalf("active lockdown = %+v after 3 verdicts from 3 accounts with the threshold at 3, want an AI burst lockdown", row)
		}
		if row.TriggerCount != 3 {
			t.Errorf("TriggerCount = %d, want 3", row.TriggerCount)
		}
	})

	t.Run("the next burst obeys ai off on any replica", func(t *testing.T) {
		env := newAIBurstEnv(t)
		env.send(env.admin, "/autolockdown ai off")
		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_ai_off")+" 5")
		env.wantAutoAIRow(false, 5)

		dbcache.ResetLocalForTest()

		accounts := env.newMembers(3, 0)
		for _, account := range accounts {
			env.spamFrom(account, 2)
		}
		env.settleAI()

		if row := env.activeLockdown(); row != nil {
			t.Errorf("a lockdown (kind %q) exists after 6 verdicts with the AI trigger off, want none", row.TriggerKind)
		}
		if got := aiVerdictSetSize(env, "av"); got != 0 {
			t.Errorf("verdict items counted = %d, want 0 while the trigger is off", got)
		}
	})
}

func TestAutoLockdownArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want autoLockdownChange
		err  error
	}{
		{name: "no words is the status", args: nil, want: autoLockdownChange{}},
		{name: "a number sets the threshold and turns the trigger on", args: []string{"joins", "20"},
			want: autoLockdownChange{Trigger: autoTriggerJoins, Enable: true, SetThreshold: true, Threshold: 20}},
		{name: "the trigger is case-insensitive", args: []string{"JOINS", "Off"},
			want: autoLockdownChange{Trigger: autoTriggerJoins}},
		{name: "flood on", args: []string{"flood", "on"},
			want: autoLockdownChange{Trigger: autoTriggerFlood, Enable: true}},
		{name: "flood with the lowest threshold", args: []string{"flood", "3"},
			want: autoLockdownChange{Trigger: autoTriggerFlood, Enable: true, SetThreshold: true, Threshold: 3}},
		{name: "joins with the highest threshold", args: []string{"joins", "500"},
			want: autoLockdownChange{Trigger: autoTriggerJoins, Enable: true, SetThreshold: true, Threshold: 500}},
		{name: "a trigger alone", args: []string{"joins"}, err: errAutoLockdownUsage},
		{name: "below the range", args: []string{"joins", "2"}, err: errAutoLockdownRange},
		{name: "above the range", args: []string{"joins", "501"}, err: errAutoLockdownRange},
		{name: "negative", args: []string{"joins", "-5"}, err: errAutoLockdownRange},
		{name: "zero", args: []string{"flood", "0"}, err: errAutoLockdownRange},
		{name: "longer than 64 bits", args: []string{"flood", "99999999999999999999"}, err: errAutoLockdownRange},
		{name: "not a number", args: []string{"joins", "abc"}, err: errAutoLockdownUsage},
		{name: "a decimal", args: []string{"joins", "20.5"}, err: errAutoLockdownUsage},
		{name: "a plus sign", args: []string{"joins", "+20"}, err: errAutoLockdownUsage},
		{name: "a lone minus", args: []string{"joins", "-"}, err: errAutoLockdownUsage},
		{name: "a third word", args: []string{"flood", "20", "extra"}, err: errAutoLockdownUsage},
		{name: "an unknown trigger", args: []string{"banana", "20"}, err: errAutoLockdownUsage},
		{name: "ai with a number", args: []string{"ai", "8"},
			want: autoLockdownChange{Trigger: autoTriggerAI, Enable: true, SetThreshold: true, Threshold: 8}},
		{name: "ai is case-insensitive", args: []string{"AI", "on"},
			want: autoLockdownChange{Trigger: autoTriggerAI, Enable: true}},
		{name: "ai off", args: []string{"ai", "off"}, want: autoLockdownChange{Trigger: autoTriggerAI}},
		{name: "ai below the range", args: []string{"ai", "2"}, err: errAutoLockdownRange},
		{name: "ai above the range", args: []string{"ai", "501"}, err: errAutoLockdownRange},
		{name: "ai with a word", args: []string{"ai", "maybe"}, err: errAutoLockdownUsage},
		{name: "ai alone", args: []string{"ai"}, err: errAutoLockdownUsage},
		{name: "ai with a third word", args: []string{"ai", "8", "9"}, err: errAutoLockdownUsage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAutoLockdownArgs(tc.args)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if tc.err == nil && got != tc.want {
				t.Errorf("change = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// sendAs runs a message in the env's chat as from, posted on behalf of senderChat.
func (e *lockdownEnv) sendAs(from gotgbot.User, senderChat gotgbot.Chat, text string) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		Message: &gotgbot.Message{
			MessageId:  id,
			Date:       1,
			Chat:       e.chat,
			From:       &from,
			SenderChat: &senderChat,
			Text:       text,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

func TestAutoLockdownAuthority(t *testing.T) {
	t.Run("an admin without the restrict right may view but not change", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		helper := env.newOutsider()
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})

		env.send(helper, "/autolockdown")
		env.wantReplyHas(staffMarker("lockdown_auto_title"), staffMarker("lockdown_auto_joins_on")+" 15")

		env.send(helper, "/autolockdown joins 20")
		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("the creator changes", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		owner := env.newOutsider()
		env.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

		env.send(owner, "/autolockdown flood 40")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_flood_on")+" 40")
		env.wantAutoRow(true, 15, true, 40)
	})

	t.Run("an administrator with the restrict right changes", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)

		env.send(env.admin, "/autolockdown flood off")

		env.wantReplyHas(staffMarker("lockdown_auto_saved"))
		env.wantAutoRow(true, 15, false, 30)
	})

	t.Run("a plain member is refused, to view and to change", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		member := env.newOutsider()
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(member, "/autolockdown")
		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))

		env.send(member, "/autolockdown joins 20")
		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("the admin cache never authorizes", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		stale := env.newOutsider()
		env.fake.setCreator(env.chat.Id, stale.Id)
		env.fake.setMember(env.chat.Id, stale.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})

		env.send(stale, "/autolockdown joins 20")

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNoAutoRow()
		if calls := env.calls("getChatAdministrators"); len(calls) != 0 {
			t.Errorf("getChatAdministrators calls = %d, want 0: the admin list is never authority", len(calls))
		}
	})

	t.Run("a failed lookup refuses", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))

		env.send(env.admin, "/autolockdown joins 20")

		env.wantReplyHas(staffMarker("lockdown_check_failed"))
		env.wantNoAutoRow()
	})

	t.Run("a private chat is refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		env.chat.Type = "private"

		env.send(env.admin, "/autolockdown joins 20")

		env.wantReplyHas(staffMarker("chat_status_group_only_error"))
		env.wantNoAutoRow()
	})

	t.Run("a message sent as another chat is refused", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		channel := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "channel", Title: "Elsewhere"}

		env.sendAs(env.admin, channel, "/autolockdown joins 20")

		env.wantReplyHas(staffMarker("chat_status_user_admin_cmd_error"))
		env.wantNoAutoRow()
	})

	t.Run("an anonymous admin proves who they are first, then is checked live", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, false)
		raidCleanup(t, env.chat.Id)

		env.sendAnonymous("/autolockdown joins 20")
		env.wantProofPrompt()
		env.wantNoAutoRow()

		env.tapProof(creator)
		env.wantReplyHas(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_joins_on")+" 20")
		env.wantAutoRow(true, 20, true, 30)
	})

	t.Run("an anonymous admin may view after the proof", func(t *testing.T) {
		env, creator := newAnonLockdownEnv(t, true)
		raidCleanup(t, env.chat.Id)

		env.sendAnonymous("/autolockdown")
		env.wantProofPrompt()

		env.tapProof(creator)
		env.wantReplyHas(staffMarker("lockdown_auto_title"))
		env.wantNoAutoRow()
	})

	t.Run("an anonymous admin who taps without the restrict right saves nothing", func(t *testing.T) {
		env, _ := newAnonLockdownEnv(t, true)
		raidCleanup(t, env.chat.Id)
		helper := gotgbot.User{Id: uniqueLinkOwnerID() + 41, FirstName: "Helper"}
		env.fake.setMember(env.chat.Id, helper.Id, staffFakeMember{
			Status:             gotgbot.ChatMemberStatusAdministrator,
			CanRestrictMembers: false,
		})
		env.fake.setCreator(env.chat.Id, helper.Id)

		env.sendAnonymous("/autolockdown joins 20")
		env.wantProofPrompt()
		env.tapProof(helper)

		env.wantReplyHas(staffMarker("chat_status_restrict_cmd_error"))
		env.wantNoAutoRow()
	})
}

func TestAutoLockdownBadInput(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"a trigger alone", "/autolockdown joins", staffMarker("lockdown_auto_usage")},
		{"an unknown trigger", "/autolockdown banana 20", staffMarker("lockdown_auto_usage")},
		{"not a number", "/autolockdown joins abc", staffMarker("lockdown_auto_usage")},
		{"a third word", "/autolockdown flood 20 extra", staffMarker("lockdown_auto_usage")},
		{"below the range", "/autolockdown joins 2", staffMarker("lockdown_auto_range") + " 3 500"},
		{"above the range", "/autolockdown flood 501", staffMarker("lockdown_auto_range") + " 3 500"},
		{"negative", "/autolockdown joins -5", staffMarker("lockdown_auto_range") + " 3 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)

			env.send(env.admin, tc.text)

			env.wantReplyHas(tc.want)
			env.wantReplyLacks(staffMarker("lockdown_auto_saved"))
			env.wantNoAutoRow()
		})
	}
}

func TestAutoLockdownBasicGroup(t *testing.T) {
	for _, text := range []string{"/autolockdown joins 20", "/autolockdown"} {
		t.Run(text, func(t *testing.T) {
			env := newLockdownEnv(t)
			raidCleanup(t, env.chat.Id)
			env.chat.Type = "group"

			env.send(env.admin, text)

			env.wantReplyHas(staffMarker("lockdown_basic_group"))
			env.wantReplyLacks(staffMarker("lockdown_auto_saved"), staffMarker("lockdown_auto_title"))
			env.wantNoAutoRow()
		})
	}
}

func TestAutoLockdownSaveFails(t *testing.T) {
	env := newLockdownEnv(t)
	raidCleanup(t, env.chat.Id)

	env.send(env.admin, "/autolockdown joins 25")
	env.wantReplyHas(staffMarker("lockdown_auto_saved"))
	env.wantAutoRow(true, 25, true, 30)

	previous := autolockdownWrite
	autolockdownWrite = func(int64, autoLockdownChange) error { return errors.New("database is gone") }
	t.Cleanup(func() { autolockdownWrite = previous })

	env.send(env.admin, "/autolockdown joins 40")

	env.wantReplyHas(staffMarker("lockdown_auto_save_failed"))
	env.wantReplyLacks(staffMarker("lockdown_auto_saved"), "database is gone")
	env.wantAutoRow(true, 25, true, 30)
}

// TestAutoLockdownReadsAreBounded pins that the settings reads of /autolockdown run under
// raidSettingsTimeout, the view and the readback after a save alike, and that a failed
// read gets its reply instead of holding the handler.
func TestAutoLockdownReadsAreBounded(t *testing.T) {
	withLookup := func(t *testing.T, deadlines *[]time.Duration) {
		t.Helper()
		previous := raidSettingsLookup
		raidSettingsLookup = func(ctx context.Context, _ int64) (*models.ChatRaidDetection, error) {
			if deadline, ok := ctx.Deadline(); ok {
				*deadlines = append(*deadlines, time.Until(deadline))
			} else {
				*deadlines = append(*deadlines, -1)
			}
			return nil, errors.New("settings unreadable")
		}
		t.Cleanup(func() { raidSettingsLookup = previous })
	}
	wantBounded := func(t *testing.T, deadlines []time.Duration, reads int) {
		t.Helper()
		if len(deadlines) != reads {
			t.Fatalf("settings reads = %d, want %d", len(deadlines), reads)
		}
		for _, left := range deadlines {
			if left <= 0 || left > raidSettingsTimeout {
				t.Errorf("a settings read had %v left, want a deadline within (0, %v]", left, raidSettingsTimeout)
			}
		}
	}

	t.Run("the view", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		var deadlines []time.Duration
		withLookup(t, &deadlines)

		env.send(env.admin, "/autolockdown")

		wantBounded(t, deadlines, 1)
		env.wantReplyHas(staffMarker("lockdown_auto_read_failed"))
	})

	t.Run("the readback after a save", func(t *testing.T) {
		env := newLockdownEnv(t)
		raidCleanup(t, env.chat.Id)
		var deadlines []time.Duration
		withLookup(t, &deadlines)

		env.send(env.admin, "/autolockdown joins 20")

		wantBounded(t, deadlines, 1)
		env.wantReplyHas(staffMarker("lockdown_auto_saved"))
		env.wantAutoRow(true, 20, true, 30)
	})
}
