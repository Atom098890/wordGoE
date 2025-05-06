package models

import "time"

// Statistics tracks user's progress with topics
type Statistics struct {
    ID                   int64     `json:"id" db:"id"`
    UserID               int64     `json:"user_id" db:"user_id"`
    TopicID              int64     `json:"topic_id" db:"topic_id"`
    TopicName            string    `json:"topic_name" db:"topic_name"`
    TotalRepetitions     int       `json:"total_repetitions" db:"total_repetitions"`
    CompletedRepetitions int       `json:"completed_repetitions" db:"completed_repetitions"`
    CreatedAt           time.Time `json:"created_at" db:"created_at"`
    UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
} 