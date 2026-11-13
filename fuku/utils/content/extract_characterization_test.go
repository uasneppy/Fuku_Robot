//go:build testtools

package content

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

// extractCharCase is one command message of the characterization table. The commands /filter and
// /save share ExtractNoteAndFilter and never read the command word, so one table serves both.
type extractCharCase struct {
	name string
	msg  *gotgbot.Message
}

// extractCharString renders every field of a result, so a change to any of them shows.
func extractCharString(r ExtractResult) string {
	buttons, err := json.Marshal(r.Buttons)
	if err != nil {
		return "buttons: " + err.Error()
	}
	text := r.Text
	if runes := []rune(text); len(runes) > 80 {
		text = fmt.Sprintf("%s...(%d code points)", string(runes[:40]), len(runes))
	}
	return fmt.Sprintf("kw=%q text=%q type=%d file=%q buttons=%s flags=%t%t%t%t%t%t err=%q",
		r.KeyWord, text, r.DataType, r.FileID, buttons,
		r.PvtOnly, r.GrpOnly, r.AdminOnly, r.WebPreview, r.IsProtected, r.NoNotif, r.ErrorMsg)
}

func extractCharKeyboard() *gotgbot.InlineKeyboardMarkup {
	return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{
		{{Text: "A", Url: "https://a.example"}, {Text: "B", Url: "https://b.example"}},
		{{Text: "C", Url: "javascript:alert(1)"}},
		{{Text: "D", Url: "https://d.example"}},
	}}
}

// extractCharCases is the table of command messages of the characterization: the inline form, the
// reply form over text and over every media kind, buttons, directives and the refusals.
func extractCharCases() []extractCharCase {
	bold := []gotgbot.MessageEntity{{Type: "bold", Offset: 0, Length: 4}}
	reply := func(text string, r *gotgbot.Message) *gotgbot.Message {
		r.MessageId = 7
		return &gotgbot.Message{Text: text, ReplyToMessage: r}
	}
	return []extractCharCase{
		{"inline_entities_and_newlines", &gotgbot.Message{
			Text:     "/filter hello some bold\nsecond  line",
			Entities: []gotgbot.MessageEntity{{Type: "bold", Offset: 19, Length: 4}},
		}},
		{"inline_markdown_button", &gotgbot.Message{Text: "/filter hello see [Site](buttonurl://https://example.com) now"}},
		{"inline_no_text", &gotgbot.Message{Text: "/filter hello"}},
		{"reply_text_entities_and_newlines", reply("/filter hello", &gotgbot.Message{Text: "bold line one\nline two", Entities: bold})},
		{"reply_text_two_word_keyword_quirk", reply("/filter hello world", &gotgbot.Message{Text: "original"})},
		{"reply_text_three_words", reply("/filter hello big world", &gotgbot.Message{Text: "original"})},
		{"reply_photo_no_caption", reply("/filter hello", &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "p-small"}, {FileId: "p-big"}}})},
		{"reply_photo_caption_entities", reply("/filter hello", &gotgbot.Message{
			Photo:           []gotgbot.PhotoSize{{FileId: "p-small"}, {FileId: "p-big"}},
			Caption:         "bold caption\nsecond",
			CaptionEntities: bold,
		})},
		{"reply_video", reply("/filter hello", &gotgbot.Message{Video: &gotgbot.Video{FileId: "v-1"}})},
		{"reply_video_caption", reply("/filter hello", &gotgbot.Message{Video: &gotgbot.Video{FileId: "v-1"}, Caption: "clip"})},
		{"reply_animation_stays_document", reply("/filter hello", &gotgbot.Message{
			Animation: &gotgbot.Animation{FileId: "gif-1"}, Document: &gotgbot.Document{FileId: "gif-doc"},
		})},
		{"reply_audio", reply("/filter hello", &gotgbot.Message{Audio: &gotgbot.Audio{FileId: "a-1"}, Caption: "song"})},
		{"reply_voice", reply("/filter hello", &gotgbot.Message{Voice: &gotgbot.Voice{FileId: "vo-1"}})},
		{"reply_document", reply("/filter hello", &gotgbot.Message{Document: &gotgbot.Document{FileId: "d-1"}, Caption: "file"})},
		{"reply_sticker", reply("/filter hello", &gotgbot.Message{Sticker: &gotgbot.Sticker{FileId: "s-1"}})},
		{"reply_video_note", reply("/filter hello", &gotgbot.Message{VideoNote: &gotgbot.VideoNote{FileId: "vn-1"}})},
		{"reply_photo_with_inline_keyboard", reply("/filter hello", &gotgbot.Message{
			Photo: []gotgbot.PhotoSize{{FileId: "p-1"}}, Caption: "keyed", ReplyMarkup: extractCharKeyboard(),
		})},
		{"reply_text_with_inline_keyboard", reply("/filter hello", &gotgbot.Message{Text: "keyed text", ReplyMarkup: extractCharKeyboard()})},
		{"reply_markdown_buttons_javascript_dropped", reply("/filter hello", &gotgbot.Message{
			Text: "text [ok](buttonurl://https://ok.example) [bad](buttonurl://javascript:alert(1)) [same](buttonurl://https://same.example:same)",
		})},
		{"reply_note_directives", reply("/filter hello", &gotgbot.Message{Text: "hi {admin} {private} {preview} {protect} {nonotif} there"})},
		{"reply_noprivate_directive", reply("/filter hello", &gotgbot.Message{Text: "{noprivate} open"})},
		{"reply_text_over_4096", reply("/filter hello", &gotgbot.Message{Text: strings.Repeat("a", 4097)})},
		{"reply_caption_over_1024", reply("/filter hello", &gotgbot.Message{Photo: []gotgbot.PhotoSize{{FileId: "p-1"}}, Caption: strings.Repeat("c", 1025)})},
		{"reply_poll", reply("/filter hello", &gotgbot.Message{Poll: &gotgbot.Poll{Id: "poll-1", Question: "Q"}})},
		{"reply_with_no_keyword", reply("/filter", &gotgbot.Message{Text: "x"})},
	}
}

// TestExtractNoteAndFilterCharacterization pins what ExtractNoteAndFilter returned before the typed
// content refactor, for both /filter and /save, over the table above. It is committed alone, before the
// refactor, and is never re-recorded or weakened.
func TestExtractNoteAndFilterCharacterization(t *testing.T) {
	t.Parallel()
	for _, mode := range []struct {
		name     string
		isFilter bool
	}{{"filter", true}, {"note", false}} {
		for _, tc := range extractCharCases() {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				got := extractCharString(ExtractNoteAndFilter(tc.msg, mode.isFilter, "en"))
				want, ok := extractCharGoldens[mode.name+"/"+tc.name]
				if !ok {
					t.Logf("GOLDEN\t%s\t%q", mode.name+"/"+tc.name, got)
					return
				}
				if got != want {
					t.Errorf("result changed\n got %s\nwant %s", got, want)
				}
			})
		}
	}
}

// Goldens recorded on the extraction as it was before Phase 19.
var extractCharGoldens = map[string]string{
	"filter/inline_entities_and_newlines":              "kw=\"hello\" text=\"some bold second line\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/inline_markdown_button":                    "kw=\"hello\" text=\"see  now\" type=1 file=\"\" buttons=[{\"name\":\"Site\",\"url\":\"https://example.com\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/inline_no_text":                            "kw=\"\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_animation_stays_document":            "kw=\"hello\" text=\"\" type=3 file=\"gif-doc\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_audio":                               "kw=\"hello\" text=\"song\" type=5 file=\"a-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_caption_over_1024":                   "kw=\"hello\" text=\"cccccccccccccccccccccccccccccccccccccccc...(1025 code points)\" type=-1 file=\"p-1\" buttons=null flags=falsefalsefalsefalsefalsefalse err=\"Your message caption is 1025 characters long. The maximum caption length is 1024; please trim it to a smaller size. Note that markdown characters may take more space than expected.\"",
	"filter/reply_document":                            "kw=\"hello\" text=\"file\" type=3 file=\"d-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_markdown_buttons_javascript_dropped": "kw=\"hello\" text=\"text  )\" type=1 file=\"\" buttons=[{\"name\":\"ok\",\"url\":\"https://ok.example\",\"btn_sameline\":false},{\"name\":\"same\",\"url\":\"https://same.example\",\"btn_sameline\":true}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_noprivate_directive":                 "kw=\"hello\" text=\"{noprivate} open\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_note_directives":                     "kw=\"hello\" text=\"hi {admin} {private} {preview} {protect} {nonotif} there\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_photo_caption_entities":              "kw=\"hello\" text=\"<b>bold</b> caption\\nsecond\" type=4 file=\"p-big\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_photo_no_caption":                    "kw=\"hello\" text=\"\" type=4 file=\"p-big\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_photo_with_inline_keyboard":          "kw=\"hello\" text=\"keyed\" type=4 file=\"p-1\" buttons=[{\"name\":\"A\",\"url\":\"https://a.example\",\"btn_sameline\":false},{\"name\":\"B\",\"url\":\"https://b.example\",\"btn_sameline\":true},{\"name\":\"D\",\"url\":\"https://d.example\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_poll":                                "kw=\"hello\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_sticker":                             "kw=\"hello\" text=\"\" type=2 file=\"s-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_text_entities_and_newlines":          "kw=\"hello\" text=\"<b>bold</b> line one\\nline two\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_text_over_4096":                      "kw=\"hello\" text=\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa...(4097 code points)\" type=-1 file=\"\" buttons=null flags=falsefalsefalsefalsefalsefalse err=\"Your message text is 4097 characters long. The maximum length for text is 4096; please trim it to a smaller size. Note that markdown characters may take more space than expected.\"",
	"filter/reply_text_three_words":                    "kw=\"hello big world\" text=\"big world\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_text_two_word_keyword_quirk":         "kw=\"hello world\" text=\"world\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_text_with_inline_keyboard":           "kw=\"hello\" text=\"keyed text\" type=1 file=\"\" buttons=[{\"name\":\"A\",\"url\":\"https://a.example\",\"btn_sameline\":false},{\"name\":\"B\",\"url\":\"https://b.example\",\"btn_sameline\":true},{\"name\":\"D\",\"url\":\"https://d.example\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_video":                               "kw=\"hello\" text=\"\" type=7 file=\"v-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_video_caption":                       "kw=\"hello\" text=\"clip\" type=7 file=\"v-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_video_note":                          "kw=\"hello\" text=\"\" type=8 file=\"vn-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_voice":                               "kw=\"hello\" text=\"\" type=6 file=\"vo-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"filter/reply_with_no_keyword":                     "kw=\"\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the filter some content!\"",
	"note/inline_entities_and_newlines":                "kw=\"hello\" text=\"some bold second line\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/inline_markdown_button":                      "kw=\"hello\" text=\"see  now\" type=1 file=\"\" buttons=[{\"name\":\"Site\",\"url\":\"https://example.com\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/inline_no_text":                              "kw=\"\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_animation_stays_document":              "kw=\"hello\" text=\"\" type=3 file=\"gif-doc\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_audio":                                 "kw=\"hello\" text=\"song\" type=5 file=\"a-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_caption_over_1024":                     "kw=\"hello\" text=\"cccccccccccccccccccccccccccccccccccccccc...(1025 code points)\" type=-1 file=\"p-1\" buttons=null flags=falsefalsefalsefalsefalsefalse err=\"Your message caption is 1025 characters long. The maximum caption length is 1024; please trim it to a smaller size. Note that markdown characters may take more space than expected.\"",
	"note/reply_document":                              "kw=\"hello\" text=\"file\" type=3 file=\"d-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_markdown_buttons_javascript_dropped":   "kw=\"hello\" text=\"text  )\" type=1 file=\"\" buttons=[{\"name\":\"ok\",\"url\":\"https://ok.example\",\"btn_sameline\":false},{\"name\":\"same\",\"url\":\"https://same.example\",\"btn_sameline\":true}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_noprivate_directive":                   "kw=\"hello\" text=\"{noprivate} open\" type=1 file=\"\" buttons=[] flags=falsetruefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_note_directives":                       "kw=\"hello\" text=\"hi {admin} {private} {preview} {protect} {nonotif} there\" type=1 file=\"\" buttons=[] flags=truefalsetruetruetruetrue err=\"You need to give the note some content!\"",
	"note/reply_photo_caption_entities":                "kw=\"hello\" text=\"<b>bold</b> caption\\nsecond\" type=4 file=\"p-big\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_photo_no_caption":                      "kw=\"hello\" text=\"\" type=4 file=\"p-big\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_photo_with_inline_keyboard":            "kw=\"hello\" text=\"keyed\" type=4 file=\"p-1\" buttons=[{\"name\":\"A\",\"url\":\"https://a.example\",\"btn_sameline\":false},{\"name\":\"B\",\"url\":\"https://b.example\",\"btn_sameline\":true},{\"name\":\"D\",\"url\":\"https://d.example\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_poll":                                  "kw=\"hello\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_sticker":                               "kw=\"hello\" text=\"\" type=2 file=\"s-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_text_entities_and_newlines":            "kw=\"hello\" text=\"<b>bold</b> line one\\nline two\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_text_over_4096":                        "kw=\"hello\" text=\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa...(4097 code points)\" type=-1 file=\"\" buttons=null flags=falsefalsefalsefalsefalsefalse err=\"Your message text is 4097 characters long. The maximum length for text is 4096; please trim it to a smaller size. Note that markdown characters may take more space than expected.\"",
	"note/reply_text_three_words":                      "kw=\"hello big world\" text=\"big world\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_text_two_word_keyword_quirk":           "kw=\"hello world\" text=\"world\" type=1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_text_with_inline_keyboard":             "kw=\"hello\" text=\"keyed text\" type=1 file=\"\" buttons=[{\"name\":\"A\",\"url\":\"https://a.example\",\"btn_sameline\":false},{\"name\":\"B\",\"url\":\"https://b.example\",\"btn_sameline\":true},{\"name\":\"D\",\"url\":\"https://d.example\",\"btn_sameline\":false}] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_video":                                 "kw=\"hello\" text=\"\" type=7 file=\"v-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_video_caption":                         "kw=\"hello\" text=\"clip\" type=7 file=\"v-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_video_note":                            "kw=\"hello\" text=\"\" type=8 file=\"vn-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_voice":                                 "kw=\"hello\" text=\"\" type=6 file=\"vo-1\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
	"note/reply_with_no_keyword":                       "kw=\"\" text=\"\" type=-1 file=\"\" buttons=[] flags=falsefalsefalsefalsefalsefalse err=\"You need to give the note some content!\"",
}
