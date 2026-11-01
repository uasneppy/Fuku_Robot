//go:build testtools

package captchaweb

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This file holds the fixtures of the tests that drive the verify route: a signer that
// produces initData Telegram would, and a fake Cloudflare. Production code never imports it.

const testTurnstilePrefix = "test-turnstile."

// SignInitDataForTest returns the initData string for fields, signed with botToken: the
// hash of the data-check string is added as the hash field.
func SignInitDataForTest(botToken string, fields map[string]string) string {
	lines := make([]string, 0, len(fields))
	values := url.Values{}
	for k, v := range fields {
		lines = append(lines, k+"="+v)
		values.Set(k, v)
	}
	sort.Strings(lines)
	values.Set("hash", hex.EncodeToString(initDataMAC(botToken, strings.Join(lines, "\n"))))
	return values.Encode()
}

// InitDataForTest returns signed initData for a user, with start_param when it is not empty.
func InitDataForTest(botToken string, userID int64, firstName, languageCode, startParam string, authDate time.Time) string {
	user, _ := json.Marshal(map[string]any{
		"id":            userID,
		"first_name":    firstName,
		"language_code": languageCode,
	})
	fields := map[string]string{
		"user":      string(user),
		"auth_date": strconv.FormatInt(authDate.Unix(), 10),
	}
	if startParam != "" {
		fields["start_param"] = startParam
	}
	return SignInitDataForTest(botToken, fields)
}

// TurnstileTokenForTest is the Turnstile token a test page posts for challenge; the fake
// Cloudflare of SiteverifyPassForTest derives the echoed cdata from it.
func TurnstileTokenForTest(challenge string) string {
	return testTurnstilePrefix + CData(challenge)
}

// SiteverifyServerForTest starts a fake Cloudflare served by h and returns a Client that
// talks to it with the secret "turnstile-test-secret-value".
func SiteverifyServerForTest(t testing.TB, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	return &Client{
		Endpoint: server.URL,
		Secret:   "turnstile-test-secret-value",
		HTTP:     server.Client(),
	}
}

// SiteverifyPassForTest answers success for a response made by TurnstileTokenForTest,
// echoing hostname, TurnstileAction and the cdata, and success false with
// invalid-input-response for any other response.
func SiteverifyPassForTest(hostname string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Secret   string `json:"secret"`
			Response string `json:"response"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasPrefix(req.Response, testTurnstilePrefix) {
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"hostname": hostname,
			"action":   TurnstileAction,
			"cdata":    strings.TrimPrefix(req.Response, testTurnstilePrefix),
		})
	}
}

// SiteverifyAnswerForTest answers every request with status and body.
func SiteverifyAnswerForTest(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}
