//go:build testtools

package modules

import (
	"strconv"
	"strings"
	"testing"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/rules"
)

// settingsRulesSeedAll stores rules with the text "Be kind", a photo and two buttons for chatID, through
// the repository's own writers.
func settingsRulesSeedAll(t *testing.T, chatID int64) {
	t.Helper()
	if err := rules.SetChatRules(chatID, "Be kind"); err != nil {
		t.Fatalf("SetChatRules: %v", err)
	}
	if err := rules.SetRulesMediaOnly(chatID, "rules-photo", db.PHOTO); err != nil {
		t.Fatalf("SetRulesMediaOnly: %v", err)
	}
	buttons := []models.Button{{Name: "One", Url: "https://one.example"}, {Name: "Two", Url: "https://two.example"}}
	if err := rules.SetRulesButtonsOnly(chatID, buttons); err != nil {
		t.Fatalf("SetRulesButtonsOnly: %v", err)
	}
}

func TestSettingsEditRulesParts(t *testing.T) {
	env := newSettingsEditEnv(t)
	owner := env.newOwner(env.chat)
	settingsRulesSeedAll(t, env.chat.Id)
	menuID := settingsEditPrivateMenu(env, owner, settingsPageRules)

	text, keyboard := env.lastEdit(owner.Id, menuID)
	for _, want := range []string{
		staffMarker("settings_edit_state_text_len") + " 7",
		staffMarker("settings_edit_line_media") + " " + staffMarker("settings_edit_media_photo"),
		staffMarker("settings_edit_state_buttons_n") + " 2",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the Rules page %q lacks %q", text, want)
		}
	}
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "rr"), ","); got != "x,m,b" {
		t.Fatalf("Remove buttons on a fully set Rules page = %q, want x,m,b", got)
	}
	for _, label := range []string{"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons"} {
		if !settingsEditHasButton(keyboard, label) {
			t.Errorf("the Rules page has no %s button", label)
		}
	}

	// Remove media clears the file and the type only.
	answers := env.answerCount()
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_remove_media")
	if got := env.answerCount() - answers; got != 1 {
		t.Errorf("Remove was answered %d times, want once", got)
	}
	row, ok := settingsEditRulesStored(t, env.chat.Id)
	if !ok || row.FileID != "" || row.MediaType != 0 || row.Rules != "Be kind" || len(row.Buttons) != 2 {
		t.Errorf("after Remove media the rules are %+v (found %v), want no media with the text and buttons kept", row, ok)
	}
	text, keyboard = env.lastEdit(owner.Id, menuID)
	if !strings.Contains(text, settingsNoticeOKIcon+staffMarker("settings_edit_removed_media")) {
		t.Errorf("the page %q lacks the removed-media notice", text)
	}
	if want := staffMarker("settings_edit_line_media") + " " + staffMarker("settings_state_not_set"); !strings.Contains(text, want) {
		t.Errorf("the page %q lacks %q after Remove media", text, want)
	}
	if got := strings.Join(settingsEditRemoveParts(t, keyboard, "rr"), ","); got != "x,b" {
		t.Errorf("Remove buttons after Remove media = %q, want x,b", got)
	}

	// Set media from a private menu writes the wait for part m.
	settingsEditPressButton(env, owner, menuID, "settings_btn_edit_set_media")
	wait := settingsEditWaitOf(env, owner)
	if wait["c"] != strconv.FormatInt(env.chat.Id, 10) || wait["p"] != "r" || wait["t"] != "m" {
		t.Fatalf("wait = %v, want the group, page r and part m", wait)
	}
	if prompt, _ := env.lastEdit(owner.Id, menuID); !strings.Contains(prompt, staffMarker("settings_edit_wait_media")) {
		t.Errorf("the prompt %q lacks settings_edit_wait_media", prompt)
	}
}
