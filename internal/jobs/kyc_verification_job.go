package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/kyc"
	"gorm.io/gorm"
)

const (
	// KYCVerificationJobType is the job type for processing KYC verifications
	KYCVerificationJobType = "process_kyc_verification"
)

// KYCVerificationJobPayload represents the payload for a KYC verification job
type KYCVerificationJobPayload struct {
	VerificationID uuid.UUID `json:"verification_id"`
}

// KYCVerificationJob handles processing KYC verifications through Didit
type KYCVerificationJob struct {
	db      *gorm.DB
	queue   queue.QueueInterface
	kycSvc  *kyc.KYCService
}

// NewKYCVerificationJob creates a new KYC verification job handler
func NewKYCVerificationJob(db *gorm.DB, q queue.QueueInterface, kycSvc *kyc.KYCService) *KYCVerificationJob {
	return &KYCVerificationJob{
		db:      db,
		queue:   q,
		kycSvc:  kycSvc,
	}
}

// RegisterKYCVerificationJobHandlers registers the KYC verification job handlers
func RegisterKYCVerificationJobHandlers(q queue.QueueInterface, db *gorm.DB, kycSvc *kyc.KYCService) {
	handler := NewKYCVerificationJob(db, q, kycSvc)
	// Convert the method to match the queue.JobHandler function signature
	jobHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		// Convert queue.Job to *queue.Job for our handler
		jobCopy := job // Make a copy to avoid modifying the original
		err := handler.ProcessKYCVerification(ctx, &jobCopy)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success"}, nil
	}
	q.RegisterHandler(queue.JobType(KYCVerificationJobType), jobHandler)
}

// EnqueueKYCVerificationJob enqueues a job to process a KYC verification
func (j *KYCVerificationJob) EnqueueKYCVerificationJob(verificationID uuid.UUID) error {
	payload := KYCVerificationJobPayload{
		VerificationID: verificationID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal KYC verification job payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(KYCVerificationJobType),
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// ProcessKYCVerification processes a KYC verification through Didit
// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func (j *KYCVerificationJob) ProcessKYCVerification(ctx context.Context, job *queue.Job) error {
	// Parse payload
	var payload KYCVerificationJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal KYC verification job payload: %w", err)
	}

	// Get verification record
	var verification models.KYCVerification
	if err := j.db.Preload("Documents").First(&verification, "id = ?", payload.VerificationID).Error; err != nil {
		return fmt.Errorf("failed to get KYC verification: %w", err)
	}

	// Check if verification is already processed
	if verification.Status != models.KYCStatusPending {
		log.Printf("KYC verification %s is already in status %s, skipping processing", verification.ID, verification.Status)
		return nil
	}

	// Get user
	var user models.User
	if err := j.db.First(&user, "id = ?", verification.UserID).Error; err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Update verification status to in progress
	verification.Status = models.KYCStatusInProgress
	verification.UpdatedAt = time.Now()
	
	if err := j.db.Save(&verification).Error; err != nil {
		return fmt.Errorf("failed to update verification status: %w", err)
	}

	// Create verification history record
	history := models.KYCVerificationHistory{
		ID:             uuid.New(),
		VerificationID: verification.ID,
		PreviousStatus: models.KYCStatusPending,
		NewStatus:      models.KYCStatusInProgress,
		Notes:          stringPtr("Verification submitted to Didit for processing"),
		CreatedAt:      time.Now(),
	}
	
	if err := j.db.Create(&history).Error; err != nil {
		log.Printf("Failed to create verification history record: %v", err)
	}

	// Process verification through Didit
	_, err := j.processWithDidit(&verification)
	if err != nil {
		// Update verification status to rejected
		verification.Status = models.KYCStatusRejected
		verification.RejectionReason = stringPtr(err.Error())
		verification.UpdatedAt = time.Now()
		
		if dbErr := j.db.Save(&verification).Error; dbErr != nil {
			log.Printf("Failed to update verification status: %v", dbErr)
		}
		
		// Create verification history record
		history := models.KYCVerificationHistory{
			ID:             uuid.New(),
			VerificationID: verification.ID,
			PreviousStatus: models.KYCStatusInProgress,
			NewStatus:      models.KYCStatusRejected,
			Notes:          stringPtr(fmt.Sprintf("Verification failed: %s", err.Error())),
			CreatedAt:      time.Now(),
		}
		
		if dbErr := j.db.Create(&history).Error; dbErr != nil {
			log.Printf("Failed to create verification history record: %v", dbErr)
		}
		
		return fmt.Errorf("failed to process KYC verification: %w", err)
	}

	// The KYC service has already updated the verification status
	// We just need to reload the verification to get the latest status
	if err := j.db.First(&verification, verification.ID).Error; err != nil {
		return fmt.Errorf("failed to reload verification: %w", err)
	}
	
	verification.UpdatedAt = time.Now()

	if err := j.db.Save(&verification).Error; err != nil {
		return fmt.Errorf("failed to update verification with result: %w", err)
	}

	// Create verification history record
	historyComment := fmt.Sprintf("Verification %s by identity provider", verification.Status)
	
	historyRecord := models.KYCVerificationHistory{
		ID:             uuid.New(),
		VerificationID: verification.ID,
		PreviousStatus: models.KYCStatusInProgress,
		NewStatus:      verification.Status,
		Notes:          stringPtr(historyComment),
		CreatedAt:      time.Now(),
	}
	
	if err := j.db.Create(&historyRecord).Error; err != nil {
		log.Printf("Failed to create verification history record: %v", err)
	}

	// If verification was approved, update user's verification status
	if verification.Status == models.KYCStatusApproved {
		// Update user with KYC verification info
		user.UpdatedAt = time.Now()
		
		if err := j.db.Save(&user).Error; err != nil {
			log.Printf("Failed to update user's KYC verification status: %v", err)
		}
	}

	log.Printf("KYC verification %s processed with status %s", verification.ID, verification.Status)
	return nil
}

// processWithDidit processes a KYC verification through Didit
func (j *KYCVerificationJob) processWithDidit(verification *models.KYCVerification) (interface{}, error) {
	// Use the KYC service to process the verification
	err := j.kycSvc.ProcessKYCVerification(context.Background(), verification.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to process KYC verification: %w", err)
	}
	
	// Return a simple result object
	result := map[string]interface{}{
		"success": true,
		"message": "Verification processed successfully",
	}
	
	return result, nil
}
