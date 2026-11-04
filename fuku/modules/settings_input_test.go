//go:build testtools

package modules

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
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

// settingsInputSeed writes a valid wait for user and returns it.
func settingsInputSeed(t *testing.T, userID, groupID int64, page, part string) settingsInputWait {
	t.Helper()
	wait := settingsInputTestWait(t, groupID)
	wait.Page, wait.Part = page, part
	if _, err := settingsInputSet(context.Background(), userID, wait); err != nil {
		t.Fatalf("settingsInputSet() error = %v", err)
	}
	return wait
}

func TestSettingsInputCancelOutcomes(t *testing.T) {
	env := newSettingsEnv(t)
	ctx := context.Background()

	t.Run("claim is for the holder of the nonce and only once", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")

		if _, ok, err := settingsInputClaim(ctx, user.Id, "0123456789abcdef"); err != nil || ok {
			t.Fatalf("claim with a stale nonce = %v, %v, want false, nil", ok, err)
		}
		claim, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce)
		if err != nil || !ok || !settingsInputNonceValid(claim) {
			t.Fatalf("claim with the right nonce = %q, %v, %v, want a claim", claim, ok, err)
		}
		if _, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce); err != nil || ok {
			t.Fatalf("second claim = %v, %v, want false, nil", ok, err)
		}
	})

	t.Run("claim after the wait expired", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")
		env.redis.FastForward(settingsInputTTL + time.Minute)
		if _, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce); err != nil || ok {
			t.Fatalf("claim after the expiry = %v, %v, want false, nil", ok, err)
		}
	})

	t.Run("finish deletes the wait and marker for the holder only", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")
		claim, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce)
		if err != nil || !ok {
			t.Fatalf("claim = %v, %v", ok, err)
		}
		if err := settingsInputFinish(ctx, user.Id, "ffffffffffffffff"); err != nil {
			t.Fatalf("finish with another claim error = %v", err)
		}
		if !env.redis.Exists(settingsInputKey(user.Id)) || !env.redis.Exists(settingsInputDoneKey(user.Id)) {
			t.Fatal("finish with another claim deleted the wait or its marker")
		}
		if err := settingsInputFinish(ctx, user.Id, claim); err != nil {
			t.Fatalf("finish error = %v", err)
		}
		if env.redis.Exists(settingsInputKey(user.Id)) || env.redis.Exists(settingsInputDoneKey(user.Id)) {
			t.Error("finish by the holder left the wait or its marker")
		}
	})

	t.Run("release gives the wait back without resetting its TTL", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")
		claim, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce)
		if err != nil || !ok {
			t.Fatalf("claim = %v, %v", ok, err)
		}
		env.redis.FastForward(time.Minute)
		before := env.redis.TTL(settingsInputKey(user.Id))

		if err := settingsInputRelease(ctx, user.Id, "ffffffffffffffff"); err != nil {
			t.Fatalf("release with another claim error = %v", err)
		}
		if env.redis.HGet(settingsInputKey(user.Id), "k") == "" {
			t.Fatal("release with another claim removed the claim")
		}
		if err := settingsInputRelease(ctx, user.Id, claim); err != nil {
			t.Fatalf("release error = %v", err)
		}
		if got := env.redis.HGet(settingsInputKey(user.Id), "k"); got != "" {
			t.Errorf("claim field after release = %q, want none", got)
		}
		if after := env.redis.TTL(settingsInputKey(user.Id)); after != before {
			t.Errorf("TTL after release = %v, want unchanged %v", after, before)
		}
		if _, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce); err != nil || !ok {
			t.Errorf("claim after release = %v, %v, want claimable again", ok, err)
		}
	})

	t.Run("cancel of the same part deletes both keys", func(t *testing.T) {
		user := settingsInputTestUser()
		settingsInputSeed(t, user.Id, -100, "w", "x")
		outcome, err := settingsInputCancel(ctx, user.Id, -100, "w", "x")
		if err != nil || outcome != settingsInputCancelDeleted {
			t.Fatalf("cancel = %v, %v, want Deleted", outcome, err)
		}
		if env.redis.Exists(settingsInputKey(user.Id)) || env.redis.Exists(settingsInputDoneKey(user.Id)) {
			t.Error("cancel left the wait or its marker")
		}
	})

	t.Run("cancel of another part leaves the live wait", func(t *testing.T) {
		user := settingsInputTestUser()
		settingsInputSeed(t, user.Id, -100, "w", "x")
		for _, args := range []struct {
			group      int64
			page, part string
		}{{-100, "w", "m"}, {-100, "g", "x"}, {-200, "w", "x"}} {
			outcome, err := settingsInputCancel(ctx, user.Id, args.group, args.page, args.part)
			if err != nil || outcome != settingsInputCancelOther {
				t.Fatalf("cancel %+v = %v, %v, want Other", args, outcome, err)
			}
		}
		if !env.redis.Exists(settingsInputKey(user.Id)) || !env.redis.Exists(settingsInputDoneKey(user.Id)) {
			t.Error("a stale cancel deleted the live wait")
		}
	})

	t.Run("cancel while a claim is held is busy", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")
		if _, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce); err != nil || !ok {
			t.Fatalf("claim = %v, %v", ok, err)
		}
		outcome, err := settingsInputCancel(ctx, user.Id, -100, "w", "x")
		if err != nil || outcome != settingsInputCancelBusy {
			t.Fatalf("cancel = %v, %v, want Busy", outcome, err)
		}
		if !env.redis.Exists(settingsInputKey(user.Id)) {
			t.Error("a busy cancel deleted the wait")
		}
	})

	t.Run("cancel with neither key and with only the marker", func(t *testing.T) {
		user := settingsInputTestUser()
		outcome, err := settingsInputCancel(ctx, user.Id, -100, "w", "x")
		if err != nil || outcome != settingsInputCancelNone {
			t.Fatalf("cancel with nothing = %v, %v, want None", outcome, err)
		}
		settingsInputSeed(t, user.Id, -100, "w", "x")
		env.redis.FastForward(settingsInputTTL + time.Minute)
		if env.redis.Exists(settingsInputKey(user.Id)) {
			t.Fatal("the wait did not expire")
		}
		outcome, err = settingsInputCancel(ctx, user.Id, -100, "w", "x")
		if err != nil || outcome != settingsInputCancelExpired {
			t.Fatalf("cancel with only the marker = %v, %v, want Expired", outcome, err)
		}
		if env.redis.Exists(settingsInputDoneKey(user.Id)) {
			t.Error("an Expired cancel left the marker")
		}
	})

	t.Run("/cancel while a claim is held leaves the wait", func(t *testing.T) {
		user := settingsInputTestUser()
		wait := settingsInputSeed(t, user.Id, -100, "w", "x")
		if _, ok, err := settingsInputClaim(ctx, user.Id, wait.Nonce); err != nil || !ok {
			t.Fatalf("claim = %v, %v", ok, err)
		}
		env.sendPrivate(user, "/cancel")
		replies := env.settingsInputRepliesTo(user.Id)
		if len(replies) != 1 || !strings.Contains(replies[0], staffMarker("settings_edit_cancel_nothing")) {
			t.Fatalf("/cancel replies = %v, want one settings_edit_cancel_nothing", replies)
		}
		if !env.redis.Exists(settingsInputKey(user.Id)) {
			t.Error("/cancel deleted a wait whose write is in flight")
		}
	})
}

func TestSettingsInputRead(t *testing.T) {
	env := newSettingsEnv(t)
	ctx := context.Background()
	user := settingsInputTestUser()

	wait, marker, err := settingsInputRead(ctx, user.Id)
	if err != nil || wait != nil || marker != "" {
		t.Fatalf("read with nothing = %+v, %q, %v, want nil, empty, nil", wait, marker, err)
	}

	seeded := settingsInputSeed(t, user.Id, -100, "r", "b")
	wait, marker, err = settingsInputRead(ctx, user.Id)
	if err != nil || wait == nil {
		t.Fatalf("read = %+v, %v, want the wait", wait, err)
	}
	if wait.GroupID != -100 || wait.Page != "r" || wait.Part != "b" || wait.PromptMsgID != 7 || wait.Nonce != seeded.Nonce || wait.ExpiresAt.IsZero() {
		t.Errorf("read wait = %+v, want the seeded one", wait)
	}
	if marker != "-100|r|b" {
		t.Errorf("read marker = %q, want -100|r|b", marker)
	}

	// An expired wait leaves only the marker.
	env.redis.FastForward(settingsInputTTL + time.Minute)
	wait, marker, err = settingsInputRead(ctx, user.Id)
	if err != nil || wait != nil || marker != "-100|r|b" {
		t.Errorf("read after the expiry = %+v, %q, %v, want no wait and the marker", wait, marker, err)
	}

	corrupt := map[string]func(key string){
		"missing field":  func(key string) { env.redis.HDel(key, "m") },
		"unknown page":   func(key string) { env.redis.HSet(key, "p", "q") },
		"unknown part":   func(key string) { env.redis.HSet(key, "t", "z") },
		"non-numeric c":  func(key string) { env.redis.HSet(key, "c", "abc") },
		"positive c":     func(key string) { env.redis.HSet(key, "c", "100") },
		"bad nonce":      func(key string) { env.redis.HSet(key, "n", "nothex") },
		"bad expiry":     func(key string) { env.redis.HSet(key, "x", "soon") },
		"bad prompt ID":  func(key string) { env.redis.HSet(key, "m", "-3") },
		"unrelated only": func(key string) { env.redis.Del(key); env.redis.HSet(key, "k", "x") },
	}
	for name, corruptIt := range corrupt {
		t.Run(name, func(t *testing.T) {
			victim := settingsInputTestUser()
			victim.Id += 5_000
			settingsInputSeed(t, victim.Id, -100, "w", "x")
			corruptIt(settingsInputKey(victim.Id))
			wait, _, err := settingsInputRead(ctx, victim.Id)
			if err != nil || wait != nil {
				t.Errorf("read of a corrupt hash = %+v, %v, want no wait", wait, err)
			}
		})
	}
}

func TestSettingsInputAlbumAndMarker(t *testing.T) {
	env := newSettingsEnv(t)
	ctx := context.Background()
	user := settingsInputTestUser()

	t.Run("album memory is first caller only", func(t *testing.T) {
		first, err := settingsInputAlbumFirst(ctx, user.Id, "13579")
		if err != nil || !first {
			t.Fatalf("first album call = %v, %v, want true", first, err)
		}
		if ttl := env.redis.TTL(settingsInputAlbumKey(user.Id, "13579")); ttl != settingsInputAlbumTTL {
			t.Errorf("album TTL = %v, want %v", ttl, settingsInputAlbumTTL)
		}
		second, err := settingsInputAlbumFirst(ctx, user.Id, "13579")
		if err != nil || second {
			t.Errorf("second album call = %v, %v, want false", second, err)
		}
		other, err := settingsInputAlbumFirst(ctx, user.Id, "24680")
		if err != nil || !other {
			t.Errorf("another album = %v, %v, want true", other, err)
		}
	})

	t.Run("an album ID that is not 1 to 20 digits writes nothing", func(t *testing.T) {
		for _, id := range []string{"", "12a", "-5", strings.Repeat("9", 21), "1 2"} {
			got, err := settingsInputAlbumFirst(ctx, user.Id, id)
			if err != nil || got {
				t.Errorf("album %q = %v, %v, want false, nil", id, got, err)
			}
		}
		for _, key := range env.redis.Keys() {
			if strings.HasPrefix(key, settingsInputAlbumKeyPrefix+strconv64(user.Id)+":") &&
				key != settingsInputAlbumKey(user.Id, "13579") && key != settingsInputAlbumKey(user.Id, "24680") {
				t.Errorf("an invalid album ID wrote %q", key)
			}
		}
	})

	t.Run("the marker is taken once", func(t *testing.T) {
		settingsInputSeed(t, user.Id, -1002, "g", "m")
		marker, ok, err := settingsInputTakeMarker(ctx, user.Id)
		if err != nil || !ok || marker != "-1002|g|m" {
			t.Fatalf("first take = %q, %v, %v, want -1002|g|m", marker, ok, err)
		}
		marker, ok, err = settingsInputTakeMarker(ctx, user.Id)
		if err != nil || ok || marker != "" {
			t.Errorf("second take = %q, %v, %v, want nothing", marker, ok, err)
		}
	})

	t.Run("parse marker", func(t *testing.T) {
		groupID, page, part, ok := settingsInputParseMarker("-100|w|x")
		if !ok || groupID != -100 || page != "w" || part != "x" {
			t.Errorf("parse of -100|w|x = %d, %q, %q, %v", groupID, page, part, ok)
		}
		for _, bad := range []string{"x", "", "-100|q|x", "abc|w|x", "-100|w|z", "100|w|x", "-100|w", "-100|w|x|y"} {
			if _, _, _, ok := settingsInputParseMarker(bad); ok {
				t.Errorf("parse of %q accepted it", bad)
			}
		}
	})
}

func TestSettingsInputWithoutRedis(t *testing.T) {
	env := newSettingsEnv(t)
	ctx := context.Background()
	user := settingsInputTestUser()
	restore := cache.DisableRedisForTest()
	defer restore()

	checks := map[string]func() error{
		"set": func() error {
			_, err := settingsInputSet(ctx, user.Id, settingsInputTestWait(t, -100))
			return err
		},
		"read": func() error { _, _, err := settingsInputRead(ctx, user.Id); return err },
		"take marker": func() error {
			_, _, err := settingsInputTakeMarker(ctx, user.Id)
			return err
		},
		"claim":  func() error { _, _, err := settingsInputClaim(ctx, user.Id, "0123456789abcdef"); return err },
		"finish": func() error { return settingsInputFinish(ctx, user.Id, "0123456789abcdef") },
		"release": func() error {
			return settingsInputRelease(ctx, user.Id, "0123456789abcdef")
		},
		"cancel": func() error {
			_, err := settingsInputCancel(ctx, user.Id, -100, "w", "x")
			return err
		},
		"cancel any": func() error { _, err := settingsInputCancelAny(ctx, user.Id); return err },
		"album":      func() error { _, err := settingsInputAlbumFirst(ctx, user.Id, "12345"); return err },
	}
	for name, run := range checks {
		if err := run(); err != errSettingsInputUnavailable {
			t.Errorf("%s without Redis error = %v, want errSettingsInputUnavailable", name, err)
		}
	}

	env.sendPrivate(user, "/cancel")
	replies := env.settingsInputRepliesTo(user.Id)
	if len(replies) != 1 || !strings.Contains(replies[0], staffMarker("settings_edit_cancel_nothing")) {
		t.Errorf("/cancel without Redis replies = %v, want one settings_edit_cancel_nothing", replies)
	}
}

// strconv64 formats an ID for a key prefix.
func strconv64(id int64) string { return strconv.FormatInt(id, 10) }
