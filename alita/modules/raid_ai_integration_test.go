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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/config"
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

// failOpenBound is how long one update may take with the AI providers down (the research's
// fail-open bound).
const failOpenBound = 500 * time.Millisecond

// failOpenMode is one way the AI providers can fail, TypeSafe and Gemini both.
type failOpenMode struct {
	name string
	// holds is true when the mode leaves raid AI jobs unfinished until released.
	holds bool
	// silent is true when no provider request may be made once the mode is applied.
	silent bool
	// apply puts env's providers in the mode and returns what releases anything it holds.
	apply func(env *raidAIImageEnv, typesafe *atomic.Int64) (release func())
}

// failOpenProviders installs handlers for the TypeSafe and the Gemini fakes of env, the
// TypeSafe one counted in typesafe (Gemini's requests are env.geminiRequests).
func failOpenProviders(env *raidAIImageEnv, typesafe *atomic.Int64, ts, gemini http.HandlerFunc) {
	withAISpamJevServer(env.t, func(w http.ResponseWriter, r *http.Request) {
		typesafe.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		ts(w, r)
	})
	env.answerWith(gemini)
}

// failOpenAnswer is a handler that always answers with status and body.
func failOpenAnswer(status int, header, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if header != "" {
			w.Header().Set("Retry-After", header)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// failOpenSpamAnswers is a pair of handlers that judge everything spam.
func failOpenSpamAnswers() (ts, gemini http.HandlerFunc) {
	ts = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(aispamDecisionBody(0.95, "scam"))) }
	gemini = func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(raidAIGeminiAnswer(0.95, ""))) }
	return ts, gemini
}

var failOpenModes = []failOpenMode{
	{
		name: "hung", holds: true,
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			gate := make(chan struct{})
			var once sync.Once
			release := func() { once.Do(func() { close(gate) }) }
			hang := func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-gate:
				case <-r.Context().Done():
				}
				w.WriteHeader(http.StatusInternalServerError)
			}
			failOpenProviders(env, typesafe, hang, hang)
			env.t.Cleanup(release)
			return release
		},
	},
	{
		name: "500",
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			failOpenProviders(env, typesafe,
				failOpenAnswer(http.StatusInternalServerError, "", "boom"),
				failOpenAnswer(http.StatusInternalServerError, "", "boom"))
			return func() {}
		},
	},
	{
		name: "429 with a retry delay of 30 s",
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			const limited = `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"30s"}]}}`
			failOpenProviders(env, typesafe,
				failOpenAnswer(http.StatusTooManyRequests, "30", "slow down"),
				failOpenAnswer(http.StatusTooManyRequests, "30", limited))
			return func() {}
		},
	},
	{
		name: "unreadable JSON",
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			failOpenProviders(env, typesafe,
				failOpenAnswer(http.StatusOK, "", "{not json"),
				failOpenAnswer(http.StatusOK, "", "{not json"))
			return func() {}
		},
	},
	{
		name: "keys missing", silent: true,
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			ts, gemini := failOpenSpamAnswers()
			failOpenProviders(env, typesafe, ts, gemini)
			withAISpamTestConfig(env.t, "", true, 0)
			previous := config.AppConfig.GeminiAPIKey
			config.AppConfig.GeminiAPIKey = ""
			env.t.Cleanup(func() { config.AppConfig.GeminiAPIKey = previous })
			return func() {}
		},
	},
	{
		name: "breakers open", silent: true,
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			failOpenProviders(env, typesafe,
				failOpenAnswer(http.StatusInternalServerError, "", "boom"),
				failOpenAnswer(http.StatusInternalServerError, "", "boom"))
			// Enough real failures, text and image, to open both of the chat's breakers.
			members := env.newMembers(3, 2*time.Hour)
			for i := 0; i < 8; i++ {
				env.sendText(members[i%len(members)], fmt.Sprintf("SPAM deal t.me/scam trip %d", i))
				env.sendMedia(members[i%len(members)], gotgbot.Message{Photo: aiPhoto(fmt.Sprintf("trip-%d", i))})
			}
			env.settleAI()
			return func() {}
		},
	},
	{
		name: "queue full", holds: true, silent: true,
		apply: func(env *raidAIImageEnv, typesafe *atomic.Int64) func() {
			ts, gemini := failOpenSpamAnswers()
			failOpenProviders(env, typesafe, ts, gemini)

			raidAIJobs.wait()
			gate := make(chan struct{})
			var once sync.Once
			started := make(chan struct{}, raidAIWorkerCount)
			previous := raidAIProcess
			raidAIProcess = func(raidAIJob) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-gate
			}
			release := func() {
				once.Do(func() {
					close(gate)
					raidAIJobs.wait()
					raidAIProcess = previous
				})
			}
			env.t.Cleanup(release)

			job := raidAIJob{chat: gotgbot.Chat{Id: -1001234500888, Type: "supergroup"}, userID: 11, text: "SPAM t.me/x"}
			for i := 0; i < raidAIWorkerCount; i++ {
				if !raidAIEnqueue(job) {
					env.t.Fatalf("job %d was shed by an idle pool", i+1)
				}
			}
			for i := 0; i < raidAIWorkerCount; i++ {
				select {
				case <-started:
				case <-time.After(10 * time.Second):
					env.t.Fatalf("only %d of %d workers took a job", i, raidAIWorkerCount)
				}
			}
			for i := 0; i < raidAIQueueCapacity; i++ {
				if !raidAIEnqueue(job) {
					env.t.Fatalf("queued job %d of %d was shed before the queue was full", i+1, raidAIQueueCapacity)
				}
			}
			if raidAIEnqueue(job) {
				env.t.Fatal("a job was accepted although the workers and the queue were full")
			}
			return release
		},
	},
}

// dispatchWithin runs one message update in chat through the env's real dispatcher and
// reports whether ProcessUpdate returned within failOpenBound.
func (e *lockdownEnv) dispatchWithin(chat gotgbot.Chat, msg gotgbot.Message) bool {
	e.t.Helper()
	id := e.updateID()
	msg.MessageId, msg.Date, msg.Chat = id, 1, chat
	var processErr error
	within := returnsWithin(failOpenBound, func() {
		processErr = e.dispatcher.ProcessUpdate(e.bot, &gotgbot.Update{UpdateId: id, Message: &msg}, nil)
	})
	if within && processErr != nil {
		e.t.Fatalf("ProcessUpdate(%d) error = %v", id, processErr)
	}
	return within
}

// probeFailOpen has each member send a link text and a photo and fails the test when any
// update takes longer than failOpenBound.
func (e *raidAIImageEnv) probeFailOpen(members []gotgbot.User, tag string) {
	e.t.Helper()
	for i, member := range members {
		member := member
		text := gotgbot.Message{From: &member, Text: fmt.Sprintf("SPAM deal t.me/scam %s %d", tag, i)}
		if !e.dispatchWithin(e.chat, text) {
			e.t.Fatalf("the link text of member %d did not return within %s", i, failOpenBound)
		}
		photo := gotgbot.Message{From: &member, Photo: aiPhoto(fmt.Sprintf("%s-%d", tag, i))}
		if !e.dispatchWithin(e.chat, photo) {
			e.t.Fatalf("the photo of member %d did not return within %s", i, failOpenBound)
		}
	}
}

func TestRaidAIFailOpen(t *testing.T) {
	for _, mode := range failOpenModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Run("join surge locks", func(t *testing.T) {
				env := newRaidAIImageEnv(t)
				var typesafe atomic.Int64
				release := mode.apply(env, &typesafe)
				tsBefore, geminiBefore := typesafe.Load(), env.geminiRequests()
				members := env.newMembers(3, 2*time.Hour)

				env.probeFailOpen(members, "probe")

				if mode.silent {
					if got := typesafe.Load() - tsBefore; got != 0 {
						t.Errorf("%d TypeSafe requests, want none in this mode", got)
					}
					if got := env.geminiRequests() - geminiBefore; got != 0 {
						t.Errorf("%d Gemini requests, want none in this mode", got)
					}
				}
				if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
					t.Errorf("verdict items = %d, want 0: a failure is no verdict", got)
				}
				if !mode.holds {
					env.settleAI()
					if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
						t.Errorf("verdict items after the checks ended = %d, want 0", got)
					}
				}
				if row := env.activeLockdown(); row != nil {
					t.Fatalf("a lockdown (kind %q) exists although no verdict counted", row.TriggerKind)
				}

				// The join surge still locks the raided group, with the providers still down.
				env.selfJoins(15)
				row := env.activeLockdown()
				if row == nil || row.TriggerKind != models.LockdownTriggerJoinSurge || row.LockedAt == nil {
					t.Fatalf("active lockdown = %+v, want a confirmed join surge lockdown", row)
				}

				release()
				env.settleAI()
				if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
					t.Errorf("verdict items after the release = %d, want 0", got)
				}
				if row := env.activeLockdown(); row == nil || row.TriggerKind != models.LockdownTriggerJoinSurge {
					t.Errorf("active lockdown = %+v after the release, want still the join surge's", row)
				}
				if !mode.silent && int(typesafe.Load())+env.geminiRequests() == int(tsBefore)+geminiBefore {
					t.Error("no provider was asked: the case would prove nothing")
				}
			})

			t.Run("flood locks", func(t *testing.T) {
				env := newRaidAIImageEnv(t)
				var typesafe atomic.Int64
				release := mode.apply(env, &typesafe)
				recent := env.newMembers(4, 2*time.Hour)

				kinds := []string{"text", "photo", "gif", "sticker"}
				for i := 0; i < 30; i++ {
					msg := floodMessage(t, recent[i%len(recent)], kinds[i%len(kinds)])
					if !env.dispatchWithin(env.chat, msg) {
						t.Fatalf("flood item %d (%s) did not return within %s", i+1, kinds[i%len(kinds)], failOpenBound)
					}
				}

				row := env.activeLockdown()
				if row == nil || row.TriggerKind != models.LockdownTriggerFlood || row.LockedAt == nil {
					t.Fatalf("active lockdown = %+v, want a confirmed flood lockdown", row)
				}
				if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
					t.Errorf("verdict items = %d, want 0: a failure is no verdict", got)
				}

				release()
				env.settleAI()
				if got := aiVerdictSetSize(env.lockdownEnv, "av"); got != 0 {
					t.Errorf("verdict items after the release = %d, want 0", got)
				}
			})
		})
	}
}
