package models

import "time"

// User represents a Telegram user using the bot
type User struct {
	ID                  int64     `json:"id" db:"id"`
	TelegramID          int64     `json:"telegram_id" db:"telegram_id"`
	Username            string    `json:"username" db:"username"`
	FirstName           string    `json:"first_name" db:"first_name"`
	LastName            string    `json:"last_name" db:"last_name"`
	IsAdmin             bool      `json:"is_admin" db:"is_admin"`
	PreferredTopics     []int64   `json:"preferred_topics" db:"preferred_topics"` // Array of topic IDs
	NotificationEnabled bool      `json:"notification_enabled" db:"notification_enabled"`
	NotificationHour    int       `json:"notification_hour" db:"notification_hour"` // Hour of day for notifications (0-23)
	WordsPerDay         int       `json:"words_per_day" db:"words_per_day"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
} 