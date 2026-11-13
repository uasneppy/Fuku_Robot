//go:build testtools

package i18n

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const settingsKeyPrefix = "settings_"

// settingsLimit is the UI-SPEC length ceiling of one settings string, counted in runes of
// the template: English against max, every other language against other.
type settingsLimit struct {
	max   int
	other int
}

// settingsLocaleLimits is the "Max EN" and "Max other" columns of the UI-SPEC "New locale
// keys" table, for the keys that exist so far. The test fails when it does not cover
// exactly the settings_ keys of the locale files, so a plan that adds a key adds its row.
var settingsLocaleLimits = map[string]settingsLimit{
	"settings_header_main":       {24, 34},
	"settings_header":            {36, 50},
	"settings_page_captcha":      {14, 18},
	"settings_choose":            {90, 126},
	"settings_btn_open_here":     {20, 28},
	"settings_btn_open_private":  {24, 32},
	"settings_main_hint":         {100, 140},
	"settings_main_owner":        {48, 68},
	"settings_btn_back":          {12, 16},
	"settings_btn_close":         {12, 16},
	"settings_captcha_time":      {36, 50},
	"settings_btn_minutes":       {10, 12},
	"settings_not_your_menu":     {110, 160},
	"settings_not_admin":         {90, 130},
	"settings_no_restrict":       {110, 160},
	"settings_check_failed":      {110, 160},
	"settings_min_value":         {50, 70},
	"settings_max_value":         {50, 70},
	"settings_cannot_open_alert": {60, 84},
	"settings_cannot_open":       {140, 196},
	"settings_closed":            {24, 34},
	"settings_start_not_admin":   {90, 126},
	"settings_start_invalid":     {110, 154},

	// Page and card strings (plan 09-05). settings_lockdown_reason is not here on
	// purpose: the reason line reuses lockdown_reason_line (D-29).
	"settings_page_staff":              {14, 18},
	"settings_page_lockdown":           {14, 18},
	"settings_page_antiflood":          {14, 18},
	"settings_state_linked":            {14, 20},
	"settings_state_not_linked":        {14, 20},
	"settings_state_staff_n":           {14, 20},
	"settings_state_lock_none":         {14, 20},
	"settings_state_lock_active":       {14, 20},
	"settings_state_lock_lifting":      {14, 20},
	"settings_state_limit":             {14, 20},
	"settings_state_off_plain":         {10, 14},
	"settings_btn_yes":                 {12, 16},
	"settings_btn_no":                  {12, 16},
	"settings_staff_linked_to":         {60, 84},
	"settings_staff_linked_hidden":     {40, 56},
	"settings_btn_staff_pick_n":        {24, 32},
	"settings_staff_pick_private_hint": {80, 112},
	"settings_staff_not_linked":        {60, 84},
	"settings_staff_link_hint":         {100, 140},
	"settings_staff_readonly":          {110, 154},
	"settings_staff_pick":              {60, 84},
	"settings_btn_link":                {28, 38},
	"settings_btn_unlink":              {28, 38},
	"settings_notice_linked":           {70, 98},
	"settings_notice_unlinked":         {70, 98},
	"settings_lockdown_none":           {80, 112},
	"settings_lockdown_triggers":       {80, 112},
	"settings_trigger_joins":           {70, 98},
	"settings_trigger_flood":           {70, 98},
	"settings_trigger_ai":              {76, 106},
	"settings_btn_trigger_joins":       {24, 32},
	"settings_btn_trigger_flood":       {24, 32},
	"settings_btn_trigger_ai":          {24, 32},
	"settings_lockdown_basic":          {100, 140},
	"settings_lockdown_unreadable":     {100, 140},
	"settings_captcha_status":          {24, 34},
	"settings_captcha_mode":            {40, 56},
	"settings_captcha_action":          {40, 56},
	"settings_captcha_fallback_note":   {110, 154},
	"settings_btn_captcha_toggle":      {24, 32},
	"settings_opt_math":                {10, 12},
	"settings_opt_text":                {10, 12},
	"settings_opt_turnstile":           {10, 12},
	"settings_opt_kick":                {10, 12},
	"settings_opt_ban":                 {10, 12},
	"settings_opt_mute":                {10, 12},
	"settings_opt_warn":                {10, 12},
	"settings_opt_tban":                {14, 18},
	"settings_opt_tmute":               {14, 18},
	"settings_antiflood_limit":         {36, 50},
	"settings_antiflood_off":           {70, 98},
	"settings_antiflood_action":        {36, 50},
	"settings_antiflood_delete":        {40, 56},
	"settings_antiflood_action_note":   {110, 154},
	"settings_antiflood_now_on":        {100, 140},
	"settings_btn_antiflood_delete":    {36, 48},
	"settings_card_expired":            {80, 112},
	"settings_staff_no_group":          {120, 170},
	"settings_staff_pick_more":         {120, 170},

	// The settings hub and the owner gate (plan 10-02). The ceilings are the UI-SPEC "New
	// locale keys" columns; the alert and reply rows are held lower than Telegram's 200 so a
	// long translation still fits.
	"settings_page_groups":       {14, 18},
	"settings_page_access":       {14, 18},
	"settings_btn_groups":        {12, 18},
	"settings_main_state_line":   {60, 84},
	"settings_state_access_on":   {20, 28},
	"settings_state_access_off":  {20, 28},
	"settings_list_intro":        {80, 112},
	"settings_list_hint":         {200, 280},
	"settings_list_empty":        {140, 200},
	"settings_access_status":     {50, 70},
	"settings_access_on_desc":    {100, 140},
	"settings_access_off_desc":   {130, 182},
	"settings_btn_access_toggle": {36, 40},
	"settings_access_confirm":    {240, 336},
	"settings_gate_owner_only":   {120, 160},
	"settings_access_owner_only": {120, 160},
	"settings_pick_unavailable":  {120, 160},
	"settings_refuse_owner_only": {70, 98},
	"settings_start_owner_only":  {70, 98},

	// The /help entry (plan 09-13, widened by plan 10-02).
	"settings_help_msg": {1100, 1540},

	// The message editor (plan 11-01). The ceilings are the UI-SPEC "New locale keys" columns.
	"settings_page_welcome":                {12, 18},
	"settings_page_goodbye":                {12, 18},
	"settings_page_rules":                  {12, 18},
	"settings_btn_edit_set_text":           {18, 20},
	"settings_state_set":                   {10, 14},
	"settings_state_not_set":               {12, 18},
	"settings_state_default":               {12, 18},
	"settings_edit_line_text":              {40, 56},
	"settings_edit_line_media":             {40, 56},
	"settings_edit_line_buttons":           {40, 56},
	"settings_edit_state_text_len":         {30, 42},
	"settings_edit_state_buttons_n":        {30, 42},
	"settings_edit_media_photo":            {14, 20},
	"settings_edit_media_video":            {14, 20},
	"settings_edit_media_file":             {16, 24},
	"settings_edit_media_sticker":          {14, 20},
	"settings_edit_media_audio":            {14, 20},
	"settings_edit_media_voice":            {14, 20},
	"settings_edit_media_video_note":       {16, 24},
	"settings_edit_note_no_caption":        {90, 126},
	"settings_edit_hint_greeting":          {170, 240},
	"settings_edit_hint_rules":             {140, 196},
	"settings_edit_clean_hint_welcome":     {150, 210},
	"settings_edit_clean_hint_goodbye":     {150, 210},
	"settings_edit_line_welcome":           {20, 28},
	"settings_edit_line_goodbye":           {20, 28},
	"settings_edit_line_clean_welcome":     {28, 40},
	"settings_edit_line_clean_goodbye":     {28, 40},
	"settings_btn_edit_set_media":          {18, 20},
	"settings_btn_edit_set_buttons":        {18, 20},
	"settings_btn_edit_remove_text":        {18, 20},
	"settings_btn_edit_remove_media":       {18, 20},
	"settings_btn_edit_remove_buttons":     {18, 20},
	"settings_btn_edit_preview_text":       {18, 20},
	"settings_btn_edit_preview_media":      {18, 20},
	"settings_btn_edit_preview_buttons":    {18, 20},
	"settings_btn_edit_full_preview":       {18, 20},
	"settings_btn_edit_cancel":             {12, 20},
	"settings_edit_wait_text":              {420, 590},
	"settings_edit_wait_text_rules":        {240, 340},
	"settings_edit_wait_media":             {230, 320},
	"settings_edit_wait_buttons":           {340, 480},
	"settings_edit_wait_footer":            {110, 154},
	"settings_edit_replaced_note":          {80, 112},
	"settings_edit_removed_text_default":   {70, 98},
	"settings_edit_removed_text_rules":     {50, 70},
	"settings_edit_removed_media":          {40, 56},
	"settings_edit_removed_buttons":        {40, 56},
	"settings_edit_nothing_to_remove":      {60, 84},
	"settings_edit_wait_stopped":           {60, 84},
	"settings_edit_cancelled":              {50, 70},
	"settings_edit_cancel_nothing":         {50, 70},
	"settings_edit_group_note":             {110, 154},
	"settings_edit_prompt_closed":          {40, 56},
	"settings_edit_unavailable":            {120, 160},
	"settings_edit_prompt_stale":           {120, 160},
	"settings_edit_wait_expired":           {120, 160},
	"settings_edit_saved_text":             {40, 56},
	"settings_edit_still_waiting":          {70, 98},
	"settings_edit_saved_media":            {40, 56},
	"settings_edit_saved_buttons":          {40, 56},
	"settings_edit_album_cut":              {80, 112},
	"settings_edit_caption_ignored":        {90, 126},
	"settings_edit_nothing_to_preview":     {120, 160},
	"settings_edit_preview_sent":           {120, 160},
	"settings_edit_preview_need_start":     {120, 160},
	"settings_edit_preview_failed":         {120, 160},
	"settings_edit_refuse_need_text":       {60, 84},
	"settings_edit_refuse_need_media":      {130, 182},
	"settings_edit_refuse_too_long_filled": {150, 210},
	"settings_edit_refuse_media_text_long": {170, 238},
	"settings_edit_refuse_buttons_syntax":  {160, 224},
	"settings_edit_refuse_button_url":      {110, 154},
	"settings_edit_refuse_buttons_limit":   {120, 168},
	"settings_edit_refuse_text_buttons":    {150, 210},
	"settings_edit_refuse_access_lost":     {140, 196},
	"settings_edit_preview_header":         {90, 126},
	"settings_edit_preview_header_rules":   {50, 70},
	"settings_edit_preview_buttons_body":   {40, 56},
	"settings_edit_wait_ended_note":        {120, 168},
	"settings_edit_part_text":              {14, 20},
	"settings_edit_part_media":             {14, 20},
	"settings_edit_part_buttons":           {14, 20},
	"settings_no_change_info":              {120, 160},

	// Phase 12 (plan 12-05). The five switch rows count the template as written, with {state}
	// as 7 characters, so a correct 18-character name still fits; the binding ceiling is the
	// rendered 20 of TestSettingsLabelLengths, where {state} is the one-code-point icon.
	"settings_btn_edit_switch_welcome":      {26, 26},
	"settings_btn_edit_switch_goodbye":      {26, 26},
	"settings_btn_edit_switch_clean":        {26, 26},
	"settings_btn_edit_switch_below":        {26, 26},
	"settings_btn_edit_switch_once":         {26, 26},
	"settings_edit_line_below":              {40, 56},
	"settings_edit_line_once":               {40, 56},
	"settings_edit_hint_once":               {160, 224},
	"settings_edit_hint_below_file":         {140, 196},
	"settings_edit_media_animation":         {14, 20},
	"settings_edit_refuse_media_rules_long": {170, 238},
	"settings_edit_below_unavailable":       {100, 140},
	"settings_edit_refuse_text_rules_long":  {120, 168},

	// Phase 13 (plan 13-04), the Member requirements page. The seven cell rows count the
	// template as written, with {state} as 7 characters, so a correct 18-character name still
	// fits; the binding ceiling is the rendered 20 of TestSettingsLabelLengths, where {state}
	// is the one-code-point icon.
	"settings_page_mreq":              {14, 18},
	"settings_state_mreq_on":          {14, 20},
	"settings_mreq_intro":             {110, 154},
	"settings_mreq_all_off":           {60, 84},
	"settings_mreq_line":              {20, 28},
	"settings_mreq_rule_lastname":     {28, 40},
	"settings_mreq_rule_username":     {28, 40},
	"settings_mreq_rule_russian":      {28, 40},
	"settings_mreq_rule_arabic":       {28, 40},
	"settings_mreq_rule_chinese":      {28, 40},
	"settings_mreq_line_join":         {40, 56},
	"settings_mreq_line_delete":       {40, 56},
	"settings_mreq_hint_letters":      {280, 392},
	"settings_mreq_hint_blank":        {100, 140},
	"settings_mreq_hint_join":         {240, 336},
	"settings_mreq_hint_delete":       {130, 182},
	"settings_mreq_hint_mute":         {220, 308},
	"settings_btn_mreq_lastname":      {26, 26},
	"settings_btn_mreq_username":      {26, 26},
	"settings_btn_mreq_russian":       {26, 26},
	"settings_btn_mreq_arabic":        {26, 26},
	"settings_btn_mreq_chinese":       {26, 26},
	"settings_btn_mreq_switch_join":   {26, 26},
	"settings_btn_mreq_switch_delete": {26, 26},
	"settings_mreq_desc_lastname":     {70, 98},
	"settings_mreq_desc_username":     {70, 98},
	"settings_mreq_desc_russian":      {170, 238},
	"settings_mreq_desc_arabic":       {170, 238},
	"settings_mreq_desc_chinese":      {170, 238},
	"settings_mreq_now":               {40, 56},
	"settings_mreq_choose_hint":       {100, 140},
	"settings_mreq_actions_hint":      {190, 266},
	"settings_mreq_confirm":           {40, 56},
	"settings_mreq_confirm_kick":      {170, 238},
	"settings_mreq_confirm_ban":       {170, 238},
	"settings_mreq_confirm_mute":      {180, 252},

	// Phase 14 (plan 14-02), the photo and channel rules. The two cell rows count the template
	// as written; the binding ceiling is the rendered 20 of TestSettingsLabelLengths. The
	// three here rows with {rule} are line templates, a translated rule name is spliced in.
	"settings_mreq_rule_photo":    {32, 44},
	"settings_mreq_rule_channel":  {32, 44},
	"settings_btn_mreq_photo":     {26, 26},
	"settings_btn_mreq_channel":   {26, 26},
	"settings_mreq_desc_photo":    {200, 280},
	"settings_mreq_desc_channel":  {160, 224},
	"settings_mreq_hint_photo":    {140, 196},
	"settings_mreq_hint_lookup":   {170, 238},
	"settings_mreq_here_basic":    {110, 154},
	"settings_mreq_here_nolink":   {90, 126},
	"settings_mreq_here_notadmin": {100, 140},
	"settings_mreq_here_unknown":  {90, 126},

	// Phase 15 (plan 15-03), the AI spam-name rule (rows 1 to 8 of the UI-SPEC "New settings_
	// keys" table, 255 rows in all). The cell row counts the template as written; the binding
	// ceiling is the rendered 20 of TestSettingsLabelLengths. The here row is also an alert.
	"settings_mreq_rule_ai":         {32, 44},
	"settings_btn_mreq_ai":          {26, 26},
	"settings_mreq_desc_ai":         {280, 392},
	"settings_mreq_hint_ai":         {190, 266},
	"settings_mreq_here_ai":         {90, 126},
	"settings_mreq_confirm_ai_kick": {216, 302},
	"settings_mreq_confirm_ai_ban":  {206, 288},
	"settings_mreq_confirm_ai_mute": {226, 316},

	// Phase 18 (plan 18-01), the Warnings page (rows 1, 7, 10 and 13 of the UI-SPEC "New settings_
	// keys" table). The page name is a grid cell whose binding ceiling is the rendered 20 of
	// TestSettingsLabelLengths.
	"settings_page_warns":  {18, 18},
	"settings_state_warns": {35, 40},
	"settings_warns_limit": {30, 35},
	"settings_warns_hint":  {85, 110},

	// Phase 18 (plan 18-02), the Switches, Language, Log channel and Anonymous admins pages (rows 2
	// to 6, 8, 9, 11, 12 and 14 to 30 of the same table). The page names, the log category names
	// and the lock names below are read through table fields, so make check-translations cannot
	// see them: this table and TestSettingsLabelLengths are their guard.
	"settings_page_switches":      {18, 18},
	"settings_page_lang":          {18, 18},
	"settings_page_log":           {18, 18},
	"settings_page_anon":          {18, 18},
	"settings_page_locks":         {18, 18},
	"settings_state_sw_on":        {15, 15},
	"settings_state_locks_on":     {20, 25},
	"settings_warns_action":       {45, 55},
	"settings_warns_action_inert": {85, 105},
	"settings_btn_sw_reports":     {20, 30},
	"settings_btn_sw_aispam":      {30, 35},
	"settings_btn_sw_approve":     {35, 35},
	"settings_sw_ai_off":          {60, 70},
	"settings_sw_ai_unavailable":  {95, 120},
	"settings_lang_current":       {35, 45},
	"settings_lang_hint":          {130, 165},
	"settings_log_status":         {35, 45},
	"settings_log_hint":           {110, 140},
	"settings_log_cat_settings":   {18, 18},
	"settings_log_cat_admin":      {18, 18},
	"settings_log_cat_user":       {18, 18},
	"settings_log_cat_automated":  {18, 18},
	"settings_log_cat_reports":    {18, 18},
	"settings_log_cat_other":      {18, 18},
	"settings_anon_hint":          {90, 110},
	"settings_btn_anon_toggle":    {30, 35},

	// Phase 18 (plan 18-02), the Locks page (rows 31 to 55): the intro, the two state words and
	// the 22 lock names, each name read through the table field NameKey and so a grid cell whose
	// binding ceiling is the rendered 20 of TestSettingsLabelLengths.
	"settings_locks_intro":      {145, 185},
	"settings_locks_state_on":   {15, 15},
	"settings_locks_state_off":  {15, 20},
	"settings_lock_sticker":     {18, 18},
	"settings_lock_gif":         {18, 18},
	"settings_lock_photo":       {18, 18},
	"settings_lock_video":       {18, 18},
	"settings_lock_videonote":   {18, 18},
	"settings_lock_voice":       {18, 18},
	"settings_lock_audio":       {18, 18},
	"settings_lock_document":    {18, 18},
	"settings_lock_contact":     {18, 18},
	"settings_lock_location":    {18, 18},
	"settings_lock_game":        {18, 18},
	"settings_lock_url":         {18, 18},
	"settings_lock_forward":     {18, 18},
	"settings_lock_rtl":         {18, 18},
	"settings_lock_anonchannel": {18, 18},
	"settings_lock_bots":        {18, 18},
	"settings_lock_messages":    {18, 18},
	"settings_lock_comments":    {18, 18},
	"settings_lock_media":       {18, 18},
	"settings_lock_other":       {18, 18},
	"settings_lock_previews":    {18, 18},
	"settings_lock_all":         {18, 18},

	// Phase 16 (plan 16-07), the Deletion page: the Main cell's name and state word. The page
	// name is a grid cell whose binding ceiling is the rendered 20 of TestSettingsLabelLengths.
	"settings_page_del":     {14, 18},
	"settings_state_del_on": {14, 20},

	// Phase 16 (plan 16-07), the Deletion page (rows 3 to 57 of the UI-SPEC "New settings_ keys"
	// table, 312 rows in all with the two above). A name row counts the template, which has no
	// placeholder, so its binding ceiling is the rendered cell of 20 in TestSettingsLabelLengths.
	"settings_del_intro":             {125, 176},
	"settings_del_sec_service":       {18, 18},
	"settings_del_sec_bot":           {18, 18},
	"settings_del_list_svc":          {125, 176},
	"settings_del_list_bot":          {65, 92},
	"settings_del_hint_time":         {85, 120},
	"settings_del_hint_cleanservice": {75, 106},
	"settings_del_hint_punish_auto":  {120, 168},
	"settings_del_no_right":          {135, 190},
	"settings_del_right_unknown":     {90, 126},
	"settings_del_state_keep":        {10, 12},
	"settings_del_state_now":         {11, 13},
	"settings_del_state_after":       {20, 28},
	"settings_del_delay_10s":         {6, 10},
	"settings_del_delay_1m":          {6, 10},
	"settings_del_delay_10m":         {6, 10},
	"settings_del_delay_1h":          {6, 10},
	"settings_del_delay_6h":          {6, 10},
	"settings_del_delay_24h":         {6, 10},
	"settings_del_delay_custom":      {10, 12},
	"settings_del_currently":         {25, 36},
	"settings_del_svc_joined":        {18, 18},
	"settings_del_svc_left":          {18, 18},
	"settings_del_svc_photo":         {18, 18},
	"settings_del_svc_title":         {18, 18},
	"settings_del_svc_pinned":        {18, 18},
	"settings_del_svc_video":         {18, 18},
	"settings_del_svc_forum":         {18, 18},
	"settings_del_svc_boost":         {18, 18},
	"settings_del_svc_timer":         {18, 18},
	"settings_del_svc_background":    {18, 18},
	"settings_del_svc_gifts":         {18, 18},
	"settings_del_svc_other":         {18, 18},
	"settings_del_desc_joined":       {70, 98},
	"settings_del_desc_left":         {70, 98},
	"settings_del_desc_photo":        {65, 92},
	"settings_del_desc_title":        {50, 70},
	"settings_del_desc_pinned":       {85, 120},
	"settings_del_desc_video":        {85, 120},
	"settings_del_desc_forum":        {115, 162},
	"settings_del_desc_boost":        {50, 70},
	"settings_del_desc_timer":        {75, 106},
	"settings_del_desc_background":   {60, 84},
	"settings_del_desc_gifts":        {45, 64},
	"settings_del_desc_other":        {150, 210},
	"settings_del_bot_welcome":       {18, 18},
	"settings_del_bot_goodbye":       {18, 18},
	"settings_del_bot_rules":         {18, 18},
	"settings_del_bot_auto":          {18, 18},
	"settings_del_bot_manual":        {18, 18},
	"settings_del_desc_welcome":      {55, 78},
	"settings_del_desc_goodbye":      {55, 78},
	"settings_del_desc_rules":        {95, 134},
	"settings_del_desc_auto":         {110, 154},
	"settings_del_desc_manual":       {105, 148},
	"settings_del_sec_member":        {18, 18},
	"settings_del_list_mem":          {75, 105},
	"settings_del_mem_commands":      {18, 18},
	"settings_del_mem_edited":        {18, 18},
	"settings_del_mem_self":          {18, 18},
	"settings_del_desc_commands":     {120, 170},
	"settings_del_desc_edited":       {130, 185},
	"settings_del_desc_self":         {150, 210},
	"settings_del_hint_commands":     {75, 105},
	"settings_del_hint_edited":       {150, 210},
	"settings_del_hint_self":         {175, 245},
	"settings_del_hint_grace":        {85, 120},
	"settings_del_state_on":          {6, 12},
	"settings_del_state_grace":       {20, 28},
	"settings_del_delay_30s":         {6, 10},
	"settings_del_delay_5m":          {6, 10},
	"settings_del_delay_15m":         {6, 10},
	"settings_del_confirm_self":      {220, 310},

	// Phase 19 (plan 19-02), the list pages' group-menu strings (UI-SPEC rows 1 and 2).
	"settings_state_private_only": {16, 20},
	"settings_entries_group_note": {140, 200},

	// Phase 19 (plan 19-03, task 1), the Blacklist page (UI-SPEC rows 15 to 20 and 27).
	"settings_page_blacklist":  {18, 18},
	"settings_bl_intro":        {230, 320},
	"settings_bl_count":        {16, 24},
	"settings_bl_empty":        {110, 170},
	"settings_bl_action_line":  {30, 40},
	"settings_bl_action_mixed": {12, 16},
	"settings_bl_opt_none":     {15, 15},

	// Phase 19 (plan 19-03, task 2), the delete card and its notices (UI-SPEC rows 3, 32 and 33).
	"settings_entries_already_removed": {70, 110},
	"settings_bl_delete_ask":           {120, 180},
	"settings_bl_removed":              {64, 100},

	// Phase 19 (plan 19-03, task 3), the Blacklist action screen (UI-SPEC rows 21 to 26 and 29).
	"settings_bl_action_count":      {14, 16},
	"settings_bl_action_intro":      {70, 110},
	"settings_bl_action_mixed_hint": {90, 140},
	"settings_bl_action_hint":       {140, 200},
	"settings_bl_action_inert":      {56, 90},
	"settings_bl_action_no_words":   {90, 140},
	"settings_btn_bl_action":        {20, 20},

	// Phase 19 (plan 19-04, task 1), the Blacklist typed add: the unavailable alert, the Add button, the prompt and the
	// saved-notice count lines (UI-SPEC rows 4, 28, 30, 31 and 34 to 38).
	"settings_entries_add_unavailable": {90, 150},
	"settings_btn_bl_add":              {20, 20},
	"settings_bl_wait":                 {200, 290},
	"settings_bl_wait_rules":           {160, 230},
	"settings_bl_saved_added":          {16, 28},
	"settings_bl_saved_already":        {32, 50},
	"settings_bl_saved_too_long":       {48, 70},
	"settings_bl_saved_too_broad":      {60, 90},
	"settings_bl_saved_failed":         {34, 54},

	// Phase 19 (plan 19-04, task 2), the door's list replies: the wait-ended note, the five part words and the refusal's
	// first line (UI-SPEC rows 5 to 10 and 39).
	"settings_entries_wait_ended_note": {100, 160},
	"settings_entries_part_words":      {16, 24},
	"settings_entries_part_keyword":    {16, 24},
	"settings_entries_part_reply":      {16, 24},
	"settings_entries_part_name":       {16, 24},
	"settings_entries_part_content":    {16, 24},
	"settings_bl_refuse_none":          {48, 80},

	// Phase 19 (plan 19-06), the two-step add's step line, the closed step-1 prompt, the step-2 content limits
	// and the blank-entry refusal (UI-SPEC rows 11 to 14).
	"settings_entries_step":                {24, 32},
	"settings_entries_step_done":           {60, 90},
	"settings_entries_refuse_blank":        {64, 100},
	"settings_entries_wait_content_limits": {76, 110},

	// Phase 19 (plan 19-07, task 1), the Filters page: its name, list page, Add button, both prompts and the saved
	// notice (UI-SPEC rows 40 to 48 and 53).
	"settings_page_filters":           {18, 18},
	"settings_flt_intro":              {150, 220},
	"settings_flt_count":              {28, 40},
	"settings_flt_empty":              {40, 64},
	"settings_btn_flt_add":            {20, 20},
	"settings_flt_wait_keyword":       {190, 280},
	"settings_flt_wait_keyword_rules": {150, 220},
	"settings_flt_keyword_line":       {40, 50},
	"settings_flt_wait_reply":         {300, 420},
	"settings_flt_saved":              {96, 150},

	// Phase 19 (plan 19-07, task 2), the Filters page's refusals, delete card and removed notice (UI-SPEC rows 49 to 52).
	"settings_flt_refuse_exists": {120, 180},
	"settings_flt_refuse_cap":    {100, 150},
	"settings_flt_delete_ask":    {150, 220},
	"settings_flt_removed":       {64, 100},

	// Phase 19 (plan 19-09, task 1), the Notes page: its name, list page, admin tag, Add button, both prompts and the
	// saved notice (UI-SPEC rows 54 to 63 and 67).
	"settings_page_notes":            {18, 18},
	"settings_notes_intro":           {110, 170},
	"settings_notes_count":           {16, 24},
	"settings_notes_empty":           {40, 64},
	"settings_notes_admin_tag":       {16, 22},
	"settings_btn_notes_add":         {20, 20},
	"settings_notes_wait_name":       {170, 250},
	"settings_notes_wait_name_rules": {80, 120},
	"settings_notes_name_line":       {32, 44},
	"settings_notes_wait_content":    {390, 540},
	"settings_notes_saved":           {100, 160},
}

// settingsAlertKeys are shown as answerCallbackQuery alerts: plain text, no tag, at most
// 200 characters in every language (Telegram's cap).
var settingsAlertKeys = []string{
	"settings_entries_add_unavailable",
	"settings_not_your_menu",
	"settings_sw_ai_unavailable",
	"settings_not_admin",
	"settings_no_restrict",
	"settings_check_failed",
	"settings_min_value",
	"settings_max_value",
	"settings_cannot_open_alert",
	"settings_gate_owner_only",
	"settings_access_owner_only",
	"settings_pick_unavailable",
	"settings_edit_unavailable",
	"settings_edit_prompt_stale",
	"settings_edit_wait_expired",
	"settings_edit_nothing_to_preview",
	"settings_edit_preview_sent",
	"settings_edit_preview_need_start",
	"settings_edit_preview_failed",
	"settings_no_change_info",
	"settings_mreq_here_nolink",
	"settings_mreq_here_ai",
}

// settingsAlertMaxRunes is the length Telegram allows an alert.
const settingsAlertMaxRunes = 200

var (
	settingsTagPattern  = regexp.MustCompile(`</?([A-Za-z][A-Za-z0-9]*)[^>]*>`)
	settingsVerbPattern = regexp.MustCompile(`%[a-zA-Z]`)
)

// settingsKeys returns every settings_ key of a locale.
func settingsKeys(data map[string]any) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range data {
		if strings.HasPrefix(key, settingsKeyPrefix) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// TestSettingsLocaleKeys guards every settings_ string across all 8 locales. make
// check-translations only sees keys that are literal GetString arguments, so it cannot
// catch a key that is empty, too long for the layout, carrying markup it should not or a
// renamed placeholder.
func TestSettingsLocaleKeys(t *testing.T) {
	english := loadStaffLocale(t, "en")
	englishKeys := settingsKeys(english)
	if len(englishKeys) == 0 {
		t.Fatal("en.yml has no settings_ keys")
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			keys := settingsKeys(data)

			var failures []string
			for key := range keys {
				if _, ok := settingsLocaleLimits[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: the limits table has no row for %s", lang, key))
				}
			}
			for key := range settingsLocaleLimits {
				if _, ok := keys[key]; !ok {
					failures = append(failures, fmt.Sprintf("%s: missing key %s", lang, key))
				}
			}

			for key, limit := range settingsLocaleLimits {
				value, ok := data[key]
				if !ok {
					continue
				}
				text, isString := value.(string)
				if !isString || strings.TrimSpace(text) == "" {
					failures = append(failures, fmt.Sprintf("%s: empty or non-string value for %s", lang, key))
					continue
				}

				ceiling := limit.other
				if lang == "en" {
					ceiling = limit.max
				}
				if runes := utf8.RuneCountInString(text); runes > ceiling {
					failures = append(failures, fmt.Sprintf("%s: %s has %d characters, ceiling %d", lang, key, runes, ceiling))
				}

				englishText, _ := english[key].(string)
				want := staffPlaceholders(englishText)
				if got := staffPlaceholders(text); !slices.Equal(got, want) {
					failures = append(failures, fmt.Sprintf("%s: %s has placeholders %v, en.yml has %v", lang, key, got, want))
				}

				if settingsVerbPattern.MatchString(text) {
					failures = append(failures, fmt.Sprintf("%s: %s holds a printf verb", lang, key))
				}

				open := map[string]int{}
				for _, match := range settingsTagPattern.FindAllStringSubmatch(text, -1) {
					tag := strings.ToLower(match[1])
					if tag != "b" && tag != "code" {
						failures = append(failures, fmt.Sprintf("%s: %s uses the tag <%s>, only b and code are allowed", lang, key, tag))
						continue
					}
					if strings.HasPrefix(match[0], "</") {
						open[tag]--
					} else {
						open[tag]++
					}
				}
				for tag, balance := range open {
					if balance != 0 {
						failures = append(failures, fmt.Sprintf("%s: %s has unbalanced <%s> tags", lang, key, tag))
					}
				}
			}

			for _, key := range settingsAlertKeys {
				text, _ := data[key].(string)
				if strings.ContainsAny(text, "<>") {
					failures = append(failures, fmt.Sprintf("%s: alert key %s holds markup", lang, key))
				}
				if runes := utf8.RuneCountInString(text); runes > settingsAlertMaxRunes {
					failures = append(failures, fmt.Sprintf("%s: alert key %s has %d characters, Telegram allows %d", lang, key, runes, settingsAlertMaxRunes))
				}
			}

			failures = append(failures, settingsAIAlertFailures(t, lang, data)...)

			sort.Strings(failures)
			for _, failure := range failures {
				t.Error(failure)
			}
		})
	}
}

// settingsAIAlertFailures renders the AI rule's refused-press alert the way the gate sends it,
// with the longest translated rule name of the language as {rule}, and holds it to Telegram's
// 200 (the template check alone would not count the name).
func settingsAIAlertFailures(t *testing.T, lang string, data map[string]any) []string {
	t.Helper()
	template, ok := data["settings_mreq_here_ai"].(string)
	if !ok {
		// A missing key is already reported by the coverage check.
		return nil
	}
	longest := ""
	for key, value := range data {
		name, _ := value.(string)
		if strings.HasPrefix(key, "settings_mreq_rule_") && utf8.RuneCountInString(name) > utf8.RuneCountInString(longest) {
			longest = name
		}
	}
	rendered := strings.ReplaceAll(template, "{rule}", longest)
	if runes := utf8.RuneCountInString(rendered); runes > settingsAlertMaxRunes {
		return []string{fmt.Sprintf("%s: settings_mreq_here_ai rendered with the longest rule name has %d characters, Telegram allows %d", lang, runes, settingsAlertMaxRunes)}
	}
	return nil
}

// settingsLocaleString reads one string value of a locale and fails the test when it is
// missing or not a string.
func settingsLocaleString(t *testing.T, lang string, data map[string]any, key string) string {
	t.Helper()
	text, ok := data[key].(string)
	if !ok {
		t.Fatalf("%s: %s is missing or not a string", lang, key)
	}
	return text
}

// TestSettingsLabelLengths holds the composite buttons to the UI-SPEC sizing checks, in
// code points. A two-column grid button (the page's icon, a space and its name) is at most
// 20; the Access toggle rendered with its state (a check or cross icon and the language's
// On or Off word, the longer of the two) is at most 40; the Groups button is at most 18; and a
// choice-row button (a check mark and the option) is at most 13 in English and 15 in the
// others. The Phase 9 ceiling for state inside the Main button no longer applies: state
// moved to the status block of the Main page (UI-SPEC "Changes to Phase 9 rules"). The icons
// are the code constants of the modules, written literally here because fuku/i18n cannot
// import fuku/modules.
func TestSettingsLabelLengths(t *testing.T) {
	const (
		gridMax        = 20
		accessToggle   = 40
		groupsButton   = 18
		choiceMaxOther = 15
		choiceMaxEn    = 13
		// editorCellMax is the ceiling of a half-width switch cell rendered with its icon alone.
		editorCellMax = 20
	)
	type gridPage struct {
		icon string
		page string
	}
	pages := []gridPage{
		{"👥", "settings_page_staff"},
		{"🔒", "settings_page_lockdown"},
		{"🧩", "settings_page_captcha"},
		{"🌊", "settings_page_antiflood"},
		{"🔑", "settings_page_access"},
		{"🎉", "settings_page_welcome"},
		{"👋", "settings_page_goodbye"},
		{"📜", "settings_page_rules"},
		{"🛂", "settings_page_mreq"},
		{"🧹", "settings_page_del"},
		{"🚩", "settings_page_warns"},
		{"🔘", "settings_page_switches"},
		{"🌐", "settings_page_lang"},
		{"🧾", "settings_page_log"},
		{"🎭", "settings_page_anon"},
		{"🔐", "settings_page_locks"},
		{"🔤", "settings_page_blacklist"},
		{"🔁", "settings_page_filters"},
		{"📝", "settings_page_notes"},
	}
	options := []string{
		"settings_opt_math", "settings_opt_text", "settings_opt_turnstile",
		"settings_opt_kick", "settings_opt_ban", "settings_opt_mute",
		// The Member requirements rule screen's Off and Warn choices (Phase 13).
		"settings_state_off_plain", "settings_opt_warn",
		// The Deletion row screens' choices (Phase 16): Keep, Delete now and the six delay labels.
		"settings_del_state_keep", "settings_del_state_now", "settings_del_delay_10s", "settings_del_delay_1m",
		"settings_del_delay_10m", "settings_del_delay_1h", "settings_del_delay_6h", "settings_del_delay_24h",
		// The Member messages row screens' new choices (Phase 17): On and the three new delay labels.
		"settings_del_state_on", "settings_del_delay_30s", "settings_del_delay_5m", "settings_del_delay_15m",
	}
	// The Deletion page's list cells (Phase 16, UI-SPEC check 2): a state icon, one space and the
	// name, so every name is at most 18 code points whichever icon the row shows.
	deletionNames := []string{
		"settings_del_sec_service", "settings_del_sec_bot",
		"settings_del_svc_joined", "settings_del_svc_left", "settings_del_svc_photo", "settings_del_svc_title",
		"settings_del_svc_pinned", "settings_del_svc_video", "settings_del_svc_forum", "settings_del_svc_boost",
		"settings_del_svc_timer", "settings_del_svc_background", "settings_del_svc_gifts", "settings_del_svc_other",
		"settings_del_bot_welcome", "settings_del_bot_goodbye", "settings_del_bot_rules", "settings_del_bot_auto",
		"settings_del_bot_manual",
		// The Member messages section and its three rows (Phase 17, UI-SPEC check 2).
		"settings_del_sec_member", "settings_del_mem_commands", "settings_del_mem_edited", "settings_del_mem_self",
	}
	deletionIcons := []string{"➖", "🗑", "⏳", "💬"}
	// The Locks page's cells (Phase 18, UI-SPEC check C18-2): the lock icon, one space and the
	// name, so every lock name is at most 18 code points; the Log channel's six category names
	// are drawn with the check or the cross.
	lockNames := []string{
		"settings_lock_sticker",
		"settings_lock_gif",
		"settings_lock_photo",
		"settings_lock_video",
		"settings_lock_videonote",
		"settings_lock_voice",
		"settings_lock_audio",
		"settings_lock_document",
		"settings_lock_contact",
		"settings_lock_location",
		"settings_lock_game",
		"settings_lock_url",
		"settings_lock_forward",
		"settings_lock_rtl",
		"settings_lock_anonchannel",
		"settings_lock_bots",
		"settings_lock_messages",
		"settings_lock_comments",
		"settings_lock_media",
		"settings_lock_other",
		"settings_lock_previews",
		"settings_lock_all",
	}
	lockIcons := []string{"🔒", "🔓"}
	logCategoryNames := []string{
		"settings_log_cat_settings", "settings_log_cat_admin", "settings_log_cat_user",
		"settings_log_cat_automated", "settings_log_cat_reports", "settings_log_cat_other",
	}
	// The full-width toggles of the Switches and Anonymous admins pages (Phase 18): each is
	// rendered with the check or cross icon and the language's longer On or Off word.
	phase18Toggles := []string{"settings_btn_sw_reports", "settings_btn_sw_aispam", "settings_btn_sw_approve", "settings_btn_anon_toggle"}
	// The editor's two-column buttons (UI-SPEC rows 28 to 38): at most 20 code points as
	// written, icon included.
	editorButtons := []string{
		"settings_btn_edit_set_text", "settings_btn_edit_set_media", "settings_btn_edit_set_buttons",
		"settings_btn_edit_remove_text", "settings_btn_edit_remove_media", "settings_btn_edit_remove_buttons",
		"settings_btn_edit_preview_text", "settings_btn_edit_preview_media", "settings_btn_edit_preview_buttons",
		"settings_btn_edit_full_preview", "settings_btn_edit_cancel",
		// The Blacklist list page's Action button (Phase 19, UI-SPEC check C19-2).
		"settings_btn_bl_action",
		// The Blacklist list page's Add button (Phase 19, UI-SPEC check C19-2).
		"settings_btn_bl_add",
		// The Filters list page's Add button (Phase 19, UI-SPEC check C19-2).
		"settings_btn_flt_add",
		// The Notes list page's Add button (Phase 19, UI-SPEC check C19-2).
		"settings_btn_notes_add",
	}
	// The editor's half-width switch cells (Phase 12, UI-SPEC check 2): the template with
	// {state} replaced by the icon alone is at most 20 code points, so a name is at most 18.
	editorCells := []string{
		"settings_btn_edit_switch_welcome", "settings_btn_edit_switch_goodbye", "settings_btn_edit_switch_clean",
		"settings_btn_edit_switch_below", "settings_btn_edit_switch_once",
	}
	// The Member requirements rule cells carry the rule's punishment icon in place of {state}
	// (Phase 13, UI-SPEC check 2); the two switch cells carry the check mark or the cross.
	mreqRuleCells := []string{
		"settings_btn_mreq_lastname", "settings_btn_mreq_username", "settings_btn_mreq_russian",
		"settings_btn_mreq_arabic", "settings_btn_mreq_chinese", "settings_btn_mreq_photo",
		"settings_btn_mreq_channel", "settings_btn_mreq_ai",
	}
	mreqRuleIcons := []string{"❌", "❗", "👢", "⛔", "🔇"}
	mreqSwitchCells := []string{"settings_btn_mreq_switch_join", "settings_btn_mreq_switch_delete"}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			choiceMax := choiceMaxOther
			if lang == "en" {
				choiceMax = choiceMaxEn
			}

			for _, page := range pages {
				label := page.icon + " " + settingsLocaleString(t, lang, data, page.page)
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: grid label %q has %d characters, ceiling %d", lang, label, runes, gridMax)
				}
			}

			toggle := settingsLocaleString(t, lang, data, "settings_btn_access_toggle")
			for _, state := range []struct{ icon, key string }{
				{"✅ ", "common_status_on"},
				{"❌ ", "common_status_off"},
			} {
				rendered := strings.ReplaceAll(toggle, "{state}", state.icon+settingsLocaleString(t, lang, data, state.key))
				if runes := utf8.RuneCountInString(rendered); runes > accessToggle {
					t.Errorf("%s: Access toggle %q has %d characters, ceiling %d", lang, rendered, runes, accessToggle)
				}
			}

			groups := settingsLocaleString(t, lang, data, "settings_btn_groups")
			if runes := utf8.RuneCountInString(groups); runes > groupsButton {
				t.Errorf("%s: Groups button %q has %d characters, ceiling %d", lang, groups, runes, groupsButton)
			}

			for _, key := range editorButtons {
				label := settingsLocaleString(t, lang, data, key)
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: editor button %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
				}
			}

			for _, key := range editorCells {
				template := settingsLocaleString(t, lang, data, key)
				for _, icon := range []string{"✅", "❌"} {
					rendered := strings.ReplaceAll(template, "{state}", icon)
					if runes := utf8.RuneCountInString(rendered); runes > editorCellMax {
						t.Errorf("%s: editor cell %q (%s) has %d characters, ceiling %d", lang, rendered, key, runes, editorCellMax)
					}
				}
			}

			for _, group := range []struct {
				keys  []string
				icons []string
			}{
				{mreqRuleCells, mreqRuleIcons},
				{mreqSwitchCells, []string{"✅", "❌"}},
			} {
				for _, key := range group.keys {
					template := settingsLocaleString(t, lang, data, key)
					for _, icon := range group.icons {
						rendered := strings.ReplaceAll(template, "{state}", icon)
						if runes := utf8.RuneCountInString(rendered); runes > editorCellMax {
							t.Errorf("%s: requirements cell %q (%s) has %d characters, ceiling %d", lang, rendered, key, runes, editorCellMax)
						}
					}
				}
			}

			for _, key := range deletionNames {
				for _, icon := range deletionIcons {
					label := icon + " " + settingsLocaleString(t, lang, data, key)
					if runes := utf8.RuneCountInString(label); runes > gridMax {
						t.Errorf("%s: Deletion cell %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
					}
				}
			}

			for _, key := range lockNames {
				for _, icon := range lockIcons {
					label := icon + " " + settingsLocaleString(t, lang, data, key)
					if runes := utf8.RuneCountInString(label); runes > gridMax {
						t.Errorf("%s: Locks cell %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
					}
				}
			}

			for _, key := range logCategoryNames {
				for _, icon := range []string{"✅", "❌"} {
					label := icon + " " + settingsLocaleString(t, lang, data, key)
					if runes := utf8.RuneCountInString(label); runes > gridMax {
						t.Errorf("%s: Log channel cell %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
					}
				}
			}

			// The Language grid shows every language in its own words, so each file's own flag and
			// name are the cell, plain and marked (UI-SPEC M36).
			cell := settingsLocaleString(t, lang, data, "language_flag") + " " + settingsLocaleString(t, lang, data, "language_name")
			for _, label := range []string{cell, "✅ " + settingsLocaleString(t, lang, data, "language_name")} {
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: Language cell %q has %d characters, ceiling %d", lang, label, runes, gridMax)
				}
			}

			for _, key := range phase18Toggles {
				template := settingsLocaleString(t, lang, data, key)
				for _, state := range []struct{ icon, key string }{
					{"✅ ", "common_status_on"},
					{"❌ ", "common_status_off"},
				} {
					rendered := strings.ReplaceAll(template, "{state}", state.icon+settingsLocaleString(t, lang, data, state.key))
					if runes := utf8.RuneCountInString(rendered); runes > accessToggle {
						t.Errorf("%s: toggle %q (%s) has %d characters, ceiling %d", lang, rendered, key, runes, accessToggle)
					}
				}
			}

			// The two Main state words with their largest count: three switches, 22 locks.
			for _, state := range []struct {
				key, n string
				max    int
			}{
				{"settings_state_sw_on", "3", 15},
				{"settings_state_locks_on", "22", 25},
				// A list page's fixed Main state in a group menu (Phase 19): at most 20 in every language.
				{"settings_state_private_only", "", 20},
			} {
				rendered := strings.ReplaceAll(settingsLocaleString(t, lang, data, state.key), "{n}", state.n)
				if runes := utf8.RuneCountInString(rendered); runes > state.max {
					t.Errorf("%s: state word %q (%s) has %d characters, ceiling %d", lang, rendered, state.key, runes, state.max)
				}
			}

			// The Blacklist page's choice cell (Phase 19, UI-SPEC check C19-2): the check mark and "Delete only"
			// share a half-width cell, so at most 20 code points.
			for _, key := range []string{"settings_bl_opt_none"} {
				label := "✅ " + settingsLocaleString(t, lang, data, key)
				if runes := utf8.RuneCountInString(label); runes > gridMax {
					t.Errorf("%s: choice button %q (%s) has %d characters, ceiling %d", lang, label, key, runes, gridMax)
				}
			}

			for _, key := range options {
				label := "✅ " + settingsLocaleString(t, lang, data, key)
				if runes := utf8.RuneCountInString(label); runes > choiceMax {
					t.Errorf("%s: choice button %q (%s) has %d characters, ceiling %d", lang, label, key, runes, choiceMax)
				}
			}
		})
	}
}

// TestSettingsReusedKeyPlaceholders is the privacy check of D-21: a menu opened in a group
// is readable by every member, so the Staff texts the menu reuses must never carry a
// placeholder that could name the Staff Group. They carry none, or {group} (this group).
// Only settings_staff_linked_to may hold {staff} or {id}, and it is rendered in a private
// menu only.
func TestSettingsReusedKeyPlaceholders(t *testing.T) {
	noPlaceholder := []string{
		"staff_link_no_staff_group",
		"staff_link_refuse_not_staff_owner",
		"staff_check_failed",
		"staff_unlink_not_linked",
		"staff_unlink_refuse_not_owner",
	}
	groupOnly := []string{
		"staff_link_refuse_not_supergroup",
		"staff_link_refuse_target_is_staff",
		"staff_link_refuse_not_target_owner",
		"staff_link_refuse_already_linked",
		"staff_link_check_failed_group",
		"staff_unlink_refuse_not_group_owner",
		"staff_unlink_check_failed_group",
		"staff_unlink_confirm_prompt",
		"lockdown_alert_lift_ask",
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)

			for _, key := range noPlaceholder {
				text := settingsLocaleString(t, lang, data, key)
				if got := staffPlaceholders(text); len(got) != 0 {
					t.Errorf("%s: %s has placeholders %v, want none", lang, key, got)
				}
			}
			for _, key := range groupOnly {
				text := settingsLocaleString(t, lang, data, key)
				if got := staffPlaceholders(text); !slices.Equal(got, []string{"group"}) {
					t.Errorf("%s: %s has placeholders %v, want only [group]", lang, key, got)
				}
			}

			for key, value := range data {
				if !strings.HasPrefix(key, settingsKeyPrefix) || key == "settings_staff_linked_to" {
					continue
				}
				text, _ := value.(string)
				for _, name := range staffPlaceholders(text) {
					// The editor's text prompt lists the literal {id} placeholder of a welcome
					// text as copyable code; it is never filled in (TestSettingsEditLocaleTokens
					// holds it byte-identical), so it cannot name anyone.
					if name == "id" && key == settingsEditWaitTextKey {
						continue
					}
					if name == "staff" || name == "id" {
						t.Errorf("%s: %s holds {%s}, only settings_staff_linked_to may", lang, key, name)
					}
				}
			}
		})
	}
}

// settingsHelpCommands are the commands settings_help_msg must name: /settings itself and
// the commands every menu change is the same as.
var settingsHelpCommands = []string{
	"/settings", "/linkstaff", "/unlinkstaff", "/unlockdown", "/autolockdown", "/captcha",
	"/captchamode", "/captchatime", "/captchaaction", "/setflood", "/setfloodmode", "/delflood",
	"/setwelcome", "/setgoodbye", "/setrules",
}

// TestSettingsHelpFitsOneMessage holds the /help entry of the settings menu to the UI-SPEC
// ceiling (1100 runes in English, 1540 in the others), to the b tag only, to naming every
// command it stands for, and, with the module help header, to one Telegram message.
func TestSettingsHelpFitsOneMessage(t *testing.T) {
	var failures []string
	for _, lang := range staffLocales {
		data := loadStaffLocale(t, lang)
		help, _ := data["settings_help_msg"].(string)
		header, _ := data["helpers_module_help_header"].(string)
		if strings.TrimSpace(help) == "" {
			failures = append(failures, fmt.Sprintf("%s: settings_help_msg is missing or empty", lang))
			continue
		}

		ceiling := 1540
		if lang == "en" {
			ceiling = 1100
		}
		if runes := utf8.RuneCountInString(help); runes > ceiling {
			failures = append(failures, fmt.Sprintf("%s: settings_help_msg has %d characters, ceiling %d", lang, runes, ceiling))
		}

		for _, match := range settingsTagPattern.FindAllStringSubmatch(help, -1) {
			if tag := strings.ToLower(match[1]); tag != "b" {
				failures = append(failures, fmt.Sprintf("%s: settings_help_msg uses the tag <%s>, only b is allowed", lang, tag))
			}
		}

		for _, command := range settingsHelpCommands {
			if !strings.Contains(help, command) {
				failures = append(failures, fmt.Sprintf("%s: settings_help_msg does not name %s", lang, command))
			}
		}

		// Member requirements have no command (Phase 13): the help says so in English, and in
		// every other language names the page the way the menu does, which ties the two.
		if lang == "en" {
			for _, phrase := range []string{"member requirements", "have no command"} {
				if !strings.Contains(help, phrase) {
					failures = append(failures, fmt.Sprintf("en: settings_help_msg does not contain %q", phrase))
				}
			}
		} else if page, _ := data["settings_page_mreq"].(string); page == "" || !strings.Contains(strings.ToLower(help), strings.ToLower(page)) {
			failures = append(failures, fmt.Sprintf("%s: settings_help_msg does not name the page %q", lang, page))
		}

		total := lockdownHelpLength(strings.ReplaceAll(header, "%s", "Settings")) + lockdownHelpLength(help)
		t.Logf("%s: help %d of %d UTF-16 units, headroom %d", lang, total, lockdownHelpTelegramLimit, lockdownHelpTelegramLimit-total)
		if total > lockdownHelpTelegramLimit {
			failures = append(failures, fmt.Sprintf("%s: help header and settings_help_msg are %d characters, over Telegram's %d", lang, total, lockdownHelpTelegramLimit))
		}
	}
	sort.Strings(failures)
	for _, failure := range failures {
		t.Error(failure)
	}
}

// settingsEditWaitTextKey is the Welcome and Goodbye text prompt, the one settings_ string
// that carries the literal {id} placeholder.
const settingsEditWaitTextKey = "settings_edit_wait_text"

// settingsEditLiteralTokens are the welcome-text placeholders and the button syntax the
// editor's prompts show as copyable code. They are never translated, in any language, and
// the translator leaves unknown {name} tokens alone, so each must reach the owner unchanged.
var settingsEditLiteralTokens = []string{
	"<code>{first}</code>", "<code>{last}</code>", "<code>{fullname}</code>", "<code>{username}</code>",
	"<code>{mention}</code>", "<code>{id}</code>", "<code>{count}</code>", "<code>{chatname}</code>",
	"<code>{rules}</code>",
}

const (
	settingsEditButtonExample1 = "<code>[Rules](buttonurl://https://example.com/rules)</code>"
	settingsEditButtonExample2 = "<code>[Chat](buttonurl://https://t.me/example:same)</code>"
	// settingsEditSyntaxExample is the example line of the buttons-syntax refusal (UI-SPEC row 72).
	settingsEditSyntaxExample = "<code>[Name](buttonurl://https://example.com)</code>"
)

// TestSettingsEditLocaleTokens holds the literal tokens of the editor's prompts byte-identical
// in every language, and the real parameters ({max}, {time}, {state}) where the English has
// them. A translated placeholder would ship a prompt that tells the owner to type a token the
// bot never replaces.
func TestSettingsEditLocaleTokens(t *testing.T) {
	type tokenKey struct {
		key    string
		tokens []string
	}
	literal := []tokenKey{
		{"settings_edit_wait_text", settingsEditLiteralTokens},
		{"settings_edit_wait_text_rules", []string{"<code>{first}</code>"}},
		{"settings_edit_wait_buttons", []string{
			settingsEditButtonExample1, settingsEditButtonExample2, "<code>:same</code>",
		}},
		{"settings_edit_hint_rules", []string{"<code>/rules</code>"}},
		{"settings_edit_refuse_buttons_syntax", []string{settingsEditSyntaxExample}},
	}

	parameters := []tokenKey{
		{"settings_edit_wait_text", []string{"{max}"}},
		{"settings_edit_wait_text_rules", []string{"{max}"}},
		{"settings_edit_wait_footer", []string{"{time}"}},
		{"settings_edit_refuse_too_long_filled", []string{"{n}", "{max}"}},
		{"settings_edit_refuse_media_text_long", []string{"{n}", "{max}"}},
		{"settings_edit_refuse_button_url", []string{"{n}"}},
		{"settings_edit_wait_ended_note", []string{"{part}"}},
		{"settings_edit_state_text_len", []string{"{n}"}},
		{"settings_edit_state_buttons_n", []string{"{n}"}},
		{"settings_edit_line_text", []string{"{state}"}},
		{"settings_edit_line_media", []string{"{state}"}},
		{"settings_edit_line_buttons", []string{"{state}"}},
		{"settings_btn_edit_switch_welcome", []string{"{state}"}},
		{"settings_btn_edit_switch_goodbye", []string{"{state}"}},
		{"settings_btn_edit_switch_clean", []string{"{state}"}},
		{"settings_btn_edit_switch_below", []string{"{state}"}},
		{"settings_btn_edit_switch_once", []string{"{state}"}},
		{"settings_edit_line_below", []string{"{state}"}},
		{"settings_edit_line_once", []string{"{state}"}},
		{"settings_edit_refuse_media_rules_long", []string{"{n}", "{max}"}},
		{"settings_edit_refuse_text_rules_long", []string{"{n}", "{max}"}},
		{"settings_edit_line_welcome", []string{"{state}"}},
		{"settings_edit_line_goodbye", []string{"{state}"}},
		{"settings_edit_line_clean_welcome", []string{"{state}"}},
		{"settings_edit_line_clean_goodbye", []string{"{state}"}},
	}

	english := loadStaffLocale(t, "en")
	for _, group := range [][]tokenKey{literal, parameters} {
		for _, entry := range group {
			text := settingsLocaleString(t, "en", english, entry.key)
			for _, token := range entry.tokens {
				if !strings.Contains(text, token) {
					t.Fatalf("en: %s does not carry %q, the test table is out of date", entry.key, token)
				}
			}
		}
	}

	for _, lang := range staffLocales {
		t.Run(lang, func(t *testing.T) {
			data := loadStaffLocale(t, lang)
			for _, group := range [][]tokenKey{literal, parameters} {
				for _, entry := range group {
					text := settingsLocaleString(t, lang, data, entry.key)
					for _, token := range entry.tokens {
						if got, want := strings.Count(text, token), strings.Count(settingsLocaleString(t, "en", english, entry.key), token); got != want {
							t.Errorf("%s: %s carries %q %d times, en.yml %d", lang, entry.key, token, got, want)
						}
					}
				}
			}

			// The prompts for the Welcome and Goodbye text name every placeholder once.
			text := settingsLocaleString(t, lang, data, settingsEditWaitTextKey)
			for _, token := range settingsEditLiteralTokens {
				if strings.Count(text, token) != 1 {
					t.Errorf("%s: %s must carry %q exactly once", lang, settingsEditWaitTextKey, token)
				}
			}
		})
	}
}
