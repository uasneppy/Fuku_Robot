package cache

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/eko/gocache/lib/v4/store"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/uasneppy/fuku_robot/fuku/utils/constants"
)

var adminCacheGroup singleflight.Group
var adminCacheGeneration atomic.Uint64

// AdminListKind says what a loaded admin list established about a chat.
type AdminListKind int

const (
	// AdminListLoaded means the bot is an administrator and Telegram answered a non-empty
	// administrator list; the event carries every listed member.
	AdminListLoaded AdminListKind = iota + 1
	// AdminListBotNotAdmin means the bot's own status is member, left, kicked or anything but
	// administrator or creator.
	AdminListBotNotAdmin
	// AdminListBotGone means the bot's own lookup was refused with Bot API error code 400 or
	// 403 (IsBotGoneError): the bot cannot see the chat. Any other error reports nothing.
	AdminListBotGone
)

// AdminListEvent is what the observer receives after a load established a definitive fact
// about a chat. Admins is set only for AdminListLoaded and must not be modified.
type AdminListEvent struct {
	ChatID int64
	Kind   AdminListKind
	Admins []gotgbot.MergedChatMember
}

// adminListObserver is the one installed observer, nil when none is. This package cannot
// import the database layer, so the module that stores the roster installs itself here.
var adminListObserver atomic.Pointer[func(AdminListEvent)]

// SetAdminListObserver installs fn as the observer of definitive admin list facts; nil
// removes it. The observer runs synchronously inside the load, under the chat's singleflight,
// so a slow observer delays that one load and nothing else.
func SetAdminListObserver(fn func(AdminListEvent)) {
	if fn == nil {
		adminListObserver.Store(nil)
		return
	}
	adminListObserver.Store(&fn)
}

// notifyAdminList hands ev to the installed observer. A panicking observer is logged and
// never changes the admin cache result.
func notifyAdminList(ev AdminListEvent) {
	fn := adminListObserver.Load()
	if fn == nil || *fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("[AdminCache] observer panic for chat %d: %v", ev.ChatID, r)
		}
	}()
	(*fn)(ev)
}

// IsBotGoneError reports whether err, the error of the bot's own getChatMember, is definitive
// evidence that the bot cannot see the chat: a Telegram answer with Bot API error code 400 or
// 403, also when wrapped. Those are the codes Telegram uses for "chat not found", "bot was
// kicked", "bot is not a member" and "bot was blocked". Every other answer says nothing about
// this chat and must leave a stored roster alone: a 401 (a revoked or rotated token), a 404, a
// 409, a 429 (a rate limit, whose retry_after the caller keeps), a 5xx, a transport error and a
// timeout. It is the one classifier of the roster-changing decision; the settings doors keep
// their own, broader rule (isTelegramClientError in fuku/modules) because they only choose
// what to answer.
func IsBotGoneError(err error) bool {
	var tgErr *gotgbot.TelegramError
	if !errors.As(err, &tgErr) || tgErr == nil {
		return false
	}
	return tgErr.Code == 400 || tgErr.Code == 403
}

func adminCacheKey(chatId int64) string {
	return fmt.Sprintf("%sadminCache:%d", DataCachePrefix, chatId)
}

// getChatAdministrators is called with ReturnBots so other administrator bots
// are included; without it Telegram omits them and IsUserAdmin treats them as
// regular members.
func LoadAdminCache(b *gotgbot.Bot, chatId int64) AdminCache {
	if b == nil {
		log.Error("LoadAdminCache: bot is nil")
		return AdminCache{}
	}

	genBefore := adminCacheGeneration.Load()
	v, _, _ := adminCacheGroup.Do(adminCacheKey(chatId), func() (any, error) {
		return loadAdminCacheFromTelegram(b, chatId), nil
	})
	ac, _ := v.(AdminCache)
	if genBefore != adminCacheGeneration.Load() {
		// An invalidation landed while we waited on the shared load: the value
		// predates a demotion/promote. Re-read once; on miss return empty so
		// callers fall back to per-user GetChatMember instead of trusting stale.
		if ok, fresh := GetAdminCacheList(chatId); ok {
			return fresh
		}
		return AdminCache{}
	}
	return ac
}

func loadAdminCacheFromTelegram(b *gotgbot.Bot, chatId int64) AdminCache {
	generation := adminCacheGeneration.Load()
	const negativeAdminCacheTTL = 2 * time.Minute
	const botStatusErrorAdminCacheTTL = 30 * time.Second

	storeWithTTL := func(adminCache AdminCache, ttl time.Duration) AdminCache {
		if m := GetMarshal(); m != nil && generation == adminCacheGeneration.Load() {
			if err := m.Set(Context, adminCacheKey(chatId), adminCache, store.WithExpiration(ttl)); err != nil {
				log.WithFields(log.Fields{
					"chatId": chatId,
					"error":  err,
				}).Error("LoadAdminCache: Failed to cache admin list")
			} else if generation != adminCacheGeneration.Load() {
				InvalidateAdminCache(chatId)
			}
		}
		return adminCache
	}
	storeResult := func(ac AdminCache) AdminCache {
		return storeWithTTL(ac, constants.AdminCacheTTL)
	}
	storeNegativeResult := func(ac AdminCache) AdminCache {
		return storeWithTTL(ac, negativeAdminCacheTTL)
	}

	botCtx, botCancel := context.WithTimeout(context.Background(), constants.DefaultTimeout)
	botMember, botErr := b.GetChatMemberWithContext(botCtx, chatId, b.Id, nil)
	botCancel()
	if botErr != nil {
		log.WithFields(log.Fields{
			"chatId": chatId,
			"botId":  b.Id,
			"error":  botErr,
		}).Warning("LoadAdminCache: Could not verify bot admin status")
		if IsBotGoneError(botErr) && generation == adminCacheGeneration.Load() {
			notifyAdminList(AdminListEvent{ChatID: chatId, Kind: AdminListBotGone})
		}
		return storeWithTTL(AdminCache{
			ChatId:   chatId,
			UserInfo: []gotgbot.MergedChatMember{},
			Cached:   true,
		}, botStatusErrorAdminCacheTTL)
	}

	botStatus := botMember.GetStatus()
	if botStatus != "administrator" && botStatus != "creator" {
		if generation == adminCacheGeneration.Load() {
			notifyAdminList(AdminListEvent{ChatID: chatId, Kind: AdminListBotNotAdmin})
		}
		return storeNegativeResult(AdminCache{
			ChatId:   chatId,
			UserInfo: []gotgbot.MergedChatMember{},
			Cached:   true,
			Negative: true,
		})
	}

	log.WithFields(log.Fields{
		"chatId":    chatId,
		"botId":     b.Id,
		"botStatus": botStatus,
	}).Debug("LoadAdminCache: Bot has admin privileges")

	maxRetries := 3
	var adminList []gotgbot.ChatMember
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		attemptCtx, attemptCancel := context.WithTimeout(context.Background(), constants.DefaultTimeout)
		adminList, err = b.GetChatAdministratorsWithContext(
			attemptCtx,
			chatId,
			&gotgbot.GetChatAdministratorsOpts{ReturnBots: true},
		)
		attemptCancel()
		if err != nil {
			log.WithFields(log.Fields{
				"chatId":    chatId,
				"error":     err,
				"attempt":   attempt + 1,
				"errorType": fmt.Sprintf("%T", err),
			}).Warning("LoadAdminCache: Failed to get chat administrators, retrying...")

			if attempt < maxRetries-1 {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}

			log.WithFields(log.Fields{
				"chatId": chatId,
				"error":  err,
			}).Error("LoadAdminCache: Failed to get chat administrators after all retries")
			return AdminCache{}
		}
		break
	}

	if len(adminList) == 0 {
		log.WithFields(log.Fields{
			"chatId": chatId,
		}).Warning("LoadAdminCache: No administrators found - this is unusual for a valid group")
		return storeResult(AdminCache{
			ChatId:   chatId,
			UserInfo: []gotgbot.MergedChatMember{},
			Cached:   true,
		})
	}

	userList := make([]gotgbot.MergedChatMember, 0, len(adminList))
	userMap := make(map[int64]gotgbot.MergedChatMember, len(adminList))
	for _, admin := range adminList {
		merged := admin.MergeChatMember()
		userList = append(userList, merged)
		user := admin.GetUser()
		if user.Id != 0 {
			userMap[user.Id] = merged
		}
	}

	adminCache := AdminCache{
		ChatId:   chatId,
		UserInfo: userList,
		UserMap:  userMap,
		Cached:   true,
	}

	result := storeResult(adminCache)
	// Only a fresh, non-empty list whose load no invalidation overtook becomes a fact: the
	// retry-failure and empty-list returns above never reach this point.
	if generation == adminCacheGeneration.Load() {
		notifyAdminList(AdminListEvent{ChatID: chatId, Kind: AdminListLoaded, Admins: userList})
	}
	return result
}

func GetAdminCacheList(chatId int64) (bool, AdminCache) {
	m := GetMarshal()
	if m == nil {
		return false, AdminCache{}
	}
	gotAdminlist, err := m.Get(
		Context,
		adminCacheKey(chatId),
		new(AdminCache),
	)
	if err != nil {
		log.WithFields(log.Fields{
			"chatId": chatId,
			"error":  err,
		}).Debug("GetAdminCacheList: Cache miss, will attempt fallback")
		return false, AdminCache{}
	}
	if gotAdminlist == nil {
		log.WithFields(log.Fields{
			"chatId": chatId,
		}).Debug("GetAdminCacheList: Cache empty, will attempt fallback")
		return false, AdminCache{}
	}
	return true, *gotAdminlist.(*AdminCache)
}

func GetAdminCacheUser(chatId, userId int64) (bool, gotgbot.MergedChatMember) {
	m := GetMarshal()
	if m == nil {
		return false, gotgbot.MergedChatMember{}
	}
	adminList, err := m.Get(Context, adminCacheKey(chatId), new(AdminCache))
	if err != nil || adminList == nil {
		return false, gotgbot.MergedChatMember{}
	}

	adminCache, ok := adminList.(*AdminCache)
	if !ok || adminCache == nil {
		return false, gotgbot.MergedChatMember{}
	}

	if admin, found := adminCache.UserMap[userId]; found {
		return true, admin
	}

	for i := range adminCache.UserInfo {
		admin := &adminCache.UserInfo[i]
		if admin.User.Id == userId {
			return true, *admin
		}
	}
	return false, gotgbot.MergedChatMember{}
}

func InvalidateAdminCache(chatId int64) {
	if err := ClearAdminCache(chatId); err != nil {
		log.Debugf("[AdminCache] Failed to invalidate cache for chat %d: %v", chatId, err)
	}
}

// ClearAdminCache invalidates pending lookups and removes stored permissions.
func ClearAdminCache(chatId int64) error {
	adminCacheGeneration.Add(1)
	adminCacheGroup.Forget(adminCacheKey(chatId))
	if m := GetMarshal(); m != nil {
		return m.Delete(Context, adminCacheKey(chatId))
	}
	return nil
}
