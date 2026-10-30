//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// The Staff Group the privacy test hides: its title holds characters the escaper changes,
// and its chat ID is one a leak could be found by.
const (
	settingsStaffPrivacyTitle  = "ZZ Staff & <Ö> Marker"
	settingsStaffPrivacyChatID = int64(-1009876543210)
	// settingsStaffPrivacyDigits is the ID without its sign, so a leak that drops the minus
	// is still found.
	settingsStaffPrivacyDigits = "1009876543210"
)

// settingsStaffPrivacyScreenText is everything a reader of a screen receives: its text, every
// button label and every callback data.
func settingsStaffPrivacyScreenText(screen settingsScreen) string {
	var sb strings.Builder
	sb.WriteString(screen.Text)
	for _, button := range settingsAllButtons(screen.Keyboard.InlineKeyboard) {
		sb.WriteString("\n")
		sb.WriteString(button.Text)
		sb.WriteString("\n")
		sb.WriteString(button.CallbackData)
	}
	return sb.String()
}

func TestSettingsStaffPrivacy(t *testing.T) {
	t.Run("pure renders in seven languages", func(t *testing.T) {
		withRealLocales(t)

		staffGroup := models.StaffGroup{ID: 7, ChatID: settingsStaffPrivacyChatID, OwnerUserID: 55, Title: settingsStaffPrivacyTitle}
		link := models.StaffGroupLink{
			ID: 3, GroupChatID: -1001234567890, StaffChatID: settingsStaffPrivacyChatID,
			OwnerUserID: 55, GroupTitle: "Test Group",
		}
		row := staffLinkRow{
			Link: link, Health: models.StaffHealthOK, HealthKnown: true, OwnerState: chat_status.OwnerMatch,
		}
		pickable := []models.StaffGroup{
			staffGroup,
			{ID: 8, ChatID: settingsStaffPrivacyChatID - 1, OwnerUserID: 55, Title: settingsStaffPrivacyTitle + " 2"},
			{ID: 9, ChatID: settingsStaffPrivacyChatID - 2, OwnerUserID: 55, Title: settingsStaffPrivacyTitle + " 3"},
		}

		for _, code := range []string{"en", "es", "fr", "hi", "id", "pt", "ru"} {
			t.Run(code, func(t *testing.T) {
				tr := i18n.MustNewTranslator(code)
				groupView := settingsView{
					Tr: tr, OwnerKey: "u", OwnerValue: "9999999999",
					GroupID: link.GroupChatID, GroupTitle: "Test Group", OpenerName: "Opener",
				}
				privateView := settingsView{
					Tr: tr, Private: true, OwnerKey: "c", OwnerValue: strconv.FormatInt(link.GroupChatID, 10),
					GroupID: link.GroupChatID, GroupTitle: "Test Group", OpenerName: "Opener",
				}

				notices := []string{
					"",
					settingsNoticeOK(settingsText(tr, "settings_notice_linked")),
					settingsNoticeOK(settingsText(tr, "settings_notice_unlinked")),
					settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
					settingsNoticeWarn(settingsText(tr, "staff_check_failed")),
					settingsNoticeWarn(settingsText(tr, "staff_link_no_staff_group")),
				}
				for r := staffRefusalNone + 1; r <= staffRefusalCheckFailed; r++ {
					notices = append(notices, settingsNoticeWarn(staffLinkRefusalText(tr, r, groupView.GroupTitle)))
				}
				for r := staffUnlinkAnonymous; r <= staffUnlinkGone; r++ {
					notices = append(notices, settingsNoticeWarn(staffUnlinkRefusalText(tr, r, groupView.GroupTitle)))
				}

				// Every screen a group menu can reach on the Staff Group page.
				screens := map[string]settingsScreen{
					"unlink card": renderSettingsStaffUnlinkCard(groupView, link),
					"main": renderSettingsMain(groupView, []settingsMainEntry{
						{Key: "s", Icon: "👥", Title: settingsText(tr, "settings_page_staff"),
							State: settingsText(tr, "settings_state_linked"), Known: true},
						{Key: "s", Icon: "👥", Title: settingsText(tr, "settings_page_staff"),
							State: settingsText(tr, "settings_state_not_linked"), Known: true},
					}, ""),
					"picker of one":    renderSettingsStaffPicker(groupView, pickable[:1], ""),
					"picker of three":  renderSettingsStaffPicker(groupView, pickable, ""),
					"linked":           renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, &staffGroup, &row), ""),
					"not linked":       renderSettingsStaffNotLinked(groupView, ""),
					"linked, no group": renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, nil, &row), ""),
				}
				for i, notice := range notices {
					name := fmt.Sprintf("notice %d", i)
					screens["linked, "+name] = renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, &staffGroup, &row), notice)
					screens["not linked, "+name] = renderSettingsStaffNotLinked(groupView, notice)
					screens["picker, "+name] = renderSettingsStaffPicker(groupView, pickable, notice)
				}

				for name, screen := range screens {
					all := settingsStaffPrivacyScreenText(screen)
					if strings.TrimSpace(screen.Text) == "" {
						t.Errorf("%s: the group menu screen has no text", name)
					}
					for _, leak := range []string{
						settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
						"ZZ Staff", settingsStaffPrivacyDigits,
					} {
						if strings.Contains(all, leak) {
							t.Errorf("%s: a group menu screen leaks %q: %q", name, leak, all)
						}
					}
				}

				// A private menu may name the Staff Group.
				linked := settingsStaffPrivacyScreenText(
					renderSettingsStaffLinked(privateView, settingsStaffViewFor(privateView, false, &staffGroup, &row), ""))
				for _, want := range []string{html.EscapeString(settingsStaffPrivacyTitle), strconv.FormatInt(settingsStaffPrivacyChatID, 10)} {
					if !strings.Contains(linked, want) {
						t.Errorf("private linked page does not contain %q: %q", want, linked)
					}
				}
				picker := settingsStaffPrivacyScreenText(renderSettingsStaffPicker(privateView, pickable, ""))
				if want := "👥 ZZ Staff & <Ö> Marke"; !strings.Contains(picker, want) {
					t.Errorf("private picker does not contain the label %q: %q", want, picker)
				}
			})
		}
	})

	t.Run("a group menu run never names a Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		titles := []string{"ZZ Zero & <Ö>", "ZZ One & <Ö>", "ZZ Two & <Ö>"}
		chats := make([]gotgbot.Chat, len(titles))
		for i, title := range titles {
			chats[i] = settingsStaffTestOwnedStaffGroup(env, title)
		}
		linkLockdownGroupTo(env.lockdownEnv, chats[0])

		msgID := env.openMenu(env.admin)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ua"))
		_, card := env.lastEdit(env.chat.Id, msgID)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "no", "p", "s"))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "ua"))
		env.pressRaw(env.chat, env.admin, msgID, card[0][0].CallbackData)
		if link := settingsStaffTestLinkOf(t, env); link != nil {
			t.Fatalf("Yes did not unlink: %+v", link)
		}

		// Three Staff Groups are the presser's: the picker, then a pick.
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk"))
		ids := settingsStaffTestStaffRowIDs(t, env)
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk", "s", strconv.FormatUint(uint64(ids[1]), 10)))
		if link := settingsStaffTestLinkOf(t, env); link == nil || link.StaffChatID != chats[1].Id {
			t.Fatalf("the pick linked %+v, want Staff Group %d", link, chats[1].Id)
		}

		// A forged pick of another owner's Staff Group.
		stranger := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "ZZ Stranger & <Ö>"}
		staffCleanup(t, stranger.Id)
		if _, err := staff.CreateStaffGroup(stranger.Id, env.admin.Id+1, stranger.Title); err != nil {
			t.Fatalf("create the stranger's Staff Group: %v", err)
		}
		var strangerRow models.StaffGroup
		if err := db.DB.Where("chat_id = ?", stranger.Id).First(&strangerRow).Error; err != nil {
			t.Fatalf("read the stranger's row: %v", err)
		}
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "lk", "s", strconv.FormatUint(uint64(strangerRow.ID), 10)))
		env.press(env.admin, msgID, env.groupFields(env.admin, "a", "op", "p", "s"))

		// What the group received: its messages, every edit of the menu (text and buttons)
		// and every answer.
		var seen []string
		for _, sent := range env.fake.sentTo(env.chat.Id) {
			seen = append(seen, fmt.Sprint(sent.Params["text"]))
			for _, button := range settingsAllButtons(staffKeyboardOf(sent.Params["reply_markup"])) {
				seen = append(seen, button.Text, button.CallbackData)
			}
		}
		for _, edit := range env.edits(env.chat.Id, msgID) {
			seen = append(seen, fmt.Sprint(edit.Params["text"]))
			for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
				seen = append(seen, button.Text, button.CallbackData)
			}
		}
		for _, answer := range env.fake.callsFor("answerCallbackQuery") {
			seen = append(seen, fmt.Sprint(answer.Params["text"]))
		}
		everything := strings.Join(seen, "\n")
		if len(env.edits(env.chat.Id, msgID)) < 8 {
			t.Fatalf("the run edited the menu only %d times", len(env.edits(env.chat.Id, msgID)))
		}

		leaks := []string{"ZZ Zero", "ZZ One", "ZZ Two", "ZZ Stranger"}
		for _, chat := range append(chats, stranger) {
			leaks = append(leaks, strings.TrimPrefix(strconv.FormatInt(chat.Id, 10), "-"))
		}
		for _, leak := range leaks {
			if strings.Contains(everything, leak) {
				t.Errorf("the group received %q", leak)
			}
		}

		// The posts that do name a Staff Group went to the Staff Groups' own chats.
		if got := settingsStaffTestMessagesWith(env, chats[0].Id, "staff_notice_unlinked_by"); got != 1 {
			t.Errorf("the first Staff Group got %d unlink notices, want 1", got)
		}
		if got := settingsStaffTestMessagesWith(env, chats[1].Id, "staff_link_done"); got != 1 {
			t.Errorf("the second Staff Group got %d link notices, want 1", got)
		}
	})
}
