package captchaweb

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InitDataMaxAge is how old the auth_date of a signed initData may be (D-02).
const InitDataMaxAge = 5 * time.Minute

// initDataMaxFuture is the clock skew accepted for an auth_date ahead of the server.
const initDataMaxFuture = time.Minute

// ErrInvalidInitData is the one error ValidateInitData returns. It never says which check
// failed, so the answer teaches an attacker nothing.
var ErrInvalidInitData = errors.New("captchaweb: invalid init data")

// InitData is the part of Telegram's signed Mini App data the verify route uses.
type InitData struct {
	UserID       int64
	FirstName    string
	LanguageCode string
	StartParam   string
	AuthDate     time.Time
}

// ValidateInitData checks raw, the initData string a Mini App page received from Telegram,
// against botToken and returns the signed fields. The hash is checked first, in constant
// time, and no field is read before it matches. auth_date older than InitDataMaxAge is
// invalid.
//
// The data-check string is every field except hash (the signature field included), as
// key=value lines sorted bytewise and joined with a newline; the key is
// HMAC-SHA256("WebAppData", botToken).
func ValidateInitData(raw, botToken string, now time.Time) (InitData, error) {
	if raw == "" || botToken == "" {
		return InitData{}, ErrInvalidInitData
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, ErrInvalidInitData
	}
	for _, v := range values {
		if len(v) != 1 {
			return InitData{}, ErrInvalidInitData
		}
	}
	want, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(want) != sha256.Size {
		return InitData{}, ErrInvalidInitData
	}

	lines := make([]string, 0, len(values))
	for k, v := range values {
		if k == "hash" {
			continue
		}
		lines = append(lines, k+"="+v[0])
	}
	sort.Strings(lines)
	got := initDataMAC(botToken, strings.Join(lines, "\n"))
	if !hmac.Equal(got, want) {
		return InitData{}, ErrInvalidInitData
	}

	authUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil || authUnix <= 0 {
		return InitData{}, ErrInvalidInitData
	}
	authDate := time.Unix(authUnix, 0)
	if now.Sub(authDate) > InitDataMaxAge || authDate.Sub(now) > initDataMaxFuture {
		return InitData{}, ErrInvalidInitData
	}

	var user struct {
		ID           int64  `json:"id"`
		IsBot        bool   `json:"is_bot"`
		FirstName    string `json:"first_name"`
		LanguageCode string `json:"language_code"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID <= 0 || user.IsBot {
		return InitData{}, ErrInvalidInitData
	}
	return InitData{
		UserID:       user.ID,
		FirstName:    user.FirstName,
		LanguageCode: user.LanguageCode,
		StartParam:   values.Get("start_param"),
		AuthDate:     authDate,
	}, nil
}

// initDataMAC is the hex-decoded HMAC of dataCheck under the key derived from botToken.
func initDataMAC(botToken, dataCheck string) []byte {
	key := hmac.New(sha256.New, []byte("WebAppData"))
	key.Write([]byte(botToken))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(dataCheck))
	return mac.Sum(nil)
}
