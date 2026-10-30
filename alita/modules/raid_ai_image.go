package modules

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"

	"github.com/divkix/Alita_Robot/alita/config"
	"github.com/divkix/Alita_Robot/alita/utils/cache"
	"github.com/divkix/Alita_Robot/alita/utils/gemini"
	"github.com/divkix/Alita_Robot/alita/utils/logredact"
)

// The raid-only Gemini check of one image (RAID-04, PLAT-02). A worker of the raid AI pool
// fetches the one frame raidAIMediaRef picked, asks Gemini for a spam probability, and
// reports a verdict at or above raidAIImageBar to the AI burst counter. Nothing here
// deletes, restricts, bans or posts, and only the image and the code-owned instruction go
// to Google: no caption, name, chat or user data (D-12). The check is inert without
// GEMINI_API_KEY. A blocked, failed, slow or unreadable answer is no verdict.
//
// Logs carry chat, user and message IDs, the media kind and an HTTP status only. An error
// that could carry a URL or a key is rebuilt or scrubbed before it is logged.

// raidAIImageBar is the spam probability at or above which an image verdict counts as spam
// (D-04). A code constant, and the first thing the owner tunes after the live run.
const raidAIImageBar = 0.85

const (
	// raidPartAIAlbum starts the key of an album's gate; the media group ID follows.
	raidPartAIAlbum = "ag"
	// raidPartAIBudget starts the key of a chat's hourly budget; the hour stamp follows.
	raidPartAIBudget = "ab"
	// raidPartAIUserBudget starts the key of one account's hourly budget in one chat; the user
	// ID and the hour stamp follow (alita:raid:{<chat>}:abu:<user>:<hour>).
	raidPartAIUserBudget = "abu"
	// raidAIGlobalBudgetPrefix is the key prefix of the all-groups hourly budget; the hour stamp follows.
	raidAIGlobalBudgetPrefix = "alita:raid:aibudget:"
	// raidAIVerdictCachePrefix is the key prefix of a cached verdict; Telegram's file_unique_id follows.
	raidAIVerdictCachePrefix = "alita:raid:aiv:"

	// raidAIVerdictCacheTTL is how long one image's verdict is reused.
	raidAIVerdictCacheTTL = time.Hour
	// raidAIAlbumGateTTL is how long the first photo of an album keeps the others out.
	raidAIAlbumGateTTL = 120 * time.Second
	// raidAIBudgetTTL lets an hour's budget counters expire on their own.
	raidAIBudgetTTL = 2 * time.Hour
)

// The hourly Gemini budgets (D-20): code constants, held in variables only so a test can make
// them small. Production code never reassigns them. The per-account budget sits in front of
// the two shared ones (WR-01): one account posting distinct photos spends its own 20 units
// and then nothing else, so it can neither empty its group's 150 nor the fleet's 500.
var (
	raidAIUserHourlyBudget   int64 = 20
	raidAIChatHourlyBudget   int64 = 150
	raidAIGlobalHourlyBudget int64 = 500
)

var (
	// raidAIGeminiEndpoint is the Gemini API host. A variable only so a test points it at an
	// httptest server; production code never reassigns it.
	raidAIGeminiEndpoint = gemini.DefaultEndpoint
	// raidAIGeminiHTTP is the transport of the Gemini calls; nil is the client's own, which
	// has the call timeout and follows no redirect. A variable only for tests.
	raidAIGeminiHTTP *http.Client

	// raidAIBlocked counts the image checks Google's safety filters refused to judge.
	raidAIBlocked atomic.Int64

	// raidAIModelWarned makes the deprecated-model warning appear once per process.
	raidAIModelWarned atomic.Bool

	// raidAIImageFlight lets the jobs that carry one image (one file_unique_id) in one chat at
	// the same moment share a single verdict step, so a flood of the same sticker costs one
	// call even when the posts overlap and the cache holds nothing yet. Keyed by chat and
	// file, so another chat never inherits a refusal that belongs to this one.
	raidAIImageFlight singleflight.Group
)

// raidAIHourStamp names the UTC hour of t, the suffix of the budget keys.
func raidAIHourStamp(t time.Time) string {
	return t.UTC().Format("2006010215")
}

// raidAIGeminiClient builds the Gemini client of one job from the configured key and model.
// It returns nil without GEMINI_API_KEY.
func raidAIGeminiClient() *gemini.Client {
	key := config.AppConfig.GeminiAPIKey
	if key == "" {
		return nil
	}
	return &gemini.Client{
		Endpoint: raidAIGeminiEndpoint,
		APIKey:   key,
		Model:    config.AppConfig.GeminiModel,
		HTTP:     raidAIGeminiHTTP,
	}
}

// raidAIScrub replaces every secret in text, the last line of defence for an error the
// callers above already rebuild: the bot token and the Gemini key never reach a log.
func raidAIScrub(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, logredact.Placeholder)
		}
	}
	return text
}

// raidAIImageResult is what the verdict step of one image gives its jobs: the spam
// probability, and whether it was served without a Gemini call of this job.
type raidAIImageResult struct {
	probability float64
	cached      bool
	ok          bool
	// refused is set when the step stopped before any Gemini call for a reason that belongs
	// to this moment (a paused breaker, a budget refusal, a Redis error), so the rest of the
	// image's album may still be judged.
	refused bool
}

// raidAIProcessImage runs one image job. Before any Gemini call it passes, in order: the
// album gate (an album is judged once), the fire-latch skip (a lockdown was just attempted, so
// more checks are wasted), the verdict cache (one image is judged once an hour), the budgets
// (20 calls per account, 150 per group and 500 in all per hour, D-20), and only then the download and the
// call. Every refusal is no verdict and is counted, and a Redis error on any guard sends
// nothing. ctx derives from the pool's context, so the shutdown drain aborts both the
// download and the call; a check cut off that way is no verdict and not a provider failure.
// A download or getFile failure is a failure of this job only. It never deletes, restricts,
// bans or posts.
func raidAIProcessImage(ctx context.Context, job raidAIJob) {
	if job.media == nil || !raidAIImageConfigured() {
		return
	}
	client := raidAIGeminiClient()
	if client == nil {
		return
	}
	redisClient := raidRedis()
	if redisClient == nil {
		raidAIShed.Add(1)
		return
	}
	fields := log.Fields{
		"chat_id":    job.chat.Id,
		"user_id":    job.userID,
		"message_id": job.messageID,
		"media":      job.media.Kind,
	}

	if !raidAIAlbumFirst(redisClient, job) {
		return
	}
	if !raidAINoFire(redisClient, job) {
		raidAIAlbumRelease(redisClient, job)
		return
	}

	var result raidAIImageResult
	if uid := job.media.FileUniqueID; uid != "" {
		leader := false
		// The flight is per chat: a follower in another chat must not inherit a leader's
		// refusal (a capped or paused chat) when its own chat has budget.
		flightKey := strconv.FormatInt(job.chat.Id, 10) + ":" + uid
		answer, _, _ := raidAIImageFlight.Do(flightKey, func() (any, error) {
			leader = true
			return raidAIImageVerdict(ctx, client, redisClient, job, fields), nil
		})
		result, _ = answer.(raidAIImageResult)
		if !leader {
			// Another job of this image made the call; this one reuses its answer.
			result.cached = true
		}
	} else {
		result = raidAIImageVerdict(ctx, client, redisClient, job, fields)
	}
	if !result.ok {
		if result.refused {
			raidAIAlbumRelease(redisClient, job)
		}
		return
	}
	if result.cached {
		raidAICached.Add(1)
	}

	spam := result.probability >= raidAIImageBar
	log.WithFields(fields).WithFields(log.Fields{
		"decision":    aispamDecisionName(spam),
		"probability": result.probability,
		"cached":      result.cached,
	}).Info("[RaidDetection] AI image verdict")

	if !spam {
		raidAIKept.Add(1)
		return
	}
	raidAISpam.Add(1)
	raidAIRecordVerdict(job.bot, job.chat, job.userID, job.itemKey)
}

// raidAIAlbumFirst lets the first photo of an album through and drops the rest: an album is
// judged once (its item key is g:<media_group_id>, one item for the burst count). A message
// outside an album passes. A dropped photo counts as cached; a Redis error is no verdict.
func raidAIAlbumFirst(client *redis.Client, job raidAIJob) bool {
	group, isAlbum := strings.CutPrefix(job.itemKey, "g:")
	if !isAlbum {
		return true
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	first, err := client.SetNX(ctx, raidKey(job.chat.Id, raidPartAIAlbum+":"+group), "1", raidAIAlbumGateTTL).Result()
	if err != nil {
		raidAIShed.Add(1)
		log.Debugf("[RaidDetection] AI image album gate for chat %d failed, no verdict: %v", job.chat.Id, err)
		return false
	}
	if !first {
		raidAICached.Add(1)
	}
	return first
}

// raidAIAlbumRelease gives the album's gate back after the photo that took it was refused for a
// transient reason before any Gemini call, so the next photo of the album can be judged instead
// of the whole album being dropped for the gate's 120 seconds. A photo outside an album has no
// gate. Best effort: a failed delete leaves the gate to expire.
func raidAIAlbumRelease(client *redis.Client, job raidAIJob) {
	group, isAlbum := strings.CutPrefix(job.itemKey, "g:")
	if !isAlbum {
		return
	}
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	if err := client.Del(ctx, raidKey(job.chat.Id, raidPartAIAlbum+":"+group)).Err(); err != nil {
		log.Debugf("[RaidDetection] AI image album gate for chat %d could not be released: %v", job.chat.Id, err)
	}
}

// raidAINoFire reports false while the chat's fire latch is set: a trigger just attempted a
// lockdown, so another check is wasted. The skipped check counts as cached; a Redis error is
// no verdict.
func raidAINoFire(client *redis.Client, job raidAIJob) bool {
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	latched, err := client.Exists(ctx, raidKey(job.chat.Id, raidPartFire)).Result()
	if err != nil {
		raidAIShed.Add(1)
		log.Debugf("[RaidDetection] AI image fire check for chat %d failed, no verdict: %v", job.chat.Id, err)
		return false
	}
	if latched > 0 {
		raidAICached.Add(1)
		return false
	}
	return true
}

// raidAICachedVerdict reads the cached probability of one image. A missing, unreadable or
// out-of-range value is a miss; a Redis error is returned.
func raidAICachedVerdict(client *redis.Client, uniqueID string) (probability float64, hit bool, err error) {
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	value, err := client.Get(ctx, raidAIVerdictCachePrefix+uniqueID).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	probability, parseErr := strconv.ParseFloat(value, 64)
	if parseErr != nil || probability < 0 || probability > 1 {
		return 0, false, nil
	}
	return probability, true, nil
}

// raidAIStoreVerdict caches the probability of one image for an hour. Only a verdict is ever
// cached, never a failure or a block. Best effort: a failed write costs a later repeat call.
func raidAIStoreVerdict(client *redis.Client, uniqueID string, probability float64) {
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	value := strconv.FormatFloat(probability, 'f', -1, 64)
	if err := client.Set(ctx, raidAIVerdictCachePrefix+uniqueID, value, raidAIVerdictCacheTTL).Err(); err != nil {
		log.Debugf("[RaidDetection] AI image verdict cache write failed: %v", err)
	}
}

// raidAIBudgetTake adds one to an hourly counter and reports whether it is still within
// limit. The unit is reserved before the call, so a call that fails still spends it (the
// conservative side).
func raidAIBudgetTake(client *redis.Client, key string, limit int64) (bool, error) {
	ctx, cancel := context.WithTimeout(cache.Context, raidRedisTimeout)
	defer cancel()
	pipe := client.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, raidAIBudgetTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return count.Val() <= limit, nil
}

// raidAIReserveBudget takes one unit of the account's hourly budget in this chat, then one of
// the chat's, then one of the fleet's, each with the same atomic INCR+EXPIRE. A refusal counts
// as capped and a Redis error as shed; both are no verdict. An account over its own budget
// spends neither shared unit, and a chat over its own never spends the fleet's.
func raidAIReserveBudget(client *redis.Client, chatID, userID int64) bool {
	stamp := raidAIHourStamp(raidNow())
	budgets := []struct {
		key   string
		limit int64
	}{
		{raidKey(chatID, raidPartAIUserBudget+":"+strconv.FormatInt(userID, 10)+":"+stamp), raidAIUserHourlyBudget},
		{raidKey(chatID, raidPartAIBudget+":"+stamp), raidAIChatHourlyBudget},
		{raidAIGlobalBudgetPrefix + stamp, raidAIGlobalHourlyBudget},
	}
	for _, budget := range budgets {
		allowed, err := raidAIBudgetTake(client, budget.key, budget.limit)
		if err != nil {
			raidAIShed.Add(1)
			log.Debugf("[RaidDetection] AI image budget for chat %d could not be counted, no verdict: %v", chatID, err)
			return false
		}
		if !allowed {
			raidAICapped.Add(1)
			log.Debugf("[RaidDetection] AI image budget used up for chat %d, no verdict", chatID)
			return false
		}
	}
	return true
}

// raidAIImageVerdict is the step after the cheap guards: the verdict cache, the budgets, then
// the download and the Gemini call. It counts every outcome and returns the probability when
// there is a verdict.
func raidAIImageVerdict(
	ctx context.Context, client *gemini.Client, redisClient *redis.Client, job raidAIJob, fields log.Fields,
) raidAIImageResult {
	uid := job.media.FileUniqueID
	if uid != "" {
		probability, hit, err := raidAICachedVerdict(redisClient, uid)
		if err != nil {
			raidAIShed.Add(1)
			log.Debugf("[RaidDetection] AI image verdict cache read for chat %d failed, no verdict: %v", job.chat.Id, err)
			return raidAIImageResult{refused: true}
		}
		if hit {
			return raidAIImageResult{probability: probability, cached: true, ok: true}
		}
	}
	// The breakers come before the budget: a paused chat, or a paused provider, spends no unit.
	if raidAIGeminiPaused(job.chat.Id) {
		raidAIShed.Add(1)
		return raidAIImageResult{refused: true}
	}
	if !raidAIReserveBudget(redisClient, job.chat.Id, job.userID) {
		return raidAIImageResult{refused: true}
	}

	token := ""
	if job.bot != nil {
		token = job.bot.Token
	}

	image, err := raidAIDownload(ctx, job.bot, job.media.FileID)
	if err != nil {
		if ctx.Err() != nil {
			raidAIShed.Add(1)
			return raidAIImageResult{}
		}
		raidAIFailed.Add(1)
		log.WithFields(fields).Warnf("[RaidDetection] AI image download failed, no verdict: %s",
			raidAIScrub(err.Error(), token, client.APIKey))
		return raidAIImageResult{}
	}

	verdict, err := client.Classify(ctx, image)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			// Cut off by the shutdown drain: no verdict, and not the provider's failure.
			raidAIShed.Add(1)
		case errors.Is(err, gemini.ErrBlocked):
			// The provider answered: a refusal to judge this image is not an outage.
			raidAIGeminiRecordSuccess(job.chat.Id)
			raidAIBlocked.Add(1)
			log.WithFields(fields).Info("[RaidDetection] AI image check blocked by the provider's safety filters, no verdict")
		default:
			raidAIGeminiRecordFailure(job.chat.Id, err)
			raidAIFailed.Add(1)
			entry := log.WithFields(fields)
			var httpErr *gemini.HTTPError
			if errors.As(err, &httpErr) {
				entry = entry.WithField("status", httpErr.Status)
			}
			entry.Warnf("[RaidDetection] AI image check failed, no verdict: %s",
				raidAIScrub(err.Error(), token, client.APIKey))
		}
		return raidAIImageResult{}
	}
	raidAIGeminiRecordSuccess(job.chat.Id)
	raidAIChecked.Add(1)
	raidAIWarnModelStage(verdict)
	if uid != "" {
		raidAIStoreVerdict(redisClient, uid, verdict.SpamProbability)
	}
	return raidAIImageResult{probability: verdict.SpamProbability, ok: true}
}

// raidAIWarnModelStage logs one warning per process when the answer says the configured
// model is on its way out (LEGACY, DEPRECATED or RETIRED), naming the model id and the
// stage, so the owner changes GEMINI_MODEL before the model stops answering.
func raidAIWarnModelStage(verdict *gemini.Verdict) {
	switch verdict.ModelStage {
	case "LEGACY", "DEPRECATED", "RETIRED":
	default:
		return
	}
	if !raidAIModelWarned.CompareAndSwap(false, true) {
		return
	}
	model := verdict.Model
	if model == "" {
		model = config.AppConfig.GeminiModel
	}
	log.WithFields(log.Fields{
		"model": model,
		"stage": verdict.ModelStage,
	}).Warnf("[RaidDetection] Gemini model %s is %s, set GEMINI_MODEL to a current model", model, verdict.ModelStage)
}
