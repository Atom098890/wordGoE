package database

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DB is the database connection
var DB *sqlx.DB

// Connect establishes a connection to the database
func Connect() error {
	// SQLite connection
	var err error
	
	// Создаем директорию для БД, если она не существует
	dbDir := "./data"
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for database: %w", err)
		}
	}
	
	// Подключаемся к SQLite
	DB, err = sqlx.Connect("sqlite3", "./data/engbot.db")
	if err != nil {
		return fmt.Errorf("failed to connect to SQLite database: %w", err)
	}
	
	// Включаем foreign keys для SQLite
	_, err = DB.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	
	// Initialize database schema if needed
	if err := initializeSQLiteSchema(); err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}
	
	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// initializeSQLiteSchema создает необходимые таблицы если они не существуют
func initializeSQLiteSchema() error {
	// Create users table
	_, err := DB.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			is_admin BOOLEAN DEFAULT FALSE,
			preferred_topics TEXT, -- Stored as JSON array
			notification_enabled BOOLEAN DEFAULT TRUE,
			notification_hour INTEGER DEFAULT 8,
			words_per_day INTEGER DEFAULT 5,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %w", err)
	}
	
	// Create topics table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS topics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create topics table: %w", err)
	}
	
	// Create words table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS words (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			english_word TEXT NOT NULL,
			translation TEXT NOT NULL,
			context TEXT,
			topic_id INTEGER,
			difficulty INTEGER DEFAULT 3,
			pronunciation TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (topic_id) REFERENCES topics(id),
			UNIQUE(english_word, topic_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create words table: %w", err)
	}
	
	// Create user_progress table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS user_progress (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			word_id INTEGER NOT NULL,
			repetitions INTEGER DEFAULT 0,
			easiness_factor REAL DEFAULT 2.5,
			interval INTEGER DEFAULT 0,
			next_review_date TIMESTAMP,
			last_review_date TIMESTAMP,
			last_quality INTEGER DEFAULT 0,
			consecutive_right INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			FOREIGN KEY (word_id) REFERENCES words(id),
			UNIQUE(user_id, word_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create user_progress table: %w", err)
	}
	
	// Create test_results table
	_, err = DB.Exec(`
		CREATE TABLE IF NOT EXISTS test_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			test_type TEXT NOT NULL,
			total_words INTEGER NOT NULL,
			correct_words INTEGER NOT NULL,
			topics TEXT, -- Stored as JSON array
			test_date TIMESTAMP,
			duration INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create test_results table: %w", err)
	}
	
	return nil
}

// migrateUserProgressTable updates the user_progress table schema if needed
func migrateUserProgressTable() error {
	// Check if next_review_date column exists
	var count int
	err := DB.Get(&count, `SELECT COUNT(*) FROM pragma_table_info('user_progress') WHERE name = 'next_review_date'`)
	if err != nil {
		return fmt.Errorf("failed to check if next_review_date column exists: %w", err)
	}
	
	// If next_review_date doesn't exist but next_review does, rename the column
	if count == 0 {
		// Check if the old column exists
		err = DB.Get(&count, `SELECT COUNT(*) FROM pragma_table_info('user_progress') WHERE name = 'next_review'`)
		if err != nil {
			return fmt.Errorf("failed to check if next_review column exists: %w", err)
		}
		
		if count > 0 {
			// SQLite doesn't support renaming columns directly in older versions
			// We need to create a new table and copy data
			fmt.Println("Migrating user_progress table to new schema...")
			
			// Begin transaction
			tx, err := DB.Beginx()
			if err != nil {
				return fmt.Errorf("failed to begin transaction: %w", err)
			}
			
			// Create new table
			_, err = tx.Exec(`
				CREATE TABLE user_progress_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					user_id INTEGER NOT NULL,
					word_id INTEGER NOT NULL,
					repetitions INTEGER DEFAULT 0,
					easiness_factor REAL DEFAULT 2.5,
					interval INTEGER DEFAULT 0,
					next_review_date TIMESTAMP,
					last_review_date TIMESTAMP,
					last_quality INTEGER DEFAULT 0,
					consecutive_right INTEGER DEFAULT 0,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					FOREIGN KEY (user_id) REFERENCES users(id),
					FOREIGN KEY (word_id) REFERENCES words(id),
					UNIQUE(user_id, word_id)
				)
			`)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create new user_progress table: %w", err)
			}
			
			// Copy data from old table to new table
			_, err = tx.Exec(`
				INSERT INTO user_progress_new (
					id, user_id, word_id, repetitions, easiness_factor, interval,
					next_review_date, last_review_date, last_quality, consecutive_right,
					created_at, updated_at
				)
				SELECT 
					id, user_id, word_id, repetitions, easiness_factor, interval,
					next_review, last_reviewed, 0, 0,
					CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
				FROM user_progress
			`)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to copy data to new user_progress table: %w", err)
			}
			
			// Drop old table
			_, err = tx.Exec(`DROP TABLE user_progress`)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to drop old user_progress table: %w", err)
			}
			
			// Rename new table to old name
			_, err = tx.Exec(`ALTER TABLE user_progress_new RENAME TO user_progress`)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to rename new user_progress table: %w", err)
			}
			
			// Commit transaction
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit transaction: %w", err)
			}
			
			fmt.Println("User_progress table migration completed successfully.")
		} else {
			// Add missing columns if table exists but columns don't
			fmt.Println("Adding missing columns to user_progress table...")
			_, err := DB.Exec(`ALTER TABLE user_progress ADD COLUMN next_review_date TIMESTAMP`)
			if err != nil {
				return fmt.Errorf("failed to add next_review_date column: %w", err)
			}
			
			_, err = DB.Exec(`ALTER TABLE user_progress ADD COLUMN last_review_date TIMESTAMP`)
			if err != nil {
				return fmt.Errorf("failed to add last_review_date column: %w", err)
			}
			
			_, err = DB.Exec(`ALTER TABLE user_progress ADD COLUMN last_quality INTEGER DEFAULT 0`)
			if err != nil {
				return fmt.Errorf("failed to add last_quality column: %w", err)
			}
			
			_, err = DB.Exec(`ALTER TABLE user_progress ADD COLUMN consecutive_right INTEGER DEFAULT 0`)
			if err != nil {
				return fmt.Errorf("failed to add consecutive_right column: %w", err)
			}
			
			_, err = DB.Exec(`ALTER TABLE user_progress ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			if err != nil {
				return fmt.Errorf("failed to add created_at column: %w", err)
			}
			
			_, err = DB.Exec(`ALTER TABLE user_progress ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
			if err != nil {
				return fmt.Errorf("failed to add updated_at column: %w", err)
			}
		}
	}
	
	return nil
} 