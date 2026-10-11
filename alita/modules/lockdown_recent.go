package modules

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/utils/cache"
)

const (
	// lockdownRecentKeyPrefix starts the key of a group's recent-joins tracker. It sits
	// in the operational alita:lockdown:* family, outside alita:cache:, so
	// CLEAR_CACHE_ON_STARTUP keeps it.
	lockdownRecentKeyPrefix = "alita:lockdown:recent:"
	// lockdownRecentWindow is how far back the tracker remembers a join.
	lockdownRecentWindow = 10 * time.Minute
	// lockdownRecentCap is the most joins the tracker keeps per group (the newest).
	lockdownRecentCap = 500
	// lockdownRecentTTL lets an idle group's tracker expire on its own.
	lockdownRecentTTL = 700 * time.Second
	// lockdownRecentTimeout bounds the one Redis round trip the join hot path makes.
	lockdownRecentTimeout = 250 * time.Millisecond
	// lockdownRecentUsernameMaxRunes caps a stored @username (Telegram's own limit).
	lockdownRecentUsernameMaxRunes = 32
	// lockdownRecentLinkMaxRunes caps a stored invite link.
	lockdownRecentLinkMaxRunes = 128
)

// lockdownRecentJoin is one remembered join. At is the sorted-set score, not part of
// the stored member.
type lockdownRecentJoin struct {
	UserID   int64     `json:"i"`
	Name     string    `json:"n,omitempty"`
	Username string    `json:"u,omitempty"`
	Bot      bool      `json:"b,omitempty"`
	Link     string    `json:"l,omitempty"`
	At       time.Time `json:"-"`
}

// lockdownRecentKey is the Redis key of a chat's recent-joins tracker, a sorted set
// scored by join time in unix milliseconds.
func lockdownRecentKey(chatID int64) string {
	return lockdownRecentKeyPrefix + strconv.FormatInt(chatID, 10)
}

// trackLockdownRecentJoins remembers joins of a supergroup for lockdownRecentWindow.
// It is best effort and never decides or delays anything: one pipelined round trip
// under lockdownRecentTimeout, an error logged at debug and dropped, and nothing at
// all without Redis. Names, usernames and links come from the joining user and are
// capped before they are stored.
func trackLockdownRecentJoins(chatID int64, joins []lockdownRecentJoin) {
	if len(joins) == 0 || !cache.IsRedisAvailable() {
		return
	}
	client := cache.GetRedisClient()
	if client == nil {
		return
	}

	now := time.Now()
	key := lockdownRecentKey(chatID)
	members := make([]redis.Z, 0, len(joins))
	for _, join := range joins {
		join.Name = lockdownCapRunes(join.Name, lockdownNameMaxRunes)
		join.Username = lockdownCapRunes(join.Username, lockdownRecentUsernameMaxRunes)
		join.Link = lockdownCapRunes(join.Link, lockdownRecentLinkMaxRunes)
		raw, err := json.Marshal(join)
		if err != nil {
			continue
		}
		at := join.At
		if at.IsZero() {
			at = now
		}
		members = append(members, redis.Z{Score: float64(at.UnixMilli()), Member: string(raw)})
	}
	if len(members) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(cache.Context, lockdownRecentTimeout)
	defer cancel()
	pipe := client.Pipeline()
	// NX: the same join delivered by both paths keeps the earliest time.
	pipe.ZAddNX(ctx, key, members...)
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now.Add(-lockdownRecentWindow).UnixMilli(), 10))
	pipe.ZRemRangeByRank(ctx, key, 0, -(lockdownRecentCap + 1))
	pipe.Expire(ctx, key, lockdownRecentTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Debugf("[Lockdown] join tracker write for chat %d failed: %v", chatID, err)
	}
}

// lockdownRecentLookup is the snapshot read of the tracker. It is a variable only so a
// test can make the read fail; production code never reassigns it.
var lockdownRecentLookup = readLockdownRecentJoins

// readLockdownRecentJoins returns the joins of a chat after from (exclusive) up to
// to, each user once, oldest first. Without Redis it returns nothing and no error.
// The earliest join time of a user wins, and a non-empty link, name or username from
// either delivery fills in, because only the chat_member path carries the invite link.
func readLockdownRecentJoins(chatID int64, from, to time.Time) ([]lockdownRecentJoin, error) {
	if !cache.IsRedisAvailable() {
		return nil, nil
	}
	client := cache.GetRedisClient()
	if client == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(cache.Context, lockdownRecentTimeout)
	defer cancel()
	entries, err := client.ZRangeByScoreWithScores(ctx, lockdownRecentKey(chatID), &redis.ZRangeBy{
		Min: "(" + strconv.FormatInt(from.UnixMilli(), 10),
		Max: strconv.FormatInt(to.UnixMilli(), 10),
	}).Result()
	if err != nil {
		return nil, err
	}

	byUser := make(map[int64]*lockdownRecentJoin, len(entries))
	for _, entry := range entries {
		raw, ok := entry.Member.(string)
		if !ok {
			continue
		}
		var join lockdownRecentJoin
		if json.Unmarshal([]byte(raw), &join) != nil || join.UserID == 0 {
			continue
		}
		join.At = time.UnixMilli(int64(entry.Score))
		kept, seen := byUser[join.UserID]
		if !seen {
			byUser[join.UserID] = &join
			continue
		}
		if join.At.Before(kept.At) {
			kept.At = join.At
		}
		if kept.Link == "" {
			kept.Link = join.Link
		}
		if kept.Name == "" {
			kept.Name = join.Name
		}
		if kept.Username == "" {
			kept.Username = join.Username
		}
		kept.Bot = kept.Bot || join.Bot
	}

	joins := make([]lockdownRecentJoin, 0, len(byUser))
	for _, join := range byUser {
		joins = append(joins, *join)
	}
	slices.SortFunc(joins, func(a, b lockdownRecentJoin) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		switch {
		case a.UserID < b.UserID:
			return -1
		case a.UserID > b.UserID:
			return 1
		}
		return 0
	})
	return joins, nil
}

// lockdownMemberJoins is the tracker entry for the user of a chat_member join, with
// the invite link Telegram reported when there was one. The bot's own join is not
// tracked, so it returns nothing for it.
func lockdownMemberJoins(b *gotgbot.Bot, update *gotgbot.ChatMemberUpdated) []lockdownRecentJoin {
	user := update.NewChatMember.MergeChatMember().User
	if user.Id == b.Id {
		return nil
	}
	join := lockdownRecentJoin{
		UserID:   user.Id,
		Name:     user.FirstName,
		Username: user.Username,
		Bot:      user.IsBot,
		At:       time.Now(),
	}
	if update.InviteLink != nil {
		join.Link = update.InviteLink.InviteLink
	}
	return []lockdownRecentJoin{join}
}

// lockdownMessageJoins is the tracker entries for every user a new_chat_members
// service message names except the bot itself. The service message carries no invite
// link.
func lockdownMessageJoins(b *gotgbot.Bot, members []gotgbot.User) []lockdownRecentJoin {
	now := time.Now()
	joins := make([]lockdownRecentJoin, 0, len(members))
	for _, user := range members {
		if user.Id == b.Id {
			continue
		}
		joins = append(joins, lockdownRecentJoin{
			UserID:   user.Id,
			Name:     user.FirstName,
			Username: user.Username,
			Bot:      user.IsBot,
			At:       now,
		})
	}
	return joins
}
