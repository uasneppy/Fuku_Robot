package modules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/config"
	"github.com/divkix/Alita_Robot/alita/db/aispam"
	"github.com/divkix/Alita_Robot/alita/db/lang"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
)

// The AI candidate watcher and the raid AI pool of automatic raid detection (RAID-04,
// RAID-05). A watcher at handler group 2, between the commands at 0 and /aispam at 3,
// picks the texts of members who joined this supergroup in the last 24 hours that a cheap
// rule finds suspicious (a link, a forward, or a text the sender just repeated, D-01) and
// hands them to a pool of workers that asks TypeSafe once per text. A spam verdict is
// reported to the AI burst counter (raid_ai_count.go) and nothing else.
//
// AI never decides alone: nothing here deletes, restricts, bans or posts, and the watcher
// never ends an update or waits on AI. A slow, failing or paused provider is no verdict.
// No message text or image is logged or stored: the repeat test keeps only a short hash of
// a new member's recent texts, and the logs carry only chat, user and message IDs.

const (
	// raidAIGroup is the handler group of the candidate watcher: after the commands (0),
	// whose handled texts end the update and are never judged, and before /aispam (3).
	raidAIGroup = 2

	// raidPartAIRepeat starts the key of a new member's recent text hashes; the user ID
	// follows (alita:raid:{<chat>}:ar:<user>).
	raidPartAIRepeat = "ar"
	// raidAIRepeatHistory is how many hashes of a new member's texts are kept (the newest).
	raidAIRepeatHistory = 5
	// raidAIRepeatTTL lets an idle member's hashes expire on their own.
	raidAIRepeatTTL = time.Hour
	// raidAIRepeatMinRunes is the shortest normalised text that can count as a repeat, so
	// "ok" sent twice never does.
	raidAIRepeatMinRunes = 10

	// raidAIQueueCapacity is how many checks the raid AI pool queues before it sheds (D-18).
	raidAIQueueCapacity = 128
	// raidAIWorkerCount is how many checks the pool runs at once (D-18).
	raidAIWorkerCount = 4
)

// raidAITextConfigured reports whether a provider is set up to judge texts: the same
// TypeSafe configuration /aispam needs.
func raidAITextConfigured() bool {
	return config.AppConfig.EnableAISpam && config.AppConfig.TypeSafeAPIKey != ""
}

// raidAIImageConfigured reports whether a provider is set up to judge images.
func raidAIImageConfigured() bool {
	return config.AppConfig.GeminiAPIKey != ""
}

// raidAIConfigured reports whether any AI provider of the raid path is set up.
func raidAIConfigured() bool {
	return raidAITextConfigured() || raidAIImageConfigured()
}

// aispamWillJudge reports whether /aispam will judge a text of the chat at group 3: the
// gates at the top of checkAISpam, the TypeSafe configuration, the chat's own /aispam
// switch and a breaker that is not paused. Where it holds, the raid path leaves the text
// to /aispam, whose delete verdict feeds the same burst counter (D-03, D-19).
func aispamWillJudge(chatID int64) bool {
	return raidAITextConfigured() && aispam.IsAISpamEnabled(chatID) && !aispamBreakerPaused(chatID)
}

// raidAITextHash is the repeat test's fingerprint of a text: lower-cased, whitespace
// collapsed and trimmed, then the first 8 bytes of its SHA-256 in hex. A text shorter than
// raidAIRepeatMinRunes after that has no fingerprint (empty), so it can never repeat. The
// text itself is never stored (D-15).
func raidAITextHash(text string) string {
	normalised := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if utf8.RuneCountInString(normalised) < raidAIRepeatMinRunes {
		return ""
	}
	sum := sha256.Sum256([]byte(normalised))
	return hex.EncodeToString(sum[:8])
}

// raidAICandidateScript tells whether the sender is a new member (joined within the recent
// window) and, for a new member, whether the text hash is one of their last few, then
// records it. A member who is not new records nothing, so established members leave no
// trace. An empty hash records nothing and is never a repeat.
//
// KEYS: 1 joined set, 2 the member's hash list. ARGV: 1 now (ms), 2 recent window (ms),
// 3 user ID, 4 hash (may be empty), 5 history size, 6 TTL (ms). It returns {new, repeated}.
var raidAICandidateScript = redis.NewScript(`
local joinedAt = redis.call('ZSCORE', KEYS[1], ARGV[3])
if not joinedAt or tonumber(joinedAt) <= tonumber(ARGV[1]) - tonumber(ARGV[2]) then
  return {0, 0}
end
local repeated = 0
if ARGV[4] ~= '' then
  local seen = redis.call('LRANGE', KEYS[2], 0, -1)
  for _, h in ipairs(seen) do
    if h == ARGV[4] then repeated = 1 break end
  end
  redis.call('LPUSH', KEYS[2], ARGV[4])
  redis.call('LTRIM', KEYS[2], 0, tonumber(ARGV[5]) - 1)
  redis.call('PEXPIRE', KEYS[2], ARGV[6])
end
return {1, repeated}
`)

// raidAICheckCandidate asks the candidate script about one text of userID in the chat. One
// script call under raidRedisTimeout; an error is returned for the caller to treat as "no
// information" (the watcher continues and sends nothing).
func raidAICheckCandidate(client *redis.Client, chatID, userID int64, hash string) (isNew, repeated bool, err error) {
	keys := []string{
		raidKey(chatID, raidPartJoined),
		raidKey(chatID, raidPartAIRepeat+":"+strconv.FormatInt(userID, 10)),
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	answer, err := raidAICandidateScript.Run(ctx, client, keys,
		raidNow().UnixMilli(),
		raidRecentWindow.Milliseconds(),
		strconv.FormatInt(userID, 10),
		hash,
		raidAIRepeatHistory,
		raidAIRepeatTTL.Milliseconds(),
	).Int64Slice()
	if err != nil {
		return false, false, err
	}
	if len(answer) != 2 {
		return false, false, fmt.Errorf("AI candidate script returned %d values, want 2", len(answer))
	}
	return answer[0] == 1, answer[1] == 1, nil
}

// raidAIJob is one check for the pool. It carries copies, never the update, so a worker
// holds nothing of the dispatcher. The text is sent to TypeSafe and is never logged.
type raidAIJob struct {
	bot         *gotgbot.Bot
	chat        gotgbot.Chat
	userID      int64
	messageID   int64
	itemKey     string
	text        string
	linkDomains []string
	isForward   bool
	isReply     bool
	title       string
	language    string
}

// raidAIWatcher is the group 2 candidate watcher. It returns ext.ContinueGroups on every
// path, so /aispam, antiflood, locks and the rest still see the message, and it neither
// waits on AI nor acts: it costs one settings read and one Redis script call, and a
// suspicious text of a new member goes to the pool.
func (m moduleStruct) raidAIWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	defer error_handling.RecoverFromPanic("raidAIWatcher", "Lockdown")
	raidAIWatchText(b, ctx)
	return ext.ContinueGroups
}

// raidAIWatchText applies the candidate rules of D-01 and D-02 to one message, cheapest
// first (the way raidFloodCountUpdate orders its checks): the chat type, the sender's
// shape, the item, the configuration, Redis, the chat's settings, the candidate script and
// its rule, and last the carve-outs that cost a lookup.
func raidAIWatchText(b *gotgbot.Bot, ctx *ext.Context) {
	chat := ctx.EffectiveChat
	if chat == nil || chat.Type != "supergroup" {
		return
	}
	userID, ok := raidFloodSenderCounts(b, ctx)
	if !ok {
		return
	}
	// A caption belongs to the image path, not to this one.
	msg := ctx.EffectiveMessage
	if msg == nil || msg.Text == "" {
		return
	}
	if !raidAITextConfigured() {
		return
	}
	client := raidRedis()
	if client == nil {
		return
	}

	settingsCtx, cancel := context.WithTimeout(context.Background(), raidSettingsTimeout)
	settings, err := raidSettingsLookup(settingsCtx, chat.Id)
	cancel()
	if err != nil {
		log.Debugf("[RaidDetection] settings of chat %d could not be read, checking nothing: %v", chat.Id, err)
		return
	}
	if settings == nil || !settings.AIEnabled {
		return
	}

	// D-03: where /aispam will judge this text at group 3, the raid path sends nothing and
	// reuses /aispam's own delete verdict (raidAIRecordVerdict), so a message gets at most
	// one TypeSafe call.
	if aispamWillJudge(chat.Id) {
		return
	}

	isNew, repeated, err := raidAICheckCandidate(client, chat.Id, userID, raidAITextHash(msg.Text))
	if err != nil {
		log.Debugf("[RaidDetection] AI candidate script for chat %d failed: %v", chat.Id, err)
		return
	}
	if !isNew {
		return
	}
	domains := aispamLinkDomains(msg, msg.Text)
	if len(domains) == 0 && msg.ForwardOrigin == nil && !repeated {
		return
	}
	if aispamCarvedOut(b, ctx, chat.Id, userID) {
		return
	}

	raidAIEnqueue(raidAIJob{
		bot:         b,
		chat:        *chat,
		userID:      userID,
		messageID:   msg.MessageId,
		itemKey:     raidAIItemKey(msg),
		text:        msg.Text,
		linkDomains: domains,
		isForward:   msg.ForwardOrigin != nil,
		isReply:     msg.ReplyToMessage != nil,
		title:       chat.Title,
		language:    lang.GetLanguage(ctx),
	})
}

//
// The raid AI pool
//

var (
	raidAIWorkersOnce sync.Once
	raidAIQueue       chan raidAIJob
	// raidAIJobs counts the checks queued or running. It is a counter and not a
	// sync.WaitGroup for the reason raidPostTracker gives: a drain that gives up after its
	// wait must not meet a late start.
	raidAIJobs raidPostTracker
	// raidAIDraining is set before the shutdown drain, so a late update is shed instead of
	// started.
	raidAIDraining atomic.Bool

	// raidAIProcess runs one job. It is a variable only so a test can install a gated
	// processor; production code never reassigns it.
	raidAIProcess = raidAIProcessJob
)

var (
	raidAIChecked atomic.Int64
	raidAISpam    atomic.Int64
	raidAIKept    atomic.Int64
	raidAIShed    atomic.Int64
	raidAIFailed  atomic.Int64
)

// RaidAIStats is the raid AI path's counters (D-15).
type RaidAIStats struct {
	// Checked is the provider answers that carried a verdict.
	Checked int64
	// Spam is the checked answers that were a spam verdict, each reported to the burst count.
	Spam int64
	// Kept is the checked answers that were not.
	Kept int64
	// Shed is the checks never run: the queue was full, the process was draining or the
	// chat's breaker was paused.
	Shed int64
	// Failed is the checks that got no verdict from the provider.
	Failed int64
}

// GetRaidAIStats reports the raid AI path's counters.
func GetRaidAIStats() RaidAIStats {
	return RaidAIStats{
		Checked: raidAIChecked.Load(),
		Spam:    raidAISpam.Load(),
		Kept:    raidAIKept.Load(),
		Shed:    raidAIShed.Load(),
		Failed:  raidAIFailed.Load(),
	}
}

func raidAIEnsureWorkers() {
	raidAIWorkersOnce.Do(func() {
		// A fixed pool: a provider brownout sheds checks instead of growing goroutines.
		raidAIQueue = make(chan raidAIJob, raidAIQueueCapacity)
		for range raidAIWorkerCount {
			go raidAIWorker()
		}
	})
}

func raidAIWorker() {
	defer error_handling.RecoverFromPanic("raidAIWorker", "Lockdown")

	for job := range raidAIQueue {
		func() {
			defer raidAIJobs.finish()
			defer error_handling.RecoverFromPanic("raidAIProcess", "Lockdown")
			raidAIProcess(job)
		}()
	}
}

// raidAIEnqueue hands a check to the pool, shedding it when the queue is full or the
// process is draining. A shed check means "not checked", which only lowers the AI count.
func raidAIEnqueue(job raidAIJob) bool {
	raidAIEnsureWorkers()

	// The counter is taken before the drain flag is read, like aispamEnqueue: a check
	// counted here is either waited for or sees the flag and is shed without any work.
	raidAIJobs.start()
	if raidAIDraining.Load() {
		raidAIJobs.finish()
		raidAIShed.Add(1)
		return false
	}

	select {
	case raidAIQueue <- job:
		return true
	default:
		raidAIJobs.finish()
		raidAIShed.Add(1)
		log.WithFields(log.Fields{
			"chat_id": job.chat.Id,
			"user_id": job.userID,
		}).Debug("[RaidDetection] AI queue full, check shed")
		return false
	}
}

// raidAIProcessJob runs one queued check.
func raidAIProcessJob(job raidAIJob) {
	raidAIProcessText(job)
}

// raidAIProcessText asks TypeSafe about one new member's text and reports a spam verdict to
// the AI burst counter. It never deletes, restricts, bans or posts. It shares /aispam's
// per-chat breaker silently: a paused chat is not asked, a failure counts toward the
// breaker but its notice is never posted from here (the chat may never have enabled
// /aispam), and a success resets it. It never reads or writes the /aispam sender history,
// which stores raw text, and logs only IDs.
func raidAIProcessText(job raidAIJob) {
	if aispamBreakerPaused(job.chat.Id) {
		raidAIShed.Add(1)
		return
	}

	task := aispamTask{
		bot:         job.bot,
		chat:        job.chat,
		itemKey:     job.itemKey,
		chatID:      job.chat.Id,
		messageID:   job.messageID,
		userID:      job.userID,
		text:        job.text,
		hasLink:     len(job.linkDomains) > 0,
		linkDomains: job.linkDomains,
		isForward:   job.isForward,
		isReply:     job.isReply,
		title:       job.title,
		language:    job.language,
	}
	verdict, err := aispamJevDecide(context.Background(), config.AppConfig.TypeSafeAPIKey, task.state(nil, 1))
	if err != nil {
		raidAIFailed.Add(1)
		log.WithFields(log.Fields{
			"chat_id":    job.chat.Id,
			"user_id":    job.userID,
			"message_id": job.messageID,
			"error":      err,
		}).Warn("[RaidDetection] AI text check failed, no verdict")
		// The tripped return is ignored on purpose: no breaker notice is posted from the raid path.
		_ = aispamBreakerRecordFailure(job.chat.Id)
		return
	}
	aispamBreakerRecordSuccess(job.chat.Id)
	raidAIChecked.Add(1)

	spam := aispamIsDelete(verdict)
	log.WithFields(log.Fields{
		"chat_id":     job.chat.Id,
		"user_id":     job.userID,
		"message_id":  job.messageID,
		"decision":    aispamDecisionName(spam),
		"probability": verdict.DeleteProbability,
	}).Info("[RaidDetection] AI text verdict")

	if !spam {
		raidAIKept.Add(1)
		return
	}
	raidAISpam.Add(1)
	raidAIRecordVerdict(job.bot, job.chat, job.userID, job.itemKey)
}
