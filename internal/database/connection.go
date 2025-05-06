package database

import (
	"fmt"
	"log"
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
			notification_enabled BOOLEAN DEFAULT true,
			notification_hour INTEGER DEFAULT 9,
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
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create topics table: %v", err)
	}

	// Create repetitions table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS repetitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			topic_id INTEGER NOT NULL,
			repetition_number INTEGER NOT NULL,
			completed BOOLEAN DEFAULT false,
			next_review_date TIMESTAMP NOT NULL,
			last_review_date TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (topic_id) REFERENCES topics(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create repetitions table: %v", err)
	}

	// Create statistics table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS statistics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			topic_id INTEGER NOT NULL,
			total_repetitions INTEGER DEFAULT 0,
			completed_repetitions INTEGER DEFAULT 0,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (topic_id) REFERENCES topics(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create statistics table: %v", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}

// GetDB returns the database connection
func GetDB() *sqlx.DB {
	return DB
}