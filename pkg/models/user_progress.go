package models

// UserProgress tracks a user's progress with a specific word using the SM-2 algorithm
type UserProgress struct {
	ID              int       `json:"id" db:"id"`
	UserID          int64     `json:"user_id" db:"user_id"`
	WordID          int       `json:"word_id" db:"word_id"`
	LastReviewDate  string    `json:"last_review_date" db:"last_review_date"`
	NextReviewDate  string    `json:"next_review_date" db:"next_review_date"`
	Interval        int       `json:"interval" db:"interval"`                 // Current interval in days
	EasinessFactor  float64   `json:"easiness_factor" db:"easiness_factor"`   // SM-2 EF parameter
	Repetitions     int       `json:"repetitions" db:"repetitions"`           // Number of repetitions
	LastQuality     int       `json:"last_quality" db:"last_quality"`         // 0-5 rating of last recall
	ConsecutiveRight int      `json:"consecutive_right" db:"consecutive_right"` // Number of consecutive correct recalls
	IsLearned       bool      `json:"is_learned" db:"is_learned"`             // Whether the word is considered learned
	CreatedAt       string    `json:"created_at" db:"created_at"`
	UpdatedAt       string    `json:"updated_at" db:"updated_at"`
} 