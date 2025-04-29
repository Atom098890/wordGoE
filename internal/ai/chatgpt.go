package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/example/engbot/pkg/models"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
		Code    string `json:"code"`
	} `json:"error"`
}

type ChatGPT struct {
	apiKey      string
	apiURL      string
	maxTokens   int
	temperature float64
}

func NewChatGPT(apiKey string) *ChatGPT {
	return &ChatGPT{
		apiKey:      apiKey,
		apiURL:      "https://api.openai.com/v1/chat/completions",
		maxTokens:   150,
		temperature: 0.7,
	}
}

// GenerateTextWithWords generates a short English text using the provided words
func (c *ChatGPT) GenerateTextWithWords(words []models.Word, count int) (string, string) {
	// Limit the number of words to use
	maxWords := 10
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
		"Создай короткий, занимательный текст на английском языке (3-4 предложения), "+
			"который включает следующие слова: %s. "+
			"Текст должен быть простым и понятным для начинающего изучать английский. "+
			"Используй ВСЕ предоставленные слова. "+
			"Верни только сам текст без дополнительных пояснений.",
		wordList,
	)

	messages := []Message{
		{Role: "system", Content: "Ты - помощник для изучения английского языка. Твоя задача - создавать простые и понятные тексты для начинающих."},
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

	// Get the generated text
	text := response.Choices[0].Message.Content
	text = strings.TrimSpace(text)

	// Get the translation
	translation := c.TranslateText(text)

	return text, translation
}

// TranslateText translates English text to Russian using ChatGPT
func (c *ChatGPT) TranslateText(text string) string {
	messages := []Message{
		{Role: "system", Content: "You are a translator. Translate the following English text to Russian."},
		{Role: "user", Content: text},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: 0.3,
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

	translation := response.Choices[0].Message.Content
	return strings.TrimSpace(translation)
}

// GenerateExamples generates example sentences for a given word
func (c *ChatGPT) GenerateExamples(word string, count int) (string, error) {
	prompt := fmt.Sprintf(
		"Create %d simple example sentence(s) using the word '%s'. "+
		"The sentence should be clear and easy to understand for English learners. "+
		"Return only the example sentence(s), without any additional text.",
		count, word,
	)

	messages := []Message{
		{Role: "system", Content: "You are an English teacher. Create simple, clear example sentences."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: 0.7,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("error marshaling example request: %v", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		return "", fmt.Errorf("error creating example request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending example request: %v", err)
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("error decoding example response: %v", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("API error in example generation: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no examples generated")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// GenerateIrregularVerbForms gets the forms of an irregular verb
func (c *ChatGPT) GenerateIrregularVerbForms(word string) (string, error) {
	prompt := fmt.Sprintf(
		"If '%s' is an irregular verb, provide its forms in this format:\n"+
		"Infinitive: [form]\nPast Simple: [form]\nPast Participle: [form]\n\n"+
		"If it's not an irregular verb, just respond with 'Not a verb'.",
		word,
	)

	messages := []Message{
		{Role: "system", Content: "You are an English teacher. Provide verb forms accurately and concisely."},
		{Role: "user", Content: prompt},
	}

	request := ChatRequest{
		Model:       "gpt-3.5-turbo",
		Messages:    messages,
		MaxTokens:   c.maxTokens,
		Temperature: 0.3,
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("error marshaling verb forms request: %v", err)
	}

	req, err := http.NewRequest("POST", c.apiURL, bytes.NewBuffer(requestData))
	if err != nil {
		return "", fmt.Errorf("error creating verb forms request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending verb forms request: %v", err)
	}
	defer resp.Body.Close()

	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("error decoding verb forms response: %v", err)
	}

	if response.Error != nil {
		return "", fmt.Errorf("API error in verb forms generation: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no verb forms generated")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}