//go:build testtools

package modules

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/captcha"
	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/utils/captchaweb"
)

// callsForUser returns the calls of method whose user_id is userID.
func callsForUser(client *moduleBotClient, method string, userID int64) []moduleBotCall {
	var out []moduleBotCall
	for _, call := range client.callsFor(method) {
		if fmt.Sprint(call.Params["user_id"]) == fmt.Sprint(userID) {
			out = append(out, call)
		}
	}
	return out
}

// kicksOf counts the member-removing unbans (only_if_banned false) sent for userID: how a
// kick is applied in this codebase, one call that removes a current member and lets them
// rejoin.
func kicksOf(client *moduleBotClient, userID int64) int {
	var kicks int
	for _, call := range callsForUser(client, "unbanChatMember", userID) {
		if onlyIfBanned, _ := call.Params["only_if_banned"].(bool); !onlyIfBanned {
			kicks++
		}
	}
	return kicks
}

// sentTextContaining counts the sendMessage calls whose text contains marker.
func sentTextContaining(client *moduleBotClient, marker string) int {
	var n int
	for _, call := range client.callsFor("sendMessage") {
		if strings.Contains(fmt.Sprint(call.Params["text"]), marker) {
			n++
		}
	}
	return n
}

func TestCaptchaWebFailedSolveAppliesAction(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}
	invalid := captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-response"]}`)

	for _, action := range []string{"kick", "ban", "mute"} {
		t.Run(action, func(t *testing.T) {
			env := newCaptchaWebEnv(t)
			if err := captcha.SetCaptchaFailureAction(env.chat.Id, action); err != nil {
				t.Fatalf("SetCaptchaFailureAction() error = %v", err)
			}
			token := env.join(member)
			env.setSiteverify(invalid)

			status, answer := env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultFailed || answer.Action != action {
				t.Fatalf("verify = %d %+v, want 200 failed with action %s", status, answer, action)
			}

			switch action {
			case "kick":
				if got := kicksOf(env.client, member.Id); got != 1 {
					t.Fatalf("kicks = %d, want exactly one", got)
				}
				if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
					t.Fatalf("banChatMember calls = %d, want none for a kick", len(bans))
				}
			case "ban":
				if bans := callsForUser(env.client, "banChatMember", member.Id); len(bans) != 1 {
					t.Fatalf("banChatMember calls = %d, want exactly one", len(bans))
				}
				if got := kicksOf(env.client, member.Id); got != 0 {
					t.Fatalf("unbanChatMember calls = %d, want none after a ban", got)
				}
			case "mute":
				restricts := env.client.callsFor("restrictChatMember")
				if len(restricts) != 2 || permissionsOf(t, restricts[1]).CanSendMessages {
					t.Fatalf("restrictChatMember calls = %d, want the join mute then a mute that keeps messages off", len(restricts))
				}
				row, err := captcha.GetMutedUser(member.Id, env.chat.Id)
				if err != nil || row == nil {
					t.Fatalf("captcha_muted_users row = %v, %v; want the 24 h unmute", row, err)
				}
				if left := time.Until(row.UnmuteAt); left < 23*time.Hour || left > 25*time.Hour {
					t.Fatalf("unmute in %v, want about 24 h", left)
				}
				if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
					t.Fatalf("banChatMember calls = %d, want none for a mute", len(bans))
				}
			}

			if deletes := callsWithMessageID(env.client, "deleteMessage", 9001); len(deletes) == 0 {
				t.Fatal("the challenge message was not deleted")
			}
			marker := staffMarker("captcha_timeout_failure_" + map[string]string{"kick": "kicked", "ban": "banned", "mute": "muted"}[action])
			if got := sentTextContaining(env.client, marker); got != 1 {
				t.Fatalf("failure notices with %s = %d, want one", marker, got)
			}
			if env.attempt(member.Id) != nil {
				t.Fatal("the attempt row survived a failed solve")
			}

			status, answer = env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultInvalid {
				t.Fatalf("second verify = %d %+v, want 200 invalid (one solve only)", status, answer)
			}
			if got := env.cfRequests.Load(); got != 1 {
				t.Fatalf("Cloudflare requests = %d, want 1", got)
			}
		})
	}
}

func TestCaptchaWebCloudflareErrorIsNotAFailure(t *testing.T) {
	member := gotgbot.User{Id: 42, FirstName: "Member"}

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"500", captchaweb.SiteverifyAnswerForTest(http.StatusInternalServerError, `{"error":"upstream"}`)},
		{"internal-error", captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["internal-error"]}`)},
		{"invalid-input-secret", captchaweb.SiteverifyAnswerForTest(http.StatusOK, `{"success":false,"error-codes":["invalid-input-secret"]}`)},
		{"hostname mismatch", captchaweb.SiteverifyPassForTest("not-our-host.example.net")},
		{"network down", func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("the fake Cloudflare cannot hijack")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptchaWebEnv(t)
			token := env.join(member)
			before := env.attempt(member.Id)
			env.setSiteverify(tc.handler)

			status, answer := env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultRetry {
				t.Fatalf("verify = %d %+v, want 200 retry", status, answer)
			}
			if bans := env.client.callsFor("banChatMember"); len(bans) != 0 {
				t.Fatalf("banChatMember calls = %d, want none for a Cloudflare fault", len(bans))
			}
			if got := kicksOf(env.client, member.Id); got != 0 {
				t.Fatalf("kicks = %d, want none for a Cloudflare fault", got)
			}
			if restricts := env.client.callsFor("restrictChatMember"); len(restricts) != 1 {
				t.Fatalf("restrictChatMember calls = %d, want only the join mute (the member stays muted)", len(restricts))
			}
			after := env.attempt(member.Id)
			if after == nil || after.ID != before.ID || after.Answer != before.Answer ||
				after.Attempts != before.Attempts || !after.ExpiresAt.Equal(before.ExpiresAt) {
				t.Fatalf("attempt changed: before %+v after %+v", before, after)
			}
			if !env.mr.Exists(env.challengeKey(token)) {
				t.Fatal("the challenge was not given back")
			}
			if got := env.mr.HGet(env.challengeKey(token), "u"); got != "42" {
				t.Fatalf("re-armed challenge user = %q, want 42", got)
			}

			env.setSiteverify(captchaweb.SiteverifyPassForTest(captchaWebTestHost))
			status, answer = env.verify(member, token)
			if status != http.StatusOK || answer.Result != captchaweb.ResultPassed {
				t.Fatalf("verify after Cloudflare is back = %d %+v, want 200 passed", status, answer)
			}
			if env.attempt(member.Id) != nil {
				t.Fatal("attempt row survived the pass")
			}
			var rows int64
			if err := db.DB.Model(&models.CaptchaMutedUsers{}).
				Where("user_id = ? AND chat_id = ?", member.Id, env.chat.Id).Count(&rows).Error; err != nil || rows != 0 {
				t.Fatalf("captcha_muted_users rows = %d, %v; want none after the pass", rows, err)
			}
		})
	}
}
