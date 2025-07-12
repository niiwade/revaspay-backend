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

// Auto withdraw job types
const (
	AutoWithdrawCheckJobType   queue.JobType = "auto_withdraw_check"
	ProcessAutoWithdrawJobType queue.JobType = "process_auto_withdraw"
)

// AutoWithdrawPayload represents the payload for an auto-withdraw job
type AutoWithdrawPayload struct {
	ConfigID uuid.UUID `json:"config_id"`
	WalletID uuid.UUID `json:"wallet_id"`
}

// AutoWithdrawJob processes auto-withdrawals for users
type AutoWithdrawJob struct {
	db            *gorm.DB
	walletService *wallet.WalletService
	queue         queue.QueueInterface
}

// NewAutoWithdrawJob creates a new auto-withdraw job
func NewAutoWithdrawJob(db *gorm.DB, jobQueue queue.QueueInterface) *AutoWithdrawJob {
	job := &AutoWithdrawJob{
		db:            db,
		walletService: wallet.NewWalletService(db),
		queue:         jobQueue,
	}
	
	// Register handlers for auto-withdraw jobs
	jobQueue.RegisterHandler(AutoWithdrawCheckJobType, job.checkAutoWithdrawals)
	jobQueue.RegisterHandler(ProcessAutoWithdrawJobType, job.processWithdrawal)
	
	return job
}

// ScheduleAutoWithdrawCheck schedules a job to check for auto-withdrawals
func (j *AutoWithdrawJob) ScheduleAutoWithdrawCheck() (string, error) {
	// Create job payload
	payload, err := json.Marshal(map[string]interface{}{
		"scheduled_at": time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal auto withdraw check payload: %w", err)
	}
	
	// Create job
	job := &queue.Job{
		Type:       AutoWithdrawCheckJobType,
		Payload:    payload,
		MaxRetries: 3,
	}
	
	// Enqueue job
	if err := j.queue.Enqueue(job); err != nil {
		return "", err
	}
	
	return job.ID.String(), nil
}

// checkAutoWithdrawals checks for wallets that need auto-withdrawal
func (j *AutoWithdrawJob) checkAutoWithdrawals(ctx context.Context, job queue.Job) (interface{}, error) {
	log.Println("Checking for auto-withdrawals")
	
	// Find all enabled auto-withdraw configs
	var configs []models.AutoWithdrawConfig
	if err := j.db.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		return nil, fmt.Errorf("error finding auto-withdraw configs: %w", err)
	}
	
	processedCount := 0
	for _, config := range configs {
		// Find user's wallet for the configured currency
		var wallet models.Wallet
		if err := j.db.Where("user_id = ? AND currency = ?", config.UserID, config.Currency).First(&wallet).Error; err != nil {
			log.Printf("Error finding wallet for user %s: %v", config.UserID, err)
			continue
		}
		
		// Check if balance exceeds threshold
		if wallet.Available >= config.Threshold {
			// Enqueue a job to process this withdrawal
			withdrawPayload := AutoWithdrawPayload{
				ConfigID: config.ID,
				WalletID: wallet.ID,
			}
			
			// Marshal payload
			payloadBytes, err := json.Marshal(withdrawPayload)
			if err != nil {
				log.Printf("Error marshaling auto-withdrawal payload for user %s: %v", config.UserID, err)
				continue
			}

			// Create job
			job := &queue.Job{
				Type:       ProcessAutoWithdrawJobType,
				Payload:    payloadBytes,
				MaxRetries: 3,
			}

			// Enqueue job
			err = j.queue.Enqueue(job)
			if err != nil {
				log.Printf("Error enqueueing auto-withdrawal job for user %s: %v", config.UserID, err)
				continue
			}
			
			log.Printf("Auto-withdrawal job enqueued for user %s: %s", config.UserID, job.ID.String())
			processedCount++
		}
	}
	
	return map[string]interface{}{
		"processed_count": processedCount,
		"checked_at": time.Now(),
	}, nil
}

// processWithdrawal handles the withdrawal process
func (j *AutoWithdrawJob) processWithdrawal(ctx context.Context, job queue.Job) (interface{}, error) {
	// Parse the payload
	var withdrawPayload AutoWithdrawPayload
	if err := json.Unmarshal(job.Payload, &withdrawPayload); err != nil {
		return nil, fmt.Errorf("error unmarshaling payload: %w", err)
	}
	
	// Get the config and wallet
	var config models.AutoWithdrawConfig
	if err := j.db.First(&config, "id = ?", withdrawPayload.ConfigID).Error; err != nil {
		return nil, fmt.Errorf("error finding auto-withdraw config: %w", err)
	}
	
	var wallet models.Wallet
	if err := j.db.First(&wallet, "id = ?", withdrawPayload.WalletID).Error; err != nil {
		return nil, fmt.Errorf("error finding wallet: %w", err)
	}
	
	// Verify the wallet still has enough balance
	if wallet.Available < config.Threshold {
		return nil, fmt.Errorf("wallet balance below threshold: %f < %f", wallet.Available, config.Threshold)
	}
	
	// Use a transaction to ensure atomicity
	tx := j.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	
	// Create withdrawal record
	withdrawal := models.Withdrawal{
		UserID:        config.UserID,
		WalletID:      wallet.ID,
		Amount:        wallet.Available,
		Currency:      wallet.Currency,
		Method:        config.WithdrawMethod,
		Status:        "pending",
		Reference:     uuid.New().String(),
		ProcessingFee: 0, // Fee will be calculated by withdrawal service
		InitiatedAt:   time.Now(),
	}
	
	// Store destination ID in metadata
	metadataMap := map[string]interface{}{
		"auto_withdraw": true,
	}
	
	// Add destination ID to metadata if it exists
	if config.DestinationID != uuid.Nil {
		metadataMap["destination_id"] = config.DestinationID.String()
	}
	
	// Use the metadata map directly
	withdrawal.MetaData = models.JSON(metadataMap)
	
	// MetaData is set above

	if err := tx.Create(&withdrawal).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating withdrawal record: %w", err)
	}
	
	// Debit the wallet
	balanceBefore := wallet.Balance
	wallet.Balance -= wallet.Available
	wallet.Available = 0
	
	if err := tx.Save(&wallet).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error updating wallet balance: %w", err)
	}
	
	// Create transaction record
	txMetadataMap := map[string]interface{}{
		"auto_withdraw": true,
		"withdrawal_id": withdrawal.ID.String(),
	}
	// No need to marshal to JSON bytes anymore
	
	transaction := models.Transaction{
		WalletID:      wallet.ID,
		Type:          "withdrawal",
		Amount:        -withdrawal.Amount, // Negative for debit
		Currency:      wallet.Currency,
		Status:        "pending",
		Reference:     withdrawal.Reference,
		Description:   "Auto-withdrawal",
		MetaData:      models.JSON(txMetadataMap),
		BalanceBefore: balanceBefore,
		BalanceAfter:  wallet.Balance,
	}
	
	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating transaction record: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}
	
	// Enqueue a job to process the actual withdrawal through the payment provider
	// This would typically be handled by a separate withdrawal service
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"withdrawal_id": withdrawal.ID.String(),
		"type": "auto_withdraw",
	})
	if err != nil {
		log.Printf("Error marshaling payment processing payload for withdrawal %s: %v", withdrawal.ID, err)
		return nil, nil
	}
	
	// Create job
	processJob := &queue.Job{
		Type:       queue.JobTypeProcessPayment,
		Payload:    payloadBytes,
		MaxRetries: 3,
	}
	
	// Enqueue job
	if err := j.queue.Enqueue(processJob); err != nil {
		log.Printf("Error enqueueing payment processing job for withdrawal %s: %v", withdrawal.ID, err)
	}
	
	log.Printf("Auto-withdrawal processed for user %s: %f %s", config.UserID, withdrawal.Amount, wallet.Currency)
	
	return map[string]interface{}{
		"withdrawal_id": withdrawal.ID.String(),
		"amount": withdrawal.Amount,
		"currency": wallet.Currency,
		"status": "pending",
		"processed_at": time.Now(),
	}, nil
}
