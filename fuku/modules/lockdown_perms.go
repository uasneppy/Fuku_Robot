package modules

// Raw permission helpers for lockdowns. The group's default permissions are read
// and written as raw JSON and never through gotgbot.ChatPermissions: its bool fields
// are omitempty, and three of its fields are pointers documented as "defaults to
// can_send_messages / can_pin_messages", so a typed round trip loses a right that
// was explicitly off. The snapshot is the permissions member of a getChat answer,
// kept as the bytes Telegram sent and replayed as they are.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// lockdownLockedPermissions is the group default permission set a lockdown sends:
// every one of the 16 keys is explicit false, so no omitempty or "defaults to"
// rule of the Bot API can leave a right open. Single-line JSON, in this key order.
const lockdownLockedPermissions = `{"can_send_messages":false,"can_send_audios":false,"can_send_documents":false,` +
	`"can_send_photos":false,"can_send_videos":false,"can_send_video_notes":false,"can_send_voice_notes":false,` +
	`"can_send_polls":false,"can_send_other_messages":false,"can_add_web_page_previews":false,` +
	`"can_react_to_messages":false,"can_edit_tag":false,"can_change_info":false,"can_invite_users":false,` +
	`"can_pin_messages":false,"can_manage_topics":false}`

// lockdownCallTimeout bounds each raw getChat and setChatPermissions call.
const lockdownCallTimeout = 10 * time.Second

// lockdownChat is the part of a getChat answer a lockdown reads: the chat type and
// the permissions member as raw JSON. Permissions is empty when the answer carried
// none.
type lockdownChat struct {
	Type        string          `json:"type"`
	Permissions json.RawMessage `json:"permissions"`
}

// fetchLockdownChat asks Telegram for the chat and keeps its permissions member as
// raw JSON, exactly as sent.
func fetchLockdownChat(ctx context.Context, b *gotgbot.Bot, chatID int64) (lockdownChat, error) {
	callCtx, cancel := context.WithTimeout(ctx, lockdownCallTimeout)
	defer cancel()

	answer, err := b.RequestWithContext(callCtx, "getChat", map[string]any{"chat_id": chatID}, nil)
	if err != nil {
		return lockdownChat{}, err
	}
	var chat lockdownChat
	if err := json.Unmarshal(answer, &chat); err != nil {
		return lockdownChat{}, fmt.Errorf("decode getChat answer: %w", err)
	}
	return chat, nil
}

// setLockdownPermissions sets the group's default permissions to raw, which must be
// a JSON object, with use_independent_chat_permissions on so Telegram applies each
// key as sent and does not imply one right from another. Only an answer of JSON
// true counts as Telegram's confirmation.
func setLockdownPermissions(ctx context.Context, b *gotgbot.Bot, chatID int64, raw string) error {
	callCtx, cancel := context.WithTimeout(ctx, lockdownCallTimeout)
	defer cancel()

	answer, err := b.RequestWithContext(callCtx, "setChatPermissions", map[string]any{
		"chat_id":                          chatID,
		"permissions":                      json.RawMessage(raw),
		"use_independent_chat_permissions": true,
	}, nil)
	if err != nil {
		return err
	}
	var ok bool
	if err := json.Unmarshal(answer, &ok); err != nil || !ok {
		return fmt.Errorf("setChatPermissions answered %s, want true", string(answer))
	}
	return nil
}

// canonicalPermissions decodes a permission set into the keys whose value is the
// boolean true. A false value, a missing key and a value that is not a boolean are
// all the same here, so two sets compare independently of key order and of whether
// a false key was written out.
func canonicalPermissions(raw string) (map[string]bool, error) {
	var generic map[string]any
	if err := json.Unmarshal([]byte(raw), &generic); err != nil {
		return nil, err
	}
	granted := make(map[string]bool, len(generic))
	for key, value := range generic {
		if on, ok := value.(bool); ok && on {
			granted[key] = true
		}
	}
	return granted, nil
}

// samePermissions reports whether two permission sets grant the same rights.
func samePermissions(a, b string) (bool, error) {
	left, err := canonicalPermissions(a)
	if err != nil {
		return false, err
	}
	right, err := canonicalPermissions(b)
	if err != nil {
		return false, err
	}
	return maps.Equal(left, right), nil
}
