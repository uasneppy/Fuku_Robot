//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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

	t.Run("an admincache with a bot that is not an administrator loads nothing", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		LoadAdmin(env.dispatcher)
		owner := env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		cache.InvalidateAdminCache(env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		listCalls := len(env.calls("getChatAdministrators"))

		env.send(env.admin, "/admincache")
		if got := len(env.calls("getChatAdministrators")); got != listCalls {
			t.Fatalf("getChatAdministrators calls = %d, want %d: nothing is loaded", got, listCalls)
		}
		// The rows are left to the demotion update or the sweep.
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

func TestAdminIndexKeepsRowsOnFailure(t *testing.T) {
	env := newSettingsEnv(t)
	withAdminIndexObserver(t)
	owner := env.newOwner(env.chat)
	cache.LoadAdminCache(env.bot, env.chat.Id)
	want := map[int64]string{
		owner.Id:     models.AdminIndexRoleCreator,
		env.admin.Id: models.AdminIndexRoleAdministrator,
	}
	wantAdminIndexRoster(t, env.chat.Id, want)

	// Every retry of the list call fails (the loader sleeps 3 s over its retries): the roster
	// must stay exactly as it was.
	env.fake.setFailure("getChatAdministrators", env.chat.Id,
		&gotgbot.TelegramError{Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error"})
	cache.InvalidateAdminCache(env.chat.Id)
	if got := cache.LoadAdminCache(env.bot, env.chat.Id); got.Cached {
		t.Fatalf("LoadAdminCache() = %+v, want the empty uncached result after failed retries", got)
	}
	wantAdminIndexRoster(t, env.chat.Id, want)

	// A bot lookup that fails with a server error says nothing either.
	env.fake.setFailure("getChatAdministrators", env.chat.Id, nil)
	env.fake.setFailure("getChatMember", env.chat.Id,
		&gotgbot.TelegramError{Method: "getChatMember", Code: 502, Description: "Bad Gateway"})
	cache.InvalidateAdminCache(env.chat.Id)
	cache.LoadAdminCache(env.bot, env.chat.Id)
	wantAdminIndexRoster(t, env.chat.Id, want)
}

// botStatusUpdate delivers a my_chat_member update in the env's chat through the real
// dispatcher: the bot went from administrator to next.
func (e *settingsEnv) botStatusUpdate(next gotgbot.ChatMember) {
	e.t.Helper()
	id := e.updateID()
	update := &gotgbot.Update{
		UpdateId: id,
		MyChatMember: &gotgbot.ChatMemberUpdated{
			Chat:          e.chat,
			From:          e.admin,
			Date:          1,
			OldChatMember: gotgbot.ChatMemberAdministrator{User: botUser, CanRestrictMembers: true},
			NewChatMember: next,
		},
	}
	if err := e.dispatcher.ProcessUpdate(e.bot, update, nil); err != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, err)
	}
}

func TestAdminIndexBotRemoval(t *testing.T) {
	for name, next := range map[string]gotgbot.ChatMember{
		"left":   gotgbot.ChatMemberLeft{User: botUser},
		"kicked": gotgbot.ChatMemberBanned{User: botUser},
	} {
		t.Run("the bot is "+name, func(t *testing.T) {
			env := newSettingsEnv(t)
			withAdminIndexObserver(t)
			env.newOwner(env.chat)
			cache.LoadAdminCache(env.bot, env.chat.Id)
			if len(adminIndexRoster(t, env.chat.Id)) != 2 {
				t.Fatal("the first load did not store the roster")
			}
			// The reload that follows must say nothing, so the clear is the handler's own.
			env.fake.setFailure("getChatMember", env.chat.Id,
				&gotgbot.TelegramError{Method: "getChatMember", Code: 502, Description: "Bad Gateway"})

			env.botStatusUpdate(next)
			wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
		})
	}

	t.Run("the bot is demoted to member", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		env.fake.setBotRole(env.chat.Id, staffRoleMember)

		env.botStatusUpdate(gotgbot.ChatMemberMember{User: botUser})
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
	})

	t.Run("another group keeps its rows", func(t *testing.T) {
		env := newSettingsEnv(t)
		withAdminIndexObserver(t)
		other := env.addChat()
		seedAdminIndexRoster(t, other.Id, adminindex.Entry{UserID: 77, Role: models.AdminIndexRoleCreator})
		env.newOwner(env.chat)
		cache.LoadAdminCache(env.bot, env.chat.Id)
		env.fake.setFailure("getChatMember", env.chat.Id,
			&gotgbot.TelegramError{Method: "getChatMember", Code: 502, Description: "Bad Gateway"})

		env.botStatusUpdate(gotgbot.ChatMemberLeft{User: botUser})
		wantAdminIndexRoster(t, other.Id, map[int64]string{77: models.AdminIndexRoleCreator})
	})
}

// unlistAdministrator removes userID from the fake's administrator answer for chatID.
func unlistAdministrator(env *settingsEnv, chatID, userID int64) {
	env.fake.smu.Lock()
	defer env.fake.smu.Unlock()
	kept := env.fake.listed[chatID][:0]
	for _, id := range env.fake.listed[chatID] {
		if id != userID {
			kept = append(kept, id)
		}
	}
	env.fake.listed[chatID] = kept
}

func TestAdminIndexPromotion(t *testing.T) {
	env := newSettingsEnv(t)
	withAdminIndexObserver(t)
	owner := env.newOwner(env.chat)
	cache.LoadAdminCache(env.bot, env.chat.Id)

	promoted := gotgbot.User{Id: owner.Id + 100, FirstName: "Promoted"}
	env.listAdmin(env.chat.Id, promoted.Id)
	env.memberUpdate(env.admin,
		gotgbot.ChatMemberMember{User: promoted},
		gotgbot.ChatMemberAdministrator{User: promoted, CanRestrictMembers: true}, false)
	wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
		owner.Id:     models.AdminIndexRoleCreator,
		env.admin.Id: models.AdminIndexRoleAdministrator,
		promoted.Id:  models.AdminIndexRoleAdministrator,
	})

	unlistAdministrator(env, env.chat.Id, promoted.Id)
	env.memberUpdate(env.admin,
		gotgbot.ChatMemberAdministrator{User: promoted, CanRestrictMembers: true},
		gotgbot.ChatMemberMember{User: promoted}, false)
	wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
		owner.Id:     models.AdminIndexRoleCreator,
		env.admin.Id: models.AdminIndexRoleAdministrator,
	})
}

// adminIndexProbe wraps the env's fake: it counts the requests it passes on, can answer an
// empty administrator list and can hang a method until its context ends.
type adminIndexProbe struct {
	gotgbot.BotClient

	mu         sync.Mutex
	calls      []string
	emptyList  bool
	hangMethod string
}

func (p *adminIndexProbe) RequestWithContext(ctx context.Context, token, method string, params map[string]any, opts *gotgbot.RequestOpts) (json.RawMessage, error) {
	p.mu.Lock()
	p.calls = append(p.calls, method)
	empty, hang := p.emptyList, p.hangMethod
	p.mu.Unlock()
	if hang == method {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if empty && method == "getChatAdministrators" {
		return json.RawMessage(`[]`), nil
	}
	return p.BotClient.RequestWithContext(ctx, token, method, params, opts)
}

func (p *adminIndexProbe) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// probeBot returns a copy of the env's bot that goes through a probe over the env's fake.
func (e *settingsEnv) probeBot() (*gotgbot.Bot, *adminIndexProbe) {
	probe := &adminIndexProbe{BotClient: e.fake}
	bot := *e.bot
	bot.BotClient = probe
	return &bot, probe
}

func TestAdminIndexRefresh(t *testing.T) {
	t.Run("an administrator bot with a list replaces the rows in two calls", func(t *testing.T) {
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleAdministrator})
		bot, probe := env.probeBot()

		outcome, err := refreshChatAdminIndex(context.Background(), bot, env.chat.Id)
		if err != nil || outcome != adminIndexReplaced {
			t.Fatalf("refreshChatAdminIndex() = (%v, %v), want replaced", outcome, err)
		}
		if probe.callCount() > 2 {
			t.Fatalf("calls = %v, want at most two", probe.calls)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			owner.Id:     models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a bot that is a member clears the rows", func(t *testing.T) {
		env := newSettingsEnv(t)
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})
		env.fake.setBotRole(env.chat.Id, staffRoleMember)
		bot, probe := env.probeBot()

		outcome, err := refreshChatAdminIndex(context.Background(), bot, env.chat.Id)
		if err != nil || outcome != adminIndexCleared {
			t.Fatalf("refreshChatAdminIndex() = (%v, %v), want cleared", outcome, err)
		}
		if probe.callCount() != 1 {
			t.Fatalf("calls = %v, want only the bot lookup", probe.calls)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
	})

	t.Run("a refused bot lookup clears the rows and returns the error", func(t *testing.T) {
		env := newSettingsEnv(t)
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})
		env.fake.setFailure("getChatMember", env.chat.Id,
			&gotgbot.TelegramError{Method: "getChatMember", Code: 403, Description: "Forbidden: bot was kicked"})
		bot, _ := env.probeBot()

		outcome, err := refreshChatAdminIndex(context.Background(), bot, env.chat.Id)
		if err == nil || outcome != adminIndexCleared {
			t.Fatalf("refreshChatAdminIndex() = (%v, %v), want cleared with the error", outcome, err)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{})
	})

	t.Run("an ambiguous answer leaves the rows", func(t *testing.T) {
		keep := adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator}
		cases := map[string]func(env *settingsEnv, probe *adminIndexProbe){
			"bot lookup 500": func(env *settingsEnv, _ *adminIndexProbe) {
				env.fake.setFailure("getChatMember", env.chat.Id,
					&gotgbot.TelegramError{Method: "getChatMember", Code: 500, Description: "Internal Server Error"})
			},
			"bot lookup 429": func(env *settingsEnv, _ *adminIndexProbe) {
				env.fake.setFailure("getChatMember", env.chat.Id, &gotgbot.TelegramError{
					Method: "getChatMember", Code: 429, Description: "Too Many Requests",
					ResponseParams: &gotgbot.ResponseParameters{RetryAfter: 17},
				})
			},
			"bot lookup timeout": func(_ *settingsEnv, probe *adminIndexProbe) {
				probe.hangMethod = "getChatMember"
			},
			"list 500": func(env *settingsEnv, _ *adminIndexProbe) {
				env.fake.setFailure("getChatAdministrators", env.chat.Id,
					&gotgbot.TelegramError{Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error"})
			},
			"empty list": func(_ *settingsEnv, probe *adminIndexProbe) {
				probe.emptyList = true
			},
		}
		for name, arrange := range cases {
			t.Run(name, func(t *testing.T) {
				env := newSettingsEnv(t)
				env.newOwner(env.chat)
				seedAdminIndexRoster(t, env.chat.Id, keep)
				bot, probe := env.probeBot()
				arrange(env, probe)

				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				outcome, err := refreshChatAdminIndex(ctx, bot, env.chat.Id)
				if outcome != adminIndexUnchanged {
					t.Fatalf("outcome = %v, want unchanged", outcome)
				}
				if name != "empty list" && err == nil {
					t.Fatal("err = nil, want the error returned")
				}
				if probe.callCount() > 2 {
					t.Fatalf("calls = %v, want at most two", probe.calls)
				}
				wantAdminIndexRoster(t, env.chat.Id, map[int64]string{55: models.AdminIndexRoleCreator})
			})
		}
	})

	t.Run("a rate limit keeps its retry_after", func(t *testing.T) {
		env := newSettingsEnv(t)
		env.fake.setFailure("getChatMember", env.chat.Id, &gotgbot.TelegramError{
			Method: "getChatMember", Code: 429, Description: "Too Many Requests",
			ResponseParams: &gotgbot.ResponseParameters{RetryAfter: 17},
		})
		bot, _ := env.probeBot()

		_, err := refreshChatAdminIndex(context.Background(), bot, env.chat.Id)
		var tgErr *gotgbot.TelegramError
		if !errors.As(err, &tgErr) || tgErr.ResponseParams == nil || tgErr.ResponseParams.RetryAfter != 17 {
			t.Fatalf("err = %v, want the 429 with retry_after 17", err)
		}
	})
}

func TestAdminIndexSettingsCommandRefresh(t *testing.T) {
	t.Run("the owner's /settings repairs the group's rows after the choice message", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleAdministrator})

		env.send(owner, "/settings")

		if _, button := env.newestChoice(); button.CallbackData == "" {
			t.Fatal("the owner got no choice message")
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{
			owner.Id:     models.AdminIndexRoleCreator,
			env.admin.Id: models.AdminIndexRoleAdministrator,
		})
	})

	t.Run("a failing administrator list changes no row and the reply is the same", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		owner := env.newOwner(env.chat)

		env.send(owner, "/settings")
		baseline := env.replies()
		if len(baseline) != 1 {
			t.Fatalf("baseline replies = %d, want 1", len(baseline))
		}

		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})
		env.fake.setFailure("getChatAdministrators", env.chat.Id,
			&gotgbot.TelegramError{Method: "getChatAdministrators", Code: 500, Description: "Internal Server Error"})
		env.send(owner, "/settings")

		replies := env.replies()
		if len(replies) != 2 {
			t.Fatalf("replies = %d, want the choice message sent again despite the failed refresh", len(replies))
		}
		first, second := baseline[0].Params, replies[1].Params
		if first["text"] != second["text"] || !reflect.DeepEqual(
			staffKeyboardOf(first["reply_markup"]), staffKeyboardOf(second["reply_markup"])) {
			t.Fatalf("the choice message changed: %v vs %v", first, second)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{55: models.AdminIndexRoleCreator})
	})

	t.Run("a gated administrator triggers no refresh and changes no row", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		env.setAdminsAllowed(env.chat.Id, false)
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})

		env.send(env.admin, "/settings")

		env.wantReplyHas(staffMarker("settings_refuse_owner_only"))
		// The gate's own owner-state read is the only list call; the repair path adds none.
		if got := settingsAdminListCalls(env, env.chat.Id); got > 1 {
			t.Errorf("getChatAdministrators was called %d times, want at most the gate's one", got)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{55: models.AdminIndexRoleCreator})
	})

	t.Run("a plain member triggers no refresh", func(t *testing.T) {
		resetSettingsOwnerMemo(t)
		env := newSettingsEnv(t)
		env.newOwner(env.chat)
		member := gotgbot.User{Id: env.admin.Id + 4242, FirstName: "Member"}
		seedAdminIndexRoster(t, env.chat.Id, adminindex.Entry{UserID: 55, Role: models.AdminIndexRoleCreator})

		env.send(member, "/settings")

		if got := settingsAdminListCalls(env, env.chat.Id); got != 0 {
			t.Errorf("getChatAdministrators was called %d times for a plain member, want 0", got)
		}
		wantAdminIndexRoster(t, env.chat.Id, map[int64]string{55: models.AdminIndexRoleCreator})
	})
}
