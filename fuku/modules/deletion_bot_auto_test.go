//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/aispam"
	"github.com/uasneppy/fuku_robot/fuku/db/antiflood"
	"github.com/uasneppy/fuku_robot/fuku/db/blacklists"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// autoNoticeSite is one named site that sends an automatic punishment notice to the group: run
// drives it once on the site's chat.
type autoNoticeSite struct {
	name string
	run  func(t *testing.T, s *botSite)
	// helperSite marks the sites whose send goes through helpers.SendMessageWithErrorHandling,
	// which answers no message at all for a chat the bot cannot post in.
	helperSite bool
}

// autoFloodSite drives the antiflood action: the member's second message passes the limit of one.
func autoFloodSite(action string) autoNoticeSite {
	return autoNoticeSite{name: "antiflood " + action, helperSite: true, run: func(t *testing.T, s *botSite) {
		t.Helper()
		resetAntifloodState(t)
		t.Cleanup(func() { db.DB.Where("chat_id = ?", s.chat.Id).Delete(&models.AntifloodSettings{}) })
		if err := antiflood.SetFlood(s.chat.Id, 1); err != nil {
			t.Fatalf("SetFlood() error = %v", err)
		}
		if err := antiflood.SetFloodMode(s.chat.Id, action); err != nil {
			t.Fatalf("SetFloodMode() error = %v", err)
		}
		member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Flooder"}
		first := newModuleMessageContext(s.bot, s.chat, member, "one")
		_ = antifloodModule.checkFlood(s.bot, first)
		second := newModuleMessageContext(s.bot, s.chat, member, "two")
		second.EffectiveMessage.MessageId = 102
		_ = antifloodModule.checkFlood(s.bot, second)
	}}
}

// autoBlacklistSite drives the blacklist watcher with the given action on a matching message.
func autoBlacklistSite(action string) autoNoticeSite {
	return autoNoticeSite{name: "blacklist " + action, run: func(t *testing.T, s *botSite) {
		t.Helper()
		member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
		memberReqCleanup(t, []int64{s.chat.Id}, []int64{member.Id})
		botSiteWarnsCleanup(t, s.chat.Id)
		t.Cleanup(func() {
			if err := blacklists.RemoveAllBlacklist(s.chat.Id); err != nil {
				t.Errorf("RemoveAllBlacklist error = %v", err)
			}
		})
		if err := blacklists.AddBlacklist(s.chat.Id, "spam"); err != nil {
			t.Fatalf("AddBlacklist error = %v", err)
		}
		if err := blacklists.SetBlacklistAction(s.chat.Id, action); err != nil {
			t.Fatalf("SetBlacklistAction error = %v", err)
		}
		ctx := newModuleMessageContext(s.bot, s.chat, member, "this has spam inside")
		ctx.EffectiveMessage.Date = s.now.Unix()
		_ = blacklistsModule.blacklistWatcher(s.bot, ctx)
	}}
}

// autoBreakerSite trips the AI spam breaker with failing provider answers, which posts the pause
// notice.
func autoBreakerSite() autoNoticeSite {
	return autoNoticeSite{name: "aispam breaker pause", helperSite: true, run: func(t *testing.T, s *botSite) {
		t.Helper()
		withRealLocales(t)
		resetAISpamRuntime()
		withAISpamTestConfig(t, "typesafe-test-key", true, 0)
		withAISpamJevServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusInternalServerError)
		})
		t.Cleanup(func() {
			if err := aispam.SetAISpamEnabled(s.chat.Id, false); err != nil {
				t.Errorf("SetAISpamEnabled(false) error = %v", err)
			}
		})
		if err := aispam.SetAISpamEnabled(s.chat.Id, true); err != nil {
			t.Fatalf("SetAISpamEnabled() error = %v", err)
		}
		dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
		LoadAISpam(dispatcher)
		user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}
		for i := range aispamBreakerFailures {
			aispamProcessUpdate(t, dispatcher, s.bot, s.chat, user, int64(700+i), fmt.Sprintf("BUY CHEAP COINS %d", i))
		}
		DrainAISpamChecks()
	}}
}

func TestBotAutoPunishmentNotices(t *testing.T) {
	const delay = 600
	sites := []autoNoticeSite{
		autoFloodSite("mute"), autoFloodSite("ban"), autoFloodSite("kick"),
		autoBlacklistSite("mute"), autoBlacklistSite("ban"), autoBlacklistSite("kick"), autoBlacklistSite("warn"),
		autoBreakerSite(),
	}

	for _, site := range sites {
		t.Run(site.name+" notice is queued by the automatic-punishments timer", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			s.setRule(s.chat.Id, deletion.KindBotAuto, delay)

			site.run(t, s)

			s.wantRow(s.chat.Id, 9001, deletion.KindBotAuto, delay*time.Second)
		})

		t.Run(site.name+" writes nothing with the kind off", func(t *testing.T) {
			s := newBotSite(t, "supergroup")

			site.run(t, s)

			if len(s.client.callsFor("sendMessage")) == 0 {
				t.Fatal("the site sent no notice, so the case proves nothing")
			}
			s.wantNoRows(s.chat.Id)
		})

		t.Run(site.name+" ignores the manual-punishments timer", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			s.setRule(s.chat.Id, deletion.KindBotManual, delay)

			site.run(t, s)

			if len(s.client.callsFor("sendMessage")) == 0 {
				t.Fatal("the site sent no notice, so the case proves nothing")
			}
			s.wantNoRows(s.chat.Id)
		})

		for _, chatType := range []string{"private", "channel"} {
			t.Run(site.name+" never schedules a message Telegram answers from a "+chatType, func(t *testing.T) {
				s := newBotSite(t, "supergroup")
				s.setRule(s.chat.Id, deletion.KindBotAuto, delay)
				s.answerAs(gotgbot.Chat{Id: s.chat.Id, Type: chatType})

				site.run(t, s)

				if len(s.client.callsFor("sendMessage")) == 0 {
					t.Fatal("the site sent no notice, so the case proves nothing")
				}
				s.wantNoRows(s.chat.Id)
			})
		}

		t.Run(site.name+" writes nothing when the send fails", func(t *testing.T) {
			s := newBotSite(t, "supergroup")
			s.setRule(s.chat.Id, deletion.KindBotAuto, delay)
			s.client.errors["sendMessage"] = errors.New("send failed")

			site.run(t, s)

			if len(s.client.callsFor("sendMessage")) == 0 {
				t.Fatal("the site sent no notice, so the case proves nothing")
			}
			s.wantNoRows(s.chat.Id)
		})

		if site.helperSite {
			t.Run(site.name+" in a chat the helper cannot post in writes nothing and does not panic", func(t *testing.T) {
				s := newBotSite(t, "supergroup")
				s.setRule(s.chat.Id, deletion.KindBotAuto, delay)
				s.client.errors["sendMessage"] = errors.New("Bad Request: CHAT_WRITE_FORBIDDEN")

				site.run(t, s)

				if len(s.client.callsFor("sendMessage")) == 0 {
					t.Fatal("the site sent no notice, so the case proves nothing")
				}
				s.wantNoRows(s.chat.Id)
			})
		}
	}

	// A delete verdict is silent in the group, and the owner-log dump is a post to the owner's
	// own chat, never a kind: neither writes a row, even though every send the fake answers here
	// carries the group's ID and a group type, which is what a wrongly hooked dump would schedule.
	t.Run("a delete verdict and the owner-log dump write nothing", func(t *testing.T) {
		const dumpChatID = -100998
		s := newBotSite(t, "supergroup")
		s.setRule(s.chat.Id, deletion.KindBotAuto, delay)
		s.setRule(s.chat.Id, deletion.KindBotManual, delay)
		resetAISpamRuntime()
		withAISpamTestConfig(t, "typesafe-test-key", true, dumpChatID)
		var requests atomic.Int64
		withAISpamJevServer(t, func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, _ = fmt.Fprint(w, aispamDecisionBody(0.95, "scam"))
		})
		t.Cleanup(func() {
			if err := aispam.SetAISpamEnabled(s.chat.Id, false); err != nil {
				t.Errorf("SetAISpamEnabled(false) error = %v", err)
			}
		})
		if err := aispam.SetAISpamEnabled(s.chat.Id, true); err != nil {
			t.Fatalf("SetAISpamEnabled() error = %v", err)
		}
		dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{MaxRoutines: -1})
		LoadAISpam(dispatcher)

		user := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Spammer"}
		aispamProcessUpdate(t, dispatcher, s.bot, s.chat, user, 501, "BUY CHEAP COINS NOW t.me/scam")
		DrainAISpamChecks()

		if requests.Load() != 1 {
			t.Fatalf("provider requests = %d, want 1", requests.Load())
		}
		if got := aispamSendMessageCount(s.client.callsFor("sendMessage"), dumpChatID); got != 1 {
			t.Fatalf("owner-log dump posts = %d, want 1 (the case must reach the dump)", got)
		}
		s.wantNoRows(s.chat.Id)
		s.wantNoRows(dumpChatID)
	})
}
