package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/example/engbot/internal/database"
	"github.com/example/engbot/internal/scheduler"
	"github.com/example/engbot/pkg/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func init() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading it: %v", err)
	}
}

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

// Bot represents the Telegram bot instance
type Bot struct {
	api               *tgbotapi.BotAPI
	token             string
	schedulerEnabled  bool
	scheduler         *scheduler.Scheduler
	mu               sync.RWMutex
	
	userRepo          *database.UserRepository
	topicRepo         *database.TopicRepository
	repetitionRepo    *database.RepetitionRepository
	statsRepo         *database.StatisticsRepository
}

// NewBot creates a new bot instance
func NewBot(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot API: %w", err)
	}

	return &Bot{
		api:               api,
		token:             token,
		schedulerEnabled:  os.Getenv("ENABLE_SCHEDULER") != "false",
		mu:               sync.RWMutex{},
		userRepo:          database.NewUserRepository(),
		topicRepo:         database.NewTopicRepository(),
		repetitionRepo:    database.NewRepetitionRepository(),
		statsRepo:         database.NewStatisticsRepository(),
	}, nil
}

// safeGoroutine выполняет функцию в горутине с восстановлением после паники
func safeGoroutine(f func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic in goroutine: %v", r)
				// Можно добавить stack trace для отладки
				debug.PrintStack()
			}
		}()
		f()
	}()
}

// Start initializes and starts the bot
func (b *Bot) Start(ctx context.Context) error {
	log.Println("Starting bot initialization...")

	// Initialize the bot with the given token
	botAPI, err := tgbotapi.NewBotAPI(b.token)
	if err != nil {
		return fmt.Errorf("unable to create bot: %w", err)
	}
	
	b.api = botAPI
	log.Printf("Authorized on account %s (ID: %d)", botAPI.Self.UserName, botAPI.Self.ID)

	// Set up bot commands menu
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "🚀 Запустить бота"},
		{Command: "add", Description: "📝 Добавить новую тему"},
		{Command: "list", Description: "📋 Список всех тем"},
		{Command: "delete", Description: "🗑 Удалить тему"},
		{Command: "stats", Description: "📊 Статистика"},
		{Command: "notify", Description: "🔔 Вкл/выкл уведомления"},
		{Command: "time", Description: "🕒 Время уведомлений"},
		{Command: "help", Description: "❓ Помощь"},
	}

	// Set commands for the menu button
	log.Println("Setting up bot commands menu...")
	cmdConfig := tgbotapi.NewSetMyCommands(commands...)
	if _, err := b.api.Request(cmdConfig); err != nil {
		log.Printf("Warning: Failed to set bot commands menu: %v", err)
	} else {
		log.Printf("Successfully set up bot commands menu")
	}
	
	// Set up the update configuration
	log.Println("Configuring updates...")
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	
	// Get updates channel
	updates := b.api.GetUpdatesChan(updateConfig)
	log.Println("Successfully started receiving updates")
	
	// Start goroutine to handle scheduled reminders
	if b.schedulerEnabled {
		log.Println("Starting scheduler...")
		if err := b.scheduleReminders(ctx); err != nil {
			return fmt.Errorf("failed to schedule reminders: %w", err)
		}
		log.Println("Scheduler started successfully")
	} else {
		log.Println("Scheduler is disabled")
	}
	
	// Wait for termination signal in a separate goroutine
	errChan := make(chan error, 1)
	safeGoroutine(func() {
		errChan <- b.waitForTermination(ctx)
	})
	
	log.Println("Bot is ready to handle messages")
	
	// Handle incoming updates
	for update := range updates {
		select {
		case <-ctx.Done():
			log.Println("Context cancelled, stopping bot...")
			return ctx.Err()
		case err := <-errChan:
			log.Printf("Termination signal received: %v", err)
			return fmt.Errorf("termination error: %w", err)
		default:
			update := update // Create a new variable for the goroutine
			safeGoroutine(func() {
				if err := b.handleUpdate(ctx, update); err != nil {
					log.Printf("Error handling update [ID: %d]: %v", update.UpdateID, err)
				}
			})
		}
	}
	
	log.Println("Update loop finished")
	return nil
}

// Stop gracefully stops the bot
func (b *Bot) Stop(ctx context.Context) error {
	// Stop the scheduler
	if b.schedulerEnabled && b.scheduler != nil {
		b.scheduler.Stop()
	}
	
	log.Println("Bot stopped successfully")
	return nil
}

// scheduleReminders sets up scheduled reminder jobs
func (b *Bot) scheduleReminders(ctx context.Context) error {
	log.Println("Starting reminder scheduler...")
	
	// Create scheduler with current bot as Notifier
	b.scheduler = scheduler.New(b)
	
	// Start scheduler
	if err := b.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}
	
	log.Println("Reminder scheduler started successfully")
	return nil
}

// waitForTermination waits for termination signal and gracefully stops the bot
func (b *Bot) waitForTermination(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// handleUpdate processes incoming updates from Telegram
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) error {
	if update.Message != nil {
		if update.Message.From == nil {
			return nil
		}
		if update.Message.Chat == nil {
			return nil
		}

		log.Printf("Received message from user %d: %s", update.Message.From.ID, update.Message.Text)

		// Handle commands
		if update.Message.IsCommand() {
			return b.HandleCommand(ctx, update.Message)
		}
		
		// Handle text messages based on user state
		if state, exists := userStates[update.Message.From.ID]; exists {
			log.Printf("Found user state: %+v", state)
			switch state.Action {
			case "adding_topic":
				return b.handleAddTopicText(update.Message)
			default:
				log.Printf("Unknown action in user state: %s", state.Action)
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, используйте команды из меню для взаимодействия с ботом.")
				msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
				return b.sendMessage(msg)
			}
		}

		log.Printf("No state found for user %d", update.Message.From.ID)
		// For users without state, show the main menu
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Пожалуйста, используйте команды из меню для взаимодействия с ботом.")
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
		return b.sendMessage(msg)
	} else if update.CallbackQuery != nil {
		// Handle button callbacks
		return b.HandleCallback(ctx, update.CallbackQuery)
	}

	return nil
}

// SendReminders implements the scheduler.Notifier interface
func (b *Bot) SendReminders(userID int64, count int) error {
	ctx := context.Background()
	// Check if user exists
	_, err := b.userRepo.GetByTelegramID(ctx, userID)
	if err != nil {
		log.Printf("Error getting user %d: %v", userID, err)
		return err
	}

	chatID := userID

	// Format message based on word count
	wordForm := "слов"
	if count == 1 {
		wordForm = "слово"
	} else if count > 1 && count < 5 {
		wordForm = "слова"
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("У вас %d %s для повторения! Откройте список тем, чтобы начать повторение.", count, wordForm))
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	return b.sendMessage(msg)
}

// MainMenuButtons returns the buttons for the main menu
func (b *Bot) MainMenuButtons() [][]MenuButton {
	buttons := [][]MenuButton{
		{
			{Text: "📚 Управление темами", CallbackData: "topics_menu"},
		},
		{
			{Text: "📊 Статистика", CallbackData: "stats"},
			{Text: "⚙️ Настройки", CallbackData: "settings"},
		},
		{
			{Text: "❓ Помощь", CallbackData: "help"},
		},
	}
	return buttons
}

// TopicsMenuButtons returns the buttons for the topics submenu
func (b *Bot) TopicsMenuButtons() [][]MenuButton {
	buttons := [][]MenuButton{
		{
			{Text: "📝 Добавить тему", CallbackData: callbackStartAddTopic},
			{Text: "📋 Список тем", CallbackData: "list_topics"},
		},
		{
			{Text: "🗑 Удалить тему", CallbackData: "delete_topic"},
		},
		{
			{Text: "⬅️ Назад в меню", CallbackData: "main_menu"},
		},
	}
	return buttons
}

// SettingsMenuButtons returns the buttons for the settings submenu
func (b *Bot) SettingsMenuButtons() [][]MenuButton {
	buttons := [][]MenuButton{
		{
			{Text: "🔔 Уведомления", CallbackData: "notifications_settings"},
			{Text: "🕒 Время уведомлений", CallbackData: "time_settings"},
		},
		{
			{Text: "⬅️ Назад в меню", CallbackData: "main_menu"},
		},
	}
	return buttons
}

// sendMessage sends a message with proper error handling
func (b *Bot) sendMessage(msg tgbotapi.MessageConfig) error {
	// Validate and clean message text
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return fmt.Errorf("cannot send empty message: message text is required")
	}

	// Add main menu buttons if no other markup is specified
	if msg.ReplyMarkup == nil {
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	}

	// Set the cleaned text back
	msg.Text = text

	// Try to send message
	_, err := b.api.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

// editMessage edits a message with proper error handling
func (b *Bot) editMessage(msg tgbotapi.EditMessageTextConfig) error {
	// Validate and clean message text
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return fmt.Errorf("cannot edit with empty message: message text is required")
	}

	// Set the cleaned text back
	msg.Text = text

	// Try to edit message
	_, err := b.api.Send(msg)
	if err != nil {
		// If editing fails, try sending a new message
		newMsg := tgbotapi.NewMessage(msg.ChatID, text)
		if msg.ReplyMarkup != nil {
			newMsg.ReplyMarkup = msg.ReplyMarkup
		} else {
			// Add main menu buttons if no other markup is specified
			newMsg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
		}
		return b.sendMessage(newMsg)
	}
	return nil
}

func (b *Bot) handleAddTopicText(message *tgbotapi.Message) error {
	if message == nil || message.From == nil || message.Chat == nil {
		return fmt.Errorf("invalid message: missing required fields")
	}

	topicName := strings.TrimSpace(message.Text)
	if topicName == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Название темы не может быть пустым. Пожалуйста, отправьте название темы или нажмите кнопку \"Отмена\".")
		return b.sendMessage(msg)
	}

	ctx := context.Background()

	// Создаем или получаем пользователя
	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil || user == nil {
		// Создаем нового пользователя
		newUser := &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}

		if err := b.userRepo.Create(ctx, newUser); err != nil {
			log.Printf("Ошибка создания пользователя: %v", err)
			return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать профиль пользователя. Попробуйте еще раз."))
		}

		// Получаем созданного пользователя для получения его ID
		user, err = b.userRepo.GetByTelegramID(ctx, message.From.ID)
		if err != nil {
			log.Printf("Ошибка получения пользователя после создания: %v", err)
			return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при получении данных пользователя. Попробуйте еще раз."))
		}

		if user == nil || user.ID == 0 {
			log.Printf("Пользователь не найден или ID = 0 для telegram_id %d", message.From.ID)
			return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: не удалось получить профиль пользователя"))
		}
	}

	// Создаем тему
	topic := &models.Topic{
		Name:      topicName,
		UserID:    user.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := b.topicRepo.Create(ctx, topic); err != nil {
		log.Printf("Ошибка создания темы для пользователя %d (telegram_id %d): %v", user.ID, message.From.ID, err)
		return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать тему. Попробуйте еще раз."))
	}

	if topic.ID == 0 {
		log.Printf("Тема создана, но ID = 0 для пользователя %d", user.ID)
		return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка при создании темы"))
	}

	// Создаем статистику для темы
	stats := &models.Statistics{
		UserID:  user.ID,
		TopicID: topic.ID,
	}
	if err := b.statsRepo.Create(ctx, stats); err != nil {
		log.Printf("Ошибка создания статистики: %v", err)
		// Если не удалось создать статистику, удаляем тему
		if delErr := b.topicRepo.Delete(ctx, user.ID, topic.ID); delErr != nil {
			log.Printf("Ошибка удаления темы после неудачного создания статистики: %v", delErr)
		}
		return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось создать тему. Попробуйте еще раз."))
	}

	// Создаем первое повторение
	repetition := &models.Repetition{
		UserID:           user.ID,
		TopicID:          topic.ID,
		RepetitionNumber: 1,
		NextReviewDate:   time.Now().Add(24 * time.Hour),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := b.repetitionRepo.Create(ctx, repetition); err != nil {
		log.Printf("Ошибка создания повторения: %v", err)
		// Если не удалось создать повторение, удаляем тему
		if delErr := b.topicRepo.Delete(ctx, user.ID, topic.ID); delErr != nil {
			log.Printf("Ошибка удаления темы после неудачного создания повторения: %v", delErr)
		}
		return b.sendMessage(tgbotapi.NewMessage(message.Chat.ID, "❌ Не удалось запланировать повторения. Попробуйте еще раз."))
	}

	// Очищаем состояние пользователя
	delete(userStates, message.From.ID)

	// Отправляем сообщение об успехе
	text := fmt.Sprintf("✅ Тема \"%s\" успешно добавлена!\n\nТеперь вы можете:", topic.Name) +
		"\n1. Добавить еще одну тему" +
		"\n2. Посмотреть список всех тем" +
		"\n3. Вернуться в главное меню"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = createKeyboard([][]MenuButton{
		{{Text: "📝 Добавить тему", CallbackData: callbackStartAddTopic}},
		{{Text: "📋 Список тем", CallbackData: "list_topics"}},
		{{Text: "⬅️ В меню", CallbackData: "main_menu"}},
	})
	return b.sendMessage(msg)
} 