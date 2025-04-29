package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// DB is the global database connection
var DB *sqlx.DB

// Connect establishes a connection to the database
func Connect() error {
	// Create data directory if it doesn't exist
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// Open database connection
	dbPath := filepath.Join(dataDir, "engbot.db")
	db, err := sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Enable foreign keys
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return fmt.Errorf("failed to enable foreign keys: %v", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't support multiple writers
	db.SetMaxIdleConns(1)

	DB = db

	// Initialize schema
	return initializeSchema()
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// initializeSchema creates necessary tables if they don't exist
func initializeSchema() error {
	// Create users table
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER UNIQUE NOT NULL,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			preferred_topics TEXT,
			words_per_day INTEGER DEFAULT 10,
			notification_hour INTEGER DEFAULT 9,
			notification_enabled BOOLEAN DEFAULT true,
			is_admin BOOLEAN DEFAULT false,
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
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create topics table: %v", err)
	}

	// Create words table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS words (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			word TEXT NOT NULL,
			translation TEXT NOT NULL,
			description TEXT,
			examples TEXT,
			topic_id INTEGER NOT NULL,
			difficulty INTEGER DEFAULT 1,
			pronunciation TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (topic_id) REFERENCES topics(id),
			UNIQUE(word, topic_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create words table: %v", err)
	}

	// Create learned_words table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS learned_words (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			word_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			learned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (word_id) REFERENCES words(id),
			FOREIGN KEY (user_id) REFERENCES users(id),
			UNIQUE(word_id, user_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create learned_words table: %v", err)
	}

	// Create user_configs table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS user_configs (
			user_id INTEGER PRIMARY KEY,
			words_per_batch INTEGER DEFAULT 10,
			repetitions INTEGER DEFAULT 5,
			is_active BOOLEAN DEFAULT true,
			last_batch_time TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_configs table: %v", err)
	}

	// Create user_progress table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS user_progress (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			word_id INTEGER NOT NULL,
			easiness_factor REAL DEFAULT 2.5,
			interval INTEGER DEFAULT 1,
			repetitions INTEGER DEFAULT 0,
			last_quality INTEGER DEFAULT 3,
			consecutive_right INTEGER DEFAULT 0,
			is_learned BOOLEAN DEFAULT FALSE,
			last_review_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			next_review_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (word_id) REFERENCES words(id),
			UNIQUE(user_id, word_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_progress table: %v", err)
	}

	return nil
}