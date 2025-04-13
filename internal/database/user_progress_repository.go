package database

import (
	"fmt"
	"os"
	"time"

	"github.com/example/engbot/pkg/models"
)

// UserProgressRepository handles database operations for user progress
type UserProgressRepository struct{}

// NewUserProgressRepository creates a new repository instance
func NewUserProgressRepository() *UserProgressRepository {
	return &UserProgressRepository{}
}

// GetByUserAndWord returns progress for a specific user and word
func (r *UserProgressRepository) GetByUserAndWord(userID int64, wordID int) (*models.UserProgress, error) {
	var progress models.UserProgress
	err := DB.Get(&progress, "SELECT * FROM user_progress WHERE user_id = $1 AND word_id = $2", userID, wordID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user progress: %v", err)
	}
	return &progress, nil
}

// GetDueWordsForUser returns words due for review for a specific user
func (r *UserProgressRepository) GetDueWordsForUser(userID int64) ([]models.UserProgress, error) {
	var progress []models.UserProgress
	
	// Определяем тип базы данных
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "postgres" // По умолчанию postgres для совместимости
	}
	
	// Выбираем правильный запрос в зависимости от типа БД
	var query string
	if dbType == "sqlite" {
		query = `
			SELECT * FROM user_progress
			WHERE user_id = $1 AND next_review_date <= datetime('now')
			ORDER BY next_review_date ASC
		`
	} else {
		// Postgres
		query = `
			SELECT * FROM user_progress
			WHERE user_id = $1 AND next_review_date <= NOW()
			ORDER BY next_review_date ASC
		`
	}
	
	err := DB.Select(&progress, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get due words: %v", err)
	}
	return progress, nil
}

// Create inserts a new progress record
func (r *UserProgressRepository) Create(progress *models.UserProgress) error {
	query := `
		INSERT INTO user_progress (
			user_id, word_id, last_review_date, next_review_date, 
			interval, easiness_factor, repetitions, last_quality, consecutive_right
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`
	return DB.QueryRow(
		query,
		progress.UserID,
		progress.WordID,
		progress.LastReviewDate,
		progress.NextReviewDate,
		progress.Interval,
		progress.EasinessFactor,
		progress.Repetitions,
		progress.LastQuality,
		progress.ConsecutiveRight,
	).Scan(&progress.ID, &progress.CreatedAt, &progress.UpdatedAt)
}

// Update modifies an existing progress record
func (r *UserProgressRepository) Update(progress *models.UserProgress) error {
	// Определяем тип базы данных
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "postgres" // По умолчанию postgres для совместимости
	}
	
	var query string
	var err error
	
	if dbType == "sqlite" {
		// SQLite использует CURRENT_TIMESTAMP вместо NOW()
		query = `
			UPDATE user_progress SET 
				last_review_date = $1,
				next_review_date = $2,
				interval = $3,
				easiness_factor = $4,
				repetitions = $5,
				last_quality = $6,
				consecutive_right = $7,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $8
			RETURNING updated_at
		`
		// SQLite не поддерживает RETURNING напрямую, поэтому нужно сделать отдельно запрос
		_, err = DB.Exec(
			`UPDATE user_progress SET 
				last_review_date = $1,
				next_review_date = $2,
				interval = $3,
				easiness_factor = $4,
				repetitions = $5,
				last_quality = $6,
				consecutive_right = $7,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = $8`,
			progress.LastReviewDate,
			progress.NextReviewDate,
			progress.Interval,
			progress.EasinessFactor,
			progress.Repetitions,
			progress.LastQuality,
			progress.ConsecutiveRight,
			progress.ID,
		)
		
		if err != nil {
			return err
		}
		
		// Получаем обновленное значение updated_at
		return DB.QueryRow("SELECT updated_at FROM user_progress WHERE id = $1", progress.ID).Scan(&progress.UpdatedAt)
		
	} else {
		// PostgreSQL использует NOW()
		query = `
			UPDATE user_progress SET 
				last_review_date = $1,
				next_review_date = $2,
				interval = $3,
				easiness_factor = $4,
				repetitions = $5,
				last_quality = $6,
				consecutive_right = $7,
				updated_at = NOW()
			WHERE id = $8
			RETURNING updated_at
		`
		return DB.QueryRow(
			query,
			progress.LastReviewDate,
			progress.NextReviewDate,
			progress.Interval,
			progress.EasinessFactor,
			progress.Repetitions,
			progress.LastQuality,
			progress.ConsecutiveRight,
			progress.ID,
		).Scan(&progress.UpdatedAt)
	}
}

// Delete removes a progress record
func (r *UserProgressRepository) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM user_progress WHERE id = $1", id)
	return err
}

// CreateOrUpdate creates or updates a progress record
func (r *UserProgressRepository) CreateOrUpdate(progress *models.UserProgress) error {
	// Определяем тип базы данных
	dbType := os.Getenv("DB_TYPE")
	if dbType == "" {
		dbType = "postgres" // По умолчанию postgres для совместимости
	}
	
	var query string
	var err error
	
	if dbType == "sqlite" {
		// SQLite не поддерживает одновременно ON CONFLICT и RETURNING
		// Сначала проверяем, существует ли запись
		var existingID int
		err = DB.QueryRow("SELECT id FROM user_progress WHERE user_id = $1 AND word_id = $2", 
			progress.UserID, progress.WordID).Scan(&existingID)
		
		if err == nil {
			// Запись существует, обновляем её
			progress.ID = existingID
			return r.Update(progress)
		}
		
		// Запись не существует, создаем новую
		query = `
			INSERT INTO user_progress (
				user_id, word_id, last_review_date, next_review_date, 
				interval, easiness_factor, repetitions, last_quality, consecutive_right,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`
		
		result, err := DB.Exec(
			query,
			progress.UserID,
			progress.WordID,
			progress.LastReviewDate,
			progress.NextReviewDate,
			progress.Interval,
			progress.EasinessFactor,
			progress.Repetitions,
			progress.LastQuality,
			progress.ConsecutiveRight,
		)
		
		if err != nil {
			return err
		}
		
		// Получаем ID новой записи
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		progress.ID = int(id)
		
		// Получаем created_at и updated_at
		return DB.QueryRow("SELECT created_at, updated_at FROM user_progress WHERE id = $1", 
			progress.ID).Scan(&progress.CreatedAt, &progress.UpdatedAt)
		
	} else {
		// PostgreSQL поддерживает ON CONFLICT и RETURNING
		query = `
			INSERT INTO user_progress (
				user_id, word_id, last_review_date, next_review_date, 
				interval, easiness_factor, repetitions, last_quality, consecutive_right
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (user_id, word_id) DO UPDATE SET
				last_review_date = EXCLUDED.last_review_date,
				next_review_date = EXCLUDED.next_review_date,
				interval = EXCLUDED.interval,
				easiness_factor = EXCLUDED.easiness_factor,
				repetitions = EXCLUDED.repetitions,
				last_quality = EXCLUDED.last_quality,
				consecutive_right = EXCLUDED.consecutive_right,
				updated_at = NOW()
			RETURNING id, created_at, updated_at
		`
		
		return DB.QueryRow(
			query,
			progress.UserID,
			progress.WordID,
			progress.LastReviewDate,
			progress.NextReviewDate,
			progress.Interval,
			progress.EasinessFactor,
			progress.Repetitions,
			progress.LastQuality,
			progress.ConsecutiveRight,
		).Scan(&progress.ID, &progress.CreatedAt, &progress.UpdatedAt)
	}
}

// GetUserStatistics returns statistics about a user's progress
func (r *UserProgressRepository) GetUserStatistics(userID int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get total words in progress
	var totalCount int
	err := DB.Get(&totalCount, "SELECT COUNT(*) FROM user_progress WHERE user_id = $1", userID)
	if err != nil {
		return nil, err
	}
	stats["total_words"] = totalCount
	
	// Get words due today
	var dueToday int
	err = DB.Get(&dueToday, 
		"SELECT COUNT(*) FROM user_progress WHERE user_id = $1 AND next_review_date <= $2", 
		userID, time.Now().AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	stats["due_today"] = dueToday
	
	// Get words mastered (reviewed at least 5 times with high rating)
	var mastered int
	err = DB.Get(&mastered, 
		"SELECT COUNT(*) FROM user_progress WHERE user_id = $1 AND repetitions >= 5 AND last_quality >= 4", 
		userID)
	if err != nil {
		return nil, err
	}
	stats["mastered"] = mastered
	
	// Get average easiness factor
	var avgEF float64
	err = DB.Get(&avgEF, 
		"SELECT COALESCE(AVG(easiness_factor), 2.5) FROM user_progress WHERE user_id = $1", 
		userID)
	if err != nil {
		return nil, err
	}
	stats["avg_easiness_factor"] = avgEF
	
	return stats, nil
}

// GetTopicCompletionStats returns statistics about user's progress for a specific topic
// Returns: total words in topic, mastered words in topic, completion percentage
func (r *UserProgressRepository) GetTopicCompletionStats(userID int64, topicID int64) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get total words in the topic
	var totalWordsInTopic int
	err := DB.Get(&totalWordsInTopic, "SELECT COUNT(*) FROM words WHERE topic_id = $1", topicID)
	if err != nil {
		return nil, err
	}
	stats["total_words"] = totalWordsInTopic
	
	// Get words from topic that user has started learning
	var wordsInProgress int
	err = DB.Get(&wordsInProgress, `
		SELECT COUNT(*) FROM user_progress up
		JOIN words w ON up.word_id = w.id
		WHERE up.user_id = $1 AND w.topic_id = $2
	`, userID, topicID)
	if err != nil {
		return nil, err
	}
	stats["words_in_progress"] = wordsInProgress
	
	// Get words from topic that user has mastered
	var masteredWords int
	err = DB.Get(&masteredWords, `
		SELECT COUNT(*) FROM user_progress up
		JOIN words w ON up.word_id = w.id
		WHERE up.user_id = $1 AND w.topic_id = $2
		AND up.repetitions >= 5 AND up.last_quality >= 4
	`, userID, topicID)
	if err != nil {
		return nil, err
	}
	stats["mastered_words"] = masteredWords
	
	// Calculate completion percentage
	completionPercentage := 0.0
	if totalWordsInTopic > 0 {
		completionPercentage = float64(masteredWords) / float64(totalWordsInTopic) * 100
	}
	stats["completion_percentage"] = completionPercentage
	
	// Get topic name
	var topicName string
	err = DB.Get(&topicName, "SELECT name FROM topics WHERE id = $1", topicID)
	if err != nil {
		return nil, err
	}
	stats["topic_name"] = topicName
	
	return stats, nil
} 