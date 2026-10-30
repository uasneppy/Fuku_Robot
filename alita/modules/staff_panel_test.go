//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// panelGateClient wraps the scripted staff client so a test can slow chosen chats
// down and observe how many Telegram calls were in flight at once.
type panelGateClient struct {
	*staffBotClient

	inflight atomic.Int32
	peak     atomic.Int32

	dmu    sync.Mutex
	delays map[int64]time.Duration
}

func newPanelGateClient() *panelGateClient {
	return &panelGateClient{staffBotClient: newStaffBotClient(), delays: make(map[int64]time.Duration)}
}

func (c *panelGateClient) setDelay(chatID int64, d time.Duration) {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	c.delays[chatID] = d
}

func (c *panelGateClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method == "getChatAdministrators" || method == "getChatMember" {
		n := c.inflight.Add(1)
		defer c.inflight.Add(-1)
		for {
			peak := c.peak.Load()
			if n <= peak || c.peak.CompareAndSwap(peak, n) {
				break
			}
		}
		var chatID int64
		_, _ = fmt.Sscan(fmt.Sprint(params["chat_id"]), &chatID)
		c.dmu.Lock()
		delay := c.delays[chatID]
		c.dmu.Unlock()
		if delay == 0 {
			delay = 5 * time.Millisecond
		}
		time.Sleep(delay)
	}
	return c.staffBotClient.RequestWithContext(ctx, token, method, params, opts)
}

// panelEnv is an ownershipEnv whose bot runs through a panelGateClient.
type panelEnv struct {
	*ownershipEnv
	gate *panelGateClient
}

func newPanelEnv(t *testing.T) *panelEnv {
	t.Helper()
	env := newOwnershipEnv(t)
	gate := &panelGateClient{staffBotClient: env.client, delays: make(map[int64]time.Duration)}
	env.bot.BotClient = gate
	return &panelEnv{ownershipEnv: env, gate: gate}
}

func (e *panelEnv) setBotRole(chatID int64, role string) {
	e.client.smu.Lock()
	defer e.client.smu.Unlock()
	e.client.botRole[chatID] = role
}

func (e *panelEnv) setMember(chatID, userID int64, status string) {
	e.client.smu.Lock()
	defer e.client.smu.Unlock()
	e.client.members[[2]int64{chatID, userID}] = status
}

func (e *panelEnv) staffChat() gotgbot.Chat { return staffSupergroup(e.staffID) }

func (e *panelEnv) owner() gotgbot.User { return gotgbot.User{Id: e.ownerID, FirstName: "Owner"} }

// openPanel runs /staff in the Staff Group as the owner.
func (e *panelEnv) openPanel(t *testing.T) {
	t.Helper()
	ctx := newModuleMessageContext(e.bot, e.staffChat(), e.owner(), "/staff")
	if err := runStaffCommand(t, e.bot, ctx, staffDesc, staffModule.staffPanel); err != ext.EndGroups {
		t.Fatalf("/staff returned %v, want ext.EndGroups", err)
	}
}

// press delivers a panel callback with the given raw data from user in chat.
func (e *panelEnv) press(t *testing.T, chat gotgbot.Chat, from gotgbot.User, action, page string) {
	t.Helper()
	data := encodeCallbackData(staffCallbackNamespace, map[string]string{"a": action, "p": page})
	ctx := newModuleCallbackContext(e.bot, chat, from, data)
	if err := staffModule.staffCallback(e.bot, ctx); err != ext.EndGroups {
		t.Fatalf("staffCallback(%q) returned %v, want ext.EndGroups", action, err)
	}
}

// panelTexts returns the texts sent to the Staff Group that are the panel itself
// (they carry the chat ID line), as opposed to notices.
func (e *panelEnv) panelTexts() []string {
	var panels []string
	for _, text := range textsToChat(e.client, e.staffID) {
		if strings.Contains(text, staffMarker("staff_panel_chat_id")) {
			panels = append(panels, text)
		}
	}
	return panels
}

// noticesWith returns how many texts sent to the Staff Group carry key.
func (e *panelEnv) noticesWith(key string) int {
	n := 0
	for _, text := range textsToChat(e.client, e.staffID) {
		if strings.Contains(text, staffMarker(key)) {
			n++
		}
	}
	return n
}

func (e *panelEnv) edits() []moduleBotCall { return e.client.callsFor("editMessageText") }

func (e *panelEnv) answers() []moduleBotCall { return e.client.callsFor("answerCallbackQuery") }

func panelEditText(call moduleBotCall) string { return fmt.Sprint(call.Params["text"]) }

// panelRowBlock returns the block of text that starts at title and runs to the
// next blank line, which is one panel row.
func panelRowBlock(text, title string) string {
	start := strings.Index(text, title)
	if start < 0 {
		return ""
	}
	block := text[start:]
	if end := strings.Index(block, "\n\n"); end >= 0 {
		block = block[:end]
	}
	return block
}

// panelErrorHook records every log entry at error level or worse.
type panelErrorHook struct {
	mu      sync.Mutex
	entries []string
}

func (h *panelErrorHook) Levels() []log.Level {
	return []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel}
}

func (h *panelErrorHook) Fire(entry *log.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = append(h.entries, entry.Message)
	return nil
}

func (h *panelErrorHook) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.entries...)
}

// panelCaptureErrors swaps the standard logger's hooks for one that records
// error-level entries, and restores them when the test ends.
func panelCaptureErrors(t *testing.T) *panelErrorHook {
	t.Helper()
	hook := &panelErrorHook{}
	hooks := log.LevelHooks{}
	hooks.Add(hook)
	previous := log.StandardLogger().ReplaceHooks(hooks)
	t.Cleanup(func() { log.StandardLogger().ReplaceHooks(previous) })
	return hook
}

func TestStaffPanelLiveStatuses(t *testing.T) {
	env := newPanelEnv(t)
	g1 := env.addGroup(t, env.ownerID, "Alpha")
	g2 := env.addGroup(t, env.ownerID, "Bravo")
	g3 := env.addGroup(t, env.ownerID, "Charlie")
	g4 := env.addGroup(t, env.ownerID, "Delta")

	env.setBotRole(g2.GroupChatID, staffRoleMember)
	env.client.setCreator(g3.GroupChatID, env.ownerID+1)
	env.client.setFailure("getChatAdministrators", g4.GroupChatID, errors.New("boom"))

	env.openPanel(t)

	panels := env.panelTexts()
	if len(panels) != 1 {
		t.Fatalf("%d panels were sent, want 1", len(panels))
	}
	panel := panels[0]

	idx := func(title string) int { return strings.Index(panel, title) }
	if idx("Alpha") < 0 || idx("Bravo") < 0 || idx("Delta") < 0 {
		t.Fatalf("panel %q must list Alpha, Bravo and Delta", panel)
	}
	if !(idx("Alpha") < idx("Bravo") && idx("Bravo") < idx("Delta")) {
		t.Errorf("panel %q must list the links in link-id order", panel)
	}
	if idx("Charlie") >= 0 {
		t.Errorf("panel %q must not list Charlie, whose owner changed", panel)
	}

	if block := panelRowBlock(panel, "Alpha"); strings.Contains(block, staffMarker("staff_panel_reason_bot_not_admin")) ||
		strings.Contains(block, staffMarker("staff_panel_reason_owner_unknown")) {
		t.Errorf("healthy row %q must carry no reason", block)
	}
	if block := panelRowBlock(panel, "Bravo"); !strings.Contains(block, staffMarker("staff_panel_reason_bot_not_admin")) {
		t.Errorf("Bravo row %q must carry staff_panel_reason_bot_not_admin", block)
	}
	if block := panelRowBlock(panel, "Delta"); !strings.Contains(block, staffMarker("staff_panel_reason_owner_unknown")) {
		t.Errorf("Delta row %q must carry staff_panel_reason_owner_unknown", block)
	}

	if n := env.noticesWith("staff_notice_unlinked_group_owner_changed"); n != 1 {
		t.Errorf("%d owner-changed notices, want exactly 1", n)
	}
	if n := env.noticesWith("staff_notice_health_bot_not_admin"); n != 1 {
		t.Errorf("%d bot-not-admin heads-ups, want exactly 1", n)
	}
	wantLinkGone(t, g3.GroupChatID)
	wantLinkKept(t, g4.GroupChatID)
	if got := wantHealth(t, g2.GroupChatID, models.StaffHealthBotNotAdmin); got == nil {
		t.Fatal("Bravo's link must remain")
	}
	wantHealth(t, g1.GroupChatID, models.StaffHealthOK)
}

func TestStaffPanelRefreshInPlace(t *testing.T) {
	env := newPanelEnv(t)
	env.addGroup(t, env.ownerID, "Alpha")
	g2 := env.addGroup(t, env.ownerID, "Bravo")
	errs := panelCaptureErrors(t)

	env.press(t, env.staffChat(), env.owner(), staffActRefresh, "0")
	if edits := env.edits(); len(edits) != 1 {
		t.Fatalf("%d edits after the first Refresh, want exactly one", len(edits))
	} else if text := panelEditText(edits[0]); !strings.Contains(text, "Alpha") || !strings.Contains(text, staffMarker("staff_panel_updated")) {
		t.Errorf("refreshed panel %q must list the groups and the Updated line", text)
	}
	if sent := env.client.callsFor("sendMessage"); len(sent) != 0 {
		t.Fatalf("Refresh sent %d messages, want none (edit in place)", len(sent))
	}
	if answers := env.answers(); len(answers) != 1 {
		t.Fatalf("Refresh answered %d times, want exactly once", len(answers))
	}

	// A change found by Refresh posts one heads-up; pressing again posts no more.
	env.setBotRole(g2.GroupChatID, staffRoleMember)
	env.press(t, env.staffChat(), env.owner(), staffActRefresh, "0")
	if n := env.noticesWith("staff_notice_health_bot_not_admin"); n != 1 {
		t.Fatalf("%d heads-ups after the change, want 1", n)
	}
	env.press(t, env.staffChat(), env.owner(), staffActRefresh, "0")
	if n := env.noticesWith("staff_notice_health_bot_not_admin"); n != 1 {
		t.Fatalf("%d heads-ups after a repeat Refresh, want still 1", n)
	}
	if edits := env.edits(); len(edits) != 3 {
		t.Fatalf("%d edits after three Refreshes, want 3", len(edits))
	}

	// Telegram's "message is not modified" answer counts as success.
	env.client.setFailure("editMessageText", env.staffID, &gotgbot.TelegramError{
		Method:      "editMessageText",
		Code:        400,
		Description: "Bad Request: message is not modified: specified new message content and reply markup are exactly the same",
	})
	before := len(env.answers())
	env.press(t, env.staffChat(), env.owner(), staffActRefresh, "0")
	if got := len(env.answers()); got != before+1 {
		t.Fatalf("answers went from %d to %d, want the unmodified press answered once", before, got)
	}
	if messages := errs.messages(); len(messages) != 0 {
		t.Errorf("error-level log entries %q, want none for a not-modified edit", messages)
	}
}

func TestStaffPanelRefreshNonMember(t *testing.T) {
	env := newPanelEnv(t)
	env.addGroup(t, env.ownerID, "Alpha")
	const strangerID int64 = 6161
	env.setMember(env.staffID, strangerID, "left")

	env.press(t, env.staffChat(), gotgbot.User{Id: strangerID, FirstName: "Stranger"}, staffActRefresh, "0")

	if edits := env.edits(); len(edits) != 0 {
		t.Fatalf("a non-member's Refresh edited the panel %d times, want none", len(edits))
	}
	answers := env.answers()
	if len(answers) != 1 {
		t.Fatalf("answered %d times, want exactly once", len(answers))
	}
	alert, _ := answers[0].Params["show_alert"].(bool)
	if !alert || !strings.Contains(fmt.Sprint(answers[0].Params["text"]), staffMarker("staff_cb_members_only")) {
		t.Fatalf("answer = %+v, want a members-only alert", answers[0].Params)
	}
	if got := len(env.client.callsFor("getChatAdministrators")); got != 0 {
		t.Errorf("a refused press made %d owner lookups, want none", got)
	}
}

func TestStaffPanelRefreshOutsideStaffGroup(t *testing.T) {
	env := newPanelEnv(t)
	env.addGroup(t, env.ownerID, "Alpha")
	elsewhere := staffSupergroup(uniqueModuleChatID())

	env.press(t, elsewhere, env.owner(), staffActRefresh, "0")

	if edits := env.edits(); len(edits) != 0 {
		t.Fatalf("a press outside a Staff Group edited %d messages, want none", len(edits))
	}
	answers := env.answers()
	if len(answers) != 1 || !strings.Contains(fmt.Sprint(answers[0].Params["text"]), staffMarker("staff_cb_expired")) {
		t.Fatalf("answers = %+v, want one staff_cb_expired", answers)
	}
}

func TestStaffPanelCallBudget(t *testing.T) {
	const links = 5
	env := newPanelEnv(t)
	for i := 0; i < links; i++ {
		env.addGroup(t, env.ownerID, "Group"+strconv.Itoa(i))
	}

	env.openPanel(t)

	lookups := len(env.client.callsFor("getChatAdministrators")) + len(env.client.callsFor("getChatMember"))
	if lookups > 2*links+1 {
		t.Fatalf("one /staff made %d lookups for %d links, want at most %d", lookups, links, 2*links+1)
	}
	if got := len(env.panelTexts()); got != 1 {
		t.Fatalf("%d panels were sent, want 1", got)
	}
}

func TestStaffPanelConcurrencyLimit(t *testing.T) {
	const links = 12
	env := newPanelEnv(t)
	for i := 0; i < links; i++ {
		env.addGroup(t, env.ownerID, "Group"+strconv.Itoa(i))
	}

	env.openPanel(t)

	if peak := env.gate.peak.Load(); peak > 4 {
		t.Fatalf("%d Telegram lookups were in flight at once, want at most 4", peak)
	}
}

func TestStaffPanelLiveStableOrder(t *testing.T) {
	env := newPanelEnv(t)
	titles := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"}
	for i, title := range titles {
		link := env.addGroup(t, env.ownerID, title)
		// Earlier links answer slowest, so the checks finish in reverse order.
		env.gate.setDelay(link.GroupChatID, time.Duration(len(titles)-i)*15*time.Millisecond)
	}

	env.openPanel(t)

	panels := env.panelTexts()
	if len(panels) != 1 {
		t.Fatalf("%d panels were sent, want 1", len(panels))
	}
	last := -1
	for _, title := range titles {
		at := strings.Index(panels[0], title)
		if at < 0 || at < last {
			t.Fatalf("panel %q lists %s out of link-id order", panels[0], title)
		}
		last = at
	}
}

func TestStaffPanelRowsCancelledContext(t *testing.T) {
	env := newPanelEnv(t)
	env.addGroup(t, env.ownerID, "Alpha")
	env.addGroup(t, env.ownerID, "Bravo")
	links, err := staff.ListLinksByStaffFresh(env.staffID)
	if err != nil || len(links) != 2 {
		t.Fatalf("links = (%+v, %v), want 2", links, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rows := buildStaffPanelRows(ctx, env.bot, links)

	if len(rows) != 2 {
		t.Fatalf("%d rows, want both links kept when the build is cancelled", len(rows))
	}
	for _, row := range rows {
		if row.HealthKnown || row.OwnerState != chat_status.OwnerUnknown {
			t.Errorf("row %+v must show unknown statuses", row)
		}
	}
	if got := len(env.client.callsFor("getChatAdministrators")) + len(env.client.callsFor("getChatMember")); got != 0 {
		t.Errorf("a cancelled build made %d lookups, want none", got)
	}
}
