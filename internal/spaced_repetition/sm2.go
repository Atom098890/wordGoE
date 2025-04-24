package spaced_repetition

import (
	"sort"
	"time"

	"github.com/example/engbot/pkg/models"
)

// SM2 implements the SuperMemo-2 algorithm for spaced repetition
type SM2 struct {
	// Пороговое значение "хорошего ответа"
	PassThreshold int
	// Максимальный интервал повторения в днях
	MaxInterval int
	// Начальные интервалы повторения в днях
	InitialIntervals []int
}

// NewSM2 создает новый экземпляр SM2 с настройками по умолчанию
func NewSM2() *SM2 {
	return &SM2{
		PassThreshold:    3, // Ответы 3 и выше считаются успешными
		MaxInterval:      365, // Максимальный интервал - 1 год
		InitialIntervals: []int{0, 1, 2, 3, 7, 10, 15, 20, 30}, // Предустановленные интервалы для первых повторений
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
	now := time.Now()
	progress.LastReviewDate = now.Format(time.RFC3339)
	progress.LastQuality = int(quality)
	
	// Calculate the easiness factor (EF)
	newEF := progress.EasinessFactor + (0.1 - (5.0-float64(quality))*(0.08+(5.0-float64(quality))*0.02))
	
	// Ensure minimum easiness factor
	if newEF < 1.3 {
		newEF = 1.3 // Не опускаем ниже 1.3
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
	nextDate := now.AddDate(0, 0, progress.Interval)
	progress.NextReviewDate = nextDate.Format(time.RFC3339)
}

// GetNextWords returns the next n words due for review for a user
func (sm *SM2) GetNextWords(userProgress []models.UserProgress, limit int) []models.UserProgress {
	// Filter words due for review (next_review_date <= now)
	now := time.Now()
	var dueProgress []models.UserProgress
	
	for _, p := range userProgress {
		// Parse the date and compare with current time
		nextReviewDate, err := time.Parse(time.RFC3339, p.NextReviewDate)
		if err != nil || !nextReviewDate.After(now) {
			dueProgress = append(dueProgress, p)
		}
	}
	
	// Sort due items by priority:
	// 1. Words that have never been reviewed (repetitions = 0)
	// 2. Words with lowest easiness factor (hardest words)
	// 3. Words with earliest next review date
	
	// Sort by priority criteria
	sort.Slice(dueProgress, func(i, j int) bool {
		// First priority: words that have never been reviewed
		if dueProgress[i].Repetitions == 0 && dueProgress[j].Repetitions > 0 {
			return true
		}
		if dueProgress[j].Repetitions == 0 && dueProgress[i].Repetitions > 0 {
			return false
		}
		
		// Second priority: words with lower easiness factor (harder words)
		if dueProgress[i].EasinessFactor < dueProgress[j].EasinessFactor {
			return true
		}
		if dueProgress[i].EasinessFactor > dueProgress[j].EasinessFactor {
			return false
		}
		
		// Third priority: words that are more overdue
		// Parse dates first
		nextReviewDateI, errI := time.Parse(time.RFC3339, dueProgress[i].NextReviewDate)
		nextReviewDateJ, errJ := time.Parse(time.RFC3339, dueProgress[j].NextReviewDate)
		
		// If both dates are valid, compare them
		if errI == nil && errJ == nil {
			return nextReviewDateI.Before(nextReviewDateJ)
		}
		
		// If one date is invalid, prioritize valid dates
		if errI != nil {
			return false
		}
		if errJ != nil {
			return true
		}
		
		// Default: maintain original order
		return i < j
	})
	
	// Return limited number of items
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

// ComputeNextInterval вычисляет следующий интервал повторения на основе ответа
// quality - качество ответа (от 0 до 5)
// repetitions - текущее количество повторений
// currentEF - текущий фактор легкости
// currentInterval - текущий интервал в днях
func (sm2 *SM2) ComputeNextInterval(quality, repetitions int, currentEF float64, currentInterval int) (int, float64, int) {
	// Обновляем фактор легкости
	newEF := currentEF + (0.1 - float64(5-quality)*(0.08+float64(5-quality)*0.02))
	if newEF < 1.3 {
		newEF = 1.3 // Не опускаем ниже 1.3
	}
	
	var newInterval int
	var newRepetitions int
	
	if quality >= sm2.PassThreshold {
		// Ответ был правильным
		newRepetitions = repetitions + 1
		
		if newRepetitions < len(sm2.InitialIntervals) {
			// Используем предустановленные интервалы для начальных повторений
			newInterval = sm2.InitialIntervals[newRepetitions]
		} else {
			// Для последующих повторений используем формулу
			newInterval = int(float64(currentInterval) * newEF)
			if newInterval > sm2.MaxInterval {
				newInterval = sm2.MaxInterval
			}
		}
	} else {
		// Ответ был неправильным - сбрасываем прогресс
		newRepetitions = 0
		newInterval = 1 // Повторение на следующий день
	}
	
	return newInterval, newEF, newRepetitions
}

// CalculateQuality определяет качество ответа на основе затраченного времени и точности
// Это просто пример реализации. В реальности вы можете использовать любую логику.
// accuracy - точность ответа (0.0 - 1.0)
// timeSpent - время ответа в секундах
func (sm2 *SM2) CalculateQuality(accuracy float64, timeSpent float64) int {
	if accuracy == 0 {
		return 0 // Полностью неверный ответ
	}
	
	// Базовое качество на основе точности
	baseQuality := int(accuracy * 5)
	
	// Корректировка на основе времени
	// Тут можно реализовать свою логику
	
	if baseQuality > 5 {
		baseQuality = 5
	}
	
	return baseQuality
} 