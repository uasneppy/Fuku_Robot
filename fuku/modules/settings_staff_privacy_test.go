//go:build testtools

package modules

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
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

// settingsPrivacyMainRulesSet is entries with the Rules line reading Set, as a Rules row with only a
// photo reports it on Main.
func settingsPrivacyMainRulesSet(tr *i18n.Translator, entries []settingsMainEntry) []settingsMainEntry {
	out := append([]settingsMainEntry(nil), entries...)
	for i := range out {
		if out[i].Key == settingsPageRules {
			out[i].State = settingsText(tr, "settings_state_set")
		}
	}
	return out
}

func TestSettingsStaffPrivacy(t *testing.T) {
	t.Run("pure renders in every language", func(t *testing.T) {
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

		for _, code := range []string{"uk", "en", "es", "fr", "hi", "id", "pt", "ru"} {
			t.Run(code, func(t *testing.T) {
				tr := i18n.MustNewTranslator(code)
				groupView := settingsView{
					Tr: tr, OwnerKey: "u", OwnerValue: "9999999999",
					GroupID: link.GroupChatID, GroupTitle: "Test Group", OpenerName: "Opener",
				}
				// The creator of the group, by the press's live member record (D-32).
				privateView := settingsView{
					Tr: tr, Private: true, Creator: true, OwnerKey: "c", OwnerValue: strconv.FormatInt(link.GroupChatID, 10),
					GroupID: link.GroupChatID, GroupTitle: "Test Group", OpenerName: "Opener",
				}
				// Any other admin of the group: a private menu, but not the creator.
				privateAdminView := privateView
				privateAdminView.Creator = false

				notices := []string{
					"",
					settingsNoticeOK(settingsText(tr, "settings_notice_linked")),
					settingsNoticeOK(settingsText(tr, "settings_notice_unlinked")),
					settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
					settingsNoticeWarn(settingsText(tr, "staff_check_failed")),
					settingsNoticeWarn(settingsText(tr, "staff_link_no_staff_group")),
					settingsNoticeWarn(settingsText(tr, "settings_staff_no_group")),
					// Phase 12: the notice a Media under text press that cannot apply puts on a page.
					settingsNoticeWarn(settingsText(tr, "settings_edit_below_unavailable")),
				}
				for r := staffRefusalNone + 1; r <= staffRefusalCheckFailed; r++ {
					notices = append(notices, settingsNoticeWarn(staffLinkRefusalText(tr, r, groupView.GroupTitle)))
				}
				for r := staffUnlinkAnonymous; r <= staffUnlinkGone; r++ {
					notices = append(notices, settingsNoticeWarn(staffUnlinkRefusalText(tr, r, groupView.GroupTitle)))
				}

				// Main as every page reports it: the status block holds each page's state and
				// never the Staff Group.
				mainEntries := func(staffState string) []settingsMainEntry {
					return []settingsMainEntry{
						{Key: "s", Icon: "👥", Title: settingsText(tr, "settings_page_staff"), State: staffState, Known: true},
						{Key: "l", Icon: "🔒", Title: settingsText(tr, "settings_page_lockdown"), State: settingsText(tr, "settings_state_lock_none"), Known: true},
						{Key: "c", Icon: "🧩", Title: settingsText(tr, "settings_page_captcha"), State: settingsStateWord(tr, true), Known: true},
						{Key: "f", Icon: "🌊", Title: settingsText(tr, "settings_page_antiflood"), State: "", Known: false},
						{Key: "w", Icon: "🎉", Title: settingsText(tr, "settings_page_welcome"), State: settingsText(tr, "common_status_on"), Known: true},
						{Key: "g", Icon: "👋", Title: settingsText(tr, "settings_page_goodbye"), State: settingsText(tr, "common_status_off"), Known: true},
						{Key: "r", Icon: "📜", Title: settingsText(tr, "settings_page_rules"), State: settingsText(tr, "settings_state_not_set"), Known: true},
						{Key: "m", Icon: "🛂", Title: settingsText(tr, "settings_page_mreq"), State: settingsText(tr, "settings_state_off_plain"), Known: true},
						{Key: "a", Icon: "🔑", Title: settingsText(tr, "settings_page_access"), State: settingsText(tr, "settings_state_access_off"), Known: true, OwnerOnly: true},
					}
				}

				// Every screen a group menu can reach on the Staff Group page.
				screens := map[string]settingsScreen{
					"unlink card": renderSettingsStaffUnlinkCard(groupView, link),
					// A private menu of an admin who is not the creator is masked like a group menu.
					"private admin linked":           renderSettingsStaffLinked(privateAdminView, settingsStaffViewFor(privateAdminView, false, &staffGroup, &row), ""),
					"private admin linked, no group": renderSettingsStaffLinked(privateAdminView, settingsStaffViewFor(privateAdminView, false, nil, &row), ""),
					"private admin not linked":       renderSettingsStaffNotLinked(privateAdminView, ""),
					"main":                           renderSettingsMain(groupView, mainEntries(settingsText(tr, "settings_state_linked")), ""),
					"main, not linked":               renderSettingsMain(groupView, mainEntries(settingsText(tr, "settings_state_not_linked")), ""),
					"private admin main":             renderSettingsMain(privateAdminView, mainEntries(settingsText(tr, "settings_state_linked")), ""),
					"private creator main":           renderSettingsMain(privateView, mainEntries(settingsText(tr, "settings_state_linked")), ""),
					"private admin main, Rules Set":  renderSettingsMain(privateAdminView, settingsPrivacyMainRulesSet(tr, mainEntries(settingsText(tr, "settings_state_linked"))), ""),
					"list empty":                     renderSettingsHubList(tr, nil, 1),
					"list populated":                 renderSettingsHubList(tr, []adminindex.ListedGroup{{ChatID: link.GroupChatID, Title: "Test Group"}, {ChatID: link.GroupChatID - 1, Title: "Other"}}, 1),
					"access page on":                 renderSettingsAccessPage(groupView, true, ""),
					"access page off":                renderSettingsAccessPage(groupView, false, ""),
					"access card":                    renderSettingsAccessCard(groupView),
					"private access on":              renderSettingsAccessPage(privateAdminView, true, ""),
					"private card":                   renderSettingsAccessCard(privateAdminView),
					"picker of one":                  renderSettingsStaffPicker(groupView, pickable[:1], ""),
					"picker of three":                renderSettingsStaffPicker(groupView, pickable, ""),
					"linked":                         renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, &staffGroup, &row), ""),
					"not linked":                     renderSettingsStaffNotLinked(groupView, ""),
					"linked, no group":               renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, nil, &row), ""),
				}
				// The message editor pages, in every state, for a group menu and for the private
				// menu of an admin who is not the creator.
				for _, def := range settingsEditPages {
					states := map[string]settingsEditState{
						"default":    {Kind: def.Kind, TextIsDefault: def.Kind != settingsEditRules, Enabled: true},
						"all set":    {Kind: def.Kind, Text: "custom <b>text</b>", MediaType: db.PHOTO, FileID: "f", Buttons: models.ButtonArray{{Name: "B", Url: "https://example.com"}}, Enabled: true, Clean: true},
						"sticker":    {Kind: def.Kind, Text: "custom", MediaType: db.STICKER, FileID: "f"},
						"video note": {Kind: def.Kind, MediaType: db.VIDEO_NOTE, FileID: "f", TextIsDefault: true},
						// Phase 12: a photo alone (a Rules row with only media) and Greet once with Media
						// under text on.
						"photo only":  {Kind: def.Kind, MediaType: db.PHOTO, FileID: "f", TextIsDefault: def.Kind != settingsEditRules},
						"video below": {Kind: def.Kind, Text: "custom", MediaType: db.VIDEO, FileID: "f", Enabled: true, Once: true, MediaBelow: true},
					}
					// Every state the page tests of Phase 12 draw: each switch on and off, Greet once,
					// Media under text for a photo, a video and an animation, and the file hint of a document.
					for name, st := range settingsEditCallbackStates(def) {
						states["phase 12 "+name] = st
					}
					for stateName, st := range states {
						for viewName, view := range map[string]settingsView{"group": groupView, "private admin": privateAdminView} {
							for i, notice := range notices {
								key := fmt.Sprintf("editor %s %s %s notice %d", def.Key, stateName, viewName, i)
								screens[key] = renderSettingsEditor(view, def, st, settingsEditScreenOpts{Notice: notice, BotUsername: "FukuTestBot"})
							}
						}
					}
				}
				// Every other screen of the message editor: the prompts, the typed-reply refusals,
				// the ended and wait-ended replies, the saved page, the cancel and group notes and
				// the preview headers, as the code sends them.
				welcome, _ := settingsEditPageByKey(settingsPageWelcome)
				screens["editor prompt, private admin"] = renderSettingsEditPrompt(
					privateAdminView, welcome, settingsEditPartText, time.Now(), settingsEditTextMax, true)
				for viewName, view := range map[string]settingsView{"group": groupView, "private admin": privateAdminView} {
					for name, screen := range settingsEditScreens(tr, view) {
						screens["editor screen "+viewName+" "+name] = screen
					}
				}
				// Phase 13: the Member requirements page with every rule off, and with every punishment
				// used once and both switches on, under every notice, for a group menu and for the private
				// menu of an admin who is not the creator.
				allOff := &models.ChatRequirements{}
				everyPunishment := &models.ChatRequirements{
					LastNameAction: models.RequirementActionWarn, UsernameAction: models.RequirementActionKick,
					RussianAction: models.RequirementActionBan, ArabicAction: models.RequirementActionMute,
					ChineseAction: models.RequirementActionOff, CheckOnJoin: true, DeleteMessages: true,
				}
				for viewName, view := range map[string]settingsView{"group": groupView, "private admin": privateAdminView} {
					for i, notice := range notices {
						screens[fmt.Sprintf("member requirements %s all off notice %d", viewName, i)] = renderSettingsMreqPage(view, allOff, notice)
						screens[fmt.Sprintf("member requirements %s every punishment notice %d", viewName, i)] = renderSettingsMreqPage(view, everyPunishment, notice)
					}
				}
				for i, notice := range notices {
					name := fmt.Sprintf("notice %d", i)
					screens["linked, "+name] = renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, &staffGroup, &row), notice)
					screens["not linked, "+name] = renderSettingsStaffNotLinked(groupView, notice)
					screens["private admin linked, "+name] = renderSettingsStaffLinked(privateAdminView, settingsStaffViewFor(privateAdminView, false, &staffGroup, &row), notice)
					screens["picker, "+name] = renderSettingsStaffPicker(groupView, pickable, notice)
				}

				for name, screen := range screens {
					all := settingsStaffPrivacyScreenText(screen)
					if strings.TrimSpace(screen.Text) == "" {
						t.Errorf("%s: the screen has no text", name)
					}
					for _, leak := range []string{
						settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
						"ZZ Staff", settingsStaffPrivacyDigits,
					} {
						if strings.Contains(all, leak) {
							t.Errorf("%s: a screen that must not name the Staff Group leaks %q: %q", name, leak, all)
						}
					}
				}

				// A private menu of the group's creator may name the Staff Group.
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

	// D-32 (WR-02): a private menu is shown to every live admin of the group, who need not be
	// staff. Only the group's creator sees the Staff Group's title and chat ID; every other
	// admin gets the masked line, on every screen the Staff Group page can draw.
	t.Run("a private run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		env := newSettingsEnv(t)
		editCleanup(t, env.chat.Id)
		staffChat := settingsStaffTestOwnedStaffGroup(env, settingsStaffPrivacyTitle)
		linkLockdownGroupTo(env.lockdownEnv, staffChat)

		moderator := gotgbot.User{Id: env.admin.Id + 41, FirstName: "Moderator"}
		env.fake.setMember(env.chat.Id, moderator.Id, staffFakeMember{
			Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true,
		})
		const menuMsgID = 4747
		private := func(kv ...string) {
			env.pressPrivate(moderator, menuMsgID, env.privateFields(env.chat.Id, kv...))
		}
		private("a", "op", "p", "s")
		private("a", "ua")
		private("a", "op", "p", "s")
		private("a", "lk")
		for _, page := range []string{"w", "g", "r", settingsPageMreq} {
			private("a", "op", "p", page)
		}
		private("a", "tg", "p", "w", "t", "e", "v", "0")
		private("a", "tg", "p", settingsPageMreq, "t", settingsMreqControlJoin, "v", "1")

		var seen []string
		for _, edit := range env.edits(moderator.Id, menuMsgID) {
			seen = append(seen, fmt.Sprint(edit.Params["text"]))
			for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
				seen = append(seen, button.Text, button.CallbackData)
			}
		}
		for _, answer := range env.fake.callsFor("answerCallbackQuery") {
			seen = append(seen, fmt.Sprint(answer.Params["text"]))
		}
		everything := strings.Join(seen, "\n")
		if len(env.edits(moderator.Id, menuMsgID)) < 3 {
			t.Fatalf("the run edited the menu only %d times", len(env.edits(moderator.Id, menuMsgID)))
		}
		for _, leak := range []string{
			"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
			strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"),
		} {
			if strings.Contains(everything, leak) {
				t.Errorf("a private menu of an admin who is not the creator received %q: %q", leak, everything)
			}
		}
		if !strings.Contains(everything, staffMarker("settings_staff_linked_hidden")) {
			t.Errorf("the page lacks the masked line: %q", everything)
		}

		// The creator of the same group sees the full line.
		env.fake.setMember(env.chat.Id, env.admin.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusCreator})
		env.pressPrivate(env.admin, menuMsgID+1, env.privateFields(env.chat.Id, "a", "op", "p", "s"))
		text, _ := env.lastEdit(env.admin.Id, menuMsgID+1)
		for _, want := range []string{staffMarker("settings_staff_linked_to"), strconv.FormatInt(staffChat.Id, 10)} {
			if !strings.Contains(text, want) {
				t.Errorf("the creator's private page %q lacks %q", text, want)
			}
		}
	})

	// The whole private editor flow of an administrator who is not the creator: Set, a refused
	// message, a saved text and both kinds of preview. Everything the bot produced for them (every
	// message, edit and answer) must name neither the Staff Group's title nor its chat ID.
	t.Run("a private editor run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		staffChat := settingsStaffTestOwnedStaffGroup(env, settingsStaffPrivacyTitle)
		linkLockdownGroupTo(env.lockdownEnv, staffChat)
		moderator := gotgbot.User{Id: env.admin.Id + 43, FirstName: "Moderator"}
		listEditAdmin(env, env.chat, moderator, true)

		settingsEditSetWait(env, moderator, settingsPageWelcome, settingsEditPartText)
		sendPrivateMedia(env, moderator, "photo")
		sendPrivateMessage(env, moderator, gotgbot.Message{Text: "Hello {first}, welcome to {chatname}"})
		settingsEditWaitGone(t, env, moderator)

		saved := env.fake.sentTo(moderator.Id)
		page := saved[len(saved)-1]
		for _, marker := range []string{"settings_edit_saved_text", "settings_btn_edit_preview_text", "settings_btn_edit_full_preview"} {
			if !containsMarker(fmt.Sprint(page.Params["text"]), marker) {
				if _, ok := settingsButtonByMarker(staffKeyboardOf(page.Params["reply_markup"]), marker); !ok {
					t.Fatalf("the saved page %+v has no %s", page.Params, marker)
				}
			}
		}
		for _, label := range []string{"settings_btn_edit_preview_text", "settings_btn_edit_full_preview"} {
			button, ok := settingsButtonByMarker(staffKeyboardOf(page.Params["reply_markup"]), label)
			if !ok {
				t.Fatalf("the saved page has no %s button: %+v", label, page.Params["reply_markup"])
			}
			env.pressRaw(settingsPrivateChat(moderator), moderator, page.MessageID, button.CallbackData)
		}

		var seen []string
		collect := func(params map[string]any) {
			seen = append(seen, fmt.Sprint(params["text"]))
			for _, button := range settingsAllButtons(staffKeyboardOf(params["reply_markup"])) {
				seen = append(seen, button.Text, button.CallbackData, button.Url)
			}
		}
		for _, sent := range env.fake.sentTo(moderator.Id) {
			collect(sent.Params)
		}
		edits := callsToChat(env.fake.staffBotClient, "editMessageText", moderator.Id)
		for _, edit := range edits {
			collect(edit.Params)
		}
		for _, answer := range env.fake.callsFor("answerCallbackQuery") {
			seen = append(seen, fmt.Sprint(answer.Params["text"]))
		}
		everything := strings.Join(seen, "\n")
		if len(env.fake.sentTo(moderator.Id)) < 5 || len(edits) < 2 {
			t.Fatalf("the run sent %d messages and made %d edits, want a refusal, the saved page and two previews", len(env.fake.sentTo(moderator.Id)), len(edits))
		}
		for _, wanted := range []string{"settings_edit_refuse_need_text", "settings_edit_still_waiting", "settings_edit_saved_text", "settings_edit_preview_header"} {
			if !containsMarker(everything, wanted) {
				t.Errorf("the run never produced %s: %q", wanted, everything)
			}
		}
		for _, leak := range []string{
			"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
			strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"),
		} {
			if strings.Contains(everything, leak) {
				t.Errorf("the typed editor run of an admin who is not the creator received %q: %q", leak, everything)
			}
		}
	})

	// Phase 12: the same administrator, who is not the creator, sets a Rules photo by typing it, turns the
	// Rules Media under text on, runs the Rules Full preview, then turns Greet once and Media under text on
	// for the Welcome and runs its Full preview. Nothing the bot produced names the Staff Group, and the
	// run writes no greeted_members row, so no page can show who was greeted.
	t.Run("a private Phase 12 run by an admin who is not the creator never names the Staff Group or a greeted member", func(t *testing.T) {
		env := newSettingsEditEnv(t)
		staffChat := settingsStaffTestOwnedStaffGroup(env, settingsStaffPrivacyTitle)
		linkLockdownGroupTo(env.lockdownEnv, staffChat)
		moderator := gotgbot.User{Id: env.admin.Id + 47, FirstName: "Moderator"}
		listEditAdmin(env, env.chat, moderator, true)
		settingsRulesCleanUsers(t, moderator.Id)
		settingsBelowSeed(t, env.chat.Id, true, "privacy-welcome-photo", db.PHOTO)

		menuID := settingsEditSetWait(env, moderator, settingsPageRules, settingsEditPartMedia)
		sendPrivateMessage(env, moderator, settingsMediaItemID("photo", "privacy-rules-photo"))
		settingsEditWaitGone(t, env, moderator)
		private := func(kv ...string) {
			env.pressPrivate(moderator, menuID, env.privateFields(env.chat.Id, kv...))
		}
		private("a", "tg", "p", settingsPageRules, "t", settingsEditControlBelow, "v", "1")
		private("a", "rf")
		private("a", "op", "p", settingsPageWelcome)
		private("a", "tg", "p", settingsPageWelcome, "t", settingsEditControlOnce, "v", "1")
		private("a", "tg", "p", settingsPageWelcome, "t", settingsEditControlBelow, "v", "1")
		private("a", "wf")

		var seen []string
		collect := func(params map[string]any) {
			seen = append(seen, fmt.Sprint(params["text"]), fmt.Sprint(params["caption"]))
			for _, button := range settingsAllButtons(staffKeyboardOf(params["reply_markup"])) {
				seen = append(seen, button.Text, button.CallbackData, button.Url)
			}
		}
		for _, sent := range env.fake.sentTo(moderator.Id) {
			collect(sent.Params)
		}
		for _, edit := range callsToChat(env.fake.staffBotClient, "editMessageText", moderator.Id) {
			collect(edit.Params)
		}
		for _, answer := range env.fake.callsFor("answerCallbackQuery") {
			seen = append(seen, fmt.Sprint(answer.Params["text"]))
		}
		everything := strings.Join(seen, "\n")

		for _, wanted := range []string{
			"settings_btn_edit_switch_once", "settings_edit_line_once", "settings_btn_edit_switch_below",
			"settings_edit_line_below", "settings_edit_saved_media", "settings_edit_preview_header_rules",
		} {
			if !containsMarker(everything, wanted) {
				t.Errorf("the run never produced %s: %q", wanted, everything)
			}
		}
		if !settingsBelowStored(t, env.chat.Id, true) || !settingsOnceStored(t, env.chat.Id) || !settingsRulesBelowStored(t, env.chat.Id) {
			t.Error("the run did not turn the Phase 12 switches on, so it did not exercise their pages")
		}
		if n := greetedRows(t, env.chat.Id); n != 0 {
			t.Errorf("the settings run left %d greeted_members rows, want none", n)
		}
		for _, leak := range []string{
			"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
			strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"),
		} {
			if strings.Contains(everything, leak) {
				t.Errorf("the Phase 12 run of an admin who is not the creator received %q: %q", leak, everything)
			}
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
