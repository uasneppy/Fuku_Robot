package captchaweb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	maxBodyBytes           = 16 << 10
	maxInitDataBytes       = 4096
	maxTurnstileTokenBytes = 2048

	// passWorkTimeout bounds the work after a pass is confirmed. It runs detached from the
	// request, so a client that disconnects never abandons an unmute.
	passWorkTimeout = 20 * time.Second
	// verifyWriteDeadline replaces the server's 10 s write timeout, which is shorter than a
	// siteverify call plus the Telegram calls of a pass.
	verifyWriteDeadline = 20 * time.Second
)

// Result is the decided answer of the verify route.
type Result string

// The answers of the verify route. The page shows what the server says and decides nothing.
const (
	ResultPassed    Result = "passed"
	ResultFailed    Result = "failed"
	ResultExpired   Result = "expired"
	ResultInvalid   Result = "invalid"
	ResultNotForYou Result = "not_for_you"
	ResultRetry     Result = "retry"
)

// Answer is the JSON body of every verify response. Action (kick, ban or mute) is set only
// on failed and expired, so the page can say whether the member can rejoin. No answer
// carries an error text, an ID or a code.
type Answer struct {
	Result Result `json:"result"`
	Action string `json:"action,omitempty"`
}

// AttemptStatus is what the Backend knows about a challenge's attempt.
type AttemptStatus struct {
	// Live is true while the attempt row exists, belongs to the challenge, is a Turnstile
	// attempt and has not expired.
	Live bool
	// Action is the group's failure action: kick, ban or mute.
	Action string
}

// PassResult is the outcome of Backend.Pass.
type PassResult int

const (
	// PassLost means the attempt was gone or claimed by something else (a timeout, a leave,
	// a re-join); nobody was unmuted. It is the zero value.
	PassLost PassResult = iota
	// PassOK means the attempt was claimed and the member unmuted.
	PassOK
	// PassUnmuteFailed means the attempt was claimed but the unmute call failed; a durable
	// retry is scheduled by the Backend.
	PassUnmuteFailed
)

// Backend is the bot side of the web captcha: the attempt row, the claim, the unmute and the
// greeting. alita/modules implements it, so this package never imports a package of the
// module.
type Backend interface {
	// Attempt reports, from a fresh read, whether the record's attempt can still be passed.
	Attempt(ctx context.Context, rec Record) (AttemptStatus, error)
	// Pass claims the attempt, unmutes the member and finishes the pass exactly as the
	// button pass does. It is called only after the signed user and Cloudflare confirmed.
	Pass(ctx context.Context, rec Record, user InitData) (PassResult, error)
}

// Options builds a Handler.
type Options struct {
	// BotToken verifies the signed initData.
	BotToken string
	// PublicBaseURL is the https address of the page; its host is the hostname siteverify
	// must echo.
	PublicBaseURL string
	Store         *Store
	Siteverify    *Client
	Backend       Backend
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Handler serves the verify route.
type Handler struct {
	botToken   string
	hostname   string
	store      *Store
	siteverify *Client
	backend    Backend
	now        func() time.Time
}

// ErrBadOptions means NewHandler refused its options.
var ErrBadOptions = errors.New("captchaweb: bad handler options")

// NewHandler builds a Handler. It rejects an empty token, a base URL that is not https with
// a host, and a nil Store, Siteverify or Backend.
func NewHandler(opts Options) (*Handler, error) {
	if opts.BotToken == "" || opts.Store == nil || opts.Siteverify == nil || opts.Backend == nil {
		return nil, ErrBadOptions
	}
	base, err := url.Parse(opts.PublicBaseURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		return nil, ErrBadOptions
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Handler{
		botToken:   opts.BotToken,
		hostname:   strings.ToLower(base.Hostname()),
		store:      opts.Store,
		siteverify: opts.Siteverify,
		backend:    opts.Backend,
		now:        now,
	}, nil
}

type verifyRequest struct {
	InitData       string `json:"init_data"`
	TurnstileToken string `json:"turnstile_token"`
	Challenge      string `json:"challenge"`
}

// writeAnswer is the one place a response is written: it sets the three headers every
// answer carries and the JSON body, and nothing else.
func writeAnswer(w http.ResponseWriter, status int, answer Answer) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		log.Debugf("[CaptchaWeb][Verify] write failed: %v", err)
	}
}

func decodeVerifyRequest(w http.ResponseWriter, r *http.Request) (verifyRequest, int, bool) {
	var req verifyRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return req, http.StatusRequestEntityTooLarge, false
		}
		return req, http.StatusBadRequest, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return req, http.StatusBadRequest, false
	}
	if req.InitData == "" || len(req.InitData) > maxInitDataBytes ||
		req.TurnstileToken == "" || len(req.TurnstileToken) > maxTurnstileTokenBytes ||
		!ValidToken(req.Challenge) {
		return req, http.StatusBadRequest, false
	}
	return req, http.StatusOK, true
}

// Verify decides one pass attempt. The page only forwards initData, the Turnstile token and
// the challenge, and shows the answer: a pass needs Telegram's signed initData whose user is
// the challenge's user, and Cloudflare's confirmation for the expected hostname, action and
// cData. Nothing the body says is trusted on its own.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAnswer(w, http.StatusMethodNotAllowed, Answer{Result: ResultInvalid})
		return
	}
	// The server's write timeout is shorter than a siteverify call plus a pass. The error is
	// ignored: a recorder or a wrapped writer may not support deadlines.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(verifyWriteDeadline))

	req, status, ok := decodeVerifyRequest(w, r)
	if !ok {
		writeAnswer(w, status, Answer{Result: ResultInvalid})
		return
	}

	user, err := ValidateInitData(req.InitData, h.botToken, h.now())
	if err != nil {
		writeAnswer(w, http.StatusOK, Answer{Result: ResultInvalid})
		return
	}
	// A signed start_param is authoritative: the challenge the page posts must be it.
	if user.StartParam != "" && user.StartParam != req.Challenge {
		writeAnswer(w, http.StatusOK, Answer{Result: ResultInvalid})
		return
	}

	consumed, rec, _, err := h.store.ConsumeIfUser(r.Context(), req.Challenge, user.UserID)
	if err != nil {
		log.WithField("user_id", user.UserID).Warn("[CaptchaWeb][Verify] challenge store failed")
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	switch consumed {
	case ConsumeUnknown:
		writeAnswer(w, http.StatusOK, Answer{Result: ResultInvalid})
		return
	case ConsumeWrongUser:
		writeAnswer(w, http.StatusOK, Answer{Result: ResultNotForYou})
		return
	}
	fields := log.Fields{"chat_id": rec.ChatID, "user_id": rec.UserID, "attempt_id": rec.AttemptID}

	attempt, err := h.backend.Attempt(r.Context(), rec)
	if err != nil {
		log.WithFields(fields).Warn("[CaptchaWeb][Verify] attempt lookup failed")
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	if !attempt.Live {
		log.WithFields(fields).Info("[CaptchaWeb][Verify] outcome=expired")
		writeAnswer(w, http.StatusOK, Answer{Result: ResultExpired, Action: attempt.Action})
		return
	}

	verdict := h.siteverify.Verify(r.Context(), req.TurnstileToken, Expect{
		Hostname: h.hostname,
		Action:   TurnstileAction,
		CData:    CData(req.Challenge),
	})
	if verdict.Outcome != OutcomePass {
		// Plan 08-06 classifies a failed solve and re-arms the challenge; until then every
		// other answer is a retry that changes nothing.
		log.WithFields(fields).Infof("[CaptchaWeb][Verify] outcome=retry reason=%s", verdict.Reason)
		writeAnswer(w, http.StatusOK, Answer{Result: ResultRetry})
		return
	}

	workCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), passWorkTimeout)
	defer cancel()
	passed, err := h.backend.Pass(workCtx, rec, user)
	if err != nil {
		log.WithFields(fields).Warn("[CaptchaWeb][Verify] pass failed")
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	switch passed {
	case PassOK:
		log.WithFields(fields).Info("[CaptchaWeb][Verify] outcome=passed")
		writeAnswer(w, http.StatusOK, Answer{Result: ResultPassed})
	case PassLost:
		log.WithFields(fields).Info("[CaptchaWeb][Verify] outcome=expired")
		writeAnswer(w, http.StatusOK, Answer{Result: ResultExpired, Action: attempt.Action})
	default:
		log.WithFields(fields).Warn("[CaptchaWeb][Verify] outcome=retry reason=unmute")
		writeAnswer(w, http.StatusOK, Answer{Result: ResultRetry})
	}
}
