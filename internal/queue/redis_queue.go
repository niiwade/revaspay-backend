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

const (
	// Queue names
	QueuePaymentWebhook    = "payment_webhook"
	QueueRecurringPayment  = "recurring_payment"
	QueueAutoWithdraw      = "auto_withdraw"
	QueueWithdrawProcessor = "withdraw_processor"

	// Default values
	DefaultRetryCount = 3
	DefaultTTL        = 24 * time.Hour
)

// RedisJob represents a background job for Redis implementation
type RedisJob struct {
	ID        string          `json:"id"`
	Queue     string          `json:"queue"`
	Payload   json.RawMessage `json:"payload"`
	Status    JobStatus       `json:"status"`
	RetryCount int            `json:"retry_count"`
	MaxRetries int            `json:"max_retries"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	RunAt     time.Time       `json:"run_at"`
}

// ConvertToJob converts a RedisJob to a Job
func (r *RedisJob) ConvertToJob() *Job {
	id, _ := uuid.Parse(r.ID)
	return &Job{
		ID:         id,
		Type:       JobType(r.Queue),
		Payload:    r.Payload,
		Status:     r.Status,
		RetryCount: r.RetryCount,
		MaxRetries: r.MaxRetries,
		NextRetry:  nil,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
	}
}

// ConvertFromJob converts a Job to a RedisJob
func ConvertFromJob(j *Job) *RedisJob {
	return &RedisJob{
		ID:         j.ID.String(),
		Queue:      string(j.Type),
		Payload:    j.Payload,
		Status:     j.Status,
		RetryCount: j.RetryCount,
		MaxRetries: j.MaxRetries,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
		RunAt:      time.Now(),
	}
}

// RedisQueueInterface defines the interface for job queue operations
type RedisQueueInterface interface {
	Enqueue(queueName string, payload interface{}, opts ...RedisEnqueueOption) (string, error)
	EnqueueIn(queueName string, payload interface{}, delay time.Duration, opts ...RedisEnqueueOption) (string, error)
	Dequeue(queueName string) (*RedisJob, error)
	Complete(jobID string) error
	Fail(jobID string, err error) error
	Retry(jobID string, delay time.Duration) error
	Schedule(queueName string, payload interface{}, runAt time.Time, opts ...RedisEnqueueOption) (string, error)
}

// RedisEnqueueOption defines options for enqueueing jobs
type RedisEnqueueOption func(*RedisJob)

// WithMaxRetries sets the maximum number of retries for a job
func WithMaxRetries(maxRetries int) RedisEnqueueOption {
	return func(j *RedisJob) {
		j.MaxRetries = maxRetries
	}
}

// WithJobID sets a specific job ID
func WithJobID(id string) RedisEnqueueOption {
	return func(j *RedisJob) {
		j.ID = id
	}
}

// RedisQueue implements Queue interface using Redis
type RedisQueue struct {
	client *redis.Client
	db     *gorm.DB
	ctx    context.Context
	handlers map[JobType]JobHandler
}

// NewRedisQueue creates a new Redis queue
func NewRedisQueue(client *redis.Client, db *gorm.DB) *RedisQueue {
	return &RedisQueue{
		client: client,
		db:     db,
		ctx:    context.Background(),
		handlers: make(map[JobType]JobHandler),
	}
}

// Enqueue adds a job to the queue
func (q *RedisQueue) Enqueue(queueName string, payload interface{}, opts ...RedisEnqueueOption) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	job := &RedisJob{
		ID:         uuid.New().String(),
		Queue:      queueName,
		Payload:    payloadBytes,
		Status:     JobStatusPending,
		RetryCount: 0,
		MaxRetries: DefaultRetryCount,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		RunAt:      time.Now(),
	}
	
	// Apply options
	for _, opt := range opts {
		opt(job)
	}
	
	// Serialize job
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}
	
	// Add to queue
	err = q.client.LPush(q.ctx, queueName, jobBytes).Err()
	if err != nil {
		return "", fmt.Errorf("failed to push job to queue: %w", err)
	}
	
	// Store job details in a hash for retrieval
	err = q.client.HSet(q.ctx, "jobs:"+job.ID, "data", jobBytes).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store job details: %w", err)
	}
	
	// Set TTL on job details
	err = q.client.Expire(q.ctx, "jobs:"+job.ID, DefaultTTL).Err()
	if err != nil {
		log.Printf("Warning: failed to set TTL on job %s: %v", job.ID, err)
	}
	
	return job.ID, nil
}

// EnqueueIn adds a job to the queue with a delay
func (q *RedisQueue) EnqueueIn(queueName string, payload interface{}, delay time.Duration, opts ...RedisEnqueueOption) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	runAt := time.Now().Add(delay)
	
	job := &RedisJob{
		ID:         uuid.New().String(),
		Queue:      queueName,
		Payload:    payloadBytes,
		Status:     JobStatusPending,
		RetryCount: 0,
		MaxRetries: DefaultRetryCount,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		RunAt:      runAt,
	}
	
	// Apply options
	for _, opt := range opts {
		opt(job)
	}
	
	// Serialize job
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}
	
	// Add to delayed queue with score as unix timestamp
	err = q.client.ZAdd(q.ctx, "delayed:"+queueName, &redis.Z{
		Score:  float64(runAt.Unix()),
		Member: jobBytes,
	}).Err()
	if err != nil {
		return "", fmt.Errorf("failed to add job to delayed queue: %w", err)
	}
	
	// Store job details in a hash for retrieval
	err = q.client.HSet(q.ctx, "jobs:"+job.ID, "data", jobBytes).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store job details: %w", err)
	}
	
	// Set TTL on job details
	err = q.client.Expire(q.ctx, "jobs:"+job.ID, DefaultTTL).Err()
	if err != nil {
		log.Printf("Warning: failed to set TTL on job %s: %v", job.ID, err)
	}
	
	return job.ID, nil
}

// Schedule adds a job to the queue to run at a specific time
func (q *RedisQueue) Schedule(queueName string, payload interface{}, runAt time.Time, opts ...RedisEnqueueOption) (string, error) {
	now := time.Now()
	if runAt.Before(now) {
		// If the scheduled time is in the past, run immediately
		return q.Enqueue(queueName, payload, opts...)
	}
	
	// Calculate delay
	delay := runAt.Sub(now)
	return q.EnqueueIn(queueName, payload, delay, opts...)
}

// Dequeue gets a job from the queue
func (q *RedisQueue) Dequeue(queueName string) (*RedisJob, error) {
	// First, check for delayed jobs that are ready to run
	q.moveReadyDelayedJobs(queueName)
	
	// Try to get a job from the queue
	result := q.client.BRPop(q.ctx, 1*time.Second, queueName)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return nil, nil // No jobs available
		}
		return nil, fmt.Errorf("failed to pop job from queue: %w", result.Err())
	}
	
	if len(result.Val()) < 2 {
		return nil, fmt.Errorf("unexpected result format from BRPOP")
	}
	
	// Parse job
	jobBytes := result.Val()[1]
	var job RedisJob
	if err := json.Unmarshal([]byte(jobBytes), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	// Update job status
	job.Status = JobStatusProcessing
	job.UpdatedAt = time.Now()
	
	// Serialize updated job
	updatedJobBytes, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated job: %w", err)
	}
	
	// Update job details
	err = q.client.HSet(q.ctx, "jobs:"+job.ID, "data", updatedJobBytes).Err()
	if err != nil {
		log.Printf("Warning: failed to update job status: %v", err)
	}
	
	return &job, nil
}

// moveReadyDelayedJobs moves delayed jobs that are ready to run to the main queue
func (q *RedisQueue) moveReadyDelayedJobs(queueName string) {
	now := time.Now().Unix()
	
	// Get jobs that are ready to run
	jobs, err := q.client.ZRangeByScore(q.ctx, "delayed:"+queueName, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()
	
	if err != nil {
		log.Printf("Error getting ready delayed jobs: %v", err)
		return
	}
	
	if len(jobs) == 0 {
		return
	}
	
	// Move each job to the main queue
	for _, jobStr := range jobs {
		// Parse job
		var job RedisJob
		if err := json.Unmarshal([]byte(jobStr), &job); err != nil {
			log.Printf("Error unmarshaling delayed job: %v", err)
			continue
		}
		
		// Add to main queue
		err = q.client.LPush(q.ctx, queueName, jobStr).Err()
		if err != nil {
			log.Printf("Error moving delayed job to main queue: %v", err)
			continue
		}
		
		// Remove from delayed queue
		q.client.ZRem(q.ctx, "delayed:"+queueName, jobStr)
	}
}

// RegisterHandler registers a handler for a job type
func (q *RedisQueue) RegisterHandler(jobType JobType, handler JobHandler) {
	q.handlers[jobType] = handler
}

// Complete marks a job as completed
func (q *RedisQueue) Complete(queueName string, jobID string, result interface{}) error {
	// Get job details
	jobData, err := q.client.HGet(q.ctx, "jobs:"+jobID, "data").Result()
	if err != nil {
		return fmt.Errorf("failed to get job details: %w", err)
	}
	
	// Parse job
	var job RedisJob
	if err := json.Unmarshal([]byte(jobData), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	// Update job status
	job.Status = JobStatusCompleted
	job.UpdatedAt = time.Now()
	
	// Serialize updated job
	updatedJobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal updated job: %w", err)
	}
	
	// Update job details
	err = q.client.HSet(q.ctx, "jobs:"+jobID, "data", updatedJobBytes).Err()
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	
	return nil
}

// Fail marks a job as failed
func (q *RedisQueue) Fail(jobID string, jobErr error) error {
	// Get job details
	jobData, err := q.client.HGet(q.ctx, "jobs:"+jobID, "data").Result()
	if err != nil {
		return fmt.Errorf("failed to get job details: %w", err)
	}
	
	// Parse job
	var job RedisJob
	if err := json.Unmarshal([]byte(jobData), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	// Update job status
	job.Status = JobStatusFailed
	job.UpdatedAt = time.Now()
	
	// Serialize updated job
	updatedJobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal updated job: %w", err)
	}
	
	// Update job details
	err = q.client.HSet(q.ctx, "jobs:"+jobID, "data", updatedJobBytes).Err()
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	
	// Check if we should retry
	if job.RetryCount < job.MaxRetries {
		return q.Retry(jobID, time.Duration(job.RetryCount+1)*5*time.Second)
	}
	
	return nil
}

// Retry retries a failed job after a delay
func (q *RedisQueue) Retry(jobID string, delay time.Duration) error {
	// Get job details
	jobData, err := q.client.HGet(q.ctx, "jobs:"+jobID, "data").Result()
	if err != nil {
		return fmt.Errorf("failed to get job details: %w", err)
	}
	
	// Parse job
	var job RedisJob
	if err := json.Unmarshal([]byte(jobData), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job: %w", err)
	}
	
	// Update job for retry
	job.Status = JobStatusPending
	job.RetryCount++
	job.UpdatedAt = time.Now()
	job.RunAt = time.Now().Add(delay)
	
	// Serialize updated job
	updatedJobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal updated job: %w", err)
	}
	
	// Update job details
	err = q.client.HSet(q.ctx, "jobs:"+jobID, "data", updatedJobBytes).Err()
	if err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}
	
	// Add to delayed queue
	err = q.client.ZAdd(q.ctx, "delayed:"+job.Queue, &redis.Z{
		Score:  float64(job.RunAt.Unix()),
		Member: updatedJobBytes,
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to add job to delayed queue: %w", err)
	}
	
	return nil
}
