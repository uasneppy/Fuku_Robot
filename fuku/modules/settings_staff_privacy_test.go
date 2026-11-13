//go:build testtools

package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
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

// What the Phase 14 privacy checks hide besides the Staff Group: the linked channel's title and
// ID (the fake knows both) and a held member's name. A screen, an alert or a message that holds
// one of them leaks.
const (
	settingsPrivacyChannelTitle  = "ZZ Channel Marker"
	settingsPrivacyChannelDigits = "1002468013579"
	settingsPrivacyMemberName    = "ZZ Member Marker"
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

// settingsPrivacyMainMreq is entries with the Requirements line reading the given state.
func settingsPrivacyMainMreq(entries []settingsMainEntry, state string) []settingsMainEntry {
	out := append([]settingsMainEntry(nil), entries...)
	for i := range out {
		if out[i].Key == settingsPageMreq {
			out[i].State = state
		}
	}
	return out
}

// settingsPrivacyMreqStatusScreens renders the Requirements page and every rule screen under
// every status a draw can know (nothing read, a channel the bot administers, one it does not,
// no link, an unreadable link and a basic group), for rules all Off and with every punishment
// used, under a plain draw and under a warning notice, in each view. It renders through the
// functions the code sends with, so it cannot drift from them.
func settingsPrivacyMreqStatusScreens(views map[string]settingsView, notices []string, everyPunishment *models.ChatRequirements) map[string]settingsScreen {
	statuses := map[string]settingsMreqStatus{
		"unread":   {},
		"ok":       {Channel: settingsMreqChannelOK},
		"notadmin": {Channel: settingsMreqChannelNotAdmin},
		"nolink":   {Channel: settingsMreqChannelNoLink},
		"unknown":  {Channel: settingsMreqChannelUnknown},
		"basic":    {Basic: true},
		// The AI not configured, alone and in a basic group (Phase 15).
		"ai not configured":                  {AINotConfigured: true},
		"ai not configured in a basic group": {Basic: true, AINotConfigured: true},
	}
	rows := map[string]*models.ChatRequirements{"all off": {}, "every punishment": everyPunishment}
	screens := make(map[string]settingsScreen)
	for viewName, view := range views {
		for statusName, status := range statuses {
			for rowName, row := range rows {
				for _, i := range []int{0, 3} {
					prefix := fmt.Sprintf("member requirements %s %s %s notice %d", viewName, statusName, rowName, i)
					screens[prefix+" page"] = renderSettingsMreqPageStatus(view, row, notices[i], status)
					for _, rule := range memberReqRules {
						screens[prefix+" rule "+rule.Code] = renderSettingsMreqRuleScreenStatus(view, rule, row, notices[i], status)
					}
				}
			}
		}
	}
	return screens
}

func TestSettingsStaffPrivacy(t *testing.T) {
	t.Run("pure renders in every language", func(t *testing.T) {
		withRealLocales(t)
		settingsLangWithCodes(t, settingsLangDefaultCodes)

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
						{Key: "d", Icon: "🧹", Title: settingsText(tr, "settings_page_del"), State: settingsText(tr, "settings_state_del_on", i18n.TranslationParams{"n": 3}), Known: true},
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
					ChineseAction: models.RequirementActionOff, PhotoAction: models.RequirementActionWarn,
					ChannelAction: models.RequirementActionMute, AIAction: models.RequirementActionBan, CheckOnJoin: true, DeleteMessages: true,
				}
				for viewName, view := range map[string]settingsView{"group": groupView, "private admin": privateAdminView} {
					for i, notice := range notices {
						screens[fmt.Sprintf("member requirements %s all off notice %d", viewName, i)] = renderSettingsMreqPage(view, allOff, notice)
						screens[fmt.Sprintf("member requirements %s every punishment notice %d", viewName, i)] = renderSettingsMreqPage(view, everyPunishment, notice)
						// The eight rule screens (each under every notice) and the twenty-four cards.
						for _, rule := range memberReqRules {
							screens[fmt.Sprintf("member requirements rule %s %s notice %d", rule.Code, viewName, i)] = renderSettingsMreqRuleScreen(view, rule, everyPunishment, notice)
						}
					}
					for _, rule := range memberReqRules {
						for _, action := range []int{models.RequirementActionKick, models.RequirementActionBan, models.RequirementActionMute} {
							screens[fmt.Sprintf("member requirements card %s %d %s", rule.Code, action, viewName)] = renderSettingsMreqCard(view, rule, action)
						}
					}
				}
				// Phase 14: the page and every rule screen under every channel status and in a basic
				// group, Main with the new rules on and the two alerts of a refused channel press.
				views := map[string]settingsView{"group": groupView, "private admin": privateAdminView}
				for name, screen := range settingsPrivacyMreqStatusScreens(views, notices, everyPunishment) {
					screens[name] = screen
				}
				rulesOn := settingsText(tr, "settings_state_mreq_on", i18n.TranslationParams{"n": len(memberReqRules)})
				screens["main, every rule on"] = renderSettingsMain(groupView, settingsPrivacyMainMreq(mainEntries(settingsText(tr, "settings_state_linked")), rulesOn), "")
				screens["private admin main, every rule on"] = renderSettingsMain(privateAdminView, settingsPrivacyMainMreq(mainEntries(settingsText(tr, "settings_state_linked")), rulesOn), "")
				screens["alert of a refused channel press"] = settingsScreen{Text: settingsMreqNoLinkAlert(tr)}
				screens["alert could not check"] = settingsScreen{Text: settingsText(tr, "mreq_fixed_check_failed")}
				aiRule, _ := memberReqRuleByCode("ai")
				screens["alert of a refused AI press"] = settingsScreen{Text: settingsMreqUnconfiguredText(tr, aiRule)}
				// Phase 16: the Deletion hub, both lists and all 17 row screens, with every rule set and with
				// none, under every right line, with no notice and with a failed write, in a group menu and
				// in the private menu of an administrator who is not the creator; and the alerts a refused
				// press answers.
				for viewName, view := range map[string]settingsView{"group": groupView, "private admin": privateAdminView} {
					rights := map[string]string{
						"can":     "",
						"cannot":  "🚫 " + settingsText(tr, "settings_del_no_right"),
						"unknown": settingsUnknownState + " " + settingsText(tr, "settings_del_right_unknown"),
					}
					datas := map[string]settingsDelData{
						"every rule set": settingsDelEveryRuleSet(),
						"no rule":        {Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{}}},
						// Phase 17: member rules holding values their rules do not accept, written by hand.
						"unlisted values": {Rules: &deletion.Rules{ChatID: 1, Delays: map[string]int{
							deletion.KindMemberCommands: 60, deletion.KindMemberEdited: 45, deletion.KindMemberSelfDestruct: 45, deletion.KindPinned: 45,
						}}},
					}
					for rightName, right := range rights {
						for dataName, data := range datas {
							for noticeName, delNotice := range map[string]string{
								"no notice":    "",
								"failed write": settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
							} {
								prep := settingsDelPrepared{View: view, Data: data, Right: right}
								for _, named := range settingsDelEveryScreen(prep, delNotice) {
									screens[fmt.Sprintf("deletion %s %s %s %s %s", viewName, rightName, dataName, noticeName, named.name)] = named.screen
								}
							}
						}
					}
				}
				for _, key := range []string{
					"settings_no_change_info", "settings_gate_owner_only", "settings_check_failed", "settings_not_your_menu",
					"settings_not_admin", "common_callback_invalid_request", "settings_cannot_open", "settings_cannot_open_alert",
				} {
					screens["deletion refusal "+key] = settingsScreen{Text: settingsText(tr, key)}
				}
				for i, notice := range notices {
					name := fmt.Sprintf("notice %d", i)
					screens["linked, "+name] = renderSettingsStaffLinked(groupView, settingsStaffViewFor(groupView, false, &staffGroup, &row), notice)
					screens["not linked, "+name] = renderSettingsStaffNotLinked(groupView, notice)
					screens["private admin linked, "+name] = renderSettingsStaffLinked(privateAdminView, settingsStaffViewFor(privateAdminView, false, &staffGroup, &row), notice)
					screens["picker, "+name] = renderSettingsStaffPicker(groupView, pickable, notice)
				}

				// Phase 18: every screen of the six pages the renderers can draw, once per language with
				// the worst case of each kind (a notice adds only its own length, so one plain draw and one
				// with the longest refusal of the page stand for all), in a group menu and in the private
				// menu of an administrator who is not the creator.
				for name, screen := range settingsPrivacyPhase18Screens(tr, map[string]settingsView{"group": groupView, "private admin": privateAdminView}) {
					screens[name] = screen
				}
				// Phase 19: the three list pages' screens (the group note in a group menu, every screen of the
				// private menu), once per language with the worst entry and the longest two notices.
				for name, screen := range settingsPrivacyPhase19Screens(tr, map[string]settingsView{"group": groupView, "private admin": privateAdminView}) {
					screens[name] = screen
				}

				for name, screen := range screens {
					all := settingsStaffPrivacyScreenText(screen)
					if strings.TrimSpace(screen.Text) == "" {
						t.Errorf("%s: the screen has no text", name)
					}
					for _, leak := range []string{
						settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
						"ZZ Staff", settingsStaffPrivacyDigits,
						settingsPrivacyChannelTitle, settingsPrivacyChannelDigits, settingsPrivacyMemberName,
					} {
						if strings.Contains(all, leak) {
							t.Errorf("%s: a screen that must not name the Staff Group, the channel or a member leaks %q: %q", name, leak, all)
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
		private("a", "tg", "p", settingsPageMreq, "t", settingsMreqControlDelete, "v", "1")
		// A rule screen, a strict choice (the card), its Yes and a Warn press, as the admin sees them.
		private("a", settingsActMreqOpen, "t", "ln")
		private("a", "st", "p", settingsPageMreq, "t", "ln", "v", "4")
		private("a", settingsActMreqYes, "t", "ln", "v", "4", "e", settingsCardExpiry(time.Now()))
		private("a", "st", "p", settingsPageMreq, "t", "un", "v", "1")
		// The photo rule's screen (with its hint), its Warn press, its Mute card and the channel rule's
		// screen and Off press. The channel's strict presses need a linked channel (a press in a group
		// with none is refused), so the Phase 14 runs below make them with one.
		private("a", settingsActMreqOpen, "t", "ph")
		private("a", "st", "p", settingsPageMreq, "t", "ph", "v", "1")
		private("a", "st", "p", settingsPageMreq, "t", "ph", "v", "4")
		private("a", settingsActMreqOpen, "t", "ch")
		private("a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")

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

	// Phase 14: the same administrator, who is not the creator, drives the Requirements page, the photo
	// screen and the channel screen through a linked channel the fake knows by title and ID, a held
	// member the fake knows by name, a Warn press on each new rule, a Mute card with its Yes, a lost
	// bot standing, a lost link with its refused press and the two switches. Nothing the bot produced
	// names the Staff Group, the channel, the member or a count.
	t.Run("a private Phase 14 run by an admin who is not the creator names neither the Staff Group, the channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase14Run(t, true)
	})
	t.Run("a group menu Phase 14 run by an admin who is not the creator names neither the Staff Group, the channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase14Run(t, false)
	})

	// Phase 15: the same administrator drives the AI rule's screen in both forms (the AI configured
	// and not), a refused punishing press, a Warn, a Mute card with its Yes and the page with the AI
	// line, over a group linked to a Staff Group, a channel and a held member the fake knows.
	t.Run("a private Phase 15 run by an admin who is not the creator names neither the Staff Group, the channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase15Run(t, true)
	})
	t.Run("a group menu Phase 15 run by an admin who is not the creator names neither the Staff Group, the channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase15Run(t, false)
	})

	// Phase 16: the same administrator drives the Deletion page: a change refused for want of the
	// change-info right, then the presets with the right, every right line the bot can produce, a
	// failed write and the lists, over a group linked to a Staff Group.
	t.Run("a private Phase 16 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase16Run(t, true)
	})
	t.Run("a group menu Phase 16 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase16Run(t, false)
	})

	// Phase 17: the same administrator drives the Member messages section and the Self-destruct card.
	t.Run("a private Phase 17 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase17Run(t, true)
	})
	t.Run("a group menu Phase 17 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase17Run(t, false)
	})

	// Phase 18: the same administrator drives the six pages (Warnings, Switches, Language, Log channel,
	// Anonymous admins and both Locks pages), with permitted and refused presses, over a group linked to a
	// Staff Group and a log channel whose title and ID the fake knows.
	t.Run("a private Phase 18 run by an admin who is not the creator names neither the Staff Group, the log channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase18Run(t, true)
	})
	t.Run("a group menu Phase 18 run by an admin who is not the creator names neither the Staff Group, the log channel nor a member", func(t *testing.T) {
		settingsPrivacyPhase18Run(t, false)
	})

	// Phase 19: the same administrator drives the three list pages: in the private menu a delete ask, an action choice,
	// an Add with its typed words and both typed steps of Filters and Notes; in a group menu every list cell and a
	// forged press.
	t.Run("a private Phase 19 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase19Run(t, true)
	})
	t.Run("a group menu Phase 19 run by an admin who is not the creator never names the Staff Group", func(t *testing.T) {
		settingsPrivacyPhase19Run(t, false)
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

// settingsPrivacyPhase14Run is the Phase 14 run of an administrator who is not the creator, in the
// private menu or in a menu opened in the group, over a group linked to a Staff Group, a channel
// whose title and ID the fake knows and a held member whose name the fake knows.
func settingsPrivacyPhase14Run(t *testing.T, private bool) {
	t.Helper()
	env := newSettingsEnv(t)
	editCleanup(t, env.chat.Id)
	staffChat := settingsStaffTestOwnedStaffGroup(env, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)

	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(&memberReqEnv{settingsEnv: env}, state)
	channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
	state.setTitle(channel, settingsPrivacyChannelTitle)

	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	held := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: settingsPrivacyMemberName, Username: "zzmarkeruser"}
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id, held.Id})
	env.fake.setMember(env.chat.Id, moderator.Id, staffFakeMember{
		Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true,
	})
	env.fake.setProfile(held.Id, held.FirstName, "", held.Username)
	if err := db.DB.Create(&models.User{UserId: held.Id, Name: held.FirstName, UserName: held.Username}).Error; err != nil {
		t.Fatalf("create the held member's users row: %v", err)
	}
	created := time.Now().Add(-time.Hour)
	won, err := memberreq.ClaimHold(context.Background(), &models.ChatRequirementHold{
		ChatID: env.chat.Id, UserID: held.Id, Rules: mreqBitOf(t, "ph"), Action: models.RequirementActionMute,
		CreatedAt: created, PunishedAt: &created, MuteUntil: memberReqMuteUntil(created),
	})
	if err != nil || !won {
		t.Fatalf("seed the held member's hold: won = %t, error = %v", won, err)
	}

	var (
		msgID  int64
		press  func(kv ...string)
		chatID = env.chat.Id
	)
	if private {
		msgID, chatID = 4747, moderator.Id
		press = func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(env.chat.Id, kv...)) }
	} else {
		msgID = env.openMenu(moderator)
		press = func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
	}
	forgetLink := func() {
		memberReqDeleteRedisKeys([]int64{env.chat.Id})
		dbcache.ResetLocalForTest()
	}

	// The page and the photo screen with every rule still Off, then a Warn on the photo rule and its
	// Mute card.
	press("a", "op", "p", settingsPageMreq)
	press("a", settingsActMreqOpen, "t", "ph")
	press("a", "st", "p", settingsPageMreq, "t", "ph", "v", "1")
	press("a", "st", "p", settingsPageMreq, "t", "ph", "v", "4")
	// The channel screen (a fresh read), a Warn press (the link is checked live and holds), a Mute
	// card and its Yes.
	press("a", settingsActMreqOpen, "t", "ch")
	press(settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
	press(settingsMreqChannelStrictKV(models.RequirementActionMute)...)
	press("a", settingsActMreqYes, "t", "ch", "v", "4", "e", settingsCardExpiry(time.Now()))
	// The bot loses its standing in the channel: the page and the channel screen say so.
	state.setMember(channel, staffTestBotID, mreqChannelMember{Status: gotgbot.ChatMemberStatusMember})
	forgetLink()
	press("a", "op", "p", settingsPageMreq)
	press("a", settingsActMreqOpen, "t", "ch")
	// The group loses its link: the page says so, and a strict press is refused with the alert.
	state.link(env.chat.Id, 0)
	forgetLink()
	press("a", "op", "p", settingsPageMreq)
	press(settingsMreqChannelStrictKV(models.RequirementActionBan)...)
	press(settingsMreqChannelStrictKV(models.RequirementActionWarn)...)
	if !private {
		// An unreadable link, in the menu opened in the group (a private press's own getChat of the group
		// would use the scripted error first).
		state.link(env.chat.Id, channel)
		forgetLink()
		state.scriptChat(env.chat.Id, staffFakeError(500, mreqLogDescription))
		press("a", "op", "p", settingsPageMreq)
	}
	press("a", "tg", "p", settingsPageMreq, "t", settingsMreqControlJoin, "v", "1")
	press("a", "tg", "p", settingsPageMreq, "t", settingsMreqControlDelete, "v", "1")
	press("a", "st", "p", settingsPageMreq, "t", "ch", "v", "0")

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 12 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	for _, wanted := range []string{"settings_mreq_here_notadmin", "settings_mreq_here_nolink"} {
		if !containsMarker(everything, wanted) {
			t.Errorf("the run never produced %s: %q", wanted, everything)
		}
	}
	if !private && !containsMarker(everything, "settings_mreq_here_unknown") {
		t.Errorf("the group menu run never produced settings_mreq_here_unknown: %q", everything)
	}
	if stored, ok := settingsMreqStored(t, env.chat.Id); !ok || stored.PhotoAction != models.RequirementActionWarn ||
		stored.ChannelAction != models.RequirementActionOff || !stored.CheckOnJoin || !stored.DeleteMessages {
		t.Errorf("stored settings = %+v (found %t), want the photo rule on Warn (its Mute card was never confirmed), the channel rule Off and both switches on", stored, ok)
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"),
		settingsPrivacyChannelTitle, strings.TrimPrefix(strconv.FormatInt(channel, 10), "-"),
		settingsPrivacyMemberName, "zzmarkeruser", strconv.FormatInt(held.Id, 10), mreqLogDescription,
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 14 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}

// settingsPrivacyPhase15Run is the Phase 15 run of an administrator who is not the creator, in the
// private menu or in a menu opened in the group, over a group linked to a Staff Group, a channel
// whose title and ID the fake knows and a held member whose name the fake knows. It opens the AI
// rule's screen with the AI not configured (the Off-only form) and configured (three rows), is
// refused a punishing press while it is not, and sets Warn and a Mute through the card while it is.
func settingsPrivacyPhase15Run(t *testing.T, private bool) {
	t.Helper()
	withAINotConfigured(t, mreqAINoKey)
	env := newSettingsEnv(t)
	editCleanup(t, env.chat.Id)
	staffChat := settingsStaffTestOwnedStaffGroup(env, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)

	withFastLookupPacer(t)
	state := newMreqLookupState()
	mreqInstallLookupFake(&memberReqEnv{settingsEnv: env}, state)
	channel := mreqChannel(t, state, env.chat.Id, gotgbot.ChatMemberStatusAdministrator)
	state.setTitle(channel, settingsPrivacyChannelTitle)
	ai := mreqInstallAIFake(t)

	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	held := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: settingsPrivacyMemberName, Username: "zzmarkeruser"}
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id, held.Id})
	env.fake.setMember(env.chat.Id, moderator.Id, staffFakeMember{
		Status: gotgbot.ChatMemberStatusAdministrator, CanRestrictMembers: true,
	})
	env.fake.setProfile(held.Id, held.FirstName, "", held.Username)
	if err := db.DB.Create(&models.User{UserId: held.Id, Name: held.FirstName, UserName: held.Username}).Error; err != nil {
		t.Fatalf("create the held member's users row: %v", err)
	}
	created := time.Now().Add(-time.Hour)
	won, err := memberreq.ClaimHold(context.Background(), &models.ChatRequirementHold{
		ChatID: env.chat.Id, UserID: held.Id, Rules: mreqBitOf(t, "ai"), Action: models.RequirementActionMute,
		CreatedAt: created, PunishedAt: &created, MuteUntil: memberReqMuteUntil(created),
	})
	if err != nil || !won {
		t.Fatalf("seed the held member's hold: won = %t, error = %v", won, err)
	}

	var (
		msgID  int64
		press  func(kv ...string)
		chatID = env.chat.Id
	)
	if private {
		msgID, chatID = 4747, moderator.Id
		press = func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(env.chat.Id, kv...)) }
	} else {
		msgID = env.openMenu(moderator)
		press = func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
	}

	// Not configured: the page, the Off-only screen, the refused Warn and Mute and the Off press.
	press("a", "op", "p", settingsPageMreq)
	press("a", settingsActMreqOpen, "t", "ai")
	press(settingsMreqAIStrictKV(models.RequirementActionWarn)...)
	press(settingsMreqAIStrictKV(models.RequirementActionMute)...)
	press("a", settingsActMreqYes, "t", "ai", "v", "4", "e", settingsCardExpiry(time.Now()))
	press(settingsMreqAIStrictKV(models.RequirementActionOff)...)
	// Configured: the same screen with three rows, a Warn, a Mute card and its Yes, and the page.
	withAIConfigured(t)
	press("a", settingsActMreqOpen, "t", "ai")
	press(settingsMreqAIStrictKV(models.RequirementActionWarn)...)
	press(settingsMreqAIStrictKV(models.RequirementActionMute)...)
	press("a", settingsActMreqYes, "t", "ai", "v", "4", "e", settingsCardExpiry(time.Now()))
	press("a", "op", "p", settingsPageMreq)
	withAINotConfigured(t, mreqAISwitchOff)
	press("a", "op", "p", settingsPageMreq)

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 10 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	if !containsMarker(everything, "settings_mreq_here_ai") {
		t.Errorf("the run never produced settings_mreq_here_ai: %q", everything)
	}
	if !containsMarker(everything, "settings_mreq_confirm_ai_mute") {
		t.Errorf("the run never drew the AI Mute card: %q", everything)
	}
	if stored, ok := settingsMreqStored(t, env.chat.Id); !ok || stored.AIAction != models.RequirementActionMute {
		t.Errorf("stored settings = %+v (found %t), want the AI rule on Mute (set while configured, after the refused presses)", stored, ok)
	}
	if n := ai.count(); n != 0 {
		t.Errorf("the menu made %d provider requests", n)
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"),
		settingsPrivacyChannelTitle, strings.TrimPrefix(strconv.FormatInt(channel, 10), "-"),
		settingsPrivacyMemberName, "zzmarkeruser", strconv.FormatInt(held.Id, 10), mreqLogDescription,
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 15 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}

// settingsPrivacyPhase16Run is the Phase 16 run of an administrator who is not the creator, in the
// private menu or in a menu opened in the group, over a group linked to a Staff Group. It opens the
// Deletion hub, both lists and rows, is refused a change for want of can_change_info, then sets the
// presets with the right, sees every right line, a failed write and the unreadable right, and nothing
// the bot produced names the Staff Group or the member the fake knows.
func settingsPrivacyPhase16Run(t *testing.T, private bool) {
	t.Helper()
	mreqSwap(t, &settingsDelHereTimeout, 50*time.Millisecond)
	env := newSettingsDelEnv(t)
	editCleanup(t, env.chat.Id)
	staffChat := settingsStaffTestOwnedStaffGroup(env.settingsEnv, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)

	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	// A member of the group the fake knows by name: no screen of the page may carry it.
	held := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: settingsPrivacyMemberName, Username: "zzmarkeruser"}
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id, held.Id})
	env.fake.setProfile(held.Id, held.FirstName, "", held.Username)
	env.fake.setMemberRaw(env.chat.Id, moderator.Id, settingsHooksChangeInfoJSON(moderator.Id, false))

	var (
		msgID  int64
		press  func(kv ...string)
		chatID = env.chat.Id
	)
	if private {
		msgID, chatID = 4747, moderator.Id
		press = func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(env.chat.Id, kv...)) }
	} else {
		msgID = env.openMenu(moderator)
		press = func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
	}
	choose := func(code, value string) { press("a", "st", "p", "d", "t", code, "v", value) }

	// Without the right: every screen opens, every change is refused.
	press("a", "op", "p", "d")
	press("a", "dl", "t", "s")
	press("a", "dr", "t", "sn")
	choose("sn", "600")
	press("a", "dl", "t", "b")
	press("a", "dr", "t", "ba")
	choose("ba", "3600")
	// With the right: presets on a service row, a joined row and a bot row, then Keep.
	env.fake.setMemberRaw(env.chat.Id, moderator.Id, settingsHooksChangeInfoJSON(moderator.Id, true))
	press("a", "dr", "t", "sn")
	choose("sn", "600")
	choose("sn", "0")
	press("a", "dr", "t", "sj")
	choose("sj", "600")
	press("a", "dr", "t", "bw")
	choose("bw", "60")
	press("a", "dr", "t", "br")
	choose("br", "3600")
	choose("sn", "n")
	// Every right line the bot can produce, on a screen with a stored rule.
	for _, raw := range []string{settingsDelBotJSON("member", false), settingsDelBotJSON("administrator", false), settingsDelBotJSON("creator", false)} {
		env.setBot(raw)
		press("a", "dl", "t", "s")
		press("a", "dr", "t", "sj")
	}
	env.scriptBot(false, staffFakeError(500, "Internal Server Error"))
	press("a", "dl", "t", "b")
	press("a", "op", "p", "d")
	// A failed write redraws with the notice.
	mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
		Set:    func(context.Context, int64, string, int) error { return errors.New("write failed") },
		Remove: func(context.Context, int64, string) error { return errors.New("write failed") },
	})
	press("a", "dr", "t", "sn")
	choose("sn", "3600")

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 14 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	for _, want := range []string{
		"settings_no_change_info", "settings_del_no_right", "settings_del_right_unknown", "common_settings_save_failed",
		"settings_del_state_after", "settings_del_state_now",
	} {
		if !containsMarker(everything, want) {
			t.Errorf("the run never produced %s: %q", want, everything)
		}
	}
	if delay, ok := env.stored(deletion.KindBotWelcome); !ok || delay != 60 {
		t.Errorf("stored welcome = %d (%v), want the 60 s the permitted press set", delay, ok)
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"), settingsStaffPrivacyDigits,
		settingsPrivacyMemberName, "zzmarkeruser", strconv.FormatInt(held.Id, 10),
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 16 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}

// settingsPrivacyPhase17Run is the Phase 17 run of an administrator who is not the creator, in the
// private menu or in a menu opened in the group, over a group linked to a Staff Group. It opens the
// Member messages section and its rows, is refused every change and the card's Yes for want of
// can_change_info, then sets Commands, Edited messages and Self-destruct (through the card) with the
// right, sees every right line, a failed write and the unreadable right, and nothing the bot produced
// names the Staff Group or the member the fake knows.
func settingsPrivacyPhase17Run(t *testing.T, private bool) {
	t.Helper()
	mreqSwap(t, &settingsDelHereTimeout, 50*time.Millisecond)
	env := newSettingsDelEnv(t)
	editCleanup(t, env.chat.Id)
	staffChat := settingsStaffTestOwnedStaffGroup(env.settingsEnv, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)

	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	held := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: settingsPrivacyMemberName, Username: "zzmarkeruser"}
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id, held.Id})
	env.fake.setProfile(held.Id, held.FirstName, "", held.Username)
	env.fake.setMemberRaw(env.chat.Id, moderator.Id, settingsHooksChangeInfoJSON(moderator.Id, false))

	var (
		msgID  int64
		press  func(kv ...string)
		chatID = env.chat.Id
	)
	if private {
		msgID, chatID = 4748, moderator.Id
		press = func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(env.chat.Id, kv...)) }
	} else {
		msgID = env.openMenu(moderator)
		press = func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
	}
	choose := func(code, value string) { press("a", "st", "p", "d", "t", code, "v", value) }
	yes := func(value string) { press("a", "dy", "t", "ms", "v", value, "e", settingsCardExpiry(time.Now())) }

	// Without the right: the section and every row open, every change and the Yes are refused.
	press("a", "op", "p", "d")
	press("a", "dl", "t", "m")
	for _, code := range []string{"mc", "me", "ms"} {
		press("a", "dr", "t", code)
	}
	choose("mc", "0")
	choose("me", "60")
	choose("ms", "600")
	yes("600")
	// With the right: Commands, Edited messages and Self-destruct through its card, then Off again.
	env.fake.setMemberRaw(env.chat.Id, moderator.Id, settingsHooksChangeInfoJSON(moderator.Id, true))
	press("a", "dr", "t", "mc")
	choose("mc", "0")
	press("a", "dr", "t", "me")
	choose("me", "60")
	choose("me", "900")
	press("a", "dr", "t", "ms")
	choose("ms", "3600")
	yes("3600")
	choose("mc", "n")
	choose("me", "n")
	choose("ms", "86400")
	// Every right line the bot can produce, on the list, a row and a card.
	for _, raw := range []string{settingsDelBotJSON("member", false), settingsDelBotJSON("administrator", false), settingsDelBotJSON("creator", false)} {
		env.setBot(raw)
		press("a", "dl", "t", "m")
		press("a", "dr", "t", "ms")
		choose("ms", "21600")
	}
	env.scriptBot(false, staffFakeError(500, "Internal Server Error"))
	press("a", "dl", "t", "m")
	press("a", "dr", "t", "ms")
	choose("ms", "300")
	// A failed write redraws with the notice; a failed Yes does the same.
	mreqSwap(t, &deletionRuleWrites, deletionRuleWriter{
		Set:    func(context.Context, int64, string, int) error { return errors.New("write failed") },
		Remove: func(context.Context, int64, string) error { return errors.New("write failed") },
	})
	press("a", "dr", "t", "me")
	choose("me", "300")
	yes("600")

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 20 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	for _, want := range []string{
		"settings_no_change_info", "settings_del_no_right", "settings_del_right_unknown", "common_settings_save_failed",
		"settings_del_confirm_self", "settings_del_state_grace", "settings_del_state_after", "settings_del_state_on",
	} {
		if !containsMarker(everything, want) {
			t.Errorf("the run never produced %s: %q", want, everything)
		}
	}
	if delay, ok := env.stored(deletion.KindMemberSelfDestruct); !ok || delay != 3600 {
		t.Errorf("stored self-destruct = %d (%v), want the 3600 s of the confirmed card", delay, ok)
	}
	if delay, ok := env.stored(deletion.KindMemberEdited); ok {
		t.Errorf("stored edited = %d, want Off after the permitted press", delay)
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"), settingsStaffPrivacyDigits,
		settingsPrivacyMemberName, "zzmarkeruser", strconv.FormatInt(held.Id, 10),
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 17 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}

// settingsPrivacyPhase18Screens renders the screens of the six Phase 18 pages through their pure
// renderers for one language, in every view given: Warnings with a stored action, an action the bot
// does not act on and an empty one; Switches in six combinations of its three switches and the AI
// filter's availability; Language with each stored language and an unknown one; the Log channel page
// linked, mixed and not linked; Anonymous admins on and off; both Locks pages with each right line
// and the longest failure notice; and the alerts and notices the six pages can answer with.
func settingsPrivacyPhase18Screens(tr *i18n.Translator, views map[string]settingsView) map[string]settingsScreen {
	screens := make(map[string]settingsScreen)
	press := &settingsPress{tr: tr}
	notices := map[string]string{"plain": "", "save failed": settingsNoticeWarn(settingsText(tr, "common_settings_save_failed"))}
	rights := map[string]string{
		"can":     "",
		"cannot":  "🚫 " + settingsText(tr, "settings_del_no_right"),
		"unknown": settingsUnknownState + " " + settingsText(tr, "settings_del_right_unknown"),
	}
	lockNotice := settingsLocksFailureNotice(press, "locks_lock_failed", settingsLockRows[0])
	mixed := settingsLogState{Linked: true}
	for i := range mixed.On {
		mixed.On[i] = i%2 == 0
	}
	allOn := settingsLogState{Linked: true}
	for i := range allOn.On {
		allOn.On[i] = true
	}
	switchStates := []settingsSwState{
		{AIAvailable: true, Reports: true},
		{AIAvailable: true, AI: true, Approve: true},
		{AIAvailable: true, Reports: true, AI: true, Approve: true},
		{AIAvailable: false},
		{AIAvailable: false, AI: true, Reports: true},
		{AIAvailable: false, Approve: true},
	}
	lockStates := settingsLocksStates()

	for viewName, view := range views {
		for noticeName, notice := range notices {
			at := func(page string) string { return fmt.Sprintf("phase 18 %s %s %s", page, viewName, noticeName) }
			for _, mode := range []string{"ban", "tban", ""} {
				screens[fmt.Sprintf("%s mode %q", at("warnings"), mode)] = renderSettingsWarns(
					view, &models.WarnSettings{WarnLimit: settingsWarnsMaxLimit, WarnMode: mode}, notice)
			}
			for i, state := range switchStates {
				screens[fmt.Sprintf("%s state %d", at("switches"), i)] = renderSettingsSwitches(view, state, notice)
			}
			for _, stored := range append(slices.Clone(settingsLangDefaultCodes), "xx") {
				screens[fmt.Sprintf("%s stored %s", at("language"), stored)] = renderSettingsLanguage(view, stored, notice)
			}
			for name, state := range map[string]settingsLogState{"mixed": mixed, "all on": allOn, "not linked": {}} {
				screens[fmt.Sprintf("%s %s", at("log channel"), name)] = renderSettingsLogChannel(view, state, notice)
			}
			for _, on := range []bool{false, true} {
				screens[fmt.Sprintf("%s on=%v", at("anonymous admins"), on)] = renderSettingsAnonAdmin(view, on, notice)
			}
		}
		// Locks: both pages, the two stored states that change the lines most, each right line, and the
		// longest failure notice with the worst right line.
		for stateName, state := range lockStates {
			if stateName == "none" {
				continue
			}
			for page := 1; page <= settingsLocksPages; page++ {
				for rightName, right := range rights {
					screens[fmt.Sprintf("phase 18 locks %s %s page %d right %s", viewName, stateName, page, rightName)] = renderSettingsLocks(view, state, page, right, "")
				}
				screens[fmt.Sprintf("phase 18 locks %s %s page %d failure", viewName, stateName, page)] = renderSettingsLocks(view, state, page, rights["unknown"], lockNotice)
			}
		}
	}
	for _, key := range []string{
		"settings_sw_ai_unavailable", "settings_no_change_info", "chat_status_owner_button_error", "chat_status_bot_not_admin",
		"connections_is_user_connected_bot_not_admin", "logs_not_set", "logs_unknown_category", "language_invalid_selection",
		"admin_anon_admin_db_error", "settings_min_value", "settings_max_value", "common_settings_save_failed",
		"settings_gate_owner_only", "settings_not_admin", "common_callback_invalid_request",
	} {
		screens["phase 18 alert "+key] = settingsScreen{Text: settingsText(tr, key)}
	}
	return screens
}

// settingsPrivacyPhase18Run is the Phase 18 run of an administrator who is not the creator, in the
// private menu or in a menu opened in the group, over a group linked to a Staff Group, a log channel
// whose title and ID the fake knows and a member whose name the fake knows. It opens each of the six
// pages, makes a permitted press on each (the warn limit and action, Reports, the language, a log
// category on and off and a lock on both Locks pages) and a refused one where the administrator lacks
// the right (Auto-approve for want of can_change_info, the AI filter that cannot run, the Anonymous
// admins toggle that needs the creator, a warn action the bot does not act on), and nothing the bot
// produced names the Staff Group, the log channel or the member.
func settingsPrivacyPhase18Run(t *testing.T, private bool) {
	t.Helper()
	const channelID = int64(-1002468013579)
	withAISpamNotConfigured(t, aispamProviderValueEmpty)
	settingsLangWithCodes(t, settingsLangDefaultCodes)
	env := newSettingsDelEnv(t)
	editCleanup(t, env.chat.Id)
	settingsWarnsCleanup(t, env.chat.Id)
	settingsSwCleanup(t, env.chat.Id)
	settingsAnonCleanup(t, env.chat.Id)
	lockGoldenCleanup(t, env.chat.Id)
	staffChat := settingsStaffTestOwnedStaffGroup(env.settingsEnv, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)
	settingsLogLinkTo(t, env.chat, channelID, settingsPrivacyChannelTitle)

	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: settingsPrivacyMemberName, Username: "zzmarkeruser"}
	memberReqCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id, member.Id})
	settingsLangCleanup(t, []int64{env.chat.Id}, []int64{moderator.Id})
	env.fake.setProfile(member.Id, member.FirstName, "", member.Username)
	env.fake.setMemberRaw(env.chat.Id, moderator.Id, settingsHooksChangeInfoJSON(moderator.Id, false))
	settingsLocksWarmBotRight(env)
	env.setBot(settingsDelBotJSON("administrator", true))
	// A stored action the bot does not act on, so the page draws its inert line before a press replaces it.
	settingsWarnsSeed(t, env.chat.Id, 3, "tban")

	var (
		msgID  int64
		press  func(kv ...string)
		chatID = env.chat.Id
	)
	if private {
		msgID, chatID = 4749, moderator.Id
		press = func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(env.chat.Id, kv...)) }
	} else {
		msgID = env.openMenu(moderator)
		press = func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
	}
	set := func(page, control, value string) { press("a", "st", "p", page, "t", control, "v", value) }
	toggle := func(page, control, value string) { press("a", "tg", "p", page, "t", control, "v", value) }
	open := func(page string) { press("a", "op", "p", page) }
	lockCode := func(kind string) string { return settingsLocksCode(t, kind) }

	open("n")
	set("n", settingsWarnsLimitKey, "5")
	set("n", settingsWarnsLimitKey, "100")
	set("n", settingsWarnsLimitKey, "100")
	set("n", settingsWarnsModeKey, "ban")
	set("n", settingsWarnsModeKey, "tban")

	open("t")
	toggle("t", settingsSwReportsKey, "0")
	toggle("t", settingsSwReportsKey, "1")
	toggle("t", settingsSwAIKey, "1")
	toggle("t", settingsSwApproveKey, "1")

	open("u")
	set("u", settingsLangControlKey, "en")
	set("u", settingsLangControlKey, "uk")
	set("u", settingsLangControlKey, "xx")

	open("o")
	toggle("o", "a", "0")
	toggle("o", "a", "1")
	toggle("o", "s", "0")

	open("y")
	toggle("y", settingsAnonControl, "1")

	open("k")
	toggle("k", lockCode("gif"), "1")
	press("a", settingsActLocksPage, "v", "2")
	toggle("k", settingsLockRows[len(settingsLockRows)-1].Code, "1")
	press("a", settingsActLocksPage, "v", "1")
	toggle("k", lockCode("gif"), "0")
	press("a", "bk")

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 20 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	for _, want := range []string{
		"settings_warns_limit", "settings_warns_action_inert", "warns_mode_unknown", "settings_sw_ai_unavailable",
		"settings_no_change_info", "settings_lang_current", "language_invalid_selection", "settings_log_status",
		"chat_status_owner_button_error", "settings_locks_state_on", "settings_header_main",
	} {
		if !containsMarker(everything, want) {
			t.Errorf("the run never produced %s: %q", want, everything)
		}
	}
	// The permitted presses took effect and the refused ones did not.
	if row, ok := settingsWarnsRow(t, env.chat.Id); !ok || row.WarnLimit != 100 || row.WarnMode != "ban" {
		t.Errorf("warn settings = %+v (stored %v), want limit 100 and ban", row, ok)
	}
	if got := settingsLocksStored(t, env.chat.Id); !strings.Contains(got, settingsLockRows[len(settingsLockRows)-1].Kind+"=true") {
		t.Errorf("locks stored %q, want the lock of page 2 on", got)
	}
	if on, err := settingsAnonRead(context.Background(), env.chat.Id); err != nil || on {
		t.Errorf("anonymous admins = %v (%v), want off after the refused press", on, err)
	}
	if row, ok := settingsEditStored(t, env.chat.Id); ok && row.ShouldAutoApprove {
		t.Error("the refused Auto-approve press was stored")
	}
	if row := logchannels.GetContext(context.Background(), env.chat.Id); row == nil || row.CatSettings {
		t.Errorf("log channel row = %+v, want the settings category off", row)
	}
	// The search is only worth something if the run reached the channel: the audit posts went there,
	// and they are the one thing the search leaves out.
	if posts := settingsLogPosts(env.settingsEnv, channelID); len(posts) < 2 {
		t.Errorf("the run posted %d audit messages to the log channel, want at least 2", len(posts))
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"), settingsStaffPrivacyDigits,
		settingsPrivacyChannelTitle, settingsPrivacyChannelDigits, strconv.FormatInt(channelID, 10),
		settingsPrivacyMemberName, "zzmarkeruser", strconv.FormatInt(member.Id, 10),
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 18 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}

// settingsPrivacyPhase19Screens renders the screens of the three list pages through their pure renderers for one
// language: in a group menu the group note (with and without a bot username), and in the private menu of an administrator
// who is not the creator each page's list (empty, one entry, eight and a middle page of three, with the longest two
// notices and the worst entry), its delete card, its prompts, its refusal replies, its saved page and, for the Blacklist,
// the action screen uniform, mixed, inert and empty. A notice only adds its own length, so two stand for all.
func settingsPrivacyPhase19Screens(tr *i18n.Translator, views map[string]settingsView) map[string]settingsScreen {
	screens := make(map[string]settingsScreen)
	ampersands := strings.Repeat("&", 100)
	until := time.Now()
	entries := func(n int) []settingsListEntry {
		out := make([]settingsListEntry, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, settingsListEntry{ID: int64(10 + i), Text: strings.Repeat("&", 32) + string(rune('a'+i)), Action: "ban", AdminOnly: true})
		}
		return out
	}
	allCounts := settingsBlacklistCounts{Added: 99999, Already: 99999, TooLong: 99999, TooBroad: 99999, Failed: 99999}

	for viewName, view := range views {
		for _, def := range []settingsListDef{settingsBlacklistDef(), settingsFiltersDef(), settingsNotesDef()} {
			at := func(what string) string { return fmt.Sprintf("phase 19 %s %s %s", def.Key, viewName, what) }
			screens[at("group note")] = renderSettingsPrivateOnlyNote(view, def.TitleKey, "FukuTestBot")
			screens[at("group note, no bot username")] = renderSettingsPrivateOnlyNote(view, def.TitleKey, "")
			if !view.Private {
				continue
			}
			first := settingsListFirstPart(def.Key)
			notices := settingsBlacklistLongest([]string{
				settingsNoticeOK(settingsListEntryText(tr, def.RemovedKey, def.EntryParam, ampersands)),
				settingsNoticeWarn(settingsText(tr, "settings_card_expired")),
				settingsNoticeWarn(settingsText(tr, "settings_entries_already_removed")),
				settingsNoticeWarn(settingsText(tr, "common_settings_save_failed")),
				settingsNoticeWarn(settingsText(tr, "connections_is_user_connected_bot_not_admin")),
			}, 2)

			screens[at("empty list")] = renderSettingsList(view, def, nil, 1, "")
			screens[at("one entry")] = renderSettingsList(view, def, entries(1), 1, "")
			for i, notice := range notices {
				screens[at(fmt.Sprintf("eight entries, notice %d", i))] = renderSettingsList(view, def, entries(8), 1, notice)
				screens[at(fmt.Sprintf("middle page, notice %d", i))] = renderSettingsList(view, def, entries(24), 2, notice)
			}
			screens[at("card")] = renderSettingsListCard(view, def, settingsListEntry{ID: 12345678901234567, Text: ampersands}, 9999)
			screens[at("prompt, replaced")] = renderSettingsListPrompt(view, def, first.Code, until, true, "")
			screens[at("prompt")] = renderSettingsListPrompt(view, def, first.Code, until, false, "")
			if first.Next != "" {
				screens[at("step 2 prompt")] = renderSettingsListPrompt(view, def, first.Next, until, true, ampersands)
			}
			for i, reason := range []string{
				settingsText(tr, "settings_entries_refuse_blank"),
				settingsText(tr, "common_settings_save_failed"),
				settingsText(tr, "connections_is_user_connected_bot_not_admin"),
				settingsText(tr, "settings_edit_refuse_need_text"),
			} {
				screens[at(fmt.Sprintf("refusal %d", i))] = renderSettingsInputRefusalCode(view, def.CancelCode, first.Code, reason)
			}
			if def.TwoStep != nil {
				saved := settingsNoticeOK(settingsListEntryText(tr, def.TwoStep.SavedKey, def.EntryParam, ampersands)) + "\n" + settingsText(tr, "settings_edit_album_cut")
				screens[at("saved page")] = renderSettingsList(view, def, entries(8), 1, saved)
			} else {
				screens[at("saved page")] = renderSettingsList(view, def, entries(8), 1, settingsBlacklistSavedNotice(tr, allCounts, true))
				screens[at("refusal of nothing addable")] = renderSettingsInputRefusalCode(view, def.CancelCode, first.Code, settingsBlacklistRefusalText(tr, allCounts))
			}
			if def.Key != settingsPageBlacklist {
				continue
			}
			eight := make([]settingsListEntry, 0, 8)
			for i, action := range []string{"warn", "mute", "kick", "ban", "none", "tban", "tmute", "delete"} {
				eight = append(eight, settingsListEntry{ID: int64(i + 1), Text: fmt.Sprintf("w%d", i), Action: action})
			}
			uniform := entries(3)
			inert := entries(3)
			for i := range inert {
				inert[i].Action = "tban"
			}
			screens[at("action, uniform")] = renderSettingsBlacklistAction(view, uniform, "")
			screens[at("action, mixed")] = renderSettingsBlacklistAction(view, eight, notices[0])
			screens[at("action, inert")] = renderSettingsBlacklistAction(view, inert, "")
			screens[at("action, empty")] = renderSettingsBlacklistAction(view, nil, "")
		}
	}
	return screens
}

// settingsPrivacyPhase19AdminJSON is a getChatMember answer for an administrator who can restrict members and change
// the group's info, the two rights the list pages' writes ask for between them.
func settingsPrivacyPhase19AdminJSON(userID int64) string {
	return fmt.Sprintf(
		`{"status":"administrator","user":{"id":%d,"is_bot":false,"first_name":"Moderator"},"can_be_edited":false,`+
			`"is_anonymous":false,"can_manage_chat":true,"can_delete_messages":false,"can_manage_video_chats":false,`+
			`"can_restrict_members":true,"can_promote_members":false,"can_change_info":true,"can_invite_users":false}`, userID)
}

// settingsPrivacyPhase19Run is the Phase 19 run of an administrator who is not the creator, in the private menu or in a menu
// opened in the group, over a group linked to a Staff Group. In the private menu it opens each list page, asks to delete an
// entry, opens the Blacklist action screen and sets an action, and adds through the typed replies (words, then the two steps
// of Filters and of Notes); in a group menu it opens each list cell, Back, a forged Cancel and every list action. Nothing the
// bot produced names the Staff Group.
func settingsPrivacyPhase19Run(t *testing.T, private bool) {
	t.Helper()
	setCachedBotUsername(t, "FukuTestBot")
	env := newSettingsListEnv(t)
	staffChat := settingsStaffTestOwnedStaffGroup(env.settingsEnv, settingsStaffPrivacyTitle)
	linkLockdownGroupTo(env.lockdownEnv, staffChat)
	chat := env.chat
	moderator := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Moderator"}
	t.Cleanup(func() {
		db.DB.Where("user_id = ?", moderator.Id).Delete(&models.User{})
		dbcache.DeleteCache(dbcache.CacheKey("user", moderator.Id))
		dbcache.ResetLocalForTest()
	})
	env.fake.setMemberRaw(chat.Id, moderator.Id, settingsPrivacyPhase19AdminJSON(moderator.Id))
	env.fake.listAdministrator(chat.Id, moderator.Id)
	settingsListSeedBlacklist(t, chat.Id, "alpha", "beta")
	settingsListSeedFilters(t, chat.Id, "alpha")
	settingsListSeedNotes(t, chat.Id, "alpha")

	var (
		msgID  int64
		chatID = chat.Id
	)
	if private {
		msgID, chatID = env.updateID(), moderator.Id
		press := func(kv ...string) { env.pressPrivate(moderator, msgID, env.privateFields(chat.Id, kv...)) }
		press("a", settingsActOpen, "p", settingsPageBlacklist)
		press("a", "bx", "r", strconv.FormatInt(settingsBlacklistIDOf(t, chat.Id, "alpha"), 10), "v", "1")
		press("a", "bp", "v", "1")
		press("a", settingsActBlacklistOpen)
		press("a", settingsActBlacklistSet, "v", "mute")
		press("a", "bn")
		settingsListDoorSay(env, moderator, "gamma delta")
		press("a", settingsActOpen, "p", settingsPageFilters)
		press("a", "ix", "r", strconv.FormatInt(settingsFiltersIDOf(t, chat.Id, "alpha"), 10), "v", "1")
		press("a", "ig", "v", "1")
		press("a", "ia")
		settingsListDoorSay(env, moderator, "Keyword")
		settingsListDoorSay(env, moderator, "A reply")
		press("a", settingsActOpen, "p", settingsPageNotes)
		press("a", "ex", "r", strconv.FormatInt(settingsNotesIDOf(t, chat.Id, "alpha"), 10), "v", "1")
		press("a", "ea")
		settingsListDoorSay(env, moderator, "Name")
		settingsListDoorSay(env, moderator, "Some content")
		press("a", settingsActBack)
	} else {
		msgID = env.openMenu(moderator)
		press := func(kv ...string) { env.press(moderator, msgID, env.groupFields(moderator, kv...)) }
		for _, page := range settingsPhase19Pages {
			press("a", settingsActOpen, "p", page)
			press("a", settingsActCancel, "p", page)
		}
		for _, row := range settingsPhase19Codes(settingsCardExpiry(time.Now())) {
			kv := []string{"a", row.code}
			for key, value := range row.fields {
				kv = append(kv, key, value)
			}
			press(kv...)
		}
		press("a", settingsActBack)
	}

	var seen []string
	for _, edit := range env.edits(chatID, msgID) {
		seen = append(seen, fmt.Sprint(edit.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(edit.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, sent := range env.fake.sentTo(chatID) {
		seen = append(seen, fmt.Sprint(sent.Params["text"]))
		for _, button := range settingsAllButtons(staffKeyboardOf(sent.Params["reply_markup"])) {
			seen = append(seen, button.Text, button.CallbackData, button.Url)
		}
	}
	for _, answer := range env.fake.callsFor("answerCallbackQuery") {
		seen = append(seen, fmt.Sprint(answer.Params["text"]))
	}
	everything := strings.Join(seen, "\n")

	if n := len(env.edits(chatID, msgID)); n < 5 {
		t.Fatalf("the run edited the menu only %d times", n)
	}
	if private {
		for _, want := range []string{"settings_bl_intro", "settings_bl_action_line", "settings_flt_intro", "settings_notes_intro", "settings_bl_saved_added", "settings_flt_saved", "settings_notes_saved"} {
			if !containsMarker(everything, want) {
				t.Errorf("the run never produced %s: %q", want, everything)
			}
		}
		if got := settingsBlacklistStored(t, chat.Id); got["gamma"] != "mute" || got["delta"] != "mute" || len(got) != 4 {
			t.Errorf("the blacklist = %v, want alpha and beta and the typed gamma and delta, all on mute", got)
		}
		if _, ok := settingsFiltersStored(t, chat.Id)["keyword"]; !ok {
			t.Errorf("the typed filter was not stored")
		}
		if _, ok := settingsNotesStored(t, chat.Id)["name"]; !ok {
			t.Errorf("the typed note was not stored")
		}
	} else if !containsMarker(everything, "settings_entries_group_note") || containsMarker(everything, "settings_bl_count") {
		t.Errorf("the group menu run did not draw only the group note: %q", everything)
	}
	for _, leak := range []string{
		"ZZ Staff", settingsStaffPrivacyTitle, html.EscapeString(settingsStaffPrivacyTitle),
		strings.TrimPrefix(strconv.FormatInt(staffChat.Id, 10), "-"), settingsStaffPrivacyDigits,
	} {
		if strings.Contains(everything, leak) {
			t.Errorf("a Phase 19 run of an admin who is not the creator received %q: %q", leak, everything)
		}
	}
}
