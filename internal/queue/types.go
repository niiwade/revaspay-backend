package queue

import (
	"math"
	"math/rand"
	"time"
)

// This file contains types and utilities for the queue package

// RecurringJob represents a recurring job
type RecurringJob struct {
	Name     string      `json:"name"`
	Queue    string      `json:"queue"`
	Payload  interface{} `json:"payload"`
	Schedule string      `json:"schedule"` // Cron expression
	Enabled  bool        `json:"enabled"`
	LastRun  *time.Time  `json:"last_run,omitempty"`
}

// QueueStats represents statistics for a queue
type QueueStats struct {
	Queue      string `json:"queue"`
	Waiting    int    `json:"waiting"`
	Processing int    `json:"processing"`
	Delayed    int    `json:"delayed"`
	Failed     int    `json:"failed"`
	Completed  int    `json:"completed"`
}

// EnqueueOptions represents options for enqueueing a job
type EnqueueOptions struct {
	delay    time.Duration
	maxRetry int
}

// EnqueueOption is a function that modifies EnqueueOptions
type EnqueueOption func(*EnqueueOptions)

// WithDelay adds a delay to a job
func WithDelay(delay time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.delay = delay
	}
}

// WithMaxRetry sets the maximum number of retries for a job
func WithMaxRetry(maxRetry int) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.maxRetry = maxRetry
	}
}

// Note: Default options are now handled directly in the Enqueue method

// calculateBackoff calculates the backoff duration for a retry
func calculateBackoff(retry int) time.Duration {
	// Exponential backoff with jitter
	// Base: 5 seconds
	// Max: 1 hour
	base := 5.0
	max := 3600.0
	
	// Calculate exponential backoff
	seconds := math.Min(max, base*math.Pow(2, float64(retry)))
	
	// Add jitter (±20%)
	jitter := seconds * 0.2
	seconds = seconds - jitter + (rand.Float64() * jitter * 2)
	
	return time.Duration(seconds) * time.Second
}
