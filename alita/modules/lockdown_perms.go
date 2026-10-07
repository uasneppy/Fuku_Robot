package modules

// lockdownLockedPermissions is the group default permission set a lockdown sends:
// every one of the 16 keys is explicit false, so no omitempty or "defaults to"
// rule of the Bot API can leave a right open. Single-line JSON, in this key order.
const lockdownLockedPermissions = `{"can_send_messages":false,"can_send_audios":false,"can_send_documents":false,` +
	`"can_send_photos":false,"can_send_videos":false,"can_send_video_notes":false,"can_send_voice_notes":false,` +
	`"can_send_polls":false,"can_send_other_messages":false,"can_add_web_page_previews":false,` +
	`"can_react_to_messages":false,"can_edit_tag":false,"can_change_info":false,"can_invite_users":false,` +
	`"can_pin_messages":false,"can_manage_topics":false}`
