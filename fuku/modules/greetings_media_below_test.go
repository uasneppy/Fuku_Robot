//go:build testtools

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/greetings"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// mediaBelowClient is the greeting fake that also answers every media send, so a send succeeds.
func mediaBelowClient() *moduleBotClient {
	client := legacyGreetingClient()
	message := json.RawMessage(`{"message_id":9010,"date":1,"chat":{"id":-1001,"type":"supergroup","title":"Test Chat"}}`)
	for _, method := range []string{"sendSticker", "sendAudio", "sendVoice", "sendVideoNote"} {
		client.responses[method] = message
	}
	return client
}

// mediaBelowJoin drives one member's join through the chat_member handler.
func mediaBelowJoin(t *testing.T, client *moduleBotClient, chat gotgbot.Chat) {
	t.Helper()
	bot := newModuleTestBot(client)
	admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
	member := gotgbot.User{Id: 4242, FirstName: "Newbie"}
	ctx := newChatMemberContext(bot, chat, admin,
		gotgbot.ChatMemberLeft{User: member}, gotgbot.ChatMemberMember{User: member})
	clearRecentJoinProcessing(chat.Id, member.Id)
	if err := greetingsModule.newMember(bot, ctx); err != ext.EndGroups {
		t.Fatalf("newMember error = %v, want EndGroups", err)
	}
}

// mediaBelowOnlyCall returns the one recorded call of the method and fails when there is not exactly one.
func mediaBelowOnlyCall(t *testing.T, client *moduleBotClient, method string) moduleBotCall {
	t.Helper()
	calls := client.callsFor(method)
	if len(calls) != 1 {
		t.Fatalf("%s calls = %d, want 1", method, len(calls))
	}
	return calls[0]
}

func TestGreetingsMediaBelowEverywhere(t *testing.T) {
	withRealLocales(t)
	group := func(id int64) gotgbot.Chat {
		return gotgbot.Chat{Id: id, Type: "supergroup", Title: "Greeting Chat"}
	}
	buttons := []db.Button{{Name: "Rules", Url: "https://example.com/rules"}}

	t.Run("join_photo_switch_on", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-photo", buttons, db.PHOTO))
		if res := applyWelcomeMediaBelow(context.Background(), chatID, true); res.Outcome != greetingApplied {
			t.Fatalf("applyWelcomeMediaBelow outcome = %v, want applied", res.Outcome)
		}
		client := mediaBelowClient()
		mediaBelowJoin(t, client, group(chatID))
		call := mediaBelowOnlyCall(t, client, "sendPhoto")
		if call.Params["show_caption_above_media"] != true {
			t.Fatalf("show_caption_above_media = %v, want true", call.Params["show_caption_above_media"])
		}
		if body := legacyRequestJSON(t, call); !strings.Contains(body, `"caption":"Hi Newbie"`) || !strings.Contains(body, `"photo":"w-photo"`) {
			t.Fatalf("request = %s, want the caption and the stored photo", body)
		}
		if call.Params["reply_markup"] == nil {
			t.Fatalf("the stored keyboard is missing from the request")
		}
	})
	t.Run("join_photo_switch_back_off", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-photo", buttons, db.PHOTO))
		applyWelcomeMediaBelow(context.Background(), chatID, true)
		if res := applyWelcomeMediaBelow(context.Background(), chatID, false); res.Outcome != greetingApplied {
			t.Fatalf("applyWelcomeMediaBelow(false) outcome = %v, want applied", res.Outcome)
		}
		client := mediaBelowClient()
		mediaBelowJoin(t, client, group(chatID))
		call := mediaBelowOnlyCall(t, client, "sendPhoto")
		if _, ok := call.Params["show_caption_above_media"]; ok {
			t.Fatalf("show_caption_above_media is present with the switch off")
		}
	})
	t.Run("leave_video_switch_on", func(t *testing.T) {
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		mustLegacy(t, greetings.SetGoodbyeText(chatID, "Bye {first}", "g-video", buttons, db.VIDEO))
		mustLegacy(t, greetings.SetGoodbyeToggle(chatID, true))
		if res := applyGoodbyeMediaBelow(context.Background(), chatID, true); res.Outcome != greetingApplied {
			t.Fatalf("applyGoodbyeMediaBelow outcome = %v, want applied", res.Outcome)
		}
		client := mediaBelowClient()
		bot := newModuleTestBot(client)
		admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
		member := gotgbot.User{Id: 4242, FirstName: "Newbie"}
		ctx := newChatMemberContext(bot, group(chatID), admin,
			gotgbot.ChatMemberMember{User: member}, gotgbot.ChatMemberLeft{User: member})
		if err := greetingsModule.leftMember(bot, ctx); err != ext.EndGroups {
			t.Fatalf("leftMember error = %v, want EndGroups", err)
		}
		call := mediaBelowOnlyCall(t, client, "sendVideo")
		if call.Params["show_caption_above_media"] != true {
			t.Fatalf("show_caption_above_media = %v, want true", call.Params["show_caption_above_media"])
		}
	})
	t.Run("displays_follow_the_flag", func(t *testing.T) {
		admin := gotgbot.User{Id: 777000, FirstName: "Telegram"}
		for _, tc := range []struct {
			name    string
			command string
			goodbye bool
			on      bool
		}{
			{"welcome_on", "/welcome", false, true},
			{"welcome_noformat_on", "/welcome noformat", false, true},
			{"goodbye_on", "/goodbye", true, true},
			{"goodbye_noformat_on", "/goodbye noformat", true, true},
			{"welcome_off", "/welcome", false, false},
			{"goodbye_off", "/goodbye", true, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				chatID := uniqueModuleChatID()
				legacyCleanup(t, chatID)
				client := mediaBelowClient()
				bot := newModuleTestBot(client)
				ctx := newGreetingMessageContext(bot, group(chatID), admin, tc.command)
				var err error
				if tc.goodbye {
					mustLegacy(t, greetings.SetGoodbyeText(chatID, "Bye {first}", "g-anim", buttons, db.ANIMATION))
					applyGoodbyeMediaBelow(context.Background(), chatID, tc.on)
					err = greetingsModule.goodbye(bot, ctx)
				} else {
					mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-anim", buttons, db.ANIMATION))
					applyWelcomeMediaBelow(context.Background(), chatID, tc.on)
					err = greetingsModule.welcome(bot, ctx)
				}
				if err != ext.EndGroups {
					t.Fatalf("%s error = %v, want EndGroups", tc.command, err)
				}
				call := mediaBelowOnlyCall(t, client, "sendAnimation")
				got, present := call.Params["show_caption_above_media"]
				if tc.on && got != true {
					t.Fatalf("show_caption_above_media = %v, want true", got)
				}
				if !tc.on && present {
					t.Fatalf("show_caption_above_media is present with the switch off")
				}
			})
		}
	})
	t.Run("kinds_that_cannot_carry_a_caption_ignore_the_flag", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			msgType int
			method  string
		}{
			{"document", db.DOCUMENT, "sendDocument"},
			{"sticker", db.STICKER, "sendSticker"},
			{"audio", db.AUDIO, "sendAudio"},
			{"voice", db.VOICE, "sendVoice"},
			{"video_note", db.VIDEO_NOTE, "sendVideoNote"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				chatID := uniqueModuleChatID()
				legacyCleanup(t, chatID)
				mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-file", nil, tc.msgType))
				applyWelcomeMediaBelow(context.Background(), chatID, true)
				client := mediaBelowClient()
				mediaBelowJoin(t, client, group(chatID))
				mediaBelowOnlyCall(t, client, tc.method)
				legacyNoCaptionAbove(t, client)
			})
		}
	})
}

// mediaBelowRow loads the stored greetings row of a chat for a column comparison.
func mediaBelowRow(t *testing.T, chatID int64) models.GreetingSettings {
	t.Helper()
	row, err := greetings.LoadSettingsContext(context.Background(), chatID)
	if err != nil || row == nil || row.WelcomeSettings == nil || row.GoodbyeSettings == nil {
		t.Fatalf("LoadSettingsContext(%d) = %v, %v", chatID, row, err)
	}
	return *row
}

func TestGreetingsMediaBelowApply(t *testing.T) {
	seed := func(t *testing.T) int64 {
		t.Helper()
		chatID := uniqueModuleChatID()
		legacyCleanup(t, chatID)
		buttons := []db.Button{{Name: "Rules", Url: "https://example.com/rules"}}
		mustLegacy(t, greetings.SetWelcomeText(chatID, "Hi {first}", "w-photo", buttons, db.PHOTO))
		mustLegacy(t, greetings.SetGoodbyeText(chatID, "Bye {first}", "g-video", buttons, db.VIDEO))
		mustLegacy(t, greetings.SetCleanWelcomeSetting(chatID, true))
		return chatID
	}
	// sameExcept compares two rows after the one column under test and the timestamp are aligned.
	sameExcept := func(t *testing.T, before, after models.GreetingSettings, welcome bool) {
		t.Helper()
		after.UpdatedAt = before.UpdatedAt
		if welcome {
			after.WelcomeSettings.MediaBelow = before.WelcomeSettings.MediaBelow
		} else {
			after.GoodbyeSettings.MediaBelow = before.GoodbyeSettings.MediaBelow
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("a column other than the switch changed\n before %+v %+v\n after  %+v %+v",
				*before.WelcomeSettings, *before.GoodbyeSettings, *after.WelcomeSettings, *after.GoodbyeSettings)
		}
	}

	t.Run("welcome_stores_one_column", func(t *testing.T) {
		chatID := seed(t)
		before := mediaBelowRow(t, chatID)
		res := applyWelcomeMediaBelow(context.Background(), chatID, true)
		if res.Outcome != greetingApplied || res.Err != nil || res.Settings == nil || !res.Settings.WelcomeSettings.MediaBelow {
			t.Fatalf("apply(true) = %+v, want applied with the row read back and MediaBelow true", res)
		}
		after := mediaBelowRow(t, chatID)
		if !after.WelcomeSettings.MediaBelow || after.GoodbyeSettings.MediaBelow {
			t.Fatalf("stored welcome/goodbye MediaBelow = %v/%v, want true/false", after.WelcomeSettings.MediaBelow, after.GoodbyeSettings.MediaBelow)
		}
		sameExcept(t, before, after, true)

		res = applyWelcomeMediaBelow(context.Background(), chatID, false)
		if res.Outcome != greetingApplied || res.Settings == nil || res.Settings.WelcomeSettings.MediaBelow {
			t.Fatalf("apply(false) = %+v, want applied with MediaBelow false", res)
		}
		sameExcept(t, before, mediaBelowRow(t, chatID), true)
	})
	t.Run("goodbye_stores_only_its_column", func(t *testing.T) {
		chatID := seed(t)
		before := mediaBelowRow(t, chatID)
		res := applyGoodbyeMediaBelow(context.Background(), chatID, true)
		if res.Outcome != greetingApplied || res.Settings == nil || !res.Settings.GoodbyeSettings.MediaBelow {
			t.Fatalf("apply(true) = %+v, want applied with MediaBelow true", res)
		}
		after := mediaBelowRow(t, chatID)
		if after.WelcomeSettings.MediaBelow || !after.GoodbyeSettings.MediaBelow {
			t.Fatalf("stored welcome/goodbye MediaBelow = %v/%v, want false/true", after.WelcomeSettings.MediaBelow, after.GoodbyeSettings.MediaBelow)
		}
		sameExcept(t, before, after, false)
	})
	t.Run("a_failed_write_changes_nothing", func(t *testing.T) {
		chatID := seed(t)
		before := mediaBelowRow(t, chatID)
		writeErr := errors.New("write refused")
		saved := greetingWrites
		greetingWrites.WelcomeMediaBelow = func(int64, bool) error { return writeErr }
		greetingWrites.GoodbyeMediaBelow = func(int64, bool) error { return writeErr }
		t.Cleanup(func() { greetingWrites = saved })

		for name, res := range map[string]greetingApplyResult{
			"welcome": applyWelcomeMediaBelow(context.Background(), chatID, true),
			"goodbye": applyGoodbyeMediaBelow(context.Background(), chatID, true),
		} {
			if res.Outcome != greetingSaveFailed || !errors.Is(res.Err, writeErr) || res.Settings != nil {
				t.Fatalf("%s apply = %+v, want greetingSaveFailed with the write error", name, res)
			}
		}
		after := mediaBelowRow(t, chatID)
		after.UpdatedAt = before.UpdatedAt
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("the row changed after a failed write")
		}
	})
}
