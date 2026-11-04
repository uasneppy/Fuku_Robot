//go:build testtools

package modules

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
)

// settingsMediaItemID builds a private message carrying one item of the given kind with fileID
// as its Telegram file ID. A photo has three sizes and the last one is fileID, an animation
// carries the document Telegram sets with it, and the kinds the editor refuses (a video note,
// a contact, a poll, a location) are there too. Kind "text" is a plain text message.
func settingsMediaItemID(kind, fileID string) gotgbot.Message {
	switch kind {
	case "photo":
		return gotgbot.Message{Photo: []gotgbot.PhotoSize{
			{FileId: fileID + "-s", FileUniqueId: fileID + "-su", Width: 1, Height: 1},
			{FileId: fileID + "-m", FileUniqueId: fileID + "-mu", Width: 2, Height: 2},
			{FileId: fileID, FileUniqueId: fileID + "-u", Width: 3, Height: 3},
		}}
	case "video":
		return gotgbot.Message{Video: &gotgbot.Video{FileId: fileID, FileUniqueId: fileID + "-u", Width: 1, Height: 1, Duration: 1}}
	case "animation":
		return gotgbot.Message{
			Animation: &gotgbot.Animation{FileId: fileID, FileUniqueId: fileID + "-u", Width: 1, Height: 1, Duration: 1},
			Document:  &gotgbot.Document{FileId: fileID, FileUniqueId: fileID + "-u"},
		}
	case "document":
		return gotgbot.Message{Document: &gotgbot.Document{FileId: fileID, FileUniqueId: fileID + "-u"}}
	case "sticker":
		return gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: fileID, FileUniqueId: fileID + "-u", Type: "regular", Width: 1, Height: 1}}
	case "audio":
		return gotgbot.Message{Audio: &gotgbot.Audio{FileId: fileID, FileUniqueId: fileID + "-u", Duration: 1}}
	case "voice":
		return gotgbot.Message{Voice: &gotgbot.Voice{FileId: fileID, FileUniqueId: fileID + "-u", Duration: 1}}
	case "videonote":
		return gotgbot.Message{VideoNote: &gotgbot.VideoNote{FileId: fileID, FileUniqueId: fileID + "-u", Length: 1, Duration: 1}}
	case "contact":
		return gotgbot.Message{Contact: &gotgbot.Contact{PhoneNumber: "+10000000000", FirstName: "Someone"}}
	case "poll":
		return gotgbot.Message{Poll: &gotgbot.Poll{Id: "poll-1", Question: "Question?"}}
	case "location":
		return gotgbot.Message{Location: &gotgbot.Location{Latitude: 1, Longitude: 2}}
	}
	return gotgbot.Message{Text: "just a text"}
}

// settingsMediaItem is settingsMediaItemID with the file ID "<kind>-file".
func settingsMediaItem(kind string) gotgbot.Message {
	return settingsMediaItemID(kind, kind+"-file")
}

// sendPrivateMedia sends one item of kind as a private message of from to the bot through the
// real dispatcher and returns the message it sent.
func sendPrivateMedia(env *settingsEnv, from gotgbot.User, kind string) gotgbot.Message {
	env.t.Helper()
	msg := settingsMediaItem(kind)
	sendPrivateMessage(env, from, msg)
	return msg
}

// sendPrivateAlbum sends items as one album (one media group ID) of from to the bot, one after
// another or, when concurrent is set, from one goroutine each (Telegram delivers the items as
// separate updates). It returns the media group ID.
func sendPrivateAlbum(env *settingsEnv, from gotgbot.User, items []gotgbot.Message, concurrent bool) string {
	env.t.Helper()
	groupID := fmt.Sprintf("%d", 9_000_000+env.updateID())
	updates := make([]*gotgbot.Update, 0, len(items))
	for _, item := range items {
		item.MediaGroupId = groupID
		updates = append(updates, privateUpdate(env, from, item))
	}
	process := func(update *gotgbot.Update) {
		if err := env.dispatcher.ProcessUpdate(env.bot, update, nil); err != nil {
			env.t.Errorf("ProcessUpdate(%d) error = %v", update.UpdateId, err)
		}
	}
	if !concurrent {
		for _, update := range updates {
			process(update)
		}
		return groupID
	}
	var wg sync.WaitGroup
	for _, update := range updates {
		wg.Add(1)
		go func() {
			defer wg.Done()
			process(update)
		}()
	}
	wg.Wait()
	return groupID
}

// settingsMediaAlbumOf builds n photos with the file IDs "album-1" to "album-n".
func settingsMediaAlbumOf(n int) []gotgbot.Message {
	items := make([]gotgbot.Message, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, settingsMediaItemID("photo", fmt.Sprintf("album-%d", i)))
	}
	return items
}

// settingsEditPair is a command chat and an editor chat of one test, each with its own creator:
// the command runs in the first and the editor in the second, and a parity test compares what
// the two leave stored.
type settingsEditPair struct {
	command, editor           gotgbot.Chat
	commandOwner, editorOwner gotgbot.User
}

// newSettingsEditPair adds the two chats and their owners to env, with the greetings and rules
// rows of both cleaned before and after the test.
func newSettingsEditPair(env *settingsEnv) settingsEditPair {
	env.t.Helper()
	command, editor := env.addChat(), env.addChat()
	editCleanup(env.t, command.Id, editor.Id)
	return settingsEditPair{
		command: command, editor: editor,
		commandOwner: env.newOwner(command), editorOwner: env.newOwner(editor),
	}
}

// onEditor runs fn with the env's group set to the editor chat, which the settings helpers
// (settingsEditSetWait and the rest) address.
func (p settingsEditPair) onEditor(env *settingsEnv, fn func()) {
	previous := env.chat
	env.chat = p.editor
	defer func() { env.chat = previous }()
	fn()
}

// settingsEditCommandName is the command that sets the page's message.
func settingsEditCommandName(page string) string {
	if page == settingsPageGoodbye {
		return "/setgoodbye"
	}
	return "/setwelcome"
}

// settingsEditWelcomeOf returns the part of the greetings row a page shows: its text, file ID,
// type and buttons.
func settingsEditWelcomeOf(t *testing.T, chatID int64, page string) (text, fileID string, dataType int, buttons []db.Button) {
	t.Helper()
	row, ok := settingsEditStored(t, chatID)
	if !ok {
		t.Fatalf("chat %d has no greetings row", chatID)
	}
	if page == settingsPageGoodbye {
		g := row.GoodbyeSettings
		return g.GoodbyeText, g.FileID, g.GoodbyeType, g.Button
	}
	w := row.WelcomeSettings
	return w.WelcomeText, w.FileID, w.WelcomeType, w.Button
}

// settingsEditCommandReplyTo runs the page's command in the command chat as a reply to msg.
func settingsEditCommandReplyTo(env *settingsEnv, p settingsEditPair, page string, msg gotgbot.Message) {
	env.t.Helper()
	command := settingsEditCommandName(page)
	sendGroupMessage(env, p.command, p.commandOwner, gotgbot.Message{
		Text:           command,
		Entities:       []gotgbot.MessageEntity{{Type: "bot_command", Offset: 0, Length: int64(len(command))}},
		ReplyToMessage: &msg,
	})
}

// settingsMediaCountingWrite wraps the Welcome media write so that its calls are counted, and
// restores the real write when the test ends.
func settingsMediaCountingWrite(t *testing.T) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	previous := greetingWrites.WelcomeMediaOnly
	greetingWrites.WelcomeMediaOnly = func(chatID int64, fileID string, dataType int) error {
		calls.Add(1)
		return previous(chatID, fileID, dataType)
	}
	t.Cleanup(func() { greetingWrites.WelcomeMediaOnly = previous })
	return &calls
}

// settingsEditAlbumKeys lists the album memory keys in Redis.
func settingsEditAlbumKeys(env *settingsEnv) []string {
	var found []string
	for _, key := range settingsEditSettingsKeys(env) {
		if strings.HasPrefix(key, "fuku:settings:inputalbum:") {
			found = append(found, key)
		}
	}
	return found
}

// settingsEditPageText is the text of the newest message the bot sent to user.
func settingsEditPageText(env *settingsEnv, user gotgbot.User) string {
	sent := env.fake.sentTo(user.Id)
	if len(sent) == 0 {
		return ""
	}
	return fmt.Sprint(sent[len(sent)-1].Params["text"])
}

func TestSettingsEditMediaParity(t *testing.T) {
	kinds := []string{"photo", "video", "animation", "document", "sticker", "audio", "voice"}
	for _, page := range []string{settingsPageWelcome, settingsPageGoodbye} {
		for _, kind := range kinds {
			t.Run(page+"/"+kind, func(t *testing.T) {
				env := newSettingsEditEnv(t)
				pair := newSettingsEditPair(env)

				settingsEditCommandReplyTo(env, pair, page, settingsMediaItem(kind))
				_, commandFile, commandType, _ := settingsEditWelcomeOf(t, pair.command.Id, page)
				if commandFile == "" {
					t.Fatalf("%s as a reply to a %s stored no media", settingsEditCommandName(page), kind)
				}

				pair.onEditor(env, func() {
					settingsEditSetWait(env, pair.editorOwner, page, settingsEditPartMedia)
					sendPrivateMedia(env, pair.editorOwner, kind)
				})
				_, editorFile, editorType, _ := settingsEditWelcomeOf(t, pair.editor.Id, page)
				if editorFile != commandFile || editorType != commandType {
					t.Errorf("the editor stored file %q type %d, the command stored file %q type %d",
						editorFile, editorType, commandFile, commandType)
				}
				if kind == "animation" && editorType != db.DOCUMENT {
					t.Errorf("a GIF is stored as type %d, want the document type %d", editorType, db.DOCUMENT)
				}
				settingsEditWaitGone(t, env, pair.editorOwner)
			})
		}
	}
}

func TestSettingsEditCaptionIgnored(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
	before := settingsEditSentCount(env, owner)

	photo := settingsMediaItemID("photo", "photo-big")
	photo.Caption = "SECRETCAPTION"
	sendPrivateMessage(env, owner, photo)

	row, _ := settingsEditStored(t, env.chat.Id)
	if row.WelcomeSettings.FileID != "photo-big" || row.WelcomeSettings.WelcomeType != db.PHOTO {
		t.Errorf("the welcome media is file %q type %d, want photo-big as a photo",
			row.WelcomeSettings.FileID, row.WelcomeSettings.WelcomeType)
	}
	if row.WelcomeSettings.WelcomeText != "a custom welcome" || len(row.WelcomeSettings.Button) != 2 {
		t.Errorf("the text or buttons changed: %+v", row.WelcomeSettings)
	}
	settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_caption_ignored")
	if strings.Contains(settingsEditPageText(env, owner), "SECRETCAPTION") {
		t.Errorf("the saved page echoes the caption: %q", settingsEditPageText(env, owner))
	}
}

func TestSettingsEditMediaKindsDoor(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
	for _, kind := range []string{"videonote", "contact", "poll", "location", "text"} {
		t.Run(kind, func(t *testing.T) {
			before := settingsEditSentCount(env, owner)

			sendPrivateMedia(env, owner, kind)

			settingsEditWantReply(t, env, owner, before, "settings_edit_refuse_need_media", "settings_edit_still_waiting")
			settingsEditWaitKept(t, env, owner)
			if row, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Errorf("a refused %s stored %+v", kind, row)
			}
		})
	}
}

func TestSettingsEditMediaTextLongDoor(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	if err := greetings.SetWelcomeText(env.chat.Id, strings.Repeat("x", 1100), "", nil, db.TEXT); err != nil {
		t.Fatalf("SetWelcomeText: %v", err)
	}
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
	before := settingsEditSentCount(env, owner)

	sendPrivateMedia(env, owner, "photo")

	settingsEditWantReply(t, env, owner, before, "settings_edit_refuse_media_text_long", "settings_edit_still_waiting")
	settingsEditWaitKept(t, env, owner)
	if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.FileID != "" {
		t.Errorf("a refused photo stored file %q", row.WelcomeSettings.FileID)
	}
}

func TestSettingsEditAlbumWritesOnce(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
	writes := settingsMediaCountingWrite(t)
	before := settingsEditSentCount(env, owner)

	groupID := sendPrivateAlbum(env, owner, settingsMediaAlbumOf(8), true)

	if got := writes.Load(); got != 1 {
		t.Errorf("the album wrote the media %d times, want once", got)
	}
	// Which item is kept is whichever won the album memory: the test asserts one write and one
	// page, never which item.
	row, _ := settingsEditStored(t, env.chat.Id)
	if !strings.HasPrefix(row.WelcomeSettings.FileID, "album-") || row.WelcomeSettings.WelcomeType != db.PHOTO {
		t.Errorf("the welcome media is file %q type %d, want one of the album's photos", row.WelcomeSettings.FileID, row.WelcomeSettings.WelcomeType)
	}
	settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_album_cut")
	if !env.redis.Exists(settingsInputAlbumKey(owner.Id, groupID)) {
		t.Errorf("the album memory %s is missing", settingsInputAlbumKey(owner.Id, groupID))
	}
	settingsEditWaitGone(t, env, owner)
}

func TestSettingsEditAlbumNotice(t *testing.T) {
	t.Run("an album keeps one item and says so", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSeedGreetings(t, env.chat.Id)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
		before := settingsEditSentCount(env, owner)

		sendPrivateAlbum(env, owner, settingsMediaAlbumOf(3), false)

		settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_album_cut")
		if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.FileID != "album-1" {
			t.Errorf("the first item of a sequential album is kept, got file %q", row.WelcomeSettings.FileID)
		}
	})

	t.Run("a media group ID that is not digits is one item", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		owner := env.newOwner(env.chat)
		settingsEditSeedGreetings(t, env.chat.Id)
		settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
		before := settingsEditSentCount(env, owner)

		photo := settingsMediaItemID("photo", "single-photo")
		photo.MediaGroupId = "abc"
		sendPrivateMessage(env, owner, photo)

		settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media")
		if strings.Contains(settingsEditPageText(env, owner), staffMarker("settings_edit_album_cut")) {
			t.Errorf("a single item says an album was cut: %q", settingsEditPageText(env, owner))
		}
		if keys := settingsEditAlbumKeys(env); len(keys) != 0 {
			t.Errorf("a media group ID of \"abc\" wrote the album keys %v", keys)
		}
		if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.FileID != "single-photo" {
			t.Errorf("the welcome media is file %q, want single-photo", row.WelcomeSettings.FileID)
		}
	})
}

func TestSettingsEditRefusedAlbumOneRefusal(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprintf("concurrent=%v", concurrent), func(t *testing.T) {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartText)
			before := settingsEditSentCount(env, owner)

			sendPrivateAlbum(env, owner, settingsMediaAlbumOf(3), concurrent)

			settingsEditWantReply(t, env, owner, before, "settings_edit_refuse_need_text", "settings_edit_still_waiting")
			settingsEditWaitKept(t, env, owner)
			if row, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Errorf("a refused album stored %+v", row)
			}
		})
	}
}

func TestSettingsEditExpiredAlbumOneReply(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprintf("concurrent=%v", concurrent), func(t *testing.T) {
			env := newSettingsEditEnv(t)
			owner := env.newOwner(env.chat)
			settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
			env.redis.FastForward(settingsInputTTL + time.Second)
			before := settingsEditSentCount(env, owner)

			sendPrivateAlbum(env, owner, settingsMediaAlbumOf(3), concurrent)

			settingsEditWantReply(t, env, owner, before, "settings_edit_wait_ended_note", "settings_edit_part_media")
			if row, ok := settingsEditStored(t, env.chat.Id); ok {
				t.Errorf("a late album stored %+v", row)
			}
			if keys := settingsEditAlbumKeys(env); len(keys) != 0 {
				t.Errorf("a late album wrote the album keys %v", keys)
			}
		})
	}
}

func TestSettingsEditStickerNote(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartMedia)
	before := settingsEditSentCount(env, owner)

	sendPrivateMedia(env, owner, "sticker")

	settingsEditWantReply(t, env, owner, before, "settings_edit_saved_media", "settings_edit_note_no_caption")
	if row, _ := settingsEditStored(t, env.chat.Id); row.WelcomeSettings.WelcomeType != db.STICKER {
		t.Errorf("the welcome media type is %d, want the sticker type %d", row.WelcomeSettings.WelcomeType, db.STICKER)
	}
}

// settingsMediaButtonLines are three button lines in the bot's syntax: a named link, a named link
// on the same row as the one before and a link with no name, which the commands label Greeting.
const settingsMediaButtonLines = "[Rules](buttonurl://https://example.com/rules)\n" +
	"[Chat](buttonurl://https://t.me/example:same)\n" +
	"[](buttonurl://https://example.com/nameless)"

func TestSettingsEditButtonsParity(t *testing.T) {
	for _, page := range []string{settingsPageWelcome, settingsPageGoodbye} {
		t.Run(page, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			pair := newSettingsEditPair(env)
			sendGroupMessage(env, pair.command, pair.commandOwner, gotgbot.Message{
				Text: settingsEditCommandName(page) + " hello\n" + settingsMediaButtonLines,
			})
			_, _, _, commandButtons := settingsEditWelcomeOf(t, pair.command.Id, page)
			if len(commandButtons) != 3 {
				t.Fatalf("%s stored %d buttons, want 3: %+v", settingsEditCommandName(page), len(commandButtons), commandButtons)
			}
			if commandButtons[2].Name != settingsEditDefaultButtonName || !commandButtons[1].SameLine {
				t.Fatalf("the command stored %+v, want the default label and the same-row mark", commandButtons)
			}

			// The editor replaces a stored set of two buttons.
			settingsEditSeedGreetings(t, pair.editor.Id)
			pair.onEditor(env, func() {
				settingsEditSetWait(env, pair.editorOwner, page, settingsEditPartButtons)
				sendPrivateMessage(env, pair.editorOwner, gotgbot.Message{Text: settingsMediaButtonLines})
			})

			_, _, _, editorButtons := settingsEditWelcomeOf(t, pair.editor.Id, page)
			if !reflect.DeepEqual(editorButtons, commandButtons) {
				t.Errorf("the editor stored %+v, the command stored %+v", editorButtons, commandButtons)
			}
			settingsEditWaitGone(t, env, pair.editorOwner)
		})
	}
}

func TestSettingsEditButtonRefusalsDoor(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"a link that is not http or https", "[a](buttonurl://ftp://e.com)", "settings_edit_refuse_button_url"},
		{"no button line", "no buttons", "settings_edit_refuse_buttons_syntax"},
		{"101 buttons", settingsEditButtonLines(101, 1000), "settings_edit_refuse_buttons_limit"},
		{"9 buttons in a row", settingsEditButtonLines(9, 1), "settings_edit_refuse_buttons_limit"},
	}
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsEditSeedGreetings(t, env.chat.Id)
	settingsEditSetWait(env, owner, settingsPageWelcome, settingsEditPartButtons)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := settingsEditSentCount(env, owner)

			sendPrivateMessage(env, owner, gotgbot.Message{Text: tc.text})

			settingsEditWantReply(t, env, owner, before, tc.want, "settings_edit_still_waiting")
			settingsEditWaitKept(t, env, owner)
			if row, _ := settingsEditStored(t, env.chat.Id); len(row.WelcomeSettings.Button) != 2 {
				t.Errorf("a refused button message changed the buttons: %+v", row.WelcomeSettings.Button)
			}
		})
	}
}

func TestSettingsEditWholeMessageParity(t *testing.T) {
	const (
		typed    = "Hi there {first}"
		bulleted = typed + "\n" + settingsMediaButtonLines
	)
	bold := []gotgbot.MessageEntity{{Type: "bold", Offset: 3, Length: 5}}

	for _, page := range []string{settingsPageWelcome, settingsPageGoodbye} {
		t.Run(page, func(t *testing.T) {
			env := newSettingsEditEnv(t)
			pair := newSettingsEditPair(env)

			// The editor: the text, then a photo, then the button lines, each on its own.
			var photo gotgbot.Message
			pair.onEditor(env, func() {
				owner := pair.editorOwner
				settingsEditSetWait(env, owner, page, settingsEditPartText)
				sendPrivateMessage(env, owner, gotgbot.Message{Text: typed, Entities: bold})
				settingsEditPressOnSavedPage(env, owner, "settings_btn_edit_set_media")
				photo = sendPrivateMedia(env, owner, "photo")
				settingsEditPressOnSavedPage(env, owner, "settings_btn_edit_set_buttons")
				sendPrivateMessage(env, owner, gotgbot.Message{Text: settingsMediaButtonLines})
			})

			// The command: one reply to the same photo whose caption holds the text and the lines.
			photo.Caption = bulleted
			photo.CaptionEntities = bold
			settingsEditCommandReplyTo(env, pair, page, photo)

			wantText, wantFile, wantType, wantButtons := settingsEditWelcomeOf(t, pair.command.Id, page)
			gotText, gotFile, gotType, gotButtons := settingsEditWelcomeOf(t, pair.editor.Id, page)
			if gotText != wantText || gotFile != wantFile || gotType != wantType {
				t.Errorf("the editor stored text %q file %q type %d, the command stored text %q file %q type %d",
					gotText, gotFile, gotType, wantText, wantFile, wantType)
			}
			if !reflect.DeepEqual(gotButtons, wantButtons) {
				t.Errorf("the editor stored buttons %+v, the command stored %+v", gotButtons, wantButtons)
			}
			if wantFile == "" || len(wantButtons) != 3 {
				t.Errorf("the command stored file %q and %d buttons, want the photo and 3 buttons", wantFile, len(wantButtons))
			}
		})
	}

	t.Run("rules", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		pair := newSettingsEditPair(env)
		sendGroupMessage(env, pair.command, pair.commandOwner, gotgbot.Message{
			Text: "/setrules " + typed, Entities: shiftEntities(bold, "/setrules "),
		})
		commandRow, ok := settingsEditRulesStored(t, pair.command.Id)
		if !ok {
			t.Fatal("/setrules stored nothing")
		}

		pair.onEditor(env, func() {
			settingsEditSetWait(env, pair.editorOwner, settingsPageRules, settingsEditPartText)
			sendPrivateMessage(env, pair.editorOwner, gotgbot.Message{Text: typed, Entities: bold})
		})

		editorRow, ok := settingsEditRulesStored(t, pair.editor.Id)
		if !ok || editorRow.Rules != commandRow.Rules {
			t.Errorf("the editor stored rules %q (row %v), the command stored %q", editorRow.Rules, ok, commandRow.Rules)
		}
	})
}
