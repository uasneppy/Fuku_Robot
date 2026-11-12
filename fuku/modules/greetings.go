package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/chatjoinrequest"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/captcha"
	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/lockdown"
	"github.com/uasneppy/fuku_robot/fuku/db/memberreq"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/keyboard"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

var recentJoinProcessTTL = 5 * time.Second

var greetingsModule = moduleStruct{moduleName: "Greetings"}
var recentJoinProcessing sync.Map

type joinClaim struct{ exp time.Time }

type greetingType int

const (
	greetingWelcome greetingType = iota
	greetingGoodbye
)

type greetingConfig struct {
	gType            greetingType
	logContext       string
	notConfiguredKey string
	statusKey        string
	enabledKey       string
	disabledKey      string
	invalidKey       string
}

var welcomeConfig = greetingConfig{
	gType:            greetingWelcome,
	logContext:       "welcome",
	notConfiguredKey: "greetings_welcome_not_configured",
	statusKey:        "greetings_welcome_status",
	enabledKey:       "greetings_welcome_enabled",
	disabledKey:      "greetings_welcome_disabled",
	invalidKey:       "greetings_welcome_invalid_option",
}

var goodbyeConfig = greetingConfig{
	gType:            greetingGoodbye,
	logContext:       "goodbye",
	notConfiguredKey: "greetings_goodbye_not_configured",
	statusKey:        "greetings_goodbye_status",
	enabledKey:       "greetings_goodbye_enable",
	disabledKey:      "greetings_goodbye_disable",
	invalidKey:       "greetings_goodbye_invalid",
}

func recentJoinProcessingKey(chatID, userID int64) string {
	return fmt.Sprintf("fuku:recentJoinProcessing:%d:%d", chatID, userID)
}

func claimRecentJoinProcessing(chatID, userID int64) bool {
	key := recentJoinProcessingKey(chatID, userID)

	if rdb := cache.GetRedisClient(); rdb != nil {
		_, err := rdb.SetArgs(cache.Context, key, true, redis.SetArgs{
			Mode: "NX",
			TTL:  recentJoinProcessTTL,
		}).Result()
		if err == nil {
			return true
		}
		if errors.Is(err, redis.Nil) {
			return false
		}
		// Genuine Redis error — fail closed to avoid double welcome/captcha in multi-instance.
		log.Debugf("[Greetings] Redis SETNX failed for join dedupe key %s, failing closed: %v", key, err)
		return false
	}

	now := time.Now()
	for {
		newClaim := joinClaim{exp: now.Add(recentJoinProcessTTL)}
		actual, loaded := recentJoinProcessing.LoadOrStore(key, newClaim)
		if !loaded {
			expireRecentJoinClaim(key, newClaim)
			return true
		}
		if c, ok := actual.(joinClaim); ok && now.Before(c.exp) {
			return false
		}
		newClaim = joinClaim{exp: time.Now().Add(recentJoinProcessTTL)}
		if recentJoinProcessing.CompareAndSwap(key, actual, newClaim) {
			expireRecentJoinClaim(key, newClaim)
			return true
		}
		now = time.Now()
	}
}

func expireRecentJoinClaim(key string, claim joinClaim) {
	time.AfterFunc(recentJoinProcessTTL, func() {
		recentJoinProcessing.CompareAndDelete(key, claim)
	})
}

func clearRecentJoinProcessing(chatID, userID int64) {
	key := recentJoinProcessingKey(chatID, userID)

	if rdb := cache.GetRedisClient(); rdb != nil {
		if err := rdb.Del(cache.Context, key).Err(); err != nil {
			log.Debugf("[Greetings] Failed to clear shared join dedupe key %s: %v", key, err)
		}
	}

	recentJoinProcessing.Delete(key)
}

// greetingApplyOutcome says whether a greeting write was stored.
type greetingApplyOutcome int

const (
	// greetingApplied means the write was stored.
	greetingApplied greetingApplyOutcome = iota
	// greetingSaveFailed means the write failed and nothing was changed.
	greetingSaveFailed
)

// greetingApplyResult is what every greeting apply function returns: the outcome, the
// stored settings read back with greetings.LoadSettingsContext (nil when the write failed
// or the read-back did, in which case the caller reloads) and the repository error.
type greetingApplyResult struct {
	Outcome  greetingApplyOutcome
	Settings *models.GreetingSettings
	Err      error
}

// greetingWrites holds the greetings repository writes the apply functions call. It is a
// variable only so a test can make a write fail.
var greetingWrites = struct {
	WelcomeToggle      func(chatID int64, on bool) error
	GoodbyeToggle      func(chatID int64, on bool) error
	CleanWelcome       func(chatID int64, on bool) error
	CleanGoodbye       func(chatID int64, on bool) error
	WelcomeTextOnly    func(chatID int64, text string) error
	WelcomeMediaOnly   func(chatID int64, fileID string, dataType int) error
	WelcomeButtonsOnly func(chatID int64, buttons []db.Button) error
	GoodbyeTextOnly    func(chatID int64, text string) error
	GoodbyeMediaOnly   func(chatID int64, fileID string, dataType int) error
	GoodbyeButtons     func(chatID int64, buttons []db.Button) error
	WelcomeText        func(chatID int64, text, fileID string, buttons []db.Button, dataType int) error
	GoodbyeText        func(chatID int64, text, fileID string, buttons []db.Button, dataType int) error
	WelcomeMediaBelow  func(chatID int64, on bool) error
	GoodbyeMediaBelow  func(chatID int64, on bool) error
	WelcomeOnce        func(chatID int64, on bool) error
	AutoApprove        func(chatID int64, on bool) error
}{
	WelcomeToggle:      greetings.SetWelcomeToggle,
	GoodbyeToggle:      greetings.SetGoodbyeToggle,
	CleanWelcome:       greetings.SetCleanWelcomeSetting,
	CleanGoodbye:       greetings.SetCleanGoodbyeSetting,
	WelcomeTextOnly:    greetings.SetWelcomeTextOnly,
	WelcomeMediaOnly:   greetings.SetWelcomeMediaOnly,
	WelcomeButtonsOnly: greetings.SetWelcomeButtonsOnly,
	GoodbyeTextOnly:    greetings.SetGoodbyeTextOnly,
	GoodbyeMediaOnly:   greetings.SetGoodbyeMediaOnly,
	GoodbyeButtons:     greetings.SetGoodbyeButtonsOnly,
	WelcomeText:        greetings.SetWelcomeText,
	GoodbyeText:        greetings.SetGoodbyeText,
	WelcomeMediaBelow:  greetings.SetWelcomeMediaBelow,
	GoodbyeMediaBelow:  greetings.SetGoodbyeMediaBelow,
	WelcomeOnce:        greetings.SetWelcomeOnce,
	AutoApprove:        greetings.SetShouldAutoApprove,
}

// greetingApplyTimeout bounds the read-back an apply function makes after its write.
const greetingApplyTimeout = 5 * time.Second

// applyReadBack is the shared tail of the greeting and rules apply functions. A failed
// write is reported through logWriteFailure and comes back as greetingSaveFailed with the
// write error. A stored write reads its value back with read, under its own timeout that
// outlives a cancelled request. A failed read-back is only reported through logReadFailure,
// because the write succeeded: the outcome is still greetingApplied and the value is the
// zero value, so the caller reloads.
func applyReadBack[T any](
	ctx context.Context,
	writeErr error,
	logWriteFailure, logReadFailure func(error),
	read func(context.Context) (T, error),
) (T, greetingApplyOutcome, error) {
	var zero T
	if writeErr != nil {
		logWriteFailure(writeErr)
		return zero, greetingSaveFailed, writeErr
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), greetingApplyTimeout)
	defer cancel()
	value, err := read(readCtx)
	if err != nil {
		logReadFailure(err)
		return zero, greetingApplied, nil
	}
	return value, greetingApplied, nil
}

// finishGreetingApply turns the error of one greeting write into the apply result: a
// failure is logged with the chat ID and returned as greetingSaveFailed, a success reads
// the stored row back. A failed read-back is only logged, because the write succeeded.
func finishGreetingApply(ctx context.Context, chatID int64, what string, writeErr error) greetingApplyResult {
	settings, outcome, err := applyReadBack(ctx, writeErr,
		func(e error) { log.Errorf("[Greetings] %s failed for chat %d: %v", what, chatID, e) },
		func(e error) { log.Warnf("[Greetings] read back settings of chat %d after %s: %v", chatID, what, e) },
		func(c context.Context) (*models.GreetingSettings, error) {
			return greetings.LoadSettingsContext(c, chatID)
		})
	return greetingApplyResult{Outcome: outcome, Settings: settings, Err: err}
}

// applyWelcomeEnabled stores the welcome switch. The value is absolute, so a repeated
// call stores the same row. It is the one write path of /welcome on|off and the menu.
func applyWelcomeEnabled(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetWelcomeToggle", greetingWrites.WelcomeToggle(chatID, on))
}

// applyGoodbyeEnabled stores the goodbye switch, the one write path of /goodbye on|off
// and the menu.
func applyGoodbyeEnabled(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetGoodbyeToggle", greetingWrites.GoodbyeToggle(chatID, on))
}

// applyWelcomeMediaBelow stores whether the welcome's photo, video or GIF is shown under its text. It is the one
// write path of the menu's Media under text switch (no command exists), column-targeted so nothing else of the row
// changes, and absolute so a repeated call stores the same row.
func applyWelcomeMediaBelow(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetWelcomeMediaBelow", greetingWrites.WelcomeMediaBelow(chatID, on))
}

// applyGoodbyeMediaBelow stores whether the goodbye's photo, video or GIF is shown under its text, the one write
// path of the menu's Media under text switch of the goodbye.
func applyGoodbyeMediaBelow(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetGoodbyeMediaBelow", greetingWrites.GoodbyeMediaBelow(chatID, on))
}

// applyWelcomeOnce stores the Greet once switch: whether a member is welcomed only the first time they ever join.
// It is the one write path of the Welcome page's switch (no command exists), column-targeted so nothing else of
// the row changes, and absolute so a repeated call stores the same row.
func applyWelcomeOnce(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetWelcomeOnce", greetingWrites.WelcomeOnce(chatID, on))
}

// greetOnceStore is the greet-once record the welcome path claims from and gives back to. It is a variable only
// so a test can make a claim fail. The record is the greeted_members table: never Redis and never
// claimRecentJoinProcessing, so it works with no Redis and across replicas.
var greetOnceStore = struct {
	Claim   func(chatID, userID int64) (bool, error)
	Release func(chatID, userID int64) error
}{
	Claim:   greetings.ClaimGreeting,
	Release: greetings.ReleaseGreeting,
}

// welcomeDefinitelyNotSent reports whether err proves Telegram did not deliver the welcome: the bot may not
// send there (media.ErrNoPermission), or Telegram answered with a 4xx (a wrong file ID, a lost right) or a 429
// (too many requests, answered without processing the request), classified by code and never by wording. A
// transport error, a timeout, a 5xx and an unreadable answer may have delivered it and are not definite.
func welcomeDefinitelyNotSent(err error) bool {
	if errors.Is(err, media.ErrNoPermission) || isTelegramClientError(err) {
		return true
	}
	var tgErr *gotgbot.TelegramError
	return errors.As(err, &tgErr) && tgErr != nil && tgErr.Code == 429
}

// claimWelcome takes the member's greet-once claim before the welcome is built and sent. It returns true when the
// welcome may be sent: Greet once is off (nothing is recorded), or this call won the claim. A lost claim means the
// member was welcomed before, and a claim database error fails closed: nothing is sent, because a double welcome
// is worse than a missing one.
func claimWelcome(chatID, userID int64, once bool) (send, claimed bool) {
	if !once {
		return true, false
	}
	won, err := greetOnceStore.Claim(chatID, userID)
	if err != nil {
		log.Errorf("[Greetings][SendWelcomeMessage] greet-once claim failed for chat %d: %v", chatID, err)
		return false, false
	}
	return won, won
}

// giveBackWelcome returns the claim of a welcome Telegram definitely refused, so the member's next join can still
// be welcomed. An unknown outcome keeps the claim. The store works on its own database connection and takes no
// request context, so a cancelled update cannot stop the give-back.
func giveBackWelcome(chatID, userID int64, sendErr error) {
	if !welcomeDefinitelyNotSent(sendErr) {
		return
	}
	if err := greetOnceStore.Release(chatID, userID); err != nil {
		log.Warnf("[Greetings][SendWelcomeMessage] greet-once release failed for chat %d: %v", chatID, err)
	}
}

// releaseUnsentWelcome returns the claim of a welcome that was never sent because the code between the claim and
// the send panicked. Like giveBackWelcome it works on the store's own connection, and a failure is only logged.
func releaseUnsentWelcome(chatID, userID int64) {
	if err := greetOnceStore.Release(chatID, userID); err != nil {
		log.Warnf("[Greetings][SendWelcomeMessage] greet-once release after a panic failed for chat %d user %d: %v",
			chatID, userID, err)
	}
}

// welcomeRender builds the welcome's text and keyboard. It is a variable only so a test can make the step between
// the greet-once claim and the send panic.
var welcomeRender = renderGreeting

// applyCleanWelcome stores the "delete the last welcome" switch.
func applyCleanWelcome(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetCleanWelcomeSetting", greetingWrites.CleanWelcome(chatID, on))
}

// applyCleanGoodbye stores the "delete the last goodbye" switch.
func applyCleanGoodbye(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetCleanGoodbyeSetting", greetingWrites.CleanGoodbye(chatID, on))
}

// applyAutoApprove stores the auto-approve switch of join requests. The value is absolute, so a
// repeated call stores the same row. It is the one write path of /autoapprove on|off and the menu.
// On a chat with no greetings row the first write creates the whole row with its defaults
// (greetings.SetShouldAutoApprove goes through upsertGreetingSettings), as /autoapprove on always did.
func applyAutoApprove(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "SetShouldAutoApprove", greetingWrites.AutoApprove(chatID, on))
}

// applyCleanService stores the shared /cleanservice flag, the on/off switch of the joined and
// left service messages, and removes both of their delay rows in one transaction, so a delay
// chosen before an off and on never comes back (D-26). It is the one write path of /cleanservice
// on and off, and drops the greetings and delete_rules cache keys.
func applyCleanService(ctx context.Context, chatID int64, on bool) greetingApplyResult {
	return finishGreetingApply(ctx, chatID, "ApplyCleanServiceFlag", deletion.ApplyCleanServiceFlag(ctx, chatID, on))
}

// applyGreetingText stores one greeting's text (the HTML the editor keeps) and nothing
// else of the row.
func applyGreetingText(ctx context.Context, chatID int64, isWelcome bool, html string) greetingApplyResult {
	if isWelcome {
		return finishGreetingApply(ctx, chatID, "SetWelcomeTextOnly", greetingWrites.WelcomeTextOnly(chatID, html))
	}
	return finishGreetingApply(ctx, chatID, "SetGoodbyeTextOnly", greetingWrites.GoodbyeTextOnly(chatID, html))
}

// applyGreetingMedia stores one greeting's file ID and media type and nothing else.
func applyGreetingMedia(ctx context.Context, chatID int64, isWelcome bool, fileID string, dataType int) greetingApplyResult {
	if isWelcome {
		return finishGreetingApply(ctx, chatID, "SetWelcomeMediaOnly", greetingWrites.WelcomeMediaOnly(chatID, fileID, dataType))
	}
	return finishGreetingApply(ctx, chatID, "SetGoodbyeMediaOnly", greetingWrites.GoodbyeMediaOnly(chatID, fileID, dataType))
}

// applyGreetingButtons stores one greeting's buttons and nothing else.
func applyGreetingButtons(ctx context.Context, chatID int64, isWelcome bool, buttons []db.Button) greetingApplyResult {
	if isWelcome {
		return finishGreetingApply(ctx, chatID, "SetWelcomeButtonsOnly", greetingWrites.WelcomeButtonsOnly(chatID, buttons))
	}
	return finishGreetingApply(ctx, chatID, "SetGoodbyeButtonsOnly", greetingWrites.GoodbyeButtons(chatID, buttons))
}

// applyGreetingRemoveText puts back the default text (db.DefaultWelcome or
// db.DefaultGoodbye), the text column a reset writes.
func applyGreetingRemoveText(ctx context.Context, chatID int64, isWelcome bool) greetingApplyResult {
	if isWelcome {
		return applyGreetingText(ctx, chatID, true, db.DefaultWelcome)
	}
	return applyGreetingText(ctx, chatID, false, db.DefaultGoodbye)
}

// applyGreetingRemoveMedia clears the file ID and makes the type text, the media columns
// a reset writes.
func applyGreetingRemoveMedia(ctx context.Context, chatID int64, isWelcome bool) greetingApplyResult {
	return applyGreetingMedia(ctx, chatID, isWelcome, "", db.TEXT)
}

// applyGreetingRemoveButtons clears the buttons, the column a reset writes.
func applyGreetingRemoveButtons(ctx context.Context, chatID int64, isWelcome bool) greetingApplyResult {
	return applyGreetingButtons(ctx, chatID, isWelcome, nil)
}

// applyGreetingReset is the four-column write /resetwelcome and /resetgoodbye make: the
// default text, no file, no buttons and the text type.
func applyGreetingReset(ctx context.Context, chatID int64, isWelcome bool) greetingApplyResult {
	if isWelcome {
		return finishGreetingApply(ctx, chatID, "SetWelcomeText", greetingWrites.WelcomeText(chatID, db.DefaultWelcome, "", nil, db.TEXT))
	}
	return finishGreetingApply(ctx, chatID, "SetGoodbyeText", greetingWrites.GoodbyeText(chatID, db.DefaultGoodbye, "", nil, db.TEXT))
}

// renderGreeting is the one builder of a member-facing greeting: the join path, the leave
// path and /welcome's formatted display send what it returns, and a preview can call it.
// It replaces the placeholders of the text and the buttons and builds the keyboard. It
// makes no write and sends nothing, so the clean-old bookkeeping stays with the senders.
func renderGreeting(b *gotgbot.Bot, group *gotgbot.Chat, user *gotgbot.User, text string, buttons []db.Button) (string, *gotgbot.InlineKeyboardMarkup) {
	res, btns := formatting.FormattingReplacer(b, group, user, text, buttons)
	return res, &gotgbot.InlineKeyboardMarkup{InlineKeyboard: keyboard.BuildKeyboard(btns)}
}

// greetingApplyContext is the context the commands give an apply function.
func greetingApplyContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), greetingApplyTimeout)
}

//nolint:dupl // displayGreeting has symmetric welcome/goodbye logic by design
func (moduleStruct) displayGreeting(bot *gotgbot.Bot, ctx *ext.Context, config greetingConfig) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()[1:]

	var greetingText string

	if len(args) == 0 || strings.ToLower(args[0]) == "noformat" {
		noformat := len(args) > 0 && strings.ToLower(args[0]) == "noformat"
		greetPrefs := greetings.GetGreetingSettings(chat.Id)

		var buttons []db.Button
		var fileID string
		var greetingDataType int
		var mediaBelow bool
		var shouldGreet bool
		var cleanGreet bool

		if config.gType == greetingWelcome {
			if greetPrefs.WelcomeSettings == nil {
				log.Warnf("[Greetings][%s] WelcomeSettings is nil for chat %d, using defaults", config.logContext, chat.Id)
				tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
				text, _ := tr.GetString(config.notConfiguredKey)
				_, err := msg.Reply(bot, text, formatting.Shtml())
				if err != nil {
					log.Error(err)
					return err
				}
				return ext.EndGroups
			}
			greetingText = greetPrefs.WelcomeSettings.WelcomeText
			buttons = greetings.GetWelcomeButtons(chat.Id)
			fileID = greetPrefs.WelcomeSettings.FileID
			greetingDataType = greetPrefs.WelcomeSettings.WelcomeType
			mediaBelow = greetPrefs.WelcomeSettings.MediaBelow
			shouldGreet = greetPrefs.WelcomeSettings.ShouldWelcome
			cleanGreet = greetPrefs.WelcomeSettings.CleanWelcome
		} else {
			if greetPrefs.GoodbyeSettings == nil {
				log.Warnf("[Greetings][%s] GoodbyeSettings is nil for chat %d, using defaults", config.logContext, chat.Id)
				tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
				text, _ := tr.GetString(config.notConfiguredKey)
				_, err := msg.Reply(bot, text, formatting.Shtml())
				if err != nil {
					log.Error(err)
					return err
				}
				return ext.EndGroups
			}
			greetingText = greetPrefs.GoodbyeSettings.GoodbyeText
			buttons = greetings.GetGoodbyeButtons(chat.Id)
			fileID = greetPrefs.GoodbyeSettings.FileID
			greetingDataType = greetPrefs.GoodbyeSettings.GoodbyeType
			mediaBelow = greetPrefs.GoodbyeSettings.MediaBelow
			shouldGreet = greetPrefs.GoodbyeSettings.ShouldGoodbye
			cleanGreet = greetPrefs.GoodbyeSettings.CleanGoodbye
		}

		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString(config.statusKey)
		_, err := msg.Reply(bot, fmt.Sprintf(text,
			shouldGreet,
			cleanGreet,
			greetPrefs.ShouldCleanService), formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}

		if noformat {
			greetingText += content.RevertButtons(buttons)
			_, err := media.SendGreetingMessage(bot, ctx.EffectiveChat.Id, media.GreetingMessage{
				Text:         greetingText,
				FileID:       fileID,
				MsgType:      greetingDataType,
				CaptionAbove: mediaBelow,
				Keyboard:     &gotgbot.InlineKeyboardMarkup{InlineKeyboard: nil},
				ThreadID:     ctx.EffectiveMessage.MessageThreadId,
			})
			if err != nil {
				log.Error(err)
				return err
			}
		} else {
			greetingText, kb := renderGreeting(bot, chat, user, greetingText, buttons)
			_, err := media.SendGreetingMessage(bot, ctx.EffectiveChat.Id, media.GreetingMessage{
				Text:         greetingText,
				FileID:       fileID,
				MsgType:      greetingDataType,
				CaptionAbove: mediaBelow,
				Keyboard:     kb,
				ThreadID:     ctx.EffectiveMessage.MessageThreadId,
			})
			if err != nil {
				log.Error(err)
				return err
			}
		}

	} else if len(args) >= 1 {
		if !chat_status.RequireUserAdmin(bot, ctx, nil, user.Id) {
			chat_status.NewPermissionResponder(bot).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
			return ext.EndGroups
		}
		var err error
		switch strings.ToLower(args[0]) {
		case "on", "yes", "off", "no":
			on := strings.ToLower(args[0]) == "on" || strings.ToLower(args[0]) == "yes"
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			applyCtx, cancel := greetingApplyContext()
			var res greetingApplyResult
			if config.gType == greetingWelcome {
				res = applyWelcomeEnabled(applyCtx, chat.Id, on)
			} else {
				res = applyGoodbyeEnabled(applyCtx, chat.Id, on)
			}
			cancel()
			if res.Outcome == greetingSaveFailed {
				errText, _ := tr.GetString("common_settings_save_failed")
				_, _ = msg.Reply(bot, errText, formatting.Shtml())
				return ext.EndGroups
			}
			key := config.disabledKey
			if on {
				key = config.enabledKey
			}
			text, _ := tr.GetString(key)
			_, err = msg.Reply(bot, text, formatting.Shtml())
		default:
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			text, _ := tr.GetString(config.invalidKey)
			_, err = msg.Reply(bot, text, formatting.Shtml())
		}

		if err != nil {
			log.Error(err)
			return err
		}
	}
	return ext.EndGroups
}

//nolint:dupl // welcome delegates to displayGreeting with different config
func (m moduleStruct) welcome(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.displayGreeting(bot, ctx, welcomeConfig)
}

//nolint:dupl // setWelcome is similar to setGoodbye but uses different DB calls and translation keys
func (moduleStruct) setWelcome(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}

	if !chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(bot).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	result := content.ExtractWelcome(msg, "welcome", lang.GetLanguage(ctx))
	text, dataType, content, buttons, errorMsg := result.Text, result.DataType, result.FileID, result.Buttons, result.ErrorMsg
	if dataType == -1 {
		_, err := msg.Reply(bot, errorMsg, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if dbErr := greetings.SetWelcomeText(chat.Id, text, content, buttons, dataType); dbErr != nil {
		log.Errorf("[Greetings] SetWelcomeText failed for chat %d: %v", chat.Id, dbErr)
		errText, _ := tr.GetString("common_settings_save_failed")
		_, _ = msg.Reply(bot, errText, formatting.Shtml())
		return ext.EndGroups
	}
	successText, _ := tr.GetString("greetings_welcome_set_success")
	_, err := msg.Reply(bot, successText, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

//nolint:dupl // resetGreeting has symmetric welcome/goodbye logic by design
func (moduleStruct) resetGreeting(bot *gotgbot.Bot, ctx *ext.Context, isWelcome bool) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}
	if !chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(bot).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	applyCtx, cancel := greetingApplyContext()
	res := applyGreetingReset(applyCtx, chat.Id, isWelcome)
	cancel()
	if res.Outcome == greetingSaveFailed {
		errText, _ := tr.GetString("common_settings_save_failed")
		_, _ = msg.Reply(bot, errText, formatting.Shtml())
		return ext.EndGroups
	}
	translationKey := "greetings_welcome_reset_success"
	if !isWelcome {
		translationKey = "greetings_goodbye_reset"
	}
	successText, _ := tr.GetString(translationKey)
	_, err := msg.Reply(bot, successText, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (m moduleStruct) resetWelcome(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.resetGreeting(bot, ctx, true)
}

//nolint:dupl // goodbye delegates to displayGreeting with different config
func (m moduleStruct) goodbye(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.displayGreeting(bot, ctx, goodbyeConfig)
}

//nolint:dupl // setGoodbye is similar to setWelcome but uses different DB calls and translation keys
func (moduleStruct) setGoodbye(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}
	if !chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(bot).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	result := content.ExtractWelcome(msg, "goodbye", lang.GetLanguage(ctx))
	text, dataType, content, buttons, errorMsg := result.Text, result.DataType, result.FileID, result.Buttons, result.ErrorMsg
	if dataType == -1 {
		_, err := msg.Reply(bot, errorMsg, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if dbErr := greetings.SetGoodbyeText(chat.Id, text, content, buttons, dataType); dbErr != nil {
		log.Errorf("[Greetings] SetGoodbyeText failed for chat %d: %v", chat.Id, dbErr)
		errText, _ := tr.GetString("common_settings_save_failed")
		_, _ = msg.Reply(bot, errText, formatting.Shtml())
		return ext.EndGroups
	}
	successText, _ := tr.GetString("greetings_goodbye_set_success")
	_, err := msg.Reply(bot, successText, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (m moduleStruct) resetGoodbye(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.resetGreeting(bot, ctx, false)
}

type greetingToggleConfig struct {
	getPref              func(chatID int64) bool
	setPref              func(chatID int64, val bool) error
	saveErrLog           string
	connectBotAdmin      bool
	showEnabled          string
	showDisabled         string
	setEnabled           string
	setDisabled          string
	invalid              string
	useMarkdownOnEnabled bool
}

var cleanWelcomeToggleConfig = greetingToggleConfig{
	getPref: func(chatID int64) bool {
		greetSettings := greetings.GetGreetingSettings(chatID)
		if greetSettings.WelcomeSettings == nil {
			log.Warnf("[Greetings][cleanWelcome] WelcomeSettings is nil for chat %d, using default (false)", chatID)
			return false
		}
		return greetSettings.WelcomeSettings.CleanWelcome
	},
	setPref: func(chatID int64, val bool) error {
		ctx, cancel := greetingApplyContext()
		defer cancel()
		return applyCleanWelcome(ctx, chatID, val).Err
	},
	saveErrLog:           "SetCleanWelcomeSetting",
	connectBotAdmin:      false,
	showEnabled:          "greetings_clean_welcome_not",
	showDisabled:         "greetings_clean_welcome_should",
	setEnabled:           "greetings_clean_welcome_enable",
	setDisabled:          "greetings_clean_welcome_disable",
	invalid:              "greetings_clean_welcome_invalid_option",
	useMarkdownOnEnabled: false,
}

var cleanGoodbyeToggleConfig = greetingToggleConfig{
	getPref: func(chatID int64) bool {
		greetSettings := greetings.GetGreetingSettings(chatID)
		if greetSettings.GoodbyeSettings == nil {
			log.Warnf("[Greetings][cleanGoodbye] GoodbyeSettings is nil for chat %d, using default (false)", chatID)
			return false
		}
		return greetSettings.GoodbyeSettings.CleanGoodbye
	},
	setPref: func(chatID int64, val bool) error {
		ctx, cancel := greetingApplyContext()
		defer cancel()
		return applyCleanGoodbye(ctx, chatID, val).Err
	},
	saveErrLog:           "SetCleanGoodbyeSetting",
	connectBotAdmin:      false,
	showEnabled:          "greetings_clean_goodbye_not",
	showDisabled:         "greetings_clean_goodbye_should",
	setEnabled:           "greetings_clean_goodbye_enable",
	setDisabled:          "greetings_clean_goodbye_disable",
	invalid:              "greetings_clean_goodbye_invalid_option",
	useMarkdownOnEnabled: false,
}

var delJoinedToggleConfig = greetingToggleConfig{
	getPref: func(chatID int64) bool {
		return greetings.GetGreetingSettings(chatID).ShouldCleanService
	},
	setPref: func(chatID int64, val bool) error {
		ctx, cancel := greetingApplyContext()
		defer cancel()
		return applyCleanService(ctx, chatID, val).Err
	},
	saveErrLog:           "ApplyCleanServiceFlag",
	connectBotAdmin:      true,
	showEnabled:          "greetings_clean_service_should",
	showDisabled:         "greetings_clean_service_not",
	setEnabled:           "greetings_clean_service_enable",
	setDisabled:          "greetings_clean_service_disable",
	invalid:              "greetings_clean_service_invalid_option",
	useMarkdownOnEnabled: true,
}

var autoApproveToggleConfig = greetingToggleConfig{
	getPref: func(chatID int64) bool {
		return greetings.GetGreetingSettings(chatID).ShouldAutoApprove
	},
	setPref: func(chatID int64, val bool) error {
		ctx, cancel := greetingApplyContext()
		defer cancel()
		return applyAutoApprove(ctx, chatID, val).Err
	},
	saveErrLog:           "SetShouldAutoApprove",
	connectBotAdmin:      true,
	showEnabled:          "greetings_auto_approve_enabled",
	showDisabled:         "greetings_auto_approve_disabled",
	setEnabled:           "greetings_auto_approve_enable",
	setDisabled:          "greetings_auto_approve_disable",
	invalid:              "greetings_auto_approve_invalid_option",
	useMarkdownOnEnabled: true,
}

func (moduleStruct) greetingToggle(bot *gotgbot.Bot, ctx *ext.Context, cfg greetingToggleConfig) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(bot, ctx, true, cfg.connectBotAdmin)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	args := ctx.Args()[1:]
	var err error
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}
	if !chat_status.CanUserChangeInfo(bot, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(bot).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	if len(args) == 0 {
		if cfg.getPref(chat.Id) {
			text, _ := tr.GetString(cfg.showEnabled)
			if cfg.useMarkdownOnEnabled {
				_, err = msg.Reply(bot, text, formatting.Smarkdown())
			} else {
				_, err = msg.Reply(bot, text, formatting.Shtml())
			}
		} else {
			text, _ := tr.GetString(cfg.showDisabled)
			_, err = msg.Reply(bot, text, formatting.Shtml())
		}
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	switch strings.ToLower(args[0]) {
	case "off", "no":
		if dbErr := cfg.setPref(chat.Id, false); dbErr != nil {
			log.Errorf("[Greetings] %s failed for chat %d: %v", cfg.saveErrLog, chat.Id, dbErr)
			errText, _ := tr.GetString("common_settings_save_failed")
			_, _ = msg.Reply(bot, errText, formatting.Shtml())
			return ext.EndGroups
		}
		text, _ := tr.GetString(cfg.setDisabled)
		_, err = msg.Reply(bot, text, formatting.Shtml())
	case "on", "yes":
		if dbErr := cfg.setPref(chat.Id, true); dbErr != nil {
			log.Errorf("[Greetings] %s failed for chat %d: %v", cfg.saveErrLog, chat.Id, dbErr)
			errText, _ := tr.GetString("common_settings_save_failed")
			_, _ = msg.Reply(bot, errText, formatting.Shtml())
			return ext.EndGroups
		}
		text, _ := tr.GetString(cfg.setEnabled)
		_, err = msg.Reply(bot, text, formatting.Shtml())
	default:
		text, _ := tr.GetString(cfg.invalid)
		_, err = msg.Reply(bot, text, formatting.Shtml())
	}

	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (m moduleStruct) cleanWelcome(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.greetingToggle(bot, ctx, cleanWelcomeToggleConfig)
}

func (m moduleStruct) cleanGoodbye(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.greetingToggle(bot, ctx, cleanGoodbyeToggleConfig)
}

func (m moduleStruct) delJoined(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.greetingToggle(bot, ctx, delJoinedToggleConfig)
}

// SendWelcomeMessage sends the configured welcome message for a user in a chat.
func SendWelcomeMessage(bot *gotgbot.Bot, ctx *ext.Context, userID int64, firstName string) error {
	var (
		claimed       bool
		claimedChatID int64
		sendAttempted bool
	)
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[Greetings][SendWelcomeMessage] Recovered from panic: %v", r)
			// A panic before the send proves no welcome went out, so the claim is given back and the member's
			// next join is welcomed. After the send was attempted the outcome is unknown and the claim stays.
			if claimed && !sendAttempted {
				releaseUnsentWelcome(claimedChatID, userID)
			}
		}
	}()
	chat := ctx.EffectiveChat
	greetPrefs, err := greetings.LoadGreetingSettings(chat.Id)
	if err != nil {
		// The read says whether Welcome and Greet once are on, so a guess could welcome a member twice: send
		// nothing and claim nothing. A double welcome is worse than a missing one.
		log.Errorf("[Greetings][SendWelcomeMessage] read greeting settings of chat %d: %v", chat.Id, err)
		return nil
	}

	if greetPrefs.WelcomeSettings == nil {
		log.Warnf("[Greetings][SendWelcomeMessage] WelcomeSettings is nil for chat %d, skipping welcome message", chat.Id)
		return nil
	}

	if greetPrefs.WelcomeSettings.ShouldWelcome {
		send, won := claimWelcome(chat.Id, userID, greetPrefs.WelcomeSettings.Once)
		if !send {
			return nil
		}
		claimed, claimedChatID = won, chat.Id
		user := &gotgbot.User{
			Id:        userID,
			FirstName: firstName,
			IsBot:     false,
		}

		buttons := greetings.GetWelcomeButtons(chat.Id)
		res, kb := welcomeRender(bot, chat, user, greetPrefs.WelcomeSettings.WelcomeText, buttons)

		var threadID int64
		if ctx.EffectiveMessage != nil {
			threadID = ctx.EffectiveMessage.MessageThreadId
		}
		sendAttempted = true
		sent, err := media.SendGreetingMessage(bot, chat.Id, media.GreetingMessage{
			Text:         res,
			FileID:       greetPrefs.WelcomeSettings.FileID,
			MsgType:      greetPrefs.WelcomeSettings.WelcomeType,
			CaptionAbove: greetPrefs.WelcomeSettings.MediaBelow,
			Keyboard:     kb,
			ThreadID:     threadID,
		})
		if err != nil {
			logJoinOperationError("[Greetings][SendWelcomeMessage] Failed to send welcome message", err)
			if claimed {
				giveBackWelcome(chat.Id, userID, err)
			}
			return err
		}
		scheduleBotMessage(sent, deletion.KindBotWelcome)
		if greetPrefs.WelcomeSettings.CleanWelcome {
			_ = helpers.DeleteMessageWithErrorHandling(bot, chat.Id, greetPrefs.WelcomeSettings.LastMsgId)
			if err := greetings.SetCleanWelcomeMsgId(chat.Id, sent.MessageId); err != nil {
				log.Warnf("[Greetings] Failed to store clean welcome msg ID for chat %d: %v", chat.Id, err)
			}
		}
	}
	return nil
}

func (moduleStruct) newMember(bot *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	newMember := ctx.ChatMember.NewChatMember.MergeChatMember().User
	var threadID int64
	if ctx.EffectiveMessage != nil {
		threadID = ctx.EffectiveMessage.MessageThreadId
	}
	chatCopy := *chat
	captchaSettings, err := captcha.GetCaptchaSettingsContext(tracing.UpdateContext(ctx), chat.Id)
	if err != nil {
		log.Errorf("[Greetings][newMember] Failed to get captcha settings for chat %d: %v", chat.Id, err)
		captchaSettings = &db.CaptchaSettings{Enabled: false}
	}
	if err := processSingleNewMember(bot, &chatCopy, threadID, newMember, captchaSettings != nil && captchaSettings.Enabled, memberReqJoinDate(ctx.ChatMember.Date)); err != nil {
		return err
	}
	return ext.EndGroups
}

func (moduleStruct) leftMember(bot *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	leftMember := ctx.ChatMember.OldChatMember.MergeChatMember().User
	greetPrefs := greetings.GetGreetingSettings(chat.Id)

	if leftMember.Id == bot.Id {
		return ext.EndGroups
	}

	clearRecentJoinProcessing(chat.Id, leftMember.Id)

	// A detected leave (a kick, a ban, a member leaving) ends the member's requirement episode,
	// so a kicked member who rejoins is checked again (D-07, D-14). A failure is logged by IDs and
	// never stops the cleanup or the goodbye below. A requirement-muted member's leave is not a
	// detected leave (restricted to restricted); the rejoin reset settles that one.
	clearCtx, clearCancel := context.WithTimeout(tracing.UpdateContext(ctx), memberReqRecordTimeout)
	if _, clearErr := memberreq.ClearHolds(clearCtx, chat.Id, leftMember.Id); clearErr != nil {
		log.Errorf("[MemberReq] clearing the holds of user %d in chat %d on their leave failed: %s", leftMember.Id, chat.Id, memberReqErrText(clearErr))
	}
	clearCancel()

	captchaAttempt, err := captcha.GetCaptchaAttemptIncludingExpired(leftMember.Id, chat.Id)
	if err != nil {
		log.Errorf("Failed to get captcha attempt for leaving user %d: %v", leftMember.Id, err)
	} else if captchaAttempt != nil {
		if captchaAttempt.MessageID > 0 {
			if delErr := helpers.DeleteMessageWithErrorHandling(bot, chat.Id, captchaAttempt.MessageID); delErr != nil {
				log.Debugf("Failed to delete captcha message for leaving user %d: %v", leftMember.Id, delErr)
			}
		}
		if _, delErr := captcha.DeleteCaptchaAttemptByIDAtomic(captchaAttempt.ID, leftMember.Id, chat.Id); delErr != nil {
			log.Errorf("Failed to delete captcha attempt for leaving user %d: %v", leftMember.Id, delErr)
		}
	}
	if err := captcha.DeleteMutedUser(leftMember.Id, chat.Id); err != nil {
		log.Errorf("Failed to delete scheduled captcha unmute for leaving user %d: %v", leftMember.Id, err)
	}

	if greetPrefs.GoodbyeSettings == nil {
		log.Warnf("[Greetings][leftMember] GoodbyeSettings is nil for chat %d, skipping goodbye message", chat.Id)
		return ext.EndGroups
	}

	if greetPrefs.GoodbyeSettings.ShouldGoodbye {
		buttons := greetings.GetGoodbyeButtons(chat.Id)
		res, kb := renderGreeting(bot, chat, &leftMember, greetPrefs.GoodbyeSettings.GoodbyeText, buttons)
		var threadID int64
		if ctx.EffectiveMessage != nil {
			threadID = ctx.EffectiveMessage.MessageThreadId
		}
		sent, err := media.SendGreetingMessage(bot, chat.Id, media.GreetingMessage{
			Text:         res,
			FileID:       greetPrefs.GoodbyeSettings.FileID,
			MsgType:      greetPrefs.GoodbyeSettings.GoodbyeType,
			CaptionAbove: greetPrefs.GoodbyeSettings.MediaBelow,
			Keyboard:     kb,
			ThreadID:     threadID,
		})
		if err != nil {
			logJoinOperationError("[Greetings][leftMember] Failed to send goodbye message", err)
			return err
		}
		scheduleBotMessage(sent, deletion.KindBotGoodbye)

		if greetPrefs.GoodbyeSettings.CleanGoodbye {
			_ = helpers.DeleteMessageWithErrorHandling(bot, chat.Id, greetPrefs.GoodbyeSettings.LastMsgId)
			if err := greetings.SetCleanGoodbyeMsgId(chat.Id, sent.MessageId); err != nil {
				log.Warnf("[Greetings] Failed to store clean goodbye msg ID for chat %d: %v", chat.Id, err)
			}
		}
	}
	return ext.EndGroups
}

func membershipDepsForJoin(bot *gotgbot.Bot, chat *gotgbot.Chat, threadID int64, joinDate time.Time) MembershipDeps {
	if joinDate.IsZero() {
		joinDate = time.Now()
	}
	return MembershipDeps{
		Claim: claimRecentJoinProcessing,
		IsApproved: func(chatID, userID int64) bool {
			return chat_status.IsApproved(bot, chatID, userID)
		},
		Challenge: func(user gotgbot.User) error {
			ctxCopy := ext.Context{EffectiveChat: chat}
			if threadID != 0 {
				ctxCopy.EffectiveMessage = &gotgbot.Message{Chat: *chat, MessageThreadId: threadID}
			}
			if err := SendCaptcha(bot, &ctxCopy, user.Id, user.FirstName); err != nil {
				if errors.Is(err, errCaptchaDisabled) {
					return ErrChallengeDisabled
				}
				return err
			}
			return nil
		},
		Welcome: func(user gotgbot.User) error {
			ctxCopy := ext.Context{EffectiveChat: chat}
			if threadID != 0 {
				ctxCopy.EffectiveMessage = &gotgbot.Message{Chat: *chat, MessageThreadId: threadID}
			}
			return memberReqJoinThenWelcome(bot, &ctxCopy, user)
		},
		Reset: func(user gotgbot.User) {
			memberReqJoinReset(bot, chat, user, joinDate)
		},
	}
}

func processSingleNewMember(bot *gotgbot.Bot, chat *gotgbot.Chat, threadID int64, newMember gotgbot.User, captchaEnabled bool, joinDate time.Time) error {
	outcome, err := ProcessSingleJoin(chat.Id, bot.Id, newMember, captchaEnabled, membershipDepsForJoin(bot, chat, threadID, joinDate))
	if err != nil {
		logJoinOperationError("Failed to process new member join", err)
		return err
	}
	if outcome == JoinIgnore && newMember.Id != bot.Id {
		log.Debugf("[Greetings][cleanService] Skipping duplicate join processing for user %d in chat %d", newMember.Id, chat.Id)
	}
	return nil
}

func (moduleStruct) cleanService(bot *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(bot, ctx)
	if user == nil {
		return ext.EndGroups
	}

	if user.Id == bot.Id {
		return ext.EndGroups
	}

	if msg.NewChatMembers != nil {
		captchaSettings, err := captcha.GetCaptchaSettingsContext(tracing.UpdateContext(ctx), chat.Id)
		if err != nil {
			log.Errorf("[Greetings][cleanService] Failed to get captcha settings for chat %d: %v", chat.Id, err)
			captchaSettings = &db.CaptchaSettings{Enabled: false}
		}
		captchaEnabled := captchaSettings != nil && captchaSettings.Enabled

		var threadID int64
		if msg != nil {
			threadID = msg.MessageThreadId
		}
		chatCopyBase := *chat

		if len(msg.NewChatMembers) > 0 {
			deps := membershipDepsForJoin(bot, &chatCopyBase, threadID, memberReqJoinDate(msg.Date))
			for _, outcome := range ProcessJoins(chat.Id, bot.Id, msg.NewChatMembers, captchaEnabled, deps) {
				if outcome == JoinIgnore {
					continue
				}
			}
		}
	}

	// The service message is deleted only now, after the join was processed, by the group's
	// effective rule (the shared /cleanservice flag and the stored delay). The delete never
	// fails the handler: an inline goroutine or a queue row carries it out.
	if row, ok := deletionServiceKindOf(msg); ok && deletion.SharesCleanServiceFlag(row.Kind) {
		deletionApplyMessage(tracing.UpdateContext(ctx), bot, msg, row)
	}
	return ext.EndGroups
}

func (m moduleStruct) pendingJoins(bot *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("Greetings", "pendingJoins")

	chat := ctx.ChatJoinRequest.Chat
	user := ctx.ChatJoinRequest.From
	joinReqStr := "join_request"

	if !m.loadPendingJoins(chat.Id, user.Id) {

		if greetings.GetGreetingSettings(chat.Id).ShouldAutoApprove {
			if _, err := bot.ApproveChatJoinRequest(chat.Id, user.Id, nil); err != nil {
				if isJoinRequestGone(err) || helpers.IsExpectedTelegramError(err) {
					log.Debugf("[Greetings] Expected error auto-approving join for user %d in chat %d: %v", user.Id, chat.Id, err)
				} else {
					log.Error(err)
					return err
				}
			}
			return ext.ContinueGroups
		}

		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		newUserText, _ := tr.GetString("greetings_join_request_new")
		approveText, _ := tr.GetString("greetings_join_request_approve_btn")
		declineText, _ := tr.GetString("greetings_join_request_decline_btn")
		banText, _ := tr.GetString("greetings_join_request_ban_btn")
		userInfoTemplate, _ := tr.GetString("format_user_info")
		userIdTemplate, _ := tr.GetString("format_user_id")

		_, err := helpers.SendMessageWithErrorHandling(
			bot,
			chat.Id,
			fmt.Sprint(
				newUserText,
				"\n"+fmt.Sprintf(userInfoTemplate, formatting.MentionHtml(user.Id, user.FirstName)),
				"\n"+fmt.Sprintf(userIdTemplate, user.Id),
			),
			&gotgbot.SendMessageOpts{
				ParseMode: formatting.HTML,
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
						{
							{
								Text:         approveText,
								CallbackData: encodeCallbackData(joinReqStr, map[string]string{"a": "accept", "u": fmt.Sprint(user.Id)}),
							},
							{
								Text:         declineText,
								CallbackData: encodeCallbackData(joinReqStr, map[string]string{"a": "decline", "u": fmt.Sprint(user.Id)}),
							},
						},
						{
							{
								Text:         banText,
								CallbackData: encodeCallbackData(joinReqStr, map[string]string{"a": "ban", "u": fmt.Sprint(user.Id)}),
							},
						},
					},
				},
			},
		)
		if err != nil {
			log.Error(err)
			return err
		}
		m.setPendingJoins(chat.Id, user.Id)
	}

	return ext.ContinueGroups
}

//nolint:gocyclo // Validation and one-shot action handling stay together to prevent partial flows.
func (m moduleStruct) joinRequestHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("Greetings", "joinRequestHandler")

	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	if query.Message == nil {
		return ext.EndGroups
	}
	user := query.From
	chat := ctx.EffectiveChat
	msg := query.Message

	if !chat_status.RequireUserAdmin(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}

	response := ""
	joinUserIDRaw := ""
	if decoded, ok := decodeCallbackData(query.Data, "join_request"); ok {
		response, _ = decoded.Field("a")
		joinUserIDRaw, _ = decoded.Field("u")
	}
	if response == "" || joinUserIDRaw == "" {
		log.Warnf("[Greetings] Invalid callback data format: %s", query.Data)
		return answerInvalidCallback(b, ctx, query)
	}
	if response != "accept" && response != "decline" && response != "ban" {
		log.Warnf("[Greetings] Invalid join request action: %s", response)
		return answerInvalidCallback(b, ctx, query)
	}
	joinUserId, err := strconv.ParseInt(joinUserIDRaw, 10, 64)
	if err != nil {
		log.Errorf("[Greetings] Failed to parse join user ID '%s': %v", joinUserIDRaw, err)
		return answerInvalidCallback(b, ctx, query)
	}

	if response == "ban" {
		if !chat_status.CanUserRestrict(b, ctx, chat, user.Id) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_restrict_cmd_error", "chat_status_restrict_button_error")
			return ext.EndGroups
		}
		if !chat_status.CanBotRestrict(b, ctx, chat) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_bot_restrict_error", "chat_status_bot_restrict_error")
			return ext.EndGroups
		}
	}
	if response == "accept" || response == "decline" || response == "ban" {
		if !chat_status.CanUserInvite(b, ctx, chat, user.Id) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_invite_link_user_error", "chat_status_invite_link_user_error")
			return ext.EndGroups
		}
		if !chat_status.CanBotInvite(b, ctx, chat) {
			chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_invite_link_bot_error", "chat_status_invite_link_bot_error")
			return ext.EndGroups
		}
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))

	// While a lockdown holds the group the Accept button approves nothing (D-25): the
	// request stays pending until the lift, or an admin approves it in Telegram's own
	// request list. A lockdown that cannot be read counts as active.
	if response == "accept" {
		active, lockErr := lockdown.GetActiveFresh(chat.Id)
		if lockErr != nil {
			log.Errorf("[Greetings] could not read the lockdown of chat %d, refusing to approve: %v", chat.Id, lockErr)
		}
		if lockErr != nil || active != nil {
			refusal, _ := tr.GetString("greetings_join_request_lockdown")
			if _, err = query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: refusal, ShowAlert: true}); err != nil {
				log.Error(err)
				return err
			}
			return ext.EndGroups
		}
	}

	joinUser, err := b.GetChat(joinUserId, nil)
	if err != nil {
		log.Error(err)
		return err
	}
	var helpText string

	// A request another admin already handled, the user withdrew, or that
	// joined some other way is gone; report that instead of the requested action.
	switch response {
	case "accept":
		helpText, _ = tr.GetString("greetings_join_request_accepted")
		if _, err = b.ApproveChatJoinRequest(chat.Id, joinUser.Id, nil); err != nil {
			if isJoinRequestGone(err) {
				helpText, _ = tr.GetString("greetings_join_request_already_handled")
			} else if helpers.IsExpectedTelegramError(err) {
				log.Debugf("[Greetings] Expected error approving join for user %d in chat %d: %v", joinUser.Id, chat.Id, err)
			} else {
				log.Error(err)
				return err
			}
		}
	case "decline":
		helpText, _ = tr.GetString("greetings_join_request_declined")
		if _, err = b.DeclineChatJoinRequest(chat.Id, joinUser.Id, nil); err != nil {
			if isJoinRequestGone(err) {
				helpText, _ = tr.GetString("greetings_join_request_already_handled")
			} else if helpers.IsExpectedTelegramError(err) {
				log.Debugf("[Greetings] Expected error declining join for user %d in chat %d: %v", joinUser.Id, chat.Id, err)
			} else {
				log.Error(err)
				return err
			}
		}
	case "ban":
		if _, err = chat.BanMember(b, joinUser.Id, nil); err != nil {
			if helpers.IsExpectedTelegramError(err) {
				log.Debugf("[Greetings] Expected error banning user %d in chat %d: %v", joinUser.Id, chat.Id, err)
			} else {
				log.Error(err)
				return err
			}
		}
		// The ban stands even when the request itself is already gone.
		if _, err = b.DeclineChatJoinRequest(chat.Id, joinUser.Id, nil); err != nil && !isJoinRequestGone(err) {
			if helpers.IsExpectedTelegramError(err) {
				log.Debugf("[Greetings] Expected error declining join after ban for user %d in chat %d: %v", joinUser.Id, chat.Id, err)
			} else {
				log.Error(err)
				return err
			}
		}
		helpText, _ = tr.GetString("greetings_join_request_banned")
	}
	m.clearPendingJoins(chat.Id, joinUser.Id)

	_, _, err = msg.EditText(b, &gotgbot.EditMessageTextOpts{Text: fmt.Sprintf(helpText, formatting.MentionHtml(joinUser.Id, joinUser.FirstName)), ParseMode: formatting.HTML})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b,
		&gotgbot.AnswerCallbackQueryOpts{
			Text: fmt.Sprintf(helpText, joinUser.FirstName),
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

// isJoinRequestGone reports Telegram rejecting a join-request action because the
// request no longer exists.
func isJoinRequestGone(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "HIDE_REQUESTER_MISSING") || strings.Contains(s, "USER_ALREADY_PARTICIPANT")
}

func (m moduleStruct) autoApprove(bot *gotgbot.Bot, ctx *ext.Context) error {
	return m.greetingToggle(bot, ctx, autoApproveToggleConfig)
}

func pendingJoinsKey(chatId, userId int64) string {
	return fmt.Sprintf("fuku:pendingJoins:%d:%d", chatId, userId)
}

func (moduleStruct) loadPendingJoins(chatId, userId int64) bool {
	m := cache.GetMarshal()
	if m == nil {
		return false
	}
	alreadyAsked, err := m.Get(cache.Context, pendingJoinsKey(chatId, userId), new(bool))
	if err != nil || alreadyAsked == nil {
		return false
	}
	if boolVal, ok := alreadyAsked.(*bool); ok && boolVal != nil {
		return *boolVal
	}
	return false
}

func (moduleStruct) setPendingJoins(chatId, userId int64) {
	m := cache.GetMarshal()
	if m == nil {
		return
	}
	_ = m.Set(cache.Context, pendingJoinsKey(chatId, userId), true, store.WithExpiration(5*time.Minute))
}

func (moduleStruct) clearPendingJoins(chatId, userId int64) {
	if m := cache.GetMarshal(); m != nil {
		_ = m.Delete(cache.Context, pendingJoinsKey(chatId, userId))
	}
}

func LoadGreetings(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(greetingsModule.moduleName, true)

	SetModuleHelp(greetingsModule.moduleName, [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text:         trS(i18n.MustNewTranslator("uk"), "button_formatting"),
				CallbackData: encodeCallbackData("helpq", map[string]string{"m": "Formatting"}),
			},
		},
	})

	dispatcher.AddHandler(
		handlers.NewChatJoinRequest(
			chatjoinrequest.All, greetingsModule.pendingJoins,
		),
	)

	dispatcher.AddHandler(
		handlers.NewChatMember(
			func(u *gotgbot.ChatMemberUpdated) bool {
				wasMember, isMember := chat_status.ExtractJoinLeftStatusChange(u)
				return !wasMember && isMember
			},
			greetingsModule.newMember,
		),
	)

	dispatcher.AddHandler(
		handlers.NewChatMember(
			func(u *gotgbot.ChatMemberUpdated) bool {
				wasMember, isMember := chat_status.ExtractJoinLeftStatusChange(u)
				return wasMember && !isMember
			},
			greetingsModule.leftMember,
		),
	)

	dispatcher.AddHandler(
		handlers.NewMessage(
			func(msg *gotgbot.Message) bool {
				return msg.LeftChatMember != nil || msg.NewChatMembers != nil
			},
			greetingsModule.cleanService,
		),
	)

	for _, cfg := range []struct {
		name    string
		handler func(*gotgbot.Bot, *ext.Context) error
	}{
		{"welcome", greetingsModule.welcome},
		{"setwelcome", greetingsModule.setWelcome},
		{"resetwelcome", greetingsModule.resetWelcome},
		{"goodbye", greetingsModule.goodbye},
		{"setgoodbye", greetingsModule.setGoodbye},
		{"resetgoodbye", greetingsModule.resetGoodbye},
		{"cleanwelcome", greetingsModule.cleanWelcome},
		{"cleangoodbye", greetingsModule.cleanGoodbye},
		{"cleanservice", greetingsModule.delJoined},
		{"autoapprove", greetingsModule.autoApprove},
	} {
		desc := helpers.CommandDescriptor{
			Name:           cfg.name,
			RequiredChecks: []helpers.CheckFunc{helpers.CheckDisabled(cfg.name)},
		}
		helpers.WrapCommand(dispatcher, desc, pipelineHandler(cfg.handler))
	}
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("join_request"), greetingsModule.joinRequestHandler))
}

func init() {
	RegisterLegacyModule("Greetings", 210, LoadGreetings)
}
