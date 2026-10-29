package raiddetection

import (
	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

// LoadRaidDetectionStats counts the chats that changed the automatic raid detection
// settings (a row exists), the chats with the join surge trigger turned off, the chats
// with the flood trigger turned off and the chats with the AI trigger turned off, for the
// developer /stats. A chat without a row uses the defaults, so it is counted nowhere.
func LoadRaidDetectionStats() (configured, joinOff, floodOff, aiOff int64) {
	configured = db.CountRows(&models.ChatRaidDetection{}, "")
	joinOff = db.CountRows(&models.ChatRaidDetection{}, "join_enabled = ?", false)
	floodOff = db.CountRows(&models.ChatRaidDetection{}, "flood_enabled = ?", false)
	aiOff = db.CountRows(&models.ChatRaidDetection{}, "ai_enabled = ?", false)
	return
}
