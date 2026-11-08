//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/cache"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

// mreqPhotoAnswer is what the lookup fake answers for one user's getUserProfilePhotos. The
// zero value is a member with no photo: total 0 and an empty list.
type mreqPhotoAnswer struct {
	// Total is total_count.
	Total int64
	// List adds one photo to the list, whatever Total says.
	List bool
	// Null answers a body of null.
	Null bool
}

// mreqLookupState is the state of the lookup fake. Two fakes (two replicas) share one state,
// so a test can count the requests that reached "Telegram" across both and read their
// arrival times as one timeline.
type mreqLookupState struct {
	mu      sync.Mutex
	answers map[int64]mreqPhotoAnswer
	errs    map[int64][]error
	gates   map[int64]chan struct{}
	times   []mreqArrival

	// The channel half of the fake. links is the linked_chat_id getChat reports for a group
	// (0 for none); a group in it is answered by the fake, every other getChat goes to the
	// wrapped fake. members is the getChatMember answer per (chat, user), the bot's own
	// status included (user staffTestBotID). chanErrs are scripted errors and chanGates
	// are holds, both keyed by mreqChannelKey.
	links     map[int64]int64
	nullChat  map[int64]bool
	members   map[[2]int64]mreqChannelMember
	chanErrs  map[string][]error
	chanGates map[string]chan struct{}
}

// mreqChannelMember is what the fake answers for getChatMember of one (chat, user).
type mreqChannelMember struct {
	// Status is the answer's status: creator, administrator, member, restricted, left,
	// kicked, or any other text for an unrecognised type.
	Status string
	// IsMember is is_member of a restricted answer.
	IsMember bool
	// Null answers a body of null.
	Null bool
}

// mreqArrival is one request that reached the fake: who it asked about and when.
type mreqArrival struct {
	User int64
	At   time.Time
}

func newMreqLookupState() *mreqLookupState {
	return &mreqLookupState{
		answers: make(map[int64]mreqPhotoAnswer),
		errs:    make(map[int64][]error),
		gates:   make(map[int64]chan struct{}),

		links:     make(map[int64]int64),
		nullChat:  make(map[int64]bool),
		members:   make(map[[2]int64]mreqChannelMember),
		chanErrs:  make(map[string][]error),
		chanGates: make(map[string]chan struct{}),
	}
}

// mreqChannelKey names one fake call for scripting and holding: a getChat of chat (user 0)
// or a getChatMember of user in chat.
func mreqChannelKey(method string, chat, user int64) string {
	return fmt.Sprintf("%s:%d:%d", method, chat, user)
}

// link makes getChat of group report channel as its linked chat; 0 reports none.
func (s *mreqLookupState) link(group, channel int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[group] = channel
}

// nullGetChat makes getChat of group answer a body of null.
func (s *mreqLookupState) nullGetChat(group int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nullChat[group] = true
}

// setMember sets the getChatMember answer for user in chat.
func (s *mreqLookupState) setMember(chat, user int64, answer mreqChannelMember) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members[[2]int64{chat, user}] = answer
}

// scriptChat queues errors for the next getChat requests of group.
func (s *mreqLookupState) scriptChat(group int64, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := mreqChannelKey("getChat", group, 0)
	s.chanErrs[key] = append(s.chanErrs[key], errs...)
}

// scriptMember queues errors for the next getChatMember requests of user in chat.
func (s *mreqLookupState) scriptMember(chat, user int64, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := mreqChannelKey("getChatMember", chat, user)
	s.chanErrs[key] = append(s.chanErrs[key], errs...)
}

// holdKey makes every request named by key wait, honouring its context, until the returned
// function runs.
func (s *mreqLookupState) holdKey(key string) (release func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	s.chanGates[key] = gate
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

// mreqChannelStep is what a channel-half request must do.
type mreqChannelStep struct {
	// Handled is false when the request is not for a chat of this fake.
	Handled bool
	// Gate is the hold to wait on, nil for none.
	Gate chan struct{}
	// Err is a scripted error, returned instead of an answer.
	Err error
	// Member is the getChatMember answer.
	Member mreqChannelMember
	// Linked is the linked_chat_id of a getChat answer, Null a body of null.
	Linked int64
	Null   bool
}

// beginChannel decides what a getChat of chat or a getChatMember of user in chat does.
func (s *mreqLookupState) beginChannel(method string, chat, user int64) mreqChannelStep {
	s.mu.Lock()
	defer s.mu.Unlock()
	step := mreqChannelStep{}
	if method == "getChat" {
		linked, ok := s.links[chat]
		if !ok {
			return step
		}
		step.Linked, step.Null = linked, s.nullChat[chat]
	} else {
		member, ok := s.members[[2]int64{chat, user}]
		if !ok {
			// A chat the fake knows no member of is not a channel of this fake.
			for k := range s.members {
				if k[0] == chat {
					ok = true
					break
				}
			}
			if !ok {
				return step
			}
			member = mreqChannelMember{Status: "left"}
		}
		step.Member = member
	}
	step.Handled = true
	key := mreqChannelKey(method, chat, user)
	if queue := s.chanErrs[key]; len(queue) > 0 {
		s.chanErrs[key] = queue[1:]
		step.Err = queue[0]
		return step
	}
	step.Gate = s.chanGates[key]
	return step
}

// set stores the answer for userID.
func (s *mreqLookupState) set(userID int64, answer mreqPhotoAnswer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[userID] = answer
}

// script queues errors that the next requests for userID return, one each, before any answer.
func (s *mreqLookupState) script(userID int64, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errs[userID] = append(s.errs[userID], errs...)
}

// hold makes every request for userID wait, honouring the request's context, until the
// returned release function runs (it is safe to call more than once).
func (s *mreqLookupState) hold(userID int64) (release func()) {
	gate := make(chan struct{})
	s.mu.Lock()
	s.gates[userID] = gate
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { close(gate) }) }
}

// arrivals returns when each request arrived, in order.
func (s *mreqLookupState) arrivals() []mreqArrival {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]mreqArrival(nil), s.times...)
}

// begin records an arrival and returns what the request must do: the gate to wait on, the
// answer and a scripted error.
func (s *mreqLookupState) begin(userID int64) (gate chan struct{}, answer mreqPhotoAnswer, scripted error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.times = append(s.times, mreqArrival{User: userID, At: time.Now()})
	if queue := s.errs[userID]; len(queue) > 0 {
		s.errs[userID] = queue[1:]
		return nil, mreqPhotoAnswer{}, queue[0]
	}
	return s.gates[userID], s.answers[userID], nil
}

// mreqLookupFake wraps the shared lockdown fake and answers getUserProfilePhotos from a
// mreqLookupState. Every other method is the wrapped fake's, and every photo request is
// recorded through it, so callsFor("getUserProfilePhotos") counts them.
type mreqLookupFake struct {
	*lockdownFake
	state *mreqLookupState
}

func (f *mreqLookupFake) RequestWithContext(
	ctx context.Context,
	token, method string,
	params map[string]any,
	opts *gotgbot.RequestOpts,
) (json.RawMessage, error) {
	switch method {
	case "getUserProfilePhotos":
		return f.photos(ctx, method, params)
	case "getChat", "getChatMember":
		chat := staffParamInt(params, "chat_id")
		user := staffParamInt(params, "user_id")
		step := f.state.beginChannel(method, chat, user)
		if !step.Handled {
			return f.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
		}
		f.record(method, params)
		if step.Err != nil {
			return nil, step.Err
		}
		if err := mreqAwaitGate(ctx, step.Gate); err != nil {
			return nil, err
		}
		return mreqChannelAnswer(method, chat, user, step), nil
	}
	return f.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
}

// mreqAwaitGate waits for gate to close or ctx to end; a nil gate does not wait.
func mreqAwaitGate(ctx context.Context, gate chan struct{}) error {
	if gate == nil {
		return nil
	}
	select {
	case <-gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// mreqChannelAnswer builds the JSON the fake answers a channel-half request with.
func mreqChannelAnswer(method string, chat, user int64, step mreqChannelStep) json.RawMessage {
	if method == "getChat" {
		if step.Null {
			return json.RawMessage(`null`)
		}
		if step.Linked == 0 {
			return json.RawMessage(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Test Chat"}`, chat))
		}
		return json.RawMessage(fmt.Sprintf(`{"id":%d,"type":"supergroup","title":"Test Chat","linked_chat_id":%d}`, chat, step.Linked))
	}
	member := step.Member
	if member.Null {
		return json.RawMessage(`null`)
	}
	var extra strings.Builder
	if member.Status == gotgbot.ChatMemberStatusRestricted {
		fmt.Fprintf(&extra, `,"is_member":%t`, member.IsMember)
	}
	return json.RawMessage(fmt.Sprintf(`{"status":%q,"user":{"id":%d,"is_bot":false,"first_name":"U"}%s}`, member.Status, user, extra.String()))
}

// photos answers getUserProfilePhotos from the photo answers.
func (f *mreqLookupFake) photos(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	f.record(method, params)
	gate, answer, scripted := f.state.begin(staffParamInt(params, "user_id"))
	if scripted != nil {
		return nil, scripted
	}
	if err := mreqAwaitGate(ctx, gate); err != nil {
		return nil, err
	}
	if answer.Null {
		return json.RawMessage(`null`), nil
	}
	photos := `[]`
	if answer.List {
		photos = `[[{"file_id":"f","file_unique_id":"u","width":1,"height":1}]]`
	}
	return json.RawMessage(fmt.Sprintf(`{"total_count":%d,"photos":%s}`, answer.Total, photos)), nil
}

// mreqInstallLookupFake makes the environment's bot answer getUserProfilePhotos from state.
func mreqInstallLookupFake(e *memberReqEnv, state *mreqLookupState) *mreqLookupFake {
	e.t.Helper()
	wrapper := &mreqLookupFake{lockdownFake: e.fake, state: state}
	e.bot.BotClient = wrapper
	return wrapper
}

// mreqLookupPhotoCalls counts the getUserProfilePhotos requests the environment's fake saw.
func mreqLookupPhotoCalls(e *memberReqEnv) int {
	return len(e.fake.callsFor("getUserProfilePhotos"))
}

// mreqChannel links a fresh channel to group in the fake, gives the bot botStatus there
// (administrator, member, left, kicked, restricted) and registers the channel ID, so the
// sub: and ba: keys it makes are deleted when the test ends. It returns the channel ID.
func mreqChannel(t *testing.T, state *mreqLookupState, group int64, botStatus string) int64 {
	t.Helper()
	channel := uniqueModuleChatID()
	state.link(group, channel)
	state.setMember(channel, staffTestBotID, mreqChannelMember{Status: botStatus})
	mreqLookupCleanup(t, nil, []int64{channel, group})
	return channel
}

// mreqChatCalls counts the getChat requests the environment's fake saw for chat.
func mreqChatCalls(e *memberReqEnv, chat int64) int {
	n := 0
	for _, call := range e.fake.callsFor("getChat") {
		if staffParamInt(call.Params, "chat_id") == chat {
			n++
		}
	}
	return n
}

// mreqMemberCalls counts the getChatMember requests the fake saw for user in chat; the bot's
// own lookup is user staffTestBotID.
func mreqMemberCalls(e *memberReqEnv, chat, user int64) int {
	n := 0
	for _, call := range e.fake.callsFor("getChatMember") {
		if staffParamInt(call.Params, "chat_id") == chat && staffParamInt(call.Params, "user_id") == user {
			n++
		}
	}
	return n
}

// withFastLookupPacer installs a lookup pacer built from the production options with a 1 ms
// interval and a 5 ms retry_after unit for the test, and puts the previous one back.
func withFastLookupPacer(t *testing.T) {
	t.Helper()
	previous := memberReqLookupPacer
	opts := memberReqLookupPacerOptions
	opts.Interval = time.Millisecond
	opts.RetryAfterUnit = 5 * time.Millisecond
	memberReqLookupPacer = ratelimit.NewTelegramPacer(opts)
	t.Cleanup(func() { memberReqLookupPacer = previous })
}

// mreqLookupCleanup deletes what a lookup test wrote: the fuku:memberreq:* keys that name a
// channel (registered as a chat ID, so sub: and lc: keys go) and the users rows and cache
// keys of userIDs (through memberReqCleanup), and fuku:memberreq:photo:<user> for every
// user, which holds only a user ID.
func mreqLookupCleanup(t *testing.T, userIDs, channelIDs []int64) {
	t.Helper()
	memberReqCleanup(t, channelIDs, userIDs)
	t.Cleanup(func() {
		_, _, client := cache.GetCacheState()
		if client == nil {
			return
		}
		for _, id := range userIDs {
			client.Del(context.Background(), memberReqPhotoKey(id))
		}
	})
}
