package greetings

import (
	"context"
	"errors"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// LoadGreetingSettings reads a chat's greetings row for the welcome path and returns the read's own error. On a
// successful read, and when the chat has no row, it behaves exactly as GetGreetingSettings (stored values, or the
// defaults, creating the row for a chat the bot knows). Any other read error (a timeout, a dropped connection) is
// returned together with the fallback defaults GetGreetingSettings would use; a caller that must not act on a guess,
// like the Greet once welcome, ignores the settings and fails closed on the error.
func LoadGreetingSettings(chatID int64) (*models.GreetingSettings, error) {
	var readErr error
	greetingSrc := &models.GreetingSettings{}
	err := db.GetRecord(greetingSrc, map[string]any{"chat_id": chatID})

	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !db.ChatExists(chatID) {
			log.Debugf("[Database][checkGreetingSettings]: Chat %d doesn't exist, returning default settings", chatID)
			return &models.GreetingSettings{
				ChatID:             chatID,
				ShouldCleanService: false,
				WelcomeSettings: &models.WelcomeSettings{
					LastMsgId:     0,
					CleanWelcome:  false,
					ShouldWelcome: true,
					WelcomeText:   db.DefaultWelcome,
					WelcomeType:   db.TEXT,
					Button:        models.ButtonArray{},
				},
				GoodbyeSettings: &models.GoodbyeSettings{
					LastMsgId:     0,
					CleanGoodbye:  false,
					ShouldGoodbye: false,
					GoodbyeText:   db.DefaultGoodbye,
					GoodbyeType:   db.TEXT,
					Button:        models.ButtonArray{},
				},
			}, nil
		}

		greetingSrc = &models.GreetingSettings{
			ChatID:             chatID,
			ShouldCleanService: false,
			WelcomeSettings: &models.WelcomeSettings{
				LastMsgId:     0,
				CleanWelcome:  false,
				ShouldWelcome: true,
				WelcomeText:   db.DefaultWelcome,
				WelcomeType:   db.TEXT,
				Button:        models.ButtonArray{},
			},
			GoodbyeSettings: &models.GoodbyeSettings{
				LastMsgId:     0,
				CleanGoodbye:  false,
				ShouldGoodbye: false,
				GoodbyeText:   db.DefaultGoodbye,
				GoodbyeType:   db.TEXT,
				Button:        models.ButtonArray{},
			},
		}

		err := db.CreateRecord(greetingSrc)
		if err != nil {
			log.Errorf("[Database][checkGreetingSettings]: %v ", err)
		}
	} else if err != nil {
		readErr = fukuerrors.Wrapf(err, "read greeting settings of chat %d", chatID)
		greetingSrc = &models.GreetingSettings{
			ChatID:             chatID,
			ShouldCleanService: false,
			WelcomeSettings: &models.WelcomeSettings{
				LastMsgId:     0,
				CleanWelcome:  false,
				ShouldWelcome: true,
				WelcomeText:   db.DefaultWelcome,
				WelcomeType:   db.TEXT,
				Button:        models.ButtonArray{},
			},
			GoodbyeSettings: &models.GoodbyeSettings{
				LastMsgId:     0,
				CleanGoodbye:  false,
				ShouldGoodbye: false,
				GoodbyeText:   db.DefaultGoodbye,
				GoodbyeType:   db.TEXT,
				Button:        models.ButtonArray{},
			},
		}
	}

	if greetingSrc.WelcomeSettings == nil {
		greetingSrc.WelcomeSettings = &models.WelcomeSettings{
			LastMsgId:     0,
			CleanWelcome:  false,
			ShouldWelcome: true,
			WelcomeText:   db.DefaultWelcome,
			WelcomeType:   db.TEXT,
			Button:        models.ButtonArray{},
		}
	} else if greetingSrc.WelcomeSettings.WelcomeText == "" {
		greetingSrc.WelcomeSettings.WelcomeText = db.DefaultWelcome
	}

	if greetingSrc.GoodbyeSettings == nil {
		greetingSrc.GoodbyeSettings = &models.GoodbyeSettings{
			LastMsgId:     0,
			CleanGoodbye:  false,
			ShouldGoodbye: false,
			GoodbyeText:   db.DefaultGoodbye,
			GoodbyeType:   db.TEXT,
			Button:        models.ButtonArray{},
		}
	} else if greetingSrc.GoodbyeSettings.GoodbyeText == "" {
		greetingSrc.GoodbyeSettings.GoodbyeText = db.DefaultGoodbye
	}

	return greetingSrc, readErr
}

// checkGreetingSettings is LoadGreetingSettings for every caller that keeps the fallback defaults on a read error:
// it logs the error once and returns them.
func checkGreetingSettings(chatID int64) *models.GreetingSettings {
	greetingSrc, err := LoadGreetingSettings(chatID)
	if err != nil {
		log.Errorf("[Database][checkGreetingSettings]: %v", err)
	}
	return greetingSrc
}

// LoadSettingsContext reads a chat's greetings row with the caller's context and
// creates nothing. With no row it returns, without writing, the settings
// GetGreetingSettings would end up with: Welcome on, Delete last off, the default
// texts and Goodbye on for a chat the chats table knows (the database default of
// goodbye_enabled is true, which the first write stores) or off for a chat it does
// not know. Any other read error is returned, never turned into defaults, so a
// settings page must use this instead of GetGreetingSettings, which creates rows
// and hides errors.
func LoadSettingsContext(ctx context.Context, chatID int64) (*models.GreetingSettings, error) {
	settings := &models.GreetingSettings{}
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var known int64
		if cerr := db.DB.WithContext(ctx).Model(&models.Chat{}).
			Where("chat_id = ?", chatID).Count(&known).Error; cerr != nil {
			return nil, fukuerrors.Wrapf(cerr, "check chat %d for greeting settings", chatID)
		}
		return defaultGreetingSettingsView(chatID, known > 0), nil
	}
	if err != nil {
		return nil, fukuerrors.Wrapf(err, "load greeting settings for chat %d", chatID)
	}

	if settings.WelcomeSettings == nil {
		settings.WelcomeSettings = defaultGreetingSettingsView(chatID, false).WelcomeSettings
	} else if settings.WelcomeSettings.WelcomeText == "" {
		settings.WelcomeSettings.WelcomeText = db.DefaultWelcome
	}
	if settings.GoodbyeSettings == nil {
		settings.GoodbyeSettings = defaultGreetingSettingsView(chatID, false).GoodbyeSettings
	} else if settings.GoodbyeSettings.GoodbyeText == "" {
		settings.GoodbyeSettings.GoodbyeText = db.DefaultGoodbye
	}
	return settings, nil
}

// defaultGreetingSettingsView is the in-memory settings of a chat with no greetings
// row. goodbyeOn is true for a chat in the chats table, whose first write stores the
// column default of goodbye_enabled.
func defaultGreetingSettingsView(chatID int64, goodbyeOn bool) *models.GreetingSettings {
	return &models.GreetingSettings{
		ChatID: chatID,
		WelcomeSettings: &models.WelcomeSettings{
			ShouldWelcome: true,
			WelcomeText:   db.DefaultWelcome,
			WelcomeType:   db.TEXT,
			Button:        models.ButtonArray{},
		},
		GoodbyeSettings: &models.GoodbyeSettings{
			ShouldGoodbye: goodbyeOn,
			GoodbyeText:   db.DefaultGoodbye,
			GoodbyeType:   db.TEXT,
			Button:        models.ButtonArray{},
		},
	}
}

func GetGreetingSettings(chatID int64) *models.GreetingSettings {
	return checkGreetingSettings(chatID)
}

func GetWelcomeButtons(chatId int64) []models.Button {
	greetingSettings := checkGreetingSettings(chatId)
	if greetingSettings.WelcomeSettings != nil {
		return []models.Button(greetingSettings.WelcomeSettings.Button)
	}
	return []models.Button{}
}

func GetGoodbyeButtons(chatId int64) []models.Button {
	greetingSettings := checkGreetingSettings(chatId)
	if greetingSettings.GoodbyeSettings != nil {
		return []models.Button(greetingSettings.GoodbyeSettings.Button)
	}
	return []models.Button{}
}

func defaultGreetingSettingsAttrs(chatID int64) map[string]any {
	return map[string]any{
		"chat_id":                chatID,
		"clean_service_settings": false,
		"welcome_enabled":        true,
		"welcome_text":           db.DefaultWelcome,
		"welcome_type":           db.TEXT,
		"welcome_btns":           models.ButtonArray{},
		"goodbye_enabled":        false,
		"goodbye_text":           db.DefaultGoodbye,
		"goodbye_type":           db.TEXT,
		"goodbye_btns":           models.ButtonArray{},
		"auto_approve":           false,
	}
}

func upsertGreetingSettings(chatID int64, updates map[string]any) error {
	if !db.ChatExists(chatID) {
		if err := chats.EnsureChatInDb(chatID, ""); err != nil {
			return fukuerrors.Wrapf(err, "ensure chat %d in db", chatID)
		}
	}
	updates["updated_at"] = time.Now()
	settings := models.GreetingSettings{}
	if err := db.DB.Where("chat_id = ?", chatID).
		Attrs(defaultGreetingSettingsAttrs(chatID)).
		FirstOrCreate(&settings).Error; err != nil {
		return fukuerrors.Wrapf(err, "first-or-create greeting settings for chat %d", chatID)
	}
	if err := db.DB.Model(&models.GreetingSettings{}).
		Where("chat_id = ?", chatID).
		Updates(updates).Error; err != nil {
		return fukuerrors.Wrapf(err, "update greeting settings for chat %d", chatID)
	}
	cache.DeleteCache(cache.CacheKey("greetings", chatID))
	return nil
}

//nolint:dupl // SetGoodbyeText has similar structure but different struct fields
func SetWelcomeText(chatID int64, welcometxt, fileId string, buttons []models.Button, welcType int) error {
	updates := map[string]any{
		"welcome_text":    welcometxt,
		"welcome_btns":    models.ButtonArray(buttons),
		"welcome_type":    welcType,
		"welcome_file_id": fileId,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetWelcomeText]: %v", err)
		return err
	}

	return nil
}

// The Set{Welcome,Goodbye}{Text,Media,Buttons}Only writers are the settings editor's
// one-part writes: each stores only its own columns through upsertGreetingSettings, so
// the other parts of the row stay as they were and the greetings cache key is
// invalidated like every other greetings write. SetWelcomeText and SetGoodbyeText stay
// the commands' four-column writes.

// SetWelcomeTextOnly stores only the welcome text.
func SetWelcomeTextOnly(chatID int64, text string) error {
	updates := map[string]any{
		"welcome_text": text,
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetWelcomeTextOnly]: %v", err)
		return err
	}

	return nil
}

// SetWelcomeMediaOnly stores only the welcome media: file ID and type.
func SetWelcomeMediaOnly(chatID int64, fileID string, dataType int) error {
	updates := map[string]any{
		"welcome_file_id": fileID,
		"welcome_type":    dataType,
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetWelcomeMediaOnly]: %v", err)
		return err
	}

	return nil
}

// SetWelcomeButtonsOnly stores only the welcome buttons.
func SetWelcomeButtonsOnly(chatID int64, buttons []models.Button) error {
	updates := map[string]any{
		"welcome_btns": models.ButtonArray(buttons),
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetWelcomeButtonsOnly]: %v", err)
		return err
	}

	return nil
}

// SetGoodbyeTextOnly stores only the goodbye text.
func SetGoodbyeTextOnly(chatID int64, text string) error {
	updates := map[string]any{
		"goodbye_text": text,
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetGoodbyeTextOnly]: %v", err)
		return err
	}

	return nil
}

// SetGoodbyeMediaOnly stores only the goodbye media: file ID and type.
func SetGoodbyeMediaOnly(chatID int64, fileID string, dataType int) error {
	updates := map[string]any{
		"goodbye_file_id": fileID,
		"goodbye_type":    dataType,
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetGoodbyeMediaOnly]: %v", err)
		return err
	}

	return nil
}

// SetGoodbyeButtonsOnly stores only the goodbye buttons.
func SetGoodbyeButtonsOnly(chatID int64, buttons []models.Button) error {
	updates := map[string]any{
		"goodbye_btns": models.ButtonArray(buttons),
	}

	if err := upsertGreetingSettings(chatID, updates); err != nil {
		log.Errorf("[Database][SetGoodbyeButtonsOnly]: %v", err)
		return err
	}

	return nil
}

func SetWelcomeToggle(chatID int64, pref bool) error {
	updates := map[string]any{
		"welcome_enabled": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetWelcomeToggle]: %v", err)
		return err
	}

	return nil
}

//nolint:dupl // SetGoodbyeText has similar structure to SetWelcomeText but different struct fields
func SetGoodbyeText(chatID int64, goodbyetext, fileId string, buttons []models.Button, goodbyeType int) error {
	updates := map[string]any{
		"goodbye_text":    goodbyetext,
		"goodbye_btns":    models.ButtonArray(buttons),
		"goodbye_type":    goodbyeType,
		"goodbye_file_id": fileId,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetGoodbyeText]: %v", err)
		return err
	}

	return nil
}

func SetGoodbyeToggle(chatID int64, pref bool) error {
	updates := map[string]any{
		"goodbye_enabled": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetGoodbyeToggle]: %v", err)
		return err
	}

	return nil
}

func SetShouldCleanService(chatID int64, pref bool) error {
	updates := map[string]any{
		"clean_service_settings": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetShouldCleanService]: %v", err)
		return err
	}

	return nil
}

func SetShouldAutoApprove(chatID int64, pref bool) error {
	updates := map[string]any{
		"auto_approve": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetShouldAutoApprove]: %v", err)
		return err
	}

	return nil
}

func SetCleanWelcomeSetting(chatID int64, pref bool) error {
	updates := map[string]any{
		"welcome_clean_old": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetCleanWelcomeSetting]: %v", err)
		return err
	}

	return nil
}

func SetCleanWelcomeMsgId(chatId, msgId int64) error {
	updates := map[string]any{
		"welcome_last_msg_id": msgId,
	}

	err := upsertGreetingSettings(chatId, updates)
	if err != nil {
		log.Errorf("[Database][SetCleanWelcomeMsgId]: %v", err)
		return err
	}

	return nil
}

func SetCleanGoodbyeSetting(chatID int64, pref bool) error {
	updates := map[string]any{
		"goodbye_clean_old": pref,
	}

	err := upsertGreetingSettings(chatID, updates)
	if err != nil {
		log.Errorf("[Database][SetCleanGoodbyeSetting]: %v", err)
		return err
	}

	return nil
}

func SetCleanGoodbyeMsgId(chatId, msgId int64) error {
	updates := map[string]any{
		"goodbye_last_msg_id": msgId,
	}

	err := upsertGreetingSettings(chatId, updates)
	if err != nil {
		log.Errorf("[Database][SetCleanGoodbyeMsgId]: %v", err)
		return err
	}

	return nil
}

func LoadGreetingsStats() (enabledWelcome, enabledGoodbye, cleanServiceEnabled, cleanWelcomeEnabled, cleanGoodbyeEnabled int64) {
	type greetingStats struct {
		EnabledWelcome      int64
		EnabledGoodbye      int64
		CleanServiceEnabled int64
		CleanWelcomeEnabled int64
		CleanGoodbyeEnabled int64
	}

	var stats greetingStats
	query := `
		SELECT
			COUNT(CASE WHEN welcome_enabled = true THEN 1 END) as enabled_welcome,
			COUNT(CASE WHEN goodbye_enabled = true THEN 1 END) as enabled_goodbye,
			COUNT(CASE WHEN clean_service_settings = true THEN 1 END) as clean_service_enabled,
			COUNT(CASE WHEN welcome_clean_old = true THEN 1 END) as clean_welcome_enabled,
			COUNT(CASE WHEN goodbye_clean_old = true THEN 1 END) as clean_goodbye_enabled
		FROM greetings
	`

	err := db.DB.Raw(query).Scan(&stats).Error
	if err != nil {
		log.Errorf("[Database][LoadGreetingsStats] querying stats: %v", err)
		return 0, 0, 0, 0, 0
	}

	return stats.EnabledWelcome, stats.EnabledGoodbye, stats.CleanServiceEnabled, stats.CleanWelcomeEnabled, stats.CleanGoodbyeEnabled
}

// SetWelcomeMediaBelow stores only whether the welcome's media is shown under its text.
// A false is written like a true: the column is set through a column-targeted upsert.
func SetWelcomeMediaBelow(chatID int64, on bool) error {
	if err := upsertGreetingSettings(chatID, map[string]any{"welcome_media_below": on}); err != nil {
		log.Errorf("[Database][SetWelcomeMediaBelow]: %v", err)
		return err
	}

	return nil
}

// SetGoodbyeMediaBelow stores only whether the goodbye's media is shown under its text.
func SetGoodbyeMediaBelow(chatID int64, on bool) error {
	if err := upsertGreetingSettings(chatID, map[string]any{"goodbye_media_below": on}); err != nil {
		log.Errorf("[Database][SetGoodbyeMediaBelow]: %v", err)
		return err
	}

	return nil
}

// SetWelcomeOnce stores only the Greet once switch: whether a member is welcomed the
// first time they join and never again.
func SetWelcomeOnce(chatID int64, on bool) error {
	if err := upsertGreetingSettings(chatID, map[string]any{"welcome_once": on}); err != nil {
		log.Errorf("[Database][SetWelcomeOnce]: %v", err)
		return err
	}

	return nil
}
