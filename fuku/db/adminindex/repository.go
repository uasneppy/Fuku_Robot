// Package adminindex holds the bot's record of each group's creator and human
// administrators (table chat_admin_index). It is a hint for the private settings list
// and never authority: every pick from the list is checked against a live getChatMember.
//
// Every read is a fresh query and nothing is cached, so no cache.DeleteCache applies. A cache
// added later would need a skipLocal entry and a DeleteCache on every write. The table is
// never part of backup, export, import or reset.
package adminindex

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// Entry is one member of a chat's roster: a user and the role they hold, one of
// models.AdminIndexRoleCreator and models.AdminIndexRoleAdministrator.
type Entry struct {
	UserID int64
	Role   string
}

// ListedGroup is one group in a user's private settings list: the chat ID and the title the
// bot last stored for it (empty when the bot has no chats row).
type ListedGroup struct {
	ChatID int64
	Title  string
}

// ErrUnknownRole is returned by ReplaceChat for an entry whose role is neither creator nor
// administrator. Nothing is written.
var ErrUnknownRole = errors.New("admin index role is neither creator nor administrator")

// errNoDatabase is returned, wrapped, when the database is not initialized.
var errNoDatabase = errors.New("database not initialized")

// insertBatchSize bounds how many roster rows one INSERT carries, well inside the bind
// variable limits of both databases.
const insertBatchSize = 500

// collapseEntries validates every role and collapses duplicate users, the creator winning
// over an administrator whatever the order of the input. The result is ordered by user ID.
func collapseEntries(entries []Entry) ([]Entry, error) {
	byUser := make(map[int64]string, len(entries))
	for _, entry := range entries {
		if entry.Role != models.AdminIndexRoleCreator && entry.Role != models.AdminIndexRoleAdministrator {
			return nil, fmt.Errorf("%w: user %d has role %q", ErrUnknownRole, entry.UserID, entry.Role)
		}
		if previous, ok := byUser[entry.UserID]; ok && previous == models.AdminIndexRoleCreator {
			continue
		}
		byUser[entry.UserID] = entry.Role
	}
	collapsed := make([]Entry, 0, len(byUser))
	for userID, role := range byUser {
		collapsed = append(collapsed, Entry{UserID: userID, Role: role})
	}
	sort.Slice(collapsed, func(i, j int) bool { return collapsed[i].UserID < collapsed[j].UserID })
	return collapsed, nil
}

// ReplaceChat leaves exactly entries as the chat's roster, in one transaction: an upsert of
// every entry, then a prune of every other user of the chat. An empty roster removes every row
// of the chat. A user listed twice is stored once and the creator wins. An unknown role
// returns ErrUnknownRole before any write. Rows of other chats are never touched.
//
// It never deletes everything and inserts again: under read committed, two such replaces of
// one chat would race into a unique violation. Instead, on PostgreSQL the first statement of
// the transaction is a per-chat transaction advisory lock, because two replaces with
// disjoint rosters would otherwise each prune only the rows they can see and commit the
// union of both. On SQLite the database write lock serializes the transactions, and no read
// precedes the first write, so a waiting writer is never asked to upgrade a read snapshot.
func ReplaceChat(ctx context.Context, chatID int64, entries []Entry) error {
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, "replace admin index")
	}
	collapsed, err := collapseEntries(entries)
	if err != nil {
		return fukuerrors.Wrapf(err, "replace admin index of chat %d", chatID)
	}
	err = db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			lockKey := fmt.Sprintf("fuku:chat_admin_index:%d", chatID)
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
				return err
			}
		}
		if len(collapsed) == 0 {
			return tx.Where("chat_id = ?", chatID).Delete(&models.ChatAdminIndex{}).Error
		}
		rows := make([]models.ChatAdminIndex, len(collapsed))
		keep := make([]int64, len(collapsed))
		for i, entry := range collapsed {
			rows[i] = models.ChatAdminIndex{ChatID: chatID, UserID: entry.UserID, Role: entry.Role}
			keep[i] = entry.UserID
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "chat_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"role", "updated_at"}),
		}).CreateInBatches(&rows, insertBatchSize).Error; err != nil {
			return err
		}
		return tx.Where("chat_id = ? AND user_id NOT IN ?", chatID, keep).Delete(&models.ChatAdminIndex{}).Error
	})
	if err != nil {
		return fukuerrors.Wrapf(err, "replace admin index of chat %d", chatID)
	}
	return nil
}

// ClearChat removes every roster row of the chat. It goes through the same locked
// transaction as an empty roster, so a clear and a replace of one chat never interleave.
func ClearChat(ctx context.Context, chatID int64) error {
	return ReplaceChat(ctx, chatID, nil)
}

// ListChatFresh returns the chat's roster rows ordered by user ID.
func ListChatFresh(ctx context.Context, chatID int64) ([]models.ChatAdminIndex, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "list admin index")
	}
	var rows []models.ChatAdminIndex
	if err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Order("user_id ASC").Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "list admin index of chat %d", chatID)
	}
	return rows, nil
}

// ListForUserFresh lists the groups a user may see in the private settings list. A chat where
// the user has a creator row is always listed. A chat where the user has an administrator row
// is listed when that chat's owner switch is on or the chat has no creator row at all (D-15).
// It never filters on chats.is_inactive, because the activity monitor marks quiet groups
// inactive. Each title is chats.chat_name, empty when the chats row is missing. The groups are
// ordered by chat ID.
//
// Three portable queries and no JOIN keep the rule identical on SQLite and PostgreSQL.
func ListForUserFresh(ctx context.Context, userID int64) ([]ListedGroup, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "list groups of a user")
	}
	conn := db.DB.WithContext(ctx)

	var rows []models.ChatAdminIndex
	if err := conn.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "list admin index rows of user %d", userID)
	}

	kept := make(map[int64]struct{}, len(rows))
	var adminChats []int64
	for _, row := range rows {
		if row.Role == models.AdminIndexRoleCreator {
			kept[row.ChatID] = struct{}{}
			continue
		}
		adminChats = append(adminChats, row.ChatID)
	}

	if len(adminChats) > 0 {
		var switchedOn []int64
		if err := conn.Model(&models.ChatSettingsAccess{}).
			Where("chat_id IN ? AND admins_allowed = ?", adminChats, true).
			Pluck("chat_id", &switchedOn).Error; err != nil {
			return nil, fukuerrors.Wrapf(err, "list owner switches for user %d", userID)
		}
		var withCreator []int64
		if err := conn.Model(&models.ChatAdminIndex{}).
			Where("chat_id IN ? AND role = ?", adminChats, models.AdminIndexRoleCreator).
			Distinct().Pluck("chat_id", &withCreator).Error; err != nil {
			return nil, fukuerrors.Wrapf(err, "list creator rows for user %d", userID)
		}
		open := make(map[int64]struct{}, len(switchedOn))
		for _, chatID := range switchedOn {
			open[chatID] = struct{}{}
		}
		hasCreator := make(map[int64]struct{}, len(withCreator))
		for _, chatID := range withCreator {
			hasCreator[chatID] = struct{}{}
		}
		for _, chatID := range adminChats {
			_, isOpen := open[chatID]
			_, owned := hasCreator[chatID]
			if isOpen || !owned {
				kept[chatID] = struct{}{}
			}
		}
	}

	if len(kept) == 0 {
		return []ListedGroup{}, nil
	}
	ids := make([]int64, 0, len(kept))
	for chatID := range kept {
		ids = append(ids, chatID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var chats []models.Chat
	if err := conn.Select("chat_id", "chat_name").Where("chat_id IN ?", ids).Find(&chats).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "read chat titles for user %d", userID)
	}
	titles := make(map[int64]string, len(chats))
	for _, chat := range chats {
		titles[chat.ChatId] = chat.ChatName
	}
	groups := make([]ListedGroup, len(ids))
	for i, chatID := range ids {
		groups[i] = ListedGroup{ChatID: chatID, Title: titles[chatID]}
	}
	return groups, nil
}

// ListCandidateChatIDsFresh returns every chats.chat_id below zero in ascending order,
// inactive chats included: the sweep's candidates (a negative ID is a group, a channel lives
// in the channels table).
func ListCandidateChatIDsFresh(ctx context.Context) ([]int64, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "list candidate chats")
	}
	ids := []int64{}
	if err := db.DB.WithContext(ctx).Model(&models.Chat{}).
		Where("chat_id < ?", 0).Order("chat_id ASC").Pluck("chat_id", &ids).Error; err != nil {
		return nil, fukuerrors.Wrap(err, "list candidate chats")
	}
	return ids, nil
}
