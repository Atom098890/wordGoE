package scheduler

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/example/engbot/internal/database"
	"github.com/go-co-op/gocron"
)

// Scheduler manages scheduled tasks for the application
type Scheduler struct {
	scheduler *gocron.Scheduler
	notifier  Notifier
}

// Notifier interface for sending notifications
type Notifier interface {
	SendReminders(userID int64, count int) error
	SendDailyWordsToChannel(channelID int64) error
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
	
	// Schedule daily word delivery to channel - every day at 8 AM UTC
	channelID := getChannelIDFromEnv()
	if channelID != 0 {
		s.scheduler.Every(1).Day().At("08:00").Do(func() {
			log.Println("Sending daily words to channel...")
			if err := s.notifier.SendDailyWordsToChannel(channelID); err != nil {
				log.Printf("Error sending daily words to channel: %v", err)
			}
		})
		log.Printf("Scheduled daily words delivery to channel %d at 8:00 UTC", channelID)
	}
	
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

// getChannelIDFromEnv gets the channel ID from environment variable
func getChannelIDFromEnv() int64 {
	channelIDStr := os.Getenv("TELEGRAM_CHANNEL_ID")
	if channelIDStr == "" {
		log.Println("TELEGRAM_CHANNEL_ID not set, daily channel delivery disabled")
		return 0
	}
	
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		log.Printf("Invalid TELEGRAM_CHANNEL_ID: %v", err)
		return 0
	}
	
	return channelID
} 