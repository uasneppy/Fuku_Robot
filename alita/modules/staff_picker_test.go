//go:build testtools

package modules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// pickerPayload is the start payload the Add group button carries for the env's
// Staff Group: "stf_" plus the decimal of the negated chat ID.
func (e *linkEnv) pickerPayload() string {
	return "stf_" + strconv.FormatInt(-e.staffID, 10)
}

// sendStart runs /start@AlitaTestBot <payload> in chat the way the group picker
// delivers it, through the real /start handler.
func (e *linkEnv) sendStart(t *testing.T, chat gotgbot.Chat, userID int64, payload string) {
	t.Helper()
	text := "/start@AlitaTestBot"
	if payload != "" {
		text += " " + payload
	}
	user := gotgbot.User{Id: userID, FirstName: "User"}
	ctx := newModuleMessageContext(e.bot, chat, user, text)
	if err := DefaultHelpRegistry().start(e.bot, ctx); err != ext.EndGroups {
		t.Fatalf("start returned %v, want ext.EndGroups", err)
	}
}

func TestStaffPickerLinksGroup(t *testing.T) {
	env := newLinkEnv(t)

	env.sendStart(t, env.groupChat("Picked Group"), env.ownerID, env.pickerPayload())

	deletes := callsToChat(env.client, "deleteMessage", env.groupID)
	if len(deletes) != 1 || fmt.Sprint(deletes[0].Params["message_id"]) != "101" {
		t.Fatalf("deleteMessage calls in the picked group = %+v, want one for the /start message", deletes)
	}
	env.wantLink(t, models.StaffHealthOK)
	env.wantInStaff(t, "staff_link_done")
}

func TestStaffPickerRefusesStranger(t *testing.T) {
	env := newLinkEnv(t)

	env.sendStart(t, env.groupChat("Picked Group"), linkStrangerID, env.pickerPayload())

	env.wantInPlace(t, "staff_link_refuse_not_staff_owner")
	if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
		t.Fatalf("%d messages were sent to the Staff Group, want none", len(sent))
	}
}

func TestStaffPickerRefusesAnonymous(t *testing.T) {
	env := newLinkEnv(t)
	ctx := anonymousLinkContext(env.bot, env.groupChat("Picked Group"), "/start@AlitaTestBot "+env.pickerPayload())

	if err := DefaultHelpRegistry().start(env.bot, ctx); err != ext.EndGroups {
		t.Fatalf("start returned %v, want ext.EndGroups", err)
	}

	env.wantInPlace(t, "staff_post_as_yourself")
	if env.lookups() != 0 {
		t.Fatalf("an anonymous picker message triggered %d lookups, want 0", env.lookups())
	}
}

// Anything that is not stf_ followed by digits keeps the old /start behaviour: one
// help_pm_questions reply, no deleted message, no link.
func TestStartGroupPayloadIgnoresJunk(t *testing.T) {
	payloads := []string{
		"stf_", "stf_abc", "stf_-5", "stf_12x", "stf_0x10",
		"stf_" + strings.Repeat("1", 20),
		"stf_" + strings.Repeat("1", 70),
		"foo",
	}
	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			env := newLinkEnv(t)

			env.sendStart(t, env.groupChat("Group"), env.ownerID, payload)

			texts := textsToChat(env.client, env.groupID)
			if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("help_pm_questions")) {
				t.Fatalf("replies = %q, want exactly the old help_pm_questions reply", texts)
			}
			if deletes := callsToChat(env.client, "deleteMessage", env.groupID); len(deletes) != 0 {
				t.Fatalf("deleteMessage calls = %+v, want none for an ignored payload", deletes)
			}
			if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
				t.Fatalf("%d messages were sent to the Staff Group, want none", len(sent))
			}
			env.wantNoLink(t)
		})
	}
}

func TestParseStaffPickerPayload(t *testing.T) {
	valid := map[string]int64{
		"stf_1":                   -1,
		"stf_1001234567890":       -1001234567890,
		"stf_9223372036854775807": -9223372036854775807,
	}
	for payload, want := range valid {
		if got, ok := parseStaffPickerPayload(payload); !ok || got != want {
			t.Errorf("parseStaffPickerPayload(%q) = (%d, %v), want (%d, true)", payload, got, ok, want)
		}
	}
	for _, payload := range []string{
		"", "stf_", "stf_0", "stf_-5", "stf_abc", "stf_12x", "stf_ 12", "STF_12", "stf12",
		"stf_9223372036854775808", "stf_" + strings.Repeat("9", 20),
	} {
		if got, ok := parseStaffPickerPayload(payload); ok {
			t.Errorf("parseStaffPickerPayload(%q) = (%d, true), want a rejection", payload, got)
		}
	}
}

func TestStaffAddGroupURLFitsTelegramPayloadRules(t *testing.T) {
	url := staffAddGroupURL("AlitaTestBot", -1001234567890)
	want := "https://t.me/AlitaTestBot?startgroup=stf_1001234567890&admin=restrict_members+delete_messages"
	if url != want {
		t.Fatalf("staffAddGroupURL = %q, want %q", url, want)
	}
	// The largest chat ID still yields a payload Telegram accepts.
	payload := strings.TrimPrefix(staffAddGroupURL("B", -9223372036854775807), "https://t.me/B?startgroup=")
	payload = strings.SplitN(payload, "&", 2)[0]
	if len(payload) > 64 || !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(payload) {
		t.Fatalf("payload %q breaks Telegram's start payload rules", payload)
	}
	if _, ok := parseStaffPickerPayload(payload); !ok {
		t.Fatalf("payload %q does not parse back", payload)
	}
}

// inlineKeyboardOf decodes the reply_markup of a recorded sendMessage call.
func inlineKeyboardOf(t *testing.T, call moduleBotCall) gotgbot.InlineKeyboardMarkup {
	t.Helper()
	raw := call.Params["reply_markup"]
	if raw == nil {
		t.Fatalf("call %+v carries no reply_markup", call.Params)
	}
	switch markup := raw.(type) {
	case gotgbot.InlineKeyboardMarkup:
		return markup
	case *gotgbot.InlineKeyboardMarkup:
		return *markup
	}
	var markup gotgbot.InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(fmt.Sprint(raw)), &markup); err != nil {
		t.Fatalf("decode reply_markup %v: %v", raw, err)
	}
	return markup
}

func TestStaffPanelHasAddGroupButton(t *testing.T) {
	env := newLinkEnv(t)
	owner := gotgbot.User{Id: env.ownerID, FirstName: "Owner"}
	ctx := newModuleMessageContext(env.bot, staffSupergroup(env.staffID), owner, "/staff")

	if err := runStaffCommand(t, env.bot, ctx, staffDesc, staffModule.staffPanel); err != ext.EndGroups {
		t.Fatalf("/staff returned %v, want ext.EndGroups", err)
	}

	calls := callsToChat(env.client, "sendMessage", env.staffID)
	if len(calls) != 1 {
		t.Fatalf("Staff Group messages = %d, want the panel", len(calls))
	}
	keyboard := inlineKeyboardOf(t, calls[0])
	if len(keyboard.InlineKeyboard) == 0 || len(keyboard.InlineKeyboard[0]) == 0 {
		t.Fatalf("panel keyboard = %+v, want an Add group button in the first row", keyboard)
	}
	button := keyboard.InlineKeyboard[0][0]
	wantURL := fmt.Sprintf("https://t.me/AlitaTestBot?startgroup=stf_%d&admin=restrict_members+delete_messages", -env.staffID)
	if button.Url != wantURL {
		t.Fatalf("button URL = %q, want %q", button.Url, wantURL)
	}
	if !strings.Contains(button.Text, staffMarker("staff_panel_add_group_button")) {
		t.Fatalf("button text = %q, want staff_panel_add_group_button", button.Text)
	}
	payload := strings.SplitN(strings.SplitN(button.Url, "startgroup=", 2)[1], "&", 2)[0]
	if len(payload) > 64 || !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(payload) {
		t.Fatalf("payload %q breaks Telegram's start payload rules", payload)
	}
}

func TestHandleGroupDeepLinkUsesLongestPrefix(t *testing.T) {
	var called []string
	record := func(name string) GroupDeepLinkHandler {
		return func(_ *gotgbot.Bot, _ *ext.Context, _ *gotgbot.User, arg string) (bool, error) {
			called = append(called, name+":"+arg)
			return true, ext.EndGroups
		}
	}
	RegisterGroupDeepLinkHandler("zzt_", record("short"))
	RegisterGroupDeepLinkHandler("zzt_long_", record("long"))
	t.Cleanup(func() {
		delete(groupDeepLinkRegistry, "zzt_")
		delete(groupDeepLinkRegistry, "zzt_long_")
	})

	if handled, err := HandleGroupDeepLink(nil, nil, nil, "zzt_long_1"); !handled || err != ext.EndGroups {
		t.Fatalf("long payload = (%v, %v), want (true, EndGroups)", handled, err)
	}
	if handled, _ := HandleGroupDeepLink(nil, nil, nil, "zzt_2"); !handled {
		t.Fatal("short payload was not handled")
	}
	if handled, err := HandleGroupDeepLink(nil, nil, nil, "other"); handled || err != nil {
		t.Fatalf("unmatched payload = (%v, %v), want (false, nil)", handled, err)
	}
	if strings.Join(called, ",") != "long:zzt_long_1,short:zzt_2" {
		t.Fatalf("handlers called = %v, want longest prefix first match", called)
	}
}

// The picker must reach the same flow as /linkstaff: a link made through it and
// one made by typing end in the same row.
func TestStaffPickerAndTypedCommandMakeTheSameLink(t *testing.T) {
	picked := newLinkEnv(t)
	picked.sendStart(t, picked.groupChat("Same"), picked.ownerID, picked.pickerPayload())
	typed := newLinkEnv(t)
	typed.sendLink(t, typed.groupChat("Same"), typed.ownerID, fmt.Sprintf("/linkstaff %d", typed.staffID))

	pickedLink, err := staff.GetLinkOfGroupFresh(picked.groupID)
	if err != nil || pickedLink == nil {
		t.Fatalf("picker link = (%+v, %v)", pickedLink, err)
	}
	typedLink, err := staff.GetLinkOfGroupFresh(typed.groupID)
	if err != nil || typedLink == nil {
		t.Fatalf("typed link = (%+v, %v)", typedLink, err)
	}
	if pickedLink.GroupTitle != typedLink.GroupTitle || pickedLink.Health != typedLink.Health ||
		pickedLink.OwnerUserID != picked.ownerID || typedLink.OwnerUserID != typed.ownerID {
		t.Fatalf("picker link %+v and typed link %+v differ", pickedLink, typedLink)
	}
}
