package modules

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// The name check's refusal latch (security SEC-02). Some states stop every provider call for a while
// yet are invisible to the watcher: a spent hourly allowance of the group or of the fleet, a gate
// that stayed full and a counter that failed. Each is a sticky unknown, an unknown writes nothing,
// so a member who still has a join marker is never "satisfied" and, without this latch, each of
// their messages would be a runner job and a paced live getChatMember that ends in nothing, a cost
// an attacker can cause (20 accounts spend one group's 100 units). The AI row sets the latch when
// it is refused for one of those reasons, and the watcher reads it exactly as it reads the breaker
// (memberReqAIOnlyPaused), so while it holds the watcher starts no job and runs no clean path.
//
// The latch is in memory, per replica and per chat, like the breaker's pause; a refusal on another
// replica latches that replica when it meets the same refusal, which costs one job per chat per
// replica. It belongs to the name check alone, in its own type, and every time comes from raidNow
// so a test moves it. An account's own budget never sets it: that is one member's limit and the
// watcher cannot key on it (accepted, AR-18).

// memberReqAILatchCooldown is how long a chat stays latched after a full gate or a failed budget
// counter, both faults of the moment. A spent group or fleet allowance latches until the end of the
// UTC hour instead.
const memberReqAILatchCooldown = 60 * time.Second

// memberReqAILatchMaxChats bounds the number of chats the latch tracks.
const memberReqAILatchMaxChats = 10000

// memberReqAILatch is the latch's state: the latched chats with the time each ends (a bounded map)
// and whether the map-full warning was said.
type memberReqAILatch struct {
	mu         sync.Mutex
	till       map[int64]time.Time
	fullWarned bool
}

// newMemberReqAILatch returns a latch that holds nothing.
func newMemberReqAILatch() *memberReqAILatch {
	return &memberReqAILatch{till: make(map[int64]time.Time)}
}

// memberReqAILatches is the process's latch. A variable only so a test can install an empty one;
// production code never reassigns it.
var memberReqAILatches = newMemberReqAILatch()

// latched reports whether the chat is latched at now. An ended entry is forgotten.
func (l *memberReqAILatch) latched(chatID int64, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	end, ok := l.till[chatID]
	if !ok {
		return false
	}
	if !now.Before(end) {
		delete(l.till, chatID)
		return false
	}
	return true
}

// pruneEnded drops the chats whose latch has ended. The caller holds l.mu.
func (l *memberReqAILatch) pruneEnded(now time.Time) {
	for chatID, end := range l.till {
		if !now.Before(end) {
			delete(l.till, chatID)
		}
	}
}

// hold latches the chat until end (never shortening a longer latch). A full map is pruned of ended
// latches first, and when every tracked chat is still latched the new one stays untracked, so it
// can only cost the jobs it cost before; warn is true once per latch, for the first chat that could
// not be tracked.
func (l *memberReqAILatch) hold(chatID int64, now, end time.Time) (warn bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if current, ok := l.till[chatID]; ok {
		if end.After(current) {
			l.till[chatID] = end
		}
		return false
	}
	if len(l.till) >= memberReqAILatchMaxChats {
		l.pruneEnded(now)
	}
	if len(l.till) >= memberReqAILatchMaxChats {
		warn = !l.fullWarned
		l.fullWarned = true
		return warn
	}
	l.till[chatID] = end
	return false
}

// memberReqAILatched reports whether the chat's name checks are latched now by a refusal the
// watcher cannot see. In memory and pure, like memberReqAIPaused.
func memberReqAILatched(chatID int64) bool {
	return memberReqAILatches.latched(chatID, raidNow())
}

// memberReqAILatchChat latches the chat until end and logs the one warning when the map is full.
func memberReqAILatchChat(chatID int64, end time.Time) {
	if memberReqAILatches.hold(chatID, raidNow(), end) {
		log.Warnf("[MemberReq] the AI name check latch map is full (%d chats); further chats are not tracked and keep costing a job per message",
			memberReqAILatchMaxChats)
	}
}

// memberReqAILatchHour latches the chat until the end of the current UTC hour: the group's or the
// fleet's allowance is spent and does not come back before then.
func memberReqAILatchHour(chatID int64) {
	memberReqAILatchChat(chatID, raidNow().UTC().Truncate(time.Hour).Add(time.Hour))
}

// memberReqAILatchBriefly latches the chat for memberReqAILatchCooldown: a full gate or a failed
// counter is a fault of the moment, so the watcher looks again after a minute.
func memberReqAILatchBriefly(chatID int64) {
	memberReqAILatchChat(chatID, raidNow().Add(memberReqAILatchCooldown))
}
