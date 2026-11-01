//go:build testtools

package captchaweb

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two published samples below are not real bots: the tokens come from the README of
// telegram-mini-apps/init-data-golang and from the Telegram-Mini-Apps docs.
const (
	// #nosec G101 -- published sample token of init-data-golang, not a real bot.
	knownTokenSignature = "7342037359:AAHI25ES9xCOMPokpYoz-p8XVrZUdygo2J4"
	// #nosec G101 -- published sample token of the Telegram-Mini-Apps docs, not a real bot.
	knownTokenPlain = "5768337691:AAH5YkoiEuPk8-FZa32hStHTqXiLPtAEhx8"

	knownInitDataSignature = "user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22Vladislav%20%2B%20-%20%3F%20%5C%2F%22%2C%22last_name%22%3A%22Kibenko%22%2C%22username%22%3A%22vdkfrost%22%2C%22language_code%22%3A%22ru%22%2C%22is_premium%22%3Atrue%2C%22allows_write_to_pm%22%3Atrue%2C%22photo_url%22%3A%22https%3A%5C%2F%5C%2Ft.me%5C%2Fi%5C%2Fuserpic%5C%2F320%5C%2F4FPEE4tmP3ATHa57u6MqTDih13LTOiMoKoLDRG4PnSA.svg%22%7D&chat_instance=8134722200314281151&chat_type=private&auth_date=1733509682&signature=TYJxVcisqbWjtodPepiJ6ghziUL94-KNpG8Pau-X7oNNLNBM72APCpi_RKiUlBvcqo5L-LAxIc3dnTzcZX_PDg&hash=a433d8f9847bd6addcc563bff7cc82c89e97ea0d90c11fe5729cae6796a36d73"

	knownInitDataPlain = "query_id=AAHdF6IQAAAAAN0XohDhrOrc&user=%7B%22id%22%3A279058397%2C%22first_name%22%3A%22Vladislav%22%2C%22last_name%22%3A%22Kibenko%22%2C%22username%22%3A%22vdkfrost%22%2C%22language_code%22%3A%22ru%22%2C%22is_premium%22%3Atrue%7D&auth_date=1662771648&hash=c501b71e775f74ce10e377dea85a7ea24ecd640b223ea86dfe453e0eaed2e2b2"
)

func TestInitDataKnownAnswer(t *testing.T) {
	t.Run("signature field stays in the data-check string", func(t *testing.T) {
		got, err := ValidateInitData(knownInitDataSignature, knownTokenSignature, time.Unix(1733509682, 0))
		require.NoError(t, err)
		assert.Equal(t, int64(279058397), got.UserID)
		assert.Equal(t, "Vladislav + - ? /", got.FirstName)
		assert.Equal(t, "ru", got.LanguageCode)
		assert.Equal(t, "", got.StartParam)
		assert.Equal(t, int64(1733509682), got.AuthDate.Unix())
	})

	t.Run("without the signature field the hash no longer matches", func(t *testing.T) {
		values, err := url.ParseQuery(knownInitDataSignature)
		require.NoError(t, err)
		values.Del("signature")
		got, err := ValidateInitData(values.Encode(), knownTokenSignature, time.Unix(1733509682, 0))
		require.ErrorIs(t, err, ErrInvalidInitData)
		assert.Equal(t, InitData{}, got)
	})

	t.Run("a one character change in the hash is rejected", func(t *testing.T) {
		values, err := url.ParseQuery(knownInitDataSignature)
		require.NoError(t, err)
		hash := values.Get("hash")
		changed := strings.Replace(knownInitDataSignature, hash, "b"+hash[1:], 1)
		require.NotEqual(t, knownInitDataSignature, changed)
		_, err = ValidateInitData(changed, knownTokenSignature, time.Unix(1733509682, 0))
		require.ErrorIs(t, err, ErrInvalidInitData)
	})

	t.Run("data without a signature field", func(t *testing.T) {
		got, err := ValidateInitData(knownInitDataPlain, knownTokenPlain, time.Unix(1662771648, 0))
		require.NoError(t, err)
		assert.Equal(t, int64(279058397), got.UserID)
		assert.Equal(t, "Vladislav", got.FirstName)
		assert.Equal(t, "ru", got.LanguageCode)
		assert.Equal(t, "", got.StartParam)
		assert.Equal(t, int64(1662771648), got.AuthDate.Unix())
	})

	t.Run("each vector refuses the other vector's token", func(t *testing.T) {
		_, err := ValidateInitData(knownInitDataSignature, knownTokenPlain, time.Unix(1733509682, 0))
		require.ErrorIs(t, err, ErrInvalidInitData)
		_, err = ValidateInitData(knownInitDataPlain, knownTokenSignature, time.Unix(1662771648, 0))
		require.ErrorIs(t, err, ErrInvalidInitData)
	})
}

func TestInitDataRejects(t *testing.T) {
	const token = "123456:test-bot-token-for-initdata"
	now := time.Unix(1_800_000_000, 0)
	userJSON := `{"id":4242,"first_name":"Ada","language_code":"en"}`

	baseFields := func() map[string]string {
		return map[string]string{
			"query_id":  "AAHdF6IQAAAAAN0XohDhrOrc",
			"user":      userJSON,
			"auth_date": strconv.FormatInt(now.Unix(), 10),
		}
	}
	signed := func(mutate func(f map[string]string)) string {
		f := baseFields()
		if mutate != nil {
			mutate(f)
		}
		return SignInitDataForTest(token, f)
	}
	// edit changes a signed string after signing.
	edit := func(t *testing.T, raw string, mutate func(v url.Values)) string {
		t.Helper()
		v, err := url.ParseQuery(raw)
		require.NoError(t, err)
		mutate(v)
		return v.Encode()
	}
	withAge := func(d time.Duration) string {
		return signed(func(f map[string]string) { f["auth_date"] = strconv.FormatInt(now.Add(-d).Unix(), 10) })
	}
	withAhead := func(d time.Duration) string {
		return signed(func(f map[string]string) { f["auth_date"] = strconv.FormatInt(now.Add(d).Unix(), 10) })
	}
	withUser := func(user string) string {
		return signed(func(f map[string]string) { f["user"] = user })
	}

	// Sanity: the base data is accepted, so every rejection below is caused by its own change.
	got, err := ValidateInitData(signed(nil), token, now)
	require.NoError(t, err)
	require.Equal(t, int64(4242), got.UserID)

	good := signed(nil)
	goodHash := func() string {
		v, err := url.ParseQuery(good)
		require.NoError(t, err)
		return v.Get("hash")
	}()

	cases := []struct {
		name string
		raw  string
	}{
		{"changed user value after signing", edit(t, good, func(v url.Values) {
			v.Set("user", `{"id":9999,"first_name":"Ada","language_code":"en"}`)
		})},
		{"added field after signing", edit(t, good, func(v url.Values) { v.Set("extra", "1") })},
		{"removed field after signing", edit(t, good, func(v url.Values) { v.Del("query_id") })},
		{"two hash parameters", edit(t, good, func(v url.Values) { v.Add("hash", goodHash) })},
		{"repeated non-hash key", edit(t, good, func(v url.Values) {
			v.Add("auth_date", strconv.FormatInt(now.Unix(), 10))
		})},
		{"hash in uppercase hex", edit(t, good, func(v url.Values) { v.Set("hash", strings.ToUpper(goodHash)) })},
		{"hash of 63 characters", edit(t, good, func(v url.Values) { v.Set("hash", goodHash[:63]) })},
		{"signed with another bot token", SignInitDataForTest("654321:another-bot-token", baseFields())},
		{"auth_date 5 minutes 1 second old", withAge(5*time.Minute + time.Second)},
		{"auth_date 61 seconds in the future", withAhead(61 * time.Second)},
		{"auth_date missing", signed(func(f map[string]string) { delete(f, "auth_date") })},
		{"auth_date not a number", signed(func(f map[string]string) { f["auth_date"] = "abc" })},
		{"user is not json", withUser("{not json")},
		{"user id zero", withUser(`{"id":0,"first_name":"Ada"}`)},
		{"user id negative", withUser(`{"id":-5,"first_name":"Ada"}`)},
		{"user is a bot", withUser(`{"id":4242,"is_bot":true,"first_name":"Ada"}`)},
		{"no user field", signed(func(f map[string]string) { delete(f, "user") })},
		{"raw string of 4097 bytes", func() string {
			// Signed correctly, so only the size cap can refuse it. An empty pad already
			// encodes as "pad=", so the pad value fills the rest.
			empty := signed(func(f map[string]string) { f["pad"] = "" })
			raw := signed(func(f map[string]string) { f["pad"] = strings.Repeat("a", 4097-len(empty)) })
			require.Len(t, raw, 4097)
			return raw
		}()},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateInitData(tc.raw, token, now)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidInitData)
			assert.Equal(t, InitData{}, got)
		})
	}

	t.Run("auth_date exactly 5 minutes old is accepted", func(t *testing.T) {
		got, err := ValidateInitData(withAge(InitDataMaxAge), token, now)
		require.NoError(t, err)
		assert.Equal(t, int64(4242), got.UserID)
	})

	t.Run("auth_date exactly 60 seconds ahead is accepted", func(t *testing.T) {
		got, err := ValidateInitData(withAhead(60*time.Second), token, now)
		require.NoError(t, err)
		assert.Equal(t, int64(4242), got.UserID)
	})
}
