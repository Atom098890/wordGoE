-- Create topics table
CREATE TABLE IF NOT EXISTS topics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create words table
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
);

-- Create learned_words table to track word learning progress
CREATE TABLE IF NOT EXISTS learned_words (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    learned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (word_id) REFERENCES words(id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    UNIQUE(word_id, user_id)
);

-- Create user_progress table
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
);

-- Create user_configs table to store user preferences
CREATE TABLE IF NOT EXISTS user_configs (
    user_id INTEGER PRIMARY KEY,
    words_per_batch INTEGER NOT NULL DEFAULT 5,
    repetitions INTEGER NOT NULL DEFAULT 3,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    notification_hour INTEGER NOT NULL DEFAULT 9,
    last_batch_time DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_id INTEGER UNIQUE NOT NULL,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    preferred_topics TEXT,  -- JSON array of topic IDs
    words_per_day INTEGER DEFAULT 10,
    notification_hour INTEGER DEFAULT 9,
    notification_enabled BOOLEAN DEFAULT true,
    is_admin BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
); 