package modules

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db"
	"github.com/uasneppy/fuku_robot/fuku/db/lang"
	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/notes"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
	"github.com/uasneppy/fuku_robot/fuku/utils/content"
	"github.com/uasneppy/fuku_robot/fuku/utils/extraction"
	"github.com/uasneppy/fuku_robot/fuku/utils/formatting"
	"github.com/uasneppy/fuku_robot/fuku/utils/helpers"
	"github.com/uasneppy/fuku_robot/fuku/utils/media"
	"github.com/uasneppy/fuku_robot/fuku/utils/tracing"
)

var notesModule = moduleStruct{
	moduleName: "Notes",
}

func noteOverwriteCacheKey(token string) string {
	return overwriteCacheKey("note", token)
}

func setNoteOverwriteCache(token string, data overwriteNote) error {
	return setOverwriteCache(noteOverwriteCacheKey(token), data)
}

func consumeNoteOverwriteCache(token string) (*overwriteNote, error) {
	return consumeOverwriteCache[overwriteNote](noteOverwriteCacheKey(token))
}

func deleteNoteOverwriteCache(token string) {
	deleteOverwriteCache(noteOverwriteCacheKey(token))
}

// noteWriteSeams are the repository calls the note apply functions make, a variable so a test can make one
// fail.
type noteWriteSeams struct {
	// Add stores one note and reports whether a row was inserted (false: the name was already there).
	Add    func(ctx context.Context, chatID int64, noteName, replyText, fileID string, buttons models.ButtonArray, filtType int, pvtOnly, grpOnly, adminOnly, webPrev, isProtected, noNotif bool) (bool, error)
	Remove func(ctx context.Context, chatID int64, noteName string) error
	// RemoveByID deletes one row by ID only when it belongs to the chat and returns its name.
	RemoveByID func(ctx context.Context, chatID, id int64) (name string, removed bool, err error)
	Exists     func(ctx context.Context, chatID int64, noteName string) (bool, error)
	// PrivateNotes turns the chat's private notes switch (/privnote) on or off.
	PrivateNotes func(chatID int64, on bool) error
}

// noteWrites is the one place the note commands reach the repository through.
var noteWrites = noteWriteSeams{
	Add:          notes.AddNoteContext,
	Remove:       notes.RemoveNoteContext,
	RemoveByID:   notes.RemoveNoteByID,
	Exists:       notes.NoteExistsContext,
	PrivateNotes: notes.TooglePrivateNote,
}

// noteAddOutcome is what applyNoteAdd did; its zero value is a stored note.
type noteAddOutcome int

const (
	// noteAdded means the note was stored (and, from checkNoteName, that no check refused it).
	noteAdded noteAddOutcome = iota
	// noteNameBlank means the name is empty.
	noteNameBlank
	// noteNameBadShape means the name has white space or a leading # or is over listKeywordMaxRunes code
	// points (noteNameInvalid): it could never be fetched by #name or /get.
	noteNameBadShape
	// noteExists means the chat already has a note with this name; the command offers to overwrite it.
	noteExists
	// noteRaced means another writer stored the name between the check and the insert; /save answers it
	// like a stored note, as it always did.
	noteRaced
	// noteSaveFailed means a database call failed and nothing was stored.
	noteSaveFailed
)

// noteAddInput is the note content applyNoteAdd stores.
type noteAddInput struct {
	Name        string
	Text        string
	FileID      string
	Buttons     models.ButtonArray
	DataType    int
	PvtOnly     bool
	GrpOnly     bool
	AdminOnly   bool
	WebPrev     bool
	IsProtected bool
	NoNotif     bool
}

// noteAddResult is what applyNoteAdd and checkNoteName return for the caller to word.
type noteAddResult struct {
	Outcome noteAddOutcome
	Err     error
}

// noteRemoveOutcome is what applyNoteRemove did; its zero value is a removed note.
type noteRemoveOutcome int

const (
	// noteRemoved means the delete ran (a name that is not stored is not an error).
	noteRemoved noteRemoveOutcome = iota
	// noteRemoveSaveFailed means the delete failed.
	noteRemoveSaveFailed
)

// noteRemoveResult is what applyNoteRemove returns.
type noteRemoveResult struct {
	Outcome noteRemoveOutcome
	Err     error
}

// checkNoteName runs the checks every note add shares, in order: a blank name, a name that could never be
// fetched (noteNameInvalid), then a name the chat already has. It returns a result whose outcome is noteAdded
// when nothing refuses the name; a lookup that fails refuses it as noteSaveFailed (nothing is stored on a
// guess). It logs the chat ID and the error only, never the name.
func checkNoteName(ctx context.Context, chatID int64, name string) noteAddResult {
	if strings.TrimSpace(name) == "" {
		return noteAddResult{Outcome: noteNameBlank}
	}
	if noteNameInvalid(name) {
		return noteAddResult{Outcome: noteNameBadShape}
	}
	exists, err := noteWrites.Exists(ctx, chatID, name)
	if err != nil {
		log.Errorf("[Notes] Existing name lookup failed for chat %d: %v", chatID, err)
		return noteAddResult{Outcome: noteSaveFailed, Err: err}
	}
	if exists {
		return noteAddResult{Outcome: noteExists}
	}
	return noteAddResult{}
}

// applyNoteAdd stores one note for the chat after checkNoteName accepts the name. The name is lower-cased
// here as the command always did. A name stored by another writer in the meantime is noteRaced. It judges no
// person's rights and logs the chat ID and the error only.
func applyNoteAdd(ctx context.Context, chatID int64, in noteAddInput) noteAddResult {
	name := strings.ToLower(in.Name)
	if refusal := checkNoteName(ctx, chatID, name); refusal.Outcome != noteAdded {
		return refusal
	}
	inserted, err := noteWrites.Add(ctx, chatID, name, in.Text, in.FileID, in.Buttons, in.DataType, in.PvtOnly, in.GrpOnly, in.AdminOnly, in.WebPrev, in.IsProtected, in.NoNotif)
	if err != nil {
		log.Errorf("[Notes] AddNote failed for chat %d: %v", chatID, err)
		return noteAddResult{Outcome: noteSaveFailed, Err: err}
	}
	if !inserted {
		return noteAddResult{Outcome: noteRaced}
	}
	return noteAddResult{}
}

// applyNoteRemove deletes the chat's note with this name (lower-cased here). It logs the chat ID and the error
// only.
func applyNoteRemove(ctx context.Context, chatID int64, name string) noteRemoveResult {
	if err := noteWrites.Remove(ctx, chatID, strings.ToLower(name)); err != nil {
		log.Errorf("[Notes] RemoveNote failed for chat %d: %v", chatID, err)
		return noteRemoveResult{Outcome: noteRemoveSaveFailed, Err: err}
	}
	return noteRemoveResult{}
}

// noteRemoveByIDOutcome is what applyNoteRemoveByID did; its zero value is a removed note.
type noteRemoveByIDOutcome int

const (
	// noteRemovedByID means the row went.
	noteRemovedByID noteRemoveByIDOutcome = iota
	// noteRemoveByIDGone means no row of this chat has that ID (already removed, or another chat's).
	noteRemoveByIDGone
	// noteRemoveByIDSaveFailed means the delete failed.
	noteRemoveByIDSaveFailed
)

// noteRemoveByIDResult is what applyNoteRemoveByID returns.
type noteRemoveByIDResult struct {
	Outcome noteRemoveByIDOutcome
	// Name is the stored name of the removed note.
	Name string
	Err  error
}

// applyNoteRemoveByID deletes one note by its row ID, only when the row belongs to the chat, so an ID of
// another chat deletes nothing. It judges no person's rights and logs the chat ID and the error only, never
// the name.
func applyNoteRemoveByID(ctx context.Context, chatID, id int64) noteRemoveByIDResult {
	name, removed, err := noteWrites.RemoveByID(ctx, chatID, id)
	if err != nil {
		log.Errorf("[Notes] RemoveNoteByID failed for chat %d: %v", chatID, err)
		return noteRemoveByIDResult{Outcome: noteRemoveByIDSaveFailed, Err: err}
	}
	if !removed {
		return noteRemoveByIDResult{Outcome: noteRemoveByIDGone}
	}
	return noteRemoveByIDResult{Outcome: noteRemovedByID, Name: name}
}

// resolveNotePrivateConflict is the rule /save and the Notes page share for the two private-notes directives: a
// note that asks to be sent to the private chat ({private}) and to the group ({noprivate}) at once keeps neither, so
// it follows the group's own setting, and the caller says so (conflict true). Any other combination is unchanged.
func resolveNotePrivateConflict(pvtOnly, grpOnly bool) (pvt, grp, conflict bool) {
	if pvtOnly && grpOnly {
		return false, false, true
	}
	return pvtOnly, grpOnly, false
}

// offerNoteOverwrite answers /save for a name the chat already has with the overwrite card: the new content
// is cached under a fresh token and the Yes and No buttons carry only that token.
func offerNoteOverwrite(b *gotgbot.Bot, ctx *ext.Context, data overwriteNote) error {
	msg := ctx.EffectiveMessage
	token, tokenErr := newOverwriteToken()
	if tokenErr != nil {
		log.Errorf("[Notes] Failed to generate overwrite token: %v", tokenErr)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errorText, _ := tr.GetString("notes_overwrite_token_failed")
		_, _ = msg.Reply(b, errorText, formatting.Shtml())
		return ext.EndGroups
	}
	if err := setNoteOverwriteCache(token, data); err != nil {
		log.Errorf("[Notes] Failed to cache overwrite data: %v", err)
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		errorText, _ := tr.GetString("notes_overwrite_token_failed")
		_, _ = msg.Reply(b, errorText, formatting.Shtml())
		return ext.EndGroups
	}
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	overwriteText, _ := tr.GetString("notes_overwrite_confirm")
	yesText, _ := tr.GetString("button_yes")
	noText, _ := tr.GetString("button_no")
	_, err := msg.Reply(b,
		overwriteText,
		&gotgbot.SendMessageOpts{
			ParseMode: formatting.HTML,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{
				InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
					{
						{
							Text: yesText,
							CallbackData: encodeCallbackData("notes.overwrite", map[string]string{
								"a": "yes",
								"t": token,
							}),
						},
						{
							Text: noText,
							CallbackData: encodeCallbackData("notes.overwrite", map[string]string{
								"a": "no",
								"t": token,
							}),
						},
					},
				},
			},
		},
	)
	if err != nil {
		deleteNoteOverwriteCache(token)
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

//nolint:dupl // addNote shares validation logic with filters module by design
func (m moduleStruct) addNote(b *gotgbot.Bot, ctx *ext.Context) error {
	connectedChat := chat_status.IsUserConnected(b, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	msg := ctx.EffectiveMessage
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	noteString, _ := tr.GetString("notes_save_success")

	if msg.ReplyToMessage != nil && len(args) <= 1 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_keyword_required")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	} else if len(args) <= 2 && msg.ReplyToMessage == nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_invalid")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	result := content.ExtractNoteAndFilter(msg, false, lang.GetLanguage(ctx))
	noteWord, fileid, text, dataType, buttons, pvtOnly, grpOnly, adminOnly, webPrev, isProtected, noNotif, errorMsg := result.KeyWord, result.FileID, result.Text, result.DataType, result.Buttons, result.PvtOnly, result.GrpOnly, result.AdminOnly, result.WebPreview, result.IsProtected, result.NoNotif, result.ErrorMsg
	if dataType == -1 && errorMsg != "" {
		_, err := msg.Reply(b, errorMsg, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	var conflict bool
	pvtOnly, grpOnly, conflict = resolveNotePrivateConflict(pvtOnly, grpOnly)
	if conflict {
		noteConflictText, _ := tr.GetString("notes_private_conflict_warning")
		noteString += noteConflictText
	}

	noteWord = strings.ToLower(noteWord)

	added := applyNoteAdd(tracing.UpdateContext(ctx), chat.Id, noteAddInput{
		Name:        noteWord,
		Text:        text,
		FileID:      fileid,
		Buttons:     buttons,
		DataType:    dataType,
		PvtOnly:     pvtOnly,
		GrpOnly:     grpOnly,
		AdminOnly:   adminOnly,
		WebPrev:     webPrev,
		IsProtected: isProtected,
		NoNotif:     noNotif,
	})
	switch added.Outcome {
	case noteNameBlank:
		return replyNoteKey(b, ctx, "notes_keyword_required")
	case noteNameBadShape:
		return replyNoteKey(b, ctx, "notes_name_invalid")
	case noteExists:
		return offerNoteOverwrite(b, ctx, overwriteNote{
			overwriteBase: overwriteBase{
				ChatID:   chat.Id,
				UserID:   user.Id,
				ItemName: noteWord,
				Text:     text,
				FileID:   fileid,
				Buttons:  buttons,
				DataType: dataType,
			},
			PvtOnly:     pvtOnly,
			GrpOnly:     grpOnly,
			AdminOnly:   adminOnly,
			WebPrev:     webPrev,
			IsProtected: isProtected,
			NoNotif:     noNotif,
		})
	case noteSaveFailed:
		return replyNoteKey(b, ctx, "notes_save_failed")
	}

	_, err := msg.Reply(b, fmt.Sprintf(noteString, noteWord, noteWord, noteWord), formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

// replyNoteKey answers a refused note command with the translated text of one locale key.
func replyNoteKey(b *gotgbot.Bot, ctx *ext.Context, key string) error {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString(key)
	_, err := ctx.EffectiveMessage.Reply(b, text, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (moduleStruct) rmNote(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	connectedChat := chat_status.IsUserConnected(b, ctx, true, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	args := ctx.Args()

	if len(args) == 1 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_remove_keyword_required")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	parts := strings.SplitN(msg.Text, " ", 2)
	if len(parts) < 2 {
		return ext.EndGroups
	}
	noteWord := strings.TrimLeft(parts[1], "#")

	if !chat_status.CanUserChangeInfo(b, ctx, chat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_change_info_cmd_error", "chat_status_change_info_button_error")
		return ext.EndGroups
	}

	if !slices.Contains(notes.GetNotesListContext(tracing.UpdateContext(ctx), chat.Id, true), strings.ToLower(noteWord)) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_not_exists")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}
	noteWord, _ = extraction.ExtractQuotes(noteWord, false, true)

	if removed := applyNoteRemove(tracing.UpdateContext(ctx), chat.Id, noteWord); removed.Outcome == noteRemoveSaveFailed {
		return replyNoteKey(b, ctx, "error_generic")
	}

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("notes_removed_success")
	_, err := msg.Reply(b, fmt.Sprintf(text, noteWord), formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (moduleStruct) privNote(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	if !chat_status.RequireUserAdmin(b, ctx, nil, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}
	args := ctx.Args()[1:]
	var txt string

	if len(args) == 1 {
		option := args[0]
		switch option {
		case "on", "yes", "true":
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			txt, _ = tr.GetString("notes_private_enabled")
			if err := noteWrites.PrivateNotes(chat.Id, true); err != nil {
				log.Errorf("[Notes] Failed to enable private notes for chat %d: %v", chat.Id, err)
				txt, _ = tr.GetString("common_settings_save_failed")
			}
		case "off", "no", "false":
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			txt, _ = tr.GetString("notes_private_disabled")
			if err := noteWrites.PrivateNotes(chat.Id, false); err != nil {
				log.Errorf("[Notes] Failed to disable private notes for chat %d: %v", chat.Id, err)
				txt, _ = tr.GetString("common_settings_save_failed")
			}
		default:
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			txt, _ = tr.GetString("notes_private_invalid_option")
		}
	} else {
		tmp := notes.GetNotes(chat.Id).PrivateNotesEnabled()
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		if tmp {
			txt, _ = tr.GetString("notes_private_status_on")
		} else {
			txt, _ = tr.GetString("notes_private_status_off")
		}
	}
	_, err := msg.Reply(b, txt, formatting.Smarkdown())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (moduleStruct) notesList(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if chat_status.CheckDisabledCmd(b, msg, "notes") {
		return ext.EndGroups
	}
	connectedChat := chat_status.IsUserConnected(b, ctx, false, true)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}

	noteKeys := notes.GetNotesListContext(tracing.UpdateContext(ctx), chat.Id, chat_status.RequireUserAdmin(b, ctx, chat, user.Id))
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	info, _ := tr.GetString("notes_none_in_chat")

	if len(noteKeys) == 0 {
		_, err := msg.Reply(b, info, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	if ctx.Message.Chat.Type == "private" {
		admin := chat_status.IsUserAdmin(b, chat.Id, user.Id)
		noteEntries := notes.GetNotesListEntriesContext(tracing.UpdateContext(ctx), chat.Id, admin)
		listText, _ := tr.GetString("notes_list_for_chat")
		info = fmt.Sprintf(listText, html.EscapeString(chat.Title))
		var sb strings.Builder
		for _, note := range noteEntries {
			fmt.Fprintf(&sb, "\n - <a href='%s'>%s</a>",
				html.EscapeString(noteDeepLinkURL(b.Username, chat.Id, note.ID, note.Name)), html.EscapeString(note.Name))
		}
		info += sb.String()
		_, err := msg.Reply(b, info, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	privNote := notes.GetNotes(chat.Id).PrivateNotesEnabled()
	if privNote {
		checkBtnText, _ := tr.GetString("notes_check_button")
		_, err := msg.Reply(b, checkBtnText,
			&gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
						{
							{
								Text: func() string {
									tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
									t, _ := tr.GetString("button_click_me")
									return t
								}(),
								Url: fmt.Sprintf("https://t.me/%s?start=notes_%d", b.Username, chat.Id),
							},
						},
					},
				},
			},
		)
		if err != nil {
			log.Error(err)
			return err
		}
	} else {
		currentNotesText, _ := tr.GetString("notes_current_in_chat")
		info = currentNotesText
		var sb strings.Builder
		for _, note := range noteKeys {
			fmt.Fprintf(&sb, " - <code>#%s</code>\n", note)
		}
		info += sb.String()
		instructionText, _ := tr.GetString("notes_get_instruction")
		info += instructionText
		_, err := msg.Reply(b, info, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return ext.EndGroups
}

//nolint:dupl // rmAllNotes shares confirmation pattern with filters module by design
func (moduleStruct) rmAllNotes(b *gotgbot.Bot, ctx *ext.Context) error {
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat

	if !chat_status.RequireGroup(b, ctx, nil) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_group_only_error", "", chat_status.WithReply())
		return ext.EndGroups
	}

	noteKeys := notes.GetNotesListContext(tracing.UpdateContext(ctx), chat.Id, true)
	if len(noteKeys) == 0 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_none_in_chat")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	mem, err := chat.GetMember(b, user.Id, nil)
	if err != nil {
		log.Error(err)
		return err
	}

	if mem.MergeChatMember().Status == "creator" {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		clearAllText, _ := tr.GetString("notes_clear_all_confirm")
		yesText, _ := tr.GetString("button_yes")
		noText, _ := tr.GetString("button_no")
		_, err := msg.Reply(b, clearAllText,
			&gotgbot.SendMessageOpts{
				ReplyMarkup: gotgbot.InlineKeyboardMarkup{
					InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
						{
							{
								Text:         yesText,
								CallbackData: encodeCallbackData("rmAllNotes", map[string]string{"a": "yes"}),
							},
							{
								Text:         noText,
								CallbackData: encodeCallbackData("rmAllNotes", map[string]string{"a": "no"}),
							},
						},
					},
				},
			},
		)
		if err != nil {
			log.Error(err)
			return err
		}
	} else {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_creator_only")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
	}

	return ext.EndGroups
}

func (m moduleStruct) noteOverWriteHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	user := query.From

	var helpText string
	action, token, ok := parseOverwriteCallbackData(query.Data, "notes.overwrite")
	if !ok {
		log.WithField("data", query.Data).Warn("Invalid note overwrite callback data format")
		return ext.EndGroups
	}

	var pressChat *gotgbot.Chat
	if query.Message != nil {
		qchat := query.Message.GetChat()
		pressChat = &qchat
	} else if ctx.EffectiveChat != nil {
		pressChat = ctx.EffectiveChat
	}
	targetChatID, press := overwritePressTarget(b, ctx, pressChat, user.Id, func() (*overwriteBase, error) {
		card, err := getOverwriteCache[overwriteNote](noteOverwriteCacheKey(token))
		if err != nil {
			return nil, err
		}
		return &card.overwriteBase, nil
	})
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	switch press {
	case overwritePressNotAdmin:
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	case overwritePressStale:
		helpText, _ = tr.GetString("notes_overwrite_cancelled")
		if _, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText}); err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	switch action {
	case "no":
		if token != "" {
			if data, err := consumeNoteOverwriteCache(token); err == nil {
				if data.UserID != 0 && data.UserID != user.Id {
					helpText, _ = tr.GetString("notes_overwrite_cancelled")
					break
				}
			}
		}
		helpText, _ = tr.GetString("notes_overwrite_cancelled")
	case "yes":
		var chatId int64

		if token == "" {
			helpText, _ = tr.GetString("notes_overwrite_cancelled")
			break
		}
		noteData, err := consumeNoteOverwriteCache(token)
		if err != nil || (noteData.UserID != 0 && noteData.UserID != user.Id) {
			helpText, _ = tr.GetString("notes_overwrite_cancelled")
			break
		}
		chatId = noteData.ChatID
		if chatId == 0 {
			chatId = targetChatID
		}

		if noteData.ChatID != 0 && targetChatID != 0 && noteData.ChatID != targetChatID {
			helpText, _ = tr.GetString("notes_overwrite_cancelled")
			break
		}

		updated, err := notes.UpdateNote(
			chatId,
			noteData.ItemName,
			noteData.Text,
			noteData.FileID,
			noteData.Buttons,
			noteData.DataType,
			noteData.PvtOnly,
			noteData.GrpOnly,
			noteData.AdminOnly,
			noteData.WebPrev,
			noteData.IsProtected,
			noteData.NoNotif,
		)
		if err != nil {
			log.Errorf("[Notes] Failed to update note during overwrite: %v", err)
			helpText, _ = tr.GetString("notes_save_failed")
		} else if updated {
			helpText, _ = tr.GetString("notes_overwrite_success")
		} else {
			helpText, _ = tr.GetString("notes_overwrite_cancelled")
		}
	default:
		log.WithField("action", action).Warn("Unknown note overwrite action")
		return answerInvalidCallback(b, ctx, query)
	}

	if query.Message == nil {
		if _, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText}); err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	_, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b,
		&gotgbot.AnswerCallbackQueryOpts{
			Text: helpText,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) notesButtonHandler(b *gotgbot.Bot, ctx *ext.Context) error {
	query, ok := callbackQueryFromContext(ctx)
	if !ok {
		return ext.EndGroups
	}
	user := query.From

	if !chat_status.RequireUserOwner(b, ctx, nil, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_owner_cmd_error", "chat_status_owner_button_error", chat_status.WithReply())
		return ext.EndGroups
	}

	response := ""
	if decoded, ok := decodeCallbackData(query.Data, "rmAllNotes"); ok {
		response, _ = decoded.Field("a")
	}
	if response == "" {
		log.Warnf("[Notes] Invalid callback data format: %s", query.Data)
		return answerInvalidCallback(b, ctx, query)
	}
	var helpText string

	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	chat := ctx.EffectiveChat
	switch response {
	case "yes":
		if chat == nil {
			helpText, _ = tr.GetString("error_generic")
			break
		}
		if err := notes.RemoveAllNotes(chat.Id); err != nil {
			log.Errorf("[Notes] Failed to remove all notes: %v", err)
			helpText, _ = tr.GetString("error_generic")
		} else {
			helpText, _ = tr.GetString("notes_clear_all_success")
		}
	case "no":
		helpText, _ = tr.GetString("notes_clear_all_cancelled")
	default:
		return answerInvalidCallback(b, ctx, query)
	}

	if query.Message == nil {
		if _, err := query.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: helpText}); err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	_, _, err := query.Message.EditText(b, &gotgbot.EditMessageTextOpts{Text: helpText})
	if err != nil {
		log.Error(err)
		return err
	}

	_, err = query.Answer(b,
		&gotgbot.AnswerCallbackQueryOpts{
			Text: helpText,
		},
	)
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func (m moduleStruct) notesWatcher(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	chat := ctx.EffectiveChat
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.ContinueGroups
	}

	var replyMsgId int64
	var err error

	if reply := extraction.ReplyTarget(msg); reply != nil {
		replyMsgId = reply.MessageId
	} else {
		replyMsgId = msg.MessageId
	}

	parseText := strings.ToLower(msg.Text)[1:]
	noteNameArgs := strings.Split(parseText, " ")
	noteName := noteNameArgs[0]
	noformatNote := len(noteNameArgs) == 2 && noteNameArgs[1] == "noformat"

	if !slices.Contains(notes.GetNotesListContext(tracing.UpdateContext(ctx), chat.Id, true), strings.ToLower(noteName)) {
		return ext.ContinueGroups
	}

	noteData := notes.GetNote(chat.Id, strings.ToLower(noteName))

	if noteData.NoteContent == "" && noteData.FileID == "" {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_parsing_error")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return nil
	}

	if noteData.AdminOnly {
		if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			text, _ := tr.GetString("notes_admin_only")
			_, err := msg.Reply(b, text, formatting.Shtml())
			if err != nil {
				log.Error(err)
				return err
			}
			return nil
		}
	}

	if noformatNote {
		err = m.sendNoFormatNote(b, ctx, replyMsgId, noteData)
		if err != nil {
			if errors.Is(err, ext.EndGroups) || errors.Is(err, ext.ContinueGroups) {
				// The refusal was answered; the message still goes on to the later groups.
				return nil
			}
			log.Error(err)
			return err
		}
	} else {

		privateNoteOnly := (notes.GetNotes(chat.Id).PrivateNotesEnabled() || noteData.PrivateOnly) && !noteData.GroupOnly

		if privateNoteOnly {
			if ctx.Message.Chat.Type == "private" {
				_, err = media.SendNote(b, ctx, chat, noteData, replyMsgId, ctx.Message.MessageThreadId)
			} else {
				tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
				clickForPrivateText, _ := tr.GetString("notes_click_for_private")
				_, err = msg.Reply(b,
					fmt.Sprintf(clickForPrivateText, noteName),
					&gotgbot.SendMessageOpts{
						ReplyParameters: &gotgbot.ReplyParameters{
							MessageId:                replyMsgId,
							AllowSendingWithoutReply: true,
						},
						ReplyMarkup: gotgbot.InlineKeyboardMarkup{
							InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
								{
									{
										Text: trS(tr, "button_click_me"),
										Url:  noteDeepLinkURL(b.Username, chat.Id, noteData.ID, noteName),
									},
								},
							},
						},
						ParseMode: formatting.Markdown,
					},
				)
			}
		} else {
			_, err = media.SendNote(b, ctx, chat, noteData, replyMsgId, ctx.Message.MessageThreadId)
		}
	}

	if err != nil {
		log.Error(err)
		return err
	}

	// nil leaves group 0 but lets the later groups (locks, blacklists, antiflood, the log channel) see
	// the message: ext.EndGroups would let a "#note" prefix carry anything past them.
	return nil
}

func (m moduleStruct) getNotes(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if chat_status.CheckDisabledCmd(b, msg, "get") {
		return ext.EndGroups
	}
	connectedChat := chat_status.IsUserConnected(b, ctx, false, false)
	if connectedChat == nil {
		return ext.EndGroups
	}
	ctx.EffectiveChat = connectedChat
	chat := ctx.EffectiveChat
	args := ctx.Args()[1:]
	var err error

	if len(args) == 0 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_get_insufficient_args")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	var replyMsgId int64

	if reply := msg.ReplyToMessage; reply != nil {
		replyMsgId = reply.MessageId
	} else {
		replyMsgId = msg.MessageId
	}

	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		return ext.EndGroups
	}
	noteName := args[0]

	if !slices.Contains(notes.GetNotesListContext(tracing.UpdateContext(ctx), chat.Id, true), strings.ToLower(noteName)) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_does_not_exist")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	noteData := notes.GetNote(chat.Id, strings.ToLower(noteName))

	if noteData.NoteContent == "" && noteData.FileID == "" {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("notes_parsing_error_support")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}

	if noteData.AdminOnly {
		if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			text, _ := tr.GetString("notes_admin_only_access")
			_, err = msg.Reply(b, text, formatting.Shtml())
			if err != nil {
				log.Error(err)
				return err
			}
			return ext.ContinueGroups
		}
	}

	if len(args) == 2 && strings.ToLower(args[1]) == "noformat" {
		err = m.sendNoFormatNote(b, ctx, replyMsgId, noteData)
	} else {
		if (notes.GetNotes(chat.Id).PrivateNotesEnabled() || noteData.PrivateOnly) && !noteData.GroupOnly {
			tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
			clickForPrivateText, _ := tr.GetString("notes_click_for_private")
			_, err = msg.Reply(b,
				fmt.Sprintf(clickForPrivateText, noteName),
				&gotgbot.SendMessageOpts{
					ReplyMarkup: gotgbot.InlineKeyboardMarkup{
						InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
							{
								{
									Text: trS(tr, "button_click_me"),
									Url:  noteDeepLinkURL(b.Username, chat.Id, noteData.ID, noteName),
								},
							},
						},
					},
					ParseMode: formatting.Markdown,
					ReplyParameters: &gotgbot.ReplyParameters{
						MessageId:                replyMsgId,
						AllowSendingWithoutReply: true,
					},
				},
			)
		} else {
			_, err = media.SendNote(b, ctx, chat, noteData, replyMsgId, ctx.Message.MessageThreadId)
		}
	}

	if err != nil {
		if !errors.Is(err, ext.EndGroups) && !errors.Is(err, ext.ContinueGroups) {
			log.Error(err)
		}
		return err
	}

	return ext.EndGroups
}

func (moduleStruct) sendNoFormatNote(b *gotgbot.Bot, ctx *ext.Context, replyMsgId int64, noteData *db.Notes) error {
	user := chat_status.RequireUser(b, ctx)
	if user == nil {
		chat_status.NewPermissionResponder(b).Respond(ctx, "common_cannot_identify_user", "", chat_status.WithReply())
		return ext.EndGroups
	}

	if !chat_status.RequireUserAdmin(b, ctx, ctx.EffectiveChat, user.Id) {
		chat_status.NewPermissionResponder(b).Respond(ctx, "chat_status_user_admin_cmd_error", "chat_status_user_admin_button_error", chat_status.WithReplyFallback())
		return ext.EndGroups
	}

	noteData.NoteContent = formatting.ReverseHTML2MD(noteData.NoteContent)

	noteData.NoteContent += content.RevertButtons(noteData.Buttons)

	_, err := media.Send(b, media.Content{
		Text:    noteData.NoteContent,
		FileID:  noteData.FileID,
		MsgType: noteData.MsgType,
		Name:    noteData.NoteName,
	}, media.Options{
		ChatID:            ctx.Message.Chat.Id,
		ReplyMsgID:        replyMsgId,
		ThreadID:          ctx.Message.MessageThreadId,
		Keyboard:          &gotgbot.InlineKeyboardMarkup{InlineKeyboard: nil},
		NoFormat:          true,
		NoNotif:           noteData.NoNotif,
		WebPreview:        false,
		IsProtected:       noteData.IsProtected,
		AllowWithoutReply: true,
	})
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

func LoadNotes(dispatcher *ext.Dispatcher) {
	SetModuleEnabled(notesModule.moduleName, true)

	SetModuleHelp(notesModule.moduleName, [][]gotgbot.InlineKeyboardButton{
		{
			{
				Text:         trS(i18n.MustNewTranslator("uk"), "button_formatting"),
				CallbackData: encodeCallbackData("helpq", map[string]string{"m": "Formatting"}),
			},
		},
	})
	dispatcher.AddHandler(helpers.NewBotCommand("save", notesModule.addNote))
	dispatcher.AddHandler(helpers.NewBotCommand("addnote", notesModule.addNote))
	dispatcher.AddHandler(helpers.NewBotCommand("clear", notesModule.rmNote))
	dispatcher.AddHandler(helpers.NewBotCommand("rmnote", notesModule.rmNote))
	dispatcher.AddHandler(helpers.NewBotCommand("notes", notesModule.notesList))
	dispatcher.AddHandler(helpers.NewBotCommand("saved", notesModule.notesList))
	helpers.AddCmdToDisableable("notes")
	dispatcher.AddHandler(helpers.NewBotCommand("clearall", notesModule.rmAllNotes))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("rmAllNotes"), notesModule.notesButtonHandler))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("notes.overwrite"), notesModule.noteOverWriteHandler))
	dispatcher.AddHandler(
		handlers.NewMessage(
			func(msg *gotgbot.Message) bool {
				return strings.HasPrefix(msg.Text, "#")
			},
			notesModule.notesWatcher,
		),
	)
	helpers.MultiCommand(dispatcher, []string{"privnote", "privatenotes"}, notesModule.privNote)
	dispatcher.AddHandler(helpers.NewBotCommand("get", notesModule.getNotes))
	helpers.AddCmdToDisableable("get")
}

func init() {
	RegisterLegacyModule("Notes", 160, LoadNotes)
	RegisterDeepLinkHandler("notes_", notesListDeepLinkHandler)
	RegisterDeepLinkHandler("note_", noteDeepLinkHandler)
	RegisterDeepLinkHandler("noteid_", noteIDDeepLinkHandler)
	RegisterDeepLinkHandler("note", invalidNoteDeepLinkHandler)
}

func parseChatInfoFromDeepLink(b *gotgbot.Bot, ctx *ext.Context, arg string) (chatinfo *gotgbot.ChatFullInfo, err error) {
	nArgs := strings.SplitN(arg, "_", 3)
	msg := ctx.EffectiveMessage

	if len(nArgs) < 2 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_invalid_deep_link")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return nil, ext.EndGroups
	}

	chatID, parseErr := strconv.ParseInt(nArgs[1], 10, 64)
	if parseErr != nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_invalid_deep_link")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return nil, ext.EndGroups
	}

	chatinfo, chatErr := b.GetChat(chatID, nil)
	if chatErr != nil || chatinfo == nil {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_chat_not_found")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return nil, ext.EndGroups
	}
	return chatinfo, nil
}

func notesListDeepLinkHandler(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	msg := ctx.EffectiveMessage

	chatinfo, err := parseChatInfoFromDeepLink(b, ctx, arg)
	if err != nil {
		return err
	}

	_chat := chatinfo.ToChat()
	if !chat_status.IsUserInChat(b, &_chat, user.Id) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_chat_not_found")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	admin := chat_status.IsUserAdmin(b, chatinfo.Id, user.Id)
	noteEntries := notes.GetNotesListEntriesContext(tracing.UpdateContext(ctx), chatinfo.Id, admin)
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	info, _ := tr.GetString("notes_none_in_chat")
	if len(noteEntries) > 0 {
		info, _ = tr.GetString("helpers_notes_current_header")
		var sb strings.Builder
		for _, note := range noteEntries {
			fmt.Fprintf(&sb, " - <a href='%s'>%s</a>\n",
				html.EscapeString(noteDeepLinkURL(b.Username, chatinfo.Id, note.ID, note.Name)), html.EscapeString(note.Name))
		}
		info += sb.String()
	}

	_, err = msg.Reply(b, info, formatting.Shtml())
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func noteDeepLinkHandler(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	msg := ctx.EffectiveMessage

	chatinfo, err := parseChatInfoFromDeepLink(b, ctx, arg)
	if err != nil {
		return err
	}

	_chat := chatinfo.ToChat()
	if !chat_status.IsUserInChat(b, &_chat, user.Id) {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_chat_not_found")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	nArgs := strings.SplitN(arg, "_", 3)
	if len(nArgs) < 3 {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString("helpers_invalid_deep_link")
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	noteName := strings.ToLower(nArgs[2])
	noteData := notes.GetNote(chatinfo.Id, noteName)
	return sendDeepLinkNote(b, ctx, user, &_chat, noteData)
}

// noteDeepLinkURL is the link of a private note. Telegram accepts only 1 to 64 characters of
// A-Z a-z 0-9 _ - as a start parameter, so the link names the note by its row ID; only an entry
// cached by an earlier build, which has no ID, falls back to the old name form.
func noteDeepLinkURL(botUsername string, chatID int64, noteID uint, noteName string) string {
	if noteID > 0 {
		return fmt.Sprintf("https://t.me/%s?start=noteid_%d_%d", botUsername, chatID, noteID)
	}
	return fmt.Sprintf("https://t.me/%s?start=note_%d_%s", botUsername, chatID, noteName)
}

// parseNoteIDDeepLink reads noteid_<chat id>_<row id> strictly: exactly three parts, a negative
// chat ID and a positive row ID of at most 19 digits.
func parseNoteIDDeepLink(arg string) (chatID, noteID int64, ok bool) {
	parts := strings.Split(arg, "_")
	if len(parts) != 3 || parts[0] != "noteid" {
		return 0, 0, false
	}
	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || chatID >= 0 {
		return 0, 0, false
	}
	if len(parts[2]) == 0 || len(parts[2]) > 19 {
		return 0, 0, false
	}
	for _, r := range parts[2] {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
	}
	noteID, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil || noteID <= 0 {
		return 0, 0, false
	}
	return chatID, noteID, true
}

// noteIDDeepLinkHandler opens /start noteid_<chat id>_<row id>. The payload is attacker-controlled,
// so it is parsed before any Telegram call, the reader must be in the chat, and the note is read by
// its row ID and the chat together, so an ID of another chat's note is not found.
func noteIDDeepLinkHandler(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	msg := ctx.EffectiveMessage
	reply := func(key string) error {
		tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
		text, _ := tr.GetString(key)
		_, _ = msg.Reply(b, text, formatting.Shtml())
		return ext.EndGroups
	}

	chatID, noteID, ok := parseNoteIDDeepLink(arg)
	if !ok {
		return reply("helpers_invalid_deep_link")
	}
	chatinfo, chatErr := b.GetChat(chatID, nil)
	if chatErr != nil || chatinfo == nil {
		return reply("helpers_chat_not_found")
	}
	_chat := chatinfo.ToChat()
	if !chat_status.IsUserInChat(b, &_chat, user.Id) {
		return reply("helpers_chat_not_found")
	}

	noteData, err := notes.GetNoteEntryFresh(tracing.UpdateContext(ctx), chatinfo.Id, noteID)
	if err != nil {
		log.Errorf("[Notes][noteIDDeepLinkHandler]: chat %d: %v", chatinfo.Id, err)
		return reply("notes_parsing_error")
	}
	return sendDeepLinkNote(b, ctx, user, &_chat, noteData)
}

// sendDeepLinkNote is the part both note deep links share once the note was read: a missing note,
// the admin-only check and the send.
func sendDeepLinkNote(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, chat *gotgbot.Chat, noteData *models.Notes) error {
	msg := ctx.EffectiveMessage
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	if noteData == nil {
		text, _ := tr.GetString("helpers_note_not_exist")
		_, err := msg.Reply(b, text, formatting.Shtml())
		if err != nil {
			log.Error(err)
			return err
		}
		return ext.EndGroups
	}
	if noteData.AdminOnly {
		if !chat_status.IsUserAdmin(b, chat.Id, user.Id) {
			text, _ := tr.GetString("helpers_note_admin_only")
			_, err := msg.Reply(b, text, formatting.Shtml())
			if err != nil {
				log.Error(err)
				return err
			}
			return ext.ContinueGroups
		}
	}
	_, err := media.SendNote(b, ctx, chat, noteData, msg.MessageId, msg.MessageThreadId)
	if err != nil {
		log.Error(err)
		return err
	}
	return ext.EndGroups
}

func invalidNoteDeepLinkHandler(b *gotgbot.Bot, ctx *ext.Context, user *gotgbot.User, arg string) error {
	tr := i18n.MustNewTranslator(lang.GetLanguage(ctx))
	text, _ := tr.GetString("helpers_invalid_deep_link")
	_, _ = ctx.EffectiveMessage.Reply(b, text, formatting.Shtml())
	return ext.EndGroups
}
