//go:build testtools

package raiddetection

import (
	"testing"

	"github.com/divkix/Alita_Robot/alita/db"
	"github.com/divkix/Alita_Robot/alita/db/models"
)

func TestLoadRaidDetectionStats(t *testing.T) {
	// Other tests of the package share the database, so count the change, not the total.
	baseConfigured, baseJoinOff, baseFloodOff, baseAIOff := LoadRaidDetectionStats()

	rows := []*models.ChatRaidDetection{
		{ChatID: uniqueRaidChatID(), JoinEnabled: true, JoinThreshold: DefaultJoinThreshold, FloodEnabled: true, FloodThreshold: DefaultFloodThreshold, AIEnabled: true, AIThreshold: DefaultAIThreshold},
		{ChatID: uniqueRaidChatID(), JoinEnabled: false, JoinThreshold: DefaultJoinThreshold, FloodEnabled: true, FloodThreshold: DefaultFloodThreshold, AIEnabled: true, AIThreshold: DefaultAIThreshold},
		{ChatID: uniqueRaidChatID(), JoinEnabled: true, JoinThreshold: DefaultJoinThreshold, FloodEnabled: false, FloodThreshold: DefaultFloodThreshold, AIEnabled: true, AIThreshold: DefaultAIThreshold},
		{ChatID: uniqueRaidChatID(), JoinEnabled: true, JoinThreshold: DefaultJoinThreshold, FloodEnabled: true, FloodThreshold: DefaultFloodThreshold, AIEnabled: false, AIThreshold: DefaultAIThreshold},
	}
	t.Cleanup(func() {
		for _, row := range rows {
			db.DB.Where("chat_id = ?", row.ChatID).Delete(&models.ChatRaidDetection{})
		}
	})
	for _, row := range rows {
		if err := db.DB.Create(row).Error; err != nil {
			t.Fatalf("seed a settings row: %v", err)
		}
	}

	configured, joinOff, floodOff, aiOff := LoadRaidDetectionStats()
	if got := configured - baseConfigured; got != 4 {
		t.Errorf("chats with their own settings grew by %d, want 4", got)
	}
	if got := joinOff - baseJoinOff; got != 1 {
		t.Errorf("chats with the join surge off grew by %d, want 1", got)
	}
	if got := floodOff - baseFloodOff; got != 1 {
		t.Errorf("chats with the flood off grew by %d, want 1", got)
	}
	if got := aiOff - baseAIOff; got != 1 {
		t.Errorf("chats with the AI trigger off grew by %d, want 1", got)
	}
}
