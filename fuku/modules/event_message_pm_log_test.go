//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// The log line of an undelivered private copy (plan 22-07, PMSG-07, D-21): one #PM_COPY post in the group's log
// channel under its admin category, and nothing else.

// eventPMLogLink links a fresh log channel to the site's group with every category on, removes the row when the test
// ends and returns the channel's ID.
func eventPMLogLink(t *testing.T, s *botSite) int64 {
	t.Helper()
	channelID := uniqueModuleChatID()
	if err := logchannels.Set(s.chat.Id, s.chat.Title, channelID); err != nil {
		t.Fatalf("logchannels.Set error = %v", err)
	}
	t.Cleanup(func() { db.DB.Where("chat_id = ?", s.chat.Id).Delete(&models.LogChannel{}) })
	return channelID
}

// eventPMLogOn switches the chat's log option on through the repository.
func eventPMLogOn(t *testing.T, chatID int64) {
	t.Helper()
	if err := eventmsg.SetPMLog(t.Context(), chatID, true, 1); err != nil {
		t.Fatalf("SetPMLog(%d) error = %v", chatID, err)
	}
}

// eventPMLogPosts returns the #PM_COPY posts the site made to channelID (to any chat when channelID is 0), after the
// jobs, decoded through JSON. The ban's own #BAN post to the same channel is not one of them.
func eventPMLogPosts(t *testing.T, s *botSite, channelID int64) []map[string]any {
	t.Helper()
	waitEventPM(t)
	var posts []map[string]any
	for _, call := range eventPMAllCalls(s) {
		if call.Method != "sendMessage" || (channelID != 0 && fmt.Sprint(call.Params["chat_id"]) != strconv.FormatInt(channelID, 10)) {
			continue
		}
		if params := rulesCallParams(t, call); strings.Contains(fmt.Sprint(params["text"]), pmCopyLogTag) {
			posts = append(posts, params)
		}
	}
	return posts
}

// eventPMLogWant fails unless the site made exactly one #PM_COPY post to channelID, as HTML with link previews off and
// no reply, holding the group's header and the line in the group's language (uk) naming the member, the ban and the
// reason whyKey. It returns the post's text.
func eventPMLogWant(t *testing.T, s *botSite, channelID int64, whyKey string) string {
	t.Helper()
	posts := eventPMLogPosts(t, s, channelID)
	if len(posts) != 1 {
		t.Fatalf("#PM_COPY posts to the log channel = %d, want one", len(posts))
	}
	post := posts[0]
	if post["parse_mode"] != formatting.HTML || post["reply_parameters"] != nil {
		t.Fatalf("the log post = %v, want HTML and no reply", post)
	}
	if preview, _ := post["link_preview_options"].(map[string]any); preview["is_disabled"] != true {
		t.Fatalf("the log post's link preview options = %v, want previews off", post["link_preview_options"])
	}
	tr := i18n.MustNewTranslator("uk")
	why, _ := tr.GetString(whyKey)
	event, _ := tr.GetString("settings_msg_ev_ban")
	user := formatting.MentionHtml(eventPMMember, moderationGoldenTarget.FirstName) + " (<code>42</code>)"
	sentence, _ := tr.GetString("eventmsg_pm_log", i18n.TranslationParams{"user": user, "event": event, "why": why})
	header := fmt.Sprintf("<b>%s</b> (<code>%d</code>)\n", s.chat.Title, s.chat.Id)
	text := fmt.Sprint(post["text"])
	if want := header + pmCopyLogTag + "\n" + sentence; text != want || why == "" || sentence == "" {
		t.Fatalf("the log post = %q, want %q", text, want)
	}
	return text
}

func TestEventPMLogLine(t *testing.T) {
	withModerationGoldenLocales(t)

	t.Run("blocked posts one line", func(t *testing.T) {
		s := newEventPMSite(t)
		channel := eventPMLogLink(t, s)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMLogOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: bot was blocked by the user", 0))
		eventPMBanCommand(t, s, "spam <b>")
		eventPMWantBan(t, s)
		eventPMLogWant(t, s, channel, "eventmsg_pm_why_blocked")
		eventPMWantGroupGolden(t, s, goldenModerationBanReason)
		eventPMDetailsNothing(t, s)
	})

	t.Run("failed posts the other reason", func(t *testing.T) {
		s := newEventPMSite(t)
		channel := eventPMLogLink(t, s)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMLogOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(502, "Bad Gateway", 0))
		eventPMBanCommand(t, s, "spam")
		eventPMLogWant(t, s, channel, "eventmsg_pm_why_failed")
	})

	// The cases that post nothing: each prepares the site (with a linked log channel), then /ban runs.
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, s *botSite)
	}{
		{"option off", func(t *testing.T, s *botSite) {
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"delivered", func(t *testing.T, s *botSite) {
			eventPMLogOn(t, s.chat.Id)
		}},
		{"admin category off", func(t *testing.T, s *botSite) {
			eventPMLogOn(t, s.chat.Id)
			if err := logchannels.SetCategory(s.chat.Id, logchannels.CategoryAdmin, false); err != nil {
				t.Fatalf("SetCategory error = %v", err)
			}
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
		{"no log channel", func(t *testing.T, s *botSite) {
			eventPMLogOn(t, s.chat.Id)
			if err := logchannels.Unset(s.chat.Id); err != nil {
				t.Fatalf("Unset error = %v", err)
			}
			eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newEventPMSite(t)
			eventPMLogLink(t, s)
			storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
			tc.setup(t, s)
			eventPMBanCommand(t, s, "spam")
			eventPMWantBan(t, s)
			if _, private := eventPMCalls(t, s); len(private) == 0 {
				t.Fatal("no private copy was tried, so the case proves nothing")
			}
			if posts := eventPMLogPosts(t, s, 0); len(posts) != 0 {
				t.Fatalf("#PM_COPY posts = %v, want none", posts)
			}
		})
	}

	t.Run("capped", func(t *testing.T) {
		s := newEventPMSite(t)
		eventPMLogLink(t, s)
		withTestEventPM(t, "pmtest_log_auto", eventPMSpec{DefaultKey: "eventmsg_pm_ban", Automatic: true})
		mreqSwap(t, &pmCopyAutoCapPerMinute, 0)
		eventPMWaitBeforeRestore(t)
		offerEventPM(eventPMOffer{
			Bot: s.bot, Chat: s.chat, Event: "pmtest_log_auto", UserID: eventPMMember,
			Message: media.GreetingMessage{Text: "copy", MsgType: db.TEXT},
			Options: eventmsg.Options{PMLog: true}, TargetName: "Ann", Tr: i18n.MustNewTranslator("uk"),
		})
		eventPMNone(t, s)
		if posts := eventPMLogPosts(t, s, 0); len(posts) != 0 {
			t.Fatalf("a capped copy posted %v, want nothing", posts)
		}
	})

	t.Run("both options", func(t *testing.T) {
		s := newEventPMSite(t)
		channel := eventPMLogLink(t, s)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		eventPMDetailsOn(t, s.chat.Id)
		eventPMLogOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden: bot can't initiate conversation with a user", 0))
		eventPMBanCommand(t, s, "spam")
		eventPMDetailsWantEdit(t, s)
		eventPMLogWant(t, s, channel, "eventmsg_pm_why_blocked")
	})

	t.Run("no content", func(t *testing.T) {
		const copyText, reason, name = "SENTINEL_COPY_TEXT", "SENTINEL_REASON", "SENTINEL_NAME"
		s := newEventPMSite(t)
		channel := eventPMLogLink(t, s)
		script := eventPMScriptOf(t, s)
		script.mu.Lock()
		script.memberName = name
		script.mu.Unlock()
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, copyText+" {reason}", true)
		eventPMDetailsOn(t, s.chat.Id)
		eventPMLogOn(t, s.chat.Id)
		eventPMRefuse(t, s, eventPMTelegramError(403, "Forbidden", 0))
		eventPMBanCommand(t, s, reason)
		token := eventPMDetailsWantEdit(t, s)
		posts := eventPMLogPosts(t, s, channel)
		if len(posts) != 1 {
			t.Fatalf("#PM_COPY posts = %d, want one", len(posts))
		}
		text := fmt.Sprint(posts[0]["text"])
		for _, forbidden := range []string{copyText, reason, token, pmCopyDetailsPrefix} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("the log line %q carries %q", text, forbidden)
			}
		}
		mention := formatting.MentionHtml(eventPMMember, name)
		if strings.Count(text, name) != 1 || !strings.Contains(text, mention) {
			t.Fatalf("the log line %q names the member other than once inside the mention %q", text, mention)
		}
	})

	t.Run("event name", func(t *testing.T) {
		tr := i18n.MustNewTranslator("uk")
		if got, _ := tr.GetString("settings_msg_ev_kick"); eventPMEventName(tr, eventmsg.EventKick) != got || got == "" {
			t.Fatalf("eventPMEventName(kick) = %q, want the page's name %q", eventPMEventName(tr, eventmsg.EventKick), got)
		}
		if got := eventPMEventName(tr, "pm<x>"); got != "pm&lt;x&gt;" {
			t.Fatalf("eventPMEventName of an unlisted event = %q, want it escaped", got)
		}
		line := pmCopyLogText(tr, eventPMOffer{Event: eventmsg.EventBan, UserID: 7}, pmCopyUnknown)
		if !strings.Contains(line, formatting.MentionHtml(7, "7")) {
			t.Fatalf("a line with no name = %q, want the ID as the mention's text", line)
		}
	})
}
