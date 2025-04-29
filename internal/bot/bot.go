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

// MenuButton represents a button in the menu
type MenuButton struct {
	Text         string
	CallbackData string
}

// createKeyboard creates a keyboard from menu buttons
func createKeyboard(buttons [][]MenuButton) tgbotapi.InlineKeyboardMarkup {
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for _, row := range buttons {
		var keyboardRow []tgbotapi.InlineKeyboardButton
		for _, button := range row {
			keyboardRow = append(keyboardRow, tgbotapi.NewInlineKeyboardButtonData(button.Text, button.CallbackData))
		}
		keyboard = append(keyboard, keyboardRow)
	}
	return tgbotapi.NewInlineKeyboardMarkup(keyboard...)
}

// Repository represents the interface for accessing data for the bot
type repository interface {
	GetTopicByName(ctx context.Context, name string) (models.Topic, error)
	CreateTopic(ctx context.Context, name string) (int64, error)
	GetWordByWordAndTopicID(ctx context.Context, word string, topicID int64) (models.Word, error)
	CreateWord(ctx context.Context, word models.Word) (int64, error)
	UpdateWord(ctx context.Context, word models.Word) error
	GetNextWords(ctx context.Context, limit int) ([]models.Word, error)
	MarkWordAsLearned(ctx context.Context, wordID int64, userID int64) error
	GetUserStats(ctx context.Context, userID int64) (int, error)
	GetUserConfig(ctx context.Context, userID int64) (*database.UserConfig, error)
	UpdateUserConfig(ctx context.Context, config *database.UserConfig) error
	UpdateLastBatchTime(ctx context.Context, userID int64) error
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
	config            *BotConfig
	userConfigs       map[int64]*database.UserConfig
}

// New creates a new bot instance
func New() (*Bot, error) {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is not set")
	}
	
	if database.DB == nil {
		return nil, fmt.Errorf("database connection is not established")
	}
	
	openAiEnabled := os.Getenv("OPENAI_API_KEY") != ""
	var chatGPT *ai.ChatGPT
	
	if openAiEnabled {
		chatGPT = ai.NewChatGPT(os.Getenv("OPENAI_API_KEY"))
		if chatGPT == nil {
			log.Printf("Warning: Unable to initialize OpenAI client")
			openAiEnabled = false
		}
	}
	
	schedulerEnabled := os.Getenv("ENABLE_SCHEDULER") != "false"
	
	repo := &defaultRepository{}
	
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
		config:            DefaultConfig(),
		userConfigs:       make(map[int64]*database.UserConfig),
	}
	
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

// defaultRepository implements the repository interface
type defaultRepository struct{}

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
		if description.Valid {
			w.Description = description.String
		}
		if examples.Valid {
			w.Examples = examples.String
		}
		if pronunciation.Valid {
			w.Pronunciation = pronunciation.String
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

func (r *defaultRepository) GetNextWords(ctx context.Context, limit int) ([]models.Word, error) {
	query := `SELECT id, word, translation, description, examples, topic_id, difficulty, pronunciation
			 FROM words 
			 WHERE id NOT IN (SELECT word_id FROM learned_words)
			 ORDER BY id ASC
			 LIMIT ?`
	
	rows, err := database.DB.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var words []models.Word
	for rows.Next() {
		var w models.Word
		var description, examples, pronunciation sql.NullString
		
		err := rows.Scan(&w.ID, &w.Word, &w.Translation, &description, &examples,
			&w.TopicID, &w.Difficulty, &pronunciation)
		if err != nil {
			return nil, err
		}
		
		if description.Valid {
			w.Description = description.String
		}
		if examples.Valid {
			w.Examples = examples.String
		}
		if pronunciation.Valid {
			w.Pronunciation = pronunciation.String
		}
		
		words = append(words, w)
	}
	
	return words, nil
}

func (r *defaultRepository) MarkWordAsLearned(ctx context.Context, wordID int64, userID int64) error {
	query := `INSERT INTO learned_words (word_id, user_id) VALUES (?, ?)`
	_, err := database.DB.ExecContext(ctx, query, wordID, userID)
	return err
}

func (r *defaultRepository) GetUserStats(ctx context.Context, userID int64) (int, error) {
	var count int
	err := database.DB.QueryRowContext(ctx, 
		"SELECT COUNT(*) FROM learned_words WHERE user_id = ?", userID).Scan(&count)
	return count, err
}

func (r *defaultRepository) GetUserConfig(ctx context.Context, userID int64) (*database.UserConfig, error) {
	return database.GetUserConfig(ctx, userID)
}

func (r *defaultRepository) UpdateUserConfig(ctx context.Context, config *database.UserConfig) error {
	return database.UpdateUserConfig(ctx, config)
}

func (r *defaultRepository) UpdateLastBatchTime(ctx context.Context, userID int64) error {
	return database.UpdateLastBatchTime(ctx, userID)
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

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("У вас %d %s для повторения! Нажмите кнопку Start Learning, чтобы начать обучение.", count, wordForm))
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

// handleUpdate handles incoming updates from Telegram
func (b *Bot) handleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		// Handle messages
		if update.Message.IsCommand() {
			switch update.Message.Command() {
			case "start":
				b.handleStartCommand(update.Message)
			case "menu":
				b.showMainMenu(update.Message.Chat.ID)
			case "add":
				b.handleAddWordsCommand(update.Message)
			case "stats":
				b.handleStatsCommand(update.Message)
			case "settings":
				b.handleSettingsCommand(update.Message)
			case "import":
				// Admin-only command
				if b.isAdmin(update.Message.From.ID) {
					b.handleImportCommand(update.Message)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This command is only available for administrators.")
					msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
					b.api.Send(msg)
				}
			case "admin_stats":
				// Admin-only command
				if b.isAdmin(update.Message.From.ID) {
					b.handleAdminStatsCommand(update.Message)
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "This command is only available for administrators.")
					msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
					b.api.Send(msg)
				}
			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Unknown command. Use /menu to show the main menu.")
				msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
				b.api.Send(msg)
			}
		} else if b.awaitingFileUpload[update.Message.Chat.ID] {
			// Handle file upload
			if update.Message.Document != nil {
				b.processWordList(update.Message)
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Please send words in text format.")
				msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
				b.api.Send(msg)
			}
		} else {
			// Handle settings input or word list input
			userID := update.Message.From.ID
			state, exists := b.userStates[userID]
			if exists {
				switch state.State {
				case "waiting_for_word_list":
					b.processWordList(update.Message)
				case "waiting_for_words_per_batch":
					// Parse the number from user input
					count, err := strconv.Atoi(strings.TrimSpace(update.Message.Text))
					if err != nil || count < 1 || count > 20 {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, введите число от 1 до 20.")
						b.api.Send(msg)
						return
					}
					b.handleWordsCountSelection(userID, update.Message.Chat.ID, count)
					delete(b.userStates, userID) // Clear the state after handling
				default:
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "I don't understand. Use /menu to show the main menu.")
					msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
					b.api.Send(msg)
				}
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "I don't understand. Use /menu to show the main menu.")
				msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
				b.api.Send(msg)
			}
		}
	} else if update.CallbackQuery != nil {
		// Handle callback queries from buttons
		b.handleCallbackQuery(update.CallbackQuery)
	}
}

// handleStartCommand handles the /start command
func (b *Bot) handleStartCommand(message *tgbotapi.Message) {
	welcomeText := `Welcome to English Learning Bot! 🎓

Available commands:
/menu - Show main menu
/add - Add new words
/stats - Show your statistics
/settings - Configure your preferences`

	msg := tgbotapi.NewMessage(message.Chat.ID, welcomeText)
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	b.api.Send(msg)
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
		msg := tgbotapi.NewMessage(chatID, "🎉 Поздравляем! Вы закончили изучение всех слов в этой сессии. Нажмите Start Learning, чтобы начать новую сессию.")
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
		msg := tgbotapi.NewMessage(message.Chat.ID, "Статистика пока недоступна. Начните изучение слов, чтобы увидеть свой прогресс!")
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
		b.api.Send(msg)
		return
	}
	
	// Format statistics
	statsText := "📊 *Ваша статистика*\n\n" +
		fmt.Sprintf("Всего слов в системе: %d\n", stats["total_words"]) +
		fmt.Sprintf("Слов в процессе изучения: %d\n", stats["words_in_progress"]) +
		fmt.Sprintf("Слов на сегодня: %d\n", stats["due_today"]) +
		fmt.Sprintf("Изучено полностью: %d\n", stats["mastered"])
	
	if stats["words_in_progress"].(int) > 0 {
		statsText += fmt.Sprintf("Средняя сложность: %.2f\n", stats["avg_easiness_factor"])
	}
	
	msg := tgbotapi.NewMessage(message.Chat.ID, statsText)
	msg.ParseMode = "Markdown"
	
	// Add button to start learning
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎯 Начать изучение", "start_learning"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Назад в меню", "main_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

// handleSettingsCommand shows the settings menu
func (b *Bot) handleSettingsCommand(message *tgbotapi.Message) {
	settingsText := "⚙️ Settings\n\nConfigure your learning preferences:"
	
	msg := tgbotapi.NewMessage(message.Chat.ID, settingsText)
	msg.ParseMode = "Markdown"
	
	// Add settings options
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔢 Words per batch", "set_words_per_batch"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⏰ Notification time", "notification_time"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Back to Menu", "main_menu"),
		),
	)
	msg.ReplyMarkup = keyboard
	
	b.api.Send(msg)
}

// handleWordsPerBatchSettings shows words per batch settings
func (b *Bot) handleWordsPerBatchSettings(userID int64, chatID int64) {
	// Get user's current settings
	config, err := b.repo.GetUserConfig(context.Background(), userID)
	if err != nil || config == nil {
		// Create default config if it doesn't exist
		config = &database.UserConfig{
			UserID:        userID,
			WordsPerBatch: b.config.DefaultWordsPerBatch,
			Repetitions:   b.config.DefaultRepetitions,
			IsActive:      true,
		}
		err = b.repo.UpdateUserConfig(context.Background(), config)
		if err != nil {
			log.Printf("Error creating user config: %v", err)
			msg := tgbotapi.NewMessage(chatID, "❌ Error creating your settings. Please try again.")
			b.api.Send(msg)
			return
		}
	}

	// Create message for words per batch selection
	msg := tgbotapi.NewMessage(chatID, "Choose how many words you want to learn per batch:")

	// Build keyboard with options
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, count := range []int{3, 5, 7, 10, 15, 20} {
		label := fmt.Sprintf("%d words", count)
		if config.WordsPerBatch == count {
			label = "✅ " + label
		}
		
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("words_per_batch_%d", count)),
		)
		rows = append(rows, row)
	}

	// Add a back button
	backButton := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("« Back to Settings", "settings"),
	)
	rows = append(rows, backButton)

	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.api.Send(msg)
}

// handleWordsPerBatchChange updates user's words per batch setting
func (b *Bot) handleWordsPerBatchChange(userID int64, chatID int64, count int) {
	// First, ensure user exists
	userRepo := database.NewUserRepository()
	if _, err := userRepo.GetByID(userID); err != nil {
		// User doesn't exist, create them
		user := &models.User{
			ID:                  userID,
			NotificationEnabled: true,
			NotificationHour:    9, // Default notification hour
			WordsPerDay:         10, // Default words per day
			CreatedAt:          time.Now().Format(time.RFC3339),
			UpdatedAt:          time.Now().Format(time.RFC3339),
		}
		err = userRepo.Create(user)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			msg := tgbotapi.NewMessage(chatID, "❌ Error updating settings. Please try again.")
			b.api.Send(msg)
			return
		}
	}

	// Get or create user config
	config := &database.UserConfig{
		UserID:        userID,
		WordsPerBatch: count,
		Repetitions:   5, // Default repetitions
		IsActive:      true,
	}

	err := b.repo.UpdateUserConfig(context.Background(), config)
	if err != nil {
		log.Printf("Error updating user config: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Error updating settings. Please try again.")
		b.api.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Words per batch set to %d", count))
	b.api.Send(msg)

	// Show settings menu again
	b.handleSettingsCommand(&tgbotapi.Message{
		From: &tgbotapi.User{ID: userID},
		Chat: &tgbotapi.Chat{ID: chatID},
	})
}

func (b *Bot) handleImportCommand(message *tgbotapi.Message) {
	// Admin-only command for importing words from Excel
	msg := tgbotapi.NewMessage(message.Chat.ID, "Please send words in text format instead, using the format:\nword - translation\n\nExample:\nhello - привет\nworld - мир")
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
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

// handleCallbackQuery handles callback queries from buttons
func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery) {
	userID := callback.From.ID
	chatID := callback.Message.Chat.ID

	switch callback.Data {
	case "main_menu":
		b.showMainMenu(chatID)
	case "start_learning":
		if err := b.handleStartLearning(chatID, userID); err != nil {
			log.Printf("Error handling start learning: %v", err)
		}
	case "show_stats":
		b.handleStatsCommand(&tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: chatID},
		})
	case "load_words":
		b.handleLoadWords(chatID)
	case "add_words_text":
		b.userStates[userID] = UserState{
			State:     "waiting_for_word_list",
			Timestamp: time.Now(),
		}
		msg := tgbotapi.NewMessage(chatID, "Send me your list of words in the format:\n"+
			"word - translation\n\n"+
			"Example:\n"+
			"hello - привет\n"+
			"world - мир")
		b.api.Send(msg)
	case "settings":
		b.handleSettingsCommand(&tgbotapi.Message{
			From: &tgbotapi.User{ID: userID},
			Chat: &tgbotapi.Chat{ID: chatID},
		})
	case "notification_time":
		b.handleNotificationTimeSettings(userID, chatID)
	case "set_words_per_batch":
		b.handleWordsPerBatchSettings(userID, chatID)
	default:
		// Check if it's a notification time change
		if strings.HasPrefix(callback.Data, "set_notification_time_") {
			hourStr := strings.TrimPrefix(callback.Data, "set_notification_time_")
			hour, err := strconv.Atoi(hourStr)
			if err != nil {
				log.Printf("Error parsing notification hour: %v", err)
				return
			}
			b.handleNotificationTimeChange(userID, chatID, hour)
			return
		}
		
		// Check if it's a words per batch change
		if strings.HasPrefix(callback.Data, "words_per_batch_") {
			countStr := strings.TrimPrefix(callback.Data, "words_per_batch_")
			count, err := strconv.Atoi(countStr)
			if err != nil {
				log.Printf("Error parsing words per batch: %v", err)
				return
			}
			b.handleWordsPerBatchChange(userID, chatID, count)
			return
		}
	}
}

// showMainMenu shows the main menu
func (b *Bot) showMainMenu(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Main Menu - choose an option:")
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	b.api.Send(msg)
}

// MainMenuButtons returns the buttons for the main menu
func (b *Bot) MainMenuButtons() [][]MenuButton {
	return [][]MenuButton{
		{
			{Text: "🎯 Start Learning", CallbackData: "start_learning"},
			{Text: "📊 Statistics", CallbackData: "show_stats"},
		},
		{
			{Text: "📝 Load Words", CallbackData: "load_words"},
			{Text: "⚙️ Settings", CallbackData: "settings"},
		},
	}
}

// processWordList processes the uploaded word list
func (b *Bot) processWordList(message *tgbotapi.Message) {
	// Remove user from waiting state
	delete(b.awaitingFileUpload, message.Chat.ID)
	delete(b.userStates, message.From.ID)

	// Ensure default topic exists
	defaultTopic, err := b.repo.GetTopicByName(context.Background(), "General")
	if err != nil {
		// Create default topic if it doesn't exist
		defaultTopicID, err := b.repo.CreateTopic(context.Background(), "General")
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Error: Could not create default topic")
			msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
			b.api.Send(msg)
			return
		}
		defaultTopic.ID = defaultTopicID
	}

	// Split the message into lines
	lines := strings.Split(message.Text, "\n")
	
	// Process each line
	var addedWords, skippedWords int
	var errorMsgs []string
	
	for _, line := range lines {
		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		
		// Split line into word and translation
		parts := strings.Split(line, "-")
		if len(parts) != 2 {
			errorMsgs = append(errorMsgs, fmt.Sprintf("Invalid format: %s", line))
			continue
		}
		
		word := strings.TrimSpace(parts[0])
		translation := strings.TrimSpace(parts[1])
		
		if word == "" || translation == "" {
			errorMsgs = append(errorMsgs, fmt.Sprintf("Empty word or translation: %s", line))
			continue
		}

		// Create word in database
		newWord := models.Word{
			Word:        word,
			Translation: translation,
			TopicID:    defaultTopic.ID, // Use the default topic ID
			Difficulty: 1, // Default difficulty
			CreatedAt:  time.Now().Format(time.RFC3339),
			UpdatedAt:  time.Now().Format(time.RFC3339),
		}

		// Check if word already exists
		existingWord, err := b.repo.GetWordByWordAndTopicID(context.Background(), word, newWord.TopicID)
		if err == nil && existingWord.ID != 0 {
			// Word exists, update it
			existingWord.Translation = translation
			existingWord.UpdatedAt = time.Now().Format(time.RFC3339)
			err = b.repo.UpdateWord(context.Background(), existingWord)
			if err != nil {
				errorMsgs = append(errorMsgs, fmt.Sprintf("Error updating word '%s': %v", word, err))
				continue
			}
			skippedWords++
		} else {
			// Word doesn't exist, create it
			_, err = b.repo.CreateWord(context.Background(), newWord)
			if err != nil {
				errorMsgs = append(errorMsgs, fmt.Sprintf("Error adding word '%s': %v", word, err))
				continue
			}
			addedWords++
		}
	}
	
	// Prepare result message
	var resultMsg strings.Builder
	resultMsg.WriteString(fmt.Sprintf("✅ Words processed:\n"+
		"- Added: %d\n"+
		"- Updated: %d\n", addedWords, skippedWords))
	
	if len(errorMsgs) > 0 {
		resultMsg.WriteString(fmt.Sprintf("\n❌ Errors (%d):\n", len(errorMsgs)))
		for _, errMsg := range errorMsgs {
			resultMsg.WriteString("- " + errMsg + "\n")
		}
	}
	
	resultMsg.WriteString("\nНажмите Start Learning, чтобы начать изучение слов!")
	
	msg := tgbotapi.NewMessage(message.Chat.ID, resultMsg.String())
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	b.api.Send(msg)
}

// sendWordBatch sends a batch of words to the user
func (b *Bot) sendWordBatch(chatID int64, words []models.Word, batchNumber int) error {
	if len(words) == 0 {
		msg := tgbotapi.NewMessage(chatID, "No words in this batch.")
		_, err := b.api.Send(msg)
		return err
	}

	// Format words message
	var messageText strings.Builder
	messageText.WriteString(fmt.Sprintf("📚 Batch %d\n\n", batchNumber))

	// Collect words for generating a combined example text
	wordsForExample := make([]string, 0, len(words))

	for i, word := range words {
		// Add word to the list for example generation
		wordsForExample = append(wordsForExample, word.Word)

		// Word number and main word with pronunciation
		messageText.WriteString(fmt.Sprintf("*%d. %s*", i+1, word.Word))
		if word.Pronunciation != "" {
			messageText.WriteString(fmt.Sprintf(" [%s]", word.Pronunciation))
		}
		messageText.WriteString("\n\n")

		// Generate verb forms if needed
		if b.chatGPT != nil {
			verbForms, err := b.chatGPT.GenerateIrregularVerbForms(word.Word)
			if err == nil && verbForms != "" && !strings.Contains(verbForms, "Not a verb") {
				messageText.WriteString("*Verb forms:*\n")
				// Split verb forms into lines and format each line
				forms := strings.Split(verbForms, "\n")
				for _, form := range forms {
					if strings.TrimSpace(form) != "" {
						messageText.WriteString(form + "\n")
					}
				}
				messageText.WriteString("\n")
			}
		}

		// Translation
		messageText.WriteString(fmt.Sprintf("Перевод: ➡️ *%s*\n\n", word.Translation))

		// Generate example using ChatGPT if needed
		if b.chatGPT != nil {
			example, err := b.chatGPT.GenerateExamples(word.Word, 1)
			if err == nil && example != "" {
				// Take only the first sentence from the example
				sentences := strings.Split(example, ".")
				if len(sentences) > 0 {
					firstSentence := strings.TrimSpace(sentences[0])
					if firstSentence != "" {
						messageText.WriteString(fmt.Sprintf("✏️ *Example:* %s.\n\n", firstSentence))
					}
				}
			}
		}

		// Add separator between words
		messageText.WriteString("━━━━━━━━━━━━━━━━━━━━━\n\n")
	}

	// Generate a text using all words
	if b.chatGPT != nil && len(wordsForExample) > 0 {
		englishText, russianText := b.chatGPT.GenerateTextWithWords(words, len(words))
		if englishText != "" {
			messageText.WriteString("\n*Text using these words:*\n\n")
			messageText.WriteString(fmt.Sprintf("🇬🇧 %s\n\n", englishText))
			if russianText != "" {
				messageText.WriteString(fmt.Sprintf("🇷🇺 %s\n", russianText))
			}
		}
	}

	msg := tgbotapi.NewMessage(chatID, messageText.String())
	msg.ParseMode = "Markdown"

	_, err := b.api.Send(msg)
	return err
}

// handleStartLearning starts the learning process for a user
func (b *Bot) handleStartLearning(chatID int64, userID int64) error {
	// Get user config or create default
	config, err := b.repo.GetUserConfig(context.Background(), userID)
	if err != nil || config == nil {
		config = &database.UserConfig{
			UserID:        userID,
			WordsPerBatch: b.config.DefaultWordsPerBatch,
			Repetitions:   b.config.DefaultRepetitions,
			IsActive:      true,
		}
		err = b.repo.UpdateUserConfig(context.Background(), config)
		if err != nil {
			return fmt.Errorf("failed to create user config: %v", err)
		}
	}

	// Check if it's time for a new batch
	now := time.Now()
	if config.LastBatchTime.Valid {
		lastBatch := config.LastBatchTime.Time
		nextBatch := lastBatch.Add(b.config.BatchInterval)
		if now.Before(nextBatch) {
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
				"Your next batch will be available at %s. Keep practicing the current words! 🎯",
				nextBatch.Format("15:04")))
			b.api.Send(msg)
			return nil
		}
	}

	// Get next words for learning
	words, err := b.repo.GetNextWords(context.Background(), config.WordsPerBatch)
	if err != nil {
		return fmt.Errorf("failed to get words: %v", err)
	}

	if len(words) == 0 {
		msg := tgbotapi.NewMessage(chatID, "No more words to learn! Please load more words.")
		_, err = b.api.Send(msg)
		return err
	}

	// Check if previous batch was completed
	if config.LastBatchTime.Valid {
		completedDays := int(now.Sub(config.LastBatchTime.Time).Hours() / 24)
		if completedDays >= config.Repetitions {
			// Mark previous batch words as learned
			for _, word := range b.learningSessions[userID].Words {
				err = b.repo.MarkWordAsLearned(context.Background(), int64(word.ID), userID)
				if err != nil {
					log.Printf("Error marking word %d as learned: %v", word.ID, err)
				}
			}
		}
	}

	// Start new learning session
	b.learningSessions[userID] = learningSession{
		Words:      words,
		CurrentIdx: 0,
	}

	// Update last batch time
	config.LastBatchTime = sql.NullTime{Time: now, Valid: true}
	err = b.repo.UpdateLastBatchTime(context.Background(), userID)
	if err != nil {
		log.Printf("Error updating last batch time: %v", err)
	}

	return b.sendWordBatch(chatID, words, 1)
}

// handleLoadWords shows word loading instructions
func (b *Bot) handleLoadWords(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "📝 *Load New Words*\n\n"+
		"Send your list of words in the format:\n"+
		"word - translation\n\n"+
		"Example:\n"+
		"hello - привет\n"+
		"world - мир\n")

	// Create keyboard with options
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 Add Words", "add_words_text"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("« Back to Menu", "main_menu"),
		),
	)

	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

// handleNotificationTimeSettings displays notification time options
func (b *Bot) handleNotificationTimeSettings(userID int64, chatID int64) {
	config, err := b.repo.GetUserConfig(context.Background(), userID)
	if err != nil || config == nil {
		// Create default config if it doesn't exist
		config = &database.UserConfig{
			UserID:          userID,
			WordsPerBatch:   b.config.DefaultWordsPerBatch,
			Repetitions:     b.config.DefaultRepetitions,
			IsActive:        true,
			NotificationHour: 9, // Default notification hour
		}
		err = b.repo.UpdateUserConfig(context.Background(), config)
		if err != nil {
			log.Printf("Error creating user config: %v", err)
			msg := tgbotapi.NewMessage(chatID, "❌ Error creating your settings. Please try again.")
			b.api.Send(msg)
			return
		}
	}

	var keyboard [][]tgbotapi.InlineKeyboardButton
	timeOptions := []int{9, 12, 15, 18, 21}
	
	for _, hour := range timeOptions {
		var text string
		if hour == config.NotificationHour {
			text = fmt.Sprintf("✓ %d:00", hour)
		} else {
			text = fmt.Sprintf("%d:00", hour)
		}
		
		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(text, fmt.Sprintf("set_notification_time_%d", hour)),
		})
	}
	
	// Add back button
	keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("⬅️ Back to Settings", "settings"),
	})

	msg := tgbotapi.NewMessage(chatID, "🕒 Choose when you want to receive new words:")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	b.api.Send(msg)
}

// handleNotificationTimeChange updates user's notification time setting
func (b *Bot) handleNotificationTimeChange(userID int64, chatID int64, hour int) {
	// First, ensure user exists
	userRepo := database.NewUserRepository()
	if _, err := userRepo.GetByID(userID); err != nil {
		// User doesn't exist, create them
		user := &models.User{
			ID:                  userID,
			NotificationEnabled: true,
			NotificationHour:    hour, // Set the requested hour
			WordsPerDay:         10, // Default words per day
			CreatedAt:          time.Now().Format(time.RFC3339),
			UpdatedAt:          time.Now().Format(time.RFC3339),
		}
		err = userRepo.Create(user)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			msg := tgbotapi.NewMessage(chatID, "❌ Error updating settings. Please try again.")
			b.api.Send(msg)
			return
		}
	}

	// Get or create user config
	config := &database.UserConfig{
		UserID:          userID,
		WordsPerBatch:   5, // Default words per batch
		Repetitions:     5, // Default repetitions
		IsActive:        true,
		NotificationHour: hour,
	}

	err := b.repo.UpdateUserConfig(context.Background(), config)
	if err != nil {
		log.Printf("Error updating user config: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Error updating settings. Please try again.")
		b.api.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ Notification time set to %d:00", hour))
	b.api.Send(msg)

	// Show updated notification time settings
	b.handleNotificationTimeSettings(userID, chatID)
}

// handleWordsCountSelection обрабатывает выбор количества слов для изучения
func (b *Bot) handleWordsCountSelection(userID int64, chatID int64, count int) {
	// Получаем текущую сессию
	session, exists := b.learningSessions[userID]
	if !exists {
		log.Printf("No active learning session for user %d", userID)
		return
	}
	
	// Сохраняем выбранное количество слов в конфигурации пользователя
	config, err := b.repo.GetUserConfig(context.Background(), userID)
	if err != nil {
		log.Printf("Error getting user config: %v", err)
		return
	}
	
	config.WordsPerBatch = count
	if err := b.repo.UpdateUserConfig(context.Background(), config); err != nil {
		log.Printf("Error updating user config: %v", err)
		return
	}
	
	// Обновляем текущую сессию
	session.WordsPerGroup = count
	b.learningSessions[userID] = session
	
	// Показываем первую группу слов
	b.showNextWordGroup(chatID, userID)
} 