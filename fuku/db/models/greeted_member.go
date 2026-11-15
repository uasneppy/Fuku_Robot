package models

import "time"

// GreetedMember records that the bot welcomed a member in a chat while Greet once was on:
// one row per (chat, member), kept for good. It is read fresh from the database and never
// cached, so no cache key needs deleting on a write, and it is never part of backup,
// export, import or reset.
//
// There is deliberately no foreign key and no gorm default tag: the claim must work for a
// chat the chats table does not know yet, and the primary key on both columns is what lets
// two replicas that claim at once agree on one winner.
type GreetedMember struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey;autoIncrement:false"`
	UserID    int64     `gorm:"column:user_id;primaryKey;autoIncrement:false"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName returns the migration's table name.
func (GreetedMember) TableName() string {
	return "greeted_members"
}
