package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/example/engbot/pkg/models"
)

// UserRepository handles database operations for users
type UserRepository struct{}

// NewUserRepository creates a new repository instance
func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

// Create inserts a new user
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (
			telegram_id, username, first_name, last_name,
			notification_enabled, notification_hour
		) VALUES (?, ?, ?, ?, ?, ?)
	`
	result, err := DB.ExecContext(ctx, query,
		user.TelegramID,
		user.Username,
		user.FirstName,
		user.LastName,
		user.NotificationEnabled,
		user.NotificationHour,
	)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %v", err)
	}
	user.ID = id

	return nil
}

// Update modifies an existing user
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users SET
			username = ?,
			first_name = ?,
			last_name = ?,
			notification_enabled = ?,
			notification_hour = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := DB.ExecContext(ctx, query,
		user.Username,
		user.FirstName,
		user.LastName,
		user.NotificationEnabled,
		user.NotificationHour,
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %v", err)
	}
	return nil
}

// GetUsersForNotification returns all users who should receive notifications at the current hour
func (r *UserRepository) GetUsersForNotification(ctx context.Context, hour int) ([]models.User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, last_name,
			   notification_enabled, notification_hour, created_at, updated_at
		FROM users
		WHERE notification_enabled = true AND notification_hour = ?
	`
	var users []models.User
	err := DB.SelectContext(ctx, &users, query, hour)
	if err != nil {
		return nil, fmt.Errorf("failed to get users for notification: %v", err)
	}
	return users, nil
}

// GetAdminUsers returns all admin users
func (r *UserRepository) GetAdminUsers(ctx context.Context) ([]models.User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, last_name,
			   notification_enabled, notification_hour, created_at, updated_at
		FROM users
		WHERE is_admin = true
	`
	var users []models.User
	err := DB.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get admin users: %v", err)
	}
	return users, nil
}

// UserStats represents user's learning statistics
type UserStats struct {
	TotalWords      int
	LearnedToday    int
	LearningStreak  int
	TotalLearned    int
}

// GetUserStats retrieves user's learning statistics
func (r *UserRepository) GetUserStats(ctx context.Context, userID int64) (*UserStats, error) {
	stats := &UserStats{}

	// Get total learned words
	err := DB.QueryRowContext(ctx, `
		SELECT COUNT(*) 
		FROM learned_words lw
		JOIN users u ON u.id = lw.user_id
		WHERE u.telegram_id = ?
	`, userID).Scan(&stats.TotalLearned)
	if err != nil {
		return nil, fmt.Errorf("failed to get total learned words: %v", err)
	}

	// Get words learned today
	err = DB.QueryRowContext(ctx, `
		SELECT COUNT(*) 
		FROM learned_words lw
		JOIN users u ON u.id = lw.user_id
		WHERE u.telegram_id = ? 
		AND date(lw.learned_at) = date('now')
	`, userID).Scan(&stats.LearnedToday)
	if err != nil {
		return nil, fmt.Errorf("failed to get today's learned words: %v", err)
	}

	// Get learning streak (consecutive days with learned words)
	err = DB.QueryRowContext(ctx, `
		WITH RECURSIVE dates(date) AS (
			SELECT date('now', '-30 days')
			UNION ALL
			SELECT date(date, '+1 day')
			FROM dates
			WHERE date < date('now')
		),
		daily_progress AS (
			SELECT date(lw.learned_at) as learn_date, COUNT(*) as words_count
			FROM learned_words lw
			JOIN users u ON u.id = lw.user_id
			WHERE u.telegram_id = ?
			GROUP BY date(lw.learned_at)
		)
		SELECT COUNT(*) as streak
		FROM (
			SELECT dates.date
			FROM dates
			LEFT JOIN daily_progress ON dates.date = daily_progress.learn_date
			WHERE daily_progress.words_count > 0
			ORDER BY dates.date DESC
		) as consecutive_days
		WHERE consecutive_days.date = date('now', '-' || (rowid - 1) || ' days')
	`, userID).Scan(&stats.LearningStreak)
	if err != nil {
		// If error, just set streak to 0 instead of failing
		stats.LearningStreak = 0
	}

	// Get total available words
	err = DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM words
	`).Scan(&stats.TotalWords)
	if err != nil {
		return nil, fmt.Errorf("failed to get total words: %v", err)
	}

	return stats, nil
}

// GetByTelegramID returns a user by Telegram ID
func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	query := `
		SELECT id, telegram_id, username, first_name, last_name, 
			   notification_enabled, notification_hour, created_at, updated_at
		FROM users 
		WHERE telegram_id = ?
	`
	
	user := &models.User{}
	err := DB.GetContext(ctx, user, query, telegramID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %v", err)
	}
	
	// Verify that we got a valid user
	if user.ID == 0 {
		return nil, nil
	}
	
	return user, nil
} 