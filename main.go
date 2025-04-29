package main

import (
	"log"

	"github.com/example/engbot/internal/bot"
	"github.com/example/engbot/internal/database"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}

	// Initialize database connection
	if err := database.Connect(); err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	// Create and start the bot
	b, err := bot.New()
	if err != nil {
		log.Fatal(err)
	}

	if err := b.Start(); err != nil {
		log.Fatal(err)
	}
} 