package modules

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
	"github.com/divkix/Alita_Robot/alita/utils/error_handling"
	"github.com/divkix/Alita_Robot/alita/utils/helpers"
)

// The bot side of the Turnstile web captcha. The page and the verify route live in
// alita/utils/captchaweb; this file builds the runtime, decides whether the web captcha is
// available for a join (D-09) and implements captchaweb.Backend on top of the shared pass
// path of captcha.go.

const (
	// captchaWebProbeTTL is how long one Redis PING answer is reused, so a raid of joins
	// costs one probe per interval.
	captchaWebProbeTTL = 5 * time.Second
	// captchaWebFallbackLogEvery limits the "falling back to math" notice to one a minute.
	captchaWebFallbackLogEvery = time.Minute
)

// CaptchaWebConfig is the web captcha's settings, all of them required to run it.
type CaptchaWebConfig struct {
	// BaseURL is the public https address of the page.
	BaseURL string
	// ShortName is the Mini App short name registered with BotFather.
	ShortName string
	// SiteKey is the public Turnstile site key the page renders the widget with.
	SiteKey string
	// Secret is the Turnstile secret key. It is sent only to Cloudflare.
	Secret string
	// Redis holds the challenges. Without it the web captcha is unavailable.
	Redis *redis.Client
	// Siteverify is the Cloudflare client; nil builds one with Secret.
	Siteverify *captchaweb.Client
	// TrustedProxies is CAPTCHA_WEB_TRUSTED_PROXIES: the comma-separated IPs and CIDRs whose
	// X-Forwarded-For is believed. Empty means the client address is RemoteAddr. A bad entry
	// fails the configuration.
	TrustedProxies string
	// PageStrings are the page's string tables by language (the i18n StringTable of
	// captchaweb.PageLanguages and PageKeys). With it set the runtime builds the page and
	// CaptchaWebRoutes returns the three routes; without it there is no page to serve.
	PageStrings map[string]map[string]string
	// StartNotReady builds the verify route closed (503 retry) until MarkCaptchaWebReady, for
	// polling mode, where the HTTP server starts before the modules are loaded (D-30).
	StartNotReady bool
}

// captchaWebRuntime is a configured web captcha: the challenge store, the verify handler
// and the settings the join flow needs.
type captchaWebRuntime struct {
	bot     *gotgbot.Bot
	cfg     CaptchaWebConfig
	store   *captchaweb.Store
	handler *captchaweb.Handler
	// page is the Mini App page handler, nil when the config carried no PageStrings.
	page *captchaweb.Page
}

// captchaWebCurrent is the installed runtime, or nil while the web captcha is off.
var captchaWebCurrent atomic.Pointer[captchaWebRuntime]

// captchaWebProbeResult is one memoised Redis probe: the runtime it was for, when it ran (unix
// nano) and whether Redis answered. It is replaced as a unit, so a reader never sees the
// runtime of one probe with the time or the answer of another.
type captchaWebProbeResult struct {
	rt *captchaWebRuntime
	at int64
	ok bool
}

// The availability memo is a package variable so a test can reset or set it.
var (
	captchaWebProbe atomic.Pointer[captchaWebProbeResult]

	captchaWebFallbackLoggedAt atomic.Int64
)

// ConfigureCaptchaWeb builds and installs the web captcha runtime. It is called once at
// startup when all the settings are present.
func ConfigureCaptchaWeb(bot *gotgbot.Bot, cfg CaptchaWebConfig) error {
	rt, err := newCaptchaWebRuntime(bot, cfg)
	if err != nil {
		return err
	}
	captchaWebCurrent.Store(rt)
	return nil
}

// newCaptchaWebRuntime builds a runtime without installing it, so a test can stand up a
// second replica.
func newCaptchaWebRuntime(bot *gotgbot.Bot, cfg CaptchaWebConfig) (*captchaWebRuntime, error) {
	if bot == nil {
		return nil, errors.New("web captcha requires a bot")
	}
	if cfg.Redis == nil {
		return nil, errors.New("web captcha requires Redis")
	}
	if !captchaweb.ValidToken(cfg.ShortName) {
		return nil, errors.New("web captcha: invalid Mini App short name")
	}
	if cfg.Siteverify == nil {
		cfg.Siteverify = &captchaweb.Client{Secret: cfg.Secret}
	}
	trusted, err := captchaweb.ParseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("web captcha trusted proxies: %w", err)
	}
	rt := &captchaWebRuntime{bot: bot, cfg: cfg, store: captchaweb.NewStore(cfg.Redis)}
	if cfg.PageStrings != nil {
		rt.page, err = captchaweb.NewPage(captchaweb.PageOptions{SiteKey: cfg.SiteKey, Strings: cfg.PageStrings})
		if err != nil {
			return nil, fmt.Errorf("web captcha page: %w", err)
		}
	}
	handler, err := captchaweb.NewHandler(captchaweb.Options{
		BotToken:       bot.Token,
		PublicBaseURL:  cfg.BaseURL,
		Store:          rt.store,
		Siteverify:     cfg.Siteverify,
		Backend:        captchaWebBackend{rt: rt},
		TrustedProxies: trusted,
		StartNotReady:  cfg.StartNotReady,
	})
	if err != nil {
		return nil, fmt.Errorf("web captcha handler: %w", err)
	}
	rt.handler = handler
	return rt, nil
}

// CaptchaWebRoutes returns the three web captcha routes (the page, the verify route and the
// health check) keyed by their patterns (no method, so each handler answers a wrong method itself), for the HTTP server to mount before Start. It
// is nil while no runtime with a page is installed, so with the settings unset none of the
// three routes exists (D-10).
func CaptchaWebRoutes() map[string]http.Handler {
	rt := captchaWebCurrent.Load()
	if rt == nil || rt.page == nil {
		return nil
	}
	return rt.handler.Routes(rt.page)
}

// MarkCaptchaWebReady opens the verify route and the health check once the modules are loaded
// (D-30). It does nothing while no runtime is installed.
func MarkCaptchaWebReady() {
	if rt := captchaWebCurrent.Load(); rt != nil {
		rt.handler.SetReady(true)
	}
}

// captchaWebStatsSection is the part of the developer /stats text that comes from this
// replica's web captcha counters (D-17). They belong to the process that answers, start at
// zero on every restart, and the heading says so, so nobody reads them as fleet totals. It
// prints numbers only: no token, initData, ID or Cloudflare text.
func captchaWebStatsSection() string {
	s := captchaweb.StatsSnapshot()
	return "\n<b>Web captcha (this replica, since its last restart):</b>" +
		fmt.Sprintf(
			"\n    issued %d, passed %d, failed %d, expired %d, invalid %d, retry %d, limited %d, errors %d",
			s.Issued, s.Passed, s.Failed, s.Expired, s.Invalid, s.Retry, s.Limited, s.Errors,
		)
}

// CaptchaWebStartupWarnings returns the startup warnings of an enabled web captcha (D-31):
// the captcha routes share the HTTP port with /metrics, /db_metrics, /health, pprof and the
// webhook, and an empty trusted proxy list collapses the per-address limit. It takes no
// secret, so it cannot leak one.
func CaptchaWebStartupWarnings(metricsTokenSet bool, trustedProxies string) []string {
	var warnings []string
	if !metricsTokenSet {
		warnings = append(warnings, "[CaptchaWeb] METRICS_AUTH_TOKEN is not set: the captcha routes share the HTTP port "+
			"with /metrics, /db_metrics, /health, pprof and the webhook. Forward only /captcha and /captcha/* through the public proxy.")
	}
	if strings.TrimSpace(trustedProxies) == "" {
		warnings = append(warnings, "[CaptchaWeb] CAPTCHA_WEB_TRUSTED_PROXIES is empty: X-Forwarded-For is ignored "+
			"and requests from a private or loopback address get no per-address limit. Set it to your reverse proxy's address.")
	}
	return warnings
}

// captchaWebAvailable reports whether a join in a Turnstile group can get a web challenge:
// a runtime is installed, the bot has a username (the direct link needs it) and Redis
// answered a PING within RedisTimeout in the last captchaWebProbeTTL. cache.IsRedisAvailable
// is not used: it only says a client object exists.
func captchaWebAvailable(bot *gotgbot.Bot) (*captchaWebRuntime, bool) {
	rt := captchaWebCurrent.Load()
	if rt == nil || bot == nil || bot.Username == "" {
		return nil, false
	}
	now := time.Now()
	if memo := captchaWebProbe.Load(); memo != nil && memo.rt == rt && now.UnixNano()-memo.at < int64(captchaWebProbeTTL) {
		return rt, memo.ok
	}

	ctx, cancel := context.WithTimeout(context.Background(), captchaweb.RedisTimeout)
	defer cancel()
	ok := rt.cfg.Redis.Ping(ctx).Err() == nil
	captchaWebProbe.Store(&captchaWebProbeResult{rt: rt, at: now.UnixNano(), ok: ok})
	return rt, ok
}

// logCaptchaWebFallback notes at most once a minute that a Turnstile group got the math
// captcha because the web captcha was unavailable.
func logCaptchaWebFallback(chatID int64) {
	now := time.Now().UnixNano()
	last := captchaWebFallbackLoggedAt.Load()
	if now-last < int64(captchaWebFallbackLogEvery) || !captchaWebFallbackLoggedAt.CompareAndSwap(last, now) {
		return
	}
	log.Infof("[Captcha][Web] The web captcha is unavailable, so Turnstile groups (such as %d) get the math captcha", chatID)
}

// issueChallenge stores a fresh challenge token for attempt and returns it.
func (rt *captchaWebRuntime) issueChallenge(attempt *db.CaptchaAttempts) (string, error) {
	token, err := captchaweb.NewToken()
	if err != nil {
		return "", err
	}
	err = rt.store.Issue(context.Background(), token, captchaweb.Record{
		AttemptID: attempt.ID,
		ChatID:    attempt.ChatID,
		UserID:    attempt.UserID,
		ExpiresAt: attempt.ExpiresAt,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// captchaWebBackend is captchaweb.Backend over the captcha_attempts table and the shared
// pass path.
type captchaWebBackend struct {
	rt *captchaWebRuntime
}

// liveTurnstileAttempt reads the record's attempt fresh and returns it only while it is the
// same Turnstile attempt the challenge was issued for. The record's attempt ID is what is
// matched, never the member's current attempt, so a re-join's old token finds nothing.
func liveTurnstileAttempt(rec captchaweb.Record) (*db.CaptchaAttempts, error) {
	attempt, err := captcha.GetCaptchaAttemptByID(rec.AttemptID)
	if err != nil {
		return nil, err
	}
	if attempt == nil ||
		attempt.ID != rec.AttemptID ||
		attempt.UserID != rec.UserID ||
		attempt.ChatID != rec.ChatID ||
		!captchaweb.IsReservedAnswer(attempt.Answer) ||
		!time.Now().Before(attempt.ExpiresAt) {
		return nil, nil
	}
	return attempt, nil
}

// Attempt implements captchaweb.Backend.
func (b captchaWebBackend) Attempt(_ context.Context, rec captchaweb.Record) (captchaweb.AttemptStatus, error) {
	attempt, err := liveTurnstileAttempt(rec)
	if err != nil {
		return captchaweb.AttemptStatus{}, err
	}
	settings, err := captcha.GetCaptchaSettings(rec.ChatID)
	if err != nil {
		return captchaweb.AttemptStatus{}, err
	}
	return captchaweb.AttemptStatus{Live: attempt != nil, Action: settings.FailureAction}, nil
}

// Pass implements captchaweb.Backend. It claims the attempt and unmutes through
// claimCaptchaPass, the function the button pass calls, then finishes the pass through
// finishCaptchaPass on the captcha lifecycle so a shutdown waits for it.
func (b captchaWebBackend) Pass(ctx context.Context, rec captchaweb.Record, user captchaweb.InitData) (captchaweb.PassResult, error) {
	attempt, err := liveTurnstileAttempt(rec)
	if err != nil {
		return captchaweb.PassLost, err
	}
	if attempt == nil {
		return captchaweb.PassLost, nil
	}

	info, err := b.rt.bot.GetChatWithContext(ctx, rec.ChatID, nil)
	if err != nil {
		// Nothing was claimed, so the member can still pass.
		return captchaweb.PassLost, fmt.Errorf("read chat before the pass: %w", err)
	}
	chat := info.ToChat()
	in := captchaPassInput{
		Bot:       b.rt.bot,
		Chat:      &chat,
		Attempt:   attempt,
		UserID:    rec.UserID,
		FirstName: user.FirstName,
		Ctx:       &ext.Context{EffectiveChat: &chat},
	}

	stored, claimed, err := claimCaptchaPass(in)
	if !claimed {
		return captchaweb.PassLost, err
	}
	if err != nil {
		log.Errorf("[Captcha][Web] Failed to unmute user %d in chat %d after the pass: %v", rec.UserID, rec.ChatID, err)
		_ = helpers.DeleteMessageWithErrorHandling(in.Bot, in.Chat.Id, attempt.MessageID)
		return captchaweb.PassUnmuteFailed, nil
	}

	finish := func() {
		defer error_handling.RecoverFromPanic("captchaWebFinish", "captcha")
		finishCaptchaPass(in, stored)
	}
	if !startCaptchaLifecycleTask(func(context.Context) { finish() }) {
		finish()
	}
	return captchaweb.PassOK, nil
}

// BotCanRestrict implements captchaweb.Backend. It reads the bot's own membership live, with
// the caller's context, never the admin cache: the failure action (a kick, a ban or a mute)
// needs the Restrict right, and a check that cannot be answered is not a yes.
func (b captchaWebBackend) BotCanRestrict(ctx context.Context, chatID int64) (bool, error) {
	member, err := b.rt.bot.GetChatMemberWithContext(ctx, chatID, b.rt.bot.Id, nil)
	if err != nil {
		return false, fmt.Errorf("read the bot's membership: %w", err)
	}
	if member == nil {
		return false, errors.New("getChatMember returned no member")
	}
	merged := member.MergeChatMember()
	switch merged.Status {
	case gotgbot.ChatMemberStatusCreator:
		return true, nil
	case gotgbot.ChatMemberStatusAdministrator:
		return merged.CanRestrictMembers, nil
	}
	return false, nil
}

// pendingTurnstileAttempt reads the record's attempt fresh and returns it, expired or not,
// only while it is still the same Turnstile attempt the challenge was issued for.
func pendingTurnstileAttempt(rec captchaweb.Record) (*db.CaptchaAttempts, error) {
	attempt, err := captcha.GetCaptchaAttemptByID(rec.AttemptID)
	if err != nil {
		return nil, err
	}
	if attempt == nil ||
		attempt.UserID != rec.UserID ||
		attempt.ChatID != rec.ChatID ||
		!captchaweb.IsReservedAnswer(attempt.Answer) {
		return nil, nil
	}
	return attempt, nil
}

// Fail implements captchaweb.Backend. A provably failed solve is the same event as a timeout,
// so it runs settleCaptchaAttempt, the decision the timer makes: the conditional delete of the
// attempt row decides who applies the group's action, the challenge message is deleted and the
// failure notice is posted (D-08), except that an approved member or a group with the captcha
// off is released and unmuted, with no removal reported. It reads the row first so that a
// database error is reported as an error, with nothing changed, instead of looking like a lost
// claim.
func (b captchaWebBackend) Fail(_ context.Context, rec captchaweb.Record) (captchaweb.ActionResult, error) {
	attempt, err := pendingTurnstileAttempt(rec)
	if err != nil {
		return captchaweb.ActionLost, err
	}
	if attempt == nil {
		return captchaweb.ActionLost, nil
	}
	// The same decision as the timer: an approved member, or a group that turned the captcha
	// off, is released and unmuted instead of punished.
	settled, err := settleCaptchaAttempt(b.rt.bot, attempt)
	if settled.Claimed && err != nil {
		// The attempt is decided; only a follow-up step (an unmute retry) failed.
		log.Warnf("[Captcha][Web] Fail of attempt %d was claimed but a follow-up failed: %v", rec.AttemptID, err)
		err = nil
	}
	if err != nil {
		return captchaweb.ActionLost, err
	}
	if !settled.Claimed {
		// Not claimed is either "someone else decided" or a fault that changed nothing (the
		// timeout path swallows a database error as a lost claim). A row that is still there
		// was not decided, so the call changed nothing and must say so.
		if again, err := pendingTurnstileAttempt(rec); err != nil {
			return captchaweb.ActionLost, err
		} else if again != nil {
			return captchaweb.ActionLost, errors.New("attempt still pending after an unclaimed fail")
		}
	}
	return actionResultOf(settled.Claimed, settled.Applied), nil
}

// actionResultOf maps the (claimed, applied) pair of the timeout path to the Backend result.
func actionResultOf(claimed, applied bool) captchaweb.ActionResult {
	switch {
	case !claimed:
		return captchaweb.ActionLost
	case applied:
		return captchaweb.ActionApplied
	}
	return captchaweb.ActionNotApplied
}

// Expire implements captchaweb.Backend. It applies the group's action to an attempt whose
// time is up through expireCaptchaAttempt, the function the timer and the cleanup tick use,
// so whichever of the three arrives first decides and the others find the row gone. A row
// that is gone, belongs to another member or chat, or is still live is left alone: the timer
// owns a live row.
func (b captchaWebBackend) Expire(_ context.Context, rec captchaweb.Record) (captchaweb.ActionResult, error) {
	attempt, err := pendingTurnstileAttempt(rec)
	if err != nil {
		return captchaweb.ActionLost, err
	}
	if attempt == nil || time.Now().Before(attempt.ExpiresAt) {
		return captchaweb.ActionLost, nil
	}
	claimed, applied, err := expireCaptchaAttemptApplied(b.rt.bot, attempt)
	if claimed && err != nil {
		// The attempt is decided; only a follow-up step (an unmute retry) failed. The contract
		// reserves an error for a call that changed nothing.
		log.Warnf("[Captcha][Web] Expire of attempt %d was claimed but a follow-up failed: %v", rec.AttemptID, err)
		err = nil
	}
	return actionResultOf(claimed, applied), err
}
