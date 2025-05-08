package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/example/engbot/pkg/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Constants for callback data
const (
	callbackStartAddTopic = "start_add_topic"
	callbackCancelAction  = "cancel_action"
)

// UserState represents the current state of user interaction
type UserState struct {
	Action    string
	Step      int
	Data      map[string]string
}

var userStates = make(map[int64]*UserState)

// HandleCommand handles bot commands
func (b *Bot) HandleCommand(ctx context.Context, message *tgbotapi.Message) error {
	var err error
	switch message.Command() {
	case "start":
		err = b.handleStart(message)
	case "help":
		err = b.handleHelp(message)
	case "add":
		err = b.handleAddTopic(message)
	case "list":
		err = b.handleListTopics(ctx, message)
	case "delete":
		err = b.handleDeleteTopic(ctx, message)
	case "stats":
		err = b.handleStats(ctx, message)
	case "settings":
		err = b.handleSettings(ctx, message)
	case "notify":
		err = b.handleNotifyCommand(ctx, message)
	case "time":
		err = b.handleTimeCommand(ctx, message)
	default:
		err = b.handleUnknownCommand(message)
	}
	return err
}

func (b *Bot) handleStart(message *tgbotapi.Message) error {
	if message == nil || message.From == nil || message.Chat == nil {
		return fmt.Errorf("invalid message: required fields are missing")
	}

	// Создаем пользователя при первом взаимодействии
	_, err := b.userRepo.GetByTelegramID(context.Background(), message.From.ID)
	if err != nil {
		// Если пользователь не найден, создаем его
		newUser := &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		
		if err = b.userRepo.Create(context.Background(), newUser); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	text := "👋 Добро пожаловать в Spaced Repetition Manager!\n\n" +
		"Я помогу вам эффективно изучать темы с помощью метода интервального повторения.\n\n" +
		"🔹 Как это работает:\n" +
		"1. Добавьте тему для изучения\n" +
		"2. Получайте уведомления о повторении\n" +
		"3. Отмечайте выполненные повторения\n" +
		"4. Отслеживайте свой прогресс"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	return b.sendMessage(msg)
}

func (b *Bot) handleHelp(message *tgbotapi.Message) error {
	text := "📖 Справка по использованию бота\n\n" +
		"🔸 Основные команды:\n" +
		"/start - Запустить бота и показать главное меню\n" +
		"/help - Показать эту справку\n\n" +
		
		"📚 Управление темами:\n" +
		"/add - Добавить новую тему\n" +
		"/list - Показать список всех тем\n" +
		"/delete - Удалить тему\n\n" +
		
		"⚙️ Настройки:\n" +
		"/notify on|off - Включить/выключить уведомления\n" +
		"/time - Установить время уведомлений\n\n" +
		
		"🔄 Интервалы повторения:\n" +
		"1️⃣ Через 1 день\n" +
		"2️⃣ Через 2 дня\n" +
		"3️⃣ Через 3 дня\n" +
		"4️⃣ Через 7 дней\n" +
		"5️⃣ Через 15 дней\n" +
		"6️⃣ Через 25 дней\n" +
		"7️⃣ Через 40 дней\n\n" +
		
		"💡 Советы:\n" +
		"• Регулярно отмечайте выполненные повторения\n" +
		"• Следите за статистикой прогресса\n" +
		"• Настройте удобное время уведомлений"

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = createKeyboard([][]MenuButton{
		{{Text: "⬅️ Вернуться в меню", CallbackData: "main_menu"}},
	})
	return b.sendMessage(msg)
}

func (b *Bot) handleAddTopic(message *tgbotapi.Message) error {
	// Set user state to adding topic
	userStates[message.From.ID] = &UserState{
		Action: "adding_topic",
		Step:   1,
		Data:   make(map[string]string),
	}

	text := "📝 *Добавление новой темы*\n\n" +
		"Пожалуйста, отправьте название темы, которую хотите добавить.\n" +
		"Например: \"Английская грамматика\" или \"Алгоритмы сортировки\""

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = createKeyboard([][]MenuButton{
		{{Text: "❌ Отмена", CallbackData: "cancel_action"}},
	})
	
	return b.sendMessage(msg)
}

func (b *Bot) handleListTopics(ctx context.Context, message *tgbotapi.Message) error {
	if message.From == nil {
		return fmt.Errorf("message.From is nil")
	}

	log.Printf("Listing topics for telegram_id: %d", message.From.ID)

	// Get or create user first
	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil || user == nil {
		log.Printf("User not found or error: %v", err)
		// Create new user if not found
		newUser := &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		
		if err = b.userRepo.Create(ctx, newUser); err != nil {
			log.Printf("Failed to create user: %v", err)
			return fmt.Errorf("failed to create user: %w", err)
		}
		
		// Get the created user to get their ID
		user, err = b.userRepo.GetByTelegramID(ctx, message.From.ID)
		if err != nil {
			log.Printf("Failed to get created user: %v", err)
			return fmt.Errorf("failed to get created user: %w", err)
		}
	}

	if user == nil || user.ID == 0 {
		log.Printf("User is nil or has ID=0")
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: не удалось получить профиль пользователя")
		return b.sendMessage(msg)
	}

	log.Printf("Getting topics for user_id: %d", user.ID)
	topics, err := b.topicRepo.GetAllByUserID(ctx, user.ID)
	if err != nil {
		log.Printf("Failed to get topics: %v", err)
		return fmt.Errorf("failed to get topics: %w", err)
	}

	log.Printf("Found %d topics", len(topics))

	if len(topics) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "У вас пока нет добавленных тем. Нажмите кнопку \"📝 Добавить тему\" чтобы начать.")
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
		return b.sendMessage(msg)
	}

	// Получаем все повторения для пользователя одним запросом
	repetitions, err := b.repetitionRepo.GetDueRepetitions(ctx, user.ID)
	if err != nil {
		log.Printf("Failed to get repetitions: %v", err)
		return fmt.Errorf("failed to get repetitions: %w", err)
	}

	// Создаем мапу для быстрого доступа к повторениям по ID темы
	topicRepetitions := make(map[int64][]models.Repetition)
	for _, rep := range repetitions {
		topicRepetitions[rep.TopicID] = append(topicRepetitions[rep.TopicID], rep)
	}

	var text strings.Builder
	text.WriteString("📋 Ваши темы:\n\n")
	
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for i, topic := range topics {
		// Добавляем информацию о теме
		text.WriteString(fmt.Sprintf("%d. %s\n", i+1, topic.Name))

		// Проверяем, есть ли активные повторения для этой темы
		if reps, ok := topicRepetitions[topic.ID]; ok && len(reps) > 0 {
			text.WriteString("🔄 Требует повторения!\n")
			// Добавляем кнопку для отметки повторения
			button := tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ Повторил тему \"%s\"", topic.Name),
				fmt.Sprintf("complete_%d", reps[0].ID),
			)
			keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{button})
		} else {
			text.WriteString("✅ Нет активных повторений\n")
		}
		text.WriteString("\n")
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text.String())
	if len(keyboard) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	} else {
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	}
	return b.sendMessage(msg)
}

func (b *Bot) handleDeleteTopic(ctx context.Context, message *tgbotapi.Message) error {
	args := message.CommandArguments()
	if args == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите номер темы для удаления: /delete <номер>")
		return b.sendMessage(msg)
	}

	index, err := strconv.Atoi(args)
	if err != nil {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите корректный номер темы")
		return b.sendMessage(msg)
	}

	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	topics, err := b.topicRepo.GetAllByUserID(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get topics: %w", err)
	}

	if index < 1 || index > len(topics) {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Указан неверный номер темы")
		return b.sendMessage(msg)
	}

	topic := topics[index-1]
	if err := b.topicRepo.Delete(ctx, user.ID, topic.ID); err != nil {
		return fmt.Errorf("failed to delete topic: %w", err)
	}

	text := fmt.Sprintf("Тема \"%s\" удалена", topic.Name)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	return b.sendMessage(msg)
}

func (b *Bot) handleStats(ctx context.Context, message *tgbotapi.Message) error {
	// Get user by telegram ID first
	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil || user == nil {
		// Create new user if not found
		newUser := &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		
		if err = b.userRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
		
		// Get the created user to get their ID
		user, err = b.userRepo.GetByTelegramID(ctx, message.From.ID)
		if err != nil {
			return fmt.Errorf("failed to get created user: %w", err)
		}
	}
	
	if user == nil || user.ID == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "❌ Ошибка: не удалось получить профиль пользователя")
		return b.sendMessage(msg)
	}

	stats, err := b.statsRepo.GetUserStatistics(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get statistics: %w", err)
	}

	if len(stats) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "У вас пока нет статистики. Добавьте темы для повторения!")
		return b.sendMessage(msg)
	}

	var text strings.Builder
	text.WriteString("📊 Ваша статистика\n\n")

	for _, stat := range stats {
		completionRate := 0.0
		if stat.TotalRepetitions > 0 {
			completionRate = float64(stat.CompletedRepetitions) / float64(stat.TotalRepetitions) * 100
		}

		text.WriteString(fmt.Sprintf("Тема: %s\n", stat.TopicName))
		text.WriteString(fmt.Sprintf("Всего повторений: %d\n", stat.TotalRepetitions))
		text.WriteString(fmt.Sprintf("Выполнено: %d (%.1f%%)\n\n", stat.CompletedRepetitions, completionRate))
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text.String())
	return b.sendMessage(msg)
}

func (b *Bot) handleSettings(ctx context.Context, message *tgbotapi.Message) error {
	if message.From == nil {
		return fmt.Errorf("message.From is nil")
	}

	// Создаем пользователя при первом взаимодействии
	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		user = &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		err = b.userRepo.Create(ctx, user)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	text := fmt.Sprintf(
		`Текущие настройки:

Уведомления: %s
Время уведомлений: %d:00

Для изменения настроек используйте команды:
/notify on|off - Включить/выключить уведомления
/time <час> - Установить время уведомлений (0-23)`,
		boolToEnabledString(user.NotificationEnabled),
		user.NotificationHour,
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	return b.sendMessage(msg)
}

func (b *Bot) handleNotifyCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.TrimSpace(strings.TrimPrefix(message.Text, "/notify"))
	if args == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите on или off: /notify <on|off>")
		return b.sendMessage(msg)
	}

	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		// Если пользователь не найден, создаем его с дефолтными настройками
		user = &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		err = b.userRepo.Create(ctx, user)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	switch strings.ToLower(args) {
	case "on":
		user.NotificationEnabled = true
	case "off":
		user.NotificationEnabled = false
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите on или off: /notify <on|off>")
		return b.sendMessage(msg)
	}

	err = b.userRepo.Update(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	text := fmt.Sprintf("✅ Уведомления %s", boolToEnabledString(user.NotificationEnabled))
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	return b.sendMessage(msg)
}

func (b *Bot) handleTimeCommand(ctx context.Context, message *tgbotapi.Message) error {
	args := strings.TrimSpace(strings.TrimPrefix(message.Text, "/time"))
	if args == "" {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите час (0-23): /time <час>")
		return b.sendMessage(msg)
	}

	hour, err := strconv.Atoi(args)
	if err != nil || hour < 0 || hour > 23 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "Пожалуйста, укажите корректный час (0-23)")
		return b.sendMessage(msg)
	}

	user, err := b.userRepo.GetByTelegramID(ctx, message.From.ID)
	if err != nil {
		// Если пользователь не найден, создаем его с дефолтными настройками
		user = &models.User{
			TelegramID:          message.From.ID,
			Username:            message.From.UserName,
			FirstName:           message.From.FirstName,
			LastName:            message.From.LastName,
			NotificationEnabled: true,
			NotificationHour:    9,
		}
		err = b.userRepo.Create(ctx, user)
		if err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
	}

	user.NotificationHour = hour
	err = b.userRepo.Update(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	text := fmt.Sprintf("✅ Время уведомлений установлено на %d:00", hour)
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	return b.sendMessage(msg)
}

func (b *Bot) handleUnknownCommand(message *tgbotapi.Message) error {
	text := "Неизвестная команда. Используйте /help для просмотра списка доступных команд."
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	return b.sendMessage(msg)
}

// boolToEnabledString converts a boolean to a human-readable enabled/disabled string
func boolToEnabledString(enabled bool) string {
	if enabled {
		return "включены"
	}
	return "выключены"
}

// CheckDueRepetitions проверяет и отправляет уведомления о повторениях
func (b *Bot) CheckDueRepetitions(ctx context.Context) error {
	currentHour := time.Now().Hour()
	
	// Получаем пользователей, у которых сейчас время уведомлений
	users, err := b.userRepo.GetUsersForNotification(ctx, currentHour)
	if err != nil {
		return fmt.Errorf("failed to get users for notification: %w", err)
	}

	for _, user := range users {
		// Получаем повторения, которые нужно выполнить
		repetitions, err := b.repetitionRepo.GetDueRepetitions(ctx, user.ID)
		if err != nil {
			log.Printf("Failed to get due repetitions for user %d: %v", user.ID, err)
			continue
		}

		if len(repetitions) == 0 {
			continue
		}

		// Получаем темы пользователя
		topics, err := b.topicRepo.GetAllByUserID(ctx, user.ID)
		if err != nil {
			log.Printf("Failed to get topics for user %d: %v", user.ID, err)
			continue
		}

		// Создаем мапу для быстрого доступа к темам
		topicMap := make(map[int64]models.Topic)
		for _, t := range topics {
			topicMap[t.ID] = t
		}

		var text strings.Builder
		text.WriteString("🔔 Напоминание о повторении:\n\n")

		for _, rep := range repetitions {
			text.WriteString(fmt.Sprintf("📚 Тема: %s\n", topicMap[rep.TopicID].Name))
			text.WriteString(fmt.Sprintf("🔄 Повторение №%d\n\n", rep.RepetitionNumber))
		}

		text.WriteString("\nПосле повторения отметьте его как выполненное, нажав на соответствующую кнопку.")

		msg := tgbotapi.NewMessage(user.TelegramID, text.String())
		
		// Добавляем кнопки для каждого повторения
		var keyboard [][]tgbotapi.InlineKeyboardButton
		for _, rep := range repetitions {
			button := tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("✅ Повторил тему \"%s\"", topicMap[rep.TopicID].Name),
				fmt.Sprintf("complete_%d", rep.ID),
			)
			keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{button})
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)

		if err := b.sendMessage(msg); err != nil {
			log.Printf("Failed to send notification to user %d: %v", user.ID, err)
		}
	}

	return nil
}

// HandleCallback обрабатывает нажатия на inline-кнопки
func (b *Bot) HandleCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) error {
	if callback == nil || callback.Message == nil || callback.From == nil {
		return fmt.Errorf("invalid callback data: required fields are missing")
	}

	// Always send an answer to the callback query to remove the loading state
	answer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(answer); err != nil {
		log.Printf("Warning: Failed to answer callback: %v", err)
	}

	message := callback.Message
	var err error

	switch callback.Data {
	case "main_menu":
		err = b.handleMainMenu(callback)
	case "topics_menu":
		err = b.handleTopicsMenu(callback)
	case "settings":
		err = b.handleSettingsMenu(callback)
	case "help":
		err = b.handleHelp(message)
	case "stats":
		err = b.handleStats(ctx, message)
	case "notifications_settings":
		err = b.handleNotificationsSettings(callback)
	case "time_settings":
		err = b.handleTimeSettings(callback)
	case "delete_topic":
		err = b.handleDeleteTopicMenu(callback)
	case "list_topics":
		// Создаем новый Message с правильным From.ID
		msg := &tgbotapi.Message{
			From: callback.From,
			Chat: callback.Message.Chat,
		}
		err = b.handleListTopics(ctx, msg)
	case callbackStartAddTopic:
		err = b.handleStartAddTopic(callback)
	case callbackCancelAction:
		err = b.handleCancelAction(callback)
	default:
		// Обработка complete_* должна идти после точных совпадений
		if strings.HasPrefix(callback.Data, "complete_") {
			repID, err := strconv.ParseInt(strings.TrimPrefix(callback.Data, "complete_"), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid repetition ID in callback data: %w", err)
			}
			if err = b.handleTopicComplete(ctx, callback.From.ID, callback.Message.Chat.ID, repID); err != nil {
				return err
			}
		} else {
			return b.sendMessage(tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ Неизвестное действие"))
		}
	}

	if err != nil {
		errorMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, "❌ Произошла ошибка. Пожалуйста, попробуйте позже.")
		return b.sendMessage(errorMsg)
	}

	return nil
}

func (b *Bot) handleMainMenu(callback *tgbotapi.CallbackQuery, chatID ...int64) error {
	text := "🤖 Главное меню\n\n" +
		"Выберите нужный раздел:\n" +
		"📚 Управление темами - добавление, просмотр и удаление тем\n" +
		"📊 Статистика - ваш прогресс в изучении\n" +
		"⚙️ Настройки - настройка уведомлений\n" +
		"❓ Помощь - информация о командах"

	if callback == nil {
		if len(chatID) == 0 {
			log.Printf("Error: chat ID is required when callback is nil")
			return fmt.Errorf("chat ID is required when callback is nil")
		}
		log.Printf("Sending main menu to chat %d (no callback)", chatID[0])
		msg := tgbotapi.NewMessage(chatID[0], text)
		msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
		return b.sendMessage(msg)
	}

	if callback.Message == nil {
		log.Printf("Error: callback message is nil for user %d", callback.From.ID)
		return fmt.Errorf("callback message is nil")
	}

	log.Printf("Editing message to show main menu for user %d", callback.From.ID)
	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text,
		createKeyboard(b.MainMenuButtons()),
	)
	return b.editMessage(msg)
}

func (b *Bot) handleTopicsMenu(callback *tgbotapi.CallbackQuery) error {
	text := "📚 Управление темами\n\n" +
		"Выберите действие:\n" +
		"📝 Добавить тему - создать новую тему для повторения\n" +
		"📋 Список тем - просмотреть все ваши темы\n" +
		"🗑 Удалить тему - удалить существующую тему"

	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text,
		createKeyboard(b.TopicsMenuButtons()),
	)
	return b.editMessage(msg)
}

func (b *Bot) handleSettingsMenu(callback *tgbotapi.CallbackQuery) error {
	text := "⚙️ Настройки\n\n" +
		"Выберите, что хотите настроить:\n" +
		"🔔 Уведомления - включение/выключение уведомлений\n" +
		"🕒 Время уведомлений - установка времени для напоминаний"

	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text,
		createKeyboard(b.SettingsMenuButtons()),
	)
	return b.editMessage(msg)
}

func (b *Bot) handleNotificationsSettings(callback *tgbotapi.CallbackQuery) error {
	user, err := b.userRepo.GetByTelegramID(context.Background(), callback.From.ID)
	if err != nil {
		return err
	}

	var buttons [][]MenuButton
	if user.NotificationEnabled {
		buttons = [][]MenuButton{
			{{Text: "🔕 Выключить уведомления", CallbackData: "notify_off"}},
		}
	} else {
		buttons = [][]MenuButton{
			{{Text: "🔔 Включить уведомления", CallbackData: "notify_on"}},
		}
	}
	buttons = append(buttons, []MenuButton{{Text: "⬅️ Назад в настройки", CallbackData: "settings_menu"}})

	text := fmt.Sprintf("🔔 Настройки уведомлений\n\n"+
		"Текущий статус: %s\n\n"+
		"Выберите действие:", boolToEnabledString(user.NotificationEnabled))

	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text,
		createKeyboard(buttons),
	)
	return b.editMessage(msg)
}

func (b *Bot) handleTimeSettings(callback *tgbotapi.CallbackQuery) error {
	user, err := b.userRepo.GetByTelegramID(context.Background(), callback.From.ID)
	if err != nil {
		return err
	}

	text := fmt.Sprintf("🕒 Настройка времени уведомлений\n\n"+
		"Текущее время: %d:00\n\n"+
		"Отправьте команду /time <час> для установки нового времени\n"+
		"Пример: /time 9 для установки времени на 9:00", user.NotificationHour)

	buttons := [][]MenuButton{
		{{Text: "⬅️ Назад в настройки", CallbackData: "settings_menu"}},
	}

	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text,
		createKeyboard(buttons),
	)
	return b.editMessage(msg)
}

func (b *Bot) handleDeleteTopicMenu(callback *tgbotapi.CallbackQuery) error {
	// First get the user by Telegram ID
	user, err := b.userRepo.GetByTelegramID(context.Background(), callback.From.ID)
	if err != nil || user == nil {
		log.Printf("Error getting user or user not found: %v", err)
		text := "❌ Ошибка: не удалось получить профиль пользователя"
		buttons := [][]MenuButton{
			{{Text: "⬅️ Назад к темам", CallbackData: "topics_menu"}},
		}
		msg := tgbotapi.NewEditMessageTextAndMarkup(
			callback.Message.Chat.ID,
			callback.Message.MessageID,
			text,
			createKeyboard(buttons),
		)
		return b.editMessage(msg)
	}
	
	// Now use the correct user.ID to get topics
	topics, err := b.topicRepo.GetAllByUserID(context.Background(), user.ID)
	if err != nil {
		log.Printf("Error getting topics: %v", err)
		return err
	}

	if len(topics) == 0 {
		text := "❌ У вас пока нет тем для удаления.\n\nСначала добавьте темы с помощью кнопки \"📝 Добавить тему\""
		buttons := [][]MenuButton{
			{{Text: "⬅️ Назад к темам", CallbackData: "topics_menu"}},
		}

		msg := tgbotapi.NewEditMessageTextAndMarkup(
			callback.Message.Chat.ID,
			callback.Message.MessageID,
			text,
			createKeyboard(buttons),
		)
		return b.editMessage(msg)
	}

	var text strings.Builder
	text.WriteString("🗑 Удаление темы\n\n")
	text.WriteString("Для удаления темы отправьте команду:\n")
	text.WriteString("/delete <номер>\n\n")
	text.WriteString("Ваши темы:\n")

	for i, topic := range topics {
		text.WriteString(fmt.Sprintf("%d. %s\n", i+1, topic.Name))
	}

	buttons := [][]MenuButton{
		{{Text: "⬅️ Назад к темам", CallbackData: "topics_menu"}},
	}

	msg := tgbotapi.NewEditMessageTextAndMarkup(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		text.String(),
		createKeyboard(buttons),
	)
	return b.editMessage(msg)
}

// handleTopicComplete handles the completion of a topic
func (b *Bot) handleTopicComplete(ctx context.Context, userID int64, chatID int64, repID int64) error {
	// Get the repetition
	repetitions, err := b.repetitionRepo.GetByTopicID(ctx, userID, repID)
	if err != nil || len(repetitions) == 0 {
		log.Printf("Error getting repetition: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Ошибка обновления прогресса. Попробуйте позже.")
		return b.sendMessage(msg)
	}

	rep := repetitions[0]

	// Mark current repetition as completed
	rep.Completed = true
	now := time.Now()
	rep.LastReviewDate = &now
	err = b.repetitionRepo.Update(ctx, &rep)
	if err != nil {
		log.Printf("Error updating repetition: %v", err)
		msg := tgbotapi.NewMessage(chatID, "❌ Ошибка обновления прогресса. Попробуйте позже.")
		return b.sendMessage(msg)
	}

	// Schedule next repetition if not the last one
	if rep.RepetitionNumber < 7 {
		nextRep := &models.Repetition{
			UserID:           userID,
			TopicID:          rep.TopicID,
			RepetitionNumber: rep.RepetitionNumber + 1,
			NextReviewDate:   b.repetitionRepo.CalculateNextReviewDate(rep.RepetitionNumber),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		err = b.repetitionRepo.Create(ctx, nextRep)
		if err != nil {
			log.Printf("Error creating next repetition: %v", err)
			msg := tgbotapi.NewMessage(chatID, "❌ Ошибка планирования следующего повторения. Попробуйте позже.")
			return b.sendMessage(msg)
		}

		// Send success message with next repetition date
		text := fmt.Sprintf("✅ Отлично! Повторение выполнено.\nСледующее повторение запланировано на %s",
			nextRep.NextReviewDate.Format("02.01.2006"))
		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	} else {
		// If this was the last repetition
		text := "🎉 Поздравляем! Вы завершили все повторения этой темы!"
		msg := tgbotapi.NewMessage(chatID, text)
		return b.sendMessage(msg)
	}
}

func (b *Bot) handleStartAddTopic(callback *tgbotapi.CallbackQuery) error {
	if callback.Message == nil || callback.From == nil {
		return fmt.Errorf("invalid callback data: Message or From is nil")
	}

	log.Printf("Starting add topic for user %d", callback.From.ID)

	userID := callback.From.ID
	userStates[userID] = &UserState{
		Action: "adding_topic",
		Step:   1,
		Data:   make(map[string]string),
	}

	log.Printf("Set user state: %+v", userStates[userID])

	text := "Пожалуйста, введите название новой темы для повторения.\n" +
		"Например: \"Алгоритмы сортировки\" или \"Паттерны проектирования\""

	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", callbackCancelAction),
		),
	)
	return b.sendMessage(msg)
}

func (b *Bot) handleCancelAction(callback *tgbotapi.CallbackQuery) error {
	if callback.Message == nil || callback.From == nil {
		return fmt.Errorf("invalid callback data: Message or From is nil")
	}

	userID := callback.From.ID
	log.Printf("Canceling action for user %d, previous state: %+v", userID, userStates[userID])
	delete(userStates, userID)
	log.Printf("State after deletion: %+v", userStates[userID])

	text := "Действие отменено. Выберите другую команду:"
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, text)
	msg.ReplyMarkup = createKeyboard(b.MainMenuButtons())
	return b.sendMessage(msg)
} 