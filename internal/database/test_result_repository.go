package database

import (
	"fmt"
	"time"

	"github.com/example/engbot/pkg/models"
)

// TestResultRepository handles database operations for test results
type TestResultRepository struct{}

// NewTestResultRepository creates a new repository instance
func NewTestResultRepository() *TestResultRepository {
	return &TestResultRepository{}
}

// GetByID returns a test result by ID
func (r *TestResultRepository) GetByID(id int) (*models.TestResult, error) {
	var result models.TestResult
	err := DB.Get(&result, "SELECT * FROM test_results WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get test result: %v", err)
	}
	return &result, nil
}

// GetByUserID returns all test results for a user
func (r *TestResultRepository) GetByUserID(userID int64) ([]models.TestResult, error) {
	var results []models.TestResult
	err := DB.Select(&results, "SELECT * FROM test_results WHERE user_id = $1 ORDER BY test_date DESC", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get test results: %v", err)
	}
	return results, nil
}

// Create inserts a new test result
func (r *TestResultRepository) Create(result *models.TestResult) error {
	query := `
		INSERT INTO test_results (
			user_id, test_type, total_words, correct_words, 
			topics, test_date, duration
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	if result.TestDate.IsZero() {
		result.TestDate = time.Now()
	}
	
	return DB.QueryRow(
		query,
		result.UserID,
		result.TestType,
		result.TotalWords,
		result.CorrectWords,
		result.Topics,
		result.TestDate,
		result.Duration,
	).Scan(&result.ID, &result.CreatedAt)
}

// Delete removes a test result
func (r *TestResultRepository) Delete(id int) error {
	_, err := DB.Exec("DELETE FROM test_results WHERE id = $1", id)
	return err
}

// GetUserStatsByPeriod returns test statistics for a user within a time period
func (r *TestResultRepository) GetUserStatsByPeriod(userID int64, startDate, endDate time.Time) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get total tests taken in period
	var totalTests int
	err := DB.Get(&totalTests, 
		"SELECT COUNT(*) FROM test_results WHERE user_id = $1 AND test_date BETWEEN $2 AND $3", 
		userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	stats["total_tests"] = totalTests
	
	// Get average score
	var avgScore float64
	err = DB.Get(&avgScore, 
		"SELECT COALESCE(AVG(correct_words::float / NULLIF(total_words, 0)), 0) FROM test_results WHERE user_id = $1 AND test_date BETWEEN $2 AND $3", 
		userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	stats["avg_score"] = avgScore
	
	// Get total words tested
	var totalWords int
	err = DB.Get(&totalWords, 
		"SELECT COALESCE(SUM(total_words), 0) FROM test_results WHERE user_id = $1 AND test_date BETWEEN $2 AND $3", 
		userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	stats["total_words_tested"] = totalWords
	
	// Get total correct answers
	var totalCorrect int
	err = DB.Get(&totalCorrect, 
		"SELECT COALESCE(SUM(correct_words), 0) FROM test_results WHERE user_id = $1 AND test_date BETWEEN $2 AND $3", 
		userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	stats["total_correct"] = totalCorrect
	
	return stats, nil
} 