package database

import (
	"fmt"
	"strings"

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

// CreateTopic creates a new topic
func CreateTopic(name string) (*models.Topic, error) {
	// Используем совместимый с SQLite и PostgreSQL запрос
	var query string
	var queryGet string
	
	if DB.DriverName() == "postgres" {
		query = "INSERT INTO topics (name) VALUES ($1) ON CONFLICT (name) DO NOTHING RETURNING id"
		queryGet = "SELECT id FROM topics WHERE name = $1"
	} else {
		query = "INSERT OR IGNORE INTO topics (name) VALUES (?)"
		queryGet = "SELECT id FROM topics WHERE name = ?"
	}
	
	topic := &models.Topic{Name: name}
	
	// PostgreSQL может вернуть ID сразу
	if DB.DriverName() == "postgres" {
		err := DB.QueryRow(query, name).Scan(&topic.ID)
		if err != nil {
			// Если запись уже существует, получаем ID
			err = DB.Get(topic, queryGet, name)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// SQLite - сначала вставляем, потом получаем ID
		result, err := DB.Exec(query, name)
		if err != nil {
			return nil, err
		}
		
		// Если ничего не вставлено (уже существует), получаем ID
		affected, _ := result.RowsAffected()
		if affected == 0 {
			err = DB.Get(topic, queryGet, name)
			if err != nil {
				return nil, err
			}
		} else {
			// Получаем ID вставленной записи
			id, err := result.LastInsertId()
			if err != nil {
				return nil, err
			}
			// ID уже имеет тип int64, который соответствует типу поля ID в структуре Topic
			topic.ID = id
		}
	}
	
	return topic, nil
}

// GetAll returns all topics
func (r *TopicRepository) GetAll() ([]models.Topic, error) {
	var topics []models.Topic
	err := DB.Select(&topics, "SELECT * FROM topics ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("failed to get topics: %v", err)
	}
	return topics, nil
}

// GetByID returns a topic by ID
func (r *TopicRepository) GetByID(id int) (*models.Topic, error) {
	var topic models.Topic
	err := DB.Get(&topic, "SELECT * FROM topics WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic by ID: %v", err)
	}
	return &topic, nil
}

// Create inserts a new topic
func (r *TopicRepository) Create(topic *models.Topic) error {
	query := `
		INSERT INTO topics (name, description)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`
	return DB.QueryRow(
		query,
		topic.Name,
		topic.Description,
	).Scan(&topic.ID, &topic.CreatedAt, &topic.UpdatedAt)
}

// Update modifies an existing topic
func (r *TopicRepository) Update(topic *models.Topic) error {
	query := `
		UPDATE topics SET 
			name = $1,
			description = $2,
			updated_at = NOW()
		WHERE id = $3
		RETURNING updated_at
	`
	return DB.QueryRow(
		query,
		topic.Name,
		topic.Description,
		topic.ID,
	).Scan(&topic.UpdatedAt)
}

// Delete removes a topic
func (r *TopicRepository) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM topics WHERE id = $1", id)
	return err
} 