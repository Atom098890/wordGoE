package models

import "time"

// Repetition represents a scheduled review of a topic
type Repetition struct {
    ID              int64     `json:"id" db:"id"`
    UserID          int64     `json:"user_id" db:"user_id"`
    TopicID         int64     `json:"topic_id" db:"topic_id"`
    TopicName       string    `json:"topic_name" db:"topic_name"`
    RepetitionNumber int      `json:"repetition_number" db:"repetition_number"`
    NextReviewDate  time.Time `json:"next_review_date" db:"next_review_date"`
    LastReviewDate  *time.Time `json:"last_review_date" db:"last_review_date"`
    Completed       bool      `json:"completed" db:"completed"`
    CreatedAt       time.Time `json:"created_at" db:"created_at"`
    UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
} 