package bot

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/example/engbot/internal/ai"
	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/internal/excel"
	"github.com/example/engbot/internal/scheduler"
	"github.com/example/engbot/internal/spaced_repetition"
	"github.com/example/engbot/internal/testing"
	"github.com/example/engbot/pkg/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot represents the Telegram bot
type Bot struct {
	api          *tgbotapi.BotAPI
	scheduler    *scheduler.Scheduler
	adminUserIDs map[int64]bool
	chatGPT      *ai.ChatGPT
	sm2          *spaced_repetition.SM2
	awaitingFileUpload map[int64]bool
	learningSessions map[int64]learningSession
}

// Struct for storing learning session data
type learningSession struct {
	Words      []models.Word // Words for this session
	CurrentIdx int           // Index of the current word
}

// New creates a new bot instance
func New() (*Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	// Create bot API instance
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %v", err)
	}

	// Parse admin user IDs from environment variable
	adminUserIDs := make(map[int64]bool)
	adminIDs := os.Getenv("ADMIN_USER_IDS")
	if adminIDs != "" {
		ids := strings.Split(adminIDs, ",")
		for _, id := range ids {
			if userID, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err == nil {
				adminUserIDs[userID] = true
			}
		}
	}

	// Create ChatGPT client
	chatGPT, err := ai.New()
	if err != nil {
		log.Printf("Warning: ChatGPT client initialization failed: %v. Will use fallback examples.", err)
		// Continue without ChatGPT - we'll use fallbacks
	}

	bot := &Bot{
		api:          api,
		adminUserIDs: adminUserIDs,
		chatGPT:      chatGPT,
		sm2:          spaced_repetition.NewSM2(),
		awaitingFileUpload: make(map[int64]bool),
		learningSessions: make(map[int64]learningSession),
	}

	// Create scheduler with bot as notifier
	bot.scheduler = scheduler.New(bot)

	return bot, nil
}

// Start begins listening for updates from Telegram
func (b *Bot) Start() error {
	// Set update configuration
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	// Get updates channel
	updates := b.api.GetUpdatesChan(updateConfig)

	// Start the scheduler
	b.scheduler.Start()
	log.Println("Bot scheduler started")

	// Process updates
	log.Printf("Bot started as @%s", b.api.Self.UserName)
	for update := range updates {
		go b.handleUpdate(update)
	}

	return nil
}

// Stop gracefully stops the bot
func (b *Bot) Stop() {
	// Stop the scheduler
	b.scheduler.Stop()
	log.Println("Bot scheduler stopped")
}

// SendReminders implements the scheduler.Notifier interface
func (b *Bot) SendReminders(userID int64, count int) error {
	msg := tgbotapi.NewMessage(userID, fmt.Sprintf("You have %d words due for review. Use /learn to start learning!", count))
	_, err := b.api.Send(msg)
	return err
}

// isAdmin checks if a user is an admin
func (b *Bot) isAdmin(userID int64) bool {
	return b.adminUserIDs[userID]
}

// handleUpdate processes a single update from Telegram
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	// Handle different types of updates
	if update.Message != nil {
		b.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		b.handleCallbackQuery(update.CallbackQuery)
	}

	// Handle document (file) uploads for import
	if update.Message != nil && update.Message.Document != nil && b.awaitingFileUpload[update.Message.From.ID] {
		// Проверяем, является ли пользователь администратором
		isAdmin := b.isAdmin(update.Message.From.ID)
		if !isAdmin {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⛔ У вас нет прав для выполнения этой команды.")
			b.api.Send(msg)
			return
		}
		
		document := update.Message.Document
		fileExt := strings.ToLower(filepath.Ext(document.FileName))
		
		// Проверяем, что файл имеет поддерживаемый формат
		if fileExt != ".xlsx" && fileExt != ".csv" {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Файл должен быть в формате .xlsx или .csv")
			b.api.Send(msg)
			return
		}
		
		// Отправляем сообщение о начале загрузки
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⏳ Загружаю файл... Пожалуйста, подождите.")
		statusMsg, _ := b.api.Send(msg)
		
		// Получаем файл
		fileURL, err := b.api.GetFileDirectURL(document.FileID)
		if err != nil {
			b.api.Send(tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsg.MessageID, "❌ Ошибка при получении файла: "+err.Error()))
			return
		}
		
		// Загружаем файл
		tempDir, err := ioutil.TempDir("", "engbot_import_")
		if err != nil {
			b.api.Send(tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsg.MessageID, "❌ Ошибка при создании временной директории: "+err.Error()))
			return
		}
		defer os.RemoveAll(tempDir)
		
		tempFilePath := filepath.Join(tempDir, document.FileName)
		err = b.downloadFile(fileURL, tempFilePath)
		if err != nil {
			b.api.Send(tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsg.MessageID, "❌ Ошибка при загрузке файла: "+err.Error()))
			return
		}
		
		// Обновляем статус
		b.api.Send(tgbotapi.NewEditMessageText(update.Message.Chat.ID, statusMsg.MessageID, "✅ Файл загружен, импортирую слова..."))
		
		// Импортируем слова
		config := excel.DefaultImportConfig(tempFilePath)
		result, err := excel.ImportWords(config)
		
		if err != nil {
			b.api.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "❌ Ошибка при импорте: "+err.Error()))
			return
		}
		
		// Формируем отчет и отправляем его
		reportText := formatImportReport(result)
		b.api.Send(tgbotapi.NewMessage(update.Message.Chat.ID, reportText))
		
		// Убираем пользователя из списка ожидающих загрузку файла
		delete(b.awaitingFileUpload, update.Message.From.ID)
		
		return
	}
}

// handleMessage processes text messages and commands
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Check if it's a command
	if message.IsCommand() {
		b.handleCommand(message)
		return
	}

	// Handle regular text messages based on user state
	// This would be implemented with a user state manager
	// For now, just reply with a placeholder message
	msg := tgbotapi.NewMessage(message.Chat.ID, "I'm not sure what to do with that message. Try using one of the commands like /start or /help.")
	b.api.Send(msg)
}

// handleCommand processes bot commands
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	userID := message.From.ID
	
	// Register user if not exists
	b.registerUserIfNeeded(userID, message.From.UserName, message.From.FirstName, message.From.LastName)

	switch message.Command() {
	case "start":
		b.handleStartCommand(message)
	case "help":
		b.handleHelpCommand(message)
	case "learn":
		b.handleLearnCommand(message)
	case "stats":
		b.handleStatsCommand(message)
	case "settings":
		b.handleSettingsCommand(message)
	case "test":
		b.handleTestCommand(message)
	case "import":
		// Admin-only command
		if b.isAdmin(userID) {
			b.handleImportCommand(message)
		} else {
			msg := tgbotapi.NewMessage(message.Chat.ID, "This command is only available for administrators.")
			b.api.Send(msg)
		}
	case "admin_stats":
		// Admin-only command
		if b.isAdmin(userID) {
			b.handleAdminStatsCommand(message)
		} else {
			msg := tgbotapi.NewMessage(message.Chat.ID, "This command is only available for administrators.")
			b.api.Send(msg)
		}
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "Unknown command. Use /help to see available commands.")
		b.api.Send(msg)
	}
}

// handleCallbackQuery processes inline keyboard button presses
func (b *Bot) handleCallbackQuery(callbackQuery *tgbotapi.CallbackQuery) {
	// Acknowledge the callback query
	callback := tgbotapi.NewCallback(callbackQuery.ID, "")
	b.api.Request(callback)

	// Extract data from the callback
	data := callbackQuery.Data
	userID := callbackQuery.From.ID

	// Handle different callback types
	if strings.HasPrefix(data, "topic_") {
		// Topic selection callback
		topicID, err := strconv.Atoi(strings.TrimPrefix(data, "topic_"))
		if err != nil {
			log.Printf("Error parsing topic ID: %v", err)
			return
		}
		b.handleTopicSelection(userID, callbackQuery.Message.Chat.ID, topicID)
	} else if strings.HasPrefix(data, "quality_") {
		// Quality response for spaced repetition
		parts := strings.Split(data, "_")
		if len(parts) != 3 {
			log.Printf("Invalid quality callback format: %s", data)
			return
		}
		
		wordID, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Error parsing word ID: %v", err)
			return
		}
		
		quality, err := strconv.Atoi(parts[2])
		if err != nil {
			log.Printf("Error parsing quality: %v", err)
			return
		}
		
		b.handleQualityResponse(userID, callbackQuery.Message.Chat.ID, wordID, quality)
	} else if strings.HasPrefix(data, "test_") {
		// Test answer selection
		parts := strings.Split(data, "_")
		if len(parts) != 3 {
			log.Printf("Invalid test callback format: %s", data)
			return
		}
		
		wordID, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Printf("Error parsing word ID: %v", err)
			return
		}
		
		answerIndex, err := strconv.Atoi(parts[2])
		if err != nil {
			log.Printf("Error parsing answer index: %v", err)
			return
		}
		
		b.handleTestAnswer(userID, callbackQuery.Message.Chat.ID, wordID, answerIndex)
	} else if data == "settings_topics" {
		// Handle topics settings
		b.handleTopicsSettings(userID, callbackQuery.Message.Chat.ID)
	} else if data == "settings_notification_time" {
		// Handle notification time settings
		b.handleNotificationTimeSettings(userID, callbackQuery.Message.Chat.ID)
	} else if data == "settings_words_per_day" {
		// Handle words per day settings
		b.handleWordsPerDaySettings(userID, callbackQuery.Message.Chat.ID)
	} else if data == "learn" {
		// Handle learn button from stats
		b.handleLearnCommand(&tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: callbackQuery.Message.Chat.ID},
		})
	} else if data == "back_to_settings" {
		// Back to main settings menu
		b.handleSettingsCommand(&tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: callbackQuery.Message.Chat.ID},
		})
	} else if strings.HasPrefix(data, "notify_time_") {
		// Handle notification time selection
		hour, err := strconv.Atoi(strings.TrimPrefix(data, "notify_time_"))
		if err != nil {
			log.Printf("Error parsing notification hour: %v", err)
			return
		}
		b.handleNotificationTimeChange(userID, callbackQuery.Message.Chat.ID, hour)
	} else if data == "toggle_notifications" {
		// Handle toggling notifications on/off
		b.handleToggleNotifications(userID, callbackQuery.Message.Chat.ID)
	} else if strings.HasPrefix(data, "words_per_day_") {
		// Handle words per day selection
		count, err := strconv.Atoi(strings.TrimPrefix(data, "words_per_day_"))
		if err != nil {
			log.Printf("Error parsing words per day: %v", err)
			return
		}
		b.handleWordsPerDayChange(userID, callbackQuery.Message.Chat.ID, count)
	}
}

// registerUserIfNeeded creates a user record if they don't exist
func (b *Bot) registerUserIfNeeded(userID int64, username, firstName, lastName string) {
	userRepo := database.NewUserRepository()
	
	// Check if user exists
	_, err := userRepo.GetByID(userID)
	if err != nil {
		// Create new user
		user := &models.User{
			ID:                  userID,
			Username:            username,
			FirstName:           firstName,
			LastName:            lastName,
			IsAdmin:             b.isAdmin(userID),
			NotificationEnabled: true,
			NotificationHour:    8, // Default to 8 AM
			WordsPerDay:         5, // Default to 5 words per day
		}
		
		if err := userRepo.Create(user); err != nil {
			log.Printf("Error creating user record: %v", err)
		}
	}
}

// Command handlers
func (b *Bot) handleStartCommand(message *tgbotapi.Message) {
	welcomeText := fmt.Sprintf(
		"Welcome to English Words Bot, %s!\n\n"+
			"This bot will help you learn English words using spaced repetition.\n\n"+
			"Commands:\n"+
			"/start - Start the bot\n"+
			"/help - Show help information\n"+
			"/learn - Start learning words\n"+
			"/stats - View your learning statistics\n"+
			"/settings - Configure your preferences\n"+
			"/test - Test your knowledge\n\n"+
			"Let's get started by selecting some topics you're interested in.",
		message.From.FirstName,
	)
	
	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	
	// Add a button to select topics
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Select Topics", "settings_topics"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

func (b *Bot) handleHelpCommand(message *tgbotapi.Message) {
	helpText := "English Words Bot Help\n\n" +
		"Commands:\n" +
		"/start - Start the bot and get an introduction\n" +
		"/help - Show this help message\n" +
		"/learn - Start your daily learning session\n"+
		"/stats - View your learning statistics\n"+
		"/settings - Configure your preferences\n"+
		"/test - Test your knowledge with quizzes\n\n" +
		"How it works:\n" +
		"1. The bot will show you words based on your selected topics\n" +
		"2. After seeing a word, you rate how well you know it\n" +
		"3. Based on your rating, the bot schedules the next review using spaced repetition\n" +
		"4. Words you know well will appear less frequently, while difficult words will appear more often\n\n" +
		"Tips:\n" +
		"- Be honest when rating your knowledge\n" +
		"- Regular practice is key to effective learning\n" +
		"- Use the /test command to verify your progress"
	
	msg := tgbotapi.NewMessage(message.Chat.ID, helpText)
	b.api.Send(msg)
}

func (b *Bot) handleLearnCommand(message *tgbotapi.Message) {
	userID := message.From.ID
	
	// Get user's due words or new words if no due words
	progressRepo := database.NewUserProgressRepository()
	wordRepo := database.NewWordRepository()
	userRepo := database.NewUserRepository()
	
	// Get user preferences
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Get words due for learning
	dueProgress, err := progressRepo.GetDueWordsForUser(userID)
	if err != nil {
		log.Printf("Error getting due words: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Произошла ошибка при получении слов для изучения. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	var wordsToLearn []models.Word
	var isNewWords bool
	
	if len(dueProgress) > 0 {
		// Get the words corresponding to the due progress records
		for _, progress := range dueProgress {
			word, err := wordRepo.GetByID(progress.WordID)
			if err != nil {
				log.Printf("Error getting word %d: %v", progress.WordID, err)
				continue
			}
			wordsToLearn = append(wordsToLearn, *word)
			
			// Limit to user's words per day setting
			if len(wordsToLearn) >= user.WordsPerDay {
				break
			}
		}
	} else {
		// No due words, get new words from user's preferred topics
		isNewWords = true
		
		if len(user.PreferredTopics) == 0 {
			msg := tgbotapi.NewMessage(message.Chat.ID, "У вас нет выбранных тем для изучения. Пожалуйста, выберите темы в настройках (/settings).")
			b.api.Send(msg)
			return
		}
		
		// Get random words from user's preferred topics
		for _, topicID := range user.PreferredTopics {
			// Check if we already have enough words
			if len(wordsToLearn) >= user.WordsPerDay {
				break
			}
			
			// Get words from this topic
			words, err := wordRepo.GetRandomWordsByTopic(topicID, user.WordsPerDay-len(wordsToLearn))
			if err != nil {
				log.Printf("Error getting random words for topic %d: %v", topicID, err)
				continue
			}
			
			// Add to words to learn
			wordsToLearn = append(wordsToLearn, words...)
		}
	}
	
	// Check if we have any words to learn
	if len(wordsToLearn) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "У вас нет слов для изучения сегодня. Попробуйте позже или выберите другие темы в настройках (/settings).")
		b.api.Send(msg)
		return
	}
	
	// Start the learning session
	sessionType := " (новые слова)"
	if !isNewWords {
		sessionType = " для повторения"
	}
	msg := tgbotapi.NewMessage(message.Chat.ID, fmt.Sprintf("Начинаем изучение! У вас %d слов%s.", 
		len(wordsToLearn), 
		sessionType))
	b.api.Send(msg)
	
	// Сохраняем сессию обучения для этого пользователя
	b.learningSessions[userID] = learningSession{
		Words:      wordsToLearn,
		CurrentIdx: 0,
	}
	
	// Show the first word
	b.showWord(message.Chat.ID, userID, wordsToLearn[0])
}

// showWord displays a word with context to the user
func (b *Bot) showWord(chatID int64, _ int64, word models.Word) {
	// Generate example with ChatGPT if available, otherwise use stored context
	var context string
	var translation string
	
	if b.chatGPT != nil {
		// Try to generate a new example with ChatGPT
		context = b.chatGPT.GenerateExampleWithFallback(&word)
	}
	
	if context == "" && word.Context != "" {
		// Use stored context if available
		context = word.Context
	} else if context == "" {
		// Create a simple context if nothing else is available
		context = fmt.Sprintf("Example: The word '%s' is useful in everyday conversations.", word.EnglishWord)
	}
	
	// Get translation of the context if possible
	if b.chatGPT != nil {
		// Try to translate the context
		translation = b.chatGPT.TranslateText(context)
	}
	
	if translation == "" {
		translation = "Перевод недоступен."
	}
	
	// Format the word card with improved visual separation
	wordCard := "*━━━━━━━━━━━━━━━━━━━━━━━*\n\n"
	wordCard += fmt.Sprintf("*🔤 %s* - _%s_\n\n", word.EnglishWord, word.Translation)
	
	// Add pronunciation if available
	if word.Pronunciation != "" {
		wordCard += fmt.Sprintf("📢 *Произношение:* %s\n\n", word.Pronunciation)
	}
	
	// Add context with translation
	wordCard += fmt.Sprintf("📝 *Пример:*\n%s\n\n", context)
	wordCard += fmt.Sprintf("🔄 *Перевод:*\n%s\n\n", translation)
	
	wordCard += "*━━━━━━━━━━━━━━━━━━━━━━━*\n\n"
	wordCard += "Насколько хорошо вы знаете это слово?"
	
	msg := tgbotapi.NewMessage(chatID, wordCard)
	msg.ParseMode = "Markdown"
	
	// Add quality rating buttons
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Не знаю", fmt.Sprintf("quality_%d_%d", word.ID, 0)),
			tgbotapi.NewInlineKeyboardButtonData("⚠️ С трудом", fmt.Sprintf("quality_%d_%d", word.ID, 2)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Помню", fmt.Sprintf("quality_%d_%d", word.ID, 3)),
			tgbotapi.NewInlineKeyboardButtonData("🌟 Хорошо знаю", fmt.Sprintf("quality_%d_%d", word.ID, 5)),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

func (b *Bot) handleStatsCommand(message *tgbotapi.Message) {
	userID := message.From.ID
	
	// Get user progress statistics
	progressRepo := database.NewUserProgressRepository()
	stats, err := progressRepo.GetUserStatistics(userID)
	if err != nil {
		log.Printf("Error getting user statistics: %v", err)
		msg := tgbotapi.NewMessage(message.Chat.ID, "Sorry, there was an error retrieving your statistics. Please try again later.")
		b.api.Send(msg)
		return
	}
	
	// Format statistics
	statsText := "Your Learning Statistics\n\n" +
		fmt.Sprintf("Total words: %d\n", stats["total_words"]) +
		fmt.Sprintf("Due today: %d\n", stats["due_today"]) +
		fmt.Sprintf("Words mastered: %d\n", stats["mastered"]) +
		fmt.Sprintf("Average difficulty: %.2f\n\n", stats["avg_easiness_factor"])
	
	msg := tgbotapi.NewMessage(message.Chat.ID, statsText)
	
	// Add button to start learning
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Start Learning", "learn"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

func (b *Bot) handleSettingsCommand(message *tgbotapi.Message) {
	settingsText := "Settings\n\n" +
		"Configure your learning preferences:"
	
	msg := tgbotapi.NewMessage(message.Chat.ID, settingsText)
	
	// Add settings options
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Topics", "settings_topics"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Notification Time", "settings_notification_time"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Words Per Day", "settings_words_per_day"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

func (b *Bot) handleTestCommand(message *tgbotapi.Message) {
	// This is a placeholder implementation
	// In a real implementation, you would:
	// 1. Select words for testing
	// 2. Create a test session
	// 3. Show the first question
	
	// Create testing module and generate a test
	_ = testing.NewTestingModule()
	
	// Create a random test
	msg := tgbotapi.NewMessage(message.Chat.ID, "Testing your knowledge! Choose the correct translation:")
	b.api.Send(msg)
	
	// Example test question (this would be replaced with actual test data)
	questionText := "What is the translation of *example*?"
	
	msg = tgbotapi.NewMessage(message.Chat.ID, questionText)
	msg.ParseMode = "Markdown"
	
	// Add answer options
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("пример", "test_1_0"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("образец", "test_1_1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("модель", "test_1_2"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

func (b *Bot) handleImportCommand(message *tgbotapi.Message) {
	// Admin-only command for importing words from Excel
	msg := tgbotapi.NewMessage(message.Chat.ID, "To import words from Excel or CSV, please upload a file. The file should contain:\n\n"+
		"For custom format:\n"+
		"- Words structured as: English word,[transcription],translation\n"+
		"- Topic headers like \"Movement,\" or \"Communication,,\"\n\n"+
		"For standard format:\n"+
		"- Column A: English word\n"+
		"- Column B: Translation\n"+
		"- Column C: Context (example sentence)\n"+
		"- Column D: Topic (required)\n"+
		"- Column E: Difficulty (1-5)\n\n"+
		"Upload the file and I'll process it.")
	b.api.Send(msg)
	
	// Add the user to the list of users in import mode
	b.awaitingFileUpload[message.From.ID] = true
}

func (b *Bot) handleAdminStatsCommand(message *tgbotapi.Message) {
	// Admin-only command for viewing system stats
	userRepo := database.NewUserRepository()
	wordRepo := database.NewWordRepository()
	
	// Get counts
	users, _ := userRepo.GetAll()
	words, _ := wordRepo.GetAll()
	
	// Format statistics
	statsText := "System Statistics\n\n" +
		fmt.Sprintf("Total users: %d\n", len(users)) +
		fmt.Sprintf("Total words: %d\n", len(words)) +
		fmt.Sprintf("Server time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	
	msg := tgbotapi.NewMessage(message.Chat.ID, statsText)
	b.api.Send(msg)
}

// Callback handlers
func (b *Bot) handleTopicSelection(userID int64, chatID int64, topicID int) {
	userRepo := database.NewUserRepository()
	
	// Get user
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Sorry, there was an error. Please try again later.")
		b.api.Send(msg)
		return
	}
	
	// Преобразуем int topicID в int64 для совместимости
	topicID64 := int64(topicID)
	
	// Update user's preferred topics
	var updatedTopics []int64
	topicExists := false
	
	for _, id := range user.PreferredTopics {
		if id == topicID64 {
			topicExists = true
			// Skip this topic (removing it)
			continue
		}
		updatedTopics = append(updatedTopics, id)
	}
	
	if !topicExists {
		// Add the topic
		updatedTopics = append(updatedTopics, topicID64)
	}
	
	user.PreferredTopics = updatedTopics
	if err := userRepo.Update(user); err != nil {
		log.Printf("Error updating user topics: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Sorry, there was an error updating your topics. Please try again later.")
		b.api.Send(msg)
		return
	}
	
	// Get topic name
	topicRepo := database.NewTopicRepository()
	topic, err := topicRepo.GetByID(topicID)
	if err != nil {
		log.Printf("Error getting topic %d: %v", topicID, err)
		return
	}
	
	// Send confirmation
	var msgText string
	if topicExists {
		msgText = fmt.Sprintf("Topic '%s' has been removed from your list.", topic.Name)
	} else {
		msgText = fmt.Sprintf("Topic '%s' has been added to your list.", topic.Name)
	}
	
	msg := tgbotapi.NewMessage(chatID, msgText)
	b.api.Send(msg)
}

// handleQualityResponse обрабатывает ответ пользователя о качестве запоминания
func (b *Bot) handleQualityResponse(userID int64, chatID int64, wordID int, quality int) {
	// Применяем алгоритм интервального повторения
	progressRepo := database.NewUserProgressRepository()
	wordRepo := database.NewWordRepository()
	
	// Получаем слово для вывода информации
	word, err := wordRepo.GetByID(wordID)
	if err != nil {
		log.Printf("Error getting word %d: %v", wordID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении информации о слове.")
		b.api.Send(msg)
		return
	}
	
	// Получаем или создаем прогресс для данного слова и пользователя
	progress, err := progressRepo.GetByUserAndWord(userID, wordID)
	if err != nil {
		// Если записи нет, создаем новую
		progress = &models.UserProgress{
			UserID:          userID,
			WordID:          wordID,
			EasinessFactor:  2.5, // Начальное значение
			Interval:        0,
			Repetitions:     0,
			LastQuality:     quality,
			ConsecutiveRight: 0,
			LastReviewDate:  time.Now(),
			NextReviewDate:  time.Now(), // Будет обновлено алгоритмом
		}
	}
	
	// Обновляем запись прогресса с учетом качества ответа
	if quality >= 3 {
		progress.ConsecutiveRight++
	} else {
		progress.ConsecutiveRight = 0
	}
	
	// Сохраняем старый интервал для отображения
	oldInterval := progress.Interval
	
	// Применяем алгоритм SM-2
	b.sm2.Process(progress, spaced_repetition.QualityResponse(quality))
	
	// Сохраняем обновленный прогресс
	if progress.ID == 0 {
		err = progressRepo.Create(progress)
	} else {
		err = progressRepo.Update(progress)
	}
	
	if err != nil {
		log.Printf("Error updating progress: %v", err)
	}
	
	// Определяем эмоджи на основе качества ответа
	qualityEmoji := "❓"
	switch {
	case quality >= 4:
		qualityEmoji = "🌟" // Звезда - отлично знаю
	case quality >= 3:
		qualityEmoji = "✅" // Галочка - помню
	case quality >= 2:
		qualityEmoji = "⚠️" // Предупреждение - с трудом
	default:
		qualityEmoji = "❌" // Крестик - не знаю
	}
	
	// Формируем сообщение с информацией о прогрессе
	responseMsg := "*━━━━━━━━━━━━━━━━━━━━━━━*\n\n"
	responseMsg += fmt.Sprintf("%s *%s* - _%s_\n\n", qualityEmoji, word.EnglishWord, word.Translation)
	
	// Добавляем информацию о прогрессе
	nextDate := progress.NextReviewDate.Format("02.01.2006")
	responseMsg += "*📊 Статистика изучения:*\n"
	
	if oldInterval > 0 {
		responseMsg += fmt.Sprintf("• *Интервал:* %d → %d дн.\n", oldInterval, progress.Interval)
	} else {
		responseMsg += fmt.Sprintf("• *Интервал:* %d дн.\n", progress.Interval)
	}
	
	responseMsg += fmt.Sprintf("• *Повторений:* %d\n", progress.Repetitions)
	responseMsg += fmt.Sprintf("• *Следующее повторение:* %s\n\n", nextDate)
	
	// Добавляем мотивационное сообщение
	responseMsg += "*💬 Результат:* "
	switch {
	case quality >= 4:
		responseMsg += "Отлично! Вы хорошо знаете это слово. 🎉"
	case quality >= 3:
		responseMsg += "Хорошо! Продолжайте практиковать это слово. 👍"
	case quality >= 2:
		responseMsg += "Неплохо. Это слово будет появляться чаще для повторения. 📝"
	default:
		responseMsg += "Понял. Мы будем повторять это слово чаще. 📚"
	}
	
	responseMsg += "\n\n*━━━━━━━━━━━━━━━━━━━━━━━*"
	
	// Если слово изучено хорошо (качество >= 4), проверяем завершение темы
	if quality >= 4 && progress.Repetitions >= 5 {
		// Проверяем, завершил ли пользователь тему, к которой относится слово
		topicStats, err := progressRepo.GetTopicCompletionStats(userID, word.TopicID)
		if err == nil {
			// Если пользователь изучил более 90% слов темы, показываем уведомление
			completionPercentage := topicStats["completion_percentage"].(float64)
			totalWordsInTopic := topicStats["total_words"].(int)
			masteredWords := topicStats["mastered_words"].(int)
			topicName := topicStats["topic_name"].(string)
			
			if completionPercentage >= 90 && totalWordsInTopic > 0 && masteredWords > 0 {
				// Отправляем отдельное сообщение о завершении темы
				topicMsg := fmt.Sprintf("*🏆 Поздравляем!*\n\n"+
					"Вы почти завершили изучение темы *%s*!\n\n"+
					"📊 *Статистика темы:*\n"+
					"• Всего слов: %d\n"+
					"• Изучено: %d\n"+
					"• Завершено: %.1f%%\n\n"+
					"Продолжайте в том же духе! 🌟", 
					topicName, totalWordsInTopic, masteredWords, completionPercentage)
				
				topicCompleteMsg := tgbotapi.NewMessage(chatID, topicMsg)
				topicCompleteMsg.ParseMode = "Markdown"
				b.api.Send(topicCompleteMsg)
			}
		}
	}
	
	msg := tgbotapi.NewMessage(chatID, responseMsg)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
	
	// Проверяем, есть ли еще слова в сессии
	session, exists := b.learningSessions[userID]
	if !exists {
		msg = tgbotapi.NewMessage(chatID, "Сессия обучения завершена.")
		b.api.Send(msg)
		return
	}
	
	// Увеличиваем индекс текущего слова
	session.CurrentIdx++
	
	// Проверяем, есть ли еще слова
	if session.CurrentIdx < len(session.Words) {
		// Обновляем сессию
		b.learningSessions[userID] = session
		
		// Показываем следующее слово
		b.showWord(chatID, userID, session.Words[session.CurrentIdx])
	} else {
		// Если слов больше нет, завершаем сессию
		
		// Сохраняем изученные слова
		completedWords := session.Words
		
		// Удаляем сессию
		delete(b.learningSessions, userID)
		
		// Показываем итоги сессии с текстом на основе изученных слов
		summaryMsg := "*━━━━━━━━━━━━━━━━━━━━━━━*\n\n"
		
		// Генерируем текст на основе изученных слов
		if b.chatGPT != nil && len(completedWords) > 0 {
			englishText, russianText := b.chatGPT.GenerateTextWithWords(completedWords, len(completedWords))
			
			if englishText != "" {
				summaryMsg += "📝 *Практический текст с изученными словами:*\n\n"
				summaryMsg += fmt.Sprintf("_%s_\n\n", englishText)
				
				if russianText != "" {
					summaryMsg += fmt.Sprintf("*Перевод:* %s\n\n", russianText)
				}
			}
		} else {
			// Если ChatGPT недоступен, показываем базовое сообщение
			summaryMsg += "🏆 *Сессия завершена*\n\n"
		}
		
		summaryMsg += "*━━━━━━━━━━━━━━━━━━━━━━━*"
		
		msg = tgbotapi.NewMessage(chatID, summaryMsg)
		msg.ParseMode = "Markdown"
		b.api.Send(msg)
	}
}

func (b *Bot) handleTestAnswer(_ int64, chatID int64, _ int, _ int) {
	// This would check if the answer is correct
	// For now, just say it's correct
	msg := tgbotapi.NewMessage(chatID, "Correct! 🎉")
	b.api.Send(msg)
	
	// Show next question or end test
	msg = tgbotapi.NewMessage(chatID, "Test complete! Your score: 1/1")
	b.api.Send(msg)
}

// downloadFile загружает файл по URL и сохраняет его по указанному пути
func (b *Bot) downloadFile(url string, filepath string) error {
	// Создаем HTTP-клиент с таймаутом
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	
	// Отправляем GET-запрос для загрузки файла
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("плохой статус ответа: %s", resp.Status)
	}
	
	// Создаем файл для записи
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	
	// Копируем содержимое ответа в файл
	_, err = io.Copy(out, resp.Body)
	return err
}

// Вспомогательная функция для форматирования сообщения об импорте
func formatImportReport(result *excel.ImportResult) string {
	// Формируем базовый отчет
	reportText := "✅ Импорт успешно завершен!\n\n"+
		"📊 Статистика импорта:\n"+
		fmt.Sprintf("- Обработано строк: %d\n", result.TotalProcessed)+
		fmt.Sprintf("- Создано тем: %d\n", result.TopicsCreated)+
		fmt.Sprintf("- Добавлено новых слов: %d\n", result.WordsCreated)+
		fmt.Sprintf("- Обновлено существующих слов: %d\n", result.WordsUpdated)
	
	// Проверяем, сколько строк было пропущено
	skippedRows := 0
	for _, errMsg := range result.Errors {
		if strings.Contains(errMsg, "skipping row") {
			skippedRows++
		}
	}
	
	if skippedRows > 0 {
		reportText += fmt.Sprintf("- Пропущено строк (заголовки, пустые): %d\n", skippedRows)
	}
	
	// Фильтруем реальные ошибки (не skipping row)
	var realErrors []string
	for _, errMsg := range result.Errors {
		if !strings.Contains(errMsg, "skipping row") {
			realErrors = append(realErrors, errMsg)
		}
	}
	
	// Показываем предупреждения, если они есть
	if len(realErrors) > 0 {
		reportText += "\n⚠️ Предупреждения при импорте:\n"
		
		// Показываем максимум 10 первых ошибок
		errorsToShow := len(realErrors)
		if errorsToShow > 10 {
			errorsToShow = 10
		}
		
		for i := 0; i < errorsToShow; i++ {
			reportText += "- " + realErrors[i] + "\n"
		}
		
		if len(realErrors) > errorsToShow {
			reportText += fmt.Sprintf("... и еще %d предупреждений\n", len(realErrors)-errorsToShow)
		}
	}
	
	return reportText
}

// handleTopicsSettings shows available topics for selection
func (b *Bot) handleTopicsSettings(userID int64, chatID int64) {
	// Get available topics
	topicRepo := database.NewTopicRepository()
	topics, err := topicRepo.GetAll()
	if err != nil {
		log.Printf("Error getting topics: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении списка тем. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Get user's current topics
	userRepo := database.NewUserRepository()
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Create a map of selected topics for quick lookup
	selectedTopics := make(map[int64]bool)
	for _, topicID := range user.PreferredTopics {
		selectedTopics[topicID] = true
	}
	
	// Create message with topic selection buttons
	msg := tgbotapi.NewMessage(chatID, "Выберите темы для изучения:")
	
	// Build keyboard with topics
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, topic := range topics {
		// Determine label based on whether the topic is selected
		label := topic.Name
		if selectedTopics[topic.ID] {
			label = "✅ " + label
		}
		
		// Add a row for this topic
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("topic_%d", topic.ID)),
		)
		rows = append(rows, row)
	}
	
	// Add a back button
	backButton := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("« Назад к настройкам", "back_to_settings"),
	)
	rows = append(rows, backButton)
	
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.api.Send(msg)
}

// handleNotificationTimeSettings shows notification time settings
func (b *Bot) handleNotificationTimeSettings(userID int64, chatID int64) {
	// Get user's current notification settings
	userRepo := database.NewUserRepository()
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Create message for notification time selection
	msg := tgbotapi.NewMessage(chatID, "Выберите время для ежедневных уведомлений:")
	
	// Build keyboard with time options
	var rows [][]tgbotapi.InlineKeyboardButton
	for hour := 8; hour <= 22; hour += 2 {
		label := fmt.Sprintf("%d:00", hour)
		if user.NotificationHour == hour {
			label = "✅ " + label
		}
		
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("notify_time_%d", hour)),
		)
		rows = append(rows, row)
	}
	
	// Add toggle notifications button
	toggleLabel := "Выключить уведомления"
	if !user.NotificationEnabled {
		toggleLabel = "Включить уведомления"
	}
	toggleRow := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(toggleLabel, "toggle_notifications"),
	)
	rows = append(rows, toggleRow)
	
	// Add a back button
	backButton := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("« Назад к настройкам", "back_to_settings"),
	)
	rows = append(rows, backButton)
	
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.api.Send(msg)
}

// handleWordsPerDaySettings shows words per day settings
func (b *Bot) handleWordsPerDaySettings(userID int64, chatID int64) {
	// Get user's current settings
	userRepo := database.NewUserRepository()
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Create message for words per day selection
	msg := tgbotapi.NewMessage(chatID, "Выберите количество слов для ежедневного изучения:")
	
	// Build keyboard with options
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, count := range []int{3, 5, 7, 10, 15, 20} {
		label := fmt.Sprintf("%d слов", count)
		if user.WordsPerDay == count {
			label = "✅ " + label
		}
		
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("words_per_day_%d", count)),
		)
		rows = append(rows, row)
	}
	
	// Add a back button
	backButton := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("« Назад к настройкам", "back_to_settings"),
	)
	rows = append(rows, backButton)
	
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.api.Send(msg)
}

// handleNotificationTimeChange updates user's notification hour setting
func (b *Bot) handleNotificationTimeChange(userID int64, chatID int64, hour int) {
	userRepo := database.NewUserRepository()
	
	// Get user
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Update notification hour
	user.NotificationHour = hour
	if err := userRepo.Update(user); err != nil {
		log.Printf("Error updating user notification time: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при обновлении времени уведомлений. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Send confirmation and show notification settings again
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Время уведомлений установлено на %d:00", hour))
	b.api.Send(msg)
	
	// Show notification settings again
	b.handleNotificationTimeSettings(userID, chatID)
}

// handleToggleNotifications toggles notifications on/off
func (b *Bot) handleToggleNotifications(userID int64, chatID int64) {
	userRepo := database.NewUserRepository()
	
	// Get user
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Toggle notification setting
	user.NotificationEnabled = !user.NotificationEnabled
	if err := userRepo.Update(user); err != nil {
		log.Printf("Error updating user notification setting: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при обновлении настроек уведомлений. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Send confirmation and show notification settings again
	statusText := "включены"
	if !user.NotificationEnabled {
		statusText = "выключены"
	}
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Уведомления %s", statusText))
	b.api.Send(msg)
	
	// Show notification settings again
	b.handleNotificationTimeSettings(userID, chatID)
}

// handleWordsPerDayChange updates user's words per day setting
func (b *Bot) handleWordsPerDayChange(userID int64, chatID int64, count int) {
	// Обновляем настройки пользователя
	userRepo := database.NewUserRepository()
	
	// Получаем пользователя
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Ошибка при получении пользователя %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "❌ Произошла ошибка при обновлении настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Обновляем количество слов в день
	user.WordsPerDay = count
	if err := userRepo.Update(user); err != nil {
		log.Printf("Ошибка при обновлении настроек пользователя %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "❌ Произошла ошибка при обновлении настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Отправляем подтверждение
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Количество слов в день изменено на <b>%d</b>.", count))
	msg.ParseMode = "HTML"
	b.api.Send(msg)
	
	// Show words per day settings again
	b.handleWordsPerDaySettings(userID, chatID)
} 