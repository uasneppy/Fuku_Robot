package modules

import (
	"fmt"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/eko/gocache/lib/v4/store"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
)

const overwriteCacheTTL = 5 * time.Minute

var overwriteConsumeMu sync.Mutex

// overwriteBase holds common fields for temporary state storage during command flows.
type overwriteBase struct {
	ChatID   int64
	UserID   int64
	ItemName string // filterWord or noteWord
	Text     string
	FileID   string
	Buttons  []db.Button
	DataType int
}

// struct for filters module
type overwriteFilter struct {
	overwriteBase
}

// struct for notes module
type overwriteNote struct {
	overwriteBase
	PvtOnly     bool
	GrpOnly     bool
	AdminOnly   bool
	WebPrev     bool
	IsProtected bool
	NoNotif     bool
}

func overwriteCacheKey(kind, token string) string {
	return fmt.Sprintf("fuku:%s_overwrite:%s", kind, token)
}

func setOverwriteCache(key string, data any) error {
	m := cache.GetMarshal()
	if m == nil {
		return fmt.Errorf("cache not initialized")
	}
	return m.Set(cache.Context, key, data, store.WithExpiration(overwriteCacheTTL))
}

func getOverwriteCache[T any](key string) (*T, error) {
	m := cache.GetMarshal()
	if m == nil {
		return nil, fmt.Errorf("cache not initialized")
	}
	var data T
	if _, err := m.Get(cache.Context, key, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func consumeOverwriteCache[T any](key string) (*T, error) {
	if rdb := cache.GetRedisClient(); rdb != nil {
		raw, err := rdb.GetDel(cache.Context, key).Bytes()
		if err != nil {
			return nil, err
		}
		var data T
		if err := msgpack.Unmarshal(raw, &data); err != nil {
			return nil, err
		}
		return &data, nil
	}

	// Test/fallback stores lack GETDEL; serialize the read and delete locally.
	overwriteConsumeMu.Lock()
	defer overwriteConsumeMu.Unlock()
	data, err := getOverwriteCache[T](key)
	if err != nil {
		return nil, err
	}
	deleteOverwriteCache(key)
	return data, nil
}

func deleteOverwriteCache(key string) {
	if m := cache.GetMarshal(); m != nil {
		_ = m.Delete(cache.Context, key)
	}
}

// overwritePress is the outcome of the authority step of an overwrite card press.
type overwritePress int

const (
	// overwritePressAllowed: the press may go on, acting on the target chat.
	overwritePressAllowed overwritePress = iota
	// overwritePressNotAdmin: the presser is not an administrator of the target chat; the caller
	// answers through the PermissionResponder.
	overwritePressNotAdmin
	// overwritePressStale: a press in a private chat on a card that is missing, is not the
	// presser's, or names no group; the caller answers with its expired or cancelled text.
	overwritePressStale
)

// overwritePressTarget decides, before any token is consumed, which chat an overwrite card press
// acts on and whether the presser may act there. In a group the target is the chat the card sits
// in and the presser must be an administrator there (the caller keeps its own check that a card
// naming a chat names this one). In a private chat (a card made through a PM connection) the target
// is the card's own group, read without consuming the card: the presser must be the member who made
// it (a non-zero UserID) and a live administrator of that group, and the card must name a group. The
// target never comes from callback data. A refusal consumes nothing, so the holder can still press.
func overwritePressTarget(
	b *gotgbot.Bot,
	ctx *ext.Context,
	chat *gotgbot.Chat,
	presser int64,
	readCard func() (*overwriteBase, error),
) (int64, overwritePress) {
	if chat == nil {
		return 0, overwritePressNotAdmin
	}
	if chat.Type != "private" {
		if !chat_status.RequireUserAdmin(b, ctx, chat, presser) {
			return 0, overwritePressNotAdmin
		}
		return chat.Id, overwritePressAllowed
	}
	card, err := readCard()
	if err != nil || card == nil || card.UserID == 0 || card.UserID != presser || card.ChatID >= 0 {
		return 0, overwritePressStale
	}
	if !chat_status.IsUserAdmin(b, card.ChatID, presser) {
		return 0, overwritePressNotAdmin
	}
	return card.ChatID, overwritePressAllowed
}
