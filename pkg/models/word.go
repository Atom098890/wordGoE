package models

import "time"

// Word represents an English word to be learned
type Word struct {
	ID           int       `json:"id" db:"id"`
	Word         string    `json:"word" db:"word"`
	Translation  string    `json:"translation" db:"translation"`
	Context      string    `json:"context,omitempty" db:"context"`
	TopicID      int64     `json:"topic_id" db:"topic_id"`
	Difficulty   int       `json:"difficulty,omitempty" db:"difficulty"` // 1-5 scale of difficulty
	Pronunciation string    `json:"pronunciation,omitempty" db:"pronunciation"` // Optional: URL to audio pronunciation
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
} 