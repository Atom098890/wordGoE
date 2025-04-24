package database

import (
	"fmt"

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
	
	query := `
		SELECT * FROM user_progress
		WHERE user_id = $1 AND next_review_date <= datetime('now') AND is_learned = FALSE
		ORDER BY next_review_date ASC
	`
	
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
			interval, easiness_factor, repetitions, last_quality, consecutive_right, is_learned,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
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
		progress.IsLearned,
	)
	
	if err != nil {
		return fmt.Errorf("failed to create progress: %v", err)
	}
	
	// Получаем ID новой записи
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %v", err)
	}
	progress.ID = int(id)
	
	// Получаем created_at и updated_at
	return DB.QueryRow("SELECT created_at, updated_at FROM user_progress WHERE id = $1", 
		progress.ID).Scan(&progress.CreatedAt, &progress.UpdatedAt)
}

// Update modifies an existing progress record
func (r *UserProgressRepository) Update(progress *models.UserProgress) error {
	query := `
		UPDATE user_progress SET 
			last_review_date = $1,
			next_review_date = $2,
			interval = $3,
			easiness_factor = $4,
			repetitions = $5,
			last_quality = $6,
			consecutive_right = $7,
			is_learned = $8,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $9
	`
	
	_, err := DB.Exec(
		query,
		progress.LastReviewDate,
		progress.NextReviewDate,
		progress.Interval,
		progress.EasinessFactor,
		progress.Repetitions,
		progress.LastQuality,
		progress.ConsecutiveRight,
		progress.IsLearned,
		progress.ID,
	)
	
	if err != nil {
		return fmt.Errorf("failed to update progress: %v", err)
	}
	
	// Получаем обновленное значение updated_at
	return DB.QueryRow("SELECT updated_at FROM user_progress WHERE id = $1", 
		progress.ID).Scan(&progress.UpdatedAt)
}

// Delete removes a progress record
func (r *UserProgressRepository) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM user_progress WHERE id = $1", id)
	return err
}

// CreateOrUpdate creates or updates a progress record
func (r *UserProgressRepository) CreateOrUpdate(progress *models.UserProgress) error {
	// Проверяем, существует ли запись
	var existingID int
	err := DB.QueryRow(
		"SELECT id FROM user_progress WHERE user_id = $1 AND word_id = $2", 
		progress.UserID, progress.WordID,
	).Scan(&existingID)
	
	if err == nil {
		// Запись существует, обновляем её
		progress.ID = existingID
		return r.Update(progress)
	}
	
	// Запись не существует, создаем новую
	return r.Create(progress)
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
		"SELECT COUNT(*) FROM user_progress WHERE user_id = $1 AND next_review_date <= datetime('now', '+1 day')", 
		userID)
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

// GetLearnedWords returns all words marked as learned for a specific user
func (r *UserProgressRepository) GetLearnedWords(userID int64) ([]models.Word, error) {
	var words []models.Word
	
	query := `
		SELECT w.*
		FROM words w
		JOIN user_progress up ON w.id = up.word_id
		WHERE up.user_id = $1 AND up.is_learned = TRUE
		ORDER BY w.word
	`
	
	err := DB.Select(&words, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get learned words: %v", err)
	}
	return words, nil
} 