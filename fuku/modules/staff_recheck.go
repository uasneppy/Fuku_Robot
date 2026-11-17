package modules

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	log "github.com/sirupsen/logrus"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/db/staff"
	"github.com/uasneppy/fuku_robot/fuku/i18n"
	"github.com/uasneppy/fuku_robot/fuku/utils/chat_status"
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
	// staffRecheckGone means the link was already removed, by another caller, an
	// unlink or because the maker no longer matches the stored row, or the group's
	// current link is another row; nothing was posted.
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
	// call, when set, runs each live CheckOwner through the caller's rate-limit
	// pacing. Only the staff fan-out sets it; the panel, the sweeper and the
	// watchers leave it nil and call CheckOwner directly.
	call func(run func() error) error
	// notices, when set, collects the owner-change notices of the links recheckLink
	// removes in this pass instead of posting each at once; the caller flushes it.
	// Only the staff fan-out sets it.
	notices *staffNoticeBatch
}

// newStaffOwnerPass returns an empty pass cache.
func newStaffOwnerPass() *staffOwnerPass {
	return &staffOwnerPass{results: make(map[[2]int64]*staffOwnerOutcome)}
}

// newPacedStaffOwnerPass returns a pass cache whose live owner checks go through
// call. call must invoke run, possibly more than once, and return its final
// error; an error call returns that run did not produce (a rate limit it gave up
// on, a cancelled context) is recorded as an unknown owner, never a mismatch.
func newPacedStaffOwnerPass(call func(run func() error) error) *staffOwnerPass {
	pass := newStaffOwnerPass()
	pass.call = call
	return pass
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
		run := func() error {
			e.result, e.live, e.err = chat_status.CheckOwner(b, chatID, want)
			return e.err
		}
		if p.call == nil {
			_ = run()
			return
		}
		// Until run finishes the answer is unknown, so a panic inside it cannot
		// leave the zero value (OwnerMatch) behind for the other callers.
		e.result, e.err = chat_status.OwnerUnknown, errors.New("owner check did not finish")
		if err := p.call(run); err != nil {
			e.result, e.live, e.err = chat_status.OwnerUnknown, 0, err
		}
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
	_ = sendStaffNotice(b, link.StaffChatID, groupOwnerChangedText(link))
}

// groupOwnerChangedText is the text of postGroupOwnerChangedNotice.
func groupOwnerChangedText(link models.StaffGroupLink) string {
	tr := staffChatTranslator(link.StaffChatID)
	text, _ := tr.GetString("staff_notice_unlinked_group_owner_changed", i18n.TranslationParams{
		"group": staffGroupTitleToken,
	})
	return strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
}

// postStaffOwnerChangedNotice tells the Staff Group, in its own language, that the
// given groups were unlinked because the Staff Group's owner changed. titles are
// stored group titles in the order to show; they are escaped and spliced in after
// translation.
func postStaffOwnerChangedNotice(b *gotgbot.Bot, staffChatID int64, titles []string) {
	if len(titles) == 0 {
		return
	}
	_ = sendStaffNotice(b, staffChatID, staffOwnerChangedText(staffChatID, titles))
}

// staffOwnerChangedText is the text of postStaffOwnerChangedNotice.
func staffOwnerChangedText(staffChatID int64, titles []string) string {
	escaped := make([]string, len(titles))
	for i, title := range titles {
		escaped[i] = staffDisplayTitle(title)
	}
	tr := staffChatTranslator(staffChatID)
	text, _ := tr.GetString("staff_notice_unlinked_staff_owner_changed", i18n.TranslationParams{
		"groups": staffGroupsListToken,
	})
	return strings.Replace(text, staffGroupsListToken, strings.Join(escaped, "\n"), 1)
}

// staffNoticeBatch collects the owner-change notices of the links one pass removed,
// so a run over many links of a Staff Group whose owner changed posts one combined
// notice instead of one per link (S7). It is safe for concurrent callers.
type staffNoticeBatch struct {
	mu         sync.Mutex
	groupSide  []models.StaffGroupLink
	staffSide  []models.StaffGroupLink
	flushedOut bool
}

func (n *staffNoticeBatch) add(link models.StaffGroupLink, groupSide bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if groupSide {
		n.groupSide = append(n.groupSide, link)
	} else {
		n.staffSide = append(n.staffSide, link)
	}
}

// flush posts what was collected, each message through send (which paces it): one
// notice per group whose own owner changed, and one combined notice per Staff Group
// whose owner changed, naming its removed groups in link-id order. It posts at most
// once.
func (n *staffNoticeBatch) flush(b *gotgbot.Bot, send func(run func() error) error) {
	n.mu.Lock()
	if n.flushedOut {
		n.mu.Unlock()
		return
	}
	n.flushedOut = true
	groupSide := append([]models.StaffGroupLink(nil), n.groupSide...)
	staffSide := append([]models.StaffGroupLink(nil), n.staffSide...)
	n.mu.Unlock()

	post := func(chatID int64, text string) {
		if err := send(func() error { return sendStaffNotice(b, chatID, text) }); err != nil {
			log.Warnf("[Staff] owner-change notice to Staff Group %d not sent: %v", chatID, err)
		}
	}
	sort.Slice(groupSide, func(i, j int) bool { return groupSide[i].ID < groupSide[j].ID })
	for _, link := range groupSide {
		post(link.StaffChatID, groupOwnerChangedText(link))
	}
	sort.Slice(staffSide, func(i, j int) bool { return staffSide[i].ID < staffSide[j].ID })
	titles := make(map[int64][]string)
	var chats []int64
	for _, link := range staffSide {
		if _, seen := titles[link.StaffChatID]; !seen {
			chats = append(chats, link.StaffChatID)
		}
		titles[link.StaffChatID] = append(titles[link.StaffChatID], link.GroupTitle)
	}
	for _, chatID := range chats {
		post(chatID, staffOwnerChangedText(chatID, titles[chatID]))
	}
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
		if pass.notices != nil {
			pass.notices.add(link, groupSideChanged)
			return staffRecheckRemoved
		}
		if groupSideChanged {
			postGroupOwnerChangedNotice(b, link)
		} else {
			postStaffOwnerChangedNotice(b, link.StaffChatID, []string{link.GroupTitle})
		}
		return staffRecheckRemoved
	}

	if groupResult == chat_status.OwnerMatch && staffResult == chat_status.OwnerMatch {
		// The owners still match, but the row may be gone (an unlink) or replaced by a
		// new link: only the very row this link names is OK.
		current, err := staff.GetLinkOfGroupFresh(link.GroupChatID)
		if err != nil {
			log.Warnf("[Staff] recheck: link of group %d could not be read: %v", link.GroupChatID, err)
			return staffRecheckUnknown
		}
		if !staffSameLink(current, link) {
			return staffRecheckGone
		}
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

// staffSameLink reports whether current, a fresh read of a group's link, is still the
// link row want names, of the same Staff Group.
func staffSameLink(current *models.StaffGroupLink, want models.StaffGroupLink) bool {
	return current != nil && current.ID == want.ID && current.StaffChatID == want.StaffChatID
}

// staffHealthNoticeText returns the Staff Group heads-up for a link's new health,
// with the group title still a placeholder token. Every case reads a literal
// locale key so make check-translations sees it.
func staffHealthNoticeText(tr *i18n.Translator, health string) string {
	params := i18n.TranslationParams{"group": staffGroupTitleToken}
	var text string
	switch health {
	case models.StaffHealthBotMissing:
		text, _ = tr.GetString("staff_notice_health_bot_missing", params)
	case models.StaffHealthBotNotAdmin:
		text, _ = tr.GetString("staff_notice_health_bot_not_admin", params)
	case models.StaffHealthBotCannotRestrict:
		text, _ = tr.GetString("staff_notice_health_bot_cannot_restrict", params)
	case models.StaffHealthOK:
		text, _ = tr.GetString("staff_notice_health_ok", params)
	}
	return text
}

// applyLinkHealth is the single path through which a link's bot health changes
// (D-14): the my_chat_member watcher, the /staff panel and the hourly sweep all
// call it, and nothing else writes the health column. Losing the bot never
// removes a link; only the health value moves.
//
// The transition is staff.SetLinkHealth, a conditional UPDATE ... WHERE health <>
// new. Only the caller whose statement changed the row posts the one heads-up to
// the Staff Group, in its own language, so a repeated check that finds the same
// health, or several racing callers, post nothing extra. Nothing is ever sent to
// the linked group itself. It reports whether this call changed the health.
func applyLinkHealth(b *gotgbot.Bot, link models.StaffGroupLink, health string) bool {
	changed, err := staff.SetLinkHealth(link.ID, health)
	if err != nil {
		log.Errorf("[Staff] applyLinkHealth: link %d -> %s: %v", link.ID, health, err)
		return false
	}
	if !changed {
		return false
	}
	text := staffHealthNoticeText(staffChatTranslator(link.StaffChatID), health)
	if text == "" {
		return true
	}
	text = strings.Replace(text, staffGroupTitleToken, staffDisplayTitle(link.GroupTitle), 1)
	_ = sendStaffNotice(b, link.StaffChatID, text)
	return true
}

// staffRecheckSummary is what one recheckStaffGroup run did.
type staffRecheckSummary struct {
	// RemovedGroupIDs lists the linked groups this run unlinked, in link-id
	// order within each kind of removal.
	RemovedGroupIDs []int64
	// Unknown is true when some Telegram answer was missing, or the pass was
	// stopped early, so not every link was judged. Nothing was removed for it.
	Unknown bool
}

// recheckStaffGroup rechecks a Staff Group and all of its links live (D-15). Like
// recheckLink it is part of the single authority recheck: the hourly sweep, the
// panel, the ownership watchers and Phase 2's pre-action check all use it, and
// only a successful Telegram answer showing a different or missing creator
// removes anything.
//
// It asks Telegram who the Staff Group's creator is. On an error nothing is
// deleted (the chat may also have migrated, which re-keys it). The recorded
// staff_groups.owner_user_id is only a lookup hint, so it is refreshed to the live
// creator when it differs. A link whose maker is not that live creator is removed
// (D-12); every such removal is one conditional DELETE, and the callers that won
// get one combined notice in id order. Links whose maker still owns the Staff
// Group continue with their own group-side recheckLink, sharing the Staff Group
// answer already in hand. A Staff Group keeps its Staff status whoever owns it.
//
// pace, when not nil, is called before each group-side check and may block to
// respect Bot API rate limits; a false return stops the run early with Unknown set.
func recheckStaffGroup(ctx context.Context, b *gotgbot.Bot, staffChatID int64, pace func(context.Context) bool) staffRecheckSummary {
	var summary staffRecheckSummary

	sg, err := staff.GetStaffGroupFresh(staffChatID)
	if err != nil {
		summary.Unknown = true
		return summary
	}
	if sg == nil {
		return summary
	}

	result, liveOwner, err := chat_status.CheckOwner(b, staffChatID, sg.OwnerUserID)
	if result == chat_status.OwnerUnknown {
		log.Warnf("[Staff] recheck: owner check for Staff Group %d failed: %v", staffChatID, err)
		rekeyFromTelegramError(staffChatID, err)
		summary.Unknown = true
		return summary
	}
	if liveOwner != 0 && liveOwner != sg.OwnerUserID {
		if _, err := staff.UpdateStaffGroupOwner(staffChatID, liveOwner); err != nil {
			log.Errorf("[Staff] recheck: refresh owner of Staff Group %d: %v", staffChatID, err)
		}
	}

	links, err := staff.ListLinksByStaffFresh(staffChatID)
	if err != nil {
		summary.Unknown = true
		return summary
	}

	pass := newStaffOwnerPass()
	if liveOwner != 0 {
		pass.seed(staffChatID, liveOwner, chat_status.OwnerMatch, liveOwner, nil)
	}
	var removedTitles []string
	defer func() { postStaffOwnerChangedNotice(b, staffChatID, removedTitles) }()

	for _, link := range links {
		if link.OwnerUserID != liveOwner {
			deleted, err := staff.DeleteLinkIfOwner(link.ID, link.OwnerUserID)
			if err != nil {
				summary.Unknown = true
				continue
			}
			if deleted {
				summary.RemovedGroupIDs = append(summary.RemovedGroupIDs, link.GroupChatID)
				removedTitles = append(removedTitles, link.GroupTitle)
			}
			continue
		}
		if ctx.Err() != nil || (pace != nil && !pace(ctx)) {
			summary.Unknown = true
			return summary
		}
		switch recheckLink(b, link, pass) {
		case staffRecheckRemoved:
			summary.RemovedGroupIDs = append(summary.RemovedGroupIDs, link.GroupChatID)
		case staffRecheckUnknown:
			summary.Unknown = true
		}
	}
	return summary
}
