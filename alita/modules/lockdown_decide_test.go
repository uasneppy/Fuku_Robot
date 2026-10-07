//go:build testtools

package modules

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestDecideLockdownJoin(t *testing.T) {
	const bot, member, admin int64 = 999, 5001, 6001
	base := lockdownJoinInput{Path: models.JoinPathMember, PerformerID: admin, MemberID: member, BotID: bot}
	with := func(edit func(in *lockdownJoinInput)) lockdownJoinInput {
		in := base
		edit(&in)
		return in
	}

	tests := []struct {
		name       string
		in         lockdownJoinInput
		want       lockdownJoinVerdict
		wantLookup bool
	}{
		{"the bot itself joining is none of the lockdown's business",
			with(func(in *lockdownJoinInput) { in.MemberID = bot }), lockdownJoinIgnore, false},
		{"a join request is declined",
			with(func(in *lockdownJoinInput) { in.Path = models.JoinPathRequest }), lockdownJoinDecline, false},
		{"a bot account is banned even when an admin added it",
			with(func(in *lockdownJoinInput) {
				in.MemberIsBot = true
				in.PerformerStatus = gotgbot.ChatMemberStatusCreator
			}),
			lockdownJoinBan, false},
		{"a bot added by a live creator is banned",
			with(func(in *lockdownJoinInput) {
				in.MemberIsBot = true
				in.Path = models.JoinPathService
				in.PerformerStatus = gotgbot.ChatMemberStatusCreator
			}),
			lockdownJoinBan, false},
		{"a self-join is banned whatever the performer status says",
			with(func(in *lockdownJoinInput) {
				in.PerformerID = member
				in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
			}),
			lockdownJoinBan, false},
		{"a join the bot performed is banned",
			with(func(in *lockdownJoinInput) {
				in.PerformerID = bot
				in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
			}),
			lockdownJoinBan, false},
		{"an anonymous admin adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerAnonymousAdmin = true }), lockdownJoinExempt, false},
		{"a live administrator adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator }),
			lockdownJoinExempt, true},
		{"a live creator adding someone is exempt",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusCreator }),
			lockdownJoinExempt, true},
		{"a live administrator on the service path is exempt too",
			with(func(in *lockdownJoinInput) {
				in.Path = models.JoinPathService
				in.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
			}),
			lockdownJoinExempt, true},
		{"a plain member adding someone is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusMember }),
			lockdownJoinBan, true},
		{"a restricted performer is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusRestricted }),
			lockdownJoinBan, true},
		{"a performer who left is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusLeft }),
			lockdownJoinBan, true},
		{"a kicked performer is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = gotgbot.ChatMemberStatusKicked }),
			lockdownJoinBan, true},
		{"a failed lookup (empty status) is banned",
			with(func(in *lockdownJoinInput) { in.PerformerStatus = "" }), lockdownJoinBan, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideLockdownJoin(tt.in); got != tt.want {
				t.Errorf("decideLockdownJoin = %v, want %v", got, tt.want)
			}
			if got := lockdownNeedsPerformerLookup(tt.in); got != tt.wantLookup {
				t.Errorf("lockdownNeedsPerformerLookup = %v, want %v", got, tt.wantLookup)
			}
		})
	}
}

// TestDecideLockdownJoinInvariants walks every combination of path, performer
// status, member kind and identity relation and checks the rules that must hold
// whatever the combination: nobody who is not an administrator's addition is let in.
func TestDecideLockdownJoinInvariants(t *testing.T) {
	const bot, member, other int64 = 999, 5001, 6001

	paths := []string{models.JoinPathMember, models.JoinPathService, models.JoinPathRequest}
	statuses := []string{
		"",
		gotgbot.ChatMemberStatusCreator,
		gotgbot.ChatMemberStatusAdministrator,
		gotgbot.ChatMemberStatusMember,
		gotgbot.ChatMemberStatusRestricted,
		gotgbot.ChatMemberStatusLeft,
		gotgbot.ChatMemberStatusKicked,
	}
	// Who joined: another user, or the bot itself.
	members := []int64{member, bot}
	// Who performed the join: someone else, the member (a self-join) or the bot.
	performers := []int64{other, member, bot}

	checked := 0
	for _, path := range paths {
		for _, status := range statuses {
			for _, isBot := range []bool{false, true} {
				for _, memberID := range members {
					for _, performerID := range performers {
						for _, anonymous := range []bool{false, true} {
							in := lockdownJoinInput{
								Path: path, PerformerID: performerID, MemberID: memberID, MemberIsBot: isBot,
								BotID: bot, PerformerAnonymousAdmin: anonymous, PerformerStatus: status,
							}
							checked++
							verdict := decideLockdownJoin(in)
							label := func() string {
								return "path=" + path + " status=" + status + " memberIsBot=" + boolText(isBot) +
									" member=" + idText(memberID) + " performer=" + idText(performerID) + " anonymous=" + boolText(anonymous)
							}

							if memberID == bot && verdict != lockdownJoinIgnore {
								t.Errorf("%s: verdict %v, want ignore for the bot itself", label(), verdict)
							}
							if memberID != bot && path == models.JoinPathRequest && verdict != lockdownJoinDecline {
								t.Errorf("%s: verdict %v, want decline for a join request", label(), verdict)
							}
							if verdict == lockdownJoinExempt {
								switch {
								case path == models.JoinPathRequest:
									t.Errorf("%s: a join request was exempt", label())
								case isBot:
									t.Errorf("%s: a bot account was exempt", label())
								case performerID == memberID:
									t.Errorf("%s: a self-join was exempt", label())
								case performerID == bot:
									t.Errorf("%s: a join the bot performed was exempt", label())
								case !anonymous && status != gotgbot.ChatMemberStatusCreator && status != gotgbot.ChatMemberStatusAdministrator:
									t.Errorf("%s: exempt without a creator or administrator performer or an anonymous admin", label())
								}
							}

							// A lookup is needed exactly when the performer's status can change
							// the verdict.
							atNone, atAdmin := in, in
							atNone.PerformerStatus = ""
							atAdmin.PerformerStatus = gotgbot.ChatMemberStatusAdministrator
							dependsOnStatus := decideLockdownJoin(atNone) != decideLockdownJoin(atAdmin)
							if got := lockdownNeedsPerformerLookup(in); got != dependsOnStatus {
								t.Errorf("%s: lockdownNeedsPerformerLookup = %v, want %v", label(), got, dependsOnStatus)
							}
						}
					}
				}
			}
		}
	}
	if checked != len(paths)*len(statuses)*2*len(members)*len(performers)*2 {
		t.Errorf("walked %d combinations, want every one", checked)
	}
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func idText(id int64) string {
	switch id {
	case 999:
		return "bot"
	case 5001:
		return "member"
	}
	return "other"
}
