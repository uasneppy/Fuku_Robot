package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/uasneppy/fuku_robot/fuku/db"
	dbcache "github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ExportModuleData(chatID int64, module string) (interface{}, error) {
	switch module {
	case BackupModuleAdmin:
		return exportAdminData(chatID)
	case BackupModuleAntiflood:
		return exportAntifloodData(chatID)
	case BackupModuleAntiraid:
		return exportAntiraidData(chatID)
	case BackupModuleApprovals:
		return exportApprovalsData(chatID)
	case BackupModuleBlacklists:
		return exportBlacklistsData(chatID)
	case BackupModuleCaptcha:
		return exportCaptchaData(chatID)
	case BackupModuleConnections:
		return exportConnectionsData(chatID)
	case BackupModuleDisabling:
		return exportDisablingData(chatID)
	case BackupModuleFilters:
		return exportFiltersData(chatID)
	case BackupModuleGreetings:
		return exportGreetingsData(chatID)
	case BackupModuleLocks:
		return exportLocksData(chatID)
	case BackupModuleNotes:
		return exportNotesData(chatID)
	case BackupModulePins:
		return exportPinsData(chatID)
	case BackupModuleReactions:
		return exportReactionsData(chatID)
	case BackupModuleReports:
		return exportReportsData(chatID)
	case BackupModuleRules:
		return exportRulesData(chatID)
	case BackupModuleWarns:
		return exportWarnsData(chatID)
	case BackupModuleFederations:
		return exportFederationsData(chatID)
	case BackupModuleLogChannels:
		return exportLogChannelsData(chatID)
	default:
		return nil, fmt.Errorf("unknown module: %s", module)
	}
}

func withBackupTx(fn func(tx *gorm.DB) ([]string, error)) error {
	database, err := backupDB()
	if err != nil {
		return err
	}
	var keys []string
	if err := database.Transaction(func(tx *gorm.DB) error {
		var err error
		keys, err = fn(tx)
		return err
	}); err != nil {
		return err
	}
	invalidate(keys...)
	return nil
}

func ImportModuleData(chatID int64, module string, data interface{}) error {
	return withBackupTx(func(tx *gorm.DB) ([]string, error) {
		return importModuleData(tx, chatID, module, data, false, &ImportReport{})
	})
}

// ImportReport is what an import left out on purpose: Skipped counts the filters, blacklist words and notes
// that would not pass the checks the commands make (a keyword or word too broad or too long, a note name that
// could never be fetched, a button link that is not http or https, a repeat after lower-casing), and
// LogChannelSkipped says the log channel categories were not imported because the target has no log channel.
type ImportReport struct {
	Skipped           int
	LogChannelSkipped bool
}

func ClearModuleData(chatID int64, module string) error {
	return withBackupTx(func(tx *gorm.DB) ([]string, error) {
		return clearModuleData(tx, chatID, module)
	})
}

func ExportChatData(chatID int64, chatName string, exportedBy int64, modules []string) (*BackupFormat, error) {
	modules, err := checkedModules(modules)
	if err != nil {
		return nil, err
	}

	backup := NewBackupFormat(chatID, chatName, exportedBy, modules)
	for _, module := range modules {
		data, err := ExportModuleData(chatID, module)
		if err != nil {
			return nil, fmt.Errorf("failed to export module %s: %w", module, err)
		}
		backup.Data[module] = data
	}
	return backup, nil
}

func ImportChatData(chatID int64, backup *BackupFormat, modules []string) error {
	_, err := ImportChatDataReport(chatID, backup, modules)
	return err
}

// ImportChatDataReport imports like ImportChatData and also returns what the import left out on purpose.
func ImportChatDataReport(chatID int64, backup *BackupFormat, modules []string) (ImportReport, error) {
	var report ImportReport
	if err := importChatData(chatID, backup, modules, &report); err != nil {
		return ImportReport{}, err
	}
	return report, nil
}

func importChatData(chatID int64, backup *BackupFormat, modules []string, report *ImportReport) error {
	if backup == nil {
		return fmt.Errorf("invalid backup: backup cannot be nil")
	}
	if err := backup.Validate(); err != nil {
		return fmt.Errorf("invalid backup: %w", err)
	}
	if !backup.IsCompatibleVersion() {
		return fmt.Errorf("unsupported backup version %q", backup.Version)
	}
	if len(modules) == 0 {
		modules = backup.Modules
	}
	for _, module := range modules {
		if !IsValidModule(module) {
			return fmt.Errorf("unknown module: %s", module)
		}
		if _, ok := backup.Data[module]; !ok {
			return fmt.Errorf("missing data for module: %s", module)
		}
	}

	return withBackupTx(func(tx *gorm.DB) ([]string, error) {
		var keys []string
		for _, module := range modules {
			data := backup.Data[module]
			moduleKeys, err := importModuleData(tx, chatID, module, data, backup.Version == legacyFormatVersion, report)
			if err != nil {
				return nil, fmt.Errorf("failed to import module %s: %w", module, err)
			}
			keys = append(keys, moduleKeys...)
		}
		return keys, nil
	})
}

func ClearChatData(chatID int64, modules []string) error {
	modules, err := checkedModules(modules)
	if err != nil {
		return err
	}
	return withBackupTx(func(tx *gorm.DB) ([]string, error) {
		var keys []string
		for _, module := range modules {
			moduleKeys, err := clearModuleData(tx, chatID, module)
			if err != nil {
				return nil, fmt.Errorf("failed to clear module %s: %w", module, err)
			}
			keys = append(keys, moduleKeys...)
		}
		return keys, nil
	})
}

func checkedModules(modules []string) ([]string, error) {
	if len(modules) == 0 {
		return AllExportableModules(), nil
	}
	for _, module := range modules {
		if !IsValidModule(module) {
			return nil, fmt.Errorf("unknown module: %s", module)
		}
	}
	return modules, nil
}

func backupDB() (*gorm.DB, error) {
	if db.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return db.DB, nil
}

func findChatSetting[T any](chatID int64) (*T, error) {
	database, err := backupDB()
	if err != nil {
		return nil, err
	}
	var setting T
	err = database.Where("chat_id = ?", chatID).Take(&setting).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &setting, nil
}

func findChatRows[T any](chatID int64) ([]T, error) {
	database, err := backupDB()
	if err != nil {
		return nil, err
	}
	var rows []T
	if err := database.Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func replaceChatSetting[T any](tx *gorm.DB, chatID int64, setting *T) error {
	if err := tx.Where("chat_id = ?", chatID).Delete(new(T)).Error; err != nil {
		return err
	}
	if setting == nil {
		return nil
	}
	var desired T
	raw, err := json.Marshal(setting)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &desired); err != nil {
		return err
	}
	stmt := &gorm.Statement{DB: tx}
	if err := stmt.Parse(&desired); err != nil {
		return err
	}
	m := make(map[string]interface{}, len(stmt.Schema.Fields))
	rv := reflect.ValueOf(&desired).Elem()
	for _, field := range stmt.Schema.Fields {
		if field.Name == "ID" || field.DBName == "created_at" {
			continue
		}
		val, _ := field.ValueOf(context.Background(), rv)
		m[field.DBName] = val
	}
	return tx.Model(new(T)).Create(m).Error
}

func replaceChatRows[T any](tx *gorm.DB, chatID int64, rows []T) error {
	if err := tx.Where("chat_id = ?", chatID).Delete(new(T)).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	var desired []T
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &desired); err != nil {
		return err
	}
	maps := make([]map[string]interface{}, len(desired))
	for i := range desired {
		stmt := &gorm.Statement{DB: tx}
		if err := stmt.Parse(&desired[i]); err != nil {
			return err
		}
		m := make(map[string]interface{}, len(stmt.Schema.Fields))
		rv := reflect.ValueOf(&desired[i]).Elem()
		for _, field := range stmt.Schema.Fields {
			if field.Name == "ID" || field.DBName == "created_at" {
				continue
			}
			val, _ := field.ValueOf(context.Background(), rv)
			m[field.DBName] = val
		}
		maps[i] = m
	}
	return tx.Model(new(T)).Create(&maps).Error
}

func decodeModuleData(data interface{}, module string, target interface{}) error {
	if _, ok := data.(map[string]interface{}); !ok {
		return fmt.Errorf("invalid %s data format", module)
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("invalid %s data format: %w", module, err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("failed to parse %s data: %w", module, err)
	}
	return nil
}

func invalidate(keys ...string) {
	for _, key := range keys {
		dbcache.DeleteCache(key)
	}
}

func cacheKey(module string, chatID int64) string {
	return dbcache.CacheKey(module, chatID)
}

func exportAdminData(chatID int64) (*AdminBackup, error) {
	adminSettings, err := findChatSetting[models.AdminSettings](chatID)
	if err != nil {
		return nil, fmt.Errorf("get admin settings: %w", err)
	}
	antifloodSettings, err := findChatSetting[models.AntifloodSettings](chatID)
	if err != nil {
		return nil, fmt.Errorf("get antiflood settings: %w", err)
	}
	captchaSettings, err := findChatSetting[models.CaptchaSettings](chatID)
	if err != nil {
		return nil, fmt.Errorf("get captcha settings: %w", err)
	}
	connectionSettings, err := findChatSetting[models.ConnectionChatSettings](chatID)
	if err != nil {
		return nil, fmt.Errorf("get connection settings: %w", err)
	}
	blacklistEntries, err := findChatRows[models.BlacklistSettings](chatID)
	if err != nil {
		return nil, fmt.Errorf("get blacklist settings: %w", err)
	}

	result := &AdminBackup{
		AdminSettings:      adminSettings,
		AntifloodSettings:  antifloodSettings,
		CaptchaSettings:    captchaSettings,
		ConnectionSettings: connectionSettings,
	}
	if len(blacklistEntries) > 0 {
		result.BlacklistMode = blacklistEntries[0].Action
	}
	return result, nil
}

func exportAntifloodData(chatID int64) (*AntifloodBackup, error) {
	settings, err := findChatSetting[models.AntifloodSettings](chatID)
	return &AntifloodBackup{Settings: settings}, err
}

func exportAntiraidData(chatID int64) (*AntiraidBackup, error) {
	settings, err := findChatSetting[models.AntiRaidSettings](chatID)
	return &AntiraidBackup{Settings: settings}, err
}

func exportApprovalsData(chatID int64) (*ApprovalsBackup, error) {
	users, err := findChatRows[models.ApprovedUsers](chatID)
	return &ApprovalsBackup{ApprovedUsers: users}, err
}

func exportBlacklistsData(chatID int64) (*BlacklistsBackup, error) {
	entries, err := findChatRows[models.BlacklistSettings](chatID)
	if err != nil {
		return nil, err
	}
	result := &BlacklistsBackup{Entries: entries}
	if len(entries) > 0 {
		result.BlacklistMode = entries[0].Action
	}
	return result, nil
}

func exportCaptchaData(chatID int64) (*CaptchaBackup, error) {
	settings, err := findChatSetting[models.CaptchaSettings](chatID)
	return &CaptchaBackup{Settings: settings}, err
}

func exportConnectionsData(chatID int64) (*ConnectionsBackup, error) {
	settings, err := findChatSetting[models.ConnectionChatSettings](chatID)
	return &ConnectionsBackup{Settings: settings}, err
}

func exportDisablingData(chatID int64) (*DisablingBackup, error) {
	settings, err := findChatSetting[models.DisableChatSettings](chatID)
	if err != nil {
		return nil, err
	}
	commands, err := findChatRows[models.DisableSettings](chatID)
	if err != nil {
		return nil, err
	}
	return &DisablingBackup{ChatSettings: settings, Commands: commands}, nil
}

func exportFiltersData(chatID int64) (*FiltersBackup, error) {
	rows, err := findChatRows[models.ChatFilters](chatID)
	return &FiltersBackup{Filters: rows}, err
}

func exportGreetingsData(chatID int64) (*GreetingsBackup, error) {
	settings, err := findChatSetting[models.GreetingSettings](chatID)
	dropGreetingMessageIDs(settings)
	return &GreetingsBackup{Settings: settings}, err
}

// dropGreetingMessageIDs clears the IDs of the last welcome and goodbye. They name messages of the group
// the settings come from, so an export leaves them out and an import never stores them: "Delete last" in
// another group would otherwise delete a message of its own that happens to carry that ID.
func dropGreetingMessageIDs(settings *models.GreetingSettings) {
	if settings == nil {
		return
	}
	if settings.WelcomeSettings != nil {
		settings.WelcomeSettings.LastMsgId = 0
	}
	if settings.GoodbyeSettings != nil {
		settings.GoodbyeSettings.LastMsgId = 0
	}
}

func exportLocksData(chatID int64) (*LocksBackup, error) {
	rows, err := findChatRows[models.LockSettings](chatID)
	return &LocksBackup{Locks: rows}, err
}

func exportNotesData(chatID int64) (*NotesBackup, error) {
	settings, err := findChatSetting[models.NotesSettings](chatID)
	if err != nil {
		return nil, err
	}
	rows, err := findChatRows[models.Notes](chatID)
	if err != nil {
		return nil, err
	}
	return &NotesBackup{Settings: settings, Notes: rows}, nil
}

func exportPinsData(chatID int64) (*PinsBackup, error) {
	settings, err := findChatSetting[models.PinSettings](chatID)
	return &PinsBackup{Settings: settings}, err
}

func exportReactionsData(chatID int64) (*ReactionsBackup, error) {
	rows, err := findChatRows[models.Reactions](chatID)
	return &ReactionsBackup{Reactions: rows}, err
}

func exportReportsData(chatID int64) (*ReportsBackup, error) {
	settings, err := findChatSetting[models.ReportChatSettings](chatID)
	return &ReportsBackup{Settings: settings}, err
}

func exportRulesData(chatID int64) (*RulesBackup, error) {
	settings, err := findChatSetting[models.RulesSettings](chatID)
	return &RulesBackup{Settings: settings}, err
}

func exportWarnsData(chatID int64) (*WarnsBackup, error) {
	settings, err := findChatSetting[models.WarnSettings](chatID)
	if err != nil {
		return nil, err
	}
	rows, err := findChatRows[models.Warns](chatID)
	if err != nil {
		return nil, err
	}
	return &WarnsBackup{WarnSettings: settings, Warns: rows}, nil
}

func exportFederationsData(chatID int64) (*FederationsBackup, error) {
	row, err := findChatSetting[models.FederationChat](chatID)
	if err != nil {
		return nil, err
	}
	return &FederationsBackup{Membership: row}, nil
}

func exportLogChannelsData(chatID int64) (*LogChannelsBackup, error) {
	settings, err := findChatSetting[models.LogChannel](chatID)
	if err != nil {
		return nil, err
	}
	return &LogChannelsBackup{Settings: settings}, nil
}

func importModuleData(tx *gorm.DB, chatID int64, module string, data interface{}, preserveLegacyOmissions bool, report *ImportReport) ([]string, error) {
	if err := ensureBackupChat(tx, chatID); err != nil {
		return nil, err
	}
	if keys, ok, err := importListModule(tx, chatID, module, data, preserveLegacyOmissions, report); ok {
		return keys, err
	}
	switch module {
	case BackupModuleAdmin:
		return importAdmin(tx, chatID, data)
	case BackupModuleAntiflood:
		return importAntiflood(tx, chatID, data)
	case BackupModuleAntiraid:
		return importAntiraid(tx, chatID, data)
	case BackupModuleApprovals:
		return importApprovals(tx, chatID, data)
	case BackupModuleCaptcha:
		return importCaptcha(tx, chatID, data)
	case BackupModuleConnections:
		return importConnections(tx, chatID, data)
	case BackupModuleDisabling:
		return importDisabling(tx, chatID, data)
	case BackupModuleGreetings:
		return importGreetings(tx, chatID, data)
	case BackupModuleLocks:
		return importLocks(tx, chatID, data)
	case BackupModulePins:
		return importPins(tx, chatID, data)
	case BackupModuleReactions:
		return importReactions(tx, chatID, data)
	case BackupModuleReports:
		return importReports(tx, chatID, data)
	case BackupModuleRules:
		return importRules(tx, chatID, data)
	case BackupModuleFederations:
		return importFederations(tx, chatID, data)
	default:
		return nil, fmt.Errorf("unknown module: %s", module)
	}
}

// importListModule imports the modules whose import takes the report or the legacy flag
// (the list modules, warns and the log channel). ok is false for every other module.
func importListModule(tx *gorm.DB, chatID int64, module string, data interface{}, preserveLegacyOmissions bool, report *ImportReport) (keys []string, ok bool, err error) {
	switch module {
	case BackupModuleBlacklists:
		keys, err = importBlacklists(tx, chatID, data, report)
	case BackupModuleFilters:
		keys, err = importFilters(tx, chatID, data, report)
	case BackupModuleNotes:
		keys, err = importNotes(tx, chatID, data, preserveLegacyOmissions, report)
	case BackupModuleWarns:
		keys, err = importWarns(tx, chatID, data, preserveLegacyOmissions)
	case BackupModuleLogChannels:
		keys, err = importLogChannels(tx, chatID, data, report)
	default:
		return nil, false, nil
	}
	return keys, true, err
}

func importAdmin(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data AdminBackup
	if err := decodeModuleData(payload, BackupModuleAdmin, &data); err != nil {
		return nil, err
	}
	if data.AdminSettings != nil {
		data.AdminSettings.ChatId = chatID
	}
	if data.AntifloodSettings != nil {
		data.AntifloodSettings.ChatId = chatID
	}
	if data.CaptchaSettings != nil {
		data.CaptchaSettings.ChatID = chatID
	}
	if data.ConnectionSettings != nil {
		data.ConnectionSettings.ChatId = chatID
	}

	if err := replaceChatSetting(tx, chatID, data.AdminSettings); err != nil {
		return nil, fmt.Errorf("restore admin settings: %w", err)
	}
	if err := replaceChatSetting(tx, chatID, data.AntifloodSettings); err != nil {
		return nil, fmt.Errorf("restore antiflood settings: %w", err)
	}
	if err := replaceChatSetting(tx, chatID, data.CaptchaSettings); err != nil {
		return nil, fmt.Errorf("restore captcha settings: %w", err)
	}
	if err := replaceChatSetting(tx, chatID, data.ConnectionSettings); err != nil {
		return nil, fmt.Errorf("restore connection settings: %w", err)
	}
	if data.BlacklistMode != "" {
		if err := tx.Model(&models.BlacklistSettings{}).
			Where("chat_id = ?", chatID).
			Update("action", data.BlacklistMode).Error; err != nil {
			return nil, fmt.Errorf("restore blacklist mode: %w", err)
		}
	}
	return []string{
		cacheKey("antiflood", chatID),
		cacheKey("captcha_settings", chatID),
		cacheKey("blacklist", chatID),
	}, nil
}

func importAntiflood(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data AntifloodBackup
	if err := decodeModuleData(payload, BackupModuleAntiflood, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
		if data.Settings.Limit < 0 {
			return nil, fmt.Errorf("invalid antiflood limit %d", data.Settings.Limit)
		}
	}
	if err := replaceChatSetting(tx, chatID, data.Settings); err != nil {
		return nil, err
	}
	return []string{cacheKey("antiflood", chatID)}, nil
}

func importAntiraid(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data AntiraidBackup
	if err := decodeModuleData(payload, BackupModuleAntiraid, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatID = chatID
		if data.Settings.RaidTime < 0 || data.Settings.RaidActionTime < 0 || data.Settings.AutoAntiRaidThreshold < 0 {
			return nil, fmt.Errorf("invalid antiraid settings")
		}
	}
	if err := replaceChatSetting(tx, chatID, data.Settings); err != nil {
		return nil, err
	}
	return []string{cacheKey("antiraid", chatID)}, nil
}

func importApprovals(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data ApprovalsBackup
	if err := decodeModuleData(payload, BackupModuleApprovals, &data); err != nil {
		return nil, err
	}
	for i := range data.ApprovedUsers {
		if data.ApprovedUsers[i].UserID == 0 {
			return nil, fmt.Errorf("invalid approved user ID")
		}
		data.ApprovedUsers[i].ChatID = chatID
	}
	if err := replaceChatRows(tx, chatID, data.ApprovedUsers); err != nil {
		return nil, err
	}
	return []string{cacheKey("approvals", chatID)}, nil
}

func importBlacklists(tx *gorm.DB, chatID int64, payload interface{}, report *ImportReport) ([]string, error) {
	var data BlacklistsBackup
	if err := decodeModuleData(payload, BackupModuleBlacklists, &data); err != nil {
		return nil, err
	}
	if len(data.Entries) == 0 && data.Settings != nil {
		data.Entries = []models.BlacklistSettings{*data.Settings}
	}
	for i := range data.Entries {
		if data.Entries[i].Word == "" {
			return nil, fmt.Errorf("invalid empty blacklist word")
		}
		data.Entries[i].ChatId = chatID
		if data.Entries[i].Action == "" {
			data.Entries[i].Action = data.BlacklistMode
		}
		if data.Entries[i].Action == "" {
			data.Entries[i].Action = "warn"
		}
	}
	data.Entries = keepValidEntries(data.Entries, report, func(e *models.BlacklistSettings) (string, bool) {
		e.Word = strings.ToLower(e.Word)
		return e.Word, validListKeyword(e.Word)
	})
	if err := replaceChatRows(tx, chatID, data.Entries); err != nil {
		return nil, err
	}
	return []string{cacheKey("blacklist", chatID)}, nil
}

func importCaptcha(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data CaptchaBackup
	if err := decodeModuleData(payload, BackupModuleCaptcha, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatID = chatID
	}
	if err := replaceChatSetting(tx, chatID, data.Settings); err != nil {
		return nil, err
	}
	return []string{cacheKey("captcha_settings", chatID)}, nil
}

func importConnections(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data ConnectionsBackup
	if err := decodeModuleData(payload, BackupModuleConnections, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
	}
	return nil, replaceChatSetting(tx, chatID, data.Settings)
}

func importDisabling(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) { //nolint:dupl // module-specific schema
	var data DisablingBackup
	if err := decodeModuleData(payload, BackupModuleDisabling, &data); err != nil {
		return nil, err
	}
	if data.ChatSettings != nil {
		data.ChatSettings.ChatId = chatID
	}
	for i := range data.Commands {
		if data.Commands[i].Command == "" {
			return nil, fmt.Errorf("invalid empty disabled command")
		}
		data.Commands[i].ChatId = chatID
	}
	if err := replaceChatSetting(tx, chatID, data.ChatSettings); err != nil {
		return nil, err
	}
	if err := replaceChatRows(tx, chatID, data.Commands); err != nil {
		return nil, err
	}
	return []string{cacheKey("disabled_cmds", chatID)}, nil
}

func importFilters(tx *gorm.DB, chatID int64, payload interface{}, report *ImportReport) ([]string, error) { //nolint:dupl // module-specific schema
	var data FiltersBackup
	if err := decodeModuleData(payload, BackupModuleFilters, &data); err != nil {
		return nil, err
	}
	for i := range data.Filters {
		if data.Filters[i].KeyWord == "" {
			return nil, fmt.Errorf("invalid empty filter keyword")
		}
		data.Filters[i].ChatId = chatID
	}
	data.Filters = keepValidEntries(data.Filters, report, func(f *models.ChatFilters) (string, bool) {
		f.KeyWord = strings.ToLower(f.KeyWord)
		return f.KeyWord, validListKeyword(f.KeyWord) && validButtonLinks(f.Buttons)
	})
	if err := replaceChatRows(tx, chatID, data.Filters); err != nil {
		return nil, err
	}
	return []string{
		cacheKey("filter_list", chatID),
		cacheKey("filters_optimized", chatID),
	}, nil
}

func importGreetings(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data GreetingsBackup
	if err := decodeModuleData(payload, BackupModuleGreetings, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatID = chatID
		// replaceChatSetting writes SQL NULL for every column of a nil embedded block, which the NOT NULL
		// switch columns of migration 20261105120000 refuse (SQLSTATE 23502). An empty block writes the
		// zero values those NULLs always read as.
		if data.Settings.WelcomeSettings == nil {
			data.Settings.WelcomeSettings = &models.WelcomeSettings{}
		}
		if data.Settings.GoodbyeSettings == nil {
			data.Settings.GoodbyeSettings = &models.GoodbyeSettings{}
		}
		dropGreetingMessageIDs(data.Settings)
	}
	if err := replaceChatSetting(tx, chatID, data.Settings); err != nil {
		return nil, err
	}
	return []string{cacheKey("greetings", chatID)}, nil
}

func importLocks(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) { //nolint:dupl // module-specific schema
	var data LocksBackup
	if err := decodeModuleData(payload, BackupModuleLocks, &data); err != nil {
		return nil, err
	}
	for i := range data.Locks {
		if data.Locks[i].LockType == "" {
			return nil, fmt.Errorf("invalid empty lock type")
		}
		data.Locks[i].ChatId = chatID
	}
	if err := replaceChatRows(tx, chatID, data.Locks); err != nil {
		return nil, err
	}
	return []string{cacheKey("lock", chatID), cacheKey("locks_map", chatID)}, nil
}

func importNotes(tx *gorm.DB, chatID int64, payload interface{}, preserveLegacyOmissions bool, report *ImportReport) ([]string, error) { //nolint:dupl // module-specific schema
	restoreSettings := !preserveLegacyOmissions || moduleFieldPresent(payload, "settings")
	var data NotesBackup
	if err := decodeModuleData(payload, BackupModuleNotes, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
	}
	for i := range data.Notes {
		if data.Notes[i].NoteName == "" {
			return nil, fmt.Errorf("invalid empty note name")
		}
		data.Notes[i].ChatId = chatID
	}
	data.Notes = keepValidEntries(data.Notes, report, func(n *models.Notes) (string, bool) {
		n.NoteName = strings.ToLower(n.NoteName)
		return n.NoteName, !content.NoteNameInvalid(n.NoteName) && validButtonLinks(n.Buttons)
	})
	if restoreSettings {
		if err := replaceChatSetting(tx, chatID, data.Settings); err != nil {
			return nil, err
		}
	}
	if err := replaceChatRows(tx, chatID, data.Notes); err != nil {
		return nil, err
	}
	return []string{cacheKey("notes_settings", chatID), cacheKey("notes_list", chatID)}, nil
}

func importPins(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data PinsBackup
	if err := decodeModuleData(payload, BackupModulePins, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
	}
	return nil, replaceChatSetting(tx, chatID, data.Settings)
}

func importReactions(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data ReactionsBackup
	if err := decodeModuleData(payload, BackupModuleReactions, &data); err != nil {
		return nil, err
	}
	for i := range data.Reactions {
		if data.Reactions[i].Keyword == "" || data.Reactions[i].Emoji == "" {
			return nil, fmt.Errorf("invalid reaction")
		}
		data.Reactions[i].ChatID = chatID
	}
	if err := replaceChatRows(tx, chatID, data.Reactions); err != nil {
		return nil, err
	}
	return []string{cacheKey("reactions", chatID)}, nil
}

func importReports(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data ReportsBackup
	if err := decodeModuleData(payload, BackupModuleReports, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
		data.Settings.Status = data.Settings.Enabled
	}
	return nil, replaceChatSetting(tx, chatID, data.Settings)
}

func importRules(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data RulesBackup
	if err := decodeModuleData(payload, BackupModuleRules, &data); err != nil {
		return nil, err
	}
	if data.Settings != nil {
		data.Settings.ChatId = chatID
	}
	return nil, replaceChatSetting(tx, chatID, data.Settings)
}

func importWarns(tx *gorm.DB, chatID int64, payload interface{}, preserveLegacyOmissions bool) ([]string, error) {
	restoreWarns := !preserveLegacyOmissions || moduleFieldPresent(payload, "warns")
	var data WarnsBackup
	if err := decodeModuleData(payload, BackupModuleWarns, &data); err != nil {
		return nil, err
	}
	if data.WarnSettings != nil {
		data.WarnSettings.ChatId = chatID
		if data.WarnSettings.WarnLimit <= 0 {
			return nil, fmt.Errorf("invalid warn limit %d", data.WarnSettings.WarnLimit)
		}
		// An export without a mode (the field is omitempty) or with an empty one is stored as mute,
		// the warn module's default: an empty mode applies nothing at the limit.
		if data.WarnSettings.WarnMode == "" {
			data.WarnSettings.WarnMode = "mute"
		}
	}
	for i := range data.Warns {
		if data.Warns[i].UserId == 0 || data.Warns[i].NumWarns < 0 {
			return nil, fmt.Errorf("invalid warn record")
		}
		data.Warns[i].ChatId = chatID
	}
	if len(data.Warns) > 0 {
		users := make([]models.User, len(data.Warns))
		for i := range data.Warns {
			users[i].UserId = data.Warns[i].UserId
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(&users).Error; err != nil {
			return nil, fmt.Errorf("ensure warned users: %w", err)
		}
	}

	var oldUserIDs []int64
	if restoreWarns {
		if err := tx.Model(&models.Warns{}).Where("chat_id = ?", chatID).Pluck("user_id", &oldUserIDs).Error; err != nil {
			return nil, err
		}
	}
	if err := replaceChatSetting(tx, chatID, data.WarnSettings); err != nil {
		return nil, err
	}
	if restoreWarns {
		if err := replaceChatRows(tx, chatID, data.Warns); err != nil {
			return nil, err
		}
	}

	keys := []string{cacheKey("warn_settings", chatID)}
	for _, userID := range oldUserIDs {
		keys = append(keys, dbcache.CacheKey("warns", userID, chatID))
	}
	if restoreWarns {
		for _, row := range data.Warns {
			keys = append(keys, dbcache.CacheKey("warns", row.UserId, chatID))
		}
	}
	return keys, nil
}

func moduleFieldPresent(payload interface{}, field string) bool {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return false
	}
	_, ok := fields[field]
	return ok
}

func clearModuleData(tx *gorm.DB, chatID int64, module string) ([]string, error) {
	if err := ensureBackupChat(tx, chatID); err != nil {
		return nil, err
	}
	switch module {
	case BackupModuleAdmin:
		return clearAdmin(tx, chatID)
	case BackupModuleAntiflood:
		return clearAntiflood(tx, chatID)
	case BackupModuleAntiraid:
		return clearAntiraid(tx, chatID)
	case BackupModuleApprovals:
		return clearApprovals(tx, chatID)
	case BackupModuleBlacklists:
		return clearBlacklists(tx, chatID)
	case BackupModuleCaptcha:
		return clearCaptcha(tx, chatID)
	case BackupModuleConnections:
		return clearConnections(tx, chatID)
	case BackupModuleDisabling:
		return clearDisabling(tx, chatID)
	case BackupModuleFilters:
		return clearFilters(tx, chatID)
	case BackupModuleGreetings:
		return clearGreetings(tx, chatID)
	case BackupModuleLocks:
		return clearLocks(tx, chatID)
	case BackupModuleNotes:
		return clearNotes(tx, chatID)
	case BackupModulePins:
		return clearPins(tx, chatID)
	case BackupModuleReactions:
		return clearReactions(tx, chatID)
	case BackupModuleReports:
		return clearReports(tx, chatID)
	case BackupModuleRules:
		return clearRules(tx, chatID)
	case BackupModuleWarns:
		return clearWarns(tx, chatID)
	case BackupModuleFederations:
		return clearFederations(tx, chatID)
	case BackupModuleLogChannels:
		return clearLogChannels(tx, chatID)
	default:
		return nil, fmt.Errorf("unknown module: %s", module)
	}
}

func ensureBackupChat(tx *gorm.DB, chatID int64) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoNothing: true,
	}).Create(&models.Chat{ChatId: chatID}).Error
}

func clearAdmin(tx *gorm.DB, chatID int64) ([]string, error) {
	if err := replaceChatSetting(tx, chatID, &models.AdminSettings{ChatId: chatID}); err != nil {
		return nil, err
	}
	if _, err := clearAntiflood(tx, chatID); err != nil {
		return nil, err
	}
	if _, err := clearCaptcha(tx, chatID); err != nil {
		return nil, err
	}
	if _, err := clearConnections(tx, chatID); err != nil {
		return nil, err
	}
	if err := tx.Model(&models.BlacklistSettings{}).
		Where("chat_id = ?", chatID).
		Update("action", "warn").Error; err != nil {
		return nil, err
	}
	return []string{
		cacheKey("antiflood", chatID),
		cacheKey("captcha_settings", chatID),
		cacheKey("blacklist", chatID),
	}, nil
}

func clearAntiflood(tx *gorm.DB, chatID int64) ([]string, error) {
	settings := &models.AntifloodSettings{ChatId: chatID, Limit: 0, Action: "mute"}
	return []string{cacheKey("antiflood", chatID)}, replaceChatSetting(tx, chatID, settings)
}

func clearAntiraid(tx *gorm.DB, chatID int64) ([]string, error) {
	settings := &models.AntiRaidSettings{
		ChatID:                chatID,
		RaidTime:              21600,
		RaidActionTime:        3600,
		AutoAntiRaidThreshold: 0,
	}
	return []string{cacheKey("antiraid", chatID)}, replaceChatSetting(tx, chatID, settings)
}

func clearApprovals(tx *gorm.DB, chatID int64) ([]string, error) {
	return []string{cacheKey("approvals", chatID)}, replaceChatRows[models.ApprovedUsers](tx, chatID, nil)
}

func clearBlacklists(tx *gorm.DB, chatID int64) ([]string, error) {
	return []string{cacheKey("blacklist", chatID)}, replaceChatRows[models.BlacklistSettings](tx, chatID, nil)
}

func clearCaptcha(tx *gorm.DB, chatID int64) ([]string, error) {
	settings := &models.CaptchaSettings{
		ChatID:        chatID,
		CaptchaMode:   "math",
		Timeout:       2,
		FailureAction: "kick",
		MaxAttempts:   3,
	}
	return []string{cacheKey("captcha_settings", chatID)}, replaceChatSetting(tx, chatID, settings)
}

func clearConnections(tx *gorm.DB, chatID int64) ([]string, error) {
	return nil, replaceChatSetting(tx, chatID, &models.ConnectionChatSettings{ChatId: chatID})
}

func clearDisabling(tx *gorm.DB, chatID int64) ([]string, error) {
	if err := replaceChatSetting(tx, chatID, &models.DisableChatSettings{ChatId: chatID}); err != nil {
		return nil, err
	}
	return []string{cacheKey("disabled_cmds", chatID)}, replaceChatRows[models.DisableSettings](tx, chatID, nil)
}

func clearFilters(tx *gorm.DB, chatID int64) ([]string, error) {
	return []string{
		cacheKey("filter_list", chatID),
		cacheKey("filters_optimized", chatID),
	}, replaceChatRows[models.ChatFilters](tx, chatID, nil)
}

func clearGreetings(tx *gorm.DB, chatID int64) ([]string, error) {
	settings := &models.GreetingSettings{
		ChatID: chatID,
		WelcomeSettings: &models.WelcomeSettings{
			WelcomeText: db.DefaultWelcome,
			WelcomeType: db.TEXT,
			Button:      models.ButtonArray{},
		},
		GoodbyeSettings: &models.GoodbyeSettings{
			GoodbyeText: db.DefaultGoodbye,
			GoodbyeType: db.TEXT,
			Button:      models.ButtonArray{},
		},
	}
	return []string{cacheKey("greetings", chatID)}, replaceChatSetting(tx, chatID, settings)
}

func clearLocks(tx *gorm.DB, chatID int64) ([]string, error) {
	return []string{
		cacheKey("lock", chatID),
		cacheKey("locks_map", chatID),
	}, replaceChatRows[models.LockSettings](tx, chatID, nil)
}

func clearNotes(tx *gorm.DB, chatID int64) ([]string, error) {
	if err := replaceChatSetting(tx, chatID, &models.NotesSettings{ChatId: chatID}); err != nil {
		return nil, err
	}
	return []string{cacheKey("notes_settings", chatID), cacheKey("notes_list", chatID)}, replaceChatRows[models.Notes](tx, chatID, nil)
}

func clearPins(tx *gorm.DB, chatID int64) ([]string, error) {
	return nil, replaceChatSetting(tx, chatID, &models.PinSettings{ChatId: chatID})
}

func clearReactions(tx *gorm.DB, chatID int64) ([]string, error) {
	return []string{cacheKey("reactions", chatID)}, replaceChatRows[models.Reactions](tx, chatID, nil)
}

func clearReports(tx *gorm.DB, chatID int64) ([]string, error) {
	settings := &models.ReportChatSettings{
		ChatId:      chatID,
		Enabled:     true,
		Status:      true,
		BlockedList: models.Int64Array{},
	}
	return nil, replaceChatSetting(tx, chatID, settings)
}

func clearRules(tx *gorm.DB, chatID int64) ([]string, error) {
	return nil, replaceChatSetting(tx, chatID, &models.RulesSettings{ChatId: chatID})
}

func clearWarns(tx *gorm.DB, chatID int64) ([]string, error) {
	var userIDs []int64
	if err := tx.Model(&models.Warns{}).Where("chat_id = ?", chatID).Pluck("user_id", &userIDs).Error; err != nil {
		return nil, err
	}
	if err := replaceChatSetting(tx, chatID, &models.WarnSettings{ChatId: chatID, WarnLimit: 3, WarnMode: "mute"}); err != nil {
		return nil, err
	}
	if err := replaceChatRows[models.Warns](tx, chatID, nil); err != nil {
		return nil, err
	}
	keys := []string{cacheKey("warn_settings", chatID)}
	for _, userID := range userIDs {
		keys = append(keys, dbcache.CacheKey("warns", userID, chatID))
	}
	return keys, nil
}

func importFederations(tx *gorm.DB, chatID int64, payload interface{}) ([]string, error) {
	var data FederationsBackup
	if err := decodeModuleData(payload, BackupModuleFederations, &data); err != nil {
		return nil, err
	}
	if err := tx.Where("chat_id = ?", chatID).Delete(&models.FederationChat{}).Error; err != nil {
		return nil, err
	}
	if data.Membership != nil && data.Membership.FedID != "" {
		row := models.FederationChat{
			FedID:  data.Membership.FedID,
			ChatID: chatID,
			Quiet:  data.Membership.Quiet,
		}
		if err := tx.Create(&row).Error; err != nil {
			return nil, fmt.Errorf("restore federation membership: %w", err)
		}
	}
	return []string{cacheKey("fed_chat", chatID)}, nil
}

// importLogChannels never imports the log channel itself: a backup names the source group's channel, and a
// link must be proved by an administrator of that channel (/setlog). The category flags are applied to the
// target's own log channel when it has one; with none the import leaves them out and says so in the report.
// A backup with no log channel leaves the target's link alone.
func importLogChannels(tx *gorm.DB, chatID int64, payload interface{}, report *ImportReport) ([]string, error) {
	var data LogChannelsBackup
	if err := decodeModuleData(payload, BackupModuleLogChannels, &data); err != nil {
		return nil, err
	}
	if data.Settings == nil {
		return nil, nil
	}
	var current models.LogChannel
	err := tx.Where("chat_id = ?", chatID).First(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && current.LogChannelID == 0) {
		report.LogChannelSkipped = true
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Model(&models.LogChannel{}).Where("chat_id = ?", chatID).Updates(map[string]interface{}{
		"cat_settings":  data.Settings.CatSettings,
		"cat_admin":     data.Settings.CatAdmin,
		"cat_user":      data.Settings.CatUser,
		"cat_automated": data.Settings.CatAutomated,
		"cat_reports":   data.Settings.CatReports,
		"cat_other":     data.Settings.CatOther,
	}).Error; err != nil {
		return nil, err
	}
	return []string{cacheKey("log_channel", chatID)}, nil
}

// validListKeyword is the check /filter and /addblacklist make of a keyword or word: not too broad and at
// most content.ListKeywordMaxRunes code points.
func validListKeyword(word string) bool {
	return !content.KeywordTooBroad(word) && utf8.RuneCountInString(word) <= content.ListKeywordMaxRunes
}

// validButtonLinks reports whether every button link may be stored (content.ValidButtonURL).
func validButtonLinks(buttons models.ButtonArray) bool {
	for _, button := range buttons {
		if !content.ValidButtonURL(button.Url) {
			return false
		}
	}
	return true
}

// keepValidEntries keeps the entries check accepts, in order, and the first of those that share a key;
// every other entry is left out and counted in report.Skipped. check may normalise the entry.
func keepValidEntries[T any](entries []T, report *ImportReport, check func(*T) (string, bool)) []T {
	kept := entries[:0]
	seen := make(map[string]bool, len(entries))
	for i := range entries {
		key, ok := check(&entries[i])
		if !ok || seen[key] {
			report.Skipped++
			continue
		}
		seen[key] = true
		kept = append(kept, entries[i])
	}
	return kept
}

func clearFederations(tx *gorm.DB, chatID int64) ([]string, error) {
	if err := tx.Where("chat_id = ?", chatID).Delete(&models.FederationChat{}).Error; err != nil {
		return nil, err
	}
	return []string{cacheKey("fed_chat", chatID)}, nil
}

func clearLogChannels(tx *gorm.DB, chatID int64) ([]string, error) {
	if err := replaceChatSetting(tx, chatID, (*models.LogChannel)(nil)); err != nil {
		return nil, err
	}
	return []string{cacheKey("log_channel", chatID)}, nil
}
