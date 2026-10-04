package modules

import (
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

var staffModule = moduleStruct{
	moduleName: "Staff",
}

var (
	setStaffDesc = helpers.CommandDescriptor{
		Name:           "setstaff",
		RequiredChecks: []helpers.CheckFunc{helpers.RequireGroup()},
	}
	staffDesc = helpers.CommandDescriptor{
		Name: "staff",
	}
)

// replyStaff sends an HTML reply to the command message and logs a failure.
func replyStaff(c *helpers.CommandContext, text string) {
	if _, err := c.Msg.Reply(c.Bot, text, formatting.Shtml()); err != nil {
		log.Errorf("[Staff] reply: %v", err)
	}
}

// setStaff makes the current group a Staff Group. Authority comes from a live
// getChatAdministrators call (never the cached admin list): only the group's
// current creator may do it, and an API error refuses without writing anything.
func (moduleStruct) setStaff(c *helpers.CommandContext) error {
	if c.Chat.Type != "group" && c.Chat.Type != "supergroup" {
		return ext.EndGroups
	}

	result, _, err := chat_status.CheckOwner(c.Bot, c.Chat.Id, c.User.Id)
	switch result {
	case chat_status.OwnerUnknown:
		log.Warnf("[Staff] setStaff: owner check for chat %d failed: %v", c.Chat.Id, err)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return ext.EndGroups
	case chat_status.OwnerMismatch:
		text, _ := c.Tr.GetString("staff_refuse_not_owner")
		replyStaff(c, text)
		return ext.EndGroups
	}

	created, err := staff.CreateStaffGroup(c.Chat.Id, c.User.Id, c.Chat.Title)
	if err != nil {
		log.Errorf("[Staff] setStaff: %v", err)
		text, _ := c.Tr.GetString("staff_check_failed")
		replyStaff(c, text)
		return ext.EndGroups
	}
	if !created {
		text, _ := c.Tr.GetString("staff_set_already")
		replyStaff(c, text)
		return ext.EndGroups
	}
	text, _ := c.Tr.GetString("staff_set_done", i18n.TranslationParams{
		"chat_id": c.Chat.Id,
	})
	replyStaff(c, text)
	return ext.EndGroups
}

// staffPanel shows the Staff Group help text and chat ID. Outside a Staff Group
// it stays silent so the command does not reveal that the feature exists.
func (moduleStruct) staffPanel(c *helpers.CommandContext) error {
	group := staff.GetStaffGroup(c.Chat.Id)
	if group == nil {
		return ext.EndGroups
	}
	text, keyboard := renderStaffPanel(c.Tr, *group, nil)
	opts := formatting.Shtml()
	if len(keyboard.InlineKeyboard) > 0 {
		opts.ReplyMarkup = keyboard
	}
	if _, err := c.Msg.Reply(c.Bot, text, opts); err != nil {
		log.Errorf("[Staff] staffPanel: %v", err)
	}
	return ext.EndGroups
}

// LoadStaff registers the Staff Group commands.
func LoadStaff(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(staffModule.moduleName, true)

	helpers.WrapCommand(dispatcher, setStaffDesc, staffModule.setStaff)
	helpers.WrapCommand(dispatcher, staffDesc, staffModule.staffPanel)
}

func init() {
	RegisterLegacyModule("Staff", 236, LoadStaff)
}
