//go:build testtools

package eventmsg

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	utilscache "github.com/uasneppy/fuku_robot/fuku/utils/cache"
)

var chatSeq atomic.Int64

// freshChat returns a negative chat ID no other test uses and deletes its rows and cache key
// when the test ends.
func freshChat(t *testing.T) int64 {
	t.Helper()
	chatID := -time.Now().UnixNano() - chatSeq.Add(1)
	t.Cleanup(func() {
		if err := db.DB.Where("chat_id = ?", chatID).Delete(&models.ChatEventMessage{}).Error; err != nil {
			t.Errorf("delete the event messages of chat %d: %v", chatID, err)
		}
		cache.DeleteCache(CacheKey(chatID))
	})
	return chatID
}

// rowCount counts the stored rows of a chat.
func rowCount(t *testing.T, chatID int64) int64 {
	t.Helper()
	var n int64
	if err := db.DB.Model(&models.ChatEventMessage{}).Where("chat_id = ?", chatID).Count(&n).Error; err != nil {
		t.Fatalf("count the rows of chat %d: %v", chatID, err)
	}
	return n
}

// mustGet reads the chat's messages through the cache and returns one event's group message.
func mustGet(t *testing.T, chatID int64, event string) (Message, bool) {
	t.Helper()
	msgs, err := Get(context.Background(), chatID)
	if err != nil {
		t.Fatalf("Get(%d): %v", chatID, err)
	}
	return msgs.Lookup(event, DestGroup)
}

func TestEventMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("a chat with no row reads as an empty set and creates nothing", func(t *testing.T) {
		utilscache.SetupTestMemoryMarshaler(t)
		chatID := freshChat(t)
		for name, read := range map[string]func(context.Context, int64) (*Messages, error){"Get": Get, "GetFresh": GetFresh} {
			msgs, err := read(ctx, chatID)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if msgs == nil || msgs.Rows == nil || len(msgs.Rows) != 0 {
				t.Fatalf("%s = %+v, want a non-nil empty set", name, msgs)
			}
			if _, ok := msgs.Lookup(EventBan, DestGroup); ok {
				t.Errorf("%s: Lookup of an absent pair reports a row", name)
			}
		}
		if n := rowCount(t, chatID); n != 0 {
			t.Errorf("reading created %d rows", n)
		}
		var nilMsgs *Messages
		if _, ok := nilMsgs.Lookup(EventBan, DestGroup); ok {
			t.Errorf("Lookup on a nil set reports a row")
		}
	})

	t.Run("text, silent, media and buttons are written column by column", func(t *testing.T) {
		utilscache.SetupTestMemoryMarshaler(t)
		chatID := freshChat(t)
		// Prime the cache with the empty set, so every read below proves the write dropped the key.
		if _, ok := mustGet(t, chatID, EventBan); ok {
			t.Fatalf("a fresh chat has a ban row")
		}

		if err := SetText(ctx, chatID, EventBan, DestGroup, "<b>Hi</b>", 7); err != nil {
			t.Fatalf("SetText: %v", err)
		}
		cache.ResetLocalForTest()
		got, ok := mustGet(t, chatID, EventBan)
		if !ok || got.Text != "<b>Hi</b>" || got.FileID != "" || got.MediaType != 0 || len(got.Buttons) != 0 || got.Silent {
			t.Fatalf("after SetText = %+v, %v, want the text alone", got, ok)
		}
		if got.Event != EventBan || got.Dest != DestGroup {
			t.Errorf("after SetText the key reads %q/%q", got.Event, got.Dest)
		}

		if err := SetSilent(ctx, chatID, EventBan, DestGroup, true, 7); err != nil {
			t.Fatalf("SetSilent true: %v", err)
		}
		if got, _ := mustGet(t, chatID, EventBan); !got.Silent || got.Text != "<b>Hi</b>" {
			t.Fatalf("after SetSilent true = %+v, want silent and the text kept", got)
		}
		if err := SetText(ctx, chatID, EventBan, DestGroup, "<i>Bye</i>", 8); err != nil {
			t.Fatalf("SetText again: %v", err)
		}
		if got, _ := mustGet(t, chatID, EventBan); !got.Silent || got.Text != "<i>Bye</i>" {
			t.Fatalf("SetText clobbered silent: %+v", got)
		}
		if err := SetSilent(ctx, chatID, EventBan, DestGroup, false, 8); err != nil {
			t.Fatalf("SetSilent false: %v", err)
		}
		if got, _ := mustGet(t, chatID, EventBan); got.Silent || got.Text != "<i>Bye</i>" {
			t.Fatalf("after SetSilent false = %+v, want a Go false written", got)
		}

		if err := SetMedia(ctx, chatID, EventBan, DestGroup, "FILE-1", 2, 9); err != nil {
			t.Fatalf("SetMedia: %v", err)
		}
		if got, _ := mustGet(t, chatID, EventBan); got.FileID != "FILE-1" || got.MediaType != 2 || got.Text != "<i>Bye</i>" {
			t.Fatalf("after SetMedia = %+v, want the file and type together and the text kept", got)
		}

		buttons := []models.Button{{Name: "Rules", Url: "https://example.org/rules"}, {Name: "Site", Url: "https://example.org", SameLine: true}}
		if err := SetButtons(ctx, chatID, EventBan, DestGroup, buttons, 9); err != nil {
			t.Fatalf("SetButtons: %v", err)
		}
		got, _ = mustGet(t, chatID, EventBan)
		if len(got.Buttons) != 2 || got.Buttons[0] != buttons[0] || got.Buttons[1] != buttons[1] || got.FileID != "FILE-1" {
			t.Fatalf("after SetButtons = %+v, want both buttons and the media kept", got)
		}
		if err := SetButtons(ctx, chatID, EventBan, DestGroup, nil, 9); err != nil {
			t.Fatalf("SetButtons nil: %v", err)
		}
		if got, _ := mustGet(t, chatID, EventBan); len(got.Buttons) != 0 {
			t.Fatalf("after SetButtons nil = %+v, want no button", got)
		}
		var stored string
		if err := db.DB.Raw(`SELECT buttons FROM chat_event_messages WHERE chat_id = ? AND event = ?`, chatID, EventBan).Scan(&stored).Error; err != nil {
			t.Fatalf("read the stored buttons: %v", err)
		}
		if stored != "[]" {
			t.Errorf("stored buttons = %q, want []", stored)
		}

		if err := SetText(ctx, chatID, EventBan, DestGroup, "", 10); err != nil {
			t.Fatalf("SetText empty: %v", err)
		}
		if err := SetMedia(ctx, chatID, EventBan, DestGroup, "", 0, 10); err != nil {
			t.Fatalf("SetMedia empty: %v", err)
		}
		got, ok = mustGet(t, chatID, EventBan)
		if !ok || got.Text != "" || got.FileID != "" || got.MediaType != 0 || got.HasContent() || got.Silent {
			t.Fatalf("after removing every part = %+v, %v, want an empty row that reads as the default", got, ok)
		}
		if n := rowCount(t, chatID); n != 1 {
			t.Errorf("chat has %d rows, want the one row kept", n)
		}
		var by int64
		if err := db.DB.Raw(`SELECT updated_by FROM chat_event_messages WHERE chat_id = ?`, chatID).Scan(&by).Error; err != nil {
			t.Fatalf("read updated_by: %v", err)
		}
		if by != 10 {
			t.Errorf("updated_by = %d, want the last writer 10", by)
		}
	})

	t.Run("every write drops the cached read", func(t *testing.T) {
		utilscache.SetupTestMemoryMarshaler(t)
		chatID := freshChat(t)
		if err := SetText(ctx, chatID, EventKick, DestGroup, "cached", 1); err != nil {
			t.Fatal(err)
		}
		if got, _ := mustGet(t, chatID, EventKick); got.Text != "cached" {
			t.Fatalf("first read = %+v", got)
		}
		// A change behind the repository's back is not seen while the cached read stands.
		if err := db.DB.Exec(`UPDATE chat_event_messages SET text = 'behind' WHERE chat_id = ?`, chatID).Error; err != nil {
			t.Fatal(err)
		}
		if got, _ := mustGet(t, chatID, EventKick); got.Text != "cached" {
			t.Fatalf("read with a cached value = %+v, want the cached text (the cache is live)", got)
		}
		writers := []func() error{
			func() error { return SetSilent(ctx, chatID, EventKick, DestGroup, true, 2) },
			func() error { return SetMedia(ctx, chatID, EventKick, DestGroup, "F", 2, 2) },
			func() error { return SetButtons(ctx, chatID, EventKick, DestGroup, nil, 2) },
			func() error { return SetText(ctx, chatID, EventKick, DestGroup, "behind", 2) },
		}
		for i, write := range writers {
			if _, err := Get(ctx, chatID); err != nil {
				t.Fatal(err)
			}
			if err := db.DB.Exec(`UPDATE chat_event_messages SET text = ? WHERE chat_id = ?`, "behind", chatID).Error; err != nil {
				t.Fatal(err)
			}
			if err := write(); err != nil {
				t.Fatalf("writer %d: %v", i, err)
			}
			if got, _ := mustGet(t, chatID, EventKick); got.Text != "behind" {
				t.Errorf("writer %d left the cached read in place: %+v", i, got)
			}
			if err := db.DB.Exec(`UPDATE chat_event_messages SET text = 'cached' WHERE chat_id = ?`, chatID).Error; err != nil {
				t.Fatal(err)
			}
			cache.DeleteCache(CacheKey(chatID))
		}
	})

	t.Run("the event and destination tables", func(t *testing.T) {
		want := []string{"ban", "tban", "kick", "mute", "tmute", "unban", "unmute"}
		if len(Events) != len(want) {
			t.Fatalf("Events = %v, want %v", Events, want)
		}
		for i, event := range want {
			if Events[i] != event || !ValidEvent(event) {
				t.Errorf("event %d: Events has %q, ValidEvent(%q) = %v", i, Events[i], event, ValidEvent(event))
			}
		}
		for _, event := range []string{"", "Ban", "warn", "ban ", "dban", "sban"} {
			if ValidEvent(event) {
				t.Errorf("ValidEvent(%q) = true", event)
			}
		}
		if len(Dests) != 1 || !ValidDest("group") {
			t.Errorf("Dests = %v, want exactly group", Dests)
		}
		for _, dest := range []string{"", "pm", "Group", "log"} {
			if ValidDest(dest) {
				t.Errorf("ValidDest(%q) = true", dest)
			}
		}
	})

	t.Run("an unknown event or destination writes nothing", func(t *testing.T) {
		chatID := freshChat(t)
		writers := map[string]func(event, dest string) error{
			"SetText":  func(e, d string) error { return SetText(ctx, chatID, e, d, "x", 1) },
			"SetMedia": func(e, d string) error { return SetMedia(ctx, chatID, e, d, "F", 1, 1) },
			"SetButtons": func(e, d string) error {
				return SetButtons(ctx, chatID, e, d, []models.Button{{Name: "a", Url: "https://a.b"}}, 1)
			},
			"SetSilent": func(e, d string) error { return SetSilent(ctx, chatID, e, d, true, 1) },
		}
		for name, write := range writers {
			if err := write("warn", DestGroup); !errors.Is(err, ErrUnknownEvent) {
				t.Errorf("%s with an unknown event = %v, want ErrUnknownEvent", name, err)
			}
			if err := write(EventBan, "pm"); !errors.Is(err, ErrUnknownDest) {
				t.Errorf("%s with an unknown destination = %v, want ErrUnknownDest", name, err)
			}
		}
		if n := rowCount(t, chatID); n != 0 {
			t.Errorf("refused writes created %d rows", n)
		}
	})

	t.Run("HasContent", func(t *testing.T) {
		cases := []struct {
			msg  Message
			want bool
		}{
			{Message{}, false},
			{Message{Silent: true, MediaType: 2}, false},
			{Message{Text: "t"}, true},
			{Message{FileID: "f"}, true},
			{Message{Buttons: []models.Button{{Name: "a", Url: "https://a.b"}}}, true},
		}
		for _, tc := range cases {
			if got := tc.msg.HasContent(); got != tc.want {
				t.Errorf("HasContent(%+v) = %v, want %v", tc.msg, got, tc.want)
			}
		}
	})
}

func TestEventMessagesUpsert(t *testing.T) {
	ctx := context.Background()
	chatA := freshChat(t)
	chatB := freshChat(t)

	// Two writers of different columns of one key, in both orders, leave both values.
	if err := SetText(ctx, chatA, EventBan, DestGroup, "A ban", 1); err != nil {
		t.Fatal(err)
	}
	if err := SetSilent(ctx, chatA, EventBan, DestGroup, true, 2); err != nil {
		t.Fatal(err)
	}
	if err := SetButtons(ctx, chatA, EventMute, DestGroup, []models.Button{{Name: "x", Url: "https://x.y"}}, 3); err != nil {
		t.Fatal(err)
	}
	if err := SetMedia(ctx, chatA, EventMute, DestGroup, "MUTE-FILE", 3, 4); err != nil {
		t.Fatal(err)
	}
	if err := SetText(ctx, chatB, EventBan, DestGroup, "B ban", 5); err != nil {
		t.Fatal(err)
	}

	msgsA, err := GetFresh(ctx, chatA)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgsA.Rows) != 2 {
		t.Fatalf("chat A has %d rows, want 2 (ban and mute)", len(msgsA.Rows))
	}
	if ban, _ := msgsA.Lookup(EventBan, DestGroup); ban.Text != "A ban" || !ban.Silent || !ban.HasContent() {
		t.Errorf("chat A ban = %+v, want the text and silent both kept", ban)
	}
	if mute, _ := msgsA.Lookup(EventMute, DestGroup); mute.FileID != "MUTE-FILE" || mute.MediaType != 3 || len(mute.Buttons) != 1 || mute.Text != "" {
		t.Errorf("chat A mute = %+v, want the media and the button both kept", mute)
	}
	msgsB, err := GetFresh(ctx, chatB)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgsB.Rows) != 1 {
		t.Fatalf("chat B has %d rows, want 1", len(msgsB.Rows))
	}
	if ban, _ := msgsB.Lookup(EventBan, DestGroup); ban.Text != "B ban" || ban.Silent {
		t.Errorf("chat B ban = %+v, want its own text and no silent", ban)
	}
	if _, ok := msgsB.Lookup(EventMute, DestGroup); ok {
		t.Errorf("chat B has chat A's mute row")
	}

	// Repeated writes of one key keep one row.
	for i := range 3 {
		if err := SetText(ctx, chatA, EventBan, DestGroup, "A ban", int64(10+i)); err != nil {
			t.Fatal(err)
		}
	}
	var n int64
	if err := db.DB.Model(&models.ChatEventMessage{}).
		Where("chat_id = ? AND event = ? AND dest = ?", chatA, EventBan, DestGroup).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("chat A has %d ban rows, want 1", n)
	}
}
