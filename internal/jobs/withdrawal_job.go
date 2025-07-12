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
	"github.com/revaspay/backend/internal/services/payment"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

const (
	// WithdrawalProcessJobType is the job type for processing withdrawals
	WithdrawalProcessJobType = "process_withdrawal"
	
	// WithdrawalStatusCheckJobType is the job type for checking withdrawal status
	WithdrawalStatusCheckJobType = "check_withdrawal_status"
)

// WithdrawalJobPayload represents the payload for a withdrawal job
type WithdrawalJobPayload struct {
	WithdrawalID uuid.UUID `json:"withdrawal_id"`
}

// WithdrawalJob handles processing withdrawals through various payment providers
type WithdrawalJob struct {
	db         *gorm.DB
	queue      queue.QueueInterface
	paymentSvc *payment.PaymentService
	walletSvc  *wallet.WalletService
}

// NewWithdrawalJob creates a new withdrawal job handler
func NewWithdrawalJob(db *gorm.DB, q queue.QueueInterface, paymentSvc *payment.PaymentService, walletSvc *wallet.WalletService) *WithdrawalJob {
	return &WithdrawalJob{
		db:         db,
		queue:      q,
		paymentSvc: paymentSvc,
		walletSvc:  walletSvc,
	}
}

// RegisterHandlers registers the withdrawal job handlers
func (j *WithdrawalJob) RegisterHandlers(q *queue.QueueAdapter) {
	handler := &WithdrawalJob{
		db:        j.db,
		queue:     j.queue,
		paymentSvc: j.paymentSvc,
		walletSvc: j.walletSvc,
	}

	// Wrap the handler methods to match the JobHandler signature
	q.RegisterHandler(queue.JobType(WithdrawalProcessJobType), func(ctx context.Context, job queue.Job) (interface{}, error) {
		err := handler.ProcessWithdrawal(ctx, &job)
		return nil, err
	})
	q.RegisterHandler(queue.JobType(WithdrawalStatusCheckJobType), func(ctx context.Context, job queue.Job) (interface{}, error) {
		err := handler.CheckWithdrawalStatus(ctx, &job)
		return nil, err
	})
}

// EnqueueWithdrawalJob enqueues a job to process a withdrawal
func (j *WithdrawalJob) EnqueueWithdrawalJob(withdrawalID uuid.UUID) error {
	payload := WithdrawalJobPayload{
		WithdrawalID: withdrawalID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal withdrawal job payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(WithdrawalProcessJobType),
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// ProcessWithdrawal processes a withdrawal through the appropriate payment provider
func (j *WithdrawalJob) ProcessWithdrawal(ctx context.Context, job *queue.Job) error {
	// Parse payload
	var payload WithdrawalJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal job payload: %w", err)
	}

	// Get withdrawal record
	var withdrawal models.Withdrawal
	if err := j.db.First(&withdrawal, "id = ?", payload.WithdrawalID).Error; err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}

	// Check if withdrawal is already processed
	if withdrawal.Status != "pending" {
		log.Printf("Withdrawal %s is already in status %s, skipping processing", withdrawal.ID, withdrawal.Status)
		return nil
	}

	// Get user
	var user models.User
	if err := j.db.First(&user, "id = ?", withdrawal.UserID).Error; err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Process withdrawal based on method
	var err error
	switch withdrawal.Method {
	case "bank_transfer":
		err = j.processBankTransfer(ctx, &withdrawal, &user)
	case "mobile_money":
		err = j.processMobileMoneyWithdrawal(ctx, &withdrawal, &user)
	case "crypto":
		err = j.processCryptoWithdrawal(ctx, &withdrawal, &user)
	case "paypal":
		err = j.processPayPalWithdrawal(ctx, &withdrawal, &user)
	default:
		err = fmt.Errorf("unsupported withdrawal method: %s", withdrawal.Method)
	}

	if err != nil {
		// If withdrawal failed, update status and refund to wallet
		if withdrawal.Status == "failed" && withdrawal.FailureReason != "" {
			err = fmt.Errorf("withdrawal failed: %w", err)
		}
		now := time.Now()
		withdrawal.FailedAt = &now
		withdrawal.UpdatedAt = now
		
		if dbErr := j.db.Save(&withdrawal).Error; dbErr != nil {
			log.Printf("Failed to update withdrawal status: %v", dbErr)
		}
		
		// Refund the user's wallet
		refundErr := j.refundWithdrawal(ctx, &withdrawal)
		if refundErr != nil {
			log.Printf("Failed to refund withdrawal: %v", refundErr)
		}
		
		return fmt.Errorf("failed to process withdrawal: %w", err)
	}

	// Schedule a status check for the withdrawal
	return j.scheduleStatusCheck(withdrawal.ID)
}

// processBankTransfer processes a bank transfer withdrawal
func (j *WithdrawalJob) processBankTransfer(_ context.Context, withdrawal *models.Withdrawal, user *models.User) error {
	log.Printf("Processing bank transfer withdrawal %s for user %s", withdrawal.ID, user.ID)

	// Update withdrawal status to processing
	withdrawal.Status = "processing"
	now := time.Now()
	withdrawal.ProcessedAt = &now
	withdrawal.UpdatedAt = now
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	// In a real implementation, you would use a payment provider SDK to initiate the bank transfer
	// For now, we'll simulate a successful initiation
	withdrawal.Reference = uuid.New().String()
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal with provider reference: %w", err)
	}

	log.Printf("Bank transfer initiated successfully, reference: %s", withdrawal.Reference)
	return nil
}

// processMobileMoneyWithdrawal processes a mobile money withdrawal
func (j *WithdrawalJob) processMobileMoneyWithdrawal(_ context.Context, withdrawal *models.Withdrawal, user *models.User) error {
	log.Printf("Processing mobile money withdrawal %s for user %s", withdrawal.ID, user.ID)

	// Update withdrawal status to processing
	withdrawal.Status = "processing"
	now := time.Now()
	withdrawal.ProcessedAt = &now
	withdrawal.UpdatedAt = now
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	// Get mobile money details from metadata
	var mobileNumber, countryCode string
	metadataMap := map[string]interface{}{}
	metadataBytes, _ := json.Marshal(withdrawal.MetaData)
	if err := json.Unmarshal(metadataBytes, &metadataMap); err == nil {
		if num, ok := metadataMap["mobile_number"].(string); ok {
			mobileNumber = num
		}
		if code, ok := metadataMap["country_code"].(string); ok {
			countryCode = code
		}
	}

	// Validate mobile number
	if mobileNumber == "" {
		return fmt.Errorf("mobile number is required for mobile money withdrawal")
	}

	// Create MoMo transaction
	momoTx := models.MoMoTransaction{
		Base: models.Base{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		UserID:        withdrawal.UserID,
		Type:          models.MoMoTransactionTypeDisbursement,
		Status:        models.MoMoTransactionStatusPending,
		Amount:        withdrawal.Amount,
		Currency:      withdrawal.Currency,
		PhoneNumber:   mobileNumber,
		CountryCode:   countryCode,
		Reference:     withdrawal.Reference,
		PayerMessage:  fmt.Sprintf("Withdrawal: %s", withdrawal.Reference),
		PayeeNote:     fmt.Sprintf("Withdrawal to %s", mobileNumber),
		Fee:           0, // Will be updated after transaction
		WithdrawalID:  &withdrawal.ID,
		Description:   "Withdrawal to mobile money",
	}

	if err := j.db.Create(&momoTx).Error; err != nil {
		return fmt.Errorf("failed to create MoMo transaction: %w", err)
	}

	// In a real implementation, you would use the MTN MoMo API to initiate the disbursement
	// For now, we'll simulate a successful initiation
	withdrawal.Reference = uuid.New().String()
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal with provider reference: %w", err)
	}

	log.Printf("Mobile money transfer initiated successfully, reference: %s", withdrawal.Reference)
	return nil
}

// processCryptoWithdrawal processes a crypto withdrawal
func (j *WithdrawalJob) processCryptoWithdrawal(_ context.Context, withdrawal *models.Withdrawal, user *models.User) error {
	log.Printf("Processing crypto withdrawal %s for user %s", withdrawal.ID, user.ID)

	// Update withdrawal status to processing
	withdrawal.Status = "processing"
	now := time.Now()
	withdrawal.ProcessedAt = &now
	withdrawal.UpdatedAt = now
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	// In a real implementation, you would use a crypto API to initiate the transfer
	// For now, we'll simulate a successful initiation
	withdrawal.Reference = uuid.New().String()
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal with provider reference: %w", err)
	}

	log.Printf("Crypto transfer initiated successfully, reference: %s", withdrawal.Reference)
	return nil
}

// processPayPalWithdrawal processes a PayPal withdrawal
func (j *WithdrawalJob) processPayPalWithdrawal(_ context.Context, withdrawal *models.Withdrawal, user *models.User) error {
	log.Printf("Processing PayPal withdrawal %s for user %s", withdrawal.ID, user.ID)

	// Update withdrawal status to processing
	withdrawal.Status = "processing"
	now := time.Now()
	withdrawal.ProcessedAt = &now
	withdrawal.UpdatedAt = now
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal status: %w", err)
	}

	// In a real implementation, you would use the PayPal API to initiate the payout
	// For now, we'll simulate a successful initiation
	withdrawal.Reference = uuid.New().String()
	
	if err := j.db.Save(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to update withdrawal with provider reference: %w", err)
	}

	log.Printf("PayPal transfer initiated successfully, reference: %s", withdrawal.Reference)
	return nil
}

// refundWithdrawal refunds a failed withdrawal back to the user's wallet
func (j *WithdrawalJob) refundWithdrawal(_ context.Context, withdrawal *models.Withdrawal) error {
	log.Printf("Refunding withdrawal %s to user %s", withdrawal.ID, withdrawal.UserID)

	// Get the wallet
	var wallet models.Wallet
	if err := j.db.First(&wallet, "id = ?", withdrawal.WalletID).Error; err != nil {
		return fmt.Errorf("failed to get wallet: %w", err)
	}

	// Credit the wallet
	_, err := j.walletSvc.Credit(
		withdrawal.UserID,
		withdrawal.Amount, // Refund the full amount
		string(withdrawal.Currency),
		fmt.Sprintf("Refund: %s", withdrawal.Reference),
		"Withdrawal failed - amount refunded", // Description
		map[string]interface{}{
			"withdrawal_id": withdrawal.ID.String(),
			"refund_reason": "withdrawal_failed",
			"error":         withdrawal.FailureReason,
		},
	)
	
	if err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	log.Printf("Successfully refunded withdrawal %s to user %s", withdrawal.ID, withdrawal.UserID)
	return nil
}

// scheduleStatusCheck schedules a job to check the status of a withdrawal
func (j *WithdrawalJob) scheduleStatusCheck(withdrawalID uuid.UUID) error {
	payload := WithdrawalJobPayload{
		WithdrawalID: withdrawalID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal withdrawal status check payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(WithdrawalStatusCheckJobType),
		Payload:    payloadBytes,
		MaxRetries: 5,
		NextRetry:  func() *time.Time { t := time.Now().Add(15 * time.Minute); return &t }(), // Check status after 15 minutes
	}

	return j.queue.Enqueue(job)
}

// CheckWithdrawalStatus checks the status of a withdrawal with the payment provider
func (j *WithdrawalJob) CheckWithdrawalStatus(ctx context.Context, job *queue.Job) error {
	// Parse payload
	var payload WithdrawalJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal withdrawal status check payload: %w", err)
	}

	// Get withdrawal record
	var withdrawal models.Withdrawal
	if err := j.db.First(&withdrawal, "id = ?", payload.WithdrawalID).Error; err != nil {
		return fmt.Errorf("failed to get withdrawal: %w", err)
	}

	// Only check withdrawals in processing status
	if withdrawal.Status != "processing" {
		log.Printf("Withdrawal %s is in status %s, not checking with provider", withdrawal.ID, withdrawal.Status)
		return nil
	}

	// Check status based on withdrawal method
	var completed bool
	var err error
	
	switch withdrawal.Method {
	case "bank_transfer":
		completed, err = j.checkBankTransferStatus(ctx, &withdrawal)
	case "mobile_money":
		completed, err = j.checkMobileMoneyStatus(ctx, &withdrawal)
	case "crypto":
		completed, err = j.checkCryptoStatus(ctx, &withdrawal)
	case "paypal":
		completed, err = j.checkPayPalStatus(ctx, &withdrawal)
	default:
		return fmt.Errorf("unsupported withdrawal method: %s", withdrawal.Method)
	}

	if err != nil {
		log.Printf("Error checking withdrawal status: %v", err)
		
		// Schedule another check after a delay
		nextRetry := time.Now().Add(30 * time.Minute)
		job.NextRetry = &nextRetry
		return j.queue.Enqueue(job)
	}

	if completed {
		// Update withdrawal to completed
		withdrawal.Status = "completed"
		now := time.Now()
		withdrawal.CompletedAt = &now
		withdrawal.UpdatedAt = time.Now()
		
		if err := j.db.Save(&withdrawal).Error; err != nil {
			return fmt.Errorf("failed to update withdrawal status: %w", err)
		}
		
		log.Printf("Withdrawal %s completed successfully", withdrawal.ID)
		return nil
	}

	// Still processing, schedule another check
	nextRetry := time.Now().Add(30 * time.Minute)
	job.NextRetry = &nextRetry
	return j.queue.Enqueue(job)
}

// checkBankTransferStatus checks the status of a bank transfer with the provider
func (j *WithdrawalJob) checkBankTransferStatus(_ context.Context, _ *models.Withdrawal) (bool, error) {
	// In a real implementation, you would use the payment provider API to check the status
	// For now, we'll simulate a successful completion
	return true, nil
}

// checkMobileMoneyStatus checks the status of a mobile money transaction with the provider
func (j *WithdrawalJob) checkMobileMoneyStatus(_ context.Context, withdrawal *models.Withdrawal) (bool, error) {
	// Get the MoMo transaction
	var momoTx models.MoMoTransaction
	if err := j.db.First(&momoTx, "withdrawal_id = ?", withdrawal.ID).Error; err != nil {
		return false, fmt.Errorf("failed to get MoMo transaction: %w", err)
	}

	// In a real implementation, you would use the MTN MoMo API to check the status
	// For now, we'll simulate a successful completion
	momoTx.Status = models.MoMoTransactionStatusSucceeded
	momoTx.UpdatedAt = time.Now()
	
	if err := j.db.Save(&momoTx).Error; err != nil {
		return false, fmt.Errorf("failed to update MoMo transaction status: %w", err)
	}

	return true, nil
}

// checkCryptoStatus checks the status of a crypto transaction with the provider
func (j *WithdrawalJob) checkCryptoStatus(_ context.Context, _ *models.Withdrawal) (bool, error) {
	// In a real implementation, you would use the crypto API to check the transaction status
	// For now, we'll simulate a successful completion
	return true, nil
}

// checkPayPalStatus checks the status of a PayPal payout with the provider
func (j *WithdrawalJob) checkPayPalStatus(_ context.Context, _ *models.Withdrawal) (bool, error) {
	// In a real implementation, you would use the PayPal API to check the payout status
	// For now, we'll simulate a successful completion
	return true, nil
}
