package scheduler

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/example/engbot/internal/database"
	"github.com/go-co-op/gocron"
)

// Константы для настроек уведомлений по умолчанию
const (
	DefaultNotificationStartHour = 4  // Время начала уведомлений (8:00)
	DefaultNotificationEndHour   = 18 // Время окончания уведомлений (22:00)
)

// Scheduler manages scheduled tasks for the application
type Scheduler struct {
	scheduler *gocron.Scheduler
	notifier  Notifier
}

// Notifier interface for sending notifications
type Notifier interface {
	SendReminders(userID int64, count int) error
}

// New creates a new scheduler instance
func New(notifier Notifier) *Scheduler {
	s := gocron.NewScheduler(time.UTC)
	return &Scheduler{
		scheduler: s,
		notifier:  notifier,
	}
}

// Start begins running all scheduled tasks
func (s *Scheduler) Start() {
	// Schedule hourly check for users who need notifications
	s.scheduler.Every(1).Hour().Do(s.checkAndSendReminders)
	
	// Start the scheduler in a non-blocking manner
	s.scheduler.StartAsync()
}

// Stop terminates all scheduled tasks
func (s *Scheduler) Stop() {
	s.scheduler.Stop()
}

// checkAndSendReminders checks for users who need reminders and sends them
func (s *Scheduler) checkAndSendReminders() {
	currentHour := time.Now().Hour()
	
	// Используем значения по умолчанию
	startHour := DefaultNotificationStartHour
	endHour := DefaultNotificationEndHour
	
	// Проверяем, задано ли время в переменных окружения
	if startHourStr := os.Getenv("NOTIFICATION_START_HOUR"); startHourStr != "" {
		if h, err := strconv.Atoi(startHourStr); err == nil && h >= 0 && h <= 23 {
			startHour = h
		}
	}
	
	if endHourStr := os.Getenv("NOTIFICATION_END_HOUR"); endHourStr != "" {
		if h, err := strconv.Atoi(endHourStr); err == nil && h >= 0 && h <= 23 {
			endHour = h
		}
	}
	
	// Проверяем, находится ли текущий час в диапазоне времени для отправки уведомлений
	if currentHour < startHour || currentHour > endHour {
		log.Printf("Current hour %d is outside notification hours (%d-%d), skipping reminders", 
			currentHour, startHour, endHour)
		return
	}
	
	// Get user repository
	userRepo := database.NewUserRepository()
	progressRepo := database.NewUserProgressRepository()
	
	// Get users who should receive notifications at the current hour
	users, err := userRepo.GetUsersForNotification(currentHour)
	if err != nil {
		log.Printf("Error getting users for notification: %v", err)
		return
	}
	
	for _, user := range users {
		// Get due words for the user
		dueProgress, err := progressRepo.GetDueWordsForUser(user.ID)
		if err != nil {
			log.Printf("Error getting due words for user %d: %v", user.ID, err)
			continue
		}
		
		// If there are due words, send a reminder
		if len(dueProgress) > 0 {
			// Don't send more than the user's daily preference
			count := len(dueProgress)
			if count > user.WordsPerDay {
				count = user.WordsPerDay
			}
			
			// Send the reminder through the notifier
			if err := s.notifier.SendReminders(user.ID, count); err != nil {
				log.Printf("Error sending reminder to user %d: %v", user.ID, err)
			}
		}
	}
}

// RunManualCheck forces a check for a specific user
func (s *Scheduler) RunManualCheck(userID int64) error {
	// Get repositories
	progressRepo := database.NewUserProgressRepository()
	
	// Get due words for the user
	dueProgress, err := progressRepo.GetDueWordsForUser(userID)
	if err != nil {
		return err
	}
	
	// If there are due words, send a reminder
	if len(dueProgress) > 0 {
		// Send reminder for all due words
		return s.notifier.SendReminders(userID, len(dueProgress))
	}
	
	return nil
} 