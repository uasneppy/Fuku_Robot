//go:build testtools

package modules

import (
	"context"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

// withAdminIndexObserver installs the module's admin list observer for the test, as
// LoadAdminIndex does in production, and removes it afterwards.
func withAdminIndexObserver(t *testing.T) {
	t.Helper()
	cache.SetAdminListObserver(adminIndexObserve)
	t.Cleanup(func() { cache.SetAdminListObserver(nil) })
}

// adminIndexRoster reads the stored roster of chatID as a user ID to role map.
func adminIndexRoster(t *testing.T, chatID int64) map[int64]string {
	t.Helper()
	rows, err := adminindex.ListChatFresh(context.Background(), chatID)
	if err != nil {
		t.Fatalf("ListChatFresh(%d) error = %v", chatID, err)
	}
	roster := make(map[int64]string, len(rows))
	for _, row := range rows {
		roster[row.UserID] = row.Role
	}
	return roster
}

// wantAdminIndexRoster fails unless the stored roster of chatID is exactly want.
func wantAdminIndexRoster(t *testing.T, chatID int64, want map[int64]string) {
	t.Helper()
	got := adminIndexRoster(t, chatID)
	if len(got) != len(want) {
		t.Fatalf("roster of %d = %v, want %v", chatID, got, want)
	}
	for userID, role := range want {
		if got[userID] != role {
			t.Fatalf("roster of %d = %v, want %v", chatID, got, want)
		}
	}
}

// seedAdminIndexRoster writes rows straight through the repository.
func seedAdminIndexRoster(t *testing.T, chatID int64, entries ...adminindex.Entry) {
	t.Helper()
	if err := adminindex.ReplaceChat(context.Background(), chatID, entries); err != nil {
		t.Fatalf("seed roster of %d: %v", chatID, err)
	}
}

func TestAdminIndexFeed(t *testing.T) {
	t.Run("a loaded list stores the creator and the human administrators", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		owner := env.newOwner(env.chat)

		got := cache.LoadAdminCache(env.bot, env.chat.Id)
		if !got.Cached {
			t.Fatalf("LoadAdminCache() = %+v, want a cached list", got)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			owner.Id:     models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
		if _, stored := adminIndexRoster(t, env.chat.Id)[staffTestBotID]; stored {
			t.Fatal("the bot was stored in the roster")
		}
	})

	t.Run("a second load replaces the roster", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		owner := env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)

		extra := gotgbot.User{Id: owner.Id + 1, FirstName: "Extra"}
		env.listAdmin(env.chat.Id, extra.Id)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			owner.Id:     models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
			extra.Id:     models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a deleted-account creator is not stored", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		env.newOwner(env.chat)
		env.fake.setDeletedCreator(env.chat.Id)

		cache.LoadAdminCache(env.bot, env.chat.Id)
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a bot that is not an administrator clears the roster", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		if len(adminIndexRoster(t, env.chat.Id)) != 2 {
			t.Fatal("the first load did not store the roster")
		}

		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
	})

	t.Run("a bot that cannot see the chat clears the roster", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)

		env.fake.setBotRole(env.chat.Id, staffRoleAbsent)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
	})

	t.Run("a group is cleared without touching another group", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		other := env.addChat()
		seedAdminIndexRoster(t, other.Id, adminindex.Entry{UserID: 77, Role: models.AdminIndexRoleCreator})
		env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		cache.LoadAdminCache(env.bot, env.chat.Id)

		wantAdminIndexRoster(t, other.Id, map[int64]string{77: models.AdminIndexRoleCreator})
	})
}

func TestAdminIndexEntries(t *testing.T) {
	deleted := gotgbot.User{Id: 5, FirstName: "Deleted Account"}
	keptDeletedName := gotgbot.User{Id: 6, FirstName: "Deleted Account", Username: "real_owner"}
	members := []gotgbot.MergedChatMember{
		{Status: gotgbot.ChatMemberStatusCreator, User: gotgbot.User{Id: 1, FirstName: "Owner"}},
		{Status: gotgbot.ChatMemberStatusAdministrator, User: gotgbot.User{Id: 2, FirstName: "Admin"}},
		{Status: gotgbot.ChatMemberStatusAdministrator, User: gotgbot.User{Id: 3, IsBot: true, FirstName: "Helper"}},
		{Status: gotgbot.ChatMemberStatusMember, User: gotgbot.User{Id: 4, FirstName: "Plain"}},
		{Status: gotgbot.ChatMemberStatusCreator, User: deleted},
		{Status: gotgbot.ChatMemberStatusCreator, User: keptDeletedName},
	}
	got := map[int64]string{}
	for _, entry := range adminIndexEntries(members) {
		got[entry.UserID] = entry.Role
	}
	want := map[int64]string{
		1: models.AdminIndexRoleCreator,
		2: models.AdminIndexRoleAdministrator,
		6: models.AdminIndexRoleCreator,
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for id, role := range want {
		if got[id] != role {
			t.Fatalf("entries = %v, want %v", got, want)
		}
	}
}

func TestAdminIndexAdmincache(t *testing.T) {
	t.Run("an admincache by an administrator refreshes the roster", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		LoadAdmin(env.dispatcher)
		owner := env.newOwner(env.chat)

		env.send(env.admin, "/admincache")
		if !strings.Contains(env.lastReply(), staffMarker("commonstrings_admin_cache_cache_reloaded")) {
			t.Fatalf("reply = %q, want the reloaded text", env.lastReply())
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			owner.Id:     models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a failing roster write is logged and the reply is unchanged", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		LoadAdmin(env.dispatcher)
		env.newOwner(env.chat)

		t.Cleanup(func() {
			if err := db.DB.AutoMigrate(&models.ChatAdminIndex{}); err != nil {
				t.Errorf("recreate chat_admin_index: %v", err)
			}
		})
		if err := db.DB.Migrator().DropTable(&models.ChatAdminIndex{}); err != nil {
			t.Fatalf("drop chat_admin_index: %v", err)
		}

		if _, err := adminindex.ListChatFresh(context.Background(), env.chat.Id); err == nil {
			t.Fatal("the roster is still readable, so the write could not have failed")
		}
		env.send(env.admin, "/admincache")
		if !strings.Contains(env.lastReply(), staffMarker("commonstrings_admin_cache_cache_reloaded")) {
			t.Fatalf("reply = %q, want the reloaded text despite the failed write", env.lastReply())
		}
		if found, list := cache.GetAdminCacheList(env.chat.Id); !found || len(list.UserInfo) == 0 {
			t.Fatalf("admin cache = (%v, %+v), want the loaded list", found, list)
		}
	})
}
