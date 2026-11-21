//go:build testtools

package modules

import (
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/eventmsg"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
)

// The safeguards of the private copies (plan 22-08): every text a private copy job can send is measured against
// Telegram's limits in all 8 languages, the job never makes the handler wait, and no copy, Details view or log line names
// the Staff Group or another group.

// eventPMWorstTitle is a 64-rune group title full of characters the escaper changes, so the escaped title is as long as a
// title can make a text.
var eventPMWorstTitle = strings.Repeat("<&Ж>", 16)

// eventPMWorstReason is a 200-character reason full of characters the escaper changes.
var eventPMWorstReason = strings.Repeat("&<b>", 50)

// eventPMWorstName is a 64-rune member name full of markup, the worst name a log line can mention.
var eventPMWorstName = strings.Repeat("<&Ы>", 16)

// eventPMTimed reports whether the Messages page draws event as a timed event (its texts fill {until}).
func eventPMTimed(t *testing.T, event string) bool {
	t.Helper()
	for _, fam := range settingsMsgFamilies {
		for _, ev := range fam.Events {
			if ev.Key == event {
				return ev.Timed
			}
		}
	}
	t.Fatalf("no family of the Messages page lists the %s event", event)
	return false
}

// eventPMSpecEvents is every event with a private copy spec, in a fixed order.
func eventPMSpecEvents() []string {
	events := make([]string, 0, len(eventPMSpecs))
	for event := range eventPMSpecs {
		events = append(events, event)
	}
	slices.Sort(events)
	return events
}

// eventPMWantFilled fails when text is empty, longer than limit UTF-16 units, or keeps an unfilled placeholder, a broken
// printf verb or an empty bold pair.
func eventPMWantFilled(t *testing.T, label, text string, limit int) {
	t.Helper()
	if units := panelUTF16Len(text); units == 0 || units > limit {
		t.Errorf("%s is %d UTF-16 units, want 1..%d: %q", label, units, limit, text)
	}
	for _, bad := range []string{"{", "%!", "<b></b>"} {
		if strings.Contains(text, bad) {
			t.Errorf("%s keeps %q: %q", label, bad, text)
		}
	}
}

func TestEventPMTextsFitEveryLocale(t *testing.T) {
	withRealLocales(t)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: eventPMWorstTitle}
	events := eventPMSpecEvents()
	if len(events) == 0 {
		t.Fatal("no event has a private copy spec, so the sweep proves nothing")
	}
	until := time.Now().Add(2 * time.Hour)
	escapedName := html.EscapeString(eventPMWorstName)
	mention := formatting.MentionHtml(eventPMMember, eventPMWorstName)

	for _, code := range memberReqScreenLanguages {
		t.Run(code, func(t *testing.T) {
			tr := i18n.MustNewTranslator(code)
			note, _ := tr.GetString("eventmsg_pm_unmute_lockdown")
			if strings.TrimSpace(note) == "" {
				t.Fatal("the unmute lockdown note is empty")
			}
			for _, event := range events {
				spec, _ := eventPMSpecFor(event)
				values := eventValues{Reason: eventPMWorstReason, Duration: "366 days"}
				if eventPMTimed(t, event) {
					values.Until = until
				}
				// The default private text with every placeholder filled, then the same text with the lockdown note, which only
				// /unmute adds, measured for every event because a note only adds its own length.
				text := eventPMDefaultText(tr, spec, &chat, values, "")
				eventPMWantFilled(t, event+" default", text, 4096)
				if !strings.Contains(text, staffDisplayTitle(eventPMWorstTitle)) {
					t.Errorf("%s default %q does not name the group", event, text)
				}
				withNote := eventPMDefaultText(tr, spec, &chat, values, note)
				eventPMWantFilled(t, event+" default with the lockdown note", withNote, 4096)
				if !strings.HasSuffix(withNote, "\n"+note) {
					t.Errorf("%s default with the note %q does not end with the note", event, withNote)
				}

				for _, outcome := range []pmCopyOutcome{pmCopyBlocked, pmCopyRefused, pmCopyBusy, pmCopyUnknown} {
					offer := eventPMOffer{Chat: chat, Event: event, UserID: eventPMMember, TargetName: eventPMWorstName}
					line := pmCopyLogText(tr, offer, outcome)
					label := fmt.Sprintf("%s log line (%s)", event, outcome)
					eventPMWantFilled(t, label, line, 1000)
					if !strings.HasPrefix(line, pmCopyLogTag+"\n") {
						t.Errorf("%s does not open with %s: %q", label, pmCopyLogTag, line)
					}
					// The member's name appears once, escaped, and only inside the mention.
					if strings.Count(line, escapedName) != 1 || !strings.Contains(line, mention) || strings.Contains(line, eventPMWorstName) {
						t.Errorf("%s names the member outside the mention or unescaped: %q", label, line)
					}
					if strings.Contains(line, eventPMWorstTitle) || strings.Contains(line, staffDisplayTitle(eventPMWorstTitle)) {
						t.Errorf("%s names the group: %q", label, line)
					}
				}
			}

			label, _ := tr.GetString("eventmsg_btn_details")
			if n := len([]rune(label)); strings.TrimSpace(label) == "" || n > 20 {
				t.Errorf("the Details label %q has %d code points, want 1..20", label, n)
			}
			none, _ := tr.GetString("eventmsg_pm_details_none")
			if n := len([]rune(none)); strings.TrimSpace(none) == "" || n > 120 {
				t.Errorf("the neutral Details reply %q has %d code points, want 1..120", none, n)
			}
			for _, text := range []string{label, none} {
				eventPMWantFilled(t, "a Details string", text, 4096)
			}

			// The Details record of the worst default copy is what the deep link shows: its record and its URL button fit.
			rec := pmCopyDetailsRecordOf(eventPMOffer{Chat: chat, Event: events[0], UserID: eventPMMember},
				media.GreetingMessage{Text: eventPMDefaultText(tr, eventPMSpecs[events[0]], &chat, eventValues{Reason: eventPMWorstReason}, note), MsgType: db.TEXT})
			eventPMWantFilled(t, "the Details record's text", rec.Text, 4096)
			if url := "https://t.me/" + strings.Repeat("b", 32) + "?start=" + pmCopyDetailsPrefix + strings.Repeat("A", pmCopyTokenLen); len(url) > 200 {
				t.Errorf("the Details URL is %d bytes", len(url))
			}
			if payload := pmCopyDetailsPrefix + strings.Repeat("A", pmCopyTokenLen); len(payload) > 64 {
				t.Errorf("the Details start payload is %d characters, over Telegram's 64", len(payload))
			}
		})
	}
}

// eventPMRequestList is every request a site recorded as method and chat, with the site's own group ID written <chat>, so
// the lists of two sites compare.
func eventPMRequestList(s *botSite) []string {
	group := strconv.FormatInt(s.chat.Id, 10)
	var list []string
	for _, call := range eventPMAllCalls(s) {
		chat := fmt.Sprint(call.Params["chat_id"])
		if chat == group {
			chat = "<chat>"
		}
		list = append(list, call.Method+" "+chat)
	}
	return list
}

func TestEventPMCost(t *testing.T) {
	withModerationGoldenLocales(t)

	// The golden case's site: no private copy engine wrapper, no Redis of its own. Its requests are the list a /ban makes.
	golden := newModerationGoldenSite(t)
	eventPMBanCommand(t, golden, "spam")
	want := eventPMRequestList(golden)
	if !slices.Contains(want, "banChatMember <chat>") || !slices.Contains(want, "sendMessage <chat>") {
		t.Fatalf("the golden /ban made %v, want the ban and the group reply, or the comparison proves nothing", want)
	}

	t.Run("copy off adds no request", func(t *testing.T) {
		s := newEventPMSite(t)
		eventPMBanCommand(t, s, "spam")
		waitEventPM(t)
		if got := eventPMRequestList(s); !slices.Equal(got, want) {
			t.Fatalf("/ban with the copy off made %v, want the golden case's %v", got, want)
		}
		eventPMNone(t, s)
		if edits := eventPMDetailsEdits(t, s); len(edits) != 0 {
			t.Fatalf("/ban with the copy off made %d Details edits", len(edits))
		}
	})

	t.Run("copy on never makes the handler wait", func(t *testing.T) {
		s := newEventPMSite(t)
		storeEventPM(t, s.chat.Id, eventmsg.EventBan, "", true)
		release := eventPMHold(t, s)
		started := time.Now()
		eventPMBanCommand(t, s, "spam")
		if took := time.Since(started); took > 5*time.Second {
			t.Fatalf("/ban took %s while the copy was held, want it at once", took)
		}
		// The handler returned with the ban and the group reply done, and made no request of the copy's: the held send is not
		// recorded yet, so the list is exactly the golden case's.
		if got := eventPMRequestList(s); !slices.Equal(got, want) {
			t.Fatalf("/ban with the copy on made %v before the copy was released, want the golden case's %v", got, want)
		}
		select {
		case <-pmCopyJobs.idleChan():
			t.Fatal("no private copy job is in flight, so the copy was not held and the test proves nothing")
		default:
		}
		release()
		eventPMOnly(t, s, "sendMessage")
		var toMember int
		for _, call := range eventPMAllCalls(s) {
			if fmt.Sprint(call.Params["chat_id"]) == strconv.FormatInt(eventPMMember, 10) && call.Method != "getChat" {
				toMember++
			}
		}
		if toMember != 1 {
			t.Fatalf("requests to the member's private chat after the release = %d, want the one copy", toMember)
		}
	})
}
