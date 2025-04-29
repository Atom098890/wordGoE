package database

import (
	"context"
	"database/sql"
	"time"
)

// GetUserConfig retrieves user configuration
func GetUserConfig(ctx context.Context, userID int64) (*UserConfig, error) {
	query := `
		SELECT user_id, words_per_batch, repetitions, is_active, last_batch_time, created_at, updated_at
		FROM user_configs
		WHERE user_id = ?
	`

	config := &UserConfig{}
	err := DB.QueryRowContext(ctx, query, userID).Scan(
		&config.UserID,
		&config.WordsPerBatch,
		&config.Repetitions,
		&config.IsActive,
		&config.LastBatchTime,
		&config.CreatedAt,
		&config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return config, nil
}

// UpdateUserConfig updates user configuration
func UpdateUserConfig(ctx context.Context, config *UserConfig) error {
	query := `
		UPDATE user_configs
		SET words_per_batch = ?, repetitions = ?, is_active = ?, updated_at = ?
		WHERE user_id = ?
	`

	_, err := DB.ExecContext(ctx, query,
		config.WordsPerBatch,
		config.Repetitions,
		config.IsActive,
		time.Now(),
		config.UserID,
	)

	return err
}

// UpdateLastBatchTime updates the last batch time for a user
func UpdateLastBatchTime(ctx context.Context, userID int64) error {
	query := `
		UPDATE user_configs
		SET last_batch_time = ?
		WHERE user_id = ?
	`

	_, err := DB.ExecContext(ctx, query, time.Now(), userID)
	return err
} 