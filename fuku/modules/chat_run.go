package modules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/chatconv"
	"github.com/uasneppy/fuku_robot/fuku/utils/chatprompt"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
)

// The conversation pool (Phase 20): a buffered queue and a fixed set of workers, shaped like the
// raid AI pool. A job, in order: the per-user cooldown, the provider's breaker, the hourly budgets,
// the typing action, the transcript, the request, the provider call, the output filter, the reply,
// the record of the exchange and the counters. Every provider call and every send derives from
// the pool's root context, which StopChat cancels first.

const (
	// chatQueueCapacity is how many jobs wait at most; a full queue sheds.
	chatQueueCapacity = 64
	// chatWorkerCount is how many replies are prepared at once on a replica.
	chatWorkerCount = 3
	// chatJobTimeout bounds one job: the provider call (40 s with its retry) and the reply.
	chatJobTimeout = 60 * time.Second
)

var (
	// chatUserHourlyBudget, chatChatHourlyBudget and chatGlobalHourlyBudget are the replies one
	// member may get in one group, one group may get and the whole bot may give in one UTC hour.
	// Package variables only so a test can shrink them.
	chatUserHourlyBudget   int64 = 12
	chatChatHourlyBudget   int64 = 80
	chatGlobalHourlyBudget int64 = 800

	// chatCooldown is the least time between two replies to one member in one group. A variable
	// only so a test can shorten it.
	chatCooldown = 5 * time.Second

	// chatStopWait is how long StopChat waits for the jobs in flight.
	chatStopWait = 2 * time.Second
)

// chatJob is one addressed message waiting for its reply. It holds the speaker's first name and
// text for the prompt, and IDs only for Telegram and the guards.
type chatJob struct {
	bot       *gotgbot.Bot
	chatID    int64
	chatTitle string
	userID    int64
	messageID int64
	threadID  int64
	provider  string
	turn      chatprompt.Turn
	quoted    *chatprompt.Turn
}

var (
	chatWorkersOnce sync.Once
	chatQueue       chan chatJob
	// chatJobs counts the jobs queued or running; a counter and not a sync.WaitGroup, for the
	// reason raidPostTracker gives.
	chatJobs raidPostTracker
	// chatDraining is set by StopChat, so a late message is shed instead of queued.
	chatDraining atomic.Bool
	// chatProcess runs one job; a variable only so a test can install a gated one.
	chatProcess = chatProcessJob
	// chatProviderOf returns a provider's adapter; a variable only so a test can swap it.
	chatProviderOf = chatProviderFor
)

// chatPoolContext holds the context every call of the pool derives from.
var chatPoolContext struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
}

// chatContext returns the pool's root context, creating it on first use.
func chatContext() context.Context {
	chatPoolContext.mu.Lock()
	defer chatPoolContext.mu.Unlock()
	if chatPoolContext.ctx == nil {
		chatPoolContext.ctx, chatPoolContext.cancel = context.WithCancel(context.Background())
	}
	return chatPoolContext.ctx
}

// chatCancel cancels the root context, creating it first when no job ever ran.
func chatCancel() {
	chatContext()
	chatPoolContext.mu.Lock()
	defer chatPoolContext.mu.Unlock()
	chatPoolContext.cancel()
}

// The breakers, one per provider, each of its own state: five provider failures in a group pause
// that group's conversations on that provider for 5 minutes; a rejected key or account, a long
// rate limit or Gemini's daily quota pauses the provider for every group.
var chatBreakers = map[string]*memberReqAIBreaker{
	chatconv.ProviderAnthropic: newMemberReqAIBreaker(),
	chatconv.ProviderGoogle:    newMemberReqAIBreaker(),
}

// chatBreaker returns a provider's breaker.
func chatBreaker(provider string) *memberReqAIBreaker {
	if breaker, ok := chatBreakers[provider]; ok {
		return breaker
	}
	return chatBreakers[chatconv.ProviderAnthropic]
}

// The in-memory counters, per replica, for the developer /stats.
var (
	chatAnsweredCount atomic.Int64
	chatRefusedCount  atomic.Int64
	chatFailedCount   atomic.Int64
	chatShedCount     atomic.Int64
	chatCappedCount   atomic.Int64
)

// chatStatsSection is the conversation part of the developer /stats: numbers of this replica since
// its last restart, no text and no ID.
func chatStatsSection() string {
	return fmt.Sprintf(
		"\n    <b>Conversations:</b> answered %d, refused %d, failed %d, shed %d, capped %d",
		chatAnsweredCount.Load(), chatRefusedCount.Load(), chatFailedCount.Load(), chatShedCount.Load(), chatCappedCount.Load(),
	)
}

func chatEnsureWorkers() {
	chatWorkersOnce.Do(func() {
		chatQueue = make(chan chatJob, chatQueueCapacity)
		chatContext()
		for range chatWorkerCount {
			go chatWorker()
		}
	})
}

func chatWorker() {
	defer error_handling.RecoverFromPanic("chatWorker", "Chat")
	for job := range chatQueue {
		func() {
			defer chatJobs.finish()
			defer error_handling.RecoverFromPanic("chatProcess", "Chat")
			if chatContext().Err() != nil {
				chatShedCount.Add(1)
				return
			}
			chatProcess(job)
		}()
	}
}

// chatEnqueue hands a job to the pool, shedding it when the queue is full or the process is
// draining. A shed job gets no reply and no line.
func chatEnqueue(job chatJob) bool {
	chatEnsureWorkers()
	chatJobs.start()
	if chatDraining.Load() {
		chatJobs.finish()
		chatShedCount.Add(1)
		return false
	}
	select {
	case chatQueue <- job:
		return true
	default:
		chatJobs.finish()
		chatShedCount.Add(1)
		log.WithFields(log.Fields{"chat_id": job.chatID, "user_id": job.userID}).Debug("[Chat] queue full, message shed")
		return false
	}
}

// chatCooldownKey is one member's cooldown in one group.
func chatCooldownKey(chatID, userID int64) string {
	return chatKeyPrefix + "{" + strconv.FormatInt(chatID, 10) + "}:cd:" + strconv.FormatInt(userID, 10)
}

// chatTakeCooldown claims the member's cooldown. A member inside it, and a Redis error, get no
// reply.
func chatTakeCooldown(ctx context.Context, client *redis.Client, chatID, userID int64) bool {
	ctx, cancel := context.WithTimeout(ctx, chatRedisTimeout)
	defer cancel()
	won, err := client.SetNX(ctx, chatCooldownKey(chatID, userID), "1", chatCooldown).Result()
	return err == nil && won
}

// chatBudgetResult is what the hourly budgets said.
type chatBudgetResult int

const (
	chatBudgetReserved chatBudgetResult = iota
	chatBudgetCapped
	chatBudgetFault
)

// chatReserve takes the member's unit first, then the group's, then the fleet's, and stops at the
// first refusal or error, so a member over their own allowance spends neither shared unit.
func chatReserve(client *redis.Client, chatID, userID int64) chatBudgetResult {
	stamp := raidAIHourStamp(raidNow())
	chat := strconv.FormatInt(chatID, 10)
	steps := [...]struct {
		key   string
		limit int64
	}{
		{chatKeyPrefix + "b:u:" + chat + ":" + strconv.FormatInt(userID, 10) + ":" + stamp, chatUserHourlyBudget},
		{chatKeyPrefix + "b:c:" + chat + ":" + stamp, chatChatHourlyBudget},
		{chatKeyPrefix + "b:g:" + stamp, chatGlobalHourlyBudget},
	}
	for _, step := range steps {
		allowed, err := raidAIBudgetTake(client, step.key, step.limit)
		if err != nil {
			return chatBudgetFault
		}
		if !allowed {
			return chatBudgetCapped
		}
	}
	return chatBudgetReserved
}

// chatProcessJob prepares and sends one reply.
func chatProcessJob(job chatJob) {
	root := chatContext()
	ctx, cancel := context.WithTimeout(root, chatJobTimeout)
	defer cancel()
	fields := log.Fields{"chat_id": job.chatID, "user_id": job.userID}

	client := chatRedisClient()
	if client == nil || !chatProviderConfigured(job.provider) {
		chatShedCount.Add(1)
		return
	}
	if !chatTakeCooldown(ctx, client, job.chatID, job.userID) {
		chatShedCount.Add(1)
		return
	}
	breaker := chatBreaker(job.provider)
	if breaker.isPaused(job.chatID, raidNow()) {
		chatShedCount.Add(1)
		chatSayGated(ctx, job.bot, client, job, chatSaidMalfunction, "chat_reply_malfunction", chatMalfunctionGate)
		return
	}
	switch chatReserve(client, job.chatID, job.userID) {
	case chatBudgetCapped:
		chatCappedCount.Add(1)
		chatSayGated(ctx, job.bot, client, job, chatSaidAllowance, "chat_reply_allowance", chatAllowanceGate)
		return
	case chatBudgetFault:
		chatShedCount.Add(1)
		return
	}

	chatSendTyping(ctx, job.bot, job)
	history := chatReadHistory(ctx, client, job.chatID)
	request := chatBuildRequest(ctx, job, job.bot.Username, history)
	answer, err := chatProviderOf(job.provider).converse(ctx, request)
	if err != nil {
		if root.Err() != nil || errors.Is(err, context.Canceled) {
			// Shutdown: neither a failure nor a success, and nothing is said.
			chatShedCount.Add(1)
			return
		}
		chatFailedCount.Add(1)
		chatRecordFailure(job, err)
		chatSayGated(ctx, job.bot, client, job, chatSaidMalfunction, "chat_reply_malfunction", chatMalfunctionGate)
		return
	}
	breaker.clear(job.chatID)

	if answer.Outcome == chatRefused {
		chatRefusedCount.Add(1)
		text := chatCannedText(job.chatID, "chat_reply_refused")
		if text == "" {
			return
		}
		sent, err := chatSendReply(ctx, job.bot, job, text)
		if err != nil {
			log.WithFields(fields).Debugf("[Chat] the refusal line could not be sent: %s", chatSendErrClass(err))
			return
		}
		if err := chatRememberOwn(client, job.chatID, sent.MessageId); err != nil {
			log.WithFields(fields).Debug("[Chat] the refusal line could not be remembered")
		}
		return
	}

	text := chatFilterReply(answer.Text)
	if text == "" {
		chatFailedCount.Add(1)
		chatSayGated(ctx, job.bot, client, job, chatSaidMalfunction, "chat_reply_malfunction", chatMalfunctionGate)
		return
	}
	sent, err := chatSendReply(ctx, job.bot, job, text)
	if err != nil && chatReplyTargetGone(err) {
		chatShedCount.Add(1)
		log.WithFields(fields).Debug("[Chat] the addressed message was deleted before the reply, nothing sent")
		return
	}
	if err != nil {
		chatFailedCount.Add(1)
		log.WithFields(fields).Warnf("[Chat] the reply could not be sent: %s", chatSendErrClass(err))
		return
	}
	chatAnsweredCount.Add(1)
	now := time.Now().Unix()
	member := chatHistoryEntry{Name: job.turn.Name, Text: job.turn.Text, MessageID: job.messageID, At: now}
	reply := chatHistoryEntry{Text: text, MessageID: sent.MessageId, At: now}
	if err := chatRecordExchange(client, job.chatID, member, reply); err != nil {
		log.WithFields(fields).Debug("[Chat] the exchange could not be recorded")
	}
	log.WithFields(fields).Debugf("[Chat] answered (%d input, %d output tokens)", answer.InputTokens, answer.OutputTokens)
}

// chatRecordFailure counts one provider failure toward the group on the provider's breaker and
// pauses the provider for every group when the error calls for it. Each pause logs one warning
// naming the status and the error type; nothing names text, a name or a key.
func chatRecordFailure(job chatJob, err error) {
	now := raidNow()
	breaker := chatBreaker(job.provider)
	class := chatErrorClass(err)
	log.WithFields(log.Fields{"chat_id": job.chatID, "user_id": job.userID}).Warnf("[Chat] the %s provider failed: %s", job.provider, class)
	if span := chatProviderSpan(err); span > 0 {
		if till, started := breaker.pauseProvider(now, span); started {
			log.Warnf("[Chat] conversations on %s paused for every group (%s) until %s", job.provider, class, till.UTC().Format(time.RFC3339))
		}
	}
	if outcome := breaker.countFailure(job.chatID, now); outcome.tripped {
		log.WithField("chat_id", job.chatID).Warnf("[Chat] conversations paused for this group after %d provider failures, until %s",
			memberReqAIFailures, outcome.till.UTC().Format(time.RFC3339))
	}
}

// StopChat is the conversation pool's shutdown stop: it sets the draining flag (a late message is
// shed), cancels the root context first (every provider call and send in flight ends at once) and
// waits at most chatStopWait for the jobs. A reply cut off is lost; nothing needs resuming.
func StopChat() bool {
	chatDraining.Store(true)
	chatCancel()
	select {
	case <-chatJobs.idleChan():
		return true
	case <-time.After(chatStopWait):
		log.Warn("[Chat] shutdown stop timed out with replies still in flight")
		return false
	}
}
