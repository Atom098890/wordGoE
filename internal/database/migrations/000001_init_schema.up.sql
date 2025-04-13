-- Create topics table
CREATE TABLE IF NOT EXISTS topics (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create words table
CREATE TABLE IF NOT EXISTS words (
    id SERIAL PRIMARY KEY,
    english_word VARCHAR(255) NOT NULL,
    translation TEXT NOT NULL,
    context TEXT,
    topic_id INTEGER REFERENCES topics(id),
    difficulty INTEGER DEFAULT 3 CHECK (difficulty BETWEEN 1 AND 5),
    pronunciation VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(english_word, topic_id)
);

-- Create index on words
CREATE INDEX idx_words_topic ON words(topic_id);

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id BIGINT PRIMARY KEY, -- Telegram user ID
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    is_admin BOOLEAN DEFAULT FALSE,
    preferred_topics INTEGER[] DEFAULT '{}',
    notification_enabled BOOLEAN DEFAULT TRUE,
    notification_hour INTEGER DEFAULT 8 CHECK (notification_hour BETWEEN 0 AND 23),
    words_per_day INTEGER DEFAULT 5 CHECK (words_per_day BETWEEN 1 AND 50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create user_progress table
CREATE TABLE IF NOT EXISTS user_progress (
    id SERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    word_id INTEGER REFERENCES words(id) ON DELETE CASCADE,
    last_review_date TIMESTAMP WITH TIME ZONE,
    next_review_date TIMESTAMP WITH TIME ZONE,
    interval INTEGER DEFAULT 0,
    easiness_factor FLOAT DEFAULT 2.5,
    repetitions INTEGER DEFAULT 0,
    last_quality INTEGER DEFAULT 0 CHECK (last_quality BETWEEN 0 AND 5),
    consecutive_right INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, word_id)
);

-- Create indexes on user_progress
CREATE INDEX idx_user_progress_user ON user_progress(user_id);
CREATE INDEX idx_user_progress_word ON user_progress(word_id);
CREATE INDEX idx_user_progress_next_review ON user_progress(next_review_date);

-- Create test_results table
CREATE TABLE IF NOT EXISTS test_results (
    id SERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    test_type VARCHAR(50) NOT NULL,
    total_words INTEGER NOT NULL,
    correct_words INTEGER NOT NULL,
    topics INTEGER[] DEFAULT '{}',
    test_date TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    duration INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create index on test_results
CREATE INDEX idx_test_results_user ON test_results(user_id); 