package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/example/engbot/pkg/models"
)

// TopicRepository handles database operations for topics
type TopicRepository struct{}

// NewTopicRepository creates a new repository instance
func NewTopicRepository() *TopicRepository {
	return &TopicRepository{}
}

// GetAllTopics retrieves all topics from the database
func GetAllTopics() ([]*models.Topic, error) {
	topics := []*models.Topic{}
	
	// Используем совместимый с SQLite и PostgreSQL запрос
	query := "SELECT id, name FROM topics ORDER BY name"
	
	err := DB.Select(&topics, query)
	if err != nil {
		return nil, err
	}
	
	return topics, nil
}

// GetTopicByID retrieves a topic by its ID
func GetTopicByID(topicID int64) (*models.Topic, error) {
	topic := &models.Topic{}
	
	// Используем совместимый с SQLite и PostgreSQL запрос
	query := "SELECT id, name FROM topics WHERE id = ?"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	err := DB.Get(topic, query, topicID)
	if err != nil {
		return nil, err
	}
	
	return topic, nil
}

// GetTopicByName retrieves a topic by its name
func GetTopicByName(name string) (*models.Topic, error) {
	topic := &models.Topic{}
	
	// Используем совместимый с SQLite и PostgreSQL запрос
	query := "SELECT id, name FROM topics WHERE name = ?"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	err := DB.Get(topic, query, name)
	if err != nil {
		return nil, err
	}
	
	return topic, nil
}

// GetAll returns all topics
func (r *TopicRepository) GetAll() ([]models.Topic, error) {
	var topics []struct {
		ID        int64     `db:"id"`
		Name      string    `db:"name"`
		CreatedAt time.Time `db:"created_at"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	
	err := DB.Select(&topics, "SELECT id, name, created_at, updated_at FROM topics ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %v", err)
	}
	
	result := make([]models.Topic, len(topics))
	for i, t := range topics {
		result[i].ID = t.ID
		result[i].Name = t.Name
		result[i].CreatedAt = t.CreatedAt
		result[i].UpdatedAt = t.UpdatedAt
	}
	
	return result, nil
}

// GetAllByUserID returns all topics for a given user
func (r *TopicRepository) GetAllByUserID(ctx context.Context, userID int64) ([]models.Topic, error) {
	var topics []models.Topic

	query := `
		SELECT id, user_id, name, created_at, updated_at
		FROM topics
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	err := DB.SelectContext(ctx, &topics, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %w", err)
	}

	return topics, nil
}

// GetByID returns a topic by ID
func (r *TopicRepository) GetByID(ctx context.Context, userID, topicID int64) (*models.Topic, error) {
	var topic models.Topic
	query := `
		SELECT id, user_id, name, created_at, updated_at
		FROM topics
		WHERE id = ? AND user_id = ?
	`
	err := DB.GetContext(ctx, &topic, query, topicID, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get topic: %w", err)
	}
	return &topic, nil
}

// Create creates a new topic
func (r *TopicRepository) Create(ctx context.Context, topic *models.Topic) error {
	query := `
		INSERT INTO topics (user_id, name, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`

	result, err := DB.ExecContext(ctx, query,
		topic.UserID,
		topic.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	topic.ID = id
	topic.CreatedAt = time.Now()
	topic.UpdatedAt = time.Now()

	return nil
}

// Update updates an existing topic
func (r *TopicRepository) Update(ctx context.Context, topic *models.Topic) error {
	query := `
		UPDATE topics
		SET name = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND user_id = ?
	`

	result, err := DB.ExecContext(ctx, query,
		topic.Name,
		topic.ID,
		topic.UserID,
	)

	if err != nil {
		return fmt.Errorf("failed to update topic: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("topic not found or user not authorized")
	}

	return nil
}

// Delete removes a topic
func (r *TopicRepository) Delete(ctx context.Context, userID, topicID int64) error {
	tx, err := DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	// Delete related repetitions
	_, err = tx.ExecContext(ctx, "DELETE FROM repetitions WHERE user_id = ? AND topic_id = ?", userID, topicID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete repetitions: %w", err)
	}

	// Delete related statistics
	_, err = tx.ExecContext(ctx, "DELETE FROM statistics WHERE user_id = ? AND topic_id = ?", userID, topicID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete statistics: %w", err)
	}

	// Delete the topic
	result, err := tx.ExecContext(ctx, "DELETE FROM topics WHERE id = ? AND user_id = ?", topicID, userID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete topic: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		tx.Rollback()
		return fmt.Errorf("topic not found or user doesn't have permission")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetGeneralTopic returns the general topic
func (r *TopicRepository) GetGeneralTopic() (*models.Topic, error) {
	topic := &models.Topic{
		Name:      "General",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return topic, nil
} 