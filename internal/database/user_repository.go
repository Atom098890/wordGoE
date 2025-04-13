package database

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/example/engbot/pkg/models"
)

// UserRepository handles database operations for users
type UserRepository struct{}

// NewUserRepository creates a new repository instance
func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// GetByID returns a user by ID
func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	var user models.User
	var preferredTopicsJSON string
	
	query := "SELECT id, username, first_name, last_name, is_admin, preferred_topics, notification_enabled, notification_hour, words_per_day, created_at, updated_at FROM users WHERE id = ?"
	
	// Convert ? placeholders to $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	err := DB.QueryRow(query, id).Scan(
		&user.ID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.IsAdmin,
		&preferredTopicsJSON,
		&user.NotificationEnabled,
		&user.NotificationHour,
		&user.WordsPerDay,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get user by ID: %v", err)
	}
	
	// Parse JSON array of topics
	if preferredTopicsJSON != "" {
		err = json.Unmarshal([]byte(preferredTopicsJSON), &user.PreferredTopics)
		if err != nil {
			return nil, fmt.Errorf("failed to parse preferred topics: %v", err)
		}
	}
	
	return &user, nil
}

// GetAll returns all users
func (r *UserRepository) GetAll() ([]models.User, error) {
	rows, err := DB.Query("SELECT id, username, first_name, last_name, is_admin, preferred_topics, notification_enabled, notification_hour, words_per_day, created_at, updated_at FROM users ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %v", err)
	}
	defer rows.Close()
	
	var users []models.User
	for rows.Next() {
		var user models.User
		var preferredTopicsJSON string
		
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.FirstName,
			&user.LastName,
			&user.IsAdmin,
			&preferredTopicsJSON,
			&user.NotificationEnabled,
			&user.NotificationHour,
			&user.WordsPerDay,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %v", err)
		}
		
		// Parse JSON array of topics
		if preferredTopicsJSON != "" {
			err = json.Unmarshal([]byte(preferredTopicsJSON), &user.PreferredTopics)
			if err != nil {
				return nil, fmt.Errorf("failed to parse preferred topics: %v", err)
			}
		}
		
		users = append(users, user)
	}
	
	return users, nil
}

// Create inserts a new user or updates if exists
func (r *UserRepository) Create(user *models.User) error {
	// Convert preferred topics to JSON
	topicsJSON, err := json.Marshal(user.PreferredTopics)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred topics: %v", err)
	}
	
	var query string
	if DB.DriverName() == "postgres" {
		query = `
			INSERT INTO users (
				id, username, first_name, last_name, is_admin, 
				preferred_topics, notification_enabled, notification_hour, words_per_day
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				username = EXCLUDED.username,
				first_name = EXCLUDED.first_name,
				last_name = EXCLUDED.last_name,
				updated_at = NOW()
			RETURNING created_at, updated_at
		`
	} else {
		// SQLite doesn't support RETURNING, so we need two separate queries
		query = `
			INSERT OR REPLACE INTO users (
				id, username, first_name, last_name, is_admin, 
				preferred_topics, notification_enabled, notification_hour, words_per_day,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`
	}
	
	if DB.DriverName() == "postgres" {
		return DB.QueryRow(
			query,
			user.ID,
			user.Username,
			user.FirstName,
			user.LastName,
			user.IsAdmin,
			topicsJSON,
			user.NotificationEnabled,
			user.NotificationHour,
			user.WordsPerDay,
		).Scan(&user.CreatedAt, &user.UpdatedAt)
	} else {
		// For SQLite
		_, err = DB.Exec(
			query,
			user.ID,
			user.Username,
			user.FirstName,
			user.LastName,
			user.IsAdmin,
			string(topicsJSON),
			user.NotificationEnabled,
			user.NotificationHour,
			user.WordsPerDay,
		)
		
		if err != nil {
			return fmt.Errorf("failed to create/update user: %v", err)
		}
		
		// Get the timestamps in a separate query
		var createdAt, updatedAt string
		err = DB.QueryRow("SELECT created_at, updated_at FROM users WHERE id = ?", user.ID).Scan(&createdAt, &updatedAt)
		if err != nil {
			return fmt.Errorf("failed to get timestamps: %v", err)
		}
		
		return nil
	}
}

// Update modifies user settings
func (r *UserRepository) Update(user *models.User) error {
	// Convert preferred topics to JSON
	topicsJSON, err := json.Marshal(user.PreferredTopics)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred topics: %v", err)
	}
	
	var query string
	if DB.DriverName() == "postgres" {
		query = `
			UPDATE users SET 
				username = $1,
				first_name = $2,
				last_name = $3,
				is_admin = $4,
				preferred_topics = $5,
				notification_enabled = $6,
				notification_hour = $7,
				words_per_day = $8,
				updated_at = NOW()
			WHERE id = $9
			RETURNING updated_at
		`
	} else {
		query = `
			UPDATE users SET 
				username = ?,
				first_name = ?,
				last_name = ?,
				is_admin = ?,
				preferred_topics = ?,
				notification_enabled = ?,
				notification_hour = ?,
				words_per_day = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`
	}
	
	if DB.DriverName() == "postgres" {
		return DB.QueryRow(
			query,
			user.Username,
			user.FirstName,
			user.LastName,
			user.IsAdmin,
			topicsJSON,
			user.NotificationEnabled,
			user.NotificationHour,
			user.WordsPerDay,
			user.ID,
		).Scan(&user.UpdatedAt)
	} else {
		// For SQLite
		_, err = DB.Exec(
			query,
			user.Username,
			user.FirstName,
			user.LastName,
			user.IsAdmin,
			string(topicsJSON),
			user.NotificationEnabled,
			user.NotificationHour,
			user.WordsPerDay,
			user.ID,
		)
		
		if err != nil {
			return fmt.Errorf("failed to update user: %v", err)
		}
		
		return nil
	}
}

// Delete removes a user
func (r *UserRepository) Delete(id int64) error {
	query := "DELETE FROM users WHERE id = ?"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
	}
	
	_, err := DB.Exec(query, id)
	return err
}

// UpdatePreferredTopics updates just the preferred topics field
func (r *UserRepository) UpdatePreferredTopics(userID int64, topicIDs []int64) error {
	// Convert preferred topics to JSON
	topicsJSON, err := json.Marshal(topicIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal preferred topics: %v", err)
	}
	
	query := "UPDATE users SET preferred_topics = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		query = strings.Replace(query, "?", "$1", -1)
		query = strings.Replace(query, "?", "$2", -1)
	}
	
	_, err = DB.Exec(query, string(topicsJSON), userID)
	return err
}

// GetAdminUsers returns all admin users
func (r *UserRepository) GetAdminUsers() ([]models.User, error) {
	return r.getUsersWithCondition("is_admin = 1")
}

// GetUsersForNotification returns users who have notifications enabled and are due for them
func (r *UserRepository) GetUsersForNotification(hour int) ([]models.User, error) {
	condition := "notification_enabled = 1 AND notification_hour = ?"
	
	// Replace ? with $ for PostgreSQL if needed
	if DB.DriverName() == "postgres" {
		condition = strings.Replace(condition, "?", "$1", -1)
	}
	
	return r.getUsersWithCondition(condition, hour)
}

// getUsersWithCondition is a helper function to get users with a specific condition
func (r *UserRepository) getUsersWithCondition(condition string, args ...interface{}) ([]models.User, error) {
	query := "SELECT id, username, first_name, last_name, is_admin, preferred_topics, notification_enabled, notification_hour, words_per_day, created_at, updated_at FROM users WHERE " + condition
	
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users with condition: %v", err)
	}
	defer rows.Close()
	
	var users []models.User
	for rows.Next() {
		var user models.User
		var preferredTopicsJSON string
		
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.FirstName,
			&user.LastName,
			&user.IsAdmin,
			&preferredTopicsJSON,
			&user.NotificationEnabled,
			&user.NotificationHour,
			&user.WordsPerDay,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %v", err)
		}
		
		// Parse JSON array of topics
		if preferredTopicsJSON != "" {
			err = json.Unmarshal([]byte(preferredTopicsJSON), &user.PreferredTopics)
			if err != nil {
				return nil, fmt.Errorf("failed to parse preferred topics: %v", err)
			}
		}
		
		users = append(users, user)
	}
	
	return users, nil
} 