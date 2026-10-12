//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

func (e *unlinkEnv) staffChat() gotgbot.Chat { return staffSupergroup(e.staffID) }

func (e *unlinkEnv) owner() gotgbot.User { return gotgbot.User{Id: e.ownerID, FirstName: "Owner"} }

func (e *unlinkEnv) stranger() gotgbot.User {
	return gotgbot.User{Id: unlinkStrangerID, FirstName: "Stranger"}
}

// pressRaw delivers an Unlink callback whose message sits in chat. linkID is the
// raw "l" field.
func (e *unlinkEnv) pressRaw(t *testing.T, chat gotgbot.Chat, from gotgbot.User, action, linkID string) {
	t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": action, "l": linkID})
	ctx := newModuleCallbackContext(e.bot, chat, from, data)
	if err := staffModule.staffCallback(e.bot, ctx); err != ext.EndGroups {
		t.Fatalf("staffCallback(%q) returned %v, want ext.EndGroups", action, err)
	}
}

// press delivers an Unlink callback for link in the Staff Group's panel.
func (e *unlinkEnv) press(t *testing.T, from gotgbot.User, action string, linkID uint) {
	t.Helper()
	e.pressRaw(t, e.staffChat(), from, action, strconv.FormatUint(uint64(linkID), 10))
}

func (e *unlinkEnv) edits() []moduleBotCall { return e.client.callsFor("editMessageText") }

func (e *unlinkEnv) editText(call moduleBotCall) string { return fmt.Sprint(call.Params["text"]) }

func (e *unlinkEnv) answers() []moduleBotCall { return e.client.callsFor("answerCallbackQuery") }

// wantAnswers fails unless the presses so far answered exactly n times, and
// returns the last answer's text and alert flag.
func (e *unlinkEnv) wantAnswers(t *testing.T, n int) (text string, alert bool) {
	t.Helper()
	answers := e.answers()
	if len(answers) != n {
		t.Fatalf("callback answered %d times, want %d", len(answers), n)
	}
	last := answers[len(answers)-1]
	alert, _ = last.Params["show_alert"].(bool)
	return fmt.Sprint(last.Params["text"]), alert
}

// wantAlert expects the latest of n answers to be an alert carrying key.
func (e *unlinkEnv) wantAlert(t *testing.T, n int, key string) {
	t.Helper()
	text, alert := e.wantAnswers(t, n)
	if !alert || !strings.Contains(text, staffMarker(key)) {
		t.Fatalf("answer = (%q, alert=%v), want an alert with %s", text, alert, key)
	}
}

func TestStaffUnlinkButtonTitle(t *testing.T) {
	if got := staffButtonTitle("<b>100%</b>"); got != "<b>100%</b>" {
		t.Errorf("staffButtonTitle must not escape, got %q", got)
	}
	long := strings.Repeat("日", 30)
	if got := staffButtonTitle(long); got != strings.Repeat("日", 20) {
		t.Errorf("staffButtonTitle(30 runes) = %q, want the first 20 runes", got)
	}
	if got := staffButtonTitle("  padded  "); got != "padded" {
		t.Errorf("staffButtonTitle must trim spaces, got %q", got)
	}
}

func TestStaffUnlinkButtonRendered(t *testing.T) {
	env := newUnlinkEnv(t)
	env.link.GroupTitle = "100%d " + strings.Repeat("x", 40)

	rows := []staffLinkRow{{Link: *env.link}, {Link: *env.other}}
	_, keyboard := renderStaffPanel(staffChatTranslator(env.staffID), models.StaffGroup{ChatID: env.staffID, OwnerUserID: env.ownerID}, rows, "fukubot", 0, time.Now())

	var unlinkButtons []gotgbot.InlineKeyboardButton
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			// The Refresh button is a callback button too; only Unlink is counted here.
			if decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace); ok && decoded.Fields["a"] == staffActUnlinkAsk {
				unlinkButtons = append(unlinkButtons, button)
			}
		}
	}
	if len(unlinkButtons) != 2 {
		t.Fatalf("%d callback buttons, want one Unlink button per link", len(unlinkButtons))
	}
	for i, link := range []uint{env.link.ID, env.other.ID} {
		button := unlinkButtons[i]
		if len(button.CallbackData) > 64 {
			t.Errorf("callback data %q is %d bytes, want at most 64", button.CallbackData, len(button.CallbackData))
		}
		decoded, ok := decodeCallbackData(button.CallbackData, staffCallbackNamespace)
		if !ok || decoded.Fields["a"] != "ul" || decoded.Fields["l"] != strconv.FormatUint(uint64(link), 10) {
			t.Errorf("button %d decodes to %+v (ok=%v), want a=ul and l=%d", i, decoded, ok, link)
		}
		if !strings.Contains(button.Text, staffMarker("staff_panel_unlink_button")) {
			t.Errorf("button text %q must use staff_panel_unlink_button", button.Text)
		}
	}
	// Button text is plain: not HTML-escaped, not mangled by the translator, capped.
	if !strings.Contains(unlinkButtons[0].Text, "100%d "+strings.Repeat("x", 14)) ||
		strings.Contains(unlinkButtons[0].Text, strings.Repeat("x", 15)) {
		t.Errorf("button text %q must carry the first 20 runes of the title", unlinkButtons[0].Text)
	}
	if keyboard.InlineKeyboard[0][0].Url == "" {
		t.Errorf("the Add group button must stay the first row")
	}
}

func TestStaffUnlinkButtonRenderedByStaffCommand(t *testing.T) {
	env := newUnlinkEnv(t)
	ctx := newModuleMessageContext(env.bot, env.staffChat(), env.owner(), "/staff")

	if err := runStaffCommand(t, env.bot, ctx, staffDesc, staffModule.staffPanel); err != ext.EndGroups {
		t.Fatalf("/staff returned %v, want ext.EndGroups", err)
	}

	calls := env.client.callsFor("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("%d sendMessage calls, want the panel", len(calls))
	}
	markup := fmt.Sprint(calls[0].Params["reply_markup"])
	for _, id := range []uint{env.link.ID, env.other.ID} {
		if want := fmt.Sprintf("a=ul&l=%d", id); !strings.Contains(markup, want) {
			t.Errorf("panel keyboard %s missing %q", markup, want)
		}
	}
}

func TestStaffUnlinkButtonConfirmFlow(t *testing.T) {
	env := newUnlinkEnv(t)

	env.press(t, env.owner(), staffActUnlinkAsk, env.link.ID)

	edits := env.edits()
	if len(edits) != 1 || !strings.Contains(env.editText(edits[0]), staffMarker("staff_unlink_confirm_prompt")) {
		t.Fatalf("edits = %+v, want one staff_unlink_confirm_prompt", edits)
	}
	if !strings.Contains(env.editText(edits[0]), "Group &lt;b&gt;One&lt;/b&gt;") {
		t.Errorf("prompt %q must name the escaped group title", env.editText(edits[0]))
	}
	markup := fmt.Sprint(edits[0].Params["reply_markup"])
	for _, want := range []string{
		fmt.Sprintf("a=uc&l=%d", env.link.ID), fmt.Sprintf("a=ux&l=%d", env.link.ID),
		staffMarker("staff_btn_confirm"), staffMarker("staff_btn_cancel"),
	} {
		if !strings.Contains(markup, want) {
			t.Errorf("prompt keyboard %s missing %q", markup, want)
		}
	}
	env.wantLinked(t, env.groupID)
	if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
		t.Fatalf("asking posted %d messages, want none", len(sent))
	}
	env.wantAnswers(t, 1)

	env.press(t, env.owner(), staffActUnlinkConfirm, env.link.ID)

	env.wantUnlinked(t, env.groupID)
	env.wantLinked(t, env.otherID)
	edits = env.edits()
	if len(edits) != 2 {
		t.Fatalf("%d edits after confirm, want the prompt then the re-rendered panel", len(edits))
	}
	panel := env.editText(edits[1])
	if !strings.Contains(panel, staffMarker("staff_panel_links_header")) || !strings.Contains(panel, "Other") ||
		strings.Contains(panel, "Group &lt;b&gt;One&lt;/b&gt;") {
		t.Errorf("re-rendered panel %q must list only the other link", panel)
	}
	panelMarkup := fmt.Sprint(edits[1].Params["reply_markup"])
	if !strings.Contains(panelMarkup, fmt.Sprintf("a=ul&l=%d", env.other.ID)) ||
		strings.Contains(panelMarkup, fmt.Sprintf("a=ul&l=%d", env.link.ID)) {
		t.Errorf("re-rendered keyboard %s must offer Unlink for the other link only", panelMarkup)
	}
	env.wantInStaff(t, "staff_notice_unlinked_by")
	env.wantAnswers(t, 2)
}

func TestStaffUnlinkButtonConfirmTwiceRemovesOnce(t *testing.T) {
	env := newUnlinkEnv(t)

	env.press(t, env.owner(), staffActUnlinkConfirm, env.link.ID)
	env.press(t, env.owner(), staffActUnlinkConfirm, env.link.ID)

	if notices := textsToChat(env.client, env.staffID); len(notices) != 1 {
		t.Fatalf("Staff Group notices = %q, want exactly one", notices)
	}
	text, alert := env.wantAnswers(t, 2)
	if alert || !strings.Contains(text, staffMarker("staff_cb_expired")) {
		t.Fatalf("second press answer = (%q, alert=%v), want a staff_cb_expired toast", text, alert)
	}
	env.wantLinked(t, env.otherID)
}

func TestStaffUnlinkButtonCancel(t *testing.T) {
	env := newUnlinkEnv(t)

	env.press(t, env.owner(), staffActUnlinkCancel, env.link.ID)

	env.wantLinked(t, env.groupID)
	env.wantLinked(t, env.otherID)
	edits := env.edits()
	if len(edits) != 1 || !strings.Contains(env.editText(edits[0]), staffMarker("staff_panel_links_header")) {
		t.Fatalf("edits = %+v, want the panel re-rendered", edits)
	}
	if !strings.Contains(env.editText(edits[0]), "Group &lt;b&gt;One&lt;/b&gt;") {
		t.Errorf("cancelled panel %q must still list the group", env.editText(edits[0]))
	}
	env.wantAnswers(t, 1)
	if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
		t.Fatalf("cancel posted %d messages, want none", len(sent))
	}
}

func TestStaffUnlinkButtonStranger(t *testing.T) {
	for _, action := range []string{staffActUnlinkAsk, staffActUnlinkConfirm, staffActUnlinkCancel} {
		t.Run(action, func(t *testing.T) {
			env := newUnlinkEnv(t)

			env.press(t, env.stranger(), action, env.link.ID)

			env.wantAlert(t, 1, "staff_cb_owner_only")
			env.wantLinked(t, env.groupID)
			if edits := env.edits(); len(edits) != 0 {
				t.Fatalf("stranger's %s edited the message: %+v", action, edits)
			}
			if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
				t.Fatalf("stranger's %s posted %d messages, want none", action, len(sent))
			}
		})
	}
}

func TestStaffUnlinkButtonOnlyCreatorOfBothGroups(t *testing.T) {
	cases := map[string]func(*unlinkEnv){
		"creator of the Staff Group only": func(e *unlinkEnv) { e.client.setCreator(e.groupID, 777) },
		"creator of the group only":       func(e *unlinkEnv) { e.client.setCreator(e.staffID, 777) },
	}
	for name, setup := range cases {
		for _, action := range []string{staffActUnlinkAsk, staffActUnlinkConfirm} {
			t.Run(name+"/"+action, func(t *testing.T) {
				env := newUnlinkEnv(t)
				setup(env)

				env.press(t, env.owner(), action, env.link.ID)

				env.wantAlert(t, 1, "staff_cb_owner_only")
				env.wantLinked(t, env.groupID)
				if edits := env.edits(); len(edits) != 0 {
					t.Fatalf("%s edited the message: %+v", action, edits)
				}
			})
		}
	}
}

func TestStaffUnlinkButtonCheckFailure(t *testing.T) {
	for _, which := range []string{"staff", "group"} {
		for _, action := range []string{staffActUnlinkAsk, staffActUnlinkConfirm} {
			t.Run(which+"/"+action, func(t *testing.T) {
				env := newUnlinkEnv(t)
				chatID := env.staffID
				if which == "group" {
					chatID = env.groupID
				}
				env.client.setFailure("getChatAdministrators", chatID, errors.New("telegram unreachable"))

				env.press(t, env.owner(), action, env.link.ID)

				env.wantAlert(t, 1, "staff_check_failed")
				env.wantLinked(t, env.groupID)
				if edits := env.edits(); len(edits) != 0 {
					t.Fatalf("%s edited the message: %+v", action, edits)
				}
				if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
					t.Fatalf("%s posted %d messages, want none", action, len(sent))
				}
			})
		}
	}
}

func TestStaffUnlinkButtonOwnerChangedBetweenPresses(t *testing.T) {
	env := newUnlinkEnv(t)

	env.press(t, env.owner(), staffActUnlinkAsk, env.link.ID)
	if edits := env.edits(); len(edits) != 1 {
		t.Fatalf("the first press made %d edits, want the prompt", len(edits))
	}
	env.client.setCreator(env.groupID, 777) // ownership of the group moved

	env.press(t, env.owner(), staffActUnlinkConfirm, env.link.ID)

	env.wantAlert(t, 2, "staff_cb_owner_only")
	env.wantLinked(t, env.groupID)
	if edits := env.edits(); len(edits) != 1 {
		t.Fatalf("the confirm press edited the message again: %d edits", len(edits))
	}
	if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
		t.Fatalf("a refused confirm posted %d messages, want none", len(sent))
	}
}

func TestStaffUnlinkButtonWrongChat(t *testing.T) {
	for _, action := range []string{staffActUnlinkAsk, staffActUnlinkConfirm, staffActUnlinkCancel} {
		t.Run(action, func(t *testing.T) {
			env := newUnlinkEnv(t)
			otherStaff := uniqueModuleChatID()
			env.client.setCreator(otherStaff, env.ownerID)
			staffCleanup(t, otherStaff)
			if _, err := staff.CreateStaffGroup(otherStaff, env.ownerID, "Other Staff"); err != nil {
				t.Fatal(err)
			}

			env.pressRaw(t, staffSupergroup(otherStaff), env.owner(), action, strconv.FormatUint(uint64(env.link.ID), 10))

			env.wantAlert(t, 1, "staff_cb_denied")
			env.wantLinked(t, env.groupID)
			env.wantLinked(t, env.otherID)
			if env.lookups() != 0 {
				t.Fatalf("a callback from the wrong chat triggered %d live lookups, want 0", env.lookups())
			}
			if edits := env.edits(); len(edits) != 0 {
				t.Fatalf("%s from the wrong chat edited the message: %+v", action, edits)
			}
			if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
				t.Fatalf("%s from the wrong chat posted %d messages, want none", action, len(sent))
			}
		})
	}
}

func TestStaffUnlinkButtonExpired(t *testing.T) {
	for _, action := range []string{staffActUnlinkAsk, staffActUnlinkConfirm, staffActUnlinkCancel} {
		t.Run(action, func(t *testing.T) {
			env := newUnlinkEnv(t)
			if deleted, err := staff.DeleteLink(env.link.ID); err != nil || !deleted {
				t.Fatalf("DeleteLink = (%v, %v)", deleted, err)
			}

			env.press(t, env.owner(), action, env.link.ID)

			text, alert := env.wantAnswers(t, 1)
			if alert || !strings.Contains(text, staffMarker("staff_cb_expired")) {
				t.Fatalf("answer = (%q, alert=%v), want a staff_cb_expired toast", text, alert)
			}
			edits := env.edits()
			if len(edits) != 1 || !strings.Contains(env.editText(edits[0]), staffMarker("staff_panel_links_header")) ||
				strings.Contains(env.editText(edits[0]), "Group &lt;b&gt;One&lt;/b&gt;") {
				t.Fatalf("edits = %+v, want the panel re-rendered without the deleted link", edits)
			}
			env.wantLinked(t, env.otherID)
			if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
				t.Fatalf("an expired press posted %d messages, want none", len(sent))
			}
		})
	}
}

func TestStaffUnlinkButtonBadLinkID(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":    "",
		"text":     "abc",
		"negative": "-1",
		"signed":   "+5",
		"float":    "1.5",
		"overflow": "99999999999999999999999",
	} {
		t.Run(name, func(t *testing.T) {
			env := newUnlinkEnv(t)

			env.pressRaw(t, env.staffChat(), env.owner(), staffActUnlinkConfirm, raw)

			text, alert := env.wantAnswers(t, 1)
			if alert || !strings.Contains(text, staffMarker("staff_cb_expired")) {
				t.Fatalf("answer = (%q, alert=%v), want a staff_cb_expired toast", text, alert)
			}
			env.wantLinked(t, env.groupID)
			env.wantLinked(t, env.otherID)
			if sent := callsToChat(env.client, "sendMessage", env.staffID); len(sent) != 0 {
				t.Fatalf("a bad link ID posted %d messages, want none", len(sent))
			}
		})
	}
}

func TestStaffUnlinkButtonOnlyNamedLinkGoes(t *testing.T) {
	env := newUnlinkEnv(t)

	env.press(t, env.owner(), staffActUnlinkConfirm, env.other.ID)

	env.wantUnlinked(t, env.otherID)
	env.wantLinked(t, env.groupID)
	notices := textsToChat(env.client, env.staffID)
	if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_notice_unlinked_by")) ||
		!strings.Contains(notices[0], "Other") {
		t.Fatalf("Staff Group notices = %q, want one notice naming Other", notices)
	}
}
