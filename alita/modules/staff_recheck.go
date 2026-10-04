package modules

import (
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// staffGroupsListToken stands in for the list of group titles while the combined
// Staff Group owner-changed notice is translated: titles are user-controlled and
// the translator runs a printf-style pass over interpolated text.
const staffGroupsListToken = "<<staff-groups-list>>"

// staffRecheckResult is the outcome of rechecking one link.
type staffRecheckResult int

const (
	// staffRecheckOK means the live creator of both chats is the link's maker.
	staffRecheckOK staffRecheckResult = iota
	// staffRecheckRemoved means this caller deleted the link and posted the notice.
	staffRecheckRemoved
	// staffRecheckUnknown means a Telegram answer was missing; nothing changed.
	staffRecheckUnknown
	// staffRecheckGone means the link was already removed, by another caller or
	// because the maker no longer matches the stored row; nothing was posted.
	staffRecheckGone
)

// staffOwnerOutcome is one memoised CheckOwner answer.
type staffOwnerOutcome struct {
	once   sync.Once
	result chat_status.OwnerResult
	live   int64
	err    error
}

// staffOwnerPass memoises CheckOwner answers for one recheck pass, so a pass over
// many links of one Staff Group asks Telegram about each (chat, user) pair once.
// It is safe for concurrent callers.
type staffOwnerPass struct {
	mu      sync.Mutex
	results map[[2]int64]*staffOwnerOutcome
}

// newStaffOwnerPass returns an empty pass cache.
func newStaffOwnerPass() *staffOwnerPass {
	return &staffOwnerPass{results: make(map[[2]int64]*staffOwnerOutcome)}
}

func (p *staffOwnerPass) entry(chatID, want int64) *staffOwnerOutcome {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := [2]int64{chatID, want}
	e, ok := p.results[key]
	if !ok {
		e = &staffOwnerOutcome{}
		p.results[key] = e
	}
	return e
}

// check returns the live ownership answer for (chatID, want), calling
// chat_status.CheckOwner at most once per key in this pass, however many callers
// ask at the same time.
func (p *staffOwnerPass) check(b *gotgbot.Bot, chatID, want int64) (chat_status.OwnerResult, int64, error) {
	e := p.entry(chatID, want)
	e.once.Do(func() {
		e.result, e.live, e.err = chat_status.CheckOwner(b, chatID, want)
	})
	return e.result, e.live, e.err
}

// seed records an answer that was already obtained live in this pass, so it is
// not asked for again.
func (p *staffOwnerPass) seed(chatID, want int64, result chat_status.OwnerResult, live int64, err error) {
	e := p.entry(chatID, want)
	e.once.Do(func() {
		e.result, e.live, e.err = result, live, err
	})
}

// postGroupOwnerChangedNotice tells the Staff Group, in its own language, that a
// group was unlinked because its owner changed. Nothing is sent anywhere else
// (D-13): the linked group and its members learn nothing from the bot.
func postGroupOwnerChangedNotice(b *gotgbot.Bot, link models.StaffGroupLink) {
	tr := staffChatTranslator(link.StaffChatID)
	text, _ := tr.GetString("staff_notice_unlinked_group_owner_changed", i18n.TranslationParams{
		"group": staffGroupTitleToken,
	})
	text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
	_ = sendStaffNotice(b, link.StaffChatID, text)
}

// postStaffOwnerChangedNotice tells the Staff Group, in its own language, that the
// given groups were unlinked because the Staff Group's owner changed. titles are
// stored group titles in the order to show; they are escaped and spliced in after
// translation.
func postStaffOwnerChangedNotice(b *gotgbot.Bot, staffChatID int64, titles []string) {
	if len(titles) == 0 {
		return
	}
	escaped := make([]string, len(titles))
	for i, title := range titles {
		escaped[i] = staffDisplayTitle(title)
	}
	tr := staffChatTranslator(staffChatID)
	text, _ := tr.GetString("staff_notice_unlinked_staff_owner_changed", i18n.TranslationParams{
		"groups": staffGroupsListToken,
	})
	text = strings.Replace(text, staffGroupsListToken, strings.Join(escaped, "\n"), 1)
	_ = sendStaffNotice(b, staffChatID, text)
}

// recheckLink is the single authority recheck of one link (D-15): the ownership
// watchers, the /staff panel, the hourly sweep and Phase 2's pre-action check all
// call it. The link stays valid only while its maker is, live, the creator of both
// the linked group and the Staff Group (D-12); neither the recorded owner, the
// cached admin list nor an update's payload is consulted.
//
// Only a successful Telegram answer showing a different creator, or none, removes
// anything. An error (rate limit, timeout, bot kicked, chat not found) leaves the
// link untouched and returns staffRecheckUnknown. The removal is the conditional
// DELETE of staff.DeleteLinkIfOwner: only the caller whose delete affected a row
// posts the notice to the Staff Group, so racing triggers produce one notice.
// pass may be shared across links of one pass; nil gets a private one.
func recheckLink(b *gotgbot.Bot, link models.StaffGroupLink, pass *staffOwnerPass) staffRecheckResult {
	if pass == nil {
		pass = newStaffOwnerPass()
	}

	groupResult, _, groupErr := pass.check(b, link.GroupChatID, link.OwnerUserID)
	staffResult := chat_status.OwnerUnknown
	var staffErr error
	if groupResult != chat_status.OwnerMismatch {
		staffResult, _, staffErr = pass.check(b, link.StaffChatID, link.OwnerUserID)
	}

	groupSideChanged := groupResult == chat_status.OwnerMismatch
	staffSideChanged := staffResult == chat_status.OwnerMismatch
	if groupSideChanged || staffSideChanged {
		deleted, err := staff.DeleteLinkIfOwner(link.ID, link.OwnerUserID)
		if err != nil {
			return staffRecheckUnknown
		}
		if !deleted {
			return staffRecheckGone
		}
		if groupSideChanged {
			postGroupOwnerChangedNotice(b, link)
		} else {
			postStaffOwnerChangedNotice(b, link.StaffChatID, []string{link.GroupTitle})
		}
		return staffRecheckRemoved
	}

	if groupResult == chat_status.OwnerMatch && staffResult == chat_status.OwnerMatch {
		return staffRecheckOK
	}
	if groupErr != nil {
		log.Warnf("[Staff] recheck: owner check for group %d failed: %v", link.GroupChatID, groupErr)
	}
	if staffErr != nil {
		log.Warnf("[Staff] recheck: owner check for Staff Group %d failed: %v", link.StaffChatID, staffErr)
	}
	return staffRecheckUnknown
}
