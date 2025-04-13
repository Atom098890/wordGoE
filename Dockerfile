FROM golang:latest

WORKDIR /app

# Копируем го-модули и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь исходный код
COPY . .

# Собираем приложение из директории cmd/engbot
RUN go build -o engbot ./cmd/engbot

# Создаем директорию для данных
RUN mkdir -p /app/data

# Устанавливаем права на запуск скриптов
RUN chmod +x /app/scripts/*.sh

# Запускаем бота
CMD ["./engbot"] 