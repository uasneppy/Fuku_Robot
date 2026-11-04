//go:build testtools

package modules

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// settingsInputTestUser is a user nobody else in the test run uses.
func settingsInputTestUser() gotgbot.User {
	return gotgbot.User{Id: uniqueLinkOwnerID() + 9_000_000, FirstName: "Typist"}
}

// settingsInputTestWait is a valid wait for the welcome message's text.
func settingsInputTestWait(t *testing.T, groupID int64) settingsInputWait {
	t.Helper()
	nonce, err := newSettingsInputNonce()
	if err != nil {
		t.Fatalf("newSettingsInputNonce() error = %v", err)
	}
	return settingsInputWait{GroupID: groupID, Page: "w", Part: "x", PromptMsgID: 7, Nonce: nonce}
}

// settingsInputRepliesTo returns the texts the bot sent to chatID.
func (e *settingsEnv) settingsInputRepliesTo(chatID int64) []string {
	var texts []string
	for _, sent := range e.fake.sentTo(chatID) {
		texts = append(texts, sent.Params["text"].(string))
	}
	return texts
}

func TestSettingsInputStore(t *testing.T) {
	env := newSettingsEnv(t)
	user := settingsInputTestUser()
	wait := settingsInputTestWait(t, -100)

	previous, err := settingsInputSet(context.Background(), user.Id, wait)
	if err != nil {
		t.Fatalf("settingsInputSet() error = %v", err)
	}
	if previous != nil {
		t.Fatalf("settingsInputSet() previous = %+v, want nil for the first wait", previous)
	}

	key := settingsInputKey(user.Id)
	fields, err := env.redis.HKeys(key)
	if err != nil {
		t.Fatalf("HKeys(%s) error = %v", key, err)
	}
	if got := strings.Join(sorted(fields), ","); got != "c,m,n,p,t,x" {
		t.Errorf("wait fields = %q, want exactly c,m,n,p,t,x", got)
	}
	if got := env.redis.HGet(key, "c"); got != "-100" {
		t.Errorf("field c = %q, want -100", got)
	}
	if got := env.redis.HGet(key, "n"); got != wait.Nonce {
		t.Errorf("field n = %q, want the nonce that was set", got)
	}
	if ttl := env.redis.TTL(key); ttl != settingsInputTTL {
		t.Errorf("wait TTL = %v, want %v", ttl, settingsInputTTL)
	}

	marker, err := env.redis.Get(settingsInputDoneKey(user.Id))
	if err != nil || marker != "-100|w|x" {
		t.Errorf("marker = %q, %v, want -100|w|x", marker, err)
	}
	if ttl := env.redis.TTL(settingsInputDoneKey(user.Id)); ttl != settingsInputMarkerTTL {
		t.Errorf("marker TTL = %v, want %v", ttl, settingsInputMarkerTTL)
	}

	// A second Set returns the first wait, so the caller can tell a replaced wait from a
	// refreshed one.
	second := settingsInputTestWait(t, -200)
	second.Part = "m"
	previous, err = settingsInputSet(context.Background(), user.Id, second)
	if err != nil {
		t.Fatalf("second settingsInputSet() error = %v", err)
	}
	if previous == nil || previous.GroupID != -100 || previous.Page != "w" || previous.Part != "x" || previous.Nonce != wait.Nonce {
		t.Errorf("second settingsInputSet() previous = %+v, want the first wait", previous)
	}
	if got := env.redis.HGet(key, "t"); got != "m" {
		t.Errorf("field t after the second Set = %q, want m", got)
	}
}

func TestSettingsInputSetRefusesInvalidWait(t *testing.T) {
	env := newSettingsEnv(t)
	user := settingsInputTestUser()

	cases := map[string]func(*settingsInputWait){
		"non-negative group": func(w *settingsInputWait) { w.GroupID = 5 },
		"unknown page":       func(w *settingsInputWait) { w.Page = "q" },
		"unknown part":       func(w *settingsInputWait) { w.Part = "z" },
		"bad nonce":          func(w *settingsInputWait) { w.Nonce = "XYZ" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			wait := settingsInputTestWait(t, -100)
			mutate(&wait)
			if _, err := settingsInputSet(context.Background(), user.Id, wait); err == nil {
				t.Fatal("settingsInputSet() error = nil, want a refusal")
			}
			if env.redis.Exists(settingsInputKey(user.Id)) || env.redis.Exists(settingsInputDoneKey(user.Id)) {
				t.Error("a refused wait wrote a key")
			}
		})
	}
}

func TestSettingsEditCancelCommand(t *testing.T) {
	env := newSettingsEnv(t)
	user := settingsInputTestUser()
	private := user.Id

	if _, err := settingsInputSet(context.Background(), user.Id, settingsInputTestWait(t, -100)); err != nil {
		t.Fatalf("settingsInputSet() error = %v", err)
	}

	env.sendPrivate(user, "/cancel")
	replies := env.settingsInputRepliesTo(private)
	if len(replies) != 1 || !strings.Contains(replies[0], staffMarker("settings_edit_cancelled")) {
		t.Fatalf("first /cancel replies = %v, want one settings_edit_cancelled", replies)
	}
	if env.redis.Exists(settingsInputKey(user.Id)) || env.redis.Exists(settingsInputDoneKey(user.Id)) {
		t.Error("/cancel left the wait or its marker behind")
	}

	env.sendPrivate(user, "/cancel")
	replies = env.settingsInputRepliesTo(private)
	if len(replies) != 2 || !strings.Contains(replies[1], staffMarker("settings_edit_cancel_nothing")) {
		t.Fatalf("second /cancel replies = %v, want a settings_edit_cancel_nothing", replies)
	}
}

func TestSettingsEditCancelCommandIsSilentInAGroup(t *testing.T) {
	env := newSettingsEnv(t)
	if _, err := settingsInputSet(context.Background(), env.admin.Id, settingsInputTestWait(t, -100)); err != nil {
		t.Fatalf("settingsInputSet() error = %v", err)
	}
	callsBefore := env.callCount()
	sentBefore := len(env.fake.sentTo(env.chat.Id))

	env.send(env.admin, "/cancel")

	if got := len(env.fake.sentTo(env.chat.Id)); got != sentBefore {
		t.Errorf("/cancel in a group sent %d message(s), want none", got-sentBefore)
	}
	if got := env.callCount(); got != callsBefore {
		t.Errorf("/cancel in a group made %d Telegram call(s), want none", got-callsBefore)
	}
	if !env.redis.Exists(settingsInputKey(env.admin.Id)) {
		t.Error("/cancel in a group deleted the sender's wait")
	}
}

func TestSettingsEditCancelCommandTouchesOnlyTheSender(t *testing.T) {
	env := newSettingsEnv(t)
	user := settingsInputTestUser()
	other := settingsInputTestUser()
	other.Id += 1_000

	for _, id := range []int64{user.Id, other.Id} {
		if _, err := settingsInputSet(context.Background(), id, settingsInputTestWait(t, -100)); err != nil {
			t.Fatalf("settingsInputSet(%d) error = %v", id, err)
		}
	}
	env.sendPrivate(user, "/cancel")
	if env.redis.Exists(settingsInputKey(user.Id)) {
		t.Error("the sender's wait survived /cancel")
	}
	if !env.redis.Exists(settingsInputKey(other.Id)) || !env.redis.Exists(settingsInputDoneKey(other.Id)) {
		t.Error("/cancel deleted another user's wait")
	}
}

// sorted returns a sorted copy of in.
func sorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
