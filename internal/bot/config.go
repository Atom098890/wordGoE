package bot

import (
	"time"
)

// BotConfig represents the configuration for the bot
type BotConfig struct {
	// Default number of words to send per batch
	DefaultWordsPerBatch int
	// Default number of repetitions per word
	DefaultRepetitions int
	// Time between sending word batches
	BatchInterval time.Duration
}

// DefaultConfig returns the default bot configuration
func DefaultConfig() *BotConfig {
	return &BotConfig{
		DefaultWordsPerBatch: 10,
		DefaultRepetitions:   5,
		BatchInterval:        time.Hour * 1,
	}
} 