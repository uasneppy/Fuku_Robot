package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/eko/gocache/lib/v4/store"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/actionlog"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

const logHandlerGroup = 11

var logChannelsModule = moduleStruct{
	moduleName:   "LogChannels",
	handlerGroup: logHandlerGroup,
}

func setlogPendingKey(channelID, messageID int64) string {
	return fmt.Sprintf("fuku:setlog:%d:%d", channelID, messageID)
}

func (moduleStruct) setLog(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	from := requireUser(b, ctx)
	if from == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	if chat == nil || msg == nil {
		return ext.EndGroups
	}
	if chat.Type == "channel" {
		m := cache.GetMarshal()
		if m == nil {
			text, _ := tr.GetString("common_settings_save_failed")
			replyHTML(b, msg, text)
			return ext.EndGroups
		}
		if err := m.Set(
			cache.Context,
			setlogPendingKey(chat.Id, msg.MessageId),
			[]byte("1"),
			store.WithExpiration(time.Hour),
		); err != nil {
			log.Errorf("[LogChannels] failed to store pending setlog: %v", err)
			text, _ := tr.GetString("common_settings_save_failed")
			replyHTML(b, msg, text)
			return ext.EndGroups
		}
		text, _ := tr.GetString("logs_forward_to_group")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	text, _ := tr.GetString("logs_need_channel")
	replyHTML(b, msg, text)
	return ext.EndGroups
}

func (moduleStruct) unsetLog(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	from := chat_status.RequireUser(b, ctx)
	if from == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	if chat == nil || chat.Type == "private" {
		text, _ := tr.GetString("logs_group_only")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	if !chat_status.RequireUserAdmin(b, ctx, chat, from.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}
	if err := logchannels.Unset(chat.Id); err != nil {
		text, _ := tr.GetString("logs_not_set")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	text, _ := tr.GetString("logs_unset_ok")
	replyHTML(b, msg, text)
	actionlog.Settings(b, chat, from, "unset log channel")
	return ext.EndGroups
}

func (moduleStruct) logChannel(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	if chat_status.RequireUser(b, ctx) == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	if chat == nil || chat.Type == "private" {
		text, _ := tr.GetString("logs_group_only")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	settings := logchannels.GetContext(tracing.UpdateContext(ctx), chat.Id)
	if settings == nil {
		text, _ := tr.GetString("logs_not_set")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	text, _ := tr.GetString("logs_current", i18n.TranslationParams{
		"id": strconv.FormatInt(settings.LogChannelID, 10),
	})
	replyHTML(b, msg, text)
	return ext.EndGroups
}

func (moduleStruct) logCategories(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if chat_status.RequireUser(b, ctx) == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	text, _ := tr.GetString("logs_categories")
	replyHTML(b, msg, text)
	return ext.EndGroups
}

func (m moduleStruct) enableLog(b *gotgbot.Bot, ctx *ext.Context) error {
	return m.setLogCategories(b, ctx, true)
}

func (m moduleStruct) disableLog(b *gotgbot.Bot, ctx *ext.Context) error {
	return m.setLogCategories(b, ctx, false)
}

// logCategoryWrites is the one-category write applyLogCategory goes through. It is a variable
// only so a test can make the write fail.
var logCategoryWrites = logchannels.SetCategory

// logApplyOutcome is how applyLogCategory ended.
type logApplyOutcome int

const (
	// logApplied is the zero value: the category was stored (or was already in that state).
	logApplied logApplyOutcome = iota
	// logUnknownCategory is a name that is not one of logchannels.AllCategories; nothing was
	// written.
	logUnknownCategory
	// logNotSet is a chat with no linked log channel; nothing was written.
	logNotSet
	// logSaveFailed is a write that failed; Err holds the error.
	logSaveFailed
)

// logApplyResult is what applyLogCategory reports: the outcome, the locale key of the reply the
// commands use for it (logs_unknown_category, logs_not_set, common_settings_save_failed, or
// logs_enabled / logs_disabled for an applied write), the log channel row read back after an
// applied write (nil when the read-back failed or found no channel) and the error: the write's
// for logSaveFailed, the read-back's for an applied write whose read-back failed.
type logApplyResult struct {
	Outcome logApplyOutcome
	Key     string
	Row     *models.LogChannel
	Err     error
}

// applyLogCategory turns one log category of a chat on or off. It is the one write path of
// /log and /nolog (which call it once per category they are given and keep their own loop,
// checks, reply and #SETTINGS audit post) and of the settings menu's Log channel page. The name
// is validated with logchannels.IsValidCategory, which lower-cases and trims it like the
// repository does, before any write; the write (logchannels.SetCategory) drops the
// log_channel:<chat> cache key. It sends nothing.
func applyLogCategory(ctx context.Context, chatID int64, name string, on bool) logApplyResult {
	if !logchannels.IsValidCategory(name) {
		return logApplyResult{Outcome: logUnknownCategory, Key: "logs_unknown_category"}
	}
	if err := logCategoryWrites(chatID, name, on); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return logApplyResult{Outcome: logNotSet, Key: "logs_not_set", Err: err}
		}
		log.Errorf("[LogChannels] SetCategory failed for chat %d: %v", chatID, err)
		return logApplyResult{Outcome: logSaveFailed, Key: "common_settings_save_failed", Err: err}
	}
	key := "logs_disabled"
	if on {
		key = "logs_enabled"
	}
	row, err := logchannels.LoadContext(ctx, chatID)
	return logApplyResult{Outcome: logApplied, Key: key, Row: row, Err: err}
}

func (moduleStruct) setLogCategories(b *gotgbot.Bot, ctx *ext.Context, enable bool) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	from := chat_status.RequireUser(b, ctx)
	if from == nil {
		return ext.EndGroups
	}
	tr := ctxTr(ctx)
	if chat == nil || chat.Type == "private" {
		text, _ := tr.GetString("logs_group_only")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	if !chat_status.RequireUserAdmin(b, ctx, chat, from.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}
	if logchannels.GetContext(tracing.UpdateContext(ctx), chat.Id) == nil {
		text, _ := tr.GetString("logs_not_set")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	args := ctx.Args()[1:]
	if len(args) == 0 {
		text, _ := tr.GetString("logs_usage")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	var unknown []string
	var changed []string
	for _, raw := range args {
		name := strings.ToLower(strings.TrimSpace(raw))
		// A failed save is logged by applyLogCategory and the loop goes on: when every save
		// fails the reply and the audit post carry an empty list (pinned by the golden).
		switch res := applyLogCategory(tracing.UpdateContext(ctx), chat.Id, name, enable); res.Outcome {
		case logUnknownCategory:
			unknown = append(unknown, name)
		case logNotSet:
			text, _ := tr.GetString(res.Key)
			replyHTML(b, msg, text)
			return ext.EndGroups
		case logApplied:
			changed = append(changed, name)
		case logSaveFailed:
		}
	}
	if len(unknown) > 0 && len(changed) == 0 {
		text, _ := tr.GetString("logs_unknown_category")
		replyHTML(b, msg, text)
		return ext.EndGroups
	}
	key := "logs_disabled"
	if enable {
		key = "logs_enabled"
	}
	text, _ := tr.GetString(key, i18n.TranslationParams{
		"cats": strings.Join(changed, ", "),
	})
	replyHTML(b, msg, text)
	actionlog.Settings(b, chat, from, key+" "+strings.Join(changed, ", "))
	return ext.EndGroups
}

func (moduleStruct) captureSetLogForward(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	if msg == nil || chat == nil || chat.Type == "private" || chat.Type == "channel" {
		return ext.ContinueGroups
	}
	if msg.ForwardOrigin == nil {
		return ext.ContinueGroups
	}
	origin := msg.ForwardOrigin.MergeMessageOrigin()
	if origin.Chat == nil || origin.Chat.Type != "channel" {
		return ext.ContinueGroups
	}
	from := chat_status.RequireUser(b, ctx)
	if from == nil {
		return ext.ContinueGroups
	}
	if !chat_status.RequireUserAdmin(b, ctx, chat, from.Id) {
		return ext.ContinueGroups
	}
	if origin.MessageId == 0 {
		return ext.ContinueGroups
	}
	pending := false
	key := setlogPendingKey(origin.Chat.Id, origin.MessageId)
	if rdb := cache.GetRedisClient(); rdb != nil {
		// Atomic consume: concurrent forwards of the same channel message bind once.
		if err := rdb.GetDel(cache.Context, key).Err(); err == nil {
			pending = true
		}
	} else if m := cache.GetMarshal(); m != nil {
		var marker []byte
		if _, err := m.Get(cache.Context, key, &marker); err == nil {
			pending = true
			_ = m.Delete(cache.Context, key)
		}
	}
	if !pending {
		return ext.ContinueGroups
	}
	if err := logchannels.Set(chat.Id, chat.Title, origin.Chat.Id); err != nil {
		log.Errorf("[LogChannels] Set: %v", err)
		return ext.ContinueGroups
	}
	tr := ctxTr(ctx)
	text, _ := tr.GetString("logs_set_ok")
	replyHTML(b, msg, text)
	actionlog.Settings(b, chat, from, "set log channel")
	return ext.ContinueGroups
}

func LoadLogChannels(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(logChannelsModule.moduleName, true)

	dispatcher.AddHandler(helpers.NewBotCommand("setlog", logChannelsModule.setLog))
	dispatcher.AddHandler(helpers.NewBotCommand("unsetlog", logChannelsModule.unsetLog))
	dispatcher.AddHandler(helpers.NewBotCommand("logchannel", logChannelsModule.logChannel))
	dispatcher.AddHandler(helpers.NewBotCommand("logcategories", logChannelsModule.logCategories))
	dispatcher.AddHandler(helpers.NewBotCommand("log", logChannelsModule.enableLog))
	dispatcher.AddHandler(helpers.NewBotCommand("nolog", logChannelsModule.disableLog))

	dispatcher.AddHandlerToGroup(
		handlers.NewMessage(message.All, logChannelsModule.captureSetLogForward),
		logChannelsModule.handlerGroup,
	)
}

func init() {
	RegisterLegacyModule("LogChannels", 55, LoadLogChannels)
}
