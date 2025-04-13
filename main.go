package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/engbot/internal/bot"
	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/internal/excel"
	"github.com/joho/godotenv"
)

func main() {
	// Parse command-line flags
	importMode := flag.Bool("import", false, "Run in import mode to import Excel file")
	excelFile := flag.String("file", "", "Excel file to import in import mode")
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Connect to the database
	if err := database.Connect(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("Connected to database")

	// Run in import mode if requested
	if *importMode {
		if *excelFile == "" {
			log.Fatal("Excel file path is required in import mode")
		}
		if err := runImport(*excelFile); err != nil {
			log.Fatalf("Import failed: %v", err)
		}
		return
	}

	// Create and start the bot
	telegramBot, err := bot.New()
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Start the bot in a separate goroutine
	go func() {
		if err := telegramBot.Start(); err != nil {
			log.Fatalf("Bot failed: %v", err)
		}
	}()

	// Wait for termination signal
	waitForTermination()

	// Gracefully stop the bot
	telegramBot.Stop()
	log.Println("Bot stopped")
}

// runImport imports words from an Excel file
func runImport(filePath string) error {
	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", filePath)
	}

	// Use the Excel importer functionality
	fmt.Printf("Importing from %s...\n", filePath)
	
	// Create import configuration
	config := excel.DefaultImportConfig(filePath)
	
	// Perform the import
	result, err := excel.ImportWords(config)
	if err != nil {
		return fmt.Errorf("import error: %v", err)
	}
	
	// Print results
	fmt.Printf("Import complete:\n")
	fmt.Printf("- Total rows processed: %d\n", result.TotalProcessed)
	fmt.Printf("- Topics created: %d\n", result.TopicsCreated)
	fmt.Printf("- Words created: %d\n", result.WordsCreated)
	fmt.Printf("- Words updated: %d\n", result.WordsUpdated)
	
	// Print errors if any
	if len(result.Errors) > 0 {
		fmt.Printf("- Errors (%d):\n", len(result.Errors))
		for _, errMsg := range result.Errors {
			fmt.Printf("  - %s\n", errMsg)
		}
	}
	
	return nil
}

// waitForTermination waits for a termination signal
func waitForTermination() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
} 