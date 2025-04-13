package excel

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/pkg/models"
	"github.com/xuri/excelize/v2"
)

// ImportConfig holds configuration for the import process
type ImportConfig struct {
	FilePath           string
	SheetName          string
	WordColumn         string
	TranslationColumn  string
	ContextColumn      string
	TopicColumn        string
	DifficultyColumn   string
	HeaderRow          int
	StartRow           int
	// Optional
	PronunciationColumn string
}

// DefaultImportConfig returns a default configuration
func DefaultImportConfig(filePath string) ImportConfig {
	return ImportConfig{
		FilePath:          filePath,
		SheetName:         "Sheet1",
		WordColumn:        "A",
		TranslationColumn: "B",
		ContextColumn:     "C",
		TopicColumn:       "D",
		DifficultyColumn:  "E",
		HeaderRow:         1,
		StartRow:          2,
		// Optional
		PronunciationColumn: "F",
	}
}

// ImportResult holds the result of an import operation
type ImportResult struct {
	TotalProcessed  int
	TopicsCreated   int
	WordsCreated    int
	WordsUpdated    int
	Errors          []string
}

// ImportWords imports words from an Excel or CSV file
func ImportWords(config ImportConfig) (*ImportResult, error) {
	// Check the file extension
	ext := strings.ToLower(filepath.Ext(config.FilePath))
	
	if ext == ".csv" {
		// Process as CSV
		return importFromCSV(config)
	} 
	
	// Process as Excel
	return importFromExcel(config)
}

// importFromExcel imports words from an Excel file
func importFromExcel(config ImportConfig) (*ImportResult, error) {
	// Open Excel file
	f, err := excelize.OpenFile(config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Excel file: %v", err)
	}
	defer f.Close()

	// Initialize repositories
	topicRepo := database.NewTopicRepository()
	wordRepo := database.NewWordRepository()

	// Initialize result
	result := &ImportResult{
		Errors: make([]string, 0),
	}

	// Get all existing topics for reference
	existingTopics, err := topicRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get existing topics: %v", err)
	}

	// Map topic names to IDs for quick lookup
	topicMap := make(map[string]int64)
	for _, topic := range existingTopics {
		topicMap[strings.ToLower(topic.Name)] = topic.ID
	}

	// Get rows from Excel
	rows, err := f.GetRows(config.SheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rows: %v", err)
	}

	// Process rows
	for i, row := range rows {
		// Skip header rows
		if i < config.StartRow-1 {
			continue
		}

		result.TotalProcessed++

		if err := processRow(row, config, topicMap, topicRepo, wordRepo, result, i+1); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Row %d: %v", i+1, err))
		}
	}

	return result, nil
}

// importFromCSV imports words from a CSV file
func importFromCSV(config ImportConfig) (*ImportResult, error) {
	// Open CSV file
	file, err := os.Open(config.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %v", err)
	}
	defer file.Close()

	// Initialize reader
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	reader.LazyQuotes = true    // Allow lazy quotes for custom CSV format

	// Initialize repositories
	topicRepo := database.NewTopicRepository()
	wordRepo := database.NewWordRepository()

	// Initialize result
	result := &ImportResult{
		Errors: make([]string, 0),
	}

	// Get all existing topics for reference
	existingTopics, err := topicRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get existing topics: %v", err)
	}

	// Map topic names to IDs for quick lookup
	topicMap := make(map[string]int64)
	for _, topic := range existingTopics {
		topicMap[strings.ToLower(topic.Name)] = topic.ID
	}

	// Read all records
	rowNum := 0
	currentTopic := "Глаголы" // Default topic

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV: %v", err)
		}

		rowNum++

		// Skip header rows
		if rowNum < config.StartRow {
			continue
		}

		// Check if this is a topic header row (e.g., "Движение,,")
		if len(row) >= 2 && strings.TrimSpace(row[0]) != "" && strings.TrimSpace(row[1]) == "" {
			// This looks like a topic header
			potentialTopic := strings.TrimSpace(row[0])
			// Remove quotes if present
			potentialTopic = strings.Trim(potentialTopic, "\"")
			if potentialTopic != "" {
				currentTopic = potentialTopic
				continue // Skip processing this row as a word
			}
		}

		result.TotalProcessed++

		// Process the row with current topic
		if err := processCSVRow(row, topicMap, topicRepo, wordRepo, result, rowNum, currentTopic); err != nil {
			// Only add error if it's not "skipping row" error
			if err.Error() != "skipping row" {
				result.Errors = append(result.Errors, fmt.Sprintf("Row %d: %v", rowNum, err))
			}
		}
	}

	return result, nil
}

// processRow processes a single row from Excel
func processRow(row []string, config ImportConfig, topicMap map[string]int64, 
                topicRepo *database.TopicRepository, wordRepo *database.WordRepository, 
                result *ImportResult, rowNum int) error {
	// Get cell values
	var word, translation, context, topicName, difficulty, pronunciation string
	
	// Check bounds for each column
	if colIdx := columnToIndex(config.WordColumn); colIdx < len(row) {
		word = row[colIdx]
	}
	if colIdx := columnToIndex(config.TranslationColumn); colIdx < len(row) {
		translation = row[colIdx]
	}
	if colIdx := columnToIndex(config.ContextColumn); colIdx < len(row) {
		context = row[colIdx]
	}
	if colIdx := columnToIndex(config.TopicColumn); colIdx < len(row) {
		topicName = row[colIdx]
	}
	if colIdx := columnToIndex(config.DifficultyColumn); colIdx < len(row) {
		difficulty = row[colIdx]
	}
	if config.PronunciationColumn != "" {
		if colIdx := columnToIndex(config.PronunciationColumn); colIdx < len(row) {
			pronunciation = row[colIdx]
		}
	}

	return processWordData(word, translation, context, topicName, difficulty, pronunciation, 
	                     topicMap, topicRepo, wordRepo, result, rowNum)
}

// processCSVRow processes a single row from CSV
func processCSVRow(row []string, topicMap map[string]int64, 
                   topicRepo *database.TopicRepository, wordRepo *database.WordRepository, 
                   result *ImportResult, rowNum int, currentTopic string) error {
	// Пропускаем заголовки и строки с названиями категорий
	if len(row) < 3 || strings.HasPrefix(row[0], "\"") || row[0] == "" {
		return fmt.Errorf("skipping row")
	}
	
	// Обрабатываем формат: Английское слово,[транскрипция],перевод
	var word, translation, context, topicName, difficulty, pronunciation string
	
	// Используем переданную тему
	topicName = currentTopic
	
	// Обработка полей
	if len(row) > 0 {
		// Очищаем от возможных скобок лишних символов "(went, gone)"
		word = cleanWord(row[0])
	}
	
	if len(row) > 1 {
		// Транскрипция в квадратных скобках - используем как произношение
		pronunciation = row[1]
	}
	
	if len(row) > 2 {
		translation = row[2]
	}
	
	// Установка контекста - добавляем транскрипцию как часть контекста
	if pronunciation != "" {
		context = "Произношение: " + pronunciation
	}
	
	// Устанавливаем среднюю сложность по умолчанию
	difficulty = "3"
	
	return processWordData(word, translation, context, topicName, difficulty, pronunciation, 
	                     topicMap, topicRepo, wordRepo, result, rowNum)
}

// cleanWord удаляет из слова дополнительную информацию в скобках
func cleanWord(word string) string {
	// Удаляем информацию в скобках "(went, gone)" из слова
	indexOpenParen := strings.Index(word, "(")
	if indexOpenParen > 0 {
		return strings.TrimSpace(word[:indexOpenParen])
	}
	return strings.TrimSpace(word)
}

// processWordData handles the common logic for processing word data from any source
func processWordData(word, translation, context, topicName, difficulty, pronunciation string, 
                    topicMap map[string]int64, topicRepo *database.TopicRepository, 
                    wordRepo *database.WordRepository, result *ImportResult, _ int) error {
	// Validate required fields
	if word == "" || translation == "" || topicName == "" {
		return fmt.Errorf("missing required fields")
	}

	// Get topic ID or create a new topic
	var topicID int64
	topicNameLower := strings.ToLower(strings.TrimSpace(topicName))
	if id, exists := topicMap[topicNameLower]; exists {
		topicID = id
	} else {
		newTopic := &models.Topic{
			Name:        topicName,
			Description: "", // Could be added as an additional column
		}
		if err := topicRepo.Create(newTopic); err != nil {
			return fmt.Errorf("failed to create topic: %v", err)
		}
		topicID = newTopic.ID
		topicMap[topicNameLower] = topicID
		result.TopicsCreated++
	}

	// Parse difficulty (default to 3 if not provided or invalid)
	difficultyVal := 3
	if difficulty != "" {
		if val, err := parseIntInRange(difficulty, 1, 5); err == nil {
			difficultyVal = val
		}
	}

	// Create or update the word
	existingWords, err := wordRepo.SearchWords(word)
	if err != nil {
		return fmt.Errorf("failed to search for existing words: %v", err)
	}
	
	isUpdate := false
	
	for _, existingWord := range existingWords {
		if strings.EqualFold(existingWord.EnglishWord, word) && existingWord.TopicID == topicID {
			// Update existing word
			existingWord.Translation = translation
			existingWord.Context = context
			existingWord.Difficulty = difficultyVal
			existingWord.Pronunciation = pronunciation
			
			if err := wordRepo.Update(&existingWord); err != nil {
				return fmt.Errorf("failed to update word: %v", err)
			}
			result.WordsUpdated++
			isUpdate = true
			break
		}
	}
	
	if !isUpdate {
		// Create new word
		newWord := &models.Word{
			EnglishWord:   word,
			Translation:   translation,
			Context:       context,
			TopicID:       topicID,
			Difficulty:    difficultyVal,
			Pronunciation: pronunciation,
		}
		
		if err := wordRepo.Create(newWord); err != nil {
			return fmt.Errorf("failed to create word: %v", err)
		}
		result.WordsCreated++
	}
	
	return nil
}

// Helper function to convert Excel column letter to index
func columnToIndex(column string) int {
	column = strings.ToUpper(column)
	index := 0
	for i := 0; i < len(column); i++ {
		index = index*26 + int(column[i]-'A'+1)
	}
	return index - 1
}

// Helper function to parse integer within a range
func parseIntInRange(s string, min, max int) (int, error) {
	var val int
	if _, err := fmt.Sscanf(s, "%d", &val); err != nil {
		return min, err
	}
	if val < min {
		return min, nil
	}
	if val > max {
		return max, nil
	}
	return val, nil
} 