package modules

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/config"
	"github.com/divkix/Alita_Robot/alita/utils/gemini"
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
// them small. Production code never reassigns them.
var (
	raidAIChatHourlyBudget   int64 = 150
	raidAIGlobalHourlyBudget int64 = 500
)

// raidAIHourStamp names the UTC hour of t, the suffix of the budget keys.
func raidAIHourStamp(t time.Time) string {
	return t.UTC().Format("2006010215")
}

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
)

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
			text = strings.ReplaceAll(text, secret, "[REDACTED]")
		}
	}
	return text
}

// raidAIProcessImage runs one image job: it downloads the frame, asks Gemini and reports the
// verdict. ctx derives from the pool's context, so the shutdown drain aborts both the
// download and the call; a check cut off that way is no verdict and not a provider failure.
// A download or getFile failure is a failure of this job only. It never deletes, restricts,
// bans or posts.
func raidAIProcessImage(ctx context.Context, job raidAIJob) {
	if job.media == nil {
		return
	}
	client := raidAIGeminiClient()
	if client == nil {
		return
	}
	fields := log.Fields{
		"chat_id":    job.chat.Id,
		"user_id":    job.userID,
		"message_id": job.messageID,
		"media":      job.media.Kind,
	}
	token := ""
	if job.bot != nil {
		token = job.bot.Token
	}

	image, err := raidAIDownload(ctx, job.bot, job.media.FileID)
	if err != nil {
		if ctx.Err() != nil {
			raidAIShed.Add(1)
			return
		}
		raidAIFailed.Add(1)
		log.WithFields(fields).Warnf("[RaidDetection] AI image download failed, no verdict: %s",
			raidAIScrub(err.Error(), token, client.APIKey))
		return
	}

	verdict, err := client.Classify(ctx, image)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			// Cut off by the shutdown drain: no verdict, and not the provider's failure.
			raidAIShed.Add(1)
		case errors.Is(err, gemini.ErrBlocked):
			raidAIBlocked.Add(1)
			log.WithFields(fields).Info("[RaidDetection] AI image check blocked by the provider's safety filters, no verdict")
		default:
			raidAIFailed.Add(1)
			entry := log.WithFields(fields)
			var httpErr *gemini.HTTPError
			if errors.As(err, &httpErr) {
				entry = entry.WithField("status", httpErr.Status)
			}
			entry.Warnf("[RaidDetection] AI image check failed, no verdict: %s",
				raidAIScrub(err.Error(), token, client.APIKey))
		}
		return
	}
	raidAIChecked.Add(1)
	raidAIWarnModelStage(verdict)

	spam := verdict.SpamProbability >= raidAIImageBar
	log.WithFields(fields).WithFields(log.Fields{
		"decision":    aispamDecisionName(spam),
		"probability": verdict.SpamProbability,
	}).Info("[RaidDetection] AI image verdict")

	if !spam {
		raidAIKept.Add(1)
		return
	}
	raidAISpam.Add(1)
	raidAIRecordVerdict(job.bot, job.chat, job.userID, job.itemKey)
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
