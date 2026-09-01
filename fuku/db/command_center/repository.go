package command_center

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/Fuku_Robot/fuku/db"
	"github.com/uasneppy/Fuku_Robot/fuku/db/cache"
	"github.com/uasneppy/Fuku_Robot/fuku/db/models"
	"github.com/uasneppy/Fuku_Robot/fuku/db/user"
)

const (
	maxCCNameLen        = 64
	maxCCDescriptionLen = 256
	cachePrefixCC       = "command_center"
)

// MaxConnectedChats caps how many chats a single command center may manage.
const MaxConnectedChats = 50

var (
	// ErrNotFound is returned when no command center matches the lookup.
	ErrNotFound = errors.New("command center not found")
	// ErrAlreadyOwnsCC is returned when a user already owns a command center.
	ErrAlreadyOwnsCC = errors.New("user already owns a command center")
	// ErrChatIsCommandCenter is returned when the chat is itself a command center.
	ErrChatIsCommandCenter = errors.New("chat is already a command center")
	// ErrAlreadyConnected is returned when a chat is already connected somewhere.
	ErrAlreadyConnected = errors.New("chat is already connected to a command center")
	// ErrNotConnected is returned when a chat is not connected to the command center.
	ErrNotConnected = errors.New("chat is not connected to this command center")
	// ErrLimitReached is returned when a command center is at MaxConnectedChats.
	ErrLimitReached = errors.New("connected chat limit reached")
)

// CreateCommandCenter registers chatID as a command center owned by ownerID.
// A user may own only one command center, and a chat may only be one.
func CreateCommandCenter(chatID, ownerID int64, name string) (*models.CommandCenter, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxCCNameLen {
		return nil, fmt.Errorf("command center name is required and must be <= %d characters", maxCCNameLen)
	}

	if err := user.EnsureUserInDb(ownerID, "", ""); err != nil {
		return nil, err
	}

	var existing models.CommandCenter
	err := db.DB.Where("owner_id = ?", ownerID).First(&existing).Error
	if err == nil {
		return nil, ErrAlreadyOwnsCC
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Errorf("[CommandCenter] Owner lookup failed: %v", err)
		return nil, fmt.Errorf("failed to check existing command center: %w", err)
	}

	err = db.DB.Where("chat_id = ?", chatID).First(&existing).Error
	if err == nil {
		return nil, ErrChatIsCommandCenter
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Errorf("[CommandCenter] Chat lookup failed: %v", err)
		return nil, fmt.Errorf("failed to check existing command center: %w", err)
	}

	cc := &models.CommandCenter{ChatID: chatID, OwnerID: ownerID, Name: name}
	if err := db.DB.Create(cc).Error; err != nil {
		log.Errorf("[CommandCenter] Create failed: %v", err)
		return nil, fmt.Errorf("failed to create command center: %w", err)
	}

	invalidate(cc)
	return cc, nil
}

// GetCommandCenterByID retrieves a command center by its primary key.
func GetCommandCenterByID(id uint) (*models.CommandCenter, error) {
	return firstCommandCenter("id = ?", id)
}

// GetCommandCenterByOwner retrieves the command center owned by ownerID.
func GetCommandCenterByOwner(ownerID int64) (*models.CommandCenter, error) {
	return firstCommandCenter("owner_id = ?", ownerID)
}

// GetCommandCenterByChatID retrieves the command center hosted in chatID.
func GetCommandCenterByChatID(chatID int64) (*models.CommandCenter, error) {
	return firstCommandCenter("chat_id = ?", chatID)
}

// firstCommandCenter runs a single-row lookup, mapping a miss to ErrNotFound.
func firstCommandCenter(query string, arg any) (*models.CommandCenter, error) {
	var cc models.CommandCenter
	if err := db.DB.Where(query, arg).First(&cc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		log.Errorf("[CommandCenter] Lookup (%s) failed: %v", query, err)
		return nil, fmt.Errorf("failed to get command center: %w", err)
	}
	return &cc, nil
}

// ListAllCommandCenters retrieves every command center, newest first.
func ListAllCommandCenters() ([]models.CommandCenter, error) {
	var ccs []models.CommandCenter
	if err := db.DB.Order("created_at DESC").Find(&ccs).Error; err != nil {
		log.Errorf("[CommandCenter] List all failed: %v", err)
		return nil, fmt.Errorf("failed to list command centers: %w", err)
	}
	return ccs, nil
}

// DeleteCommandCenter removes a command center and disconnects all of its chats.
func DeleteCommandCenter(id uint) error {
	cc, err := GetCommandCenterByID(id)
	if err != nil {
		return err
	}

	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("command_id = ?", id).Delete(&models.CommandCenterChat{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&models.CommandCenter{}).Error
	}); err != nil {
		log.Errorf("[CommandCenter] Delete failed: %v", err)
		return fmt.Errorf("failed to delete command center: %w", err)
	}

	invalidate(cc)
	return nil
}

// ConnectChat connects chatID to the command center identified by commandID.
func ConnectChat(commandID uint, chatID int64) error {
	if commandID == 0 || chatID == 0 {
		return fmt.Errorf("invalid command center ID or chat ID")
	}

	cc, err := GetCommandCenterByID(commandID)
	if err != nil {
		return err
	}
	if cc.ChatID == chatID {
		return ErrChatIsCommandCenter
	}

	var existing models.CommandCenterChat
	err = db.DB.Where("chat_id = ?", chatID).First(&existing).Error
	if err == nil {
		return ErrAlreadyConnected
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Errorf("[CommandCenter] Connection lookup failed: %v", err)
		return fmt.Errorf("failed to check chat connection: %w", err)
	}

	count, err := GetConnectedChatCount(commandID)
	if err != nil {
		return err
	}
	if count >= MaxConnectedChats {
		return ErrLimitReached
	}

	if err := db.DB.Create(&models.CommandCenterChat{ChatID: chatID, CommandID: commandID}).Error; err != nil {
		log.Errorf("[CommandCenter] Connect failed: %v", err)
		return fmt.Errorf("failed to connect chat: %w", err)
	}

	invalidate(cc)
	return nil
}

// DisconnectChat removes chatID from the command center identified by commandID.
func DisconnectChat(commandID uint, chatID int64) error {
	if commandID == 0 || chatID == 0 {
		return fmt.Errorf("invalid command center ID or chat ID")
	}

	result := db.DB.Where("command_id = ? AND chat_id = ?", commandID, chatID).
		Delete(&models.CommandCenterChat{})
	if result.Error != nil {
		log.Errorf("[CommandCenter] Disconnect failed: %v", result.Error)
		return fmt.Errorf("failed to disconnect chat: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotConnected
	}

	if cc, err := GetCommandCenterByID(commandID); err == nil {
		invalidate(cc)
	}
	return nil
}

// GetConnectedChats returns every chat connected to the command center.
func GetConnectedChats(commandID uint) ([]models.CommandCenterChat, error) {
	var connected []models.CommandCenterChat
	if err := db.DB.Where("command_id = ?", commandID).
		Order("chat_id ASC").Find(&connected).Error; err != nil {
		log.Errorf("[CommandCenter] Get connected chats failed: %v", err)
		return nil, fmt.Errorf("failed to get connected chats: %w", err)
	}
	return connected, nil
}

// GetConnectedChatCount returns how many chats are connected to the command center.
func GetConnectedChatCount(commandID uint) (int64, error) {
	var count int64
	if err := db.DB.Model(&models.CommandCenterChat{}).
		Where("command_id = ?", commandID).Count(&count).Error; err != nil {
		log.Errorf("[CommandCenter] Count failed: %v", err)
		return 0, fmt.Errorf("failed to count connected chats: %w", err)
	}
	return count, nil
}

// GetCommandCenterForChat returns the command center that chatID is connected to.
func GetCommandCenterForChat(chatID int64) (*models.CommandCenter, error) {
	var conn models.CommandCenterChat
	if err := db.DB.Where("chat_id = ?", chatID).First(&conn).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotConnected
		}
		log.Errorf("[CommandCenter] Connection lookup failed: %v", err)
		return nil, fmt.Errorf("failed to get chat connection: %w", err)
	}
	return GetCommandCenterByID(conn.CommandID)
}

// UpdateDescription sets the command center description, truncating over-long input.
func UpdateDescription(commandID uint, description string) error {
	description = strings.TrimSpace(description)
	if len(description) > maxCCDescriptionLen {
		description = description[:maxCCDescriptionLen]
	}

	result := db.DB.Model(&models.CommandCenter{}).Where("id = ?", commandID).
		Updates(map[string]any{"description": description})
	if result.Error != nil {
		log.Errorf("[CommandCenter] Update description failed: %v", result.Error)
		return fmt.Errorf("failed to update description: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	if cc, err := GetCommandCenterByID(commandID); err == nil {
		invalidate(cc)
	}
	return nil
}

// LogAction records a moderation action performed through the command center.
func LogAction(commandID uint, chatID, userID int64, actionType, reason string, messageID int64) error {
	if commandID == 0 || chatID == 0 || userID == 0 {
		return fmt.Errorf("invalid command center ID, chat ID, or user ID")
	}

	entry := &models.CommandCenterActionLog{
		CommandID:  commandID,
		ChatID:     chatID,
		UserID:     userID,
		ActionType: actionType,
		Reason:     reason,
		MessageID:  messageID,
	}
	if err := db.DB.Create(entry).Error; err != nil {
		log.Errorf("[CommandCenter] Log action failed: %v", err)
		return fmt.Errorf("failed to log action: %w", err)
	}
	return nil
}

// GetActionLogs returns the most recent action log entries for a command center.
func GetActionLogs(commandID uint, limit int) ([]models.CommandCenterActionLog, error) {
	var logs []models.CommandCenterActionLog
	if err := db.DB.Where("command_id = ?", commandID).
		Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		log.Errorf("[CommandCenter] Get action logs failed: %v", err)
		return nil, fmt.Errorf("failed to get action logs: %w", err)
	}
	return logs, nil
}

// invalidate clears every cache key that can resolve to this command center.
func invalidate(cc *models.CommandCenter) {
	if cc == nil {
		return
	}
	cache.DeleteCache(cache.CacheKey(cachePrefixCC, cc.ID))
	cache.DeleteCache(cache.CacheKey(cachePrefixCC, cc.ChatID))
	cache.DeleteCache(cache.CacheKey(cachePrefixCC, cc.OwnerID))
}
