//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"gopkg.in/yaml.v3"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// Bot roles accepted by staffBotClient.botRole.
const (
	staffRoleAdmin           = "admin"
	staffRoleAdminNoRestrict = "admin_norestrict"
	staffRoleMember          = "member"
	staffRoleLeft            = "left"
	staffRoleKicked          = "kicked"
	staffRoleAbsent          = "absent"
)

const staffTestBotID int64 = 999

// staffBotClient is a hand-written gotgbot.BotClient fake that answers
// getChatAdministrators and getChatMember per chat, so Staff Group tests can
// script creators, bot roles, member statuses and Telegram errors. Every other
// method falls through to the embedded moduleBotClient.
type staffBotClient struct {
	*moduleBotClient

	smu sync.Mutex
	// creators maps a chat to its creator's user ID; absent means the admin
	// list has no creator.
	creators map[int64]int64
	// botRole maps a chat to the bot's role (staffRole*); the default is admin.
	botRole map[int64]string
	// members maps (chat, user) to a getChatMember status.
	members map[[2]int64]string
	// failures maps "method:chatID" to a scripted error.
	failures map[string]error
}

func newStaffBotClient() *staffBotClient {
	return &staffBotClient{
		moduleBotClient: newModuleBotClient(),
		creators:        make(map[int64]int64),
		botRole:         make(map[int64]string),
		members:         make(map[[2]int64]string),
		failures:        make(map[string]error),
	}
}

func (c *staffBotClient) setCreator(chatID, userID int64) {
	c.smu.Lock()
	defer c.smu.Unlock()
	c.creators[chatID] = userID
}

func (c *staffBotClient) setFailure(method string, chatID int64, err error) {
	c.smu.Lock()
	defer c.smu.Unlock()
	c.failures[fmt.Sprintf("%s:%d", method, chatID)] = err
}

func (c *staffBotClient) record(method string, params map[string]any) {
	copied := make(map[string]any, len(params))
	for key, value := range params {
		copied[key] = value
	}
	c.moduleBotClient.mu.Lock()
	defer c.moduleBotClient.mu.Unlock()
	c.moduleBotClient.calls = append(c.moduleBotClient.calls, moduleBotCall{Method: method, Params: copied})
}

func staffMemberJSON(userID int64, isBot bool, status string, canRestrict bool) json.RawMessage {
	user := fmt.Sprintf(`{"id":%d,"is_bot":%t,"first_name":"User%d"}`, userID, isBot, userID)
	switch status {
	case "creator":
		return json.RawMessage(fmt.Sprintf(`{"status":"creator","user":%s}`, user))
	case "administrator":
		return json.RawMessage(fmt.Sprintf(
			`{"status":"administrator","user":%s,"can_restrict_members":%t,"can_delete_messages":true,"can_invite_users":true,"can_manage_chat":true}`,
			user, canRestrict))
	case "restricted":
		return json.RawMessage(fmt.Sprintf(`{"status":"restricted","user":%s,"is_member":true}`, user))
	default:
		return json.RawMessage(fmt.Sprintf(`{"status":%q,"user":%s}`, status, user))
	}
}

func (c *staffBotClient) botMemberStatus(chatID int64) (status string, canRestrict bool, present bool) {
	role, ok := c.botRole[chatID]
	if !ok {
		role = staffRoleAdmin
	}
	switch role {
	case staffRoleAdmin:
		return "administrator", true, true
	case staffRoleAdminNoRestrict:
		return "administrator", false, true
	case staffRoleMember:
		return "member", false, true
	case staffRoleLeft:
		return "left", false, true
	case staffRoleKicked:
		return "kicked", false, true
	default:
		return "", false, false
	}
}

func (c *staffBotClient) adminList(chatID int64) json.RawMessage {
	entries := make([]string, 0, 2)
	if creator, ok := c.creators[chatID]; ok {
		entries = append(entries, string(staffMemberJSON(creator, false, "creator", false)))
	}
	if status, canRestrict, present := c.botMemberStatus(chatID); present && status == "administrator" {
		entries = append(entries, string(staffMemberJSON(staffTestBotID, true, "administrator", canRestrict)))
	}
	return json.RawMessage("[" + strings.Join(entries, ",") + "]")
}

func (c *staffBotClient) chatMember(chatID, userID int64) (json.RawMessage, error) {
	if userID == staffTestBotID {
		status, canRestrict, present := c.botMemberStatus(chatID)
		if !present {
			return nil, &gotgbot.TelegramError{
				Method:      "getChatMember",
				Code:        403,
				Description: "Forbidden: bot is not a member of the supergroup chat",
			}
		}
		return staffMemberJSON(userID, true, status, canRestrict), nil
	}
	if status, ok := c.members[[2]int64{chatID, userID}]; ok {
		return staffMemberJSON(userID, false, status, true), nil
	}
	if creator, ok := c.creators[chatID]; ok && creator == userID {
		return staffMemberJSON(userID, false, "creator", false), nil
	}
	return staffMemberJSON(userID, false, "member", false), nil
}

// RequestWithContext implements gotgbot.BotClient. Scripted failures win, then
// the per-chat administrator and member answers; every call is recorded exactly
// once so callsFor keeps working.
func (c *staffBotClient) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	if method != "getChatAdministrators" && method != "getChatMember" {
		c.smu.Lock()
		failure := c.failures[fmt.Sprintf("%s:%v", method, params["chat_id"])]
		c.smu.Unlock()
		if failure != nil {
			c.record(method, params)
			return nil, failure
		}
		return c.moduleBotClient.RequestWithContext(ctx, token, method, params, opts)
	}

	c.record(method, params)

	c.smu.Lock()
	defer c.smu.Unlock()
	if failure := c.failures[fmt.Sprintf("%s:%v", method, params["chat_id"])]; failure != nil {
		return nil, failure
	}
	var chatID int64
	if _, err := fmt.Sscan(fmt.Sprint(params["chat_id"]), &chatID); err != nil {
		return nil, fmt.Errorf("staffBotClient: bad chat_id %v: %w", params["chat_id"], err)
	}
	if method == "getChatAdministrators" {
		return c.adminList(chatID), nil
	}
	var userID int64
	if _, err := fmt.Sscan(fmt.Sprint(params["user_id"]), &userID); err != nil {
		return nil, fmt.Errorf("staffBotClient: bad user_id %v: %w", params["user_id"], err)
	}
	return c.chatMember(chatID, userID)
}

// staffMarker returns a string that survives markdown-to-HTML conversion and
// identifies a locale key in sent text.
func staffMarker(key string) string {
	return "@@" + strings.ReplaceAll(key, "_", "-") + "@@"
}

var staffPlaceholderRe = regexp.MustCompile(`\{\w+\}`)

// withStaffLocale replaces the translator's locale data with every en.yml string
// swapped for its staffMarker plus the placeholders it carried, so tests can
// assert which key a reply used and that its params were interpolated.
func withStaffLocale(t *testing.T) {
	t.Helper()

	raw, err := os.ReadFile("../../locales/en.yml")
	if err != nil {
		t.Fatalf("read en.yml: %v", err)
	}
	var data map[string]any
	if err := yaml.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse en.yml: %v", err)
	}
	marked := make(map[string]any, len(data))
	for key, value := range data {
		text, ok := value.(string)
		if !ok {
			marked[key] = value
			continue
		}
		parts := append([]string{staffMarker(key)}, staffPlaceholderRe.FindAllString(text, -1)...)
		marked[key] = strings.Join(parts, " ")
	}
	out, err := yaml.Marshal(marked)
	if err != nil {
		t.Fatalf("marshal marker locale: %v", err)
	}
	restore, err := i18n.OverrideManagerForTest(string(out))
	if err != nil {
		t.Fatalf("override locale manager: %v", err)
	}
	t.Cleanup(restore)
}

// runStaffCommand mirrors helpers.WrapCommand without a dispatcher: it builds the
// command context, runs the descriptor's checks, then the handler.
func runStaffCommand(
	t *testing.T,
	bot *gotgbot.Bot,
	ctx *ext.Context,
	desc helpers.CommandDescriptor,
	h func(*helpers.CommandContext) error,
) error {
	t.Helper()

	c, err := helpers.BuildCommandContext(bot, ctx)
	if err != nil {
		return err
	}
	if !helpers.RunChecks(c, desc.RequiredChecks) {
		return ext.EndGroups
	}
	return h(c)
}

// staffCleanup deletes staff rows touching the given chat IDs when the test ends.
func staffCleanup(t *testing.T, chatIDs ...int64) {
	t.Helper()
	t.Cleanup(func() {
		if len(chatIDs) == 0 {
			return
		}
		db.DB.Where("chat_id IN ?", chatIDs).Delete(&models.StaffGroup{})
		db.DB.Where("group_chat_id IN ? OR staff_chat_id IN ?", chatIDs, chatIDs).Delete(&models.StaffGroupLink{})
	})
}
