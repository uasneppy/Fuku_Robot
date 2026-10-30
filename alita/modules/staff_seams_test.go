//go:build testtools

package modules

import (
	"errors"
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
	"github.com/divkix/Alita_Robot/alita/db/staff"
	"github.com/divkix/Alita_Robot/alita/i18n"
	"github.com/divkix/Alita_Robot/alita/utils/chat_status"
)

// staffSeamsEnglish returns a translator over the real en.yml strings, so a
// length test sees the lengths users see.
func staffSeamsEnglish(t *testing.T) *i18n.Translator {
	t.Helper()
	raw, err := os.ReadFile("../../locales/en.yml")
	if err != nil {
		t.Fatalf("read locale en: %v", err)
	}
	tr, err := i18n.NewTestTranslator(string(raw))
	if err != nil {
		t.Fatalf("parse locale en: %v", err)
	}
	return tr
}

func TestStaffRowSplitMatchesRow(t *testing.T) {
	tr := staffSeamsEnglish(t)
	locked := time.Date(2026, 10, 30, 12, 34, 0, 0, time.UTC)

	healths := []struct {
		name        string
		health      string
		healthKnown bool
	}{
		{"ok", models.StaffHealthOK, true},
		{"bot missing", models.StaffHealthBotMissing, true},
		{"bot not admin", models.StaffHealthBotNotAdmin, true},
		{"bot cannot restrict", models.StaffHealthBotCannotRestrict, true},
		{"unknown", "", false},
	}
	owners := []struct {
		name  string
		state chat_status.OwnerResult
	}{
		{"owner match", chat_status.OwnerMatch},
		{"owner unknown", chat_status.OwnerUnknown},
	}
	titles := []string{"Plain Group", "Fish & <b>Chips</b> \u00e9\u00e8 \U0001F680 \u4e2d\u6587"}

	for _, h := range healths {
		for _, o := range owners {
			for _, lockedSince := range []time.Time{{}, locked} {
				for ti, title := range titles {
					name := fmt.Sprintf("%s/%s/locked=%t/title%d", h.name, o.name, !lockedSince.IsZero(), ti)
					t.Run(name, func(t *testing.T) {
						row := staffLinkRow{
							Link: models.StaffGroupLink{
								ID:          7,
								GroupChatID: -1002345678901,
								StaffChatID: -1001111111111,
								OwnerUserID: 42,
								GroupTitle:  title,
							},
							Health:      h.health,
							HealthKnown: h.healthKnown,
							OwnerState:  o.state,
							LockedSince: lockedSince,
						}
						head := renderStaffRowHead(row)
						status := renderStaffRowStatus(tr, row)

						if got, want := head+"\n"+status, renderStaffRow(tr, row); got != want {
							t.Fatalf("head + newline + status =\n%q\nwant renderStaffRow =\n%q", got, want)
						}
						if !strings.Contains(head, staffDisplayTitle(title)) {
							t.Errorf("head %q must carry the escaped title %q", head, staffDisplayTitle(title))
						}
						if !strings.Contains(head, "<code>-1002345678901</code>") {
							t.Errorf("head %q must carry the chat ID in code", head)
						}
						for _, leak := range []string{title, html.EscapeString(title), staffDisplayTitle(title), "-1002345678901"} {
							if strings.Contains(status, leak) {
								t.Errorf("status %q must not contain %q", status, leak)
							}
						}
						if status == "" {
							t.Errorf("status of a row must not be empty")
						}
					})
				}
			}
		}
	}
}

func TestStaffPanelTextLimit(t *testing.T) {
	tr := staffSeamsEnglish(t)
	now := time.Date(2026, 10, 30, 9, 8, 7, 0, time.UTC)
	group := panelRenderGroup()

	view := func(rows []staffLinkRow) staffPanelView {
		return staffPanelView{
			staffGroup: group,
			total:      len(rows),
			shown:      rows,
			page:       0,
			pages:      1,
			updatedAt:  now,
		}
	}

	t.Run("limit 0 is the default cap", func(t *testing.T) {
		for _, n := range []int{0, 1, 3, 8} {
			rows := panelRows(n, func(int) string { return strings.Repeat("&", 64) })
			for i := range rows {
				rows[i].Health = models.StaffHealthBotCannotRestrict
			}
			zero := view(rows)
			capped := view(rows)
			capped.limit = staffPanelMaxUTF16

			zeroText, zeroShown := staffPanelTextFor(tr, zero)
			cappedText, cappedShown := staffPanelTextFor(tr, capped)
			if zeroText != cappedText || len(zeroShown) != len(cappedShown) {
				t.Errorf("%d rows: limit 0 gave %d rows and %q, limit %d gave %d rows and %q",
					n, len(zeroShown), zeroText, staffPanelMaxUTF16, len(cappedShown), cappedText)
			}
			if got := panelUTF16Len(zeroText); got > staffPanelMaxUTF16 {
				t.Errorf("%d rows: text is %d UTF-16 units, want at most %d", n, got, staffPanelMaxUTF16)
			}
		}
	})

	t.Run("a small limit swaps the help and drops rows with the note", func(t *testing.T) {
		rows := panelRows(8, func(i int) string { return "Group" + string(rune('A'+i)) })
		const limit = 600

		text, shown := staffPanelTextFor(tr, func() staffPanelView { v := view(rows); v.limit = limit; return v }())

		if got := panelUTF16Len(text); got > limit {
			t.Fatalf("text is %d UTF-16 units, want at most %d:\n%s", got, limit, text)
		}
		hint, _ := tr.GetString("staff_panel_help_hint")
		if !strings.Contains(text, hint) {
			t.Errorf("text must use the help hint %q", hint)
		}
		if len(shown) == 0 || len(shown) >= len(rows) {
			t.Fatalf("%d rows kept, want some but fewer than %d", len(shown), len(rows))
		}
		note, _ := tr.GetString("staff_panel_truncated", i18n.TranslationParams{"count": len(rows) - len(shown)})
		if !strings.Contains(text, note) {
			t.Errorf("text must carry the note %q for the %d dropped rows", note, len(rows)-len(shown))
		}
		// The default limit keeps every one of these rows, so the drop is the limit's doing.
		_, defaultShown := staffPanelTextFor(tr, view(rows))
		if len(defaultShown) != len(rows) {
			t.Fatalf("default limit kept %d rows, want all %d", len(defaultShown), len(rows))
		}
	})

	t.Run("a text that fits is unchanged by any limit it fits", func(t *testing.T) {
		rows := panelRows(1, func(int) string { return "Tiny" })
		base, baseShown := staffPanelTextFor(tr, view(rows))
		need := panelUTF16Len(base)
		for _, limit := range []int{0, need, need + 1, staffPanelMaxUTF16} {
			v := view(rows)
			v.limit = limit
			text, shown := staffPanelTextFor(tr, v)
			if text != base || len(shown) != len(baseShown) {
				t.Errorf("limit %d changed a text that fits in %d units: %q", limit, need, text)
			}
		}
	})
}

// staffSeamsOwner is the issuer of the seam tests' link env.
func staffSeamsOwner(e *linkEnv) *gotgbot.User {
	return &gotgbot.User{Id: e.ownerID, FirstName: "Owner"}
}

func TestListVerifiedOwnStaffGroups(t *testing.T) {
	t.Run("no Staff Group", func(t *testing.T) {
		env := newLinkEnv(t)
		stranger := &gotgbot.User{Id: uniqueLinkOwnerID(), FirstName: "Nobody"}

		matched, unknown, err := listVerifiedOwnStaffGroups(env.bot, stranger)

		if err != nil || unknown != 0 || len(matched) != 0 {
			t.Fatalf("got (%+v, %d, %v), want an empty list, 0 unknown and no error", matched, unknown, err)
		}
	})

	t.Run("two live-created Staff Groups come back in order", func(t *testing.T) {
		env := newLinkEnv(t)
		second := uniqueModuleChatID()
		env.client.setCreator(second, env.ownerID)
		staffCleanup(t, second)
		if _, err := staff.CreateStaffGroup(second, env.ownerID, "Second HQ"); err != nil {
			t.Fatalf("create second Staff Group: %v", err)
		}

		matched, unknown, err := listVerifiedOwnStaffGroups(env.bot, staffSeamsOwner(env))

		if err != nil || unknown != 0 {
			t.Fatalf("got unknown %d, err %v, want 0 and nil", unknown, err)
		}
		if len(matched) != 2 || matched[0].ChatID != env.staffID || matched[1].ChatID != second {
			t.Fatalf("matched = %+v, want [%d %d] in the repository's order", matched, env.staffID, second)
		}
	})

	t.Run("a Staff Group created by someone else is left out", func(t *testing.T) {
		env := newLinkEnv(t)
		second := uniqueModuleChatID()
		env.client.setCreator(second, env.ownerID+1)
		staffCleanup(t, second)
		if _, err := staff.CreateStaffGroup(second, env.ownerID, "Handed over"); err != nil {
			t.Fatalf("create second Staff Group: %v", err)
		}

		matched, unknown, err := listVerifiedOwnStaffGroups(env.bot, staffSeamsOwner(env))

		if err != nil || unknown != 0 {
			t.Fatalf("got unknown %d, err %v, want 0 and nil", unknown, err)
		}
		if len(matched) != 1 || matched[0].ChatID != env.staffID {
			t.Fatalf("matched = %+v, want only %d: the recorded owner is never an authority", matched, env.staffID)
		}
	})

	t.Run("a failed owner check is counted and the group left out", func(t *testing.T) {
		env := newLinkEnv(t)
		second := uniqueModuleChatID()
		env.client.setCreator(second, env.ownerID)
		env.client.setFailure("getChatAdministrators", second, errors.New("telegram is down"))
		staffCleanup(t, second)
		if _, err := staff.CreateStaffGroup(second, env.ownerID, "Unreachable"); err != nil {
			t.Fatalf("create second Staff Group: %v", err)
		}

		matched, unknown, err := listVerifiedOwnStaffGroups(env.bot, staffSeamsOwner(env))

		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if unknown != 1 {
			t.Fatalf("unknown = %d, want 1", unknown)
		}
		if len(matched) != 1 || matched[0].ChatID != env.staffID {
			t.Fatalf("matched = %+v, want only %d", matched, env.staffID)
		}
	})
}

func TestLinkToStaffGroup(t *testing.T) {
	staffGroupOf := func(t *testing.T, e *linkEnv) *models.StaffGroup {
		t.Helper()
		group, err := staff.GetStaffGroupFresh(e.staffID)
		if err != nil || group == nil {
			t.Fatalf("Staff Group %d = (%+v, %v), want a row", e.staffID, group, err)
		}
		return group
	}
	wantNoLink := func(t *testing.T, e *linkEnv) {
		t.Helper()
		link, err := staff.GetLinkOfGroupFresh(e.groupID)
		if err != nil || link != nil {
			t.Fatalf("link of group = (%+v, %v), want none", link, err)
		}
	}
	wantNoMessage := func(t *testing.T, e *linkEnv) {
		t.Helper()
		if sent := e.client.callsFor("sendMessage"); len(sent) != 0 {
			t.Fatalf("%d messages were sent, want none: linkToStaffGroup never speaks", len(sent))
		}
	}

	t.Run("a supergroup the issuer created is linked", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.botRole[env.groupID] = staffRoleAdminNoRestrict
		chat := env.groupChat("Linked <Group>")

		out := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, staffGroupOf(t, env))

		if out.Refusal != staffRefusalNone || out.Link == nil || out.StaffGroup == nil {
			t.Fatalf("outcome = %+v, want a link and no refusal", out)
		}
		if out.StaffGroup.ChatID != env.staffID {
			t.Errorf("outcome Staff Group = %d, want %d", out.StaffGroup.ChatID, env.staffID)
		}
		link := env.wantLink(t, models.StaffHealthBotCannotRestrict)
		if link.GroupTitle != "Linked <Group>" {
			t.Errorf("stored title = %q, want the target's title", link.GroupTitle)
		}
		wantNoMessage(t, env)
	})

	t.Run("a basic group is refused", func(t *testing.T) {
		env := newLinkEnv(t)
		chat := gotgbot.Chat{Id: env.groupID, Type: "group", Title: "Basic"}

		out := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, staffGroupOf(t, env))

		if out.Refusal != staffRefusalNotSupergroup || out.Link != nil {
			t.Fatalf("outcome = %+v, want staffRefusalNotSupergroup and no link", out)
		}
		wantNoLink(t, env)
		wantNoMessage(t, env)
	})

	t.Run("a Staff Group as target is refused", func(t *testing.T) {
		env := newLinkEnv(t)
		other := uniqueModuleChatID()
		env.client.setCreator(other, env.ownerID)
		staffCleanup(t, other)
		if _, err := staff.CreateStaffGroup(other, env.ownerID, "Other HQ"); err != nil {
			t.Fatalf("create second Staff Group: %v", err)
		}
		chat := gotgbot.Chat{Id: other, Type: "supergroup", Title: "Other HQ"}

		out := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, staffGroupOf(t, env))

		if out.Refusal != staffRefusalTargetIsStaff || out.Link != nil {
			t.Fatalf("outcome = %+v, want staffRefusalTargetIsStaff and no link", out)
		}
		if link, err := staff.GetLinkOfGroupFresh(other); err != nil || link != nil {
			t.Fatalf("link of the target Staff Group = (%+v, %v), want none", link, err)
		}
		wantNoMessage(t, env)
	})

	t.Run("a target created by someone else is refused", func(t *testing.T) {
		env := newLinkEnv(t)
		env.client.setCreator(env.groupID, env.ownerID+1)
		chat := env.groupChat("Not mine")

		out := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, staffGroupOf(t, env))

		if out.Refusal != staffRefusalNotTargetOwner || out.Link != nil {
			t.Fatalf("outcome = %+v, want staffRefusalNotTargetOwner and no link", out)
		}
		wantNoLink(t, env)
		wantNoMessage(t, env)
	})

	t.Run("a target that is already linked is refused", func(t *testing.T) {
		env := newLinkEnv(t)
		chat := env.groupChat("Twice")
		group := staffGroupOf(t, env)
		if first := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, group); first.Refusal != staffRefusalNone {
			t.Fatalf("first link refused with %v", first.Refusal)
		}

		out := linkToStaffGroup(env.bot, staffSeamsOwner(env), &chat, group)

		if out.Refusal != staffRefusalAlreadyLinked || out.Link != nil {
			t.Fatalf("outcome = %+v, want staffRefusalAlreadyLinked and no link", out)
		}
		env.wantLink(t, models.StaffHealthOK)
		wantNoMessage(t, env)
	})
}

func TestAnnounceStaffLink(t *testing.T) {
	newOutcome := func(e *linkEnv, health string) staffLinkOutcome {
		return staffLinkOutcome{
			StaffGroup: &models.StaffGroup{ChatID: e.staffID, OwnerUserID: e.ownerID},
			Link: &models.StaffGroupLink{
				GroupChatID: e.groupID,
				StaffChatID: e.staffID,
				OwnerUserID: e.ownerID,
				GroupTitle:  "Fish & <b>Chips</b>",
				Health:      health,
			},
		}
	}

	t.Run("a healthy link is announced to the Staff Group only", func(t *testing.T) {
		env := newLinkEnv(t)

		announceStaffLink(env.bot, newOutcome(env, models.StaffHealthOK))

		if all := env.client.callsFor("sendMessage"); len(all) != 1 {
			t.Fatalf("%d messages sent, want exactly one", len(all))
		}
		notices := textsToChat(env.client, env.staffID)
		if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_link_done")) {
			t.Fatalf("Staff Group notices = %q, want one staff_link_done", notices)
		}
		if !strings.Contains(notices[0], "Fish &amp; &lt;b&gt;Chips&lt;/b&gt;") ||
			!strings.Contains(notices[0], strconv.FormatInt(env.groupID, 10)) {
			t.Errorf("notice %q must carry the escaped title and the group ID", notices[0])
		}
		if strings.Contains(notices[0], "staff-link-warn") {
			t.Errorf("notice %q carries a warning for a healthy link", notices[0])
		}
		if sent := callsToChat(env.client, "sendMessage", env.groupID); len(sent) != 0 {
			t.Fatalf("%d messages sent to the linked group, want none", len(sent))
		}
	})

	t.Run("a bot without rights adds its warning line", func(t *testing.T) {
		env := newLinkEnv(t)

		announceStaffLink(env.bot, newOutcome(env, models.StaffHealthBotNotAdmin))

		notices := textsToChat(env.client, env.staffID)
		if len(notices) != 1 || !strings.Contains(notices[0], staffMarker("staff_link_done")) ||
			!strings.Contains(notices[0], staffMarker("staff_link_warn_bot_not_admin")) {
			t.Fatalf("Staff Group notices = %q, want one staff_link_done with the not-admin warning", notices)
		}
		if sent := callsToChat(env.client, "sendMessage", env.groupID); len(sent) != 0 {
			t.Fatalf("%d messages sent to the linked group, want none", len(sent))
		}
	})
}
