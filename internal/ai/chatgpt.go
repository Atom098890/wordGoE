package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/example/engbot/pkg/models"
)

// ChatGPT represents a client for the OpenAI ChatGPT API
type ChatGPT struct {
	apiKey     string
	apiURL     string
	maxTokens  int
	temperature float64
}

// New creates a new ChatGPT client
func New() (*ChatGPT, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	return &ChatGPT{
		apiKey:     apiKey,
		apiURL:     "https://api.openai.com/v1/chat/completions",
		maxTokens:  100,
		temperature: 0.7,
	}, nil
}

// Message represents a message in the ChatGPT conversation
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents a request to the ChatGPT API
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
}

// ChatResponse represents a response from the ChatGPT API
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// GenerateExample generates an example sentence for the given word
func (c *ChatGPT) GenerateExample(word *models.Word) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a short, practical example sentence in English that naturally includes the word '%s' (which translates to '%s' in Russian).",
		word.Word, word.Translation,
	)

	messages := []Message{
		{Role: "system", Content: "Ты - помощник для изучения английского языка. Твоя задача - создавать качественные примеры использования английских слов."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: c.temperature,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	// Clean up the response
	example := response.Choices[0].Message.Content
	example = strings.TrimSpace(example)

	return example, nil
}

// GenerateExampleWithFallback generates an example with fallback to the stored context
func (c *ChatGPT) GenerateExampleWithFallback(word *models.Word) string {
	example, err := c.GenerateExample(word)
	if err != nil {
		// Log the error and fall back to the stored context
		fmt.Printf("Error generating example for '%s': %v\n", word.Word, err)
		
		// If there's a stored context, use it
		if word.Description != "" {
			return word.Description
		}
		
		// If no stored context, create a basic example
		return fmt.Sprintf("This is an example of the word '%s'.", word.Word)
	}
	
	return example
}

// TranslateText translates the given text from English to Russian
func (c *ChatGPT) TranslateText(text string) string {
	prompt := fmt.Sprintf(
		"Переведи следующий текст с английского на русский:\n\n%s\n\nВерни только перевод, без дополнительных пояснений.",
		text,
	)

	messages := []Message{
		{Role: "system", Content: "Ты - переводчик с английского на русский язык. Твоя задача - делать качественные переводы, сохраняя смысл и стиль оригинала."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   c.maxTokens * 2, // More tokens for translation
		Temperature: 0.3,             // Lower temperature for more accurate translations
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("Error marshaling translation request: %v\n", err)
		return ""
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		fmt.Printf("Error creating translation request: %v\n", err)
		return ""
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending translation request: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		fmt.Printf("Error decoding translation response: %v\n", err)
		return ""
	}

	if response.Error != nil {
		fmt.Printf("API error in translation: %s\n", response.Error.Message)
		return ""
	}

	if len(response.Choices) == 0 {
		fmt.Printf("No translation choices returned\n")
		return ""
	}

	// Clean up the response
	translation := response.Choices[0].Message.Content
	translation = strings.TrimSpace(translation)

	return translation
}

// GenerateTextWithWords generates a short English text using the provided words
func (c *ChatGPT) GenerateTextWithWords(words []models.Word, count int) (string, string) {
	// Limit the number of words to use
	maxWords := 5
	if count < maxWords {
		maxWords = count
	}
	
	// Get word list for prompt
	var wordList string
	for i := 0; i < maxWords && i < len(words); i++ {
		if i > 0 {
			wordList += ", "
		}
		wordList += words[i].Word
	}
	
	prompt := fmt.Sprintf(
		"Создай короткий, занимательный текст на английском языке (2-3 предложения), "+
			"который включает следующие слова: %s. "+
			"Текст должен быть простым и понятным для начинающего изучать английский. "+
			"Верни только сам текст без дополнительных пояснений.",
		wordList,
	)

	messages := []Message{
		{Role: "system", Content: "Ты - помощник для изучения английского языка. Твоя задача - создавать короткие тексты, помогающие запомнить новые слова в контексте."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   150,
		Temperature: 0.8, // Higher temperature for more creativity
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		fmt.Printf("Error marshaling text generation request: %v\n", err)
		return "", ""
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		fmt.Printf("Error creating text generation request: %v\n", err)
		return "", ""
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending text generation request: %v\n", err)
		return "", ""
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		fmt.Printf("Error decoding text generation response: %v\n", err)
		return "", ""
	}

	if response.Error != nil {
		fmt.Printf("API error in text generation: %s\n", response.Error.Message)
		return "", ""
	}

	if len(response.Choices) == 0 {
		fmt.Printf("No text generation choices returned\n")
		return "", ""
	}

	// Get the generated English text
	englishText := strings.TrimSpace(response.Choices[0].Message.Content)
	
	// Translate the text to Russian
	russianText := c.TranslateText(englishText)
	
	return englishText, russianText
}

// GenerateVerbConjugation generates examples of verb conjugation in three tenses: present, past, and future
func (c *ChatGPT) GenerateVerbConjugation(word string) (string, error) {
	// Only generate conjugations for verbs
	prompt := fmt.Sprintf(
		"If the word '%s' is a verb in English, provide its basic conjugation in three tenses WITHOUT PRONOUNS, just the verb forms. "+
		"Format the output like this (use this exact format):\n\n"+
		"Present: [verb in present tense]\n"+
		"Past: [verb in past tense]\n"+
		"Future: [verb in future tense without 'will']\n\n"+
		"Example for the verb 'run':\n"+
		"Present: run/runs\n"+
		"Past: ran\n"+
		"Future: will run\n\n"+
		"If the word is not a verb, respond with 'Not a verb'.",
		word,
	)

	messages := []Message{
		{Role: "system", Content: "Ты - помощник для изучения английского языка. Твоя задача - предоставлять информацию о склонении глаголов в разных временах в краткой форме."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   150,
		Temperature: 0.3, // Lower temperature for more accurate information
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	conjugation := response.Choices[0].Message.Content
	conjugation = strings.TrimSpace(conjugation)

	// Check if the word is not a verb
	if strings.Contains(conjugation, "Not a verb") {
		return "", nil
	}

	return conjugation, nil
} 