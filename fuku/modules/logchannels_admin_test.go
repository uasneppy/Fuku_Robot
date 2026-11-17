//go:build testtools

package modules

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/stretchr/testify/require"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/chats"
	"github.com/uasneppy/fuku_robot/fuku/db/logchannels"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
)

// TestLogChannelCommandNeedsAdmin proves that /logchannel, which names the group's log channel, answers a plain
// member with the admin refusal and never shows them the channel's ID.
func TestLogChannelCommandNeedsAdmin(t *testing.T) {
	withStaffLocale(t)
	client := newModuleBotClient()
	bot := newModuleTestBot(client)
	chat := gotgbot.Chat{Id: uniqueModuleChatID(), Type: "supergroup", Title: "Logged Chat"}
	channelID := uniqueModuleChatID()
	require.NoError(t, chats.EnsureChatInDb(chat.Id, chat.Title))
	require.NoError(t, logchannels.Set(chat.Id, chat.Title, channelID))
	t.Cleanup(func() { db.DB.Where("chat_id = ?", chat.Id).Delete(&models.LogChannel{}) })
	member := gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Member"}

	_ = logChannelsModule.logChannel(bot, newModuleMessageContext(bot, chat, member, "/logchannel"))

	calls := client.callsFor("sendMessage")
	require.Len(t, calls, 1)
	text := fmt.Sprint(calls[0].Params["text"])
	require.False(t, strings.Contains(text, strconv.FormatInt(channelID, 10)), "a member was shown the log channel: %q", text)
	require.True(t, strings.Contains(text, staffMarker("chat_status_user_admin_cmd_error")), "answer %q, want the admin refusal", text)
}
