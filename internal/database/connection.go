package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// Константы для настроек базы данных по умолчанию
const (
	DefaultDBType = "sqlite"
	DefaultDBPath = "./data/engbot.db"
)

// DB is the global database connection
var DB *sqlx.DB

// Connect establishes a connection to the database
func Connect() error {
	// Используем стандартные значения с возможностью переопределения через переменные окружения
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = DefaultDBPath
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %v", err)
	}

	// Open database connection
	var err error
	DB, err = sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Enable foreign keys
	_, err = DB.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return fmt.Errorf("failed to enable foreign keys: %v", err)
	}

	// Initialize schema
	return initializeSQLiteSchema()
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// initializeSQLiteSchema creates necessary tables if they don't exist
func initializeSQLiteSchema() error {
	// Create users table
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			telegram_id INTEGER UNIQUE NOT NULL,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			language_code TEXT,
			is_premium BOOLEAN DEFAULT FALSE,
			is_admin BOOLEAN DEFAULT FALSE,
			preferred_topics TEXT, -- JSON array of topic IDs
			notification_enabled BOOLEAN DEFAULT TRUE,
			notification_hour INTEGER DEFAULT 8, -- Hour for daily notifications (0-23)
			words_per_day INTEGER DEFAULT 5, -- Number of words per day
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %v", err)
	}

	// Create topics table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS topics (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create topics table: %v", err)
	}

	// Create words table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS words (
			id INTEGER PRIMARY KEY,
			topic_id INTEGER,
			word TEXT NOT NULL,
			translation TEXT NOT NULL,
			description TEXT,
			pronunciation TEXT,
			examples TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (topic_id) REFERENCES topics(id) ON DELETE SET NULL,
			UNIQUE(word, topic_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create words table: %v", err)
	}

	// Create user_progress table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS user_progress (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			word_id INTEGER NOT NULL,
			last_review_date TIMESTAMP,
			next_review_date TIMESTAMP,
			interval INTEGER DEFAULT 0,
			easiness_factor REAL DEFAULT 2.5,
			repetitions INTEGER DEFAULT 0,
			last_quality INTEGER DEFAULT 0,
			consecutive_right INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (word_id) REFERENCES words(id) ON DELETE CASCADE,
			UNIQUE(user_id, word_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_progress table: %v", err)
	}

	// Create test_results table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS test_results (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			topic_id INTEGER,
			score INTEGER NOT NULL,
			max_score INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (topic_id) REFERENCES topics(id) ON DELETE SET NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create test_results table: %v", err)
	}

	return nil
}