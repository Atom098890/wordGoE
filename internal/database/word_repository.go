package database

import (
	"fmt"
	"strings"

	"github.com/example/engbot/pkg/models"
)

// WordRepository handles database operations for words
type WordRepository struct{}

// NewWordRepository creates a new repository instance
func NewWordRepository() *WordRepository {
	return &WordRepository{}
}

// GetAll returns all words
func (r *WordRepository) GetAll() ([]models.Word, error) {
	var words []models.Word
	err := DB.Select(&words, "SELECT * FROM words ORDER BY english_word")
	if err != nil {
		return nil, fmt.Errorf("failed to get words: %v", err)
	}
	return words, nil
}

// GetByID returns a word by ID
func (r *WordRepository) GetByID(id int) (*models.Word, error) {
	var word models.Word
	err := DB.Get(&word, "SELECT * FROM words WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get word by ID: %v", err)
	}
	return &word, nil
}

// GetByTopic returns words for a specific topic
func (r *WordRepository) GetByTopic(topicID int64) ([]models.Word, error) {
	var words []models.Word
	
	query := "SELECT * FROM words WHERE topic_id = ? ORDER BY english_word"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	err := DB.Select(&words, query, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to get words by topic: %v", err)
	}
	return words, nil
}

// Create inserts a new word
func (r *WordRepository) Create(word *models.Word) error {
	// Разные запросы для разных СУБД
	var query string
	
	if DB.DriverName() == "postgres" {
		query = `
			INSERT INTO words (english_word, translation, description, topic_id, difficulty, pronunciation, examples)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at, updated_at
		`
		return DB.QueryRow(
			query,
			word.Word,
			word.Translation,
			word.Context,
			word.TopicID,
			word.Difficulty,
			word.Pronunciation,
			word.Examples,
		).Scan(&word.ID, &word.CreatedAt, &word.UpdatedAt)
	} else {
		// Для SQLite (без RETURNING)
		query = `
			INSERT INTO words (word, translation, description, topic_id, difficulty, pronunciation, examples, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`
		result, err := DB.Exec(
			query,
			word.Word,
			word.Translation,
			word.Context,
			word.TopicID,
			word.Difficulty,
			word.Pronunciation,
			word.Examples,
		)
		if err != nil {
			return fmt.Errorf("failed to create word: %v", err)
		}
		
		// Получаем ID последней вставленной записи
		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert ID: %v", err)
		}
		word.ID = int(id)
		
		// Получаем временные метки
		var createdAt, updatedAt string
		err = DB.QueryRow("SELECT created_at, updated_at FROM words WHERE id = ?", word.ID).Scan(&createdAt, &updatedAt)
		if err != nil {
			return fmt.Errorf("failed to get timestamps: %v", err)
		}
		
		return nil
	}
}

// Update modifies an existing word
func (r *WordRepository) Update(word *models.Word) error {
	// Разные запросы для разных СУБД
	var query string
	
	if DB.DriverName() == "postgres" {
		query = `
			UPDATE words SET 
				english_word = $1,
				translation = $2,
				description = $3,
				topic_id = $4,
				difficulty = $5,
				pronunciation = $6,
				examples = $7,
				updated_at = NOW()
			WHERE id = $8
			RETURNING updated_at
		`
		return DB.QueryRow(
			query,
			word.Word,
			word.Translation,
			word.Context,
			word.TopicID,
			word.Difficulty,
			word.Pronunciation,
			word.Examples,
			word.ID,
		).Scan(&word.UpdatedAt)
	} else {
		// Для SQLite (без RETURNING)
		query = `
			UPDATE words SET 
				word = ?,
				translation = ?,
				description = ?,
				topic_id = ?,
				difficulty = ?,
				pronunciation = ?,
				examples = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
		_, err := DB.Exec(
			query,
			word.Word,
			word.Translation,
			word.Context,
			word.TopicID,
			word.Difficulty,
			word.Pronunciation,
			word.Examples,
			word.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update word: %v", err)
		}
		
		// Получаем временную метку обновления
		var updatedAt string
		err = DB.QueryRow("SELECT updated_at FROM words WHERE id = ?", word.ID).Scan(&updatedAt)
		if err != nil {
			return fmt.Errorf("failed to get updated_at: %v", err)
		}
		
		return nil
	}
}

// Delete removes a word
func (r *WordRepository) Delete(id int) error {
	var query string
	
	if DB.DriverName() == "postgres" {
		query = "DELETE FROM words WHERE id = $1"
	} else {
		query = "DELETE FROM words WHERE id = ?"
	}
	
	_, err := DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete word: %v", err)
	}
	
	return nil
}

// SearchWords searches for words by pattern matching
func (r *WordRepository) SearchWords(query string) ([]models.Word, error) {
	var words []models.Word
	var sqlQuery string
	pattern := "%" + query + "%"
	
	if DB.DriverName() == "postgres" {
		sqlQuery = `
			SELECT * FROM words 
			WHERE english_word ILIKE $1 OR translation ILIKE $1
			ORDER BY english_word
		`
		err := DB.Select(&words, sqlQuery, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to search words: %v", err)
		}
	} else {
		sqlQuery = `
			SELECT * FROM words 
			WHERE LOWER(word) LIKE LOWER(?) OR LOWER(translation) LIKE LOWER(?)
			ORDER BY word
		`
		err := DB.Select(&words, sqlQuery, pattern, pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to search words: %v", err)
		}
	}
	
	return words, nil
}

// GetRandomWordsByTopic returns random words from a topic, limited by count
func (r *WordRepository) GetRandomWordsByTopic(topicID int64, count int) ([]models.Word, error) {
	var words []models.Word
	
	query := `
		SELECT * FROM words 
		WHERE topic_id = ?
		ORDER BY RANDOM()
		LIMIT ?
	`
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
		query = strings.Replace(query, "?", "$2", -1)
	}
	
	err := DB.Select(&words, query, topicID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get random words: %v", err)
	}
	return words, nil
}

// GetWordByID retrieves a word by its ID
func GetWordByID(wordID int) (*models.Word, error) {
	word := &models.Word{}
	
	// Используем совместимый с SQLite и PostgreSQL запрос
	query := `
		SELECT w.id, w.english_word as english_word, w.translation, w.description, w.difficulty, w.topic_id, t.name as topic
		FROM words w
		JOIN topics t ON w.topic_id = t.id
		WHERE w.id = ?
	`
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	err := DB.QueryRowx(query, wordID).StructScan(word)
	if err != nil {
		return nil, err
	}
	
	return word, nil
} 