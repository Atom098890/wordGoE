package bot

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/example/engbot/internal/ai"
	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/internal/scheduler"
	"github.com/example/engbot/internal/spaced_repetition"
	"github.com/example/engbot/pkg/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Repository represents the interface for accessing data for the bot
type repository interface {
	GetTopicByName(ctx context.Context, name string) (models.Topic, error)
	CreateTopic(ctx context.Context, name string) (int64, error)
	GetWordByWordAndTopicID(ctx context.Context, word string, topicID int64) (models.Word, error)
	CreateWord(ctx context.Context, word models.Word) (int64, error)
	UpdateWord(ctx context.Context, word models.Word) error
}

// learningSession represents a user's ongoing session for learning words
type learningSession struct {
	Words           []models.Word
	CurrentIdx      int
	WordsPerGroup   int
}

// UserState represents the current state of a user in conversation with the bot
type UserState struct {
	State     string
	Timestamp time.Time
	Data      map[string]interface{}
}

// Bot represents the Telegram bot application
type Bot struct {
	api               *tgbotapi.BotAPI
	token             string
	db                interface{}
	repo              repository
	openAiEnabled     bool
	schedulerEnabled  bool
	scheduler         *scheduler.Scheduler
	userStates        map[int64]UserState
	learningSessions  map[int64]learningSession
	adminUserIDs      map[int64]bool
	awaitingFileUpload map[int64]bool
	chatGPT           *ai.ChatGPT
}

// New creates a new bot instance
func New() (*Bot, error) {
	// Получаем токен из переменной окружения
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is not set")
	}
	
	// Используем существующее подключение к базе данных
	// Предполагаем, что database.DB доступна извне
	if database.DB == nil {
		return nil, fmt.Errorf("database connection is not established")
	}
	
	// Проверяем, включен ли OpenAI
	openAiEnabled := os.Getenv("OPENAI_API_KEY") != ""
	var chatGPT *ai.ChatGPT
	
	if openAiEnabled {
		var err error
		chatGPT, err = ai.New()
		if err != nil {
			log.Printf("Warning: Unable to initialize OpenAI client: %v", err)
			openAiEnabled = false
		}
	}
	
	// Проверяем, должен ли быть включен планировщик
	schedulerEnabled := os.Getenv("ENABLE_SCHEDULER") != "false"
	
	// Создаем репозиторий
	repo := &defaultRepository{}
	
	// Создаем экземпляр бота
	bot := &Bot{
		token:             token,
		db:                database.DB,
		repo:              repo,
		openAiEnabled:     openAiEnabled,
		schedulerEnabled:  schedulerEnabled,
		userStates:        make(map[int64]UserState),
		learningSessions:  make(map[int64]learningSession),
		adminUserIDs:      make(map[int64]bool),
		awaitingFileUpload: make(map[int64]bool),
		chatGPT:           chatGPT,
	}
	
	// Загрузка ID администраторов из переменной окружения
	adminIDs := os.Getenv("ADMIN_USER_IDS")
	if adminIDs != "" {
		for _, idStr := range strings.Split(adminIDs, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err != nil {
				log.Printf("Warning: Invalid admin user ID: %s", idStr)
				continue
			}
			bot.adminUserIDs[id] = true
		}
	}
	
	return bot, nil
}

// defaultRepository - простая реализация репозитория по умолчанию
type defaultRepository struct {}

func (r *defaultRepository) GetTopicByName(ctx context.Context, name string) (models.Topic, error) {
	var topic models.Topic
	err := database.DB.QueryRowContext(ctx, "SELECT id, name FROM topics WHERE name = ?", name).
		Scan(&topic.ID, &topic.Name)
	return topic, err
}

func (r *defaultRepository) CreateTopic(ctx context.Context, name string) (int64, error) {
	result, err := database.DB.ExecContext(ctx, "INSERT INTO topics (name) VALUES (?)", name)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *defaultRepository) GetWordByWordAndTopicID(ctx context.Context, word string, topicID int64) (models.Word, error) {
	var w models.Word
	var description, examples, pronunciation sql.NullString
	
	query := `SELECT id, word, translation, description, examples, topic_id, difficulty, pronunciation, 
			 created_at, updated_at FROM words WHERE word = ? AND topic_id = ?`
	err := database.DB.QueryRowContext(ctx, query, word, topicID).
		Scan(&w.ID, &w.Word, &w.Translation, &description, &examples, 
			&w.TopicID, &w.Difficulty, &pronunciation, &w.CreatedAt, &w.UpdatedAt)
	
	if err == nil {
		// Преобразуем NULL значения в пустые строки
		if description.Valid {
			w.Description = description.String
		} else {
			w.Description = ""
		}
		
		if examples.Valid {
			w.Examples = examples.String
		} else {
			w.Examples = ""
		}
		
		if pronunciation.Valid {
			w.Pronunciation = pronunciation.String
		} else {
			w.Pronunciation = ""
		}
	}
	
	return w, err
}

func (r *defaultRepository) CreateWord(ctx context.Context, word models.Word) (int64, error) {
	query := `INSERT INTO words (word, translation, description, examples, topic_id, difficulty, pronunciation) 
			 VALUES (?, ?, ?, ?, ?, ?, ?)`
	result, err := database.DB.ExecContext(ctx, query, word.Word, word.Translation, word.Description, 
								   word.Examples, word.TopicID, word.Difficulty, word.Pronunciation)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (r *defaultRepository) UpdateWord(ctx context.Context, word models.Word) error {
	query := `UPDATE words SET translation = ?, description = ?, examples = ?, 
			 difficulty = ?, pronunciation = ?, updated_at = CURRENT_TIMESTAMP 
			 WHERE id = ?`
	_, err := database.DB.ExecContext(ctx, query, word.Translation, word.Description, 
							  word.Examples, word.Difficulty, word.Pronunciation, word.ID)
	return err
}

// Start initializes and starts the bot
func (b *Bot) Start() error {
	// Initialize the bot with the given token
	botAPI, err := tgbotapi.NewBotAPI(b.token)
	if err != nil {
		return fmt.Errorf("unable to create bot: %v", err)
	}
	
	b.api = botAPI
	log.Printf("Authorized on account %s", botAPI.Self.UserName)
	
	// Set up the update configuration
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	
	// Get updates channel
	updates := b.api.GetUpdatesChan(updateConfig)
	
	// Start goroutine to handle scheduled reminders
	if b.schedulerEnabled {
		go b.scheduleReminders()
	}
	
	// Wait for termination signal in a separate goroutine
	go b.waitForTermination()
	
	// Handle incoming updates
	for update := range updates {
		go b.handleUpdate(update)
	}
	
	return nil
}

// Stop gracefully stops the bot
func (b *Bot) Stop() {
	// Stop the scheduler
	if b.schedulerEnabled && b.scheduler != nil {
		b.scheduler.Stop()
	}
	log.Println("Bot stopped")
}

// scheduleReminders sets up scheduled reminder jobs
func (b *Bot) scheduleReminders() {
	log.Println("Starting reminder scheduler...")
	
	// Создаем планировщик с текущим ботом в качестве Notifier
	b.scheduler = scheduler.New(b)
	
	// Запускаем планировщик
	b.scheduler.Start()
	
	log.Println("Reminder scheduler started successfully")
}

// waitForTermination waits for termination signal and gracefully stops the bot
func (b *Bot) waitForTermination() {
	// Implement signal handling for graceful shutdown
	log.Println("Press Ctrl+C to stop the bot")
}

// SendReminders implements the scheduler.Notifier interface
func (b *Bot) SendReminders(userID int64, count int) error {
	// Проверяем, существует ли пользователь
	userRepo := database.NewUserRepository()
	_, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		return err
	}

	// В Telegram обычно user ID и chat ID одинаковы для личных чатов,
	// но лучше быть явным
	chatID := userID

	// Формируем сообщение с учетом количества слов
	wordForm := "слов"
	if count == 1 {
		wordForm = "слово"
	} else if count > 1 && count < 5 {
		wordForm = "слова"
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("У вас %d %s для повторения! Используйте /learn чтобы начать обучение.", count, wordForm))
	_, err = b.api.Send(msg)
	
	if err != nil {
		log.Printf("Error sending reminder to user %d: %v", userID, err)
	} else {
		log.Printf("Successfully sent reminder to user %d for %d words", userID, count)
	}
	
	return err
}

// isAdmin checks if a user is an admin
func (b *Bot) isAdmin(userID int64) bool {
	return b.adminUserIDs[userID]
}

// handleUpdate processes a single update from Telegram
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	// Handle all update types directly here
	if update.Message != nil {
		userID := update.Message.From.ID
		
		// Register user if needed
		b.registerUserIfNeeded(userID, update.Message.From.UserName, update.Message.From.FirstName, update.Message.From.LastName)
		
		// Check if the user is in a specific state and not sending a command
		if state, exists := b.userStates[userID]; exists && !strings.HasPrefix(update.Message.Text, "/") {
			switch state.State {
			case "waiting_for_word_list":
				b.processWordList(update.Message)
				return
			}
		}
		
		// Handle commands
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				b.handleStartCommand(update.Message)
			case "help":
				b.handleHelpCommand(update.Message)
			case "add":
				b.handleAddWordsCommand(update.Message)
			case "learn":
				b.handleLearnCommand(update.Message)
			case "stats":
				b.handleStatsCommand(update.Message)
			case "settings":
				b.handleSettingsCommand(update.Message)
			case "import":
				// Admin-only command
				if b.isAdmin(userID) {
					b.handleImportCommand(update.Message)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This command is only available for administrators.")
					b.api.Send(msg)
				}
			case "admin_stats":
				// Admin-only command
				if b.isAdmin(userID) {
					b.handleAdminStatsCommand(update.Message)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This command is only available for administrators.")
					b.api.Send(msg)
				}
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command. Use /help to see available commands.")
				b.api.Send(msg)
			}
			return
		}
		
		// Handle regular text messages
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Используйте команду /add для добавления новых слов или /help для получения списка доступных команд.")
		b.api.Send(msg)
	} else if update.CallbackQuery != nil {
		// Acknowledge the callback query
		callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
		b.api.Request(callback)
		
		// Extract data from the callback
		data := update.CallbackQuery.Data
		userID := update.CallbackQuery.From.ID
		chatID := update.CallbackQuery.Message.Chat.ID
		
		// Handle different callback types
		if strings.HasPrefix(data, "topic_") {
			// Topic selection callback
			topicID, err := strconv.Atoi(strings.TrimPrefix(data, "topic_"))
			if err != nil {
				log.Printf("Error parsing topic ID: %v", err)
				return
			}
			b.handleTopicSelection(userID, chatID, topicID)
		} else if data == "settings_topics" {
			// Handle topics settings
			b.handleTopicsSettings(userID, chatID)
		} else if data == "settings_notification_time" {
			// Handle notification time settings
			b.handleNotificationTimeSettings(userID, chatID)
		} else if data == "settings_words_per_day" {
			// Handle words per day settings
			b.handleWordsPerDaySettings(userID, chatID)
		} else if data == "learn" {
			// Handle learn button from stats
			b.handleLearnCommand(&tgbotapi.Message{
				From: &tgbotapi.User{ID: userID},
				Chat: &tgbotapi.Chat{ID: chatID},
			})
		} else if data == "back_to_settings" {
			// Back to main settings menu
			b.handleSettingsCommand(&tgbotapi.Message{
				From: &tgbotapi.User{ID: userID},
				Chat: &tgbotapi.Chat{ID: chatID},
			})
		} else if strings.HasPrefix(data, "notify_time_") {
			// Handle notification time selection
			hour, err := strconv.Atoi(strings.TrimPrefix(data, "notify_time_"))
			if err != nil {
				log.Printf("Error parsing notification hour: %v", err)
				return
			}
			b.handleNotificationTimeChange(userID, chatID, hour)
		} else if data == "toggle_notifications" {
			// Handle toggling notifications on/off
			b.handleToggleNotifications(userID, chatID)
		} else if strings.HasPrefix(data, "words_per_day_") {
			// Handle words per day selection
			count, err := strconv.Atoi(strings.TrimPrefix(data, "words_per_day_"))
			if err != nil {
				log.Printf("Error parsing words per day: %v", err)
				return
			}
			b.handleWordsPerDayChange(userID, chatID, count)
		} else if data == "next_words" {
			// Show next group of words in learning
			b.showNextWordGroup(chatID, userID)
		} else if strings.HasPrefix(data, "words_count_") {
			// Handle words count selection
			count, err := strconv.Atoi(strings.TrimPrefix(data, "words_count_"))
			if err != nil {
				log.Printf("Error parsing words count: %v", err)
				return
			}
			b.handleWordsCountSelection(userID, chatID, count)
		}
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
	userID := message.From.ID
	
	// Register user if not exists
	b.registerUserIfNeeded(userID, message.From.UserName, message.From.FirstName, message.From.LastName)
	
	// Send welcome message with instruction
	welcomeText := "👋 Добро пожаловать в бота для изучения английских слов!\n\n" +
		"🔤 Этот бот поможет вам запоминать английские слова с помощью простой системы карточек.\n\n" +
		"*Как пользоваться ботом:*\n" +
		"1️⃣ Используйте команду /add для добавления новых слов\n\n" +
		"2️⃣ Используйте команду /learn, чтобы начать изучение этих слов\n\n" +
		"3️⃣ Бот будет показывать вам карточки по 5 слов с примерами\n\n" +
		"4️⃣ Для продолжения нажимайте кнопку 'Следующие 5 слов'\n\n" +
		"🔄 Регулярно повторяйте слова для лучшего запоминания!\n\n" +
		"Используйте /help, чтобы увидеть список доступных команд."
	
	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	msg.ParseMode = "Markdown"
	
	b.api.Send(msg)
}

func (b *Bot) handleHelpCommand(message *tgbotapi.Message) {
	helpText := "*Бот для изучения английских слов*\n\n" +
		"*Команды:*\n" +
		"/start - Начать использование бота\n" +
		"/help - Показать это сообщение с помощью\n" +
		"/add - Добавить новые слова вручную\n" +
		"/learn - Начать изучение слов (по 5 слов на карточку)\n" +
		"/stats - Посмотреть статистику обучения\n" +
		"/settings - Настроить параметры\n\n" +
		"*Как это работает:*\n" +
		"1. Отправьте боту список слов через команду /add\n" +
		"2. Запустите команду /learn для изучения\n" +
		"3. Бот будет показывать карточки по 5 слов\n" +
		"4. Кнопка 'Следующие 5 слов' переключает карточки\n\n" +
		"*Советы:*\n" +
		"- Для каждого слова генерируется пример использования\n" +
		"- Регулярное повторение - ключ к эффективному запоминанию\n" +
		"- Используйте выученные слова в речи и на письме"
	
	msg := tgbotapi.NewMessage(message.Chat.ID, helpText)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

func (b *Bot) handleLearnCommand(message *tgbotapi.Message) {
	userID := message.From.ID
	chatID := message.Chat.ID
	
	// Get user's due words or new words if no due words
	progressRepo := database.NewUserProgressRepository()
	wordRepo := database.NewWordRepository()
	userRepo := database.NewUserRepository()
	
	// Get user preferences
	user, err := userRepo.GetByID(userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении ваших настроек. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	// Get words due for learning
	dueProgress, err := progressRepo.GetDueWordsForUser(userID)
	if err != nil {
		log.Printf("Error getting due words: %v", err)
		msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении слов для изучения. Пожалуйста, попробуйте позже.")
		b.api.Send(msg)
		return
	}
	
	var wordsToLearn []models.Word
	var isNewWords bool
	
	if len(dueProgress) > 0 {
		// Get the words corresponding to the due progress records
		// Сохраняем порядок добавления слов
		wordIDs := make([]int, len(dueProgress))
		wordMap := make(map[int]models.Word)
		
		for i, progress := range dueProgress {
			wordIDs[i] = progress.WordID
			word, err := wordRepo.GetByID(progress.WordID)
			if err != nil {
				log.Printf("Error getting word %d: %v", progress.WordID, err)
				continue
			}
			wordMap[progress.WordID] = *word
		}
		
		// Добавляем слова в порядке их ID, чтобы сохранить порядок добавления
		for _, id := range wordIDs {
			if word, ok := wordMap[id]; ok {
				wordsToLearn = append(wordsToLearn, word)
			}
		}
	} else {
		// No due words, get new words from all available or user's preferred topics
		isNewWords = true
		
		// Get all topics if user has no preferred topics
		var topicIDs []int64
		if len(user.PreferredTopics) == 0 {
			topicRepo := database.NewTopicRepository()
			topics, err := topicRepo.GetAll()
			if err != nil {
				log.Printf("Error getting topics: %v", err)
				msg := tgbotapi.NewMessage(chatID, "Произошла ошибка при получении тем. Пожалуйста, попробуйте позже.")
				b.api.Send(msg)
				return
			}
			for _, topic := range topics {
				topicIDs = append(topicIDs, topic.ID)
			}
		} else {
			topicIDs = user.PreferredTopics
		}
		
		// Получаем слова в порядке их добавления (по ID)
		for _, topicID := range topicIDs {
			// Get words from this topic in order of creation
			words, err := wordRepo.GetAll()
			if err != nil {
				log.Printf("Error getting words for topic %d: %v", topicID, err)
				continue
			}
			
			// Add to words to learn
			wordsToLearn = append(wordsToLearn, words...)
		}
	}
	
	// Check if we have any words to learn
	if len(wordsToLearn) == 0 {
		msg := tgbotapi.NewMessage(chatID, "У вас нет слов для изучения. Пожалуйста, добавьте слова, отправив их списком в формате 'слово - перевод'.")
		b.api.Send(msg)
		return
	}
	
	// Start the learning session
	sessionType := "новые слова"
	if !isNewWords {
		sessionType = "слова для повторения"
	}
	
	totalWords := len(wordsToLearn)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Начинаем изучение! У вас %d %s.", 
		totalWords, 
		sessionType))
	b.api.Send(msg)
	
	// Ask user how many words they want to see per session
	askMsg := tgbotapi.NewMessage(chatID, "Выберите, по сколько слов вы хотите изучать за раз:")
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔢 5 слов", "words_count_5"),
			tgbotapi.NewInlineKeyboardButtonData("🔢 10 слов", "words_count_10"),
			tgbotapi.NewInlineKeyboardButtonData("🔢 15 слов", "words_count_15"),
		),
	)
	askMsg.ReplyMarkup = keyboard
	b.api.Send(askMsg)
	
	// Сохраняем сессию обучения для этого пользователя
	b.learningSessions[userID] = learningSession{
		Words:      wordsToLearn,
		CurrentIdx: 0,
		WordsPerGroup: 5, // Default to 5 words per group
	}
}

// showNextWordGroup displays the next group of words (up to 5) for learning
func (b *Bot) showNextWordGroup(chatID int64, userID int64) {
	session, exists := b.learningSessions[userID]
	if !exists {
		log.Printf("No active learning session for user %d", userID)
		return
	}
	
	// Если количество слов не выбрано, используем значение по умолчанию
	if session.WordsPerGroup == 0 {
		session.WordsPerGroup = 5
		b.learningSessions[userID] = session
	}
	
	// Calculate how many words left to show
	wordsLeft := len(session.Words) - session.CurrentIdx
	if wordsLeft <= 0 {
		// End of session
		msg := tgbotapi.NewMessage(chatID, "🎉 Поздравляем! Вы закончили изучение всех слов в этой сессии. Используйте /learn, чтобы начать новую сессию.")
		b.api.Send(msg)
		
		// Clear session
		delete(b.learningSessions, userID)
		return
	}
	
	// Determine how many words to show
	groupSize := session.WordsPerGroup
	if wordsLeft < groupSize {
		groupSize = wordsLeft
	}
	
	// Get the words for this group
	wordGroup := session.Words[session.CurrentIdx:session.CurrentIdx+groupSize]
	
	// Display the words
	b.showWordGroup(chatID, wordGroup, session.CurrentIdx/groupSize+1, (len(session.Words)+groupSize-1)/groupSize)
	
	// Update session index
	session.CurrentIdx += groupSize
	b.learningSessions[userID] = session
	
	// Schedule next review for these words using spaced repetition
	progressRepo := database.NewUserProgressRepository()
	
	// Update progress for each word
	for _, word := range wordGroup {
		// Get or create progress record
		progress, err := progressRepo.GetByUserAndWord(userID, word.ID)
		if err != nil {
			// If record doesn't exist, create a new one with initial interval
			progress = &models.UserProgress{
				UserID:          userID,
				WordID:          word.ID,
				EasinessFactor:  2.5, // Default value
				Interval:        1,   // Start with 1 day
				Repetitions:     0,
				LastQuality:     3, // Assume average quality
				ConsecutiveRight: 0,
				LastReviewDate:  time.Now().Format(time.RFC3339),
				NextReviewDate:  time.Now().AddDate(0, 0, 1).Format(time.RFC3339), // Tomorrow
			}
			
			// Save new progress
			err = progressRepo.Create(progress)
			if err != nil {
				log.Printf("Error creating progress for word %d: %v", word.ID, err)
			}
		} else {
			// Update existing progress
			// Increment repetitions
			progress.Repetitions++
			
			// Use the SM-2 algorithm to determine the next interval
			sm2 := spaced_repetition.NewSM2()
			var nextInterval int
			
			if progress.Repetitions < len(sm2.InitialIntervals) {
				// Use predefined intervals for early repetitions
				nextInterval = sm2.InitialIntervals[progress.Repetitions]
			} else {
				// For later repetitions, double the interval
				nextInterval = progress.Interval * 2
				if nextInterval > sm2.MaxInterval {
					nextInterval = sm2.MaxInterval
				}
			}
			
			// Update progress record
			now := time.Now()
			progress.LastReviewDate = now.Format(time.RFC3339)
			progress.Interval = nextInterval
			progress.NextReviewDate = now.AddDate(0, 0, nextInterval).Format(time.RFC3339)
			
			// Save updated progress
			err = progressRepo.Update(progress)
			if err != nil {
				log.Printf("Error updating progress for word %d: %v", word.ID, err)
			}
		}
	}
}

// showWordGroup displays a group of words in a single card
func (b *Bot) showWordGroup(chatID int64, words []models.Word, _, _ int) {
	var messageText strings.Builder
	
	// Собираем слова для генерации текста
	wordsForExample := make([]string, 0, len(words))
	
	// Process each word and add it to the message
	for i, word := range words {
		// Generate an example using ChatGPT if needed
		example := word.Examples
		if example == "" && b.chatGPT != nil {
			generatedExample, err := b.chatGPT.GenerateExamples(word.Word, 1)
			if err == nil {
				example = generatedExample
			}
		}
		
		// Добавляем слово для генерации примера текста
		wordsForExample = append(wordsForExample, word.Word)
		
		// Word number and main word with pronunciation
		messageText.WriteString(fmt.Sprintf("*%d. %s*", i+1, word.Word))
		if word.Pronunciation != "" {
			messageText.WriteString(fmt.Sprintf(" [%s]", word.Pronunciation))
		}
		messageText.WriteString("\n")
		
		// Проверяем, есть ли формы неправильного глагола
		if word.VerbForms != "" {
			// Прямой вывод сохраненных форм глагола
			messageText.WriteString(fmt.Sprintf("Формы глагола:\n%s\n", word.VerbForms))
		} else if b.chatGPT != nil {
			// Получаем формы глагола
			verbForms, err := b.chatGPT.GenerateIrregularVerbForms(word.Word)
			if err == nil && verbForms != "" && !strings.Contains(verbForms, "Not a verb") {
				// Форматируем вывод форм глагола без "Infinitive:", "Past Simple:" и т.д.
				verbFormsLines := strings.Split(verbForms, "\n")
				var formattedVerbForms strings.Builder
				formattedVerbForms.WriteString("Формы глагола:\n")
				
				for _, line := range verbFormsLines {
					if strings.Contains(line, ":") {
						parts := strings.SplitN(line, ":", 2)
						if len(parts) == 2 {
							formattedVerbForms.WriteString(strings.TrimSpace(parts[1]) + "\n")
						}
					}
				}
				
				messageText.WriteString(formattedVerbForms.String())
			}
		}
		
		// Translation
		messageText.WriteString(fmt.Sprintf("Перевод: ➡️ *%s*\n", word.Translation))
		
		// Example if available
		if example != "" {
			// Extract just the first line of example
			exampleLines := strings.Split(example, "\n")
			if len(exampleLines) > 0 {
				messageText.WriteString(fmt.Sprintf("Пример: ✏️ %s\n", exampleLines[0]))
			}
		}
		
		// Add space between words
		messageText.WriteString("\n")
	}
	
	// Add separator at the bottom
	messageText.WriteString("━━━━━━━━━━━━━━━━━━━━━\n")
	
	// Генерируем текст с использованием изучаемых слов
	if b.chatGPT != nil && len(wordsForExample) > 0 {
		englishText, russianText := b.chatGPT.GenerateTextWithWords(words, len(words))
		if englishText != "" {
			messageText.WriteString("\n*Пример текста с изучаемыми словами:*\n\n")
			messageText.WriteString(fmt.Sprintf("🇬🇧 %s\n\n", englishText))
			if russianText != "" {
				messageText.WriteString(fmt.Sprintf("🇷🇺 %s\n", russianText))
			}
		}
	}
	
	// Create message with markdown
	msg := tgbotapi.NewMessage(chatID, messageText.String())
	msg.ParseMode = "Markdown"
	
	// Send the message
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Error sending word group: %v", err)
	}
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

func (b *Bot) handleImportCommand(message *tgbotapi.Message) {
	// Admin-only command for importing words from Excel
	msg := tgbotapi.NewMessage(message.Chat.ID, "To import words from Excel or CSV, please upload a file. The file should contain:\n\n"+
		"For custom format:\n"+
		"- Words structured as: English word,[transcription],translation\n" +
		"- Topic headers like \"Movement,\" or \"Communication,,\"\n\n"+
		"For standard format:\n"+
		"- Column A: English word\n"+
		"- Column B: Translation\n" +
		"- Column C: Description (example sentence)\n"+
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
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Количество слов в день изменено на %d.", count))
	b.api.Send(msg)
	
	// Show words per day settings again
	b.handleWordsPerDaySettings(userID, chatID)
}

// handleWordsCountSelection обрабатывает выбор количества слов для изучения
func (b *Bot) handleWordsCountSelection(userID int64, chatID int64, count int) {
	// Получаем текущую сессию
	session, exists := b.learningSessions[userID]
	if !exists {
		log.Printf("No active learning session for user %d", userID)
		return
	}
	
	// Сохраняем выбранное количество слов
	userRepo := database.NewUserRepository()
	user, err := userRepo.GetByID(userID)
	if err == nil {
		user.WordsPerDay = count
		userRepo.Update(user)
	}
	
	// Обновляем текущую сессию
	session.WordsPerGroup = count
	b.learningSessions[userID] = session
	
	// Показываем первую группу слов
	b.showNextWordGroup(chatID, userID)
}

// handleAddWordsCommand instructs the user how to add new words
func (b *Bot) handleAddWordsCommand(message *tgbotapi.Message) {
	userId := message.From.ID
	
	// Set user state to waiting for word list
	b.userStates[userId] = UserState{
		State:     "waiting_for_word_list",
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}
	
	instructions := "📝 *Добавление новых слов*\n\n" +
		"Отправьте список слов для добавления в следующем формате:\n\n" +
		"```\n" +
		"слово - перевод\n" +
		"```\n\n" +
		"Чтобы отменить, отправьте /cancel"
	
	msg := tgbotapi.NewMessage(message.Chat.ID, instructions)
	msg.ParseMode = "Markdown"
	b.api.Send(msg)
}

// Process word list sent by the user
func (b *Bot) processWordList(message *tgbotapi.Message) {
	userId := message.From.ID
	text := message.Text
	
	// Remove user from the waiting state
	delete(b.userStates, userId)
	
	lines := strings.Split(text, "\n")
	if len(lines) < 1 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Список слов пуст. Используйте /add для получения инструкций.")
		b.api.Send(msg)
		return
	}
	
	// Используем фиксированную тему "Общие слова"
	topicName := "Общие слова"
	
	// Get or create topic
	topic, err := b.repo.GetTopicByName(context.Background(), topicName)
	if err != nil {
		if err == sql.ErrNoRows {
			// Create new topic
			topicId, err := b.repo.CreateTopic(context.Background(), topicName)
			if err != nil {
				log.Printf("Error creating topic: %v", err)
				msg := tgbotapi.NewMessage(message.Chat.ID, "Ошибка при создании темы. Попробуйте снова позже.")
				b.api.Send(msg)
				return
			}
			topic = models.Topic{ID: topicId}
		} else {
			log.Printf("Error getting topic: %v", err)
			msg := tgbotapi.NewMessage(message.Chat.ID, "Ошибка при получении темы. Попробуйте снова позже.")
			b.api.Send(msg)
			return
		}
	}
	
	// Process word lines
	var addedCount, errorCount int
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		parts := strings.Split(line, "-")
		if len(parts) < 2 {
			errorCount++
			continue
		}
		
		word := strings.TrimSpace(parts[0])
		translation := strings.TrimSpace(parts[1])
		
		if word == "" || translation == "" {
			errorCount++
			continue
		}
		
		var description, examples string
		
		if len(parts) >= 3 {
			description = strings.TrimSpace(parts[2])
		}
		
		if len(parts) >= 4 {
			examples = strings.TrimSpace(parts[3])
		}
		
		// Check if word already exists in this topic
		existingWord, err := b.repo.GetWordByWordAndTopicID(context.Background(), word, topic.ID)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Error checking existing word: %v", err)
			errorCount++
			continue
		}
		
		if err == sql.ErrNoRows {
			// Word doesn't exist, create it
			_, err = b.repo.CreateWord(context.Background(), models.Word{
				Word:        word,
				Translation: translation,
				Description: description,
				Examples:    examples,
				TopicID:     topic.ID,
				Difficulty:  1, // Default difficulty
			})
			
			if err != nil {
				log.Printf("Error creating word: %v", err)
				errorCount++
				continue
			}
		} else {
			// Word exists, update it
			existingWord.Translation = translation
			existingWord.Description = description
			existingWord.Examples = examples
			
			err = b.repo.UpdateWord(context.Background(), existingWord)
			if err != nil {
				log.Printf("Error updating word: %v", err)
				errorCount++
				continue
			}
		}
		
		addedCount++
	}
	
	// Send result message
	var resultMsg string
	if addedCount > 0 {
		resultMsg = fmt.Sprintf("Успешно добавлено/обновлено %d слов в тему '%s'.", addedCount, topicName)
		if errorCount > 0 {
			resultMsg += fmt.Sprintf("\n%d слов не удалось обработать из-за ошибок формата.", errorCount)
		}
	} else {
		resultMsg = "Не удалось добавить ни одного слова. Проверьте формат и попробуйте снова."
	}
	
	msg := tgbotapi.NewMessage(message.Chat.ID, resultMsg)
	b.api.Send(msg)
} 