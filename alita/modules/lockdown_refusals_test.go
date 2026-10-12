//go:build testtools

package modules

import (
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// captureLockdownLogs records every log entry of the test and makes sure warn level
// is enabled, so a [Lockdown] warning is visible whatever level the suite runs at.
func captureLockdownLogs(t *testing.T) *logrustest.Hook {
	t.Helper()
	hook := logrustest.NewGlobal()
	t.Cleanup(hook.Reset)
	if previous := log.GetLevel(); previous < log.WarnLevel {
		log.SetLevel(log.WarnLevel)
		t.Cleanup(func() { log.SetLevel(previous) })
	}
	return hook
}

// wantLockdownLogged fails unless some entry at warn level or worse carries text.
func wantLockdownLogged(t *testing.T, hook *logrustest.Hook, text string) {
	t.Helper()
	for _, entry := range hook.AllEntries() {
		if entry.Level <= log.WarnLevel && strings.Contains(entry.Message, text) {
			return
		}
	}
	t.Errorf("no log entry at warn level or worse contains %q", text)
}

// newOutsider is a user with a random ID who is not the environment's admin.
func (e *lockdownEnv) newOutsider() gotgbot.User {
	return gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Pat"}
}

// wantNothingRecorded fails when the lockdown chat has an active lockdown row.
func (e *lockdownEnv) wantNothingRecorded() {
	e.t.Helper()
	row, err := lockdown.GetActiveFresh(e.chat.Id)
	if err != nil {
		e.t.Fatalf("GetActiveFresh error = %v", err)
	}
	if row != nil {
		e.t.Errorf("an active lockdown row exists (id %d), want none", row.ID)
	}
}

// wantUntouchedPermissions fails when the group's default permissions are not the
// ones the environment started with.
func (e *lockdownEnv) wantUntouchedPermissions() {
	e.t.Helper()
	if got := e.fake.chatPermsRaw(e.chat.Id); got != lockdownTestPrePermissions {
		e.t.Errorf("group permissions = %q, want them unchanged", got)
	}
}

func TestLockdownCommandsRefuseNonAuthority(t *testing.T) {
	cases := []struct {
		name  string
		setup func(env *lockdownEnv, user gotgbot.User)
		want  string
	}{
		{
			name: "plain member",
			setup: func(env *lockdownEnv, user gotgbot.User) {
				env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			},
			want: staffMarker("chat_status_user_admin_cmd_error"),
		},
		{
			name: "admin without restrict",
			setup: func(env *lockdownEnv, user gotgbot.User) {
				env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{
					Status:             gotgbot.ChatMemberStatusAdministrator,
					CanRestrictMembers: false,
				})
			},
			want: staffMarker("chat_status_restrict_cmd_error"),
		},
		{
			name: "cache says admin, live says member",
			setup: func(env *lockdownEnv, user gotgbot.User) {
				env.fake.setCreator(env.chat.Id, user.Id)
				env.fake.setMember(env.chat.Id, user.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
			},
			want: staffMarker("chat_status_user_admin_cmd_error"),
		},
		{
			name: "lookup fails",
			setup: func(env *lockdownEnv, user gotgbot.User) {
				env.fake.script("getChatMember", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
			want: staffMarker("lockdown_check_failed"),
		},
	}

	for _, tc := range cases {
		t.Run("lockdown/"+tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			user := env.newOutsider()
			tc.setup(env, user)

			env.send(user, "/lockdown")

			env.wantReplyHas(tc.want)
			if calls := env.calls("getChat"); len(calls) != 0 {
				t.Errorf("getChat calls = %d, want 0: nothing is read before the authority check", len(calls))
			}
			if calls := env.calls("setChatPermissions"); len(calls) != 0 {
				t.Errorf("setChatPermissions calls = %d, want 0", len(calls))
			}
			if calls := env.calls("getChatAdministrators"); len(calls) != 0 {
				t.Errorf("getChatAdministrators calls = %d, want 0: the admin list is never authority", len(calls))
			}
			env.wantNothingRecorded()
		})

		t.Run("unlockdown/"+tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.send(env.admin, "/lockdown")
			if row, err := lockdown.GetActiveFresh(env.chat.Id); err != nil || row == nil {
				t.Fatalf("setup: GetActiveFresh = %v, %v, want an active lockdown", row, err)
			}
			user := env.newOutsider()
			tc.setup(env, user)

			env.send(user, "/unlockdown")

			env.wantReplyHas(tc.want)
			if calls := env.calls("setChatPermissions"); len(calls) != 1 {
				t.Errorf("setChatPermissions calls = %d, want only the lock", len(calls))
			}
			row, err := lockdown.GetActiveFresh(env.chat.Id)
			if err != nil || row == nil {
				t.Errorf("GetActiveFresh = %v, %v, want the lockdown to stay active", row, err)
			}
		})
	}

	t.Run("live creator passes", func(t *testing.T) {
		env := newLockdownEnv(t)
		owner := env.newOutsider()
		env.fake.setMember(env.chat.Id, owner.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})

		env.send(owner, "/lockdown")

		row, err := lockdown.GetActiveFresh(env.chat.Id)
		if err != nil || row == nil {
			t.Fatalf("GetActiveFresh = %v, %v, want an active lockdown", row, err)
		}
		if row.StartedBy != owner.Id {
			t.Errorf("StartedBy = %d, want the creator %d", row.StartedBy, owner.Id)
		}
	})
}

func TestLockdownRefusals(t *testing.T) {
	refusals := []struct {
		name  string
		setup func(env *lockdownEnv)
		want  string
		// rawDetail is text of Telegram's own answer. It must stay out of the reply and
		// go to the [Lockdown] log.
		rawDetail string
		// logged is text a [Lockdown] log entry must carry for a refusal with no
		// Telegram answer.
		logged string
	}{
		{
			name:  "basic group",
			setup: func(env *lockdownEnv) { env.chat.Type = "group" },
			want:  staffMarker("lockdown_basic_group"),
		},
		{
			name:  "bot not admin",
			setup: func(env *lockdownEnv) { env.fake.setBotRole(env.chat.Id, staffRoleMember) },
			want:  staffMarker("lockdown_bot_cannot_restrict"),
		},
		{
			name:  "bot cannot restrict",
			setup: func(env *lockdownEnv) { env.fake.setBotRole(env.chat.Id, staffRoleAdminNoRestrict) },
			want:  staffMarker("lockdown_bot_cannot_restrict"),
		},
		{
			name: "bot lookup fails",
			setup: func(env *lockdownEnv) {
				// The first getChatMember is the commander's live check, the second the bot's.
				env.fake.script("getChatMember", env.chat.Id, nil, staffFakeError(500, "Internal Server Error"))
			},
			want: staffMarker("lockdown_bot_check_failed"),
		},
		{
			name: "getChat fails",
			setup: func(env *lockdownEnv) {
				env.fake.script("getChat", env.chat.Id, staffFakeError(500, "Internal Server Error"))
			},
			want:      staffMarker("lockdown_permissions_unreadable"),
			rawDetail: "Internal Server Error",
		},
		{
			name:   "no permissions",
			setup:  func(env *lockdownEnv) { env.fake.clearChatPerms(env.chat.Id) },
			want:   staffMarker("lockdown_permissions_unreadable"),
			logged: "returned no permissions",
		},
		{
			name: "lock call refused",
			setup: func(env *lockdownEnv) {
				env.fake.script("setChatPermissions", env.chat.Id, staffFakeError(400, "Bad Request: not enough rights"))
			},
			want:      staffMarker("lockdown_lock_failed"),
			rawDetail: "not enough rights",
		},
	}

	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			tc.setup(env)
			unchanged := env.fake.chatPermsRaw(env.chat.Id)
			hook := captureLockdownLogs(t)

			env.send(env.admin, "/lockdown")

			env.wantReplyHas(tc.want)
			if tc.rawDetail != "" {
				env.wantReplyLacks(tc.rawDetail)
				wantLockdownLogged(t, hook, tc.rawDetail)
			}
			if tc.logged != "" {
				wantLockdownLogged(t, hook, tc.logged)
			}
			env.wantNothingRecorded()
			if got := env.fake.chatPermsRaw(env.chat.Id); got != unchanged {
				t.Errorf("group permissions = %q, want %q: a refusal changes nothing", got, unchanged)
			}
			if len(env.calls("restrictChatMember")) != 0 {
				t.Error("a refusal made a per-user restrict call")
			}
		})
	}

	notes := []struct {
		name      string
		bot       string
		wantNote  string
		otherNote string
	}{
		{
			name:      "no delete right",
			bot:       lockdownBotAdminJSON(true, false, true),
			wantNote:  staffMarker("lockdown_note_no_delete"),
			otherNote: staffMarker("lockdown_note_no_invite"),
		},
		{
			name:      "no invite right",
			bot:       lockdownBotAdminJSON(true, true, false),
			wantNote:  staffMarker("lockdown_note_no_invite"),
			otherNote: staffMarker("lockdown_note_no_delete"),
		},
	}
	for _, tc := range notes {
		t.Run(tc.name, func(t *testing.T) {
			env := newLockdownEnv(t)
			env.fake.setBotMember(env.chat.Id, tc.bot)

			env.send(env.admin, "/lockdown")

			row, err := lockdown.GetActiveFresh(env.chat.Id)
			if err != nil || row == nil || row.LockedAt == nil {
				t.Fatalf("GetActiveFresh = %v, %v, want a confirmed lockdown: a missing right only adds a note", row, err)
			}
			env.wantReplyHas(staffMarker("lockdown_started"), tc.wantNote)
			if strings.Contains(env.lastReply(), tc.otherNote) {
				t.Errorf("reply carries %q although that right is present", tc.otherNote)
			}
		})
	}
}

func TestLockdownCommandReportsExisting(t *testing.T) {
	env := newLockdownEnv(t)

	env.send(env.admin, "/lockdown first reason")
	first, err := lockdown.GetActiveFresh(env.chat.Id)
	if err != nil || first == nil || first.LockedAt == nil {
		t.Fatalf("GetActiveFresh = %v, %v, want a confirmed lockdown", first, err)
	}

	env.send(env.admin, "/lockdown other reason")

	env.wantReplyHas(
		staffMarker("lockdown_already_active"),
		lockdownTime(*first.LockedAt),
		"Ad&lt;b&gt;min",
		"first reason",
	)
	if strings.Contains(env.lastReply(), "other reason") {
		t.Error("the reply carries the second reason, want the first lockdown's")
	}
	if calls := env.calls("setChatPermissions"); len(calls) != 1 {
		t.Errorf("setChatPermissions calls = %d, want 1: a second /lockdown makes no Telegram write", len(calls))
	}
	var rows int64
	if err := db.DB.Model(&models.ChatLockdown{}).Where("chat_id = ?", env.chat.Id).Count(&rows).Error; err != nil {
		t.Fatalf("count lockdown rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("chat_lockdowns rows = %d, want 1", rows)
	}
}

func TestLockdownNoticeAndNoPerUserCalls(t *testing.T) {
	env := newLockdownEnv(t)
	approved := env.newOutsider()
	if err := approvals.AddApprovedUser(env.chat.Id, approved.Id, env.admin.Id, "trusted"); err != nil {
		t.Fatalf("AddApprovedUser error = %v", err)
	}
	t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ApprovedUsers{}) })

	env.send(env.admin, "/lockdown")

	env.wantReplyHas(staffMarker("lockdown_note_approved_muted"))
	if calls := env.fake.callsFor("restrictChatMember"); len(calls) != 0 {
		t.Errorf("restrictChatMember calls = %d, want 0: approved users are muted by the default, not per user", len(calls))
	}
}
