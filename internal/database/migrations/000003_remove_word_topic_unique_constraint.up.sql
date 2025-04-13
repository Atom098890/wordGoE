-- Удаляем ограничение уникальности (english_word, topic_id)
-- В SQLite нужно пересоздать таблицу без этого ограничения

-- 1. Создаем новую таблицу без ограничения уникальности
CREATE TABLE words_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    word TEXT NOT NULL,
    translation TEXT NOT NULL,
    description TEXT,
    topic_id INTEGER NOT NULL,
    difficulty INTEGER DEFAULT 3,
    pronunciation TEXT,
    examples TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (topic_id) REFERENCES topics(id)
);

-- 2. Копируем данные из старой таблицы
INSERT INTO words_new (id, word, translation, description, topic_id, difficulty, pronunciation, examples, created_at, updated_at)
SELECT id, word, translation, description, topic_id, difficulty, pronunciation, examples, created_at, updated_at FROM words;

-- 3. Удаляем старую таблицу
DROP TABLE words;

-- 4. Переименовываем новую таблицу
ALTER TABLE words_new RENAME TO words;

-- 5. Создаем индекс для темы
CREATE INDEX idx_words_topic ON words(topic_id); 