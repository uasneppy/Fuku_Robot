//go:build testtools

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
)

// withStaffActionTimers sets the three timing knobs of a run for one test and
// restores them afterwards.
func withStaffActionTimers(t *testing.T, editEvery, retryUnit time.Duration) {
	t.Helper()
	prevEvery, prevUnit, prevStop := staffActionEditEvery, staffActionEditRetryUnit, staffActionStopWait
	staffActionEditEvery, staffActionEditRetryUnit = editEvery, retryUnit
	t.Cleanup(func() {
		staffActionEditEvery, staffActionEditRetryUnit, staffActionStopWait = prevEvery, prevUnit, prevStop
	})
}

// startRun sends command in the Staff Group and presses Confirm, without waiting
// for the run.
func (e *staffActionEnv) startRun(command string) (token string, msgID int64) {
	e.t.Helper()
	e.send(e.issuer, command)
	token, msgID = e.card()
	e.tap(e.issuer, staffActRunConfirm, token, msgID)
	return token, msgID
}

// addTitledGroups links n more groups with 64-rune titles, in which the issuer is
// only a plain member, so every one of them is skipped.
func (e *staffActionEnv) addTitledGroups(n int) {
	e.t.Helper()
	for i := 0; i < n; i++ {
		group := uniqueModuleChatID()
		staffCleanup(e.t, group)
		e.fake.setCreator(group, e.owner)
		link := &models.StaffGroupLink{
			GroupChatID: group,
			StaffChatID: e.staffChat,
			OwnerUserID: e.owner,
			GroupTitle:  summaryTitle(i),
		}
		if err := staff.CreateLink(link); err != nil {
			e.t.Fatalf("create link %d: %v", i, err)
		}
		e.fake.setMember(group, e.issuer.Id, staffFakeMember{Status: gotgbot.ChatMemberStatusMember})
		e.groups = append(e.groups, group)
	}
}

// sentTexts returns the text of every message sent to the Staff Group after the
// card, in order.
func (e *staffActionEnv) sentTexts() []string {
	var texts []string
	for _, sent := range e.fake.sentTo(e.staffChat)[1:] {
		texts = append(texts, fmt.Sprint(sent.Params["text"]))
	}
	return texts
}

func hasLinePrefix(text, prefix string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func TestStaffActionProgressBatched(t *testing.T) {
	env := newStaffActionEnv(t, 6)
	seedMembers(env)
	withStaffActionTimers(t, 40*time.Millisecond, 5*time.Millisecond)
	for _, group := range env.groups {
		env.fake.setDelay("getChatMember", group, 60*time.Millisecond)
	}
	env.send(env.issuer, "/ban 4242")
	token, msgID := env.card()

	start := time.Now()
	env.tap(env.issuer, staffActRunConfirm, token, msgID)
	env.waitRuns()
	elapsed := time.Since(start)

	edits := env.edits(env.staffChat, msgID)
	if len(edits) < 3 {
		t.Fatalf("card edits = %d, want the initial one, at least one progress edit and the final one", len(edits))
	}
	sawMixed := false
	for _, call := range edits[1 : len(edits)-1] {
		text := fmt.Sprint(call.Params["text"])
		if hasLinePrefix(text, "✅ Group ") && hasLinePrefix(text, "⏳ Group ") {
			sawMixed = true
		}
	}
	if !sawMixed {
		t.Fatalf("no progress edit showed a finished group and a pending one together; edits = %d", len(edits))
	}
	if final := env.lastEditText(env.staffChat, msgID); strings.Contains(final, "⏳") {
		t.Fatalf("final summary still has a pending line:\n%s", final)
	}
	maxAfterInitial := int((elapsed+staffActionEditEvery-1)/staffActionEditEvery) + 2
	if got := len(edits) - 1; got > maxAfterInitial {
		t.Fatalf("%d card edits after the initial one in %s, want at most %d at one per %s",
			got, elapsed, maxAfterInitial, staffActionEditEvery)
	}
}

func TestStaffActionFinalEditRetriesOn429(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	for _, group := range env.groups {
		env.fake.setDelay("banChatMember", group, 300*time.Millisecond)
	}

	_, msgID := env.startRun("/ban 4242")
	env.fake.script("editMessageText", env.staffChat, staffFake429(1), staffFake429(1))
	env.waitRuns()

	edits := env.edits(env.staffChat, msgID)
	if got := len(edits) - 1; got != 3 {
		t.Fatalf("final edit attempts = %d, want 3 (two 429s, then success)", got)
	}
	if sent := env.fake.sentTo(env.staffChat); len(sent) != 1 {
		t.Fatalf("messages to the Staff Group = %d, want only the card: the third attempt succeeded", len(sent))
	}
	last := fmt.Sprint(edits[len(edits)-1].Params["text"])
	if strings.Contains(last, "⏳") || !strings.Contains(last, "Group A") {
		t.Fatalf("last edit is not the final summary:\n%s", last)
	}
}

func TestStaffActionFinalFallsBackToNewMessage(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	for _, group := range env.groups {
		env.fake.setDelay("banChatMember", group, 300*time.Millisecond)
	}

	env.startRun("/ban 4242")
	notFound := staffFakeError(400, "Bad Request: message to edit not found")
	env.fake.script("editMessageText", env.staffChat, notFound, notFound, notFound, notFound)
	env.waitRuns()

	texts := env.sentTexts()
	if len(texts) != 1 {
		t.Fatalf("new messages to the Staff Group = %d, want exactly one summary", len(texts))
	}
	for _, want := range []string{"Group A", "Group B"} {
		if !strings.Contains(texts[0], want) {
			t.Fatalf("fallback summary lacks %q:\n%s", want, texts[0])
		}
	}
	if strings.Contains(texts[0], "⏳") {
		t.Fatalf("fallback summary still has a pending line:\n%s", texts[0])
	}
}

func TestStaffActionNotModifiedIsSuccess(t *testing.T) {
	env := newStaffActionEnv(t, 2)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	for _, group := range env.groups {
		env.fake.setDelay("banChatMember", group, 300*time.Millisecond)
	}

	_, msgID := env.startRun("/ban 4242")
	env.fake.script("editMessageText", env.staffChat, staffFakeError(400,
		"Bad Request: message is not modified: specified new message content and reply markup are exactly the same"))
	env.waitRuns()

	if sent := env.fake.sentTo(env.staffChat); len(sent) != 1 {
		t.Fatalf("messages to the Staff Group = %d, want only the card: 'not modified' is a success", len(sent))
	}
	if got := len(env.edits(env.staffChat, msgID)) - 1; got != 1 {
		t.Fatalf("final edit attempts = %d, want 1", got)
	}
}

func TestStaffActionOverflowContinuation(t *testing.T) {
	env := newStaffActionEnv(t, 0)
	env.addTitledGroups(120)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)

	_, msgID := env.startRun("/ban 4242")
	env.waitRuns()

	continuation := env.sentTexts()
	if len(continuation) == 0 {
		t.Fatal("no continuation message was posted for 120 skipped groups")
	}
	parts := append([]string{env.lastEditText(env.staffChat, msgID)}, continuation...)
	for i, part := range parts {
		wantFits(t, part)
		if strings.Contains(part, "⏳") {
			t.Fatalf("part %d still has a pending line", i)
		}
	}
	all := strings.Join(parts, "\n")
	for i := range env.groups {
		if n := strings.Count(all, summaryTitle(i)); n != 1 {
			t.Fatalf("group %d appears %d times across the final edit and the continuations, want once", i, n)
		}
	}
}

func TestStaffActionAllSkippedTally(t *testing.T) {
	env := newStaffActionEnv(t, 0)
	env.addTitledGroups(3)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)

	summary, _ := env.runStaffCommand("/ban 4242")

	if !strings.Contains(summary, "✅ 0 · ⏭ 3 · ❌ 0") {
		t.Fatalf("summary lacks the all-skipped tally:\n%s", summary)
	}
	for _, line := range summaryGroupLines(t, summary) {
		if strings.HasPrefix(line, "⏳") {
			t.Fatalf("a line is still pending: %q", line)
		}
	}
}

func TestStopStaffActionsFinalizes(t *testing.T) {
	env := newStaffActionEnv(t, 8)
	seedMembers(env)
	withStaffActionTimers(t, time.Hour, 5*time.Millisecond)
	t.Cleanup(func() {
		staffActionsMu.Lock()
		defer staffActionsMu.Unlock()
		staffActionsCtx, staffActionsCancel = context.WithCancel(context.Background())
	})
	for _, group := range env.groups {
		env.fake.setDelay("getChatMember", group, 200*time.Millisecond)
	}

	_, msgID := env.startRun("/ban 4242")
	time.Sleep(50 * time.Millisecond)
	stopped := time.Now()
	StopStaffActions()
	if took := time.Since(stopped); took > 5*time.Second {
		t.Fatalf("StopStaffActions took %s, want under 5s", took)
	}
	env.waitRuns()

	summary := env.lastEditText(env.staffChat, msgID)
	if !strings.Contains(summary, staffMarker("staff_act_fail_interrupted")) {
		t.Fatalf("final summary has no interrupted group:\n%s", summary)
	}
	if strings.Contains(summary, "⏳") {
		t.Fatalf("final summary still has a pending line:\n%s", summary)
	}
	done, skipped, failed := summaryTallyOf(t, summary)
	if done+skipped+failed != 8 {
		t.Fatalf("tally %d+%d+%d, want 8 groups", done, skipped, failed)
	}
	done2 := make(chan struct{})
	go func() {
		staffActionRunsWG.Wait()
		close(done2)
	}()
	select {
	case <-done2:
	case <-time.After(time.Second):
		t.Fatal("staffActionRunsWG is not drained after StopStaffActions")
	}
}
