package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"runtime/debug"

	"github.com/example/engbot/internal/database"
	"github.com/robfig/cron/v3"
)

// Константы для настроек уведомлений по умолчанию
const (
	DefaultNotificationStartHour = 4  // Время начала уведомлений (8:00)
	DefaultNotificationEndHour   = 18 // Время окончания уведомлений (22:00)
)

// Scheduler manages scheduled tasks for the application
type Scheduler struct {
	cron     *cron.Cron
	notifier Notifier
}

// Notifier interface for sending notifications
type Notifier interface {
	SendReminders(userID int64, count int) error
}

// New creates a new scheduler instance
func New(notifier Notifier) *Scheduler {
	c := cron.New(cron.WithSeconds())
	return &Scheduler{
		cron:     c,
		notifier: notifier,
	}
}

// Start begins running all scheduled tasks
func (s *Scheduler) Start(ctx context.Context) error {
	// Schedule hourly check for users who need notifications
	_, err := s.cron.AddFunc("0 0 * * * *", func() { s.checkAndSendReminders(ctx) })
	if err != nil {
		return fmt.Errorf("failed to schedule reminders: %w", err)
	}
	
	// Start the scheduler in a non-blocking manner
	s.cron.Start()

	// Wait for context cancellation
	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return nil
}

// Stop terminates all scheduled tasks
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// checkAndSendReminders checks for users who need reminders and sends them
func (s *Scheduler) checkAndSendReminders(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic in checkAndSendReminders: %v", r)
			debug.PrintStack()
		}
	}()

	log.Println("Starting reminder check...")

	// Get current hour
	currentHour := time.Now().Hour()
	log.Printf("Checking reminders for hour: %d", currentHour)

	// Get users who should receive notifications at this hour
	userRepo := database.NewUserRepository()
	users, err := userRepo.GetUsersForNotification(ctx, currentHour)
	if err != nil {
		log.Printf("Error getting users for notification: %v", err)
		return
	}

	if len(users) == 0 {
		log.Printf("No users to notify at hour %d", currentHour)
		return
	}

	log.Printf("Found %d users to notify", len(users))

	// Get repetitions for each user
	repetitionRepo := database.NewRepetitionRepository()
	for _, user := range users {
		log.Printf("Processing reminders for user %d", user.ID)

		// Get due repetitions for user
		repetitions, err := repetitionRepo.GetDueRepetitions(ctx, user.ID)
		if err != nil {
			log.Printf("Error getting due repetitions for user %d: %v", user.ID, err)
			continue
		}

		if len(repetitions) == 0 {
			log.Printf("No due repetitions for user %d", user.ID)
			continue
		}

		log.Printf("Found %d due repetitions for user %d", len(repetitions), user.ID)

		// Send notification
		if err := s.notifier.SendReminders(user.TelegramID, len(repetitions)); err != nil {
			log.Printf("Error sending reminder to user %d: %v", user.ID, err)
			continue
		}

		log.Printf("Successfully sent reminder to user %d", user.ID)
	}

	log.Println("Reminder check completed")
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