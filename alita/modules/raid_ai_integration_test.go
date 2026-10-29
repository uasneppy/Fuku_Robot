//go:build testtools

package modules

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/approvals"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// aiPhoto is a photo whose file ID is id, so the file server serves bytes that hold
// "files/<id>" and every photo is a different file.
func aiPhoto(id string) []gotgbot.PhotoSize {
	return []gotgbot.PhotoSize{
		{FileId: id + "-s", FileUniqueId: "u" + id + "-s", Width: 90, Height: 60},
		{FileId: id, FileUniqueId: "u" + id, Width: 800, Height: 600},
	}
}

// serveGeminiByFileName replaces the env's Gemini server with one that judges an image spam
// (0.92) when the downloaded bytes name a file that starts with "spam", and keep it (0.1)
// otherwise. It returns the counter of requests it received.
func (e *raidAIImageEnv) serveGeminiByFileName() *atomic.Int64 {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, _ := io.ReadAll(r.Body)
		probability := 0.1
		for _, image := range inlineImages(e.t, body) {
			raw, err := base64.StdEncoding.DecodeString(image.Data)
			if err != nil {
				e.t.Errorf("decode the inline image: %v", err)
			}
			if bytes.Contains(raw, []byte("files/spam")) {
				probability = 0.92
			}
		}
		_, _ = w.Write([]byte(raidAIGeminiAnswer(probability, "")))
	}))
	e.t.Cleanup(server.Close)
	previous := raidAIGeminiEndpoint
	raidAIGeminiEndpoint = server.URL
	e.t.Cleanup(func() { raidAIGeminiEndpoint = previous })
	return &requests
}

// callsSince returns every Bot API call the fake recorded after the first n, once the
// raid posts and AI jobs in flight are done.
func (e *lockdownEnv) callsSince(n int) []moduleBotCall {
	raidAIJobs.wait()
	raidPosts.wait()
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	if n > len(e.fake.calls) {
		n = len(e.fake.calls)
	}
	return append([]moduleBotCall(nil), e.fake.calls[n:]...)
}

// callCount is how many Bot API calls the fake recorded so far.
func (e *lockdownEnv) callCount() int {
	e.fake.mu.Lock()
	defer e.fake.mu.Unlock()
	return len(e.fake.calls)
}

func TestRaidAIImageBurst(t *testing.T) {
	env := newRaidAIImageEnv(t)
	staffChat := env.staffChatOf()
	t.Cleanup(func() { db.DB.Where("chat_id = ?", env.chat.Id).Delete(&models.ChatLockdownBurst{}) })
	t.Cleanup(func() { _ = approvals.RemoveAllApprovedUsers(env.chat.Id) })
	env.answerProbability(0.92)
	accounts := env.seedFlooders(3, 2*time.Hour, true)

	n := 0
	post := func(account floodAccount, photos int) {
		for i := 0; i < photos; i++ {
			n++
			env.sendMedia(account.user, gotgbot.Message{Photo: aiPhoto(fmt.Sprintf("burst-%d", n))})
		}
	}
	post(accounts[0], 2)
	post(accounts[1], 2)
	env.settleAI()
	if row := env.activeLockdown(); row != nil {
		t.Fatalf("a lockdown (kind %q) exists after 4 spam photos, want none", row.TriggerKind)
	}
	post(accounts[2], 1)
	env.settleAI()

	row := env.activeLockdown()
	if row == nil || row.TriggerKind != models.LockdownTriggerAIBurst || row.LockedAt == nil {
		t.Fatalf("active lockdown = %+v, want a confirmed AI burst lockdown", row)
	}
	if row.TriggerCount != 5 || row.TriggerWindow != 120 || row.TriggerPercent != 0 {
		t.Errorf("trigger numbers = %d verdicts in %d s, %d%%, want 5 in 120 s, 0%%",
			row.TriggerCount, row.TriggerWindow, row.TriggerPercent)
	}
	if row.StartedBy != staffTestBotID {
		t.Errorf("StartedBy = %d, want the bot %d", row.StartedBy, staffTestBotID)
	}
	if got := env.geminiRequests(); got != 5 {
		t.Errorf("Gemini requests = %d, want one per photo (5)", got)
	}
	if got := len(env.calls("setChatPermissions")); got != 1 {
		t.Errorf("setChatPermissions calls = %d, want exactly 1", got)
	}
	if got := len(env.calls("deleteMessage")) + len(env.calls("banChatMember")) + len(env.calls("restrictChatMember")); got != 0 {
		t.Errorf("%d delete, ban or restrict calls, want none: only the lockdown's lock acts", got)
	}

	notice := env.wantOneNoticeIn()
	if want := staffMarker("lockdown_auto_started_ai_burst") + " 5 120"; !strings.Contains(notice, want) {
		t.Errorf("public notice %q does not contain %q", notice, want)
	}

	env.cycle()
	text := alertText(t, env.lockdownEnv, staffChat)
	for _, want := range []string{
		staffMarker("lockdown_alert_trigger_ai_burst") + " 5 120",
		staffMarker("lockdown_alert_counts_ai"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("alert %q does not contain %q", text, want)
		}
	}
	for _, account := range accounts {
		for _, want := range []string{
			html.EscapeString(account.user.FirstName),
			fmt.Sprintf("<code>%d</code>", account.user.Id),
		} {
			if !strings.Contains(text, want) {
				t.Errorf("alert %q does not contain %q", text, want)
			}
		}
	}
	if strings.Contains(text, "<b>1</b>") {
		t.Errorf("alert %q holds an unescaped name tag", text)
	}
	if got := sampleLines(text); got != 3 {
		t.Errorf("%d account lines in the alert, want 3\n%s", got, text)
	}

	alert := lastAlert(t, env.lockdownEnv, staffChat)
	keyboard := staffKeyboardOf(alert.alert.Params["reply_markup"])
	if len(keyboard) < 2 || len(keyboard[1]) != 1 {
		t.Fatalf("alert keyboard = %+v, want a second row with the Ban button", keyboard)
	}
	if label := keyboard[1][0].Text; !strings.Contains(label, staffMarker("lockdown_alert_btn_ban")+" 3") {
		t.Errorf("Ban button label = %q, want it to carry %q with 3", label, staffMarker("lockdown_alert_btn_ban"))
	}

	locksBefore := len(env.calls("setChatPermissions"))
	env.askStatus(env.admin)
	env.wantReplyHas(
		staffMarker("lockdown_status_active_auto"),
		staffMarker("lockdown_trigger_ai_burst")+" 5 120",
	)
	if got := len(env.calls("setChatPermissions")); got != locksBefore {
		t.Errorf("setChatPermissions calls = %d after /lockdownstatus, want still %d", got, locksBefore)
	}
}

func TestRaidAINeverActs(t *testing.T) {
	// One verdict per message: a text holding SPAM or a file named spam-* is spam, the rest
	// are kept.
	type scenario struct {
		name string
		// accounts lists, per account, how many spam messages and how many kept messages it sends.
		accounts [][2]int
	}
	scenarios := []scenario{
		{"keep verdicts", [][2]int{{0, 2}, {0, 2}, {0, 2}}},
		{"one spam verdict", [][2]int{{1, 0}}},
		{"four spam verdicts from three accounts", [][2]int{{2, 0}, {1, 0}, {1, 0}}},
		{"five spam verdicts from two accounts", [][2]int{{3, 0}, {2, 0}}},
	}
	paths := []string{"text", "photo", "text and photo"}

	for _, path := range paths {
		for _, sc := range scenarios {
			t.Run(path+"/"+sc.name, func(t *testing.T) {
				env := newRaidAIImageEnv(t)
				geminiRequests := env.serveGeminiByFileName()
				users := env.newMembers(len(sc.accounts), 2*time.Hour)
				baseline := env.callCount()

				seq := 0
				send := func(user gotgbot.User, spam bool) {
					seq++
					viaPhoto := path == "photo" || (path == "text and photo" && seq%2 == 0)
					if viaPhoto {
						name := "ham"
						if spam {
							name = "spam"
						}
						env.sendMedia(user, gotgbot.Message{Photo: aiPhoto(fmt.Sprintf("%s-%d", name, seq))})
						return
					}
					text := fmt.Sprintf("see https://example.org/page %d", seq)
					if spam {
						text = fmt.Sprintf("SPAM deal t.me/scam %d", seq)
					}
					env.sendText(user, text)
				}
				wantSpam, wantAccounts := 0, 0
				for i, counts := range sc.accounts {
					for j := 0; j < counts[0]; j++ {
						send(users[i], true)
					}
					for j := 0; j < counts[1]; j++ {
						send(users[i], false)
					}
					wantSpam += counts[0]
					if counts[0] > 0 {
						wantAccounts++
					}
				}
				env.settleAI()

				if checks := env.aiRequests() + int(geminiRequests.Load()); checks != seq {
					t.Fatalf("%d provider checks for %d messages: the case would prove nothing", checks, seq)
				}
				if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != wantSpam {
					t.Errorf("verdict items = %d, want %d", got, wantSpam)
				}
				if got := aiVerdictSetSize(env.lockdownEnv, "aa"); got != wantAccounts {
					t.Errorf("verdict accounts = %d, want %d", got, wantAccounts)
				}
				if row := env.activeLockdown(); row != nil {
					t.Errorf("a lockdown (kind %q) exists, want none: below the rule AI never acts", row.TriggerKind)
				}
				if got := env.lockdownRowCount(); got != 0 {
					t.Errorf("lockdown rows = %d, want none", got)
				}

				// Nothing was written to Telegram: every call after the setup is a read.
				seen := make(map[string]bool)
				for _, call := range env.callsSince(baseline) {
					seen[call.Method] = true
					if !strings.HasPrefix(call.Method, "get") {
						t.Errorf("the AI path made a %s call, want reads only", call.Method)
					}
				}
				methods := make([]string, 0, len(seen))
				for method := range seen {
					methods = append(methods, method)
				}
				sort.Strings(methods)
				t.Logf("Telegram calls after the setup: %v, getFile: %d", methods, len(env.client.getFiles()))
				env.wantNoActions()
			})
		}
	}
}
