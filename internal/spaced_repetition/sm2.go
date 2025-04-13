package spaced_repetition

import (
	"time"

	"github.com/example/engbot/pkg/models"
)

// SM2 implements the SuperMemo 2 algorithm for spaced repetition
// This is a modified version of the original algorithm
type SM2 struct {
	// MinEasinessFactor is the minimum value for the easiness factor
	MinEasinessFactor float64
	// MaxInterval is the maximum interval between repetitions (in days)
	MaxInterval int
	// Initial interval values for each repetition
	InitialIntervals []int
}

// NewSM2 creates a new SM2 algorithm instance with default parameters
func NewSM2() *SM2 {
	return &SM2{
		MinEasinessFactor: 1.3,
		MaxInterval:       365, // Max interval of 1 year
		InitialIntervals:  []int{1, 3, 7, 10, 15, 30}, // 1, 3, 7, 10, 15, 30 дней для первых 6 повторений
	}
}

// QualityResponse represents the quality of response in SM-2
type QualityResponse int

const (
	// Complete blackout, unable to recall
	QualityBlackout QualityResponse = 0
	// Incorrect response but remembered upon seeing the correct answer
	QualityIncorrect QualityResponse = 1
	// Incorrect response but the correct answer felt familiar
	QualityIncorrectFamiliar QualityResponse = 2
	// Correct response but required significant effort
	QualityCorrectDifficult QualityResponse = 3
	// Correct response after some hesitation
	QualityCorrectHesitation QualityResponse = 4
	// Perfect response with no hesitation
	QualityPerfect QualityResponse = 5
)

// Process implements the SM-2 algorithm to update user progress
func (sm *SM2) Process(progress *models.UserProgress, quality QualityResponse) {
	// Record the last review date
	progress.LastReviewDate = time.Now()
	progress.LastQuality = int(quality)
	
	// Calculate the easiness factor (EF)
	newEF := progress.EasinessFactor + (0.1 - (5.0-float64(quality))*(0.08+(5.0-float64(quality))*0.02))
	
	// Ensure minimum easiness factor
	if newEF < sm.MinEasinessFactor {
		newEF = sm.MinEasinessFactor
	}
	progress.EasinessFactor = newEF
	
	// Handle correct/incorrect response
	if quality >= QualityCorrectDifficult {
		// Correct response
		progress.ConsecutiveRight++
		
		// Calculate next interval
		var nextInterval int
		
		if progress.Repetitions == 0 {
			// First correct review, use first initial interval
			nextInterval = sm.InitialIntervals[0]
		} else if progress.Repetitions < len(sm.InitialIntervals) {
			// Use predefined intervals for early repetitions
			nextInterval = sm.InitialIntervals[progress.Repetitions]
		} else {
			// Calculate based on the formula
			nextInterval = int(float64(progress.Interval) * progress.EasinessFactor)
		}
		
		// Ensure maximum interval
		if nextInterval > sm.MaxInterval {
			nextInterval = sm.MaxInterval
		}
		
		progress.Interval = nextInterval
		progress.Repetitions++
	} else {
		// Incorrect response - reset interval and consecutive right counter
		progress.ConsecutiveRight = 0
		progress.Interval = 1
		// We don't reset repetitions count, as it's useful for analytics
	}
	
	// Set the next review date
	progress.NextReviewDate = progress.LastReviewDate.AddDate(0, 0, progress.Interval)
}

// GetNextWords returns the next n words due for review for a user
func (sm *SM2) GetNextWords(userProgress []models.UserProgress, limit int) []models.UserProgress {
	// Filter words due for review (next_review_date <= now)
	now := time.Now()
	var dueProgress []models.UserProgress
	
	for _, p := range userProgress {
		if !p.NextReviewDate.After(now) {
			dueProgress = append(dueProgress, p)
		}
	}
	
	// Sort them by priority:
	// 1. Words that have never been reviewed
	// 2. Words with lowest easiness factor (hardest words)
	// 3. Words with earliest next review date
	
	// Simple implementation that just takes the first n items
	// In a real implementation, you would want to sort them properly
	if len(dueProgress) > limit {
		return dueProgress[:limit]
	}
	
	return dueProgress
}

// IsWordMastered determines if a word is considered "mastered"
func (sm *SM2) IsWordMastered(progress *models.UserProgress) bool {
	// A word is considered mastered if:
	// 1. It has been reviewed at least 5 times
	// 2. The latest quality response was 4 or 5
	// 3. The interval is at least 30 days
	return progress.Repetitions >= 5 && 
		   progress.LastQuality >= int(QualityCorrectHesitation) && 
		   progress.Interval >= 30
} 