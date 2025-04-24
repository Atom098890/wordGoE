FROM golang:latest

WORKDIR /app

# Копируем го-модули и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь исходный код
COPY . .

# Собираем приложение
RUN go build -o engbot

# Создаем директорию для данных
RUN mkdir -p /app/data

# Запускаем бота
CMD ["./engbot"] 