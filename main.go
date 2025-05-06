package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/engbot/internal/bot"
	"github.com/example/engbot/internal/database"
)

func main() {
	// Создаем канал для сигналов
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	// Создаем контекст с отменой
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Подключаемся к базе данных
	err := database.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Создаем бота
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	b, err := bot.NewBot(token)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Канал для ожидания завершения бота
	done := make(chan struct{})

	// Запускаем проверку повторений в отдельной горутине
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := b.CheckDueRepetitions(ctx)
				if err != nil {
					log.Printf("Error checking due repetitions: %v", err)
				}
			case <-ctx.Done():
				log.Println("Stopping repetition checker...")
				return
			}
		}
	}()

	// Горутина для обработки сигналов
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v\n", sig)
		cancel() // Отменяем контекст
		
		// Даем время на graceful shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := b.Stop(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
		
		close(done) // Сигнализируем о завершении
	}()

	// Запускаем бота
	log.Println("Bot started. Press Ctrl+C to stop.")
	go func() {
		if err := b.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("Bot error: %v", err)
		}
	}()

	// Ждем сигнала завершения
	<-done
	log.Println("Bot stopped successfully")
} 