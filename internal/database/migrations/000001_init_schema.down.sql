-- Drop test_results table and indexes
DROP INDEX IF EXISTS idx_test_results_user;
DROP TABLE IF EXISTS test_results;

-- Drop user_progress table and indexes
DROP INDEX IF EXISTS idx_user_progress_next_review;
DROP INDEX IF EXISTS idx_user_progress_word;
DROP INDEX IF EXISTS idx_user_progress_user;
DROP TABLE IF EXISTS user_progress;

-- Drop users table
DROP TABLE IF EXISTS users;

-- Drop words table and indexes
DROP INDEX IF EXISTS idx_words_topic;
DROP TABLE IF EXISTS words;

-- Drop topics table
DROP TABLE IF EXISTS topics; 