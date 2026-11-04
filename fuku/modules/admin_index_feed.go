package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/adminindex"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// adminIndexWriteTimeout bounds one roster write made from the admin list load path. The
// write runs synchronously under that chat's admin-cache singleflight, so it can delay that
// one LoadAdminCache caller by at most this long (accepted, research R-05.1).
const adminIndexWriteTimeout = 3 * time.Second

// LoadAdminIndex installs the observer that keeps chat_admin_index current from every admin
// list the bot loads. It registers no handler and does no database access, so it is safe in
// the module load phase.
func LoadAdminIndex(_ *ext.Dispatcher) {
	cache.SetAdminListObserver(adminIndexObserve)
}

// adminIndexObserve turns a definitive admin list fact into a roster write: a loaded list
// replaces the chat's rows with its creator and human administrators, and a bot that is not
// an administrator, or cannot see the chat, clears them. A failed write is logged and never
// returned, so the LoadAdminCache caller gets the result it would have got without the
// observer. The roster is a hint for the private settings list and never authority.
func adminIndexObserve(ev cache.AdminListEvent) {
	defer error_handling.RecoverFromPanic("adminIndexObserve", "AdminIndex")

	ctx, cancel := context.WithTimeout(context.Background(), adminIndexWriteTimeout)
	defer cancel()

	var err error
	switch ev.Kind {
	case cache.AdminListLoaded:
		err = adminindex.ReplaceChat(ctx, ev.ChatID, adminIndexEntries(ev.Admins))
	case cache.AdminListBotNotAdmin, cache.AdminListBotGone:
		err = adminindex.ClearChat(ctx, ev.ChatID)
	default:
		return
	}
	if err != nil {
		log.Warnf("[AdminIndex] roster write for chat %d: %v", ev.ChatID, err)
	}
}

// adminIndexEntries keeps the creator and the human administrators of a listed chat. A bot is
// never stored, and neither is a creator that settingsCreatorGone recognises as a deleted
// account, so the private list treats that group as having no live owner (D-15).
func adminIndexEntries(members []gotgbot.MergedChatMember) []adminindex.Entry {
	entries := make([]adminindex.Entry, 0, len(members))
	for _, member := range members {
		if member.User.IsBot || member.User.Id == 0 {
			continue
		}
		switch member.Status {
		case gotgbot.ChatMemberStatusCreator:
			if settingsCreatorGone(member.User) {
				continue
			}
			entries = append(entries, adminindex.Entry{UserID: member.User.Id, Role: models.AdminIndexRoleCreator})
		case gotgbot.ChatMemberStatusAdministrator:
			entries = append(entries, adminindex.Entry{UserID: member.User.Id, Role: models.AdminIndexRoleAdministrator})
		}
	}
	return entries
}

func init() {
	RegisterLegacyModule("AdminIndex", 31, LoadAdminIndex)
}
