package database

import (
	"database/sql"
)

// UserConfig represents user configuration
type UserConfig struct {
	UserID           int64
	WordsPerBatch    int
	Repetitions      int
	IsActive         bool
	NotificationHour int
	LastBatchTime    sql.NullTime
	CreatedAt        sql.NullTime
	UpdatedAt        sql.NullTime
} 