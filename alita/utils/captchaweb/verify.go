package captchaweb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
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
	// verifyMaxInFlight is how many verify requests are served at once on one replica; the
	// next one is answered 503 retry without touching Redis or the backend.
	verifyMaxInFlight = 64
)

// The route patterns (Go 1.22 method patterns), mounted by the HTTP server. A wrong method
// on the verify path gets 405 from the mux, HEAD on the page works, and /captcha/ is a 404.
const (
	RoutePage   = "GET /captcha"
	RouteVerify = "POST /captcha/verify"
	RouteHealth = "GET /captcha/health"
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

// ActionResult is what Backend.Fail and Backend.Expire report about the group's failure action.
type ActionResult int

const (
	// ActionLost means this call changed nothing: the attempt was already decided by someone
	// else (a timer, another replica, a pass), or its row is gone. The decider applied the
	// action, so the answer still names it. It is the zero value.
	ActionLost ActionResult = iota
	// ActionApplied means this call claimed the attempt and the group's action took effect.
	ActionApplied
	// ActionNotApplied means this call claimed the attempt but the group's action could not be
	// applied (Telegram refused it): the member was unmuted and is still in the group, so the
	// answer must not say they were removed.
	ActionNotApplied
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
	// through the same function the timer uses. The result says whether this call won the
	// conditional delete of the attempt row and whether the action took effect. An unclaimed
	// Fail with the attempt still pending is an error, because nothing was decided.
	Fail(ctx context.Context, rec Record) (ActionResult, error)
	// Expire applies the group's failure action to an attempt whose time is up and that
	// nobody has decided yet. It does nothing (ActionLost) when the row is gone or still live,
	// because the timer owns a live row.
	Expire(ctx context.Context, rec Record) (ActionResult, error)
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
	// TrustedProxies are the proxies whose X-Forwarded-For is believed
	// (CAPTCHA_WEB_TRUSTED_PROXIES). With none, the client address is RemoteAddr.
	TrustedProxies []netip.Prefix
	// Limiter counts the abuse windows; nil builds one on the Store's Redis client.
	Limiter *Limiter
	// StartNotReady builds the handler not ready: verify answers 503 retry and the health
	// route reports unavailable until SetReady(true) (D-30). The zero value is ready at once.
	StartNotReady bool
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
	limiter    *Limiter
	trusted    []netip.Prefix
	now        func() time.Time
	ready      atomic.Bool
	inflight   chan struct{}
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
	limiter := opts.Limiter
	if limiter == nil {
		limiter = NewLimiter(opts.Store.client)
	}
	h := &Handler{
		botToken:   opts.BotToken,
		hostname:   strings.ToLower(base.Hostname()),
		store:      opts.Store,
		siteverify: opts.Siteverify,
		backend:    opts.Backend,
		limiter:    limiter,
		trusted:    opts.TrustedProxies,
		now:        now,
		inflight:   make(chan struct{}, verifyMaxInFlight),
	}
	h.ready.Store(!opts.StartNotReady)
	return h, nil
}

// SetReady marks the handler ready (true) or not ready (false). While it is not ready, verify
// answers 503 retry before reading anything and the health route reports unavailable.
func (h *Handler) SetReady(ready bool) {
	h.ready.Store(ready)
}

// Routes returns the three route patterns with their handlers, page being the Mini App page
// handler, for the HTTP server to mount.
func (h *Handler) Routes(page http.Handler) map[string]http.Handler {
	return map[string]http.Handler{
		RoutePage:   page,
		RouteVerify: http.HandlerFunc(h.Verify),
		RouteHealth: http.HandlerFunc(h.Health),
	}
}

// Health answers 200 {"status":"ok"} when the handler is ready and Redis answers a PING
// within RedisTimeout, and 503 {"status":"unavailable"} otherwise. The body never names an ID,
// a setting or a secret (D-10).
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	status, body := http.StatusOK, `{"status":"ok"}`
	if !h.ready.Load() || h.store.Ping(r.Context()) != nil {
		status, body = http.StatusServiceUnavailable, `{"status":"unavailable"}`
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "application/json")
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body+"\n"); err != nil {
		log.Debugf("[CaptchaWeb][Health] write failed: %v", err)
	}
}

type verifyRequest struct {
	InitData       string `json:"init_data"`
	TurnstileToken string `json:"turnstile_token"`
	Challenge      string `json:"challenge"`
}

// writeAnswer writes a decided answer and counts it once by its result (D-17).
func writeAnswer(w http.ResponseWriter, status int, answer Answer) {
	countAnswer(answer.Result)
	writeBody(w, status, answer)
}

// writeLimited writes the retry answer of a request refused by a spent budget or by the
// in-flight cap. It counts Limited and not Retry, so a refusal is one event.
func writeLimited(w http.ResponseWriter, status int) {
	countLimited()
	writeBody(w, status, Answer{Result: ResultRetry})
}

// writeBody is the one place a response is written: it sets the three headers every
// answer carries and the JSON body, and nothing else. It counts nothing, so a remembered
// answer that is replayed is not counted a second time.
func writeBody(w http.ResponseWriter, status int, answer Answer) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(answer); err != nil {
		log.Debugf("[CaptchaWeb][Verify] write failed: %v", err)
	}
}

// limitKeyForAddr is the per-address window key. An IPv6 client is counted by its /64, the
// smallest block one subscriber holds, because a single host can otherwise mint 2^64 buckets.
func limitKeyForAddr(addr netip.Addr) string {
	if addr.Is6() {
		if prefix, err := addr.Prefix(64); err == nil {
			return limitIPKeyPrefix + prefix.Addr().String()
		}
	}
	return limitIPKeyPrefix + addr.String()
}

// withinLimit counts one request in the window under key and reports whether the request may
// go on. When it may not, the answer is already written: 429 retry with Retry-After in whole
// seconds when the budget is spent, 503 retry when the window could not be counted (a Redis
// error never lets a request through).
func (h *Handler) withinLimit(w http.ResponseWriter, r *http.Request, key string, limit int64) bool {
	allowed, retryAfter, err := h.limiter.Allow(r.Context(), key, limit, verifyWindow)
	if err != nil {
		log.Warn("[CaptchaWeb][Verify] rate window failed")
		countError()
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return false
	}
	if allowed {
		return true
	}
	seconds := int64((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	writeLimited(w, http.StatusTooManyRequests)
	return false
}

func decodeVerifyRequest(w http.ResponseWriter, r *http.Request) (verifyRequest, int, bool) {
	var req verifyRequest
	// Only JSON is accepted: a form post or a text/plain post from another site is refused
	// before a byte of it is read (no CORS preflight can make it pass).
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		return req, http.StatusUnsupportedMediaType, false
	}
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
	// The server's write timeout is shorter than a siteverify call plus a pass. The error is
	// ignored: a recorder or a wrapped writer may not support deadlines.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(verifyWriteDeadline))

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAnswer(w, http.StatusMethodNotAllowed, Answer{Result: ResultInvalid})
		return
	}
	// Until the modules are loaded (polling mode starts serving before they are) nothing is
	// decided and nothing is read: the page retries (D-30).
	if !h.ready.Load() {
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	// A burst beyond the in-flight cap is refused at once, before any Redis or backend work.
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		writeLimited(w, http.StatusServiceUnavailable)
		return
	}

	// The per-address window comes before the body is read. Behind a shared proxy address with
	// no trusted list there is no per-address bucket, and the per-user windows still apply.
	if addr, bucket := ClientIP(r, h.trusted); bucket {
		if !h.withinLimit(w, r, limitKeyForAddr(addr), verifyIPLimit) {
			return
		}
	}

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
	// From here on the user is the signed one, so the budgets below are spent by the member
	// who sent the request and by nobody else (D-26).
	userID := strconv.FormatInt(user.UserID, 10)
	if !h.withinLimit(w, r, limitUserKeyPrefix+userID, verifyUserLimit) {
		return
	}
	// A signed start_param is authoritative: the challenge the page posts must be it.
	if user.StartParam != "" && user.StartParam != req.Challenge {
		writeAnswer(w, http.StatusOK, Answer{Result: ResultInvalid})
		return
	}
	if !h.withinLimit(w, r, limitTokenUserKeyPrefix+req.Challenge+":"+userID, verifyTokenUserLimit) {
		return
	}

	consumedAt := time.Now()
	consumed, rec, left, err := h.store.ConsumeIfUser(r.Context(), req.Challenge, user.UserID)
	if err != nil {
		log.WithField("user_id", user.UserID).Warn("[CaptchaWeb][Verify] challenge store failed")
		countError()
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	switch consumed {
	case ConsumeUnknown:
		h.answerUnknown(w, r, req.Challenge, user.UserID)
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

// answerUnknown answers a token the store no longer holds. When the token was decided in the
// last 10 minutes the same verified member gets the same answer again, with no side effect
// (their response was lost, or they tapped twice), and any other member gets not_for_you; with
// no remembered decision the token is invalid (D-03).
func (h *Handler) answerUnknown(w http.ResponseWriter, r *http.Request, token string, userID int64) {
	decision, found, err := h.store.Done(r.Context(), token)
	if err != nil {
		log.WithField("user_id", userID).Warn("[CaptchaWeb][Verify] done marker lookup failed")
		countError()
		writeAnswer(w, http.StatusServiceUnavailable, Answer{Result: ResultRetry})
		return
	}
	switch {
	case !found:
		writeAnswer(w, http.StatusOK, Answer{Result: ResultInvalid})
	case decision.UserID != userID:
		writeAnswer(w, http.StatusOK, Answer{Result: ResultNotForYou})
	default:
		// The remembered answer of an event already counted: replayed, not counted again.
		writeBody(w, http.StatusOK, Answer{Result: decision.Result, Action: decision.Action})
	}
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

// decided remembers the answer before it is written, so a response that is lost on the way
// can be asked for again (D-03). A write failure is logged by outcome and changes nothing: the
// answer is already decided.
func (c *challengeRun) decided(result Result, action string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.r.Context()), 2*RedisTimeout)
	defer cancel()
	d := Decision{UserID: c.user.UserID, Result: result, Action: action}
	if err := c.h.store.MarkDone(ctx, c.token, d); err != nil {
		log.WithFields(c.fields).Warnf("[CaptchaWeb][Verify] done marker failed outcome=%s", result)
	}
}

// retry gives the challenge back and answers retry with status.
func (c *challengeRun) retry(status int) {
	c.rearm()
	writeAnswer(c.w, status, Answer{Result: ResultRetry})
}

// lostClaim handles a claim that was not won. "No claim" does not mean "the row is gone": the
// claim also matches the message ID and the refresh count read a moment before, so a write that
// landed in between (SendCaptcha storing the message ID after its send) loses it while the
// attempt is still live. The attempt is read again: still live, the challenge is given back and
// the member answers retry and can try again; confirmed gone or expired, the check expires; a
// read that cannot be answered is a retry too, because expiring on a guess would tell a
// member who may still pass that their time ran out (WR-01).
func (c *challengeRun) lostClaim() {
	ctx, cancel := c.work()
	attempt, err := c.h.backend.Attempt(ctx, c.rec)
	cancel()
	switch {
	case err != nil:
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] attempt lookup after a lost claim failed")
		countError()
		c.retry(http.StatusServiceUnavailable)
	case attempt.Live:
		log.WithFields(c.fields).Info("[CaptchaWeb][Verify] outcome=retry reason=claim_changed")
		c.retry(http.StatusOK)
	default:
		c.action = attempt.Action
		c.expire()
	}
}

// expire applies the action to an attempt whose time is up or whose claim was lost, then
// answers expired with the action. It never re-arms.
func (c *challengeRun) expire() {
	ctx, cancel := c.work()
	defer cancel()
	res, err := c.h.backend.Expire(ctx, c.rec)
	if err != nil {
		// Nothing changed; the timer and the cleanup tick still own the row.
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] expire failed")
		countError()
	}
	c.answerAction(ResultExpired, res)
}

// answerAction answers a decided failed or expired outcome from what the Backend did. When the
// group's action could not be applied the member was unmuted and is still in the group, so the
// page gets the neutral invalid answer ("nothing was changed") instead of a removal notice, and
// the replay marker stores the same answer (WR-07).
func (c *challengeRun) answerAction(result Result, res ActionResult) {
	if res == ActionNotApplied {
		log.WithFields(c.fields).Warnf("[CaptchaWeb][Verify] outcome=%s applied=false", result)
		countError()
		c.decided(ResultInvalid, "")
		writeAnswer(c.w, http.StatusOK, Answer{Result: ResultInvalid})
		return
	}
	log.WithFields(c.fields).Infof("[CaptchaWeb][Verify] outcome=%s", result)
	c.decided(result, c.action)
	writeAnswer(c.w, http.StatusOK, Answer{Result: result, Action: c.action})
}

func (c *challengeRun) decide() {
	rightsCtx, cancelRights := context.WithTimeout(c.r.Context(), botRightsTimeout)
	canRestrict, err := c.h.backend.BotCanRestrict(rightsCtx, c.rec.ChatID)
	cancelRights()
	if err != nil {
		countError()
	}
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
		countError()
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
			countError()
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
		countError()
		c.retry(http.StatusServiceUnavailable)
		return
	}
	switch passed {
	case PassOK:
		log.WithFields(c.fields).Info("[CaptchaWeb][Verify] outcome=passed")
		c.decided(ResultPassed, "")
		writeAnswer(c.w, http.StatusOK, Answer{Result: ResultPassed})
	case PassLost:
		c.lostClaim()
	default:
		// Claimed, but the unmute failed: the Backend left a durable retry, so the challenge
		// is not given back.
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] outcome=retry reason=unmute")
		countError()
		writeAnswer(c.w, http.StatusOK, Answer{Result: ResultRetry})
	}
}

func (c *challengeRun) fail(reason Reason) {
	ctx, cancel := c.work()
	res, err := c.h.backend.Fail(ctx, c.rec)
	cancel()
	if err != nil {
		log.WithFields(c.fields).Warn("[CaptchaWeb][Verify] fail failed")
		countError()
		c.retry(http.StatusServiceUnavailable)
		return
	}
	if res == ActionLost {
		// Lost the conditional delete, to a timer or another replica or to a write that
		// changed the row (the challenge message ID, a refresh).
		c.lostClaim()
		return
	}
	log.WithFields(c.fields).Infof("[CaptchaWeb][Verify] failed solve reason=%s", reason)
	c.answerAction(ResultFailed, res)
}
