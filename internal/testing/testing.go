package testing

import (
	"math/rand"
	"time"

	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/pkg/models"
)

// TestingModule handles knowledge testing functionality
type TestingModule struct {
	wordRepo *database.WordRepository
	testRepo *database.TestResultRepository
}

// NewTestingModule creates a new testing module
func NewTestingModule() *TestingModule {
	return &TestingModule{
		wordRepo: database.NewWordRepository(),
		testRepo: database.NewTestResultRepository(),
	}
}

// TestType represents different types of tests
type TestType string

const (
	// MultipleChoice represents a multiple choice test
	MultipleChoice TestType = "multiple_choice"
	// TextInput represents a test where the user types the answer
	TextInput TestType = "text_input"
	// ContextTest represents a test where the user fills in a blank in a sentence
	ContextTest TestType = "context"
)

// TestQuestion represents a single test question
type TestQuestion struct {
	Word           models.Word   // The word being tested
	Options        []string      // Possible answers (for multiple choice)
	CorrectIndex   int           // Index of correct answer in options
	QuestionType   TestType      // Type of question
	ContextSentence string       // Sentence with blank (for context tests)
}

// CreateTest generates a test with the specified parameters
func (t *TestingModule) CreateTest(userID int64, topicIDs []int64, questionCount int, questionType TestType) ([]TestQuestion, error) {
	// Initialize random generator
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	
	// Get words for test
	var words []models.Word
	var err error
	
	if len(topicIDs) > 0 {
		// Get words from specific topics
		for _, topicID := range topicIDs {
			topicWords, err := t.wordRepo.GetByTopic(topicID)
			if err != nil {
				return nil, err
			}
			words = append(words, topicWords...)
		}
	} else {
		// Get all words
		words, err = t.wordRepo.GetAll()
		if err != nil {
			return nil, err
		}
	}
	
	// Shuffle words
	rnd.Shuffle(len(words), func(i, j int) {
		words[i], words[j] = words[j], words[i]
	})
	
	// Limit to requested count
	if len(words) > questionCount {
		words = words[:questionCount]
	}
	
	// Create questions
	questions := make([]TestQuestion, 0, len(words))
	
	for _, word := range words {
		question := TestQuestion{
			Word:         word,
			QuestionType: questionType,
		}
		
		switch questionType {
		case MultipleChoice:
			// Get incorrect options
			incorrectOptions, err := t.getIncorrectOptions(word, words, 3)
			if err != nil {
				return nil, err
			}
			
			// Add correct option and shuffle
			allOptions := append(incorrectOptions, word.Translation)
			correctIndex := len(allOptions) - 1
			
			// Shuffle options
			rnd.Shuffle(len(allOptions), func(i, j int) {
				if i == correctIndex {
					correctIndex = j
				} else if j == correctIndex {
					correctIndex = i
				}
				allOptions[i], allOptions[j] = allOptions[j], allOptions[i]
			})
			
			question.Options = allOptions
			question.CorrectIndex = correctIndex
			
		case ContextTest:
			// Generate context if it doesn't exist
			context := word.Context
			if context == "" {
				context = "This is a sentence with the word " + word.Word + "."
			}
			
			// Create a blank sentence by replacing the word with underscores
			blankContext := replaceWordWithBlank(context, word.Word)
			question.ContextSentence = blankContext
		}
		
		questions = append(questions, question)
	}
	
	return questions, nil
}

// SaveTestResult records the results of a test
func (t *TestingModule) SaveTestResult(userID int64, testType TestType, questions []TestQuestion, correct int, duration int) error {
	// Extract topic IDs from questions
	topicIDs := make([]int64, 0)
	topicMap := make(map[int64]bool)
	
	for _, q := range questions {
		if !topicMap[q.Word.TopicID] {
			topicIDs = append(topicIDs, q.Word.TopicID)
			topicMap[q.Word.TopicID] = true
		}
	}
	
	// Create test result
	result := &models.TestResult{
		UserID:       userID,
		TestType:     string(testType),
		TotalWords:   len(questions),
		CorrectWords: correct,
		Topics:       topicIDs,
		TestDate:     time.Now(),
		Duration:     duration,
	}
	
	return t.testRepo.Create(result)
}

// getIncorrectOptions gets n incorrect options for a multiple choice test
func (t *TestingModule) getIncorrectOptions(word models.Word, allWords []models.Word, count int) ([]string, error) {
	options := make([]string, 0, count)
	
	// First try to get words from the same topic
	sameTopicWords, err := t.wordRepo.GetByTopic(word.TopicID)
	if err != nil {
		return nil, err
	}
	
	// Filter out the word itself
	filteredWords := make([]models.Word, 0, len(sameTopicWords))
	for _, w := range sameTopicWords {
		if w.ID != word.ID {
			filteredWords = append(filteredWords, w)
		}
	}
	
	// Shuffle filtered words
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(filteredWords), func(i, j int) {
		filteredWords[i], filteredWords[j] = filteredWords[j], filteredWords[i]
	})
	
	// Add translations from same topic first
	for i := 0; i < len(filteredWords) && len(options) < count; i++ {
		options = append(options, filteredWords[i].Translation)
	}
	
	// If we still need more options, use words from other topics
	if len(options) < count {
		// Shuffle all words
		rnd.Shuffle(len(allWords), func(i, j int) {
			allWords[i], allWords[j] = allWords[j], allWords[i]
		})
		
		// Add translations from other topics
		for i := 0; i < len(allWords) && len(options) < count; i++ {
			w := allWords[i]
			if w.ID != word.ID && w.TopicID != word.TopicID {
				options = append(options, w.Translation)
			}
		}
	}
	
	return options, nil
}

// replaceWordWithBlank replaces a word in a sentence with a blank
func replaceWordWithBlank(sentence, word string) string {
	// Simple implementation - replace with blank line
	// In a real implementation, you would want to use proper regex
	// to match the word with correct word boundaries
	replacement := "_______"
	
	// Case insensitive replace
	lowercaseSentence := sentence
	lowercaseWord := word
	
	// Replace the word with blank
	result := ""
	wordLen := len(lowercaseWord)
	for i := 0; i <= len(lowercaseSentence)-wordLen; i++ {
		if lowerMatchAt(lowercaseSentence, lowercaseWord, i) {
			result = sentence[:i] + replacement + sentence[i+wordLen:]
			break
		}
	}
	
	if result == "" {
		// If word not found, just append the blank
		result = sentence + " " + replacement
	}
	
	return result
}

// lowerMatchAt checks if strings match at position, ignoring case
func lowerMatchAt(s, substr string, pos int) bool {
	if pos+len(substr) > len(s) {
		return false
	}
	
	for i := 0; i < len(substr); i++ {
		if toLowerCase(s[pos+i]) != toLowerCase(substr[i]) {
			return false
		}
	}
	return true
}

// toLowerCase converts a byte to lowercase
func toLowerCase(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
} 