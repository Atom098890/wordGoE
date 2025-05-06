package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/example/engbot/pkg/models"
)

// StatisticsRepository handles database operations for statistics
type StatisticsRepository struct{}

// NewStatisticsRepository creates a new repository instance
func NewStatisticsRepository() *StatisticsRepository {
    return &StatisticsRepository{}
}

// GetByUserAndTopic returns statistics for a specific user and topic
func (r *StatisticsRepository) GetByUserAndTopic(ctx context.Context, userID, topicID int64) (*models.Statistics, error) {
    query := `
        SELECT id, user_id, topic_id, total_repetitions, completed_repetitions,
               created_at, updated_at
        FROM statistics
        WHERE user_id = ? AND topic_id = ?
    `
    var stats models.Statistics
    err := DB.GetContext(ctx, &stats, query, userID, topicID)
    if err == sql.ErrNoRows {
        // Если статистики нет, создаем новую
        stats = models.Statistics{
            UserID:  userID,
            TopicID: topicID,
        }
        err = r.Create(ctx, &stats)
        if err != nil {
            return nil, fmt.Errorf("failed to create statistics: %v", err)
        }
        return &stats, nil
    }
    if err != nil {
        return nil, fmt.Errorf("failed to get statistics: %v", err)
    }
    return &stats, nil
}

// Create inserts new statistics
func (r *StatisticsRepository) Create(ctx context.Context, stats *models.Statistics) error {
    query := `
        INSERT INTO statistics (
            user_id, topic_id, total_repetitions, completed_repetitions
        ) VALUES (?, ?, ?, ?)
    `
    result, err := DB.ExecContext(ctx, query,
        stats.UserID,
        stats.TopicID,
        stats.TotalRepetitions,
        stats.CompletedRepetitions,
    )
    if err != nil {
        return fmt.Errorf("failed to create statistics: %v", err)
    }

    id, err := result.LastInsertId()
    if err != nil {
        return fmt.Errorf("failed to get last insert ID: %v", err)
    }
    stats.ID = id

    return nil
}

// Update modifies existing statistics
func (r *StatisticsRepository) Update(ctx context.Context, stats *models.Statistics) error {
    query := `
        UPDATE statistics SET
            total_repetitions = ?,
            completed_repetitions = ?,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = ? AND user_id = ?
    `
    result, err := DB.ExecContext(ctx, query,
        stats.TotalRepetitions,
        stats.CompletedRepetitions,
        stats.ID,
        stats.UserID,
    )
    if err != nil {
        return fmt.Errorf("failed to update statistics: %v", err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %v", err)
    }
    if rows == 0 {
        return fmt.Errorf("statistics not found or user doesn't have permission")
    }

    return nil
}

// GetUserStatistics returns all statistics for a user
func (r *StatisticsRepository) GetUserStatistics(ctx context.Context, userID int64) ([]models.Statistics, error) {
    query := `
        SELECT s.*, t.name as topic_name
        FROM statistics s
        JOIN topics t ON s.topic_id = t.id
        WHERE s.user_id = ?
        ORDER BY s.total_repetitions DESC
    `
    var stats []models.Statistics
    err := DB.SelectContext(ctx, &stats, query, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to get user statistics: %v", err)
    }
    return stats, nil
}

// IncrementRepetitions increments the repetition counters
func (r *StatisticsRepository) IncrementRepetitions(ctx context.Context, userID, topicID int64, completed bool) error {
    stats, err := r.GetByUserAndTopic(ctx, userID, topicID)
    if err != nil {
        return err
    }

    stats.TotalRepetitions++
    if completed {
        stats.CompletedRepetitions++
    }

    return r.Update(ctx, stats)
} 