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
	
	// Import configuration flags
	sheetName := flag.String("sheet", "Sheet1", "Sheet name in Excel file")
	wordCol := flag.String("word-col", "A", "Column with English words")
	transCol := flag.String("trans-col", "B", "Column with translations")
	contextCol := flag.String("context-col", "C", "Column with context/description")
	topicCol := flag.String("topic-col", "D", "Column with topics")
	diffCol := flag.String("diff-col", "E", "Column with difficulty ratings")
	pronCol := flag.String("pron-col", "F", "Column with pronunciations")
	
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
		
		// Create import configuration 
		config := excel.ImportConfig{
			FilePath:            *excelFile,
			SheetName:           *sheetName,
			WordColumn:          *wordCol,
			TranslationColumn:   *transCol,
			ContextColumn:       *contextCol,
			TopicColumn:         *topicCol,
			DifficultyColumn:    *diffCol,
			PronunciationColumn: *pronCol,
			HeaderRow:           1,
			StartRow:            2,
		}
		
		if err := runImport(config); err != nil {
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
func runImport(config excel.ImportConfig) error {
	// Check if the file exists
	if _, err := os.Stat(config.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", config.FilePath)
	}

	// Use the Excel importer functionality
	fmt.Printf("Importing from %s...\n", config.FilePath)
	
	// Perform the import
	result, err := excel.ImportWords(config)
	if err != nil {
		return fmt.Errorf("import error: %v", err)
	}
	
	// Print results
	fmt.Printf("Import complete:\n")
	fmt.Printf("- Total rows processed: %d\n", result.TotalProcessed)
	fmt.Printf("- Topics created: %d\n", result.TopicsCreated)
	fmt.Printf("- Words created: %d\n", result.Created)
	fmt.Printf("- Words updated: %d\n", result.Updated)
	fmt.Printf("- Words skipped: %d\n", result.Skipped)
	
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