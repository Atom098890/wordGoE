package database

import (
	"context"
	"fmt"
	"time"

	"github.com/example/engbot/pkg/models"
)

// RepetitionRepository handles database operations for repetitions
type RepetitionRepository struct{}

// NewRepetitionRepository creates a new repository instance
func NewRepetitionRepository() *RepetitionRepository {
    return &RepetitionRepository{}
}

// Create inserts a new repetition
func (r *RepetitionRepository) Create(ctx context.Context, rep *models.Repetition) error {
    query := `
        INSERT INTO repetitions (
            user_id, topic_id, repetition_number,
            next_review_date, last_review_date, completed
        ) VALUES (?, ?, ?, ?, ?, ?)
    `
    result, err := DB.ExecContext(ctx, query,
        rep.UserID,
        rep.TopicID,
        rep.RepetitionNumber,
        rep.NextReviewDate,
        rep.LastReviewDate,
        rep.Completed,
    )
    if err != nil {
        return fmt.Errorf("failed to create repetition: %v", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return fmt.Errorf("failed to get last insert ID: %v", err)
    }
    rep.ID = id

    return nil
}

// Update modifies an existing repetition
func (r *RepetitionRepository) Update(ctx context.Context, rep *models.Repetition) error {
    query := `
        UPDATE repetitions SET
            repetition_number = ?,
            next_review_date = ?,
            last_review_date = ?,
            completed = ?,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = ? AND user_id = ?
    `
    result, err := DB.ExecContext(ctx, query,
        rep.RepetitionNumber,
        rep.NextReviewDate,
        rep.LastReviewDate,
        rep.Completed,
        rep.ID,
        rep.UserID,
    )
    if err != nil {
        return fmt.Errorf("failed to update repetition: %v", err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %v", err)
    }
    if rows == 0 {
        return fmt.Errorf("repetition not found or user doesn't have permission")
    }

    return nil
}

// GetDueRepetitions returns all repetitions that are due for review
func (r *RepetitionRepository) GetDueRepetitions(ctx context.Context, userID int64) ([]models.Repetition, error) {
    query := `
        SELECT r.*, t.name as topic_name
        FROM repetitions r
        JOIN topics t ON r.topic_id = t.id
        WHERE r.user_id = ? 
        AND r.next_review_date <= ?
        AND r.completed = false
        ORDER BY r.next_review_date ASC
    `
    var repetitions []models.Repetition
    err := DB.SelectContext(ctx, &repetitions, query, userID, time.Now())
    if err != nil {
        return nil, fmt.Errorf("failed to get due repetitions: %v", err)
    }
    return repetitions, nil
}

// GetByTopicID returns all repetitions for a specific topic
func (r *RepetitionRepository) GetByTopicID(ctx context.Context, userID, topicID int64) ([]models.Repetition, error) {
    query := `
        SELECT *
        FROM repetitions
        WHERE user_id = ? AND topic_id = ?
        ORDER BY created_at DESC
    `
    var repetitions []models.Repetition
    err := DB.SelectContext(ctx, &repetitions, query, userID, topicID)
    if err != nil {
        return nil, fmt.Errorf("failed to get repetitions: %v", err)
    }
    return repetitions, nil
}

// CalculateNextReviewDate calculates the next review date based on the repetition number
func (r *RepetitionRepository) CalculateNextReviewDate(repetitionNumber int) time.Time {
    // Интервалы повторения в днях: 1, 2, 3, 7, 15, 25, 40
    intervals := []int{1, 2, 3, 7, 15, 25, 40}
    
    // Если номер повторения больше количества интервалов, используем последний интервал
    if repetitionNumber >= len(intervals) {
        repetitionNumber = len(intervals) - 1
    }
    
    // Вычисляем следующую дату повторения
    nextDate := time.Now().AddDate(0, 0, intervals[repetitionNumber])
    
    return nextDate
}

// GetAllByUserID returns all repetitions for a user
func (r *RepetitionRepository) GetAllByUserID(ctx context.Context, userID int64) ([]models.Repetition, error) {
    query := `
        SELECT *
        FROM repetitions
        WHERE user_id = ?
        ORDER BY next_review_date ASC
    `
    var repetitions []models.Repetition
    err := DB.SelectContext(ctx, &repetitions, query, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to get repetitions: %w", err)
    }
    return repetitions, nil
} 