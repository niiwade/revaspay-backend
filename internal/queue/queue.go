package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// JobType defines the type of job
type JobType string

const (
	// Job types
	JobTypeSendTransaction       JobType = "send_transaction"
	JobTypeProcessPayment        JobType = "process_payment"
	JobTypeUpdateTransactionStatus JobType = "update_transaction_status"
	JobTypeGenerateComplianceReport JobType = "generate_compliance_report"
	JobTypeProcessBlockchainTransaction JobType = "process_blockchain_transaction"
	JobTypeMonitorBlockchainTransaction JobType = "monitor_blockchain_transaction"
	JobTypeProcessInternationalPayment  JobType = "process_international_payment"
	JobTypeNotifyPaymentStatus          JobType = "notify_payment_status"
)

// JobStatus defines the status of a job
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a background job
type Job struct {
	ID         uuid.UUID       `json:"id" gorm:"type:uuid;primaryKey"`
	Type       JobType         `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	Status     JobStatus       `json:"status"`
	RetryCount int             `json:"retry_count" gorm:"default:0"`
	MaxRetries int             `json:"max_retries" gorm:"default:3"`
	NextRetry  *time.Time      `json:"next_retry,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Error      string          `json:"error,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// Queue represents a job queue
type Queue struct {
	db          *gorm.DB
	handlers    map[JobType]JobHandler
	retryHandler *RetryHandler
	processing bool
}

// QueueInterface defines the interface for job queue operations
type QueueInterface interface {
	// Core queue operations
	RegisterHandler(jobType JobType, handler JobHandler)
	Enqueue(job *Job) error
	Dequeue(queueName string) (*RedisJob, error)
	Complete(queueName string, jobID string, result interface{}) error
	Fail(queueName string, jobID string, err error) error
	Retry(queueName string, jobID string, delay int) error
}

// JobHandler is a function that processes a job
type JobHandler func(ctx context.Context, job Job) (interface{}, error)

// NewQueue creates a new queue
func NewQueue(db *gorm.DB) *Queue {
	q := &Queue{
		db:       db,
		handlers: make(map[JobType]JobHandler),
	}
	
	// Create retry handler with reference to this queue
	q.retryHandler = NewRetryHandler(db, q)
	
	// Start retry processor
	q.retryHandler.StartRetryProcessor(1 * time.Minute)
	
	return q
}

// RegisterHandler registers a handler for a job type
func (q *Queue) RegisterHandler(jobType JobType, handler JobHandler) {
	q.handlers[jobType] = handler
}

// EnqueueJob adds a job to the queue
func (q *Queue) EnqueueJob(jobType JobType, payload interface{}) (string, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job payload: %w", err)
	}

	job := Job{
		ID:        uuid.New(),
		Type:      jobType,
		Payload:   payloadBytes,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Save job to database using GORM
	result := q.db.Create(&job)
	if result.Error != nil {
		return "", result.Error
	}

	return job.ID.String(), nil
}

// GetJob retrieves a job by ID
func (q *Queue) GetJob(jobID string) (*Job, error) {
	var job Job
	err := q.db.Model(&Job{}).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("job not found")
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	return &job, nil
}

// UpdateJobStatus updates the status of a job
func (q *Queue) UpdateJobStatus(jobID string, status JobStatus, result interface{}, err error) error {
	job, getErr := q.GetJob(jobID)
	if getErr != nil {
		return getErr
	}

	job.Status = status
	job.UpdatedAt = time.Now()

	if err != nil {
		job.Error = err.Error()
	}

	if result != nil {
		resultBytes, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal result: %w", marshalErr)
		}
		job.Result = resultBytes
	}

	return q.db.Model(&Job{}).Updates(job).Error
}

// ProcessJobs starts processing jobs from the queue
func (q *Queue) ProcessJobs() {
	q.StartProcessing()
}

// StartProcessing starts processing jobs from the queue
func (q *Queue) StartProcessing() {
	if q.processing {
		return
	}

	q.processing = true
	go func() {
		for q.processing {
			// Get a job from the queue
			var job Job
			err := q.db.Model(&Job{}).Where("status = ?", JobStatusPending).First(&job).Error
			if err != nil {
				if err != gorm.ErrRecordNotFound {
					log.Printf("Error getting job from queue: %v", err)
				}
				time.Sleep(1 * time.Second)
				continue
			}

			// Process the job
			q.processJob(job)
		}
	}()
}

func (q *Queue) processJob(job Job) {
	handler, ok := q.handlers[job.Type]
	if !ok {
		log.Printf("No handler registered for job type: %s", job.Type)
		return
	}

	// Update job status to processing
	if err := q.db.Model(&job).Updates(map[string]interface{}{
		"status":     "processing",
		"updated_at": time.Now(),
	}).Error; err != nil {
		log.Printf("Failed to update job status: %v", err)
		return
	}

	// Process the job
	result, err := handler(context.Background(), job)

	// Handle job result
	if err != nil {
		// If we have a retry handler, let it handle the failure
		if q.retryHandler != nil {
			q.retryHandler.HandleFailedJob(job, err)
			return
		}
		
		// Otherwise, mark as failed
		if err := q.db.Model(&job).Updates(map[string]interface{}{
			"status":     "failed",
			"error":      err.Error(),
			"updated_at": time.Now(),
		}).Error; err != nil {
			log.Printf("Failed to update job status: %v", err)
		}
		
		log.Printf("Job %s failed: %v", job.ID, err)
		return
	}

	// Serialize result if present
	var resultJSON []byte
	if result != nil {
		var err error
		resultJSON, err = json.Marshal(result)
		if err != nil {
			log.Printf("Failed to marshal job result: %v", err)
		}
	}

	// Update job with successful result
	if err := q.db.Model(&job).Updates(map[string]interface{}{
		"status":     "completed",
		"result":     resultJSON,
		"updated_at": time.Now(),
	}).Error; err != nil {
		log.Printf("Failed to update job result: %v", err)
	}
}

// StopProcessing stops processing jobs
func (q *Queue) StopProcessing() {
	q.processing = false
}

// Close stops all processing
func (q *Queue) Close() error {
	q.StopProcessing()
	return nil
}
