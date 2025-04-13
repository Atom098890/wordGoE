package models

import "time"

// TestResult tracks results from knowledge tests
type TestResult struct {
	ID           int       `json:"id" db:"id"`
	UserID       int64     `json:"user_id" db:"user_id"`
	TestType     string    `json:"test_type" db:"test_type"` // e.g., "multiple_choice", "text_input", "context"
	TotalWords   int       `json:"total_words" db:"total_words"`
	CorrectWords int       `json:"correct_words" db:"correct_words"`
	Topics       []int64   `json:"topics" db:"topics"` // Topic IDs included in the test
	TestDate     time.Time `json:"test_date" db:"test_date"`
	Duration     int       `json:"duration" db:"duration"` // Duration in seconds
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
} 