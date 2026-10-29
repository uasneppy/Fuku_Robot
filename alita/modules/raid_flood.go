package modules

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
)

// The message and media flood trigger of automatic raid detection (D-05 to D-08). A
// watcher at handler group -4 counts every piece of user content in a supergroup into
// three Redis sorted sets in one Lua call: all items, the items of members who joined in
// the last 24 hours (the joined set of the join counter) and those members' IDs. A
// 30-second window that holds at least the chat's threshold of items, at least 60% of
// them from such new members, from at least 3 such accounts, starts a lockdown through
// raidFire.
//
// A command the bot handles does not count (D-07), and the way to know that is the
// dispatcher's own order: a handled command returns ext.EndGroups, which ends every later
// group. So the group -4 watcher skips every slash-prefixed text, and a second watcher in
// the last group (raidFloodLateGroup) counts the slash-prefixed texts that reach it, the
// ones no command handled: "/word" with no handler, "/////", "/start@otherbot". Both run
// the same function (raidFloodCountUpdate), so the item classification, the exemptions,
// the script and the trigger are one code path.
//
// It is best effort and Redis only. It never ends a message's handling (every path
// returns ext.ContinueGroups, so aispam, antiflood, locks and the rest still see the
// message) and it deletes nothing: the lockdown's muting stops the flood (D-08). Message
// text is never logged or stored, only item keys and user IDs.

// raidFloodGroup is the handler group of the flood watcher: after the lockdown join guard
// (-7) and fed-ban (-6), before the staff watchers (-3), aispam (3),
// antiflood (4), locks (5, 6) and blacklists (7), so a message those later end was
// already counted. It counts everything but slash-prefixed texts.
const raidFloodGroup = -4

// raidFloodLateGroup is the handler group of the late flood watcher, after every other
// handler group (the log channel's is 11). Only an update that no earlier handler ended
// reaches it, so a slash-prefixed text counted here is one no command handled. It counts
// slash-prefixed texts only.
const raidFloodLateGroup = 12

const (
	// raidFloodWindow is the window the flood counts in (fixed, D-05).
	raidFloodWindow = 30 * time.Second
	// raidFloodNewPercent is the share of the window's items that must come from new
	// members (D-06).
	raidFloodNewPercent = 60
	// raidFloodMinAccounts is how many different new members must have sent them (D-06).
	raidFloodMinAccounts = 3
	// raidFloodTTL lets an idle chat's sets expire on their own.
	raidFloodTTL = 2 * time.Minute

	// raidPartFloodItems is the sorted set of the window's item keys.
	raidPartFloodItems = "fi"
	// raidPartFloodNew is the sorted set of the window's item keys sent by new members.
	raidPartFloodNew = "fn"
	// raidPartFloodAccounts is the sorted set of the new members who sent them.
	raidPartFloodAccounts = "fa"

	// raidPartFlooders is the snapshot of the flooding accounts taken when the flood trigger
	// fires: a sorted set of user IDs scored by the time of their join in unix milliseconds
	// (D-17).
	raidPartFlooders = "flooders"
	// raidFloodersTTL is how long the snapshot lives. The lockdown worker reads it when it
	// decides the alert, normally within a cycle; 15 minutes also covers the settle path's
	// one-minute grace and a restart.
	raidFloodersTTL = 15 * time.Minute
	// raidFloodersMax is the most accounts the snapshot keeps (the newest to have sent an
	// item).
	raidFloodersMax = 500
)

// raidFloodCap is the most members each of the item sets keeps per chat (the newest). It
// is a variable only so a test can make the cap small; production code never reassigns
// it.
var raidFloodCap = 2000

// raidFloodScript adds one item to the three sets and returns their sizes: all items,
// new-member items and new-member accounts. A member counts as new when the joined set
// scores them within the recent window. Every set is trimmed to the flood window and
// capped on each write, and expires on its own.
//
// KEYS: 1 items, 2 new-member items, 3 new-member accounts, 4 joined set. ARGV: 1 now
// (ms), 2 flood window (ms), 3 recent window (ms), 4 item key, 5 user ID, 6 cap, 7 TTL
// (ms).
var raidFloodScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', cutoff)
local joinedAt = redis.call('ZSCORE', KEYS[4], ARGV[5])
if joinedAt and tonumber(joinedAt) > now - tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[2], now, ARGV[4])
  redis.call('ZADD', KEYS[3], now, ARGV[5])
end
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', cutoff)
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', cutoff)
local cap = tonumber(ARGV[6])
for i = 1, 2 do
  local n = redis.call('ZCARD', KEYS[i])
  if n > cap then redis.call('ZREMRANGEBYRANK', KEYS[i], 0, n - cap - 1) end
end
for i = 1, 3 do redis.call('PEXPIRE', KEYS[i], ARGV[7]) end
return {redis.call('ZCARD', KEYS[1]), redis.call('ZCARD', KEYS[2]), redis.call('ZCARD', KEYS[3])}
`)

// raidFloodItemKey classifies a message for the flood counter. It reports the item's
// key, whether the item is a slash-prefixed text, and true when the message is user
// content; it reports false for everything that never counts: service messages and
// messages with nothing to count. User content is text, and a photo, GIF (an animation is
// checked before the document Telegram also sets on it), sticker, video, video note,
// voice, audio, document, poll, contact, location, venue, dice or story, forwards and
// replies included. An album counts once: its parts share the key g:<media_group_id>,
// every other item is m:<message_id>.
//
// A slash-prefixed text (raidFloodIsSlashText) is a command only when some handler takes
// it, and the dispatcher tells that by ending the update, so the group -4 watcher leaves
// these to the late watcher (raidFloodCountUpdate).
func raidFloodItemKey(msg *gotgbot.Message) (key string, slash bool, ok bool) {
	if msg == nil {
		return "", false, false
	}
	if msg.NewChatMembers != nil || msg.LeftChatMember != nil || msg.PinnedMessage != nil ||
		msg.NewChatTitle != "" || msg.NewChatPhoto != nil || msg.DeleteChatPhoto ||
		msg.GroupChatCreated || msg.SupergroupChatCreated ||
		msg.MigrateToChatId != 0 || msg.MigrateFromChatId != 0 {
		return "", false, false
	}

	media := msg.Animation != nil || msg.Sticker != nil || msg.Photo != nil || msg.Video != nil ||
		msg.VideoNote != nil || msg.Voice != nil || msg.Audio != nil || msg.Document != nil ||
		msg.Poll != nil || msg.Contact != nil || msg.Location != nil || msg.Venue != nil ||
		msg.Dice != nil || msg.Story != nil
	text := msg.Text != ""
	if !media && !text {
		return "", false, false
	}
	slash = !media && raidFloodIsSlashText(msg)

	if msg.MediaGroupId != "" {
		return "g:" + msg.MediaGroupId, slash, true
	}
	return "m:" + strconv.FormatInt(msg.MessageId, 10), slash, true
}

// raidFloodIsSlashText reports whether the text message looks like a bot command the way
// the dispatcher sees one: its first entity is a bot_command at offset 0, or it begins
// with "/" and a non-space character. gotgbot's command handler accepts the second form
// too (a text with no entity at offset 0). Looking like a command is not being one the
// bot handles: that is only known once the dispatcher has run the handlers.
func raidFloodIsSlashText(msg *gotgbot.Message) bool {
	for _, entity := range msg.Entities {
		if entity.Offset == 0 && entity.Type == "bot_command" {
			return true
		}
	}
	rest, found := strings.CutPrefix(msg.Text, "/")
	if !found || rest == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsSpace(first)
}

// raidFloodCounts is the flood rule (D-05, D-06): fire when the window holds at least
// threshold items, at least raidFloodNewPercent of them from new members, from at least
// raidFloodMinAccounts different new accounts. percent is the new members' share of the
// items, rounded down, 0 when there are none.
func raidFloodCounts(items, newItems, accounts, threshold int) (fire bool, percent int) {
	if items <= 0 {
		return false, 0
	}
	percent = newItems * 100 / items
	fire = items >= threshold &&
		newItems*100 >= raidFloodNewPercent*items &&
		accounts >= raidFloodMinAccounts
	return fire, percent
}

// raidFloodSenderCounts reports whether the sender of an update is a member who can count
// toward the flood at all, judged from the update alone: a real user, not a post as the
// group or a channel (sender_chat is set for an anonymous administrator, a channel and a
// linked channel's automatic forward, and gotgbot's IsBot is false for all of them), not
// a Telegram service account and not the bot itself.
func raidFloodSenderCounts(b *gotgbot.Bot, ctx *ext.Context) (int64, bool) {
	msg := ctx.EffectiveMessage
	sender := ctx.EffectiveSender
	if msg == nil || sender == nil || sender.User == nil || sender.Chat != nil {
		return 0, false
	}
	if msg.SenderChat != nil || msg.IsAutomaticForward {
		return 0, false
	}
	userID := sender.User.Id
	if userID <= 0 || userID == b.Id || staffServiceUserIDs[userID] {
		return 0, false
	}
	return userID, true
}

// recordRaidFloodItem adds one item to the chat's flood sets and returns their sizes. One
// script call under raidRedisTimeout; an error is returned for the caller to log and
// treat as "no information".
func recordRaidFloodItem(client *redis.Client, chatID, userID int64, itemKey string) (items, newItems, accounts int, err error) {
	keys := []string{
		raidKey(chatID, raidPartFloodItems),
		raidKey(chatID, raidPartFloodNew),
		raidKey(chatID, raidPartFloodAccounts),
		raidKey(chatID, raidPartJoined),
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	sizes, err := raidFloodScript.Run(ctx, client, keys,
		raidNow().UnixMilli(),
		raidFloodWindow.Milliseconds(),
		raidRecentWindow.Milliseconds(),
		itemKey,
		strconv.FormatInt(userID, 10),
		raidFloodCap,
		raidFloodTTL.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return 0, 0, 0, err
	}
	if len(sizes) != 3 {
		return 0, 0, 0, fmt.Errorf("flood script returned %d counts, want 3", len(sizes))
	}
	return int(sizes[0]), int(sizes[1]), int(sizes[2]), nil
}

// raidFloodWatcher is the group -4 flood watcher. It counts every item but a
// slash-prefixed text; those wait for raidFloodLateWatcher.
func (m moduleStruct) raidFloodWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("raidFloodWatcher", "Lockdown")
	return raidFloodCountUpdate(b, ctx, false)
}

// raidFloodLateWatcher is the group 12 flood watcher. An update that reaches it was ended
// by no earlier handler, so a slash-prefixed text it sees is one no command took, and it
// counts like any other text (D-07 exempts only the commands the bot handles). It counts
// slash-prefixed texts only; everything else was counted at -4.
func (m moduleStruct) raidFloodLateWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("raidFloodLateWatcher", "Lockdown")
	return raidFloodCountUpdate(b, ctx, true)
}

// raidFloodCountUpdate counts one message toward its chat's flood and starts a lockdown
// when the rule holds. late selects which half of the items this call is for: false for
// everything but slash-prefixed texts (the group -4 watcher), true for the slash-prefixed
// texts only (the group 12 watcher). Checks run cheapest first, because this sees every
// group message: the chat type, the sender's shape and the item (all pure Go), then Redis,
// then the chat's settings (a flood switched off returns here), then approved users, then
// the cached admin list (D-07 accepts the cached list; the live fallback it makes is
// bounded and assumes an admin when saturated, which only lowers the counts). It returns
// ext.ContinueGroups on every path.
func raidFloodCountUpdate(b *gotgbot.Bot, ctx *ext.Context, late bool) error {
	chat := ctx.EffectiveChat
	if chat == nil || chat.Type != "supergroup" {
		return ext.ContinueGroups
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok {
		return ext.ContinueGroups
	}
	itemKey, slash, ok := raidFloodItemKey(ctx.EffectiveMessage)
	if !ok || slash != late {
		return ext.ContinueGroups
	}
	client := raidRedis()
	if client == nil {
		return ext.ContinueGroups
	}

	settingsCtx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
	settings, err := raidSettingsLookup(settingsCtx, chat.Id)
	cancel()
	if err != nil {
		log.Debugf("[RaidDetection] settings of chat %d could not be read, counting nothing: %v", chat.Id, err)
		return ext.ContinueGroups
	}
	if settings == nil || !settings.FloodEnabled {
		return ext.ContinueGroups
	}

	if chat_status.IsApprovedForUpdate(b, ctx, chat.Id, userID) {
		return ext.ContinueGroups
	}
	if antifloodModule.userIsFloodExempt(b, chat.Id, userID) {
		return ext.ContinueGroups
	}

	items, newItems, accounts, err := recordRaidFloodItem(client, chat.Id, userID, itemKey)
	if err != nil {
		log.Debugf("[RaidDetection] flood counter write for chat %d failed: %v", chat.Id, err)
		return ext.ContinueGroups
	}
	if fire, percent := raidFloodCounts(items, newItems, accounts, settings.FloodThreshold); fire {
		raidFire(b, chat, raidTrigger{
			Kind:    models.LockdownTriggerFlood,
			Count:   items,
			Window:  int(raidFloodWindow / time.Second),
			Percent: percent,
		})
	}
	return ext.ContinueGroups
}

// raidFloodersScript copies the flooding accounts into the snapshot set when the flood
// trigger fires (D-17, research Pitfall 10). The flood sets only keep the last 30 seconds,
// and the join list of the lockdown tracker only the last 10 minutes, so an account that
// joined hours ago and flooded now would be on neither once the alert is decided. It
// replaces any older snapshot, takes the newest members of the accounts set (at most the
// given maximum) and keeps each whose join is within the recent window, scored by that
// join time, then sets the TTL. It returns how many accounts it kept.
//
// KEYS: 1 flood accounts, 2 joined set, 3 flooders snapshot. ARGV: 1 now (ms), 2 recent
// window (ms), 3 maximum accounts, 4 TTL (ms).
var raidFloodersScript = redis.NewScript(`
redis.call('DEL', KEYS[3])
local now = tonumber(ARGV[1])
local cutoff = now - tonumber(ARGV[2])
local ids = redis.call('ZREVRANGE', KEYS[1], 0, tonumber(ARGV[3]) - 1)
local kept = 0
for _, id in ipairs(ids) do
  local joinedAt = redis.call('ZSCORE', KEYS[2], id)
  if joinedAt and tonumber(joinedAt) > cutoff then
    redis.call('ZADD', KEYS[3], joinedAt, id)
    kept = kept + 1
  end
end
if kept > 0 then redis.call('PEXPIRE', KEYS[3], ARGV[4]) end
return kept
`)

// raidSnapshotFlooders saves the chat's flooding accounts with their join times, for the
// alert of the lockdown the flood trigger is about to start. It returns how many accounts
// it saved. Without Redis it saves nothing and returns (0, nil). One script call under
// raidRedisTimeout; an error is returned for the caller to log, and the lockdown starts
// without a snapshot.
func raidSnapshotFlooders(chatID int64) (int, error) {
	client := raidRedis()
	if client == nil {
		return 0, nil
	}
	keys := []string{
		raidKey(chatID, raidPartFloodAccounts),
		raidKey(chatID, raidPartJoined),
		raidKey(chatID, raidPartFlooders),
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	return raidFloodersScript.Run(ctx, client, keys,
		raidNow().UnixMilli(),
		raidRecentWindow.Milliseconds(),
		raidFloodersMax,
		raidFloodersTTL.Milliseconds(),
	).Int()
}

// readRaidFlooders returns the chat's flooder snapshot, oldest join first, each as a
// lockdownRecentJoin whose At is the join time and which carries no name: the snapshot
// holds IDs only. Without Redis it returns nothing and no error; no snapshot is the same.
func readRaidFlooders(chatID int64) ([]lockdownRecentJoin, error) {
	client := raidRedis()
	if client == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	entries, err := client.ZRangeWithScores(ctx, raidKey(chatID, raidPartFlooders), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	flooders := make([]lockdownRecentJoin, 0, len(entries))
	for _, entry := range entries {
		raw, ok := entry.Member.(string)
		if !ok {
			continue
		}
		userID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || userID == 0 {
			continue
		}
		flooders = append(flooders, lockdownRecentJoin{UserID: userID, At: time.UnixMilli(int64(entry.Score))})
	}
	return flooders, nil
}

// raidFloodersLookup is the snapshot read of the flooding accounts. It is a variable only
// so a test can see whether it is called; production code never reassigns it.
var raidFloodersLookup = readRaidFlooders
