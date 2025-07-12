package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RedisClient implements the Queue interface using Redis
type RedisClient struct {
	client       *redis.Client
	db           *gorm.DB
	ctx          context.Context
	handlers     map[JobType]JobHandler
	retryHandler *RetryHandler
}

// Redis key prefixes
const (
	queuePrefix      = "queue:"
	processingPrefix = "processing:"
	delayedPrefix    = "delayed:"
	recurringPrefix  = "recurring:"
	failedPrefix     = "failed:"
	completedPrefix  = "completed:"
)

// updateRecurringJobType updates the recurring job type field to use JobType

// NewRedisClient creates a new Redis client
func NewRedisClient(client *redis.Client, db *gorm.DB) *RedisClient {
	ctx := context.Background()
	r := &RedisClient{
		client:   client,
		db:       db,
		ctx:      ctx,
		handlers: make(map[JobType]JobHandler),
	}
	
	// Create a Queue wrapper to use with the retry handler
	queueWrapper := &Queue{
		db:       db,
		handlers: make(map[JobType]JobHandler),
	}
	
	// Create retry handler with reference to the queue wrapper
	r.retryHandler = NewRetryHandler(db, queueWrapper)
	
	// Start processing delayed jobs
	go r.processDelayedJobs()
	
	return r
}

// RegisterHandler registers a handler for a job type
func (r *RedisClient) RegisterHandler(jobType JobType, handler JobHandler) {
	r.handlers[jobType] = handler
}

// Close closes the Redis client
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// Enqueue adds a job to the queue
func (r *RedisClient) Enqueue(jobType JobType, payload interface{}, opts ...EnqueueOption) (string, error) {
	// Apply options
	options := &EnqueueOptions{
		delay:    0,
		maxRetry: 3, // Default max retries
	}
	
	for _, opt := range opts {
		opt(options)
	}

	// Convert payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job payload: %w", err)
	}
	
	// Create job record
	jobID := uuid.New()
	job := Job{
		ID:         jobID,
		Type:       jobType,
		Payload:    payloadBytes,
		Status:     JobStatusPending,
		RetryCount: 0,
		MaxRetries: options.maxRetry,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Save job to database
	result := r.db.Create(&job)
	if result.Error != nil {
		return "", result.Error
	}
	
	// If there's a delay, add to delayed queue
	if options.delay > 0 {
		return r.enqueueDelayed(job, options.delay)
	}
	
	// Otherwise add to normal queue
	return r.enqueueImmediate(job)
}

// enqueueImmediate adds a job to the immediate queue
func (r *RedisClient) enqueueImmediate(job Job) (string, error) {
	// Convert to JSON for Redis
	data, err := json.Marshal(map[string]interface{}{
		"id":   job.ID.String(),
		"type": string(job.Type),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}
	
	// Add to Redis queue
	queueName := queuePrefix + string(job.Type)
	if err := r.client.LPush(r.ctx, queueName, data).Err(); err != nil {
		return "", fmt.Errorf("failed to add job to queue: %w", err)
	}
	
	return job.ID.String(), nil
}

// enqueueDelayed adds a job to the delayed queue
func (r *RedisClient) enqueueDelayed(job Job, delay time.Duration) (string, error) {
	// Calculate when the job should be processed
	processAt := time.Now().Add(delay)
	
	// Add to delayed set
	delayedQueue := delayedPrefix + string(job.Type)
	score := float64(processAt.Unix())
	
	// Store job ID in delayed queue
	if err := r.client.ZAdd(r.ctx, delayedQueue, &redis.Z{
		Score:  score,
		Member: job.ID.String(),
	}).Err(); err != nil {
		return "", fmt.Errorf("failed to add job to delayed queue: %w", err)
	}
	
	return job.ID.String(), nil
}

// Dequeue gets a job from the queue
func (r *RedisClient) Dequeue(queueName string, timeout time.Duration) (*Job, error) {
	// Format queue name
	queueKey := queuePrefix + queueName
	
	// Pop a job from the queue with a timeout
	result, err := r.client.BRPop(r.ctx, timeout, queueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No jobs in queue
		}
		return nil, fmt.Errorf("error popping job from queue %s: %w", queueName, err)
	}
	
	if len(result) < 2 {
		return nil, fmt.Errorf("invalid result from BRPOP for queue %s", queueName)
	}

	// Parse job data
	jobData := result[1]
	var jobInfo map[string]string
	if err := json.Unmarshal([]byte(jobData), &jobInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job data: %w", err)
	}
	
	// Get job ID
	jobID, ok := jobInfo["id"]
	if !ok {
		return nil, fmt.Errorf("job data missing ID")
	}
	
	// Parse UUID
	jobUUID, err := uuid.Parse(jobID)
	if err != nil {
		return nil, fmt.Errorf("invalid job ID: %w", err)
	}
	
	// Get job from database
	var job Job
	if err := r.db.First(&job, "id = ?", jobUUID).Error; err != nil {
		return nil, fmt.Errorf("failed to find job %s: %w", jobID, err)
	}
	
	// Update job status to processing
	if err := r.db.Model(&job).Updates(map[string]interface{}{
		"status":     JobStatusProcessing,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update job status: %w", err)
	}
	
	// Add to processing set
	processingKey := processingPrefix + queueName
	if err := r.client.HSet(r.ctx, processingKey, job.ID.String(), time.Now().String()).Err(); err != nil {
		log.Printf("Warning: failed to add job to processing set: %v", err)
	}

	return &job, nil
}

// processDelayedJobs processes delayed jobs that are ready to be executed
func (r *RedisClient) processDelayedJobs() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		// Process all job types
		for jobType := range r.handlers {
			r.moveDelayedJobs(string(jobType))
		}
	}
}

// moveDelayedJobs moves delayed jobs that are ready to be executed to the main queue
func (r *RedisClient) moveDelayedJobs(jobType string) {
	delayedQueue := delayedPrefix + jobType
	queueName := queuePrefix + jobType
	now := float64(time.Now().Unix())
	
	// Get all jobs that are ready to be executed
	jobs, err := r.client.ZRangeByScore(r.ctx, delayedQueue, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%f", now),
	}).Result()
	
	if err != nil {
		log.Printf("Error getting delayed jobs: %v", err)
		return
	}
	
	if len(jobs) == 0 {
		return
	}
	
	// Move each job to the main queue
	for _, jobID := range jobs {
		// Get job from database
		jobUUID, err := uuid.Parse(jobID)
		if err != nil {
			log.Printf("Invalid job ID: %v", err)
			continue
		}
		
		var job Job
		if err := r.db.First(&job, "id = ?", jobUUID).Error; err != nil {
			log.Printf("Failed to find job %s: %v", jobID, err)
			continue
		}
		
		// Add to main queue
		data, err := json.Marshal(map[string]interface{}{
			"id":   job.ID.String(),
			"type": string(job.Type),
		})
		if err != nil {
			log.Printf("Failed to marshal job: %v", err)
			continue
		}
		
		if err := r.client.LPush(r.ctx, queueName, data).Err(); err != nil {
			log.Printf("Failed to add job to queue: %v", err)
			continue
		}
		
		// Remove from delayed queue
		if err := r.client.ZRem(r.ctx, delayedQueue, jobID).Err(); err != nil {
			log.Printf("Failed to remove job from delayed queue: %v", err)
		}
	}
}

// Complete marks a job as completed
func (r *RedisClient) Complete(queueName string, jobID uuid.UUID, result interface{}) error {
	// Convert result to JSON
	var resultJSON []byte
	if result != nil {
		var err error
		resultJSON, err = json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal job result: %w", err)
		}
	}

	// Update job in database
	if err := r.db.Model(&Job{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":     JobStatusCompleted,
		"result":     resultJSON,
		"updated_at": time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// Remove from processing set
	processingKey := processingPrefix + queueName
	if err := r.client.HDel(r.ctx, processingKey, jobID.String()).Err(); err != nil {
		return fmt.Errorf("failed to remove job from processing set: %w", err)
	}

	// Add to completed set with TTL
	completedKey := completedPrefix + queueName
	if err := r.client.HSet(r.ctx, completedKey, jobID.String(), time.Now().String()).Err(); err != nil {
		return fmt.Errorf("failed to add job to completed set: %w", err)
	}

	// Set TTL on completed job (24 hours)
	if err := r.client.Expire(r.ctx, completedKey, 24*time.Hour).Err(); err != nil {
		log.Printf("Warning: failed to set TTL on completed job %s: %v", jobID, err)
	}

	return nil
}

// Fail marks a job as failed
func (r *RedisClient) Fail(queueName string, job *Job, err error) error {
	// Remove from processing set
	processingKey := processingPrefix + queueName
	if err := r.client.HDel(r.ctx, processingKey, job.ID.String()).Err(); err != nil {
		return fmt.Errorf("failed to remove job from processing set: %w", err)
	}

	// Increment retry count
	retryCount := job.RetryCount + 1
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	// If retries < max retries, requeue with backoff
	if retryCount < job.MaxRetries {
		// Calculate backoff using the function from types.go
		backoff := calculateBackoff(retryCount)
		nextRetry := time.Now().Add(backoff)

		// Update job
		if err := r.db.Model(job).Updates(map[string]interface{}{
			"retry_count": retryCount,
			"next_retry":  nextRetry,
			"error":       errMsg,
			"status":      JobStatusPending,
			"updated_at":  time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("failed to update job for retry: %w", err)
		}

		// Add to delayed queue with backoff
		score := float64(nextRetry.Unix())
		delayedQueue := delayedPrefix + string(job.Type)
		if err := r.client.ZAdd(r.ctx, delayedQueue, &redis.Z{
			Score:  score,
			Member: job.ID.String(),
		}).Err(); err != nil {
			return fmt.Errorf("failed to add job to delayed queue for retry: %w", err)
		}

		return nil
	}

	// If max retries reached, mark as failed
	if err := r.db.Model(job).Updates(map[string]interface{}{
		"status":      JobStatusFailed,
		"retry_count": retryCount,
		"error":       errMsg,
		"updated_at":  time.Now(),
	}).Error; err != nil {
		return fmt.Errorf("failed to update job as failed: %w", err)
	}

	// Add to failed set
	failedKey := failedPrefix + string(job.Type)
	if err := r.client.HSet(r.ctx, failedKey, job.ID.String(), time.Now().String()).Err(); err != nil {
		return fmt.Errorf("failed to add job to failed set: %w", err)
	}

	return nil
}

// ScheduleRecurring schedules a recurring job
func (r *RedisClient) ScheduleRecurring(name string, jobType JobType, payload interface{}, schedule string) error {
	// Convert payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal recurring job payload: %w", err)
	}

	// Create recurring job data
	job := RecurringJob{
		Name:     name,
		Queue:    string(jobType),
		Payload:  string(payloadBytes),
		Schedule: schedule,
		Enabled:  true,
	}

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal recurring job: %w", err)
	}

	if err := r.client.HSet(r.ctx, recurringPrefix+"jobs", name, data).Err(); err != nil {
		return fmt.Errorf("failed to add recurring job: %w", err)
	}

	return nil
}

// RemoveRecurring removes a recurring job
func (r *RedisClient) RemoveRecurring(name string) error {
	if err := r.client.HDel(r.ctx, recurringPrefix+"jobs", name).Err(); err != nil {
		return fmt.Errorf("failed to remove recurring job: %w", err)
	}
	return nil
}

// GetRecurringJobs gets all recurring jobs
func (r *RedisClient) GetRecurringJobs() ([]RecurringJob, error) {
	result, err := r.client.HGetAll(r.ctx, recurringPrefix+"jobs").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get recurring jobs: %w", err)
	}

	jobs := make([]RecurringJob, 0, len(result))
	for _, data := range result {
		var job RecurringJob
		if err := json.Unmarshal([]byte(data), &job); err != nil {
			log.Printf("Error unmarshaling recurring job: %v", err)
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// updateQueueStats updates the queue stats with additional fields

// GetQueueStats gets statistics for a queue
func (r *RedisClient) GetQueueStats(queueName string) (*QueueStats, error) {
	stats := &QueueStats{
		Queue: queueName,
	}

	// Get waiting count
	waiting, err := r.client.LLen(r.ctx, queuePrefix+queueName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get waiting count: %w", err)
	}
	stats.Waiting = int(waiting)

	// Get delayed count
	delayed, err := r.client.ZCard(r.ctx, delayedPrefix+queueName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get delayed count: %w", err)
	}
	stats.Delayed = int(delayed)

	// Get processing count
	processing, err := r.client.HLen(r.ctx, processingPrefix+queueName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get processing count: %w", err)
	}
	stats.Processing = int(processing)

	// Get failed count
	failed, err := r.client.HLen(r.ctx, failedPrefix+queueName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get failed count: %w", err)
	}
	stats.Failed = int(failed)

	// Get completed count
	completed, err := r.client.HLen(r.ctx, completedPrefix+queueName).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get completed count: %w", err)
	}
	stats.Completed = int(completed)

	return stats, nil
}

// Use the calculateBackoff function from types.go
