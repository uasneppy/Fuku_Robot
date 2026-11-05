package modules

import (
	"context"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/uasneppy/fuku_robot/fuku/db/models"
	"github.com/uasneppy/fuku_robot/fuku/utils/ratelimit"
)

var memberReqPacer = ratelimit.NewTelegramPacer(ratelimit.TelegramPacerOptions{
	NextKey:    "fuku:memberreq:pace:next",
	BlockKey:   "fuku:memberreq:pace:block",
	Interval:   100 * time.Millisecond,
	MaxRetries: 3,
	MaxWait:    60 * time.Second,
})

type memberReqSource int

const (
	memberReqFromMessage memberReqSource = iota
	memberReqFromJoin
)

type memberReqCase struct {
	Chat                   gotgbot.Chat
	User                   gotgbot.User
	Source                 memberReqSource
	MessageID, MessageDate int64
	Settings               *models.ChatRequirements
}

type memberReqOutcome int

const (
	memberReqNone memberReqOutcome = iota
	memberReqHeld
	memberReqWarned
	memberReqRemoved
	memberReqMuted
	memberReqNotApplied
)

type memberReqResult struct {
	Outcome memberReqOutcome
	Hold    *models.ChatRequirementHold
}

var memberReqApplyWarn = applyWarn

func memberReqEnforce(_ context.Context, _ *gotgbot.Bot, _ memberReqCase) memberReqResult {
	return memberReqResult{}
}
