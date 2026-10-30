// Package captchaweb is the server side of the Turnstile web captcha: the one-time
// challenge handle kept in Redis, Telegram's signed initData check, the Cloudflare
// siteverify client and the verify handler the Mini App page posts to.
//
// It imports the standard library, go-redis and logrus only and never a package of this
// module, so it is testable without the pre-main database connection of alita/config and
// alita/db. The bot-side work (the attempt row, the unmute, the greeting) is reached
// through the Backend interface, which alita/modules implements. Nothing in this package
// logs or returns an initData string, a Turnstile token, a challenge token, the secret or
// a Cloudflare body.
package captchaweb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// ReservedAnswerPrefix starts the answer of a Turnstile attempt. No button of the math
	// or text captcha can carry it, so the callbacks refuse any attempt that has it.
	ReservedAnswerPrefix = "ts1:"
	// TurnstileAction is the action the page renders the widget with and the server
	// requires siteverify to echo (12 characters, inside Cloudflare's 32).
	TurnstileAction = "captcha_join"
	// CDataPrefix is put before the challenge token when the cData is derived.
	CDataPrefix = "cd1:"
	// ChallengeGrace is how long a challenge outlives its attempt in Redis, so a late tap
	// is answered "expired" instead of "invalid".
	ChallengeGrace = 5 * time.Minute
	// RedisTimeout bounds every Redis round trip of the Store.
	RedisTimeout = 250 * time.Millisecond

	// KeyPrefix starts the key of a challenge. It sits outside alita:cache:, so
	// CLEAR_CACHE_ON_STARTUP keeps it.
	KeyPrefix = "alita:captcha:ch:"

	// DoneKeyPrefix starts the key of a decided answer, beside the challenge key and outside
	// the cache prefix for the same reason.
	DoneKeyPrefix = "alita:captcha:done:"
	// DoneTTL is how long a decided answer is remembered.
	DoneTTL = 10 * time.Minute
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidToken reports whether s can be a challenge token: 1 to 64 letters, digits,
// underscores or hyphens, the characters Telegram allows in a startapp value.
func ValidToken(s string) bool {
	return tokenPattern.MatchString(s)
}

// NewToken returns a fresh challenge token: 16 random bytes, base64url without padding
// (22 characters).
func NewToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// NewReservedAnswer returns the answer of a Turnstile attempt: ReservedAnswerPrefix plus
// 16 random bytes in lowercase hex, so a forged callback can neither guess nor convert it.
func NewReservedAnswer() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return ReservedAnswerPrefix + hex.EncodeToString(b[:]), nil
}

// IsReservedAnswer reports whether answer belongs to a Turnstile attempt.
func IsReservedAnswer(answer string) bool {
	return strings.HasPrefix(answer, ReservedAnswerPrefix)
}

// CData is the value the widget is rendered with and siteverify must echo back: the
// lowercase hex SHA-256 of CDataPrefix plus the challenge. It binds a Turnstile token to
// one challenge.
func CData(challenge string) string {
	sum := sha256.Sum256([]byte(CDataPrefix + challenge))
	return hex.EncodeToString(sum[:])
}

// MiniAppLink is the direct link of the Mini App that carries token in startapp.
func MiniAppLink(botUsername, shortName, token string) string {
	return "https://t.me/" + botUsername + "/" + shortName + "?startapp=" + token
}

// Record is what a challenge points at: the attempt, its chat and member, and the time the
// attempt expires.
type Record struct {
	AttemptID uint
	ChatID    int64
	UserID    int64
	ExpiresAt time.Time
}

// Store keeps the challenge handles in Redis, shared by every replica.
type Store struct {
	client *redis.Client
}

// NewStore wraps client. Every call of the Store is bounded by RedisTimeout.
func NewStore(client *redis.Client) *Store {
	return &Store{client: client}
}

// ErrInvalidChallenge means the token or the record cannot be stored.
var ErrInvalidChallenge = errors.New("captchaweb: invalid challenge")

// Issue stores the challenge under token with a TTL of the attempt's time left plus
// ChallengeGrace, in one bounded round trip. The hash fields are decimal strings, because
// Lua numbers lose exactness above 14 digits.
func (s *Store) Issue(ctx context.Context, token string, rec Record) error {
	if !ValidToken(token) {
		return ErrInvalidChallenge
	}
	ttl := time.Until(rec.ExpiresAt) + ChallengeGrace
	if ttl <= 0 {
		return ErrInvalidChallenge
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()

	key := KeyPrefix + token
	_, err := s.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, key,
			"u", strconv.FormatInt(rec.UserID, 10),
			"a", strconv.FormatUint(uint64(rec.AttemptID), 10),
			"c", strconv.FormatInt(rec.ChatID, 10),
			"x", strconv.FormatInt(rec.ExpiresAt.UnixMilli(), 10),
		)
		p.PExpire(ctx, key, ttl)
		return nil
	})
	return err
}

// Decision is a decided answer of a challenge, kept so a repeat of the same request (a lost
// response, a double tap) gets the same answer without acting twice (D-03).
type Decision struct {
	// UserID is the verified member the answer was given to.
	UserID int64
	// Result is passed, failed or expired, or invalid for an outcome whose failure action
	// could not be applied (the member was spared, so the page must not say they were removed).
	Result Result
	// Action is the group's failure action (kick, ban or mute); empty for passed.
	Action string
}

// MarkDone remembers a decided answer under token for DoneTTL. The first decision wins: a
// token is decided once, so a second write is a no-op. Only passed, failed, expired and invalid
// (the neutral answer of an action that could not be applied) are decisions; a retry is never
// stored.
func (s *Store) MarkDone(ctx context.Context, token string, d Decision) error {
	if !ValidToken(token) || d.UserID <= 0 || strings.Contains(d.Action, ":") {
		return ErrInvalidChallenge
	}
	switch d.Result {
	case ResultPassed, ResultFailed, ResultExpired, ResultInvalid:
	default:
		return ErrInvalidChallenge
	}
	value := strconv.FormatInt(d.UserID, 10) + ":" + string(d.Result)
	if d.Action != "" {
		value += ":" + d.Action
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	return s.client.SetNX(ctx, DoneKeyPrefix+token, value, DoneTTL).Err()
}

// Done returns the decision remembered for token and whether there is one. A value that does
// not parse is treated as no decision.
func (s *Store) Done(ctx context.Context, token string) (Decision, bool, error) {
	if !ValidToken(token) {
		return Decision{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	value, err := s.client.Get(ctx, DoneKeyPrefix+token).Result()
	if errors.Is(err, redis.Nil) {
		return Decision{}, false, nil
	}
	if err != nil {
		return Decision{}, false, err
	}
	parts := strings.SplitN(value, ":", 3)
	if len(parts) < 2 {
		return Decision{}, false, nil
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return Decision{}, false, nil
	}
	d := Decision{UserID: userID, Result: Result(parts[1])}
	if len(parts) == 3 {
		d.Action = parts[2]
	}
	switch d.Result {
	case ResultPassed, ResultFailed, ResultExpired, ResultInvalid:
		return d, true, nil
	}
	return Decision{}, false, nil
}

// Ping reports whether Redis answers a PING within RedisTimeout.
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()
	return s.client.Ping(ctx).Err()
}

// ConsumeResult is the outcome of ConsumeIfUser.
type ConsumeResult int

const (
	// ConsumeUnknown means no such challenge: unknown, used or expired. It is the zero value.
	ConsumeUnknown ConsumeResult = iota
	// ConsumeWrongUser means the challenge exists but belongs to another member. Nothing
	// was changed.
	ConsumeWrongUser
	// ConsumeOK means the challenge belonged to the member and has been deleted.
	ConsumeOK
)

// consumeScript deletes a challenge only when it belongs to the verified user.
//
// KEYS[1] = the challenge hash. ARGV[1] = the verified user ID as a decimal string.
// Returns {0} for an unknown challenge, {2} for another member's (nothing changed), and
// {1, pttl, HGETALL} after deleting the member's own.
var consumeScript = redis.NewScript(`
local u = redis.call('HGET', KEYS[1], 'u')
if not u then return {0} end
if u ~= ARGV[1] then return {2} end
local all = redis.call('HGETALL', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
redis.call('DEL', KEYS[1])
return {1, ttl, all}
`)

// ConsumeIfUser deletes the challenge when it belongs to userID and returns its record and
// the time it had left. Another member's verify changes nothing: the barrier is the signed
// user ID, never the secrecy of the public token. An unparsable record is an error, never
// ConsumeOK.
func (s *Store) ConsumeIfUser(ctx context.Context, token string, userID int64) (ConsumeResult, Record, time.Duration, error) {
	if !ValidToken(token) {
		return ConsumeUnknown, Record{}, 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()

	raw, err := consumeScript.Run(ctx, s.client, []string{KeyPrefix + token}, strconv.FormatInt(userID, 10)).Slice()
	if err != nil {
		return ConsumeUnknown, Record{}, 0, err
	}
	if len(raw) == 0 {
		return ConsumeUnknown, Record{}, 0, ErrInvalidChallenge
	}
	code, ok := raw[0].(int64)
	if !ok {
		return ConsumeUnknown, Record{}, 0, ErrInvalidChallenge
	}
	switch code {
	case 0:
		return ConsumeUnknown, Record{}, 0, nil
	case 2:
		return ConsumeWrongUser, Record{}, 0, nil
	case 1:
		rec, left, err := parseConsumed(raw)
		if err != nil {
			return ConsumeUnknown, Record{}, 0, err
		}
		return ConsumeOK, rec, left, nil
	}
	return ConsumeUnknown, Record{}, 0, ErrInvalidChallenge
}

// rearmScript puts a consumed challenge back, only when the key is absent and time remains.
//
// KEYS[1] = the challenge hash. ARGV = remaining ms, user, attempt, chat, expiry ms (all
// decimal strings, because Lua numbers lose exactness above 14 digits). Returns 1 when the
// challenge was written and 0 when nothing was.
var rearmScript = redis.NewScript(`
local ms = tonumber(ARGV[1])
if not ms or ms <= 0 then return 0 end
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('HSET', KEYS[1], 'u', ARGV[2], 'a', ARGV[3], 'c', ARGV[4], 'x', ARGV[5])
redis.call('PEXPIRE', KEYS[1], ms)
return 1
`)

// Rearm gives a consumed challenge back so the member can try again (D-21). It writes the
// same four fields under token with remaining as the time to live, never a fresh lifetime, and
// does nothing when the key exists again or remaining is not positive, so a re-arm can never
// stretch a challenge past its attempt or overwrite another one. It reports whether it wrote.
func (s *Store) Rearm(ctx context.Context, token string, rec Record, remaining time.Duration) (bool, error) {
	if !ValidToken(token) {
		return false, ErrInvalidChallenge
	}
	ms := remaining.Milliseconds()
	if ms <= 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, RedisTimeout)
	defer cancel()

	wrote, err := rearmScript.Run(ctx, s.client, []string{KeyPrefix + token},
		strconv.FormatInt(ms, 10),
		strconv.FormatInt(rec.UserID, 10),
		strconv.FormatUint(uint64(rec.AttemptID), 10),
		strconv.FormatInt(rec.ChatID, 10),
		strconv.FormatInt(rec.ExpiresAt.UnixMilli(), 10),
	).Int64()
	if err != nil {
		return false, err
	}
	return wrote == 1, nil
}

func parseConsumed(raw []any) (Record, time.Duration, error) {
	if len(raw) != 3 {
		return Record{}, 0, ErrInvalidChallenge
	}
	pttl, ok := raw[1].(int64)
	if !ok {
		return Record{}, 0, ErrInvalidChallenge
	}
	pairs, ok := raw[2].([]any)
	if !ok || len(pairs)%2 != 0 {
		return Record{}, 0, ErrInvalidChallenge
	}
	fields := make(map[string]string, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		k, kok := pairs[i].(string)
		v, vok := pairs[i+1].(string)
		if !kok || !vok {
			return Record{}, 0, ErrInvalidChallenge
		}
		fields[k] = v
	}
	userID, err := strconv.ParseInt(fields["u"], 10, 64)
	if err != nil || userID <= 0 {
		return Record{}, 0, ErrInvalidChallenge
	}
	chatID, err := strconv.ParseInt(fields["c"], 10, 64)
	if err != nil || chatID == 0 {
		return Record{}, 0, ErrInvalidChallenge
	}
	attemptID, err := strconv.ParseUint(fields["a"], 10, 64)
	if err != nil || attemptID == 0 {
		return Record{}, 0, ErrInvalidChallenge
	}
	expiresMs, err := strconv.ParseInt(fields["x"], 10, 64)
	if err != nil || expiresMs <= 0 {
		return Record{}, 0, ErrInvalidChallenge
	}
	left := time.Duration(pttl) * time.Millisecond
	if pttl < 0 {
		left = 0
	}
	return Record{
		AttemptID: uint(attemptID),
		ChatID:    chatID,
		UserID:    userID,
		ExpiresAt: time.UnixMilli(expiresMs),
	}, left, nil
}
