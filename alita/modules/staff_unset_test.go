//go:build testtools

package modules

import (
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// unsetFixture is a Staff Group with live creator ownerID and the given linked
// group titles, wired to a scripted bot.
type unsetFixture struct {
	client  *staffBotClient
	bot     *gotgbot.Bot
	staffID int64
	ownerID int64
	groups  []int64
}

func newUnsetFixture(t *testing.T, titles ...string) *unsetFixture {
	t.Helper()
	withStaffLocale(t)

	f := &unsetFixture{
		client:  newStaffBotClient(),
		staffID: uniqueModuleChatID(),
		ownerID: 4242,
	}
	f.bot = newModuleTestBot(f.client.moduleBotClient)
	f.bot.BotClient = f.client
	f.client.setCreator(f.staffID, f.ownerID)

	ids := []int64{f.staffID}
	for range titles {
		f.groups = append(f.groups, uniqueModuleChatID())
	}
	ids = append(ids, f.groups...)
	staffCleanup(t, ids...)

	if _, err := staff.CreateStaffGroup(f.staffID, f.ownerID, "Staff HQ"); err != nil {
		t.Fatalf("CreateStaffGroup: %v", err)
	}
	for i, title := range titles {
		link := &models.StaffGroupLink{
			GroupChatID: f.groups[i],
			StaffChatID: f.staffID,
			OwnerUserID: f.ownerID,
			GroupTitle:  title,
		}
		if err := db.DB.Create(link).Error; err != nil {
			t.Fatalf("create link: %v", err)
		}
	}
	return f
}

func (f *unsetFixture) chat() gotgbot.Chat { return staffSupergroup(f.staffID) }

func (f *unsetFixture) command(t *testing.T, from gotgbot.User) {
	t.Helper()
	ctx := newModuleMessageContext(f.bot, f.chat(), from, "/unsetstaff")
	if err := runStaffCommand(t, f.bot, ctx, unsetStaffDesc, staffModule.unsetStaff); err != ext.EndGroups {
		t.Fatalf("/unsetstaff returned %v, want ext.EndGroups", err)
	}
}

func (f *unsetFixture) press(t *testing.T, from gotgbot.User, action string) {
	t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": action})
	ctx := newModuleCallbackContext(f.bot, f.chat(), from, data)
	if err := staffModule.staffCallback(f.bot, ctx); err != ext.EndGroups {
		t.Fatalf("staffCallback(%q) returned %v, want ext.EndGroups", action, err)
	}
}

func (f *unsetFixture) rowsLeft(t *testing.T) (groups, links int64) {
	t.Helper()
	if err := db.DB.Model(&models.StaffGroup{}).Where("chat_id = ?", f.staffID).Count(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.DB.Model(&models.StaffGroupLink{}).Where("staff_chat_id = ?", f.staffID).Count(&links).Error; err != nil {
		t.Fatal(err)
	}
	return groups, links
}

func (f *unsetFixture) editTexts() []string {
	var texts []string
	for _, call := range f.client.callsFor("editMessageText") {
		texts = append(texts, fmt.Sprint(call.Params["text"]))
	}
	return texts
}

func (f *unsetFixture) answers() []moduleBotCall {
	return f.client.callsFor("answerCallbackQuery")
}

func TestUnsetStaffConfirmRemovesStatusAndLinks(t *testing.T) {
	f := newUnsetFixture(t, "Alpha <b>", "100%d")
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}

	f.command(t, owner)
	texts := sentTexts(f.client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_unset_confirm_prompt")) ||
		!strings.Contains(texts[0], " 2") {
		t.Fatalf("prompt = %q, want staff_unset_confirm_prompt with count 2", texts)
	}
	markup := fmt.Sprint(f.client.callsFor("sendMessage")[0].Params["reply_markup"])
	for _, want := range []string{
		"staff|v1|a=uy", "staff|v1|a=un",
		staffMarker("staff_btn_confirm"), staffMarker("staff_btn_cancel"),
	} {
		if !strings.Contains(markup, want) {
			t.Errorf("prompt keyboard %s missing %q", markup, want)
		}
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 2 {
		t.Fatalf("asking for confirmation changed rows: groups=%d links=%d", groups, links)
	}

	f.press(t, owner, staffActUnsetConfirm)

	edits := f.editTexts()
	if len(edits) != 1 || !strings.Contains(edits[0], staffMarker("staff_unset_done")) ||
		!strings.Contains(edits[0], "staff-unset-done@@ 2 ") {
		t.Fatalf("edits = %q, want one staff_unset_done with count 2", edits)
	}
	first := fmt.Sprintf("Alpha &lt;b&gt; <code>%d</code>", f.groups[0])
	second := fmt.Sprintf("100%%d <code>%d</code>", f.groups[1])
	i, j := strings.Index(edits[0], first), strings.Index(edits[0], second)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("edit %q must list %q then %q (escaped titles, link-id order, no printf mangling)", edits[0], first, second)
	}
	if groups, links := f.rowsLeft(t); groups != 0 || links != 0 {
		t.Fatalf("rows after confirm: groups=%d links=%d, want 0 and 0", groups, links)
	}
	if got := len(f.answers()); got != 1 {
		t.Fatalf("callback answered %d times, want exactly 1", got)
	}
}

func TestUnsetStaffRecordedOwnerWhoIsNoLongerCreatorIsRefused(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	oldOwner := gotgbot.User{Id: f.ownerID, FirstName: "OldOwner"}
	const newCreatorID int64 = 7777
	newCreator := gotgbot.User{Id: newCreatorID, FirstName: "NewCreator"}
	f.client.setCreator(f.staffID, newCreatorID) // ownership was transferred

	f.command(t, oldOwner)
	texts := sentTexts(f.client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_refuse_not_owner")) {
		t.Fatalf("recorded owner replies = %q, want staff_refuse_not_owner", texts)
	}

	f.press(t, oldOwner, staffActUnsetConfirm)
	if got := f.editTexts(); len(got) != 0 {
		t.Fatalf("recorded owner's Confirm edited the message: %q", got)
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 1 {
		t.Fatalf("recorded owner changed rows: groups=%d links=%d", groups, links)
	}

	f.command(t, newCreator)
	texts = sentTexts(f.client)
	if len(texts) != 2 || !strings.Contains(texts[1], staffMarker("staff_unset_confirm_prompt")) {
		t.Fatalf("live creator replies = %q, want the confirm prompt", texts)
	}
	f.press(t, newCreator, staffActUnsetConfirm)
	if groups, links := f.rowsLeft(t); groups != 0 || links != 0 {
		t.Fatalf("live creator's Confirm left rows: groups=%d links=%d", groups, links)
	}
}

func TestUnsetStaffConfirmByNonCreatorGetsAlert(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	intruder := gotgbot.User{Id: 5151, FirstName: "Intruder"}

	f.press(t, intruder, staffActUnsetConfirm)

	answers := f.answers()
	if len(answers) != 1 {
		t.Fatalf("answered %d times, want 1", len(answers))
	}
	if text := fmt.Sprint(answers[0].Params["text"]); !strings.Contains(text, staffMarker("staff_cb_owner_only")) {
		t.Fatalf("answer text = %q, want staff_cb_owner_only", text)
	}
	if alert, _ := answers[0].Params["show_alert"].(bool); !alert {
		t.Fatalf("answer params = %v, want show_alert true", answers[0].Params)
	}
	if got := f.editTexts(); len(got) != 0 {
		t.Fatalf("non-creator's Confirm edited the message: %q", got)
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 1 {
		t.Fatalf("non-creator changed rows: groups=%d links=%d", groups, links)
	}
}

func TestUnsetStaffConfirmWithFailingOwnerCheckChangesNothing(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}
	f.client.setFailure("getChatAdministrators", f.staffID, &gotgbot.TelegramError{
		Method: "getChatAdministrators", Code: 429, Description: "Too Many Requests: retry after 5",
	})

	f.press(t, owner, staffActUnsetConfirm)

	answers := f.answers()
	if len(answers) != 1 || !strings.Contains(fmt.Sprint(answers[0].Params["text"]), staffMarker("staff_check_failed")) {
		t.Fatalf("answers = %v, want one staff_check_failed", answers)
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 1 {
		t.Fatalf("an unverifiable press removed rows: groups=%d links=%d", groups, links)
	}
}

func TestUnsetStaffDoubleConfirmRemovesOnce(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}

	f.press(t, owner, staffActUnsetConfirm)
	f.press(t, owner, staffActUnsetConfirm)

	edits := f.editTexts()
	if len(edits) != 1 || !strings.Contains(edits[0], staffMarker("staff_unset_done")) {
		t.Fatalf("edits = %q, want exactly one staff_unset_done", edits)
	}
	answers := f.answers()
	if len(answers) != 2 {
		t.Fatalf("answered %d times, want 2 (once per press)", len(answers))
	}
	if text := fmt.Sprint(answers[1].Params["text"]); !strings.Contains(text, staffMarker("staff_unset_already_removed")) {
		t.Fatalf("second answer = %q, want staff_unset_already_removed", text)
	}
}

func TestUnsetStaffCancelKeepsEverything(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}

	f.press(t, owner, staffActUnsetCancel)

	edits := f.editTexts()
	if len(edits) != 1 || !strings.Contains(edits[0], staffMarker("staff_unset_cancelled")) {
		t.Fatalf("edits = %q, want one staff_unset_cancelled", edits)
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 1 {
		t.Fatalf("cancel changed rows: groups=%d links=%d", groups, links)
	}
	if got := len(f.answers()); got != 1 {
		t.Fatalf("answered %d times, want 1", got)
	}
}

func TestUnsetStaffUnknownActionExpires(t *testing.T) {
	f := newUnsetFixture(t, "Alpha")
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}

	f.press(t, owner, "zz")

	answers := f.answers()
	if len(answers) != 1 || !strings.Contains(fmt.Sprint(answers[0].Params["text"]), staffMarker("staff_cb_expired")) {
		t.Fatalf("answers = %v, want one staff_cb_expired", answers)
	}
	if groups, links := f.rowsLeft(t); groups != 1 || links != 1 {
		t.Fatalf("unknown action changed rows: groups=%d links=%d", groups, links)
	}
}

func TestUnsetStaffZeroLinksReportsZero(t *testing.T) {
	f := newUnsetFixture(t)
	owner := gotgbot.User{Id: f.ownerID, FirstName: "Owner"}

	f.command(t, owner)
	texts := sentTexts(f.client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_unset_confirm_prompt")) ||
		!strings.Contains(texts[0], " 0") {
		t.Fatalf("prompt = %q, want the confirm prompt with count 0", texts)
	}

	f.press(t, owner, staffActUnsetConfirm)
	edits := f.editTexts()
	if len(edits) != 1 || !strings.Contains(edits[0], "staff-unset-done@@ 0") {
		t.Fatalf("edits = %q, want staff_unset_done with count 0", edits)
	}
	if groups, _ := f.rowsLeft(t); groups != 0 {
		t.Fatalf("staff_groups rows left = %d, want 0", groups)
	}
}

func TestUnsetStaffInNonStaffChatWritesNothing(t *testing.T) {
	withStaffLocale(t)
	client := newStaffBotClient()
	bot := newModuleTestBot(client.moduleBotClient)
	bot.BotClient = client
	chatID := uniqueModuleChatID()
	staffCleanup(t, chatID)
	const ownerID int64 = 4242
	client.setCreator(chatID, ownerID)

	ctx := newModuleMessageContext(bot, staffSupergroup(chatID), gotgbot.User{Id: ownerID, FirstName: "Owner"}, "/unsetstaff")
	if err := runStaffCommand(t, bot, ctx, unsetStaffDesc, staffModule.unsetStaff); err != ext.EndGroups {
		t.Fatalf("/unsetstaff returned %v, want ext.EndGroups", err)
	}

	texts := sentTexts(client)
	if len(texts) != 1 || !strings.Contains(texts[0], staffMarker("staff_unset_not_staff")) {
		t.Fatalf("replies = %q, want staff_unset_not_staff", texts)
	}
	if calls := client.callsFor("getChatAdministrators"); len(calls) != 0 {
		t.Fatalf("a non-Staff chat triggered %d getChatAdministrators calls, want 0", len(calls))
	}
	if row, err := staff.GetStaffGroupFresh(chatID); err != nil || row != nil {
		t.Fatalf("row after /unsetstaff = (%+v, %v), want (nil, nil)", row, err)
	}
}
