package modules

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/utils/claude"
	"github.com/uasneppy/fuku_robot/fuku/utils/gemini"
)

// TestChatLogsCarryIDsOnly sweeps every failure path of a conversation at debug level: no line
// may hold the member's text, their name, a reply, the provider's own error message or a key.
func TestChatLogsCarryIDsOnly(t *testing.T) {
	hook := mreqLogCapture(t)
	const (
		secretText   = "zebra-sentinel-question"
		secretName   = "Quasimodo"
		secretReply  = "walrus-sentinel-reply"
		providerText = "octopus-sentinel-provider-message"
		keyValue     = "chat-test-anthropic-value"
	)
	env := newChatEnv(t)
	member := func() *gotgbot.User { return env.member(secretName) }

	steps := []func(context.Context, chatRequest) (chatAnswer, error){
		chatAnswerErr(&claude.HTTPError{Status: 500, Type: "api_error", Message: providerText}),
		chatAnswerErr(&gemini.HTTPError{Status: 429, GRPCStatus: "RESOURCE_EXHAUSTED", Message: providerText}),
		chatAnswerErr(errors.New(providerText + " " + keyValue)),
		chatAnswerErr(&claude.HTTPError{Status: 401, Type: "authentication_error", Message: providerText}),
	}
	for _, step := range steps {
		env.provider.script(step)
		chatSwap(t, &chatBreakers, map[string]*memberReqAIBreaker{"anthropic": newMemberReqAIBreaker(), "google": newMemberReqAIBreaker()})
		env.deliver(env.mention(member(), secretText))
	}
	// A refusal, a blank answer, a reply whose target was deleted, a send Telegram refused and a
	// budget counter that failed.
	env.provider.script(
		func(context.Context, chatRequest) (chatAnswer, error) { return chatAnswer{Outcome: chatRefused}, nil },
		chatAnswerText("  "),
	)
	env.deliver(env.mention(member(), secretText))
	env.deliver(env.mention(member(), secretText))
	gone := env.mention(member(), secretText)
	env.bot.BotClient.(*chatTelegramFake).deleted[gone.MessageId] = true
	env.provider.script(chatAnswerText(secretReply))
	env.deliver(gone)
	env.client.errors["sendMessage"] = &gotgbot.TelegramError{Code: 403, Description: providerText}
	env.provider.script(chatAnswerText(secretReply))
	env.deliver(env.mention(member(), secretText))
	delete(env.client.errors, "sendMessage")
	env.mr.SetError("sentinel redis fault " + providerText)
	env.deliver(env.mention(member(), secretText))
	env.mr.SetError("")

	lines := mreqLogLines(hook)
	var chatLines int
	for _, line := range lines {
		if strings.Contains(line, "[Chat]") {
			chatLines++
		}
		for _, value := range []string{secretText, secretName, secretReply, providerText, keyValue} {
			if strings.Contains(line, value) {
				t.Fatalf("a log line carries %q: %s", value, line)
			}
		}
	}
	if chatLines == 0 {
		t.Fatalf("no [Chat] line was logged, so the sweep proves nothing")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "HTTP 500 api_error") {
		t.Fatalf("the provider failure line names no status and type")
	}
}
