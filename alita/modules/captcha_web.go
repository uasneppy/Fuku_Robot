package modules

import (
	"context"
	"errors"
	"fmt"
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
}

// captchaWebRuntime is a configured web captcha: the challenge store, the verify handler
// and the settings the join flow needs.
type captchaWebRuntime struct {
	bot     *gotgbot.Bot
	cfg     CaptchaWebConfig
	store   *captchaweb.Store
	handler *captchaweb.Handler
}

// captchaWebCurrent is the installed runtime, or nil while the web captcha is off.
var captchaWebCurrent atomic.Pointer[captchaWebRuntime]

// The availability memo: the runtime the last probe was for, when it ran (unix nano) and
// whether Redis answered. They are package variables so a test can reset or set them.
var (
	captchaWebProbeRT atomic.Pointer[captchaWebRuntime]
	captchaWebProbeAt atomic.Int64
	captchaWebProbeOK atomic.Bool

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
	rt := &captchaWebRuntime{bot: bot, cfg: cfg, store: captchaweb.NewStore(cfg.Redis)}
	handler, err := captchaweb.NewHandler(captchaweb.Options{
		BotToken:      bot.Token,
		PublicBaseURL: cfg.BaseURL,
		Store:         rt.store,
		Siteverify:    cfg.Siteverify,
		Backend:       captchaWebBackend{rt: rt},
	})
	if err != nil {
		return nil, fmt.Errorf("web captcha handler: %w", err)
	}
	rt.handler = handler
	return rt, nil
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
	if captchaWebProbeRT.Load() == rt && now.UnixNano()-captchaWebProbeAt.Load() < int64(captchaWebProbeTTL) {
		return rt, captchaWebProbeOK.Load()
	}

	ctx, cancel := context.WithTimeout(context.Background(), captchaweb.RedisTimeout)
	defer cancel()
	ok := rt.cfg.Redis.Ping(ctx).Err() == nil
	captchaWebProbeRT.Store(rt)
	captchaWebProbeOK.Store(ok)
	captchaWebProbeAt.Store(now.UnixNano())
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
