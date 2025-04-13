#!/bin/bash

echo "Применение миграции для удаления ограничения уникальности (english_word, topic_id)"

# Определяем путь к файлу базы данных из .env файла
if [ -f ".env" ]; then
    DB_PATH=$(grep -o 'DB_PATH=.*' .env | cut -d '=' -f2)
fi

# Используем путь по умолчанию, если он не определен в .env
if [ -z "$DB_PATH" ]; then
    DB_PATH="./data/engbot.db"
fi

echo "Используем базу данных: $DB_PATH"

# Создаем SQL команды для миграции
SQL_COMMANDS="
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
"

# Выполняем команды SQL
echo "$SQL_COMMANDS" | sqlite3 "$DB_PATH"

if [ $? -eq 0 ]; then
    echo "Миграция успешно выполнена!"
else
    echo "Ошибка при выполнении миграции!"
    exit 1
fi 