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
	cachePrefixCC       = "cc"
	cachePrefixCCChat   = "cc_chat"
)

var (
	ErrAlreadyOwnsCC = errors.New("user already owns a command center")
	ErrAlreadyJoined = errors.New("chat already joined to a command center")
)

// MaxSubscriptions is the maximum number of chats that can be connected to a command center.
const MaxSubscriptions = 50

// CreateCommandCenter creates a command center owned by userID. One command center per owner.
func CreateCommandCenter(ownerID int64, name string) (*models.CommandCenter, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxCCNameLen {
		return nil, fmt.Errorf("command center name is required and must be <= %d characters", maxCCNameLen)
	}

	if err := user.EnsureUserInDb(ownerID, "", ""); err != nil {
		return nil, err
	}

	var existing models.CommandCenter
	if err := db.DB.Where("owner_id = ?", ownerID).First(&existing).Error; err == nil {
		return nil, ErrAlreadyOwnsCC
	}

	cc := &models.CommandCenter{
		ChatID:      ownerID,
		OwnerID:     ownerID,
		Name:        name,
		Description: "",
	}

	if err := db.DB.Create(cc).Error; err != nil {
		log.Errorf("[CommandCenter] Create failed: %v", err)
		return nil, fmt.Errorf("failed to create command center: %w", err)
	}

	cache.DeleteCache(cache.CacheKey(cachePrefixCC, ownerID))
	return cc, nil
}

// GetCommandCenter retrieves a command center by owner ID.
func GetCommandCenter(ownerID int64) (*models.CommandCenter, error) {
	var cc models.CommandCenter
	if err := db.DB.Where("owner_id = ?", ownerID).First(&cc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAlreadyOwnsCC
		}
		log.Errorf("[CommandCenter] Get failed: %v", err)
		return nil, fmt.Errorf("failed to get command center: %w", err)
	}
	return &cc, nil
}

// DeleteCommandCenter deletes a command center and all its connections.
func DeleteCommandCenter(ownerID int64) error {
	// Delete all connected chats first
	if err := db.DB.Where("command_id = ?", ownerID).Delete(&models.CommandCenterChat{}).Error; err != nil {
		log.Errorf("[CommandCenter] Delete connected chats failed: %v", err)
	}

	// Delete the command center
	if err := db.DB.Where("owner_id = ?", ownerID).Delete(&models.CommandCenter{}).Error; err != nil {
		log.Errorf("[CommandCenter] Delete failed: %v", err)
		return fmt.Errorf("failed to delete command center: %w", err)
	}

	cache.DeleteCache(cache.CacheKey(cachePrefixCC, ownerID))
	return nil
}

// JoinCommandCenter joins a chat to a command center.
func JoinCommandCenter(commandID int64, chatID int64) error {
	if commandID <= 0 || chatID == 0 {
		return fmt.Errorf("invalid command ID or chat ID")
	}

	var cc models.CommandCenter
	if err := db.DB.Where("id = ?", commandID).First(&cc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrAlreadyOwnsCC
		}
		log.Errorf("[CommandCenter] Get command center failed: %v", err)
		return fmt.Errorf("failed to get command center: %w", err)
	}

	// Check if chat is already joined
	var existing models.CommandCenterChat
	if err := db.DB.Where("chat_id = ?", chatID).First(&existing).Error; err == nil {
		return ErrAlreadyJoined
	}

	// Check subscription limit
	var count int64
	if err := db.DB.Model(&models.CommandCenterChat{}).Where("command_id = ?", commandID).Count(&count).Error; err != nil {
		log.Errorf("[CommandCenter] Count failed: %v", err)
		return fmt.Errorf("failed to count subscriptions: %w", err)
	}

	if count >= MaxSubscriptions {
		return fmt.Errorf("subscription limit reached (max %d)", MaxSubscriptions)
	}

	conn := &models.CommandCenterChat{
		ChatID:    chatID,
		CommandID: uint(commandID),
	}

	if err := db.DB.Create(conn).Error; err != nil {
		log.Errorf("[CommandCenter] Join failed: %v", err)
		return fmt.Errorf("failed to join command center: %w", err)
	}

	cache.DeleteCache(cache.CacheKey(cachePrefixCC, commandID))
	return nil
}

// LeaveCommandCenter removes a chat from a command center.
func LeaveCommandCenter(commandID int64, chatID int64) error {
	if commandID == 0 || chatID == 0 {
		return fmt.Errorf("invalid command ID or chat ID")
	}

	result := db.DB.Where("command_id = ? AND chat_id = ?", commandID, chatID).Delete(&models.CommandCenterChat{})
	if result.Error != nil {
		log.Errorf("[CommandCenter] Leave failed: %v", result.Error)
		return fmt.Errorf("failed to leave command center: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrAlreadyJoined
	}

	cache.DeleteCache(cache.CacheKey(cachePrefixCC, commandID))
	return nil
}

// GetCommandCenterChats retrieves all chats connected to a command center.
func GetCommandCenterChats(commandID int64) ([]models.CommandCenterChat, error) {
	var chats []models.CommandCenterChat
	if err := db.DB.Where("command_id = ?", commandID).Order("chat_id ASC").Find(&chats).Error; err != nil {
		log.Errorf("[CommandCenter] Get chats failed: %v", err)
		return nil, fmt.Errorf("failed to get command center chats: %w", err)
	}
	return chats, nil
}

// GetCommandCenterChat retrieves a specific chat connection.
func GetCommandCenterChat(commandID int64, chatID int64) (*models.CommandCenterChat, error) {
	var conn models.CommandCenterChat
	if err := db.DB.Where("command_id = ? AND chat_id = ?", commandID, chatID).First(&conn).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAlreadyJoined
		}
		log.Errorf("[CommandCenter] Get chat failed: %v", err)
		return nil, fmt.Errorf("failed to get command center chat: %w", err)
	}
	return &conn, nil
}

// IsChatInCommandCenter checks if a chat is connected to a command center.
func IsChatInCommandCenter(commandID int64, chatID int64) bool {
	var count int64
	db.DB.Model(&models.CommandCenterChat{}).Where("command_id = ? AND chat_id = ?", commandID, chatID).Count(&count)
	return count > 0
}

// GetCommandCenterChatCount returns the number of chats connected to a command center.
func GetCommandCenterChatCount(commandID int64) (int, error) {
	var count int64
	if err := db.DB.Model(&models.CommandCenterChat{}).Where("command_id = ?", commandID).Count(&count).Error; err != nil {
		log.Errorf("[CommandCenter] Count failed: %v", err)
		return 0, fmt.Errorf("failed to count command center chats: %w", err)
	}
	return int(count), nil
}

// GetCommandCenterByChatID retrieves a command center by the chat ID of its owner.
func GetCommandCenterByChatID(chatID int64) (*models.CommandCenter, error) {
	var cc models.CommandCenter
	if err := db.DB.Where("chat_id = ?", chatID).First(&cc).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAlreadyOwnsCC
		}
		log.Errorf("[CommandCenter] Get by chat ID failed: %v", err)
		return nil, fmt.Errorf("failed to get command center by chat ID: %w", err)
	}
	return &cc, nil
}

// GetCommandCenterByID retrieves a command center by its primary key.
func GetCommandCenterByID(id uint) (*models.CommandCenter, error) {
	var cc models.CommandCenter
	if err := db.DB.Where("id = ?", id).First(&cc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		log.Errorf("[CommandCenter] Get by ID failed: %v", err)
		return nil, fmt.Errorf("failed to get command center by id: %w", err)
	}
	return &cc, nil
}

// ListAllCommandCenters retrieves all command centers.
func ListAllCommandCenters() ([]models.CommandCenter, error) {
	var ccs []models.CommandCenter
	if err := db.DB.Order("created_at DESC").Find(&ccs).Error; err != nil {
		log.Errorf("[CommandCenter] List all failed: %v", err)
		return nil, fmt.Errorf("failed to list command centers: %w", err)
	}
	return ccs, nil
}

// UpdateCommandCenterDescription updates the description of a command center.
func UpdateCommandCenterDescription(commandID int64, description string) error {
	description = strings.TrimSpace(description)
	if len(description) > maxCCDescriptionLen {
		description = description[:maxCCDescriptionLen]
	}

	result := db.DB.Model(&models.CommandCenter{}).Where("id = ?", commandID).Updates(map[string]interface{}{
		"description": description,
	})

	if result.Error != nil {
		log.Errorf("[CommandCenter] Update description failed: %v", result.Error)
		return fmt.Errorf("failed to update command center description: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrAlreadyJoined
	}

	cache.DeleteCache(cache.CacheKey(cachePrefixCC, commandID))
	return nil
}

// LogCommandCenterAction logs an action performed through the command center.
func LogCommandCenterAction(commandID uint, chatID int64, userID int64, actionType string, reason string, messageID int64) error {
	if commandID == 0 || chatID == 0 || userID == 0 {
		return fmt.Errorf("invalid command ID, chat ID, or user ID")
	}

	action := &models.CommandCenterActionLog{
		CommandID:  commandID,
		ChatID:     chatID,
		UserID:     userID,
		ActionType: actionType,
		Reason:     reason,
		MessageID:  messageID,
	}

	if err := db.DB.Create(action).Error; err != nil {
		log.Errorf("[CommandCenter] Log action failed: %v", err)
		return fmt.Errorf("failed to log command center action: %w", err)
	}

	return nil
}

// GetCommandCenterActionLogs retrieves action logs for a command center.
func GetCommandCenterActionLogs(commandID uint, limit int) ([]models.CommandCenterActionLog, error) {
	var logs []models.CommandCenterActionLog
	if err := db.DB.Where("command_id = ?", commandID).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		log.Errorf("[CommandCenter] Get action logs failed: %v", err)
		return nil, fmt.Errorf("failed to get command center action logs: %w", err)
	}
	return logs, nil
}

// InvalidateCache invalidates cache for a command center.
func InvalidateCache(commandID int64) {
	cache.DeleteCache(cache.CacheKey(cachePrefixCC, commandID))
}
