package queue

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// RetryConfig defines the configuration for job retries
type RetryConfig struct {
	MaxRetries      int           // Maximum number of retry attempts
	InitialInterval time.Duration // Initial retry interval
	MaxInterval     time.Duration // Maximum retry interval
	Multiplier      float64       // Backoff multiplier for subsequent retries
	JobTypes        []JobType     // Job types that can be retried
}

// RetryHandler manages job retries with exponential backoff
type RetryHandler struct {
	db         *gorm.DB
	queue      *Queue
	retryConf  RetryConfig
	retryTypes map[JobType]bool
}

// NewRetryHandler creates a new retry handler
func NewRetryHandler(db *gorm.DB, queue *Queue) *RetryHandler {
	// Default retry configuration
	conf := RetryConfig{
		MaxRetries:      5,
		InitialInterval: 30 * time.Second,
		MaxInterval:     24 * time.Hour,
		Multiplier:      2.0,
		JobTypes: []JobType{
			JobTypeProcessPayment,
			JobTypeSendTransaction,
			JobTypeUpdateTransactionStatus,
			JobTypeGenerateComplianceReport,
		},
	}

	// Build map of retryable job types for quick lookup
	retryTypes := make(map[JobType]bool)
	for _, jt := range conf.JobTypes {
		retryTypes[jt] = true
	}

	return &RetryHandler{
		db:         db,
		queue:      queue,
		retryConf:  conf,
		retryTypes: retryTypes,
	}
}

// HandleFailedJob processes a failed job and schedules a retry if appropriate
func (h *RetryHandler) HandleFailedJob(job Job, err error) {
	// Check if job type is retryable
	if !h.retryTypes[job.Type] {
		log.Printf("Job type %s is not configured for retries. Job ID: %s, Error: %v", job.Type, job.ID, err)
		h.updateJobStatus(job.ID, "failed", err.Error())
		return
	}

	// Get current retry count
	retryCount := job.RetryCount + 1

	if retryCount > h.retryConf.MaxRetries {
		log.Printf("Job exceeded maximum retry attempts (%d). Job ID: %s, Error: %v", 
			h.retryConf.MaxRetries, job.ID, err)
		h.updateJobStatus(job.ID, "failed", fmt.Sprintf("Exceeded max retries: %v", err))
		
		// Trigger failure notification
		h.notifyJobFailure(job, err)
		return
	}

	// Calculate next retry time with exponential backoff
	nextRetryDelay := h.calculateBackoff(retryCount)
	nextRetryTime := time.Now().Add(nextRetryDelay)

	log.Printf("Scheduling retry %d/%d for job %s in %v. Error: %v", 
		retryCount, h.retryConf.MaxRetries, job.ID, nextRetryDelay, err)

	// Update job with retry information
	h.updateJobForRetry(job.ID, retryCount, nextRetryTime, err.Error())
}

// calculateBackoff calculates the backoff duration for a retry attempt
func (h *RetryHandler) calculateBackoff(attempt int) time.Duration {
	// Calculate exponential backoff: initialInterval * multiplier^(attempt-1)
	// with a maximum of maxInterval
	interval := h.retryConf.InitialInterval
	for i := 1; i < attempt; i++ {
		interval = time.Duration(float64(interval) * h.retryConf.Multiplier)
		if interval > h.retryConf.MaxInterval {
			interval = h.retryConf.MaxInterval
			break
		}
	}
	return interval
}

// updateJobStatus updates the status of a job
func (h *RetryHandler) updateJobStatus(jobID uuid.UUID, status, errorMsg string) {
	if err := h.db.Model(&Job{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":     status,
			"error":      errorMsg,
			"updated_at": time.Now(),
		}).Error; err != nil {
		log.Printf("Failed to update job status: %v", err)
	}
}

// updateJobForRetry updates a job for retry
func (h *RetryHandler) updateJobForRetry(jobID uuid.UUID, retryCount int, nextRetry time.Time, errorMsg string) {
	if err := h.db.Model(&Job{}).
		Where("id = ?", jobID).
		Updates(map[string]interface{}{
			"status":      "retry_scheduled",
			"retry_count": retryCount,
			"retry_at":    nextRetry,
			"error":       errorMsg,
			"updated_at":  time.Now(),
		}).Error; err != nil {
		log.Printf("Failed to update job for retry: %v", err)
	}
}

// notifyJobFailure sends notifications about permanently failed jobs
func (h *RetryHandler) notifyJobFailure(job Job, err error) {
	// In production, this would send notifications via email, Slack, etc.
	// For now, just log it
	log.Printf("JOB FAILURE NOTIFICATION: Job %s of type %s has permanently failed after %d retries. Error: %v",
		job.ID, job.Type, h.retryConf.MaxRetries, err)
	
	// Depending on job type, we might want to update related records
	switch job.Type {
	case JobTypeProcessPayment:
		var payload ProcessPaymentPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			log.Printf("Failed to unmarshal payload for failed job notification: %v", err)
			return
		}
		
		// Update payment status
		if err := h.db.Model(&database.InternationalPayment{}).
			Where("id = ?", payload.PaymentID).
			Updates(map[string]interface{}{
				"status":     "failed",
				"error":      fmt.Sprintf("Job failed after %d retries: %v", h.retryConf.MaxRetries, err),
				"updated_at": time.Now(),
			}).Error; err != nil {
			log.Printf("Failed to update payment status: %v", err)
		}
	}
}

// ProcessRetryQueue checks for jobs scheduled for retry and re-queues them
func (h *RetryHandler) ProcessRetryQueue() {
	var jobsToRetry []Job
	
	// Find jobs scheduled for retry that are due
	if err := h.db.Where("status = ? AND retry_at <= ?", "retry_scheduled", time.Now()).
		Find(&jobsToRetry).Error; err != nil {
		log.Printf("Failed to query retry queue: %v", err)
		return
	}
	
	for _, job := range jobsToRetry {
		log.Printf("Processing retry for job %s (attempt %d/%d)", 
			job.ID, job.RetryCount, h.retryConf.MaxRetries)
		
		// Update job status to pending
		if err := h.db.Model(&Job{}).
			Where("id = ?", job.ID).
			Updates(map[string]interface{}{
				"status":     "pending",
				"updated_at": time.Now(),
			}).Error; err != nil {
			log.Printf("Failed to update retry job status: %v", err)
			continue
		}
		
		// Re-queue the job for processing
		h.queue.processJob(job)
	}
}

// StartRetryProcessor starts the background processor for retry queue
func (h *RetryHandler) StartRetryProcessor(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			h.ProcessRetryQueue()
		}
	}()
	
	log.Printf("Retry processor started with interval: %v", interval)
}
