//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"fmt"
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
	times   []time.Time
}

func newMreqLookupState() *mreqLookupState {
	return &mreqLookupState{
		answers: make(map[int64]mreqPhotoAnswer),
		errs:    make(map[int64][]error),
		gates:   make(map[int64]chan struct{}),
	}
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
func (s *mreqLookupState) arrivals() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.times...)
}

// begin records an arrival and returns what the request must do: the gate to wait on, the
// answer and a scripted error.
func (s *mreqLookupState) begin(userID int64) (gate chan struct{}, answer mreqPhotoAnswer, scripted error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.times = append(s.times, time.Now())
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
	if method != "getUserProfilePhotos" {
		return f.lockdownFake.RequestWithContext(ctx, token, method, params, opts)
	}
	f.record(method, params)
	gate, answer, scripted := f.state.begin(staffParamInt(params, "user_id"))
	if scripted != nil {
		return nil, scripted
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
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
