FROM golang:latest

WORKDIR /app

# Копируем го-модули и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь исходный код
COPY . .

# Проверяем наличие директории, если нет - создаем
RUN mkdir -p cmd/engbot

# Проверяем существование main.go
RUN if [ -f "main.go" ] && [ ! -f "cmd/engbot/main.go" ]; then \
      cp main.go cmd/engbot/; \
    fi

# Собираем приложение из директории cmd/engbot
RUN if [ -f "cmd/engbot/main.go" ]; then \
      go build -o engbot ./cmd/engbot; \
    elif [ -f "main.go" ]; then \
      go build -o engbot .; \
    else \
      echo "Error: No main.go found" && exit 1; \
    fi

# Создаем директорию для данных
RUN mkdir -p /app/data

# Устанавливаем права на запуск скриптов
RUN chmod +x /app/scripts/*.sh

# Запускаем бота
CMD ["./engbot"] 