package utils

import (
	"errors"
	"time"
)

// Retry retries the given function up to maxAttempts with exponential backoff.
// If the function returns nil, it stops retrying.
func Retry(maxAttempts int, initialDelay time.Duration, fn func() (any, error)) (any, error) {
	delay := initialDelay
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}
		if attempt < maxAttempts {
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		} else {
			return nil, err
		}
	}
	return nil, errors.New("max attempts reached")
}