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
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

const (
	// ReferralRewardJobType is the job type for processing referral rewards
	ReferralRewardJobType queue.JobType = "process_referral_reward"
)

// ReferralRewardJobPayload represents the payload for a referral reward job
type ReferralRewardJobPayload struct {
	ReferralID uuid.UUID `json:"referral_id"`
	EventType  string    `json:"event_type"` // "signup", "first_payment", "kyc_verified", etc.
}

// ReferralRewardJob handles processing referral rewards
type ReferralRewardJob struct {
	db        *gorm.DB
	queue     queue.QueueInterface
	walletSvc *wallet.WalletService
}

// NewReferralRewardJob creates a new referral reward job handler
func NewReferralRewardJob(db *gorm.DB, q queue.QueueInterface, walletSvc *wallet.WalletService) *ReferralRewardJob {
	return &ReferralRewardJob{
		db:        db,
		queue:     q,
		walletSvc: walletSvc,
	}
}

// RegisterReferralRewardJobHandlers registers the referral reward job handlers
func RegisterReferralRewardJobHandlers(q queue.QueueInterface, db *gorm.DB, walletSvc *wallet.WalletService) {
	handler := NewReferralRewardJob(db, q, walletSvc)
	// Convert the ProcessReferralReward method to a JobHandler function
	jobHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		err := handler.ProcessReferralReward(ctx, &job)
		return nil, err
	}
	q.RegisterHandler(ReferralRewardJobType, jobHandler)
}

// EnqueueReferralRewardJob enqueues a job to process a referral reward
func (j *ReferralRewardJob) EnqueueReferralRewardJob(referralID uuid.UUID, eventType string) error {
	payload := ReferralRewardJobPayload{
		ReferralID: referralID,
		EventType:  eventType,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal referral reward job payload: %w", err)
	}

	job := &queue.Job{
		Type:       ReferralRewardJobType,
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// ProcessReferralReward processes a referral reward
func (j *ReferralRewardJob) ProcessReferralReward(ctx context.Context, job *queue.Job) error {
	// Parse payload
	var payload ReferralRewardJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal referral reward job payload: %w", err)
	}

	// Get referral record
	var referral models.Referral
	if err := j.db.First(&referral, "id = ?", payload.ReferralID).Error; err != nil {
		return fmt.Errorf("failed to get referral: %w", err)
	}

	// Get referrer
	var referrer models.User
	if err := j.db.First(&referrer, "id = ?", referral.ReferrerID).Error; err != nil {
		return fmt.Errorf("failed to get referrer: %w", err)
	}

	// Get referred user
	var referredUser models.User
	if err := j.db.First(&referredUser, "id = ?", referral.ReferredUserID).Error; err != nil {
		return fmt.Errorf("failed to get referred user: %w", err)
	}

	// Check if this event type has already been rewarded
	var existingReward models.ReferralReward
	result := j.db.Where("referral_id = ? AND event_type = ?", referral.ID, payload.EventType).First(&existingReward)
	if result.Error == nil {
		log.Printf("Referral reward for event %s already processed for referral %s", payload.EventType, referral.ID)
		return nil
	} else if result.Error != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing referral reward: %w", result.Error)
	}

	// Get reward configuration
	rewardConfig, err := j.getRewardConfig(payload.EventType)
	if err != nil {
		return fmt.Errorf("failed to get reward configuration: %w", err)
	}

	// Start a transaction
	tx := j.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create referral reward record
	reward := models.ReferralReward{
		ID:            uuid.New(),
		ReferralID:    referral.ID,
		ReferrerID:    referral.ReferrerID,
		ReferredUserID: referral.ReferredUserID,
		EventType:     payload.EventType,
		Amount:        rewardConfig.Amount,
		Currency:      rewardConfig.Currency,
		Status:        "pending",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := tx.Create(&reward).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create referral reward: %w", err)
	}

	// Credit the referrer's wallet
	err = j.walletSvc.CreditWithTx(
		tx,
		referral.ReferrerID,
		rewardConfig.Amount,
		rewardConfig.Currency,
		fmt.Sprintf("Referral reward for %s by %s", payload.EventType, referredUser.Email),
		"Referral reward", // Add description parameter
		map[string]interface{}{
			"referral_id":      referral.ID.String(),
			"referred_user_id": referral.ReferredUserID.String(),
			"event_type":       payload.EventType,
			"reward_id":        reward.ID.String(),
		},
	)

	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to credit referrer wallet: %w", err)
	}

	// Update reward status
	reward.Status = "completed"
	reward.CompletedAt = time.Now()
	reward.UpdatedAt = time.Now()

	if err := tx.Save(&reward).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update referral reward status: %w", err)
	}

	// Update referral stats
	referral.RewardsEarned += rewardConfig.Amount
	referral.LastRewardAt = time.Now()
	referral.UpdatedAt = time.Now()

	if err := tx.Save(&referral).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update referral stats: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully processed referral reward for event %s, referral %s", payload.EventType, referral.ID)
	return nil
}

// RewardConfig represents a referral reward configuration
type RewardConfig struct {
	EventType string
	Amount    float64
	Currency  string
}

// getRewardConfig returns the reward configuration for a given event type
func (j *ReferralRewardJob) getRewardConfig(eventType string) (*RewardConfig, error) {
	// In a real implementation, these would be stored in the database or configuration
	// For now, we'll hardcode some example values
	switch eventType {
	case "signup":
		return &RewardConfig{
			EventType: "signup",
			Amount:    1.00,
			Currency:  "USD",
		}, nil
	case "kyc_verified":
		return &RewardConfig{
			EventType: "kyc_verified",
			Amount:    2.00,
			Currency:  "USD",
		}, nil
	case "first_payment":
		return &RewardConfig{
			EventType: "first_payment",
			Amount:    5.00,
			Currency:  "USD",
		}, nil
	case "first_withdrawal":
		return &RewardConfig{
			EventType: "first_withdrawal",
			Amount:    3.00,
			Currency:  "USD",
		}, nil
	case "subscription_created":
		return &RewardConfig{
			EventType: "subscription_created",
			Amount:    10.00,
			Currency:  "USD",
		}, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}
}
