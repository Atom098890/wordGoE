package models

import "time"

// Word represents an English word to be learned
type Word struct {
	ID           int       `json:"id" db:"id"`
	EnglishWord  string    `json:"english_word" db:"english_word"`
	Translation  string    `json:"translation" db:"translation"`
	Context      string    `json:"context" db:"context"`
	TopicID      int64     `json:"topic_id" db:"topic_id"`
	Difficulty   int       `json:"difficulty" db:"difficulty"` // 1-5 scale of difficulty
	Pronunciation string    `json:"pronunciation" db:"pronunciation"` // Optional: URL to audio pronunciation
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
} 