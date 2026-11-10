//go:build testtools

package modules

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
)

// TestDeletionEffectiveRule pins what a stored rule means for a message: the delay it gets and
// whether a rule exists at all. The subtests are the cases of the kinds that do not share the
// /cleanservice flag and, from the third task of plan 16-02, of the two that do.
func TestDeletionEffectiveRule(t *testing.T) {
	t.Run("stored kinds", func(t *testing.T) {
		rules := &deletion.Rules{Delays: map[string]int{deletion.KindPinned: 600, deletion.KindTitle: 0, deletion.KindBotRules: 3600}}
		for _, c := range []struct {
			name  string
			kind  string
			delay int
			on    bool
		}{
			{"a stored delay", deletion.KindPinned, 600, true},
			{"a stored zero is on with delay zero", deletion.KindTitle, 0, true},
			{"a bot kind", deletion.KindBotRules, 3600, true},
			{"a service kind with no row", deletion.KindPhoto, 0, false},
			{"a bot kind with no row", deletion.KindBotWelcome, 0, false},
		} {
			// The flag argument is irrelevant to a kind that does not share it.
			for _, flag := range []bool{false, true} {
				delay, on := deletionEffectiveDelay(c.kind, rules, flag)
				if delay != c.delay || on != c.on {
					t.Errorf("%s (flag %v): got (%d, %v), want (%d, %v)", c.name, flag, delay, on, c.delay, c.on)
				}
			}
		}
		if delay, on := deletionEffectiveDelay(deletion.KindPinned, nil, true); delay != 0 || on {
			t.Errorf("nil rules: got (%d, %v), want no rule", delay, on)
		}
	})

	t.Run("a pinned notice gets the stored delay", func(t *testing.T) {
		msg := &gotgbot.Message{PinnedMessage: gotgbot.Message{MessageId: 7}}
		row, ok := deletionServiceKindOf(msg)
		if !ok || row.Kind != deletion.KindPinned {
			t.Fatalf("a message holding a pinned message routed to %+v (%v), want %s", row, ok, deletion.KindPinned)
		}
	})
}

// deletionServiceFields is the expected row code of every field of gotgbot.Message that is a
// deletable service notice. Written out here, not read from the table under test.
var deletionServiceFields = map[string]string{
	"NewChatMembers": "sj", "LeftChatMember": "sl",
	"NewChatPhoto": "sp", "DeleteChatPhoto": "sp",
	"NewChatTitle":       "sh",
	"PinnedMessage":      "sn",
	"VideoChatScheduled": "sv", "VideoChatStarted": "sv", "VideoChatEnded": "sv", "VideoChatParticipantsInvited": "sv",
	"ForumTopicEdited": "sf", "ForumTopicClosed": "sf", "ForumTopicReopened": "sf",
	"GeneralForumTopicHidden": "sf", "GeneralForumTopicUnhidden": "sf",
	"BoostAdded":                    "sb",
	"MessageAutoDeleteTimerChanged": "sa",
	"ChatBackgroundSet":             "sk",
	"GiveawayCreated":               "sg", "GiveawayWinners": "sg", "GiveawayCompleted": "sg",
	"Gift": "sg", "UniqueGift": "sg", "GiftUpgradeSent": "sg",
	"UsersShared": "so", "ChatShared": "so", "WriteAccessAllowed": "so", "WebAppData": "so",
	"ProximityAlertTriggered": "so", "ChecklistTasksDone": "so", "ChecklistTasksAdded": "so",
	"SuggestedPostApproved": "so", "SuggestedPostApprovalFailed": "so", "SuggestedPostDeclined": "so",
	"SuggestedPostPaid": "so", "SuggestedPostRefunded": "so",
	"SuccessfulPayment": "so", "RefundedPayment": "so", "PassportData": "so", "ConnectedWebsite": "so",
	"ChatOwnerLeft": "so", "ChatOwnerChanged": "so",
	"CommunityChatAdded": "so", "CommunityChatJoined": "so", "CommunityChatRemoved": "so",
	"DirectMessagePriceChanged": "so", "PaidMessagePriceChanged": "so", "ManagedBotCreated": "so",
	"PollOptionAdded": "so", "PollOptionDeleted": "so",
}

// deletionNeverDeletedFields are the fields that are service notices or posts but are never
// deleted and never offered: the creation and migration notices (the group's own history, and
// Telegram refuses to delete the creation of a topic), and the giveaway post, which is content.
var deletionNeverDeletedFields = []string{
	"GroupChatCreated", "SupergroupChatCreated", "ChannelChatCreated",
	"MigrateToChatId", "MigrateFromChatId", "ForumTopicCreated", "Giveaway",
}

// deletionContentFields is every other field of gotgbot.Message: metadata, content and media.
var deletionContentFields = []string{
	"MessageId", "MessageThreadId", "DirectMessagesTopic", "From", "SenderChat", "SenderBoostCount",
	"SenderBusinessBot", "SenderTag", "ReceiverUser", "EphemeralMessageId", "Date", "GuestQueryId",
	"BusinessConnectionId", "Chat", "ForwardOrigin", "IsTopicMessage", "IsAutomaticForward", "ReplyToMessage",
	"ExternalReply", "Quote", "ReplyToStory", "ReplyToChecklistTaskId", "ReplyToPollOptionId", "ViaBot",
	"GuestBotCallerUser", "GuestBotCallerChat", "EditDate", "HasProtectedContent", "IsFromOffline",
	"IsPaidPost", "MediaGroupId", "AuthorSignature", "PaidStarCount", "Text", "Entities", "LinkPreviewOptions",
	"SuggestedPostInfo", "EffectId", "RichMessage", "Animation", "Audio", "Document", "LivePhoto", "PaidMedia",
	"Photo", "Sticker", "Story", "Video", "VideoNote", "Voice", "Caption", "CaptionEntities",
	"ShowCaptionAboveMedia", "HasMediaSpoiler", "Checklist", "Contact", "Dice", "Game", "Poll", "Venue",
	"Location", "Invoice", "ReplyMarkup",
}

// deletionMessageWith builds a message with exactly the named fields set, each to a value its
// type calls "present": a pointer to a zero element, true, a non-empty string, 1, a one-element
// slice, a Message in an interface it satisfies. A struct or an interface nothing satisfies is
// left at its zero value, which is as present as that field can be made.
func deletionMessageWith(t *testing.T, fields ...string) *gotgbot.Message {
	t.Helper()
	msg := &gotgbot.Message{}
	root := reflect.ValueOf(msg).Elem()
	for _, name := range fields {
		field := root.FieldByName(name)
		if !field.IsValid() {
			t.Fatalf("gotgbot.Message has no field %q", name)
		}
		switch field.Kind() {
		case reflect.Ptr:
			field.Set(reflect.New(field.Type().Elem()))
		case reflect.Bool:
			field.SetBool(true)
		case reflect.String:
			field.SetString("x")
		case reflect.Int64, reflect.Int:
			field.SetInt(1)
		case reflect.Slice:
			field.Set(reflect.MakeSlice(field.Type(), 1, 1))
		case reflect.Interface:
			if reflect.TypeOf(gotgbot.Message{}).Implements(field.Type()) {
				field.Set(reflect.ValueOf(gotgbot.Message{}))
			}
		}
	}
	return msg
}

// TestServiceKindsCoverMessageFields is the guard for every gotgbot upgrade: each field of the
// pinned gotgbot.Message must be in exactly one of three lists (a deletable service notice with
// its row, never deleted, everything else). A field a gotgbot upgrade adds is in none of them and
// fails this test until someone classifies it, so a new service notice is never left undeleted
// and a new content field is never swept into a row by accident.
func TestServiceKindsCoverMessageFields(t *testing.T) {
	typ := reflect.TypeOf(gotgbot.Message{})
	seen := map[string]int{}
	for name := range deletionServiceFields {
		seen[name]++
	}
	for _, name := range deletionNeverDeletedFields {
		seen[name]++
	}
	for _, name := range deletionContentFields {
		seen[name]++
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		switch seen[name] {
		case 1:
		case 0:
			t.Errorf("field %s of gotgbot.Message is in none of the three lists: classify it as a deletable service notice, a never-deleted one or content", name)
		default:
			t.Errorf("field %s of gotgbot.Message is in %d lists, want exactly one", name, seen[name])
		}
		delete(seen, name)
	}
	for name := range seen {
		t.Errorf("%s is listed but is not a field of gotgbot.Message any more", name)
	}

	for name, code := range deletionServiceFields {
		row, ok := deletionServiceKindOf(deletionMessageWith(t, name))
		if !ok || row.Code != code {
			t.Errorf("a message with only %s routed to %q (%v), want row %q", name, row.Code, ok, code)
		}
	}
	for _, name := range append(slices.Clone(deletionNeverDeletedFields), deletionContentFields...) {
		if row, ok := deletionServiceKindOf(deletionMessageWith(t, name)); ok {
			t.Errorf("a message with only %s matched row %q, want no row", name, row.Code)
		}
	}
}

func TestServiceKindTable(t *testing.T) {
	if len(deletionServiceRows) != 12 || len(deletionBotRows) != 5 {
		t.Fatalf("tables hold %d service and %d bot rows, want 12 and 5", len(deletionServiceRows), len(deletionBotRows))
	}
	for _, row := range deletionServiceRows {
		var fields []string
		for name, code := range deletionServiceFields {
			if code == row.Code {
				fields = append(fields, name)
			}
		}
		if len(fields) == 0 {
			t.Errorf("row %s has no field in the expected map", row.Code)
		}
		for _, name := range fields {
			msg := deletionMessageWith(t, name)
			got, ok := deletionServiceKindOf(msg)
			if !ok || got.Code != row.Code {
				t.Errorf("row %s: a message with only %s routed to %q (%v)", row.Code, name, got.Code, ok)
			}
			for _, other := range deletionServiceRows {
				if other.Code != row.Code && other.Match(msg) {
					t.Errorf("row %s: %s is also matched by row %s", row.Code, name, other.Code)
				}
			}
		}
	}

	for _, c := range []struct {
		name string
		msg  *gotgbot.Message
	}{
		{"nil", nil},
		{"no field", &gotgbot.Message{}},
		{"a text message", &gotgbot.Message{Text: "hello"}},
		{"a photo message", &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "f"}}}},
		{"a group creation", deletionMessageWith(t, "GroupChatCreated")},
		{"a supergroup creation", deletionMessageWith(t, "SupergroupChatCreated")},
		{"a channel creation", deletionMessageWith(t, "ChannelChatCreated")},
		{"a migrate to", deletionMessageWith(t, "MigrateToChatId")},
		{"a migrate from", deletionMessageWith(t, "MigrateFromChatId")},
		{"a topic creation", deletionMessageWith(t, "ForumTopicCreated")},
		{"the giveaway post", deletionMessageWith(t, "Giveaway")},
	} {
		if row, ok := deletionServiceKindOf(c.msg); ok {
			t.Errorf("%s matched row %q, want no row", c.name, row.Code)
		}
	}

	// Two service fields: the first row in table order wins. The title row comes before the pinned one.
	if row, ok := deletionServiceKindOf(deletionMessageWith(t, "PinnedMessage", "NewChatTitle")); !ok || row.Code != "sh" {
		t.Errorf("pinned plus title routed to %q (%v), want the first row in table order, sh", row.Code, ok)
	}
}

func TestDeletionKindRegistryMatchesRepository(t *testing.T) {
	var kinds, codes []string
	for _, row := range append(slices.Clone(deletionServiceRows), deletionBotRows...) {
		kinds = append(kinds, row.Kind)
		codes = append(codes, row.Code)
		section := "s"
		if deletion.IsBotKind(row.Kind) {
			section = "b"
		}
		if len(row.Code) != 2 || !strings.HasPrefix(row.Code, section) {
			t.Errorf("row %q (%s): code must be two letters starting with %q", row.Code, row.Kind, section)
		}
		if row.NameKey == "" || row.DescKey == "" {
			t.Errorf("row %s has an empty name or description key", row.Code)
		}
		if row.HintKey != "" && row.HintKey != "settings_del_hint_cleanservice" && row.HintKey != "settings_del_hint_punish_auto" {
			t.Errorf("row %s has the unexpected hint key %q", row.Code, row.HintKey)
		}
		checkDeletionPresets(t, row)
	}
	registry := deletion.Kinds()
	slices.Sort(kinds)
	slices.Sort(registry)
	if !slices.Equal(kinds, registry) {
		t.Errorf("table kinds %v differ from the repository's %v", kinds, registry)
	}
	slices.Sort(codes)
	if len(slices.Compact(slices.Clone(codes))) != len(codes) {
		t.Errorf("codes are not unique: %v", codes)
	}
	for _, row := range deletionServiceRows {
		if row.Match == nil {
			t.Errorf("service row %s has no predicate", row.Code)
		}
		if !deletion.IsServiceKind(row.Kind) {
			t.Errorf("service row %s has the kind %s", row.Code, row.Kind)
		}
	}
	for _, row := range deletionBotRows {
		if row.Match != nil {
			t.Errorf("bot row %s has a predicate: bot messages are scheduled where they are sent", row.Code)
		}
		if slices.Contains(row.Presets, 0) || slices.Contains(row.Presets, 10) {
			t.Errorf("bot row %s offers a preset below a minute: %v", row.Code, row.Presets)
		}
	}
	for _, code := range []string{"sj", "sl"} {
		if row, _ := deletionRowOf(code); row.HintKey != "settings_del_hint_cleanservice" {
			t.Errorf("row %s hint = %q, want the /cleanservice hint", code, row.HintKey)
		}
	}
	if row, _ := deletionRowOf("ba"); row.HintKey != "settings_del_hint_punish_auto" {
		t.Errorf("row ba hint = %q, want the auto punishment hint", row.HintKey)
	}
}

// checkDeletionPresets holds a row's presets to D-04: ascending, within 0..86400, and a service
// list that starts with 0.
func checkDeletionPresets(t *testing.T, row deletionRow) {
	t.Helper()
	if len(row.Presets) == 0 {
		t.Errorf("row %s offers no preset", row.Code)
		return
	}
	if !slices.IsSorted(row.Presets) || len(slices.Compact(slices.Clone(row.Presets))) != len(row.Presets) {
		t.Errorf("row %s presets %v are not strictly ascending", row.Code, row.Presets)
	}
	if row.Presets[0] < 0 || row.Presets[len(row.Presets)-1] > deletion.MaxDelaySecs {
		t.Errorf("row %s presets %v leave 0..%d", row.Code, row.Presets, deletion.MaxDelaySecs)
	}
	if deletion.IsServiceKind(row.Kind) && row.Presets[0] != 0 {
		t.Errorf("service row %s presets %v do not start with 0", row.Code, row.Presets)
	}
}

func TestDeletionKindsNeverStaffLockdownCaptchaLog(t *testing.T) {
	registry := deletion.Kinds()
	if len(registry) != 17 {
		t.Fatalf("the repository registers %d kinds, want 17", len(registry))
	}
	bot := map[string]bool{
		deletion.KindBotWelcome: true, deletion.KindBotGoodbye: true, deletion.KindBotRules: true,
		deletion.KindBotAuto: true, deletion.KindBotManual: true,
	}
	botKinds := 0
	for _, kind := range registry {
		for _, word := range []string{"staff", "lockdown", "captcha", "log", "raid"} {
			if strings.Contains(kind, word) {
				t.Errorf("kind %q names %q", kind, word)
			}
		}
		if deletion.IsBotKind(kind) {
			botKinds++
			if !bot[kind] {
				t.Errorf("bot kind %q is not one of the five of D-11", kind)
			}
		}
	}
	if botKinds != 5 || len(deletionBotRows) != 5 {
		t.Errorf("%d bot kinds and %d bot rows, want exactly five of each", botKinds, len(deletionBotRows))
	}
	for _, row := range deletionBotRows {
		if !bot[row.Kind] {
			t.Errorf("bot row %s has the kind %s", row.Code, row.Kind)
		}
	}
}
