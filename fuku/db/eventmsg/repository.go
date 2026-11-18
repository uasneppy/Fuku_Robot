package eventmsg

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm/clause"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/cache"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	fukuerrors "github.com/uasneppy/fuku_robot/fuku/utils/errors"
)

// cacheModule is the module name of the cache key. The key is listed in skipLocal
// (fuku/db/cache/local.go): it is read at every manual punishment reply, and a Remove or a
// Silent switch must act at the very next command on every replica.
const cacheModule = "event_messages"

var errNoDatabase = errors.New("database not initialized")

// Message is one stored event message of a chat. A Message whose parts are all empty and whose
// Silent switch is off reads exactly as no row: the default text is sent.
type Message struct {
	Event     string
	Dest      string
	Text      string
	FileID    string
	MediaType int
	Buttons   []models.Button
	Silent    bool
}

// HasContent reports whether the message has a text, a media item or at least one button.
func (m Message) HasContent() bool {
	return m.Text != "" || m.FileID != "" || len(m.Buttons) > 0
}

// Messages is every stored event message of one chat, keyed by rowKey(event, dest). A chat with
// no row has an empty, non-nil map.
type Messages struct {
	ChatID int64
	Rows   map[string]Message
}

// Lookup returns the stored message of one event and destination and whether a row exists. It
// is safe on a nil *Messages.
func (m *Messages) Lookup(event, dest string) (Message, bool) {
	if m == nil || m.Rows == nil {
		return Message{}, false
	}
	msg, ok := m.Rows[rowKey(event, dest)]
	return msg, ok
}

// rowKey is the map key of one event and destination.
func rowKey(event, dest string) string {
	return event + "|" + dest
}

// CacheKey is the cache key of a chat's event messages.
func CacheKey(chatID int64) string {
	return cache.CacheKey(cacheModule, chatID)
}

// Get returns every stored event message of a chat through the cache. A chat with no row has an
// empty set and nothing is created. A failed read is returned wrapped, never replaced by an
// empty set, so a caller can tell "unreadable" from "not configured".
func Get(ctx context.Context, chatID int64) (*Messages, error) {
	msgs, err := cache.GetFromCacheOrLoad(ctx, CacheKey(chatID), cache.DefaultCacheTTL,
		func(ctx context.Context) (*Messages, error) {
			return GetFresh(ctx, chatID)
		})
	if err != nil {
		return nil, err
	}
	if msgs == nil {
		msgs = &Messages{ChatID: chatID}
	}
	if msgs.Rows == nil {
		msgs.Rows = map[string]Message{}
	}
	return msgs, nil
}

// GetFresh is Get without the cache, for the settings page and any caller that must see a write
// another replica just made. It creates nothing.
func GetFresh(ctx context.Context, chatID int64) (*Messages, error) {
	if db.DB == nil {
		return nil, fukuerrors.Wrap(errNoDatabase, "read event messages")
	}
	var rows []models.ChatEventMessage
	if err := db.DB.WithContext(ctx).Where("chat_id = ?", chatID).Find(&rows).Error; err != nil {
		return nil, fukuerrors.Wrapf(err, "read event messages of chat %d", chatID)
	}
	msgs := &Messages{ChatID: chatID, Rows: make(map[string]Message, len(rows))}
	for _, row := range rows {
		msgs.Rows[rowKey(row.Event, row.Dest)] = Message{
			Event:     row.Event,
			Dest:      row.Dest,
			Text:      row.Text,
			FileID:    row.FileID,
			MediaType: row.MediaType,
			Buttons:   []models.Button(row.Buttons),
			Silent:    row.Silent,
		}
	}
	return msgs, nil
}

// SetText stores the text of one event and destination. An empty text is written (it removes
// the text and keeps the other parts). An unknown event or destination is refused before any
// write, and the cached read is dropped after it.
func SetText(ctx context.Context, chatID int64, event, dest, text string, by int64) error {
	row := models.ChatEventMessage{Text: text}
	if err := upsert(ctx, chatID, event, dest, by, &row, "write the event message text", "text"); err != nil {
		return err
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}

// SetMedia stores the media item of one event and destination: the file ID and its media type
// together (an empty file ID and 0 remove it). An unknown event or destination is refused before
// any write, and the cached read is dropped after it.
func SetMedia(ctx context.Context, chatID int64, event, dest, fileID string, mediaType int, by int64) error {
	row := models.ChatEventMessage{FileID: fileID, MediaType: mediaType}
	if err := upsert(ctx, chatID, event, dest, by, &row, "write the event message media", "file_id", "media_type"); err != nil {
		return err
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}

// SetButtons replaces the URL buttons of one event and destination (nil removes them; the column
// then reads []). An unknown event or destination is refused before any write, and the cached
// read is dropped after it.
func SetButtons(ctx context.Context, chatID int64, event, dest string, buttons []models.Button, by int64) error {
	row := models.ChatEventMessage{Buttons: models.ButtonArray(buttons)}
	if err := upsert(ctx, chatID, event, dest, by, &row, "write the event message buttons", "buttons"); err != nil {
		return err
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}

// SetSilent stores the Silent switch of one event and destination; a Go false is written. An
// unknown event or destination is refused before any write, and the cached read is dropped after
// it.
func SetSilent(ctx context.Context, chatID int64, event, dest string, silent bool, by int64) error {
	row := models.ChatEventMessage{Silent: silent}
	if err := upsert(ctx, chatID, event, dest, by, &row, "write the event message silent switch", "silent"); err != nil {
		return err
	}
	cache.DeleteCache(CacheKey(chatID))
	return nil
}

// upsert validates the key, then inserts row (the full struct, so a new row gets zero values for
// the other parts) or, on a conflict of the key, updates only columns plus updated_by and
// updated_at, so two writers of different parts never clobber each other. Its caller drops the
// chat's cache key after a successful write.
func upsert(ctx context.Context, chatID int64, event, dest string, by int64, row *models.ChatEventMessage, what string, columns ...string) error {
	if err := validate(event, dest); err != nil {
		return err
	}
	if db.DB == nil {
		return fukuerrors.Wrap(errNoDatabase, what)
	}
	row.ChatID = chatID
	row.Event = event
	row.Dest = dest
	row.UpdatedBy = by
	row.UpdatedAt = time.Now()
	err := db.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}, {Name: "event"}, {Name: "dest"}},
		DoUpdates: clause.AssignmentColumns(append(columns, "updated_by", "updated_at")),
	}).Create(row).Error
	if err != nil {
		return fukuerrors.Wrapf(err, "%s of chat %d", what, chatID)
	}
	return nil
}
