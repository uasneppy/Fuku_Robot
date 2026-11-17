//go:build testtools

package modules

import (
	"slices"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/disabling"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// disableNoopsEnv is a dispatcher with the production registrations of the backup, retired raid,
// blacklist and disabling modules, over a cleared disableable list, and a fresh group.
type disableNoopsEnv struct {
	t          *testing.T
	client     *moduleBotClient
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	chat       gotgbot.Chat
	nextUpdate int64
}

func newDisableNoopsEnv(t *testing.T) *disableNoopsEnv {
	t.Helper()
	helpers.SetDisableCmdsForTest(t, nil)
	client := newModuleBotClient()
	env := &disableNoopsEnv{
		t:          t,
		client:     client,
		bot:        newModuleTestBot(client),
		dispatcher: ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1}),
		chat:       gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Disable Chat"},
		nextUpdate: 7000,
	}
	LoadBackup(env.dispatcher)
	LoadAntiRaid(env.dispatcher)
	LoadBlacklists(env.dispatcher)
	LoadDisabling(env.dispatcher)
	chatID := env.chat.Id
	t.Cleanup(func() {
		db.DB.Where("chat_id = ?", chatID).Delete(&models.DisableSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.BlacklistSettings{})
		db.DB.Where("chat_id = ?", chatID).Delete(&models.Chat{})
		dbcache.DeleteCache(dbcache.CacheKey("disabled_cmds", chatID))
		dbcache.DeleteCache(dbcache.CacheKey("blacklist", chatID))
		dbcache.ResetLocalForTest()
	})
	return env
}

// send runs a message through the dispatcher and returns how many messages the bot sent for it.
func (e *disableNoopsEnv) send(from gotgbot.User, text string) int {
	e.t.Helper()
	before := len(e.client.callsFor("sendMessage"))
	e.nextUpdate++
	update := &gotgbot.Update{UpdateId: e.nextUpdate, Message: &gotgbot.Message{
		MessageId: e.nextUpdate, Date: 1, Chat: e.chat, From: &from, Text: text,
	}}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%q) error = %v", text, err)
	}
	return len(e.client.callsFor("sendMessage")) - before
}

// TestDisableRefusesNamesThatHaveNoCheck proves /disable only offers commands whose handler checks
// the name: the backup commands and the retired raid pointer are refused as unknown and nothing is
// stored for them, while blocklists, which now honours its own name, is stored.
func TestDisableRefusesNamesThatHaveNoCheck(t *testing.T) {
	env := newDisableNoopsEnv(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}

	replies := env.send(admin, "/disable export import antiraid raidtime raidactiontime blocklists")
	if replies != 6 {
		t.Fatalf("/disable sent %d replies, want one success and five unknown-command replies", replies)
	}
	if got := disabling.GetChatDisabledCMDs(env.chat.Id); !slices.Equal(got, []string{"blocklists"}) {
		t.Fatalf("disabled commands = %v, want only blocklists", got)
	}
}

// TestBlocklistsAliasHonoursItsOwnName proves /disable blocklists silences the alias for a member
// while /blacklists still answers, and /disable blacklists still silences both, an admin never
// being blocked.
func TestBlocklistsAliasHonoursItsOwnName(t *testing.T) {
	env := newDisableNoopsEnv(t)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	if err := blacklists.AddBlacklist(env.chat.Id, "spamword"); err != nil {
		t.Fatalf("AddBlacklist() error = %v", err)
	}

	// The positive control: with nothing disabled a member gets the list under both names.
	for _, cmd := range []string{"/blocklists", "/blacklists"} {
		if n := env.send(member, cmd); n != 1 {
			t.Fatalf("%s with nothing disabled sent %d replies, want the list", cmd, n)
		}
	}

	for _, tc := range []struct {
		disabled string
		silent   map[string]bool
	}{
		{disabled: "blocklists", silent: map[string]bool{"/blocklists": true, "/blacklists": false}},
		{disabled: "blacklists", silent: map[string]bool{"/blocklists": true, "/blacklists": true}},
	} {
		if err := disabling.DisableCMD(env.chat.Id, tc.disabled); err != nil {
			t.Fatalf("DisableCMD(%s) error = %v", tc.disabled, err)
		}
		for cmd, silent := range tc.silent {
			want := 1
			if silent {
				want = 0
			}
			if n := env.send(member, cmd); n != want {
				t.Errorf("%s disabled: a member's %s sent %d replies, want %d", tc.disabled, cmd, n, want)
			}
			if n := env.send(admin, cmd); n != 1 {
				t.Errorf("%s disabled: an admin's %s sent %d replies, want the list", tc.disabled, cmd, n)
			}
		}
		if err := disabling.EnableCMD(env.chat.Id, tc.disabled); err != nil {
			t.Fatalf("EnableCMD(%s) error = %v", tc.disabled, err)
		}
		if n := env.send(member, "/blocklists"); n != 1 {
			t.Fatalf("%s enabled again: a member's /blocklists sent %d replies, want the list", tc.disabled, n)
		}
	}
}
