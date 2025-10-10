package matcher

import (
	"time"
)

// TimeoutConfig configures various timeouts for task distribution
type TimeoutConfig struct {
	// Default timeouts
	DefaultResponseTimeout time.Duration // Default time to wait for agent response
	DefaultExecutionTimeout time.Duration // Default time for task execution

	// Minimum and maximum limits
	MinResponseTimeout time.Duration // Minimum response timeout (5s)
	MaxResponseTimeout time.Duration // Maximum response timeout (2min)

	// Intent type specific timeouts
	IntentTypeTimeouts map[string]TimeoutSettings

	// Dynamic calculation settings
	DeadlineResponseRatio float64 // What fraction of deadline to use for response (0.1 = 10%)
	UrgencyFactor         float64 // Multiplier for urgent tasks (0.5 = 50% of normal)
}

// TimeoutSettings contains timeout settings for specific intent types
type TimeoutSettings struct {
	ResponseTimeout  time.Duration
	ExecutionTimeout time.Duration
	MaxRetries       int
}

// DefaultTimeoutConfig returns default timeout configuration
func DefaultTimeoutConfig() *TimeoutConfig {
	return &TimeoutConfig{
		DefaultResponseTimeout:  30 * time.Second,
		DefaultExecutionTimeout: 5 * time.Minute,
		MinResponseTimeout:      5 * time.Second,
		MaxResponseTimeout:      2 * time.Minute,
		DeadlineResponseRatio:   0.1,  // Use 10% of deadline for response
		UrgencyFactor:           0.5,  // Urgent tasks get 50% of normal timeout

		IntentTypeTimeouts: map[string]TimeoutSettings{
			"real_time": {
				ResponseTimeout:  5 * time.Second,
				ExecutionTimeout: 30 * time.Second,
				MaxRetries:       1,
			},
			"urgent": {
				ResponseTimeout:  10 * time.Second,
				ExecutionTimeout: 1 * time.Minute,
				MaxRetries:       1,
			},
			"normal": {
				ResponseTimeout:  30 * time.Second,
				ExecutionTimeout: 5 * time.Minute,
				MaxRetries:       2,
			},
			"batch": {
				ResponseTimeout:  60 * time.Second,
				ExecutionTimeout: 30 * time.Minute,
				MaxRetries:       3,
			},
			"long_running": {
				ResponseTimeout:  2 * time.Minute,
				ExecutionTimeout: 1 * time.Hour,
				MaxRetries:       3,
			},
		},
	}
}

// CalculateTimeouts calculates appropriate timeouts for an intent
func (tc *TimeoutConfig) CalculateTimeouts(intentType string, deadline int64, priority string) TimeoutSettings {
	// Start with defaults
	settings := TimeoutSettings{
		ResponseTimeout:  tc.DefaultResponseTimeout,
		ExecutionTimeout: tc.DefaultExecutionTimeout,
		MaxRetries:       2,
	}

	// Check for intent type specific settings
	if typeSettings, exists := tc.IntentTypeTimeouts[intentType]; exists {
		settings = typeSettings
	}

	// Adjust based on deadline if provided
	if deadline > 0 {
		deadlineTime := time.Unix(deadline, 0)
		remaining := time.Until(deadlineTime)

		if remaining > 0 {
			// Calculate response timeout as fraction of remaining time
			calculatedResponse := time.Duration(float64(remaining) * tc.DeadlineResponseRatio)

			// Apply min/max limits
			if calculatedResponse < tc.MinResponseTimeout {
				calculatedResponse = tc.MinResponseTimeout
			} else if calculatedResponse > tc.MaxResponseTimeout {
				calculatedResponse = tc.MaxResponseTimeout
			}

			settings.ResponseTimeout = calculatedResponse

			// Execution timeout is remaining time minus response timeout
			settings.ExecutionTimeout = remaining - calculatedResponse

			// Reduce retries for tight deadlines
			if remaining < 1*time.Minute {
				settings.MaxRetries = 0 // No retries for very tight deadlines
			} else if remaining < 5*time.Minute {
				settings.MaxRetries = 1
			}
		}
	}

	// Adjust based on priority
	switch priority {
	case "critical":
		settings.ResponseTimeout = time.Duration(float64(settings.ResponseTimeout) * 0.3) // 30% of normal
		settings.MaxRetries = 0 // No retries for critical

	case "high":
		settings.ResponseTimeout = time.Duration(float64(settings.ResponseTimeout) * tc.UrgencyFactor)
		if settings.MaxRetries > 1 {
			settings.MaxRetries = 1
		}

	case "low":
		settings.ResponseTimeout = settings.ResponseTimeout * 2
		settings.ExecutionTimeout = settings.ExecutionTimeout * 2
		settings.MaxRetries = 3
	}

	// Final validation
	if settings.ResponseTimeout < tc.MinResponseTimeout {
		settings.ResponseTimeout = tc.MinResponseTimeout
	}
	if settings.ResponseTimeout > tc.MaxResponseTimeout {
		settings.ResponseTimeout = tc.MaxResponseTimeout
	}

	return settings
}

// IsUrgent checks if an intent should be treated as urgent
func (tc *TimeoutConfig) IsUrgent(deadline int64) bool {
	if deadline <= 0 {
		return false
	}

	remaining := time.Until(time.Unix(deadline, 0))
	return remaining < 1*time.Minute && remaining > 0
}