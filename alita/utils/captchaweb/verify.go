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

	// passWorkTimeout bounds the work after a Cloudflare answer is in: a pass, a failed solve
	// or an expiry. It runs detached from the request, so a client that disconnects never
	// abandons an unmute or a kick.
	passWorkTimeout = 20 * time.Second
	// verifyWriteDeadline replaces the server's 10 s write timeout, which is shorter than a
	// siteverify call plus the Telegram calls of a pass.
	verifyWriteDeadline = 20 * time.Second
	// botRightsTimeout bounds the live check of the bot's own Restrict right that runs
	// before any Cloudflare call.
	botRightsTimeout = 3 * time.Second
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
//
// Contract for every method that returns an error: it returns an error only when it changed
// nothing (no claim, no Telegram write), so the handler may give the challenge back and the
// member may try again. A method that changed something reports it through its result.
type Backend interface {
	// BotCanRestrict reports, from a live read bounded by ctx, whether the bot is the creator
	// or an administrator with the Restrict right in chatID. An unknown answer is an error.
	BotCanRestrict(ctx context.Context, chatID int64) (bool, error)
	// Attempt reports, from a fresh read, whether the record's attempt can still be passed.
	Attempt(ctx context.Context, rec Record) (AttemptStatus, error)
	// Pass claims the attempt, unmutes the member and finishes the pass exactly as the
	// button pass does. It is called only after the signed user and Cloudflare confirmed.
	Pass(ctx context.Context, rec Record, user InitData) (PassResult, error)
	// Fail applies the group's failure action to a member whose solve provably failed, once,
	// through the same function the timer uses. claimed is true when this call won the
	// conditional delete of the attempt row (and so applied, or tried to apply, the action);
	// false means the attempt was already decided by someone else.
	Fail(ctx context.Context, rec Record) (claimed bool, err error)
	// Expire applies the group's failure action to an attempt whose time is up and that
	// nobody has decided yet. It does nothing when the row is gone or still live, because the
	// timer owns a live row.
	Expire(ctx context.Context, rec Record) error
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
//
// Every outcome is decided exactly once, by the Backend's conditional delete of the attempt
// row: a pass, a failed solve and a timeout race on it, on any replica (D-16). The challenge
// is given back (Store.Rearm) only where the member can still try again: a Cloudflare,
// network or configuration fault, a bot that cannot restrict, or a Backend error before
// anything was claimed (D-21).
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

	consumedAt := time.Now()
	consumed, rec, left, err := h.store.ConsumeIfUser(r.Context(), req.Challenge, user.UserID)
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

	run := &challengeRun{
		h:          h,
		r:          r,
		w:          w,
		user:       user,
		token:      req.Challenge,
		turnstile:  req.TurnstileToken,
		rec:        rec,
		left:       left,
		consumedAt: consumedAt,
		fields:     log.Fields{"chat_id": rec.ChatID, "user_id": rec.UserID, "attempt_id": rec.AttemptID},
	}
	run.decide()
}

// challengeRun is one consumed challenge being decided. Each step either answers or hands on,
// and the challenge is given back (retry) only where the member can still try again.
type challengeRun struct {
	h          *Handler
	r          *http.Request
	w          http.ResponseWriter
	user       InitData
	token      string
	turnstile  string
	rec        Record
	left       time.Duration
	consumedAt time.Time
	fields     log.Fields
	// action is the group's failure action, set once the attempt was read.
	action string
}

// work is the context of the work that must not be abandoned when the page disconnects: the
// request's values, no cancellation, its own deadline.
func (c *challengeRun) work() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.r.Context()), passWorkTimeout)
}

// rearm puts the challenge back with the lifetime it had left when it was consumed minus the
// time since, so a retry never stretches the challenge (D-21). A failure only costs the
// member their second try: the attempt's own timeout still applies.
func (c *challengeRun) rearm() {
	remaining := c.left - time.Since(c.consumedAt)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.r.Context()), 2*RedisTimeout)
	defer cancel()
	if _, err := c.h.store.Rearm(ctx, c.token, c.rec, remaining); err != nil {
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] re-arm failed")
	}
}

// retry gives the challenge back and answers retry with status.
func (c *challengeRun) retry(status int) {
	c.rearm()
	writeAnswer(c.w, status, Answer{Result: ResultRetry})
}

// expire applies the action to an attempt whose time is up or whose claim was lost, then
// answers expired with the action. It never re-arms.
func (c *challengeRun) expire() {
	ctx, cancel := c.work()
	defer cancel()
	if err := c.h.backend.Expire(ctx, c.rec); err != nil {
		// Nothing changed; the timer and the cleanup tick still own the row.
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] expire failed")
	}
	log.WithFields(c.fields).Info("[CaptchaWeb][Verify] outcome=expired")
	writeAnswer(c.w, http.StatusOK, Answer{Result: ResultExpired, Action: c.action})
}

func (c *challengeRun) decide() {
	rightsCtx, cancelRights := context.WithTimeout(c.r.Context(), botRightsTimeout)
	canRestrict, err := c.h.backend.BotCanRestrict(rightsCtx, c.rec.ChatID)
	cancelRights()
	if err != nil || !canRestrict {
		// The failure action could not be applied, so no Cloudflare call is made and the
		// member keeps their attempt and their challenge.
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] outcome=retry reason=bot_rights")
		c.retry(http.StatusOK)
		return
	}

	attempt, err := c.h.backend.Attempt(c.r.Context(), c.rec)
	if err != nil {
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] attempt lookup failed")
		c.retry(http.StatusServiceUnavailable)
		return
	}
	c.action = attempt.Action
	if !attempt.Live {
		c.expire()
		return
	}

	verdict := c.h.siteverify.Verify(c.r.Context(), c.turnstile, Expect{
		Hostname: c.h.hostname,
		Action:   TurnstileAction,
		CData:    CData(c.token),
	})
	switch verdict.Outcome {
	case OutcomePass:
		c.pass()
	case OutcomeFailed:
		c.fail(verdict.Reason)
	default:
		if verdict.ConfigError() {
			log.Errorf("[CaptchaWeb] siteverify configuration fault: %s", verdict.Reason)
		}
		log.WithFields(c.fields).Infof("[CaptchaWeb][Verify] outcome=retry reason=%s", verdict.Reason)
		c.retry(http.StatusOK)
	}
}

func (c *challengeRun) pass() {
	ctx, cancel := c.work()
	defer cancel()
	passed, err := c.h.backend.Pass(ctx, c.rec, c.user)
	if err != nil {
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] pass failed")
		c.retry(http.StatusServiceUnavailable)
		return
	}
	switch passed {
	case PassOK:
		log.WithFields(c.fields).Info("[CaptchaWeb][Verify] outcome=passed")
		writeAnswer(c.w, http.StatusOK, Answer{Result: ResultPassed})
	case PassLost:
		c.expire()
	default:
		// Claimed, but the unmute failed: the Backend left a durable retry, so the challenge
		// is not given back.
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] outcome=retry reason=unmute")
		writeAnswer(c.w, http.StatusOK, Answer{Result: ResultRetry})
	}
}

func (c *challengeRun) fail(reason Reason) {
	ctx, cancel := c.work()
	claimed, err := c.h.backend.Fail(ctx, c.rec)
	cancel()
	if err != nil {
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] fail failed")
		c.retry(http.StatusServiceUnavailable)
		return
	}
	if !claimed {
		// Lost the conditional delete to a timer or another replica.
		c.expire()
		return
	}
	log.WithFields(c.fields).Infof("[CaptchaWeb][Verify] outcome=failed reason=%s", reason)
	writeAnswer(c.w, http.StatusOK, Answer{Result: ResultFailed, Action: c.action})
}
