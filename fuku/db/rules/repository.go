package rules

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

func checkRulesSetting(chatID int64) (*models.RulesSettings, error) {
	rulesrc := &models.RulesSettings{}
	err := db.GetRecord(rulesrc, models.RulesSettings{ChatId: chatID})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := chats.EnsureChatInDb(chatID, ""); err != nil {
			log.Errorf("[Database] checkRulesSetting: Failed to ensure chat exists for %d: %v", chatID, err)
			return &models.RulesSettings{ChatId: chatID, Rules: ""}, err
		}

		rulesrc = &models.RulesSettings{ChatId: chatID, Rules: ""}
		err := db.CreateRecord(rulesrc)
		if err != nil {
			log.Errorf("[Database] checkRulesSetting: %v - %d", err, chatID)
			return rulesrc, err
		}
	} else if err != nil {
		rulesrc = &models.RulesSettings{ChatId: chatID, Rules: ""}
		log.Errorf("[Database] checkRulesSetting: %v - %d", err, chatID)
		return rulesrc, err
	}
	return rulesrc, nil
}

func GetChatRulesInfo(chatId int64) *models.RulesSettings {
	rulesrc, _ := checkRulesSetting(chatId)
	return rulesrc
}

// LoadRulesSettingsContext reads a chat's whole rules row (text, button label, private
// switch, media, buttons and Media under text) with the caller's context and creates
// nothing: no row is a zero row for the chat with no error, and neither a rules row nor
// a chats row is written. Any other read error is returned. A settings page must use
// this instead of GetChatRulesInfo, which creates rows and hides errors.
func LoadRulesSettingsContext(ctx context.Context, chatID int64) (*models.RulesSettings, error) {
	rulesrc := &models.RulesSettings{}
	err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).First(rulesrc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &models.RulesSettings{ChatId: chatID}, nil
	}
	if err != nil {
		return nil, fukuerrors.Wrapf(err, "load rules for chat %d", chatID)
	}
	return rulesrc, nil
}

// LoadRulesContext reads a chat's rules text with the caller's context and creates
// nothing, like LoadRulesSettingsContext, which it wraps.
func LoadRulesContext(ctx context.Context, chatID int64) (string, error) {
	rulesrc, err := LoadRulesSettingsContext(ctx, chatID)
	if err != nil {
		return "", err
	}
	return rulesrc.Rules, nil
}

func SetChatRules(chatId int64, rules string) error {
	if _, err := checkRulesSetting(chatId); err != nil {
		return err
	}
	err := db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatId}, map[string]any{"rules": rules})
	if err != nil {
		log.Errorf("[Database] SetChatRules: %v - %d", err, chatId)
	}
	return err
}

func SetChatRulesButton(chatId int64, rulesButton string) error {
	if _, err := checkRulesSetting(chatId); err != nil {
		return err
	}
	err := db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatId}, map[string]any{"rules_btn": rulesButton})
	if err != nil {
		log.Errorf("[Database] SetChatRulesButton: %v", err)
	}
	return err
}

func SetPrivateRules(chatId int64, pref bool) error {
	if _, err := checkRulesSetting(chatId); err != nil {
		return err
	}
	err := db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatId}, map[string]any{"private": pref})
	if err != nil {
		log.Errorf("[Database] SetPrivateRules: %v", err)
	}
	return err
}

func LoadRulesStats() (setRules, pvtRules int64) {
	err := db.DB.Model(&models.RulesSettings{}).Where("rules != ?", "").Count(&setRules).Error
	if err != nil {
		log.Errorf("[Database] LoadRulesStats (set rules): %v", err)
	}

	err = db.DB.Model(&models.RulesSettings{}).Where("private = ?", true).Count(&pvtRules).Error
	if err != nil {
		log.Errorf("[Database] LoadRulesStats (private rules): %v", err)
	}

	return
}

// updateRulesColumns writes only the given columns of a chat's rules row, creating the row
// first when the chat has none. A Go zero is written. Rules reads are uncached, so there
// is no cache key to delete.
func updateRulesColumns(name string, chatID int64, columns map[string]any) error {
	if _, err := checkRulesSetting(chatID); err != nil {
		return err
	}
	err := db.UpdateRecordWithZeroValues(&models.RulesSettings{}, models.RulesSettings{ChatId: chatID}, columns)
	if err != nil {
		log.Errorf("[Database] %s: %v - %d", name, err, chatID)
	}
	return err
}

// SetRulesMediaOnly stores only the rules media: file ID and type. An empty file ID with
// type 0 clears the media. Rules reads are uncached, so no cache key exists to delete.
func SetRulesMediaOnly(chatID int64, fileID string, mediaType int) error {
	return updateRulesColumns("SetRulesMediaOnly", chatID, map[string]any{"file_id": fileID, "media_type": mediaType})
}

// SetRulesButtonsOnly stores only the rules buttons; nil or empty stores an empty array.
// Rules reads are uncached, so no cache key exists to delete.
func SetRulesButtonsOnly(chatID int64, buttons []models.Button) error {
	return updateRulesColumns("SetRulesButtonsOnly", chatID, map[string]any{"btns": models.ButtonArray(buttons)})
}

// SetRulesMediaBelow stores only whether the rules media is shown under the text. A
// false is written. Rules reads are uncached, so no cache key exists to delete.
func SetRulesMediaBelow(chatID int64, on bool) error {
	return updateRulesColumns("SetRulesMediaBelow", chatID, map[string]any{"media_below": on})
}

// ClearRules is the /clearrules write (D-04): it empties the text, media and buttons
// together and leaves the button label, the private switch and Media under text as they
// were (UI-SPEC Assumption A3). Rules reads are uncached, so no cache key exists to
// delete.
func ClearRules(chatID int64) error {
	return updateRulesColumns("ClearRules", chatID, map[string]any{
		"rules":      "",
		"file_id":    "",
		"media_type": 0,
		"btns":       models.ButtonArray{},
	})
}
