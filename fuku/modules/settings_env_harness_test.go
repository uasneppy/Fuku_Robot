//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/menuaccess"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// rawAdminList asks the env's fake for getChatAdministrators of chatID and returns the
// exact bytes it answered.
func rawAdminList(t *testing.T, env *settingsEnv, chatID int64) string {
	t.Helper()
	raw, err := env.fake.RequestWithContext(
		context.Background(), "token", "getChatAdministrators", map[string]any{"chat_id": chatID}, nil)
	if err != nil {
		t.Fatalf("getChatAdministrators(%d) error = %v", chatID, err)
	}
	return string(raw)
}

// adminListEntry is one entry of a getChatAdministrators answer, as far as the harness
// tests read it.
type adminListEntry struct {
	Status string `json:"status"`
	User   struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	} `json:"user"`
}

// adminListUsers decodes a getChatAdministrators answer into its entries.
func adminListUsers(t *testing.T, raw string) []adminListEntry {
	t.Helper()
	var entries []adminListEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("admin list %q does not decode: %v", raw, err)
	}
	return entries
}

func TestSettingsEnvSeedsAdminAccess(t *testing.T) {
	t.Run("the env chat and an added chat read the switch as on", func(t *testing.T) {
		env := newSettingsEnv(t)
		second := env.addChat()
		for _, chatID := range []int64{env.chat.Id, second.Id} {
			allowed, err := menuaccess.AdminsAllowedFresh(context.Background(), chatID)
			if err != nil {
				t.Fatalf("AdminsAllowedFresh(%d) error = %v", chatID, err)
			}
			if !allowed {
				t.Errorf("admins_allowed of chat %d = false, want the seeded true", chatID)
			}
		}
		// A chat the env never touched has no row: the seed is not global.
		allowed, err := menuaccess.AdminsAllowedFresh(context.Background(), uniqueModuleChatID())
		if err != nil || allowed {
			t.Errorf("AdminsAllowedFresh(untouched chat) = %t, %v, want false, nil", allowed, err)
		}
	})

	t.Run("the rows are gone once the test ends", func(t *testing.T) {
		var chatIDs []int64
		t.Run("inner", func(t *testing.T) {
			env := newSettingsEnv(t)
			second := env.addChat()
			chatIDs = []int64{env.chat.Id, second.Id}
			for _, chatID := range chatIDs {
				row := &models.ChatAdminIndex{ChatID: chatID, UserID: env.admin.Id, Role: "administrator"}
				if err := db.DB.Create(row).Error; err != nil {
					t.Fatalf("seed roster row: %v", err)
				}
			}
		})
		var access, roster int64
		db.DB.Model(&models.ChatSettingsAccess{}).Where("chat_id IN ?", chatIDs).Count(&access)
		db.DB.Model(&models.ChatAdminIndex{}).Where("chat_id IN ?", chatIDs).Count(&roster)
		if access != 0 || roster != 0 {
			t.Errorf("rows left after cleanup: chat_settings_access=%d chat_admin_index=%d, want 0 and 0", access, roster)
		}
	})

	t.Run("setAdminsAllowed writes a Go false", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.setAdminsAllowed(env.chat.Id, false)
		allowed, err := menuaccess.AdminsAllowedFresh(context.Background(), env.chat.Id)
		if err != nil || allowed {
			t.Errorf("AdminsAllowedFresh after false = %t, %v, want false, nil", allowed, err)
		}
	})

	t.Run("a chat that opts in to nothing keeps today's admin list", func(t *testing.T) {
		env := newSettingsEnv(t)
		want := "[" + string(staffMemberJSON(env.admin.Id, false, "creator", false)) + "," +
			string(staffMemberJSON(staffTestBotID, true, "administrator", true)) + "]"
		if got := rawAdminList(t, env, env.chat.Id); got != want {
			t.Errorf("admin list = %s\nwant      %s", got, want)
		}
	})

	t.Run("listAdministrator lists the user after the creator and before the bot", func(t *testing.T) {
		env := newSettingsEnv(t)
		other := gotgbot.User{Id: uniqueLinkOwnerID() + 3_000_000}
		env.listAdmin(env.chat.Id, other.Id)
		entries := adminListUsers(t, rawAdminList(t, env, env.chat.Id))
		if len(entries) != 3 {
			t.Fatalf("admin list has %d entries, want creator, listed administrator and bot", len(entries))
		}
		if entries[0].Status != "creator" || entries[0].User.ID != env.admin.Id {
			t.Errorf("entry 0 = %+v, want the creator", entries[0])
		}
		if entries[1].Status != "administrator" || entries[1].User.ID != other.Id {
			t.Errorf("entry 1 = %+v, want the listed administrator", entries[1])
		}
		if entries[2].User.ID != staffTestBotID {
			t.Errorf("entry 2 = %+v, want the bot", entries[2])
		}
	})

	t.Run("a deleted creator has the placeholder name and nothing else", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.setDeletedCreator(env.chat.Id)
		entries := adminListUsers(t, rawAdminList(t, env, env.chat.Id))
		creator := entries[0]
		if creator.Status != "creator" || creator.User.ID != env.admin.Id {
			t.Fatalf("entry 0 = %+v, want the creator", creator)
		}
		if creator.User.FirstName != "Deleted Account" || creator.User.LastName != "" || creator.User.Username != "" {
			t.Errorf("creator user = %+v, want first name Deleted Account, no last name, no username", creator.User)
		}
	})

	t.Run("clearCreator removes the creator entry and the live creator status", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.clearCreator(env.chat.Id)
		for _, entry := range adminListUsers(t, rawAdminList(t, env, env.chat.Id)) {
			if entry.Status == "creator" {
				t.Errorf("admin list still has a creator entry: %+v", entry)
			}
		}
		raw, err := env.fake.RequestWithContext(context.Background(), "token", "getChatMember",
			map[string]any{"chat_id": env.chat.Id, "user_id": env.admin.Id + 1}, nil)
		if err != nil {
			t.Fatalf("getChatMember error = %v", err)
		}
		if strings.Contains(string(raw), `"creator"`) {
			t.Errorf("getChatMember = %s, want no creator", raw)
		}
	})

	t.Run("newOwner is the creator and the admin is listed", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		entries := adminListUsers(t, rawAdminList(t, env, env.chat.Id))
		if len(entries) < 2 || entries[0].Status != "creator" || entries[0].User.ID != owner.Id {
			t.Fatalf("admin list = %+v, want the owner first as creator", entries)
		}
		if entries[1].User.ID != env.admin.Id || entries[1].Status != "administrator" {
			t.Errorf("entry 1 = %+v, want the default admin listed", entries[1])
		}
		raw, err := env.fake.RequestWithContext(context.Background(), "token", "getChatMember",
			map[string]any{"chat_id": env.chat.Id, "user_id": owner.Id}, nil)
		if err != nil || !strings.Contains(string(raw), `"creator"`) {
			t.Errorf("getChatMember(owner) = %s, %v, want the creator status", raw, err)
		}
	})

	t.Run("otherReplica answers on its own fake with its own update IDs", func(t *testing.T) {
		env := newSettingsEnv(t)
		replica := env.otherReplica()

		if replica.fake == env.fake || replica.bot == env.bot || replica.dispatcher == env.dispatcher {
			t.Fatal("the replica shares its fake, bot or dispatcher with the base environment")
		}
		if replica.redis != env.redis || replica.chat.Id != env.chat.Id || replica.admin.Id != env.admin.Id {
			t.Fatal("the replica must share the Redis server, the chat and the admin")
		}
		if base, other := env.updateID(), replica.updateID(); other <= base {
			t.Errorf("replica update ID %d does not start above the base's %d", other, base)
		}

		replica.send(replica.admin, "/settings")
		if sent := replica.fake.sentTo(replica.chat.Id); len(sent) == 0 {
			t.Fatal("the replica's dispatcher sent no choice message for /settings")
		}
		if _, button := replica.newestChoice(); button.CallbackData == "" {
			t.Error("the replica's choice message has no Open here button")
		}
		if sent := env.fake.sentTo(env.chat.Id); len(sent) != 0 {
			t.Errorf("the base fake received %d messages from the replica's /settings", len(sent))
		}
	})
}
