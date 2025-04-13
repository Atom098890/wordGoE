package database

import (
	"fmt"

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
	query := `
		SELECT 
			id, 
			word, 
			translation, 
			topic_id, 
			difficulty, 
			pronunciation, 
			created_at, 
			updated_at 
		FROM words 
		ORDER BY word
	`
	
	err := DB.Select(&words, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get words: %v", err)
	}
	return words, nil
}

// GetByID returns a word by ID
func (r *WordRepository) GetByID(id int) (*models.Word, error) {
	var word models.Word
	query := `
		SELECT 
			id, 
			word, 
			translation, 
			topic_id, 
			difficulty, 
			pronunciation, 
			created_at, 
			updated_at 
		FROM words 
		WHERE id = ?
	`
	
	err := DB.Get(&word, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get word by ID: %v", err)
	}
	return &word, nil
}

// GetByTopic returns words for a specific topic
func (r *WordRepository) GetByTopic(topicID int64) ([]models.Word, error) {
	var words []models.Word
	query := `
		SELECT 
			id, 
			word, 
			translation, 
			topic_id, 
			difficulty, 
			pronunciation, 
			created_at, 
			updated_at 
		FROM words 
		WHERE topic_id = ? 
		ORDER BY word
	`
	
	err := DB.Select(&words, query, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to get words by topic: %v", err)
	}
	return words, nil
}

// Create inserts a new word
func (r *WordRepository) Create(word *models.Word) error {
	query := `
		INSERT INTO words (word, translation, topic_id, difficulty, pronunciation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	result, err := DB.Exec(
		query,
		word.Word,
		word.Translation,
		word.TopicID,
		word.Difficulty,
		word.Pronunciation,
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

// Update modifies an existing word
func (r *WordRepository) Update(word *models.Word) error {
	query := `
		UPDATE words SET 
			word = ?,
			translation = ?,
			topic_id = ?,
			difficulty = ?,
			pronunciation = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := DB.Exec(
		query,
		word.Word,
		word.Translation,
		word.TopicID,
		word.Difficulty,
		word.Pronunciation,
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

// Delete removes a word
func (r *WordRepository) Delete(id int) error {
	query := "DELETE FROM words WHERE id = ?"
	
	_, err := DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete word: %v", err)
	}
	
	return nil
}

// SearchWords searches for words by pattern matching
func (r *WordRepository) SearchWords(query string) ([]models.Word, error) {
	var words []models.Word
	sqlQuery := `
		SELECT 
			id, 
			word, 
			translation, 
			topic_id, 
			difficulty, 
			pronunciation, 
			created_at, 
			updated_at 
		FROM words 
		WHERE LOWER(word) LIKE LOWER(?) OR LOWER(translation) LIKE LOWER(?)
		ORDER BY word
	`
	pattern := "%" + query + "%"
	
	err := DB.Select(&words, sqlQuery, pattern, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search words: %v", err)
	}
	
	return words, nil
}

// GetRandomWordsByTopic returns random words from a topic, limited by count
func (r *WordRepository) GetRandomWordsByTopic(topicID int64, count int) ([]models.Word, error) {
	var words []models.Word
	query := `
		SELECT 
			id, 
			word, 
			translation, 
			topic_id, 
			difficulty, 
			pronunciation, 
			created_at, 
			updated_at
		FROM words 
		WHERE topic_id = ?
		ORDER BY RANDOM()
		LIMIT ?
	`
	
	err := DB.Select(&words, query, topicID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get random words: %v", err)
	}
	return words, nil
}

// GetWordByID retrieves a word by its ID
func GetWordByID(wordID int) (*models.Word, error) {
	word := &models.Word{}
	query := `
		SELECT 
			w.id, 
			w.word, 
			w.translation, 
			w.difficulty, 
			w.topic_id, 
			w.pronunciation,
			w.created_at,
			w.updated_at,
			t.name as topic
		FROM words w
		JOIN topics t ON w.topic_id = t.id
		WHERE w.id = ?
	`
	
	err := DB.QueryRowx(query, wordID).StructScan(word)
	if err != nil {
		return nil, err
	}
	
	return word, nil
} 