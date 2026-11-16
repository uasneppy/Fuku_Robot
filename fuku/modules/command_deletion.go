package modules

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"

	"github.com/uasneppy/fuku_robot/fuku/db/deletion"
	"github.com/uasneppy/fuku_robot/fuku/utils/error_handling"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
)

// Command deletion (DEL-03). When a group stores member:commands as 0 ("Delete commands" on), a
// member's command is deleted once the bot has answered it. A command is a message gotgbot's own
// matcher would hand to a handler this bot registered (helpers.IsBotCommand): the bare form, the
// @thisbot form and the caption and entity-less forms the library accepts; another bot's command
// and an unregistered word are never touched.
//
// The delete runs in one place, after the dispatcher's groups: CommandDeletionProcessor wraps the
// processor main.go installs, so Inner.ProcessUpdate (every handler group, replies included) has
// returned before the delete is decided, and no reply path is edited. The wrapper sits OUTSIDE
// tracing.TracingProcessor on purpose and never reads the tracing context: in polling mode the
// tracing processor cancels its own context when it returns, so the post-step makes a fresh one,
// which works under the webhook transport too. It reads ctx.Update.Message and
// ctx.EffectiveSender after Inner returned, because the anonymous-admin tap replaces ctx.Update and
// the cached From is then a service ID. There is no defer around Inner: a handler panic that
// reaches the dispatcher skips the delete, which is the safe direction. The delete itself is
// deletionAct with delay 0 (the negative chat check, the delete-right latch, the Staff Group
// refusal and the inline delete under the deletion pacer with its due-now fallback).

// CommandDeletionProcessor deletes a member's handled command after Inner has processed the update.
type CommandDeletionProcessor struct {
	Inner ext.Processor
}

// ProcessUpdate runs the inner processor, keeps its error and then decides the command delete.
func (p CommandDeletionProcessor) ProcessUpdate(d *ext.Dispatcher, b *gotgbot.Bot, ctx *ext.Context) error {
	inner := p.Inner
	if inner == nil {
		inner = ext.BaseProcessor{}
	}
	err := inner.ProcessUpdate(d, b, ctx)
	if ctx != nil && ctx.Update != nil {
		commandDeletionAfter(b, ctx)
	}
	return err
}

// commandDeletionAfter deletes the update's message when it is a handled command of a member of a
// group whose commands rule is on. Cheapest tests first: non-command traffic pays only the message,
// chat type and text tests. A failure inside the deletion core is already logged there by chat ID;
// nothing is logged per message here.
func commandDeletionAfter(b *gotgbot.Bot, ctx *ext.Context) {
	defer error_handling.RecoverFromPanic("commandDeletion", "Deletion")

	msg := ctx.Message
	if msg == nil || b == nil || msg.Chat.Id >= 0 {
		return
	}
	if msg.Chat.Type != gotgbot.ChatTypeGroup && msg.Chat.Type != gotgbot.ChatTypeSupergroup {
		return
	}
	if msg.RichMessage == nil && !strings.HasPrefix(msg.GetText(), "/") {
		return
	}
	name, ok := botCommandName(b, msg)
	if !ok || !helpers.IsBotCommand(name) {
		return
	}
	if !commandDeletionSenderIsMember(b, ctx, msg) {
		return
	}
	if !commandDeletionRuleOn(msg.Chat.Id) {
		return
	}
	deletionAct(b, deletionAction{
		ChatID:    msg.Chat.Id,
		MessageID: msg.MessageId,
		Kind:      deletion.KindMemberCommands,
		SentAt:    deletionSentAt(msg.Date),
		Delay:     0,
	})
}

// commandDeletionSenderIsMember is true for a message sent by a user account: no sender chat (an
// anonymous admin, a channel), no bot, no Telegram service account and not the bot itself.
func commandDeletionSenderIsMember(b *gotgbot.Bot, ctx *ext.Context, msg *gotgbot.Message) bool {
	sender := ctx.EffectiveSender
	if sender == nil || sender.User == nil || sender.Chat != nil || sender.IsBot() || msg.SenderChat != nil {
		return false
	}
	id := sender.User.Id
	return id != b.Id && !staffServiceUserIDs[id]
}

// commandDeletionRuleOn reads the group's stored commands rule on a fresh context. Only a stored
// value the kind accepts (0, a switch that is on) counts: a value written by hand, such as 60, is
// treated as Off, and an unreadable rule deletes nothing.
func commandDeletionRuleOn(chatID int64) bool {
	readCtx, cancel := context.WithTimeout(context.Background(), deletionRecordTimeout)
	defer cancel()
	rules, err := deletionRulesLookup(readCtx, chatID)
	if err != nil || rules == nil {
		return false
	}
	delay, present := rules.Delays[deletion.KindMemberCommands]
	return present && deletion.ValidateRuleDelay(deletion.KindMemberCommands, delay) == nil
}

// botCommandName returns the lower-cased name of the command the library's command handler would
// accept the message for, and true; any other message is "", false. It mirrors the matcher's
// checkMessage at the pinned gotgbot version for the "/" trigger (no handler of this repository
// uses another): a rich message by its first block, otherwise the text or caption, the first
// entity rule (an entity at offset 0 must be a bot_command), the first whitespace-separated field,
// lower-casing, and an @name suffix that must be this bot's username.
func botCommandName(b *gotgbot.Bot, msg *gotgbot.Message) (string, bool) {
	if msg == nil || b == nil {
		return "", false
	}
	if msg.RichMessage != nil {
		return richCommandName(msg.RichMessage)
	}
	text := msg.GetText()
	if text == "" {
		return "", false
	}
	ents := msg.GetEntities()
	if len(ents) != 0 && ents[0].Offset == 0 && ents[0].Type != gotgbot.MessageEntityTypeBotCommand {
		return "", false
	}
	first := text
	if end := strings.IndexAny(text, " \t\n\v\f\r"); end != -1 {
		first = text[:end]
	}
	split := strings.SplitN(strings.ToLower(first), "@", 2)
	if len(split) > 1 && split[1] != strings.ToLower(b.Username) {
		return "", false
	}
	return stripCommandTrigger(split[0])
}

// stripCommandTrigger removes the leading "/" and reports whether a name is left.
func stripCommandTrigger(word string) (string, bool) {
	first, size := utf8.DecodeRuneInString(word)
	if first != '/' {
		return "", false
	}
	name := word[size:]
	return name, name != ""
}

// richCommandName mirrors the library's rich message branch: the first block must be a paragraph
// whose first text node is a plain string or a bot command. The library compares the extracted name
// with a lower-case registered name without lower-casing it, so a name with upper case matches no
// handler and is refused here too.
func richCommandName(rich *gotgbot.RichMessage) (string, bool) {
	if len(rich.Blocks) == 0 {
		return "", false
	}
	paragraph, ok := rich.Blocks[0].(gotgbot.RichBlockParagraph)
	if !ok {
		return "", false
	}
	whole := strings.TrimLeft(gotgbot.RichBlockContent(paragraph), " \n\t")
	name, ok := richNodeCommand(paragraph.Text, whole)
	if !ok || name != strings.ToLower(name) {
		return "", false
	}
	return name, true
}

func richNodeCommand(node gotgbot.RichText, whole string) (string, bool) {
	switch v := node.(type) {
	case gotgbot.RichTextArray:
		if len(v) == 0 {
			return "", false
		}
		return richNodeCommand(v[0], whole)
	case gotgbot.RichTextString:
		return stripCommandTrigger(string(v))
	case gotgbot.RichTextBotCommand:
		token := gotgbot.RichTextContent(v)
		if _, ok := stripCommandTrigger(token); !ok {
			return "", false
		}
		return v.BotCommand, v.BotCommand != "" && strings.HasPrefix(whole, token)
	}
	return "", false
}
