package modules

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/lockdown"
	"github.com/divkix/Alita_Robot/alita/db/logchannels"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/actionlog"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/formatting"
)

const (
	// raidFireTimeout bounds one automatic start: the bot-rights check, getChat, the row
	// insert and the lock call.
	raidFireTimeout = 30 * time.Second

	// raidLiftQuiet is how long after an admin started lifting a group's lockdown the
	// automatic triggers leave the group alone (D-15).
	raidLiftQuiet = 10 * time.Minute

	// raidRetryLatch is how long the fire latch lasts after an attempt that failed for a
	// reason that may pass at once (a Telegram or database error), so the next qualifying
	// join tries again soon.
	raidRetryLatch = 10 * time.Second

	// raidCantLockLatch is how long the fire latch lasts for a group the bot cannot lock
	// (not a supergroup, no restrict right, or a lock Telegram refused), so a sustained
	// raid does not repeat the same failing calls on every join.
	raidCantLockLatch = 10 * time.Minute

	// raidPostTimeout bounds one public notice or one warning copy, the wait for its pacer
	// slot included.
	raidPostTimeout = 30 * time.Second

	// raidPartWarn is the key part of the once-a-day "I saw a raid I cannot lock" gate:
	// alita:raid:{<chat>}:warn, SET NX with a raidWarnEvery expiry (D-19).
	raidPartWarn = "warn"

	// raidWarnEvery is how long one warning silences the next for the same group, across
	// replicas.
	raidWarnEvery = 24 * time.Hour

	// raidTriggerToken stands in for the trigger sentence until the translated warning is
	// assembled, so the sentence (it holds a percent sign) never meets the translator's
	// legacy printf pass.
	raidTriggerToken = "<<raid-trigger>>"
)

// raidPostStopWait is the longest StopRaidPosts waits for the posts in flight at shutdown. A
// post is best effort: the lockdown it announces is already in place and its alert is driven
// by the database, so a post cut off with the process loses only a notice. A variable so a
// test can shorten it.
var raidPostStopWait = 2 * time.Second

// raidPostTracker counts the public notices and cannot-lock warnings in flight
// (raidPostAsync). It is a counter and not a sync.WaitGroup because the shutdown drain gives
// up after a bounded wait and posts may still start while a waiter is abandoned; a
// WaitGroup must not be added to concurrently with a Wait that began at zero, and this
// counter has no such rule.
type raidPostTracker struct {
	mu      sync.Mutex
	running int
	idle    chan struct{}
}

// raidPostsClosed is the idle channel of a tracker with nothing running.
var raidPostsClosed = func() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}()

// raidPosts is the posts in flight. StopRaidPosts waits on it at shutdown, and the tests
// wait on it before they read what was sent.
var raidPosts raidPostTracker

func (t *raidPostTracker) start() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running == 0 {
		t.idle = make(chan struct{})
	}
	t.running++
}

func (t *raidPostTracker) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running--
	if t.running == 0 {
		close(t.idle)
	}
}

// idleChan is closed when no post is in flight: already closed when none is running now,
// else closed by the last post to finish.
func (t *raidPostTracker) idleChan() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running == 0 {
		return raidPostsClosed
	}
	return t.idle
}

// wait blocks until no post is in flight.
func (t *raidPostTracker) wait() { <-t.idleChan() }

// raidPostAsync runs one post (a public notice or a warning) on its own goroutine, so the
// handler that started the lockdown goes on at once: the join guard records the joiner who
// tipped the threshold, and the message pipeline reaches aispam, antiflood and the rest,
// without waiting up to raidPostTimeout per send for a pacer slot. The goroutine is counted
// in raidPosts and recovers from a panic, so a failing post can neither crash the bot nor
// be lost to the shutdown unseen.
func raidPostAsync(name string, post func()) {
	raidPosts.start()
	go func() {
		defer raidPosts.finish()
		defer error_handling.RecoverFromPanic(name, "Lockdown")
		post()
	}()
}

// StopRaidPosts waits at most raidPostStopWait for the posts in flight and reports whether
// they all finished. It is a shutdown drain registered after the database close, because a
// post reads the Staff Group link, the log channel and the language from the database. Posts
// still running when the wait ends are cut off with the process (best effort, see
// raidPostStopWait).
func StopRaidPosts() bool {
	select {
	case <-raidPosts.idleChan():
		return true
	case <-time.After(raidPostStopWait):
		log.Warn("[RaidDetection] shutdown drain timed out with posts still in flight")
		return false
	}
}

// raidSetLatch sets the remaining life of the chat's fire latch to d, best effort: the
// latch only sheds load, so a Redis error is ignored and the latch keeps the length it has.
func raidSetLatch(chatID int64, d time.Duration) {
	client := raidRedis()
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	if err := client.PExpire(ctx, raidKey(chatID, raidPartFire), d).Err(); err != nil {
		log.Debugf("[RaidDetection] could not set the fire latch of chat %d to %s: %v", chatID, d, err)
	}
}

// raidOutcomeLatch is the latch length an outcome of startLockdownRow asks for, and its
// name for the log. A zero length keeps the latch's full 60 seconds: a started lockdown,
// one that was already active, and a lock whose answer was lost (the worker settles that
// row). A failure that may pass at once is retried after raidRetryLatch, and a group the
// bot cannot lock waits raidCantLockLatch.
func raidOutcomeLatch(outcome lockdownStartOutcome) (name string, latch time.Duration) {
	switch outcome {
	case lockdownStartStarted:
		return "started", 0
	case lockdownStartAlreadyActive:
		return "already active", 0
	case lockdownStartLockUnknown:
		return "lock unknown", 0
	case lockdownStartBotCheckFailed:
		return "bot check failed", raidRetryLatch
	case lockdownStartPermissionsUnreadable:
		return "permissions unreadable", raidRetryLatch
	case lockdownStartStateFailed:
		return "state failed", raidRetryLatch
	case lockdownStartNotSupergroup:
		return "not a supergroup", raidCantLockLatch
	case lockdownStartBotCannotRestrict:
		return "bot cannot restrict", raidCantLockLatch
	case lockdownStartLockFailed:
		return "lock refused", raidCantLockLatch
	}
	return "unknown", raidRetryLatch
}

// raidTrigger is what fired: the trigger kind (models.LockdownTrigger*) and the numbers
// behind it. Percent is 0 for a join surge.
type raidTrigger struct {
	Kind    string
	Count   int
	Window  int
	Percent int
}

// raidFire starts the lockdown a trigger asks for, through startLockdownRow exactly as
// /lockdown does, with the bot as the starter and the trigger's numbers on the row. The
// start runs synchronously in the caller's handler: a crash half way leaves an unconfirmed
// row the lockdown worker settles on its own. The public notice and the cannot-lock warning
// are not part of it: they are sent from a goroutine (raidPostAsync) after the start, so the
// handler never waits on a send.
//
// The latch (SET NX with a 60 second expiry) only sheds load, so a burst of joins past
// the threshold does not make every join attempt the start; the one active lockdown per
// chat is still decided by lockdown.Start behind uk_chat_lockdowns_active, and the one
// alert by lockdown.BeginAlert, however many triggers and replicas fire at once. A lost
// latch or a Redis error ends the attempt: the next join counts again.
//
// After the latch it stays silent, with no Telegram call, while the chat has an active
// lockdown and for raidLiftQuiet after an admin started lifting one (D-15); a failed read
// of either shortens the latch to raidRetryLatch. A flood trigger then copies its flooding
// accounts into a short-lived snapshot for the alert (raidSnapshotFlooders), and an AI
// burst copies the recent joiners behind its spam verdicts the same way (raidSnapshotAISpammers).
// After the start the latch is cut or stretched by outcome (raidOutcomeLatch).
func raidFire(b *gotgbot.Bot, chat *gotgbot.Chat, trig raidTrigger) {
	client := raidRedis()
	if client == nil {
		return
	}

	latchCtx, cancelLatch := context.WithTimeout(cache.Context, raidRedisTimeout)
	won, err := client.SetNX(latchCtx, raidKey(chat.Id, raidPartFire), "1", raidFireLatch).Result()
	cancelLatch()
	if err != nil {
		log.Debugf("[RaidDetection] fire latch of chat %d failed: %v", chat.Id, err)
		return
	}
	if !won {
		return
	}

	// A lockdown that is active, confirmed or not, already answers this raid: stay silent
	// before any Telegram call, so a trigger that keeps firing costs one database read per
	// latch period. The latch is kept. Exactly one lockdown is still lockdown.Start's
	// partial unique index, this read only saves the calls a loser would make.
	active, err := lockdown.GetActiveFresh(chat.Id)
	if err != nil {
		log.Warnf("[RaidDetection] the lockdown of chat %d could not be read, trying again soon: %v", chat.Id, err)
		raidSetLatch(chat.Id, raidRetryLatch)
		return
	}
	if active != nil {
		return
	}

	// An admin lifted this group's lockdown a moment ago, so the raid they ended is not a
	// new one. lift_started_at is written with real time, so the window is real time too.
	// An unreadable answer is no information: try again soon rather than lock or wait.
	lifted, err := lockdown.RecentlyLiftedFresh(chat.Id, time.Now().Add(-raidLiftQuiet))
	if err != nil {
		log.Warnf("[RaidDetection] the recent lifts of chat %d could not be read, trying again soon: %v", chat.Id, err)
		raidSetLatch(chat.Id, raidRetryLatch)
		return
	}
	if lifted {
		log.Infof("[RaidDetection] trigger %s in chat %d ignored: its lockdown was lifted less than %s ago",
			trig.Kind, chat.Id, raidLiftQuiet)
		return
	}

	// A flood is described by the accounts that flooded, and the sets that know them keep
	// only 30 seconds, so they are copied now, before the lockdown row exists, for the alert
	// the worker decides later (D-17). It is best effort: without the snapshot the alert
	// still lists the tracker's joins.
	// An AI burst is described the same way, by the accounts behind its spam verdicts (the
	// verdict sets keep 120 seconds and expire in minutes).
	switch trig.Kind {
	case models.LockdownTriggerFlood:
		if saved, snapErr := raidSnapshotFlooders(chat.Id); snapErr != nil {
			log.Debugf("[RaidDetection] the flooding accounts of chat %d could not be saved: %v", chat.Id, snapErr)
		} else {
			log.Debugf("[RaidDetection] %d flooding accounts of chat %d saved for the alert", saved, chat.Id)
		}
	case models.LockdownTriggerAIBurst:
		if saved, snapErr := raidSnapshotAISpammers(chat.Id); snapErr != nil {
			log.Debugf("[RaidDetection] the spamming accounts of chat %d could not be saved: %v", chat.Id, snapErr)
		} else {
			log.Debugf("[RaidDetection] %d spamming accounts of chat %d saved for the alert", saved, chat.Id)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), raidFireTimeout)
	defer cancel()
	res := startLockdownRow(ctx, b, lockdownStartParams{
		Chat:           chat,
		TriggerKind:    trig.Kind,
		StarterID:      b.Id,
		StarterName:    lockdownCapRunes(b.FirstName, lockdownNameMaxRunes),
		TriggerCount:   trig.Count,
		TriggerWindow:  trig.Window,
		TriggerPercent: trig.Percent,
	})
	name, latch := raidOutcomeLatch(res.Outcome)
	if latch > 0 {
		raidSetLatch(chat.Id, latch)
	}
	log.Infof("[RaidDetection] trigger %s fired in chat %d (count %d, window %d s, percent %d): %s",
		trig.Kind, chat.Id, trig.Count, trig.Window, trig.Percent, name)

	// Only the call that won the latch and lockdown.Start gets here with Started, so the
	// notice is posted once per lockdown; a lockdown another trigger or an admin started
	// is already answered. A raid the bot cannot stop is reported, at most once a day. The
	// other failures may pass at once and say nothing. A failed post changes nothing about
	// the lockdown or its absence.
	switch res.Outcome {
	case lockdownStartStarted:
		raidPostAsync("raidPostNotice", func() { raidPostNotice(b, chat, trig, res) })
	case lockdownStartNotSupergroup, lockdownStartBotCannotRestrict, lockdownStartLockFailed:
		raidPostAsync("raidWarnCannotLock", func() { raidWarnCannotLock(b, chat, trig, res.Outcome) })
	}
}

// raidSend posts an HTML message to chatID through the lockdown pacer under
// raidPostTimeout, link previews off. It reports the error; the callers only log it.
func raidSend(b *gotgbot.Bot, chatID int64, text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), raidPostTimeout)
	defer cancel()
	return lockdownPaced(ctx, func(callCtx context.Context) error {
		call, cancelCall := context.WithTimeout(callCtx, lockdownCallTimeout)
		defer cancelCall()
		_, sendErr := b.SendMessageWithContext(call, chatID, text, &gotgbot.SendMessageOpts{
			ParseMode:          formatting.HTML,
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true},
		})
		return sendErr
	})
}

// lockdownTriggerSentence is the time-less sentence saying what an automatic trigger saw:
// a join surge names its count and window, a flood its count, window and the percent from
// new members, an AI burst its count of spam verdicts and window. Any other kind, a manual
// lockdown included, has none and gets "". The
// numbers are passed as strings built with strconv and by name only, so the translator's
// legacy printf pass never meets the percent sign, and the sentence holds no user text.
func lockdownTriggerSentence(tr *i18n.Translator, kind string, count, window, percent int) string {
	switch kind {
	case models.LockdownTriggerJoinSurge:
		return lockdownText(tr, "lockdown_trigger_join_surge", i18n.TranslationParams{
			"count":  strconv.Itoa(count),
			"window": strconv.Itoa(window),
		})
	case models.LockdownTriggerFlood:
		return lockdownText(tr, "lockdown_trigger_flood", i18n.TranslationParams{
			"count":   strconv.Itoa(count),
			"window":  strconv.Itoa(window),
			"percent": strconv.Itoa(percent),
		})
	case models.LockdownTriggerAIBurst:
		return lockdownText(tr, "lockdown_trigger_ai_burst", i18n.TranslationParams{
			"count":  strconv.Itoa(count),
			"window": strconv.Itoa(window),
		})
	}
	return ""
}

// raidPostNotice tells the locked group, once, that the bot locked it on its own (a join
// surge, a flood or an AI burst; any other kind posts nothing): which
// trigger fired with its numbers, that approved users are muted too, and the notes
// /lockdown adds when the bot lacks the delete or the invite right (read from res.BotMember,
// the rights startLockdownRow saw). It is written in the group's own language and sent
// through the lockdown pacer under raidPostTimeout. A failed send is logged at warn and
// nothing else happens: the lockdown stands, and the notice is not retried.
func raidPostNotice(b *gotgbot.Bot, chat *gotgbot.Chat, trig raidTrigger, res lockdownStartResult) {
	tr := staffChatTranslator(chat.Id)
	count, window := strconv.Itoa(trig.Count), strconv.Itoa(trig.Window)

	var intro string
	switch trig.Kind {
	case models.LockdownTriggerJoinSurge:
		intro = lockdownText(tr, "lockdown_auto_started_join_surge", i18n.TranslationParams{
			"count":  count,
			"window": window,
		})
	case models.LockdownTriggerFlood:
		intro = lockdownText(tr, "lockdown_auto_started_flood", i18n.TranslationParams{
			"count":   count,
			"window":  window,
			"percent": strconv.Itoa(trig.Percent),
		})
	case models.LockdownTriggerAIBurst:
		intro = lockdownText(tr, "lockdown_auto_started_ai_burst", i18n.TranslationParams{
			"count":  count,
			"window": window,
		})
	default:
		return
	}

	lines := []string{intro, lockdownText(tr, "lockdown_note_approved_muted")}
	// A missing delete or invite right never blocks the lock; it only adds a note.
	if !res.BotMember.CanDeleteMessages {
		lines = append(lines, lockdownText(tr, "lockdown_note_no_delete"))
	}
	if !res.BotMember.CanInviteUsers {
		lines = append(lines, lockdownText(tr, "lockdown_note_no_invite"))
	}
	if err := raidSend(b, chat.Id, lockdownJoinLines(lines)); err != nil {
		log.Warnf("[RaidDetection] the notice of the automatic lockdown in chat %d was not delivered: %v", chat.Id, err)
	}
}

// raidWarnCannotLock tells staff that the bot saw a raid it cannot stop: the group is not a
// supergroup, the bot is not an administrator who can restrict members, or Telegram refused
// the lock. No lockdown exists in any of those cases. One copy goes to the group's Staff
// Group (written in the Staff Group's language) and one to its log channel when the admin
// category is on (in the group's language, starting with the usual log header), each through
// the lockdown pacer.
//
// The destinations are resolved first, and with none the warning is not even attempted and
// nothing is recorded, so a Staff Group linked later is still warned. Then one SET NX with a
// 24 hour expiry gates both copies for the group, across replicas, so a sustained raid warns
// once a day; a Redis error or a lost SetNX sends nothing. The gate is the record of a
// warning that reached somebody: when no copy was delivered it is given back with DEL, so
// the next qualifying trigger tries again, and when at least one copy was delivered it
// stays, so the two copies count as one warning. The failures that may pass at once (bot
// rights or permissions unreadable, a database error, a lock whose answer was lost) are not
// passed in and warn nobody. The group's title is escaped and spliced in after translation.
// A lookup that fails skips that copy, and a failed send is only logged.
func raidWarnCannotLock(b *gotgbot.Bot, chat *gotgbot.Chat, trig raidTrigger, outcome lockdownStartOutcome) {
	var key string
	switch outcome {
	case lockdownStartBotCannotRestrict:
		key = "lockdown_auto_cant_lock_rights"
	case lockdownStartNotSupergroup:
		key = "lockdown_auto_cant_lock_basic"
	case lockdownStartLockFailed:
		key = "lockdown_auto_cant_lock_refused"
	default:
		return
	}

	client := raidRedis()
	if client == nil {
		return
	}

	// Resolve where the warning would go before the gate is touched.
	link, err := staff.GetLinkOfGroupFresh(chat.Id)
	if err != nil {
		log.Warnf("[RaidDetection] the Staff Group link of chat %d could not be read for the warning: %v", chat.Id, err)
		link = nil
	}
	channelID, header, hasChannel := actionlog.Destination(chat, logchannels.CategoryAdmin)
	if link == nil && !hasChannel {
		return
	}

	gateKey := raidKey(chat.Id, raidPartWarn)
	gateCtx, cancelGate := context.WithTimeout(cache.Context, raidRedisTimeout)
	won, err := client.SetNX(gateCtx, gateKey, "1", raidWarnEvery).Result()
	cancelGate()
	if err != nil {
		log.Debugf("[RaidDetection] warning gate of chat %d failed: %v", chat.Id, err)
		return
	}
	if !won {
		return
	}

	group := lockdownGroupLabel(&models.ChatLockdown{
		ChatID:    chat.Id,
		ChatTitle: lockdownCapRunes(chat.Title, lockdownTitleMaxRunes),
	})
	render := func(tr *i18n.Translator) string {
		text := lockdownText(tr, key, i18n.TranslationParams{
			"group":   lockdownGroupToken,
			"trigger": raidTriggerToken,
		})
		return lockdownSplice(text,
			lockdownGroupToken, group,
			raidTriggerToken, lockdownTriggerSentence(tr, trig.Kind, trig.Count, trig.Window, trig.Percent),
		)
	}

	delivered := 0
	if link != nil {
		if err := raidSend(b, link.StaffChatID, render(staffChatTranslator(link.StaffChatID))); err != nil {
			log.Warnf("[RaidDetection] the warning for chat %d was not delivered to its Staff Group %d: %v",
				chat.Id, link.StaffChatID, err)
		} else {
			delivered++
		}
	}
	if hasChannel {
		if err := raidSend(b, channelID, header+render(staffChatTranslator(chat.Id))); err != nil {
			log.Warnf("[RaidDetection] the warning for chat %d was not delivered to its log channel %d: %v",
				chat.Id, channelID, err)
		} else {
			delivered++
		}
	}

	if delivered == 0 {
		raidReleaseWarnGate(client, chat.Id, gateKey)
	}
}

// raidReleaseWarnGate gives the once-a-day warning gate back after a warning nobody
// received, so the next qualifying trigger warns again instead of staying silent for a day.
// Best effort: a Redis error leaves the gate, which only delays the next warning.
func raidReleaseWarnGate(client *redis.Client, chatID int64, gateKey string) {
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	if err := client.Del(ctx, gateKey).Err(); err != nil {
		log.Warnf("[RaidDetection] the warning gate of chat %d could not be given back after a warning nobody received: %v",
			chatID, err)
	}
}
