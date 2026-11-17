//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/connections"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
)

// reportsPMEnv is the lockdown environment with the reports module on its real dispatcher, the
// env admin listed as the group's creator and the bot an administrator there.
func reportsPMEnv(t *testing.T) *settingsEnv {
	t.Helper()
	env := &settingsEnv{lockdownEnv: newRealLocaleLockdownEnv(t)}
	env.fake.setCreator(env.chat.Id, env.admin.Id)
	env.fake.setBotRole(env.chat.Id, staffRoleAdmin)
	LoadReports(env.dispatcher)
	reportsGoldenCleanup(t, env.chat.Id)
	return env
}

// reportsPMUser cleans the user's connection, report preference and users rows before and after the test.
func reportsPMUser(t *testing.T, userID int64) {
	t.Helper()
	clean := func() {
		db.DB.Where("user_id = ?", userID).Delete(&models.ConnectionSettings{})
		db.DB.Where("user_id = ?", userID).Delete(&models.ReportUserSettings{})
		db.DB.Where("user_id = ?", userID).Delete(&models.User{})
		dbcache.DeleteCache(dbcache.CacheKey("user", userID))
		dbcache.ResetLocalForTest()
	}
	clean()
	t.Cleanup(clean)
}

// reportsPMText is the translated text the user reads in their private chat.
func reportsPMText(t *testing.T, env *settingsEnv, user gotgbot.User, key string) string {
	t.Helper()
	ctx := newModuleMessageContext(env.bot, gotgbot.Chat{Id: user.Id, Type: "private", FirstName: user.FirstName}, user, "/reports")
	text, _ := i18n.MustNewTranslator(lang.GetLanguage(ctx)).GetString(key)
	if text == "" {
		t.Fatalf("locale key %s is empty", key)
	}
	return text
}

// reportsLastText returns the text of the newest message the bot sent to chatID.
func reportsLastText(t *testing.T, env *settingsEnv, chatID int64) string {
	t.Helper()
	sent := env.fake.sentTo(chatID)
	if len(sent) == 0 {
		t.Fatalf("the bot sent nothing to %d", chatID)
	}
	return fmt.Sprint(sent[len(sent)-1].Params["text"])
}

func reportsUserRow(t *testing.T, userID int64) *models.ReportUserSettings {
	t.Helper()
	var row models.ReportUserSettings
	err := db.DB.Where("user_id = ?", userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("read report_user_settings: %v", err)
	}
	return &row
}

// TestReportsPrivateConnection drives /reports in a private chat through the real dispatcher: with no
// connection it is the user's own preference and needs no group; with a connection it sets and shows
// the connected group's switch, as the same command does in the group, and only for an admin.
func TestReportsPrivateConnection(t *testing.T) {
	t.Run("no connection sets the personal preference", func(t *testing.T) {
		env := reportsPMEnv(t)
		user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Reader"}
		reportsPMUser(t, user.Id)

		env.sendPrivate(user, "/reports on")
		if got, want := reportsLastText(t, env, user.Id), reportsPMText(t, env, user, "reports_turned_on_personal"); got != want {
			t.Fatalf("/reports on replied %q, want %q", got, want)
		}
		if row := reportsUserRow(t, user.Id); row == nil || !row.Status || !row.Enabled {
			t.Fatalf("the user's preference is %+v, want on", row)
		}

		env.sendPrivate(user, "/reports off")
		if got, want := reportsLastText(t, env, user.Id), reportsPMText(t, env, user, "reports_turned_off_personal"); got != want {
			t.Fatalf("/reports off replied %q, want %q", got, want)
		}
		if row := reportsUserRow(t, user.Id); row == nil || row.Status {
			t.Fatalf("the user's preference is %+v, want off", row)
		}

		env.sendPrivate(user, "/reports")
		want := reportsPMText(t, env, user, "reports_preference_disabled_private") + reportsPMText(t, env, user, "reports_change_settings_hint")
		if got := reportsLastText(t, env, user.Id); got != want {
			t.Fatalf("/reports replied %q, want %q", got, want)
		}
		if stored := reportsGoldenStored(t, env.chat.Id); stored.exists {
			t.Fatalf("a personal /reports wrote a group row: %+v", stored)
		}
	})

	t.Run("a connection sets the connected group's switch", func(t *testing.T) {
		env := reportsPMEnv(t)
		admin := env.admin
		reportsPMUser(t, admin.Id)
		if err := connections.ConnectId(admin.Id, env.chat.Id); err != nil {
			t.Fatalf("ConnectId() error = %v", err)
		}

		env.sendPrivate(admin, "/reports off")
		pmOff := reportsLastText(t, env, admin.Id)
		if stored := reportsGoldenStored(t, env.chat.Id); !stored.exists || stored.status || stored.enabled {
			t.Fatalf("the group row is %+v after /reports off in PM, want off", stored)
		}
		env.sendIn(env.chat, admin, "/reports off")
		if groupOff := reportsLastText(t, env, env.chat.Id); pmOff != groupOff {
			t.Fatalf("PM /reports off replied %q, the group %q", pmOff, groupOff)
		}

		env.sendPrivate(admin, "/reports")
		pmStatus := reportsLastText(t, env, admin.Id)
		env.sendIn(env.chat, admin, "/reports")
		if groupStatus := reportsLastText(t, env, env.chat.Id); pmStatus != groupStatus {
			t.Fatalf("PM /reports replied %q, the group %q", pmStatus, groupStatus)
		}

		env.sendPrivate(admin, "/reports on")
		if stored := reportsGoldenStored(t, env.chat.Id); !stored.exists || !stored.status || !stored.enabled {
			t.Fatalf("the group row is %+v after /reports on in PM, want on", stored)
		}
		if row := reportsUserRow(t, admin.Id); row != nil {
			t.Fatalf("a connected /reports wrote the user's own preference: %+v", row)
		}
	})

	t.Run("a connected member who is not an admin changes nothing", func(t *testing.T) {
		env := reportsPMEnv(t)
		member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
		reportsPMUser(t, member.Id)
		env.fake.setMember(env.chat.Id, member.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		if err := connections.ConnectId(member.Id, env.chat.Id); err != nil {
			t.Fatalf("ConnectId() error = %v", err)
		}

		env.sendPrivate(member, "/reports off")
		if got, want := reportsLastText(t, env, member.Id), reportsPMText(t, env, member, "connections_is_user_connected_user_not_admin"); got != want {
			t.Fatalf("/reports off replied %q, want the not-admin refusal %q", got, want)
		}
		if stored := reportsGoldenStored(t, env.chat.Id); stored.exists {
			t.Fatalf("a non-admin changed the group row: %+v", stored)
		}
		if row := reportsUserRow(t, member.Id); row != nil {
			t.Fatalf("a connected non-admin wrote a preference: %+v", row)
		}
	})
}
