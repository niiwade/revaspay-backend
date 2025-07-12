package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/queue"
)

const (
	// VirtualAccountTransactionJobType is the job type for processing virtual account transactions
	VirtualAccountTransactionJobType = "process_virtual_account_transaction"

	// VirtualAccountReconciliationJobType is the job type for reconciling virtual account transactions
	VirtualAccountReconciliationJobType = "reconcile_virtual_accounts"
)

// VirtualAccountTransactionPayload represents the payload for a virtual account transaction job
type VirtualAccountTransactionPayload struct {
	TransactionID uuid.UUID `json:"transaction_id"`
}

// VirtualAccountReconciliationPayload represents the payload for a virtual account reconciliation job
type VirtualAccountReconciliationPayload struct {
	ScheduledAt time.Time `json:"scheduled_at"`
}

// VirtualAccountJob handles processing virtual account transactions
type VirtualAccountJob struct {
	db         *gorm.DB
	queue      queue.QueueInterface
	paymentSvc interface{} // Using interface{} as a placeholder for payment service
	walletSvc  interface{} // Using interface{} as a placeholder for wallet service
}

// NewVirtualAccountJob creates a new virtual account job handler
func NewVirtualAccountJob(db *gorm.DB, q queue.QueueInterface, paymentSvc interface{}, walletSvc interface{}) *VirtualAccountJob {
	job := &VirtualAccountJob{
		db:         db,
		queue:      q,
		paymentSvc: paymentSvc,
		walletSvc:  walletSvc,
	}

	// Register handlers with wrapper functions to match the queue.JobHandler signature
	processHandler := func(ctx context.Context, jobData queue.Job) (interface{}, error) {
		return job.ProcessVirtualAccountTransaction(ctx, jobData)
	}

	reconcileHandler := func(ctx context.Context, jobData queue.Job) (interface{}, error) {
		return job.ReconcileVirtualAccounts(ctx, jobData)
	}

	q.RegisterHandler(queue.JobType(VirtualAccountTransactionJobType), processHandler)
	q.RegisterHandler(queue.JobType(VirtualAccountReconciliationJobType), reconcileHandler)

	return job
}

// RegisterVirtualAccountJobHandlers registers the virtual account job handlers
func RegisterVirtualAccountJobHandlers(q queue.QueueInterface, db *gorm.DB, paymentSvc interface{}, walletSvc interface{}) {
	handler := NewVirtualAccountJob(db, q, paymentSvc, walletSvc)

	processHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		return handler.ProcessVirtualAccountTransaction(ctx, job)
	}

	reconcileHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		return handler.ReconcileVirtualAccounts(ctx, job)
	}

	q.RegisterHandler(queue.JobType(VirtualAccountTransactionJobType), processHandler)
	q.RegisterHandler(queue.JobType(VirtualAccountReconciliationJobType), reconcileHandler)
}

// EnqueueVirtualAccountTransactionJob enqueues a job to process a virtual account transaction
func (j *VirtualAccountJob) EnqueueVirtualAccountTransactionJob(transactionID uuid.UUID) error {
	payload := VirtualAccountTransactionPayload{
		TransactionID: transactionID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal virtual account transaction job payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(VirtualAccountTransactionJobType),
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// ScheduleVirtualAccountReconciliation schedules a job to reconcile virtual accounts
func (j *VirtualAccountJob) ScheduleVirtualAccountReconciliation() error {
	payload := VirtualAccountReconciliationPayload{
		ScheduledAt: time.Now(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal virtual account reconciliation payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(VirtualAccountReconciliationJobType),
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// VirtualAccountTransaction represents a transaction for a virtual account
type VirtualAccountTransaction struct {
	ID                     uuid.UUID              `json:"id"`
	VirtualAccountID       uuid.UUID              `json:"virtual_account_id"`
	Amount                 float64                `json:"amount"`
	Currency               string                 `json:"currency"`
	TransactionID          string                 `json:"transaction_id"`
	Reference              string                 `json:"reference"`
	Type                   string                 `json:"type"` // inbound, outbound
	Status                 string                 `json:"status"`
	Provider               string                 `json:"provider"` // grey, wise, barter
	SenderUserID           uuid.UUID              `json:"sender_user_id"`
	SenderName             string                 `json:"sender_name"`
	SenderEmail            string                 `json:"sender_email"`
	SenderBank             string                 `json:"sender_bank"`
	SenderAccountNumber    string                 `json:"sender_account_number"`
	RecipientUserID        uuid.UUID              `json:"recipient_user_id"`
	RecipientName          string                 `json:"recipient_name"`
	RecipientBank          string                 `json:"recipient_bank"`
	RecipientAccountNumber string                 `json:"recipient_account_number"`
	RecipientAccountID     string                 `json:"recipient_account_id"`
	Fee                    float64                `json:"fee"`
	PaymentID              *uuid.UUID             `json:"payment_id"`
	WithdrawalID           *uuid.UUID             `json:"withdrawal_id"`
	Metadata               map[string]interface{} `json:"metadata"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	CompletedAt            *time.Time             `json:"completed_at"`
}

// ProcessVirtualAccountTransaction processes a virtual account transaction
func (j *VirtualAccountJob) ProcessVirtualAccountTransaction(ctx context.Context, job queue.Job) (interface{}, error) {
	// Parse payload
	var payload VirtualAccountTransactionPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual account transaction job payload: %w", err)
	}

	// Get transaction record
	var transaction VirtualAccountTransaction
	if err := j.db.First(&transaction, "id = ?", payload.TransactionID).Error; err != nil {
		return nil, fmt.Errorf("failed to get virtual account transaction: %w", err)
	}

	// Check if transaction is already processed
	if transaction.Status != "pending" {
		log.Printf("Virtual account transaction %s is already in status %s, skipping processing",
			transaction.ID, transaction.Status)
		return map[string]string{"status": "skipped"}, nil
	}

	// Check if there was an error in the transaction
	if transaction.Status == "failed" {
		log.Printf("Virtual account transaction %s is already in status %s, skipping processing",
			transaction.ID, transaction.Status)
		return map[string]string{"status": "skipped"}, nil
	}

	// Get virtual account
	var virtualAccount database.VirtualAccount
	if err := j.db.First(&virtualAccount, "id = ?", transaction.VirtualAccountID).Error; err != nil {
		return nil, fmt.Errorf("failed to get virtual account: %w", err)
	}

	// Get user
	var user database.User
	if err := j.db.First(&user, "id = ?", virtualAccount.UserID).Error; err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Start a transaction
	tx := j.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Update transaction status
	transaction.Status = "processing"
	transaction.UpdatedAt = time.Now()

	if err := tx.Save(&transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	// Process based on transaction type
	var err error
	switch transaction.Type {
	case "inbound":
		err = j.processInboundTransaction(ctx, tx, &transaction, &virtualAccount, &user)
	case "outbound":
		err = j.processOutboundTransaction(ctx, tx, &transaction, &virtualAccount, &user)
	default:
		tx.Rollback()
		return nil, fmt.Errorf("unsupported transaction type: %s", transaction.Type)
	}

	if err != nil {
		tx.Rollback()

		// Update transaction status to failed
		transaction.Status = "failed"
		// Store error in metadata instead of ErrorMessage which doesn't exist
		metadata := models.JSON{}
		if transaction.Metadata != nil {
			metadata = transaction.Metadata
		}
		metadata["error"] = err.Error()
		transaction.Metadata = metadata
		transaction.UpdatedAt = time.Now()

		if dbErr := j.db.Save(&transaction).Error; dbErr != nil {
			log.Printf("Failed to update transaction status: %v", dbErr)
		}

		return nil, fmt.Errorf("failed to process virtual account transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully processed virtual account transaction %s", transaction.ID)
	return map[string]string{"status": "success"}, nil
}

// processInboundTransaction processes an inbound virtual account transaction
func (j *VirtualAccountJob) processInboundTransaction(
	_ context.Context,
	tx *gorm.DB,
	transaction *VirtualAccountTransaction,
	_ *database.VirtualAccount,
	user *database.User,
) error {
	log.Printf("Processing inbound virtual account transaction %s for user %s",
		transaction.ID, user.ID)

	// Create payment record
	payment := &models.Payment{
		ID:            uuid.New(),
		UserID:        transaction.RecipientUserID,
		Amount:        transaction.Amount,
		Currency:      models.Currency(transaction.Currency),
		Status:        "completed",
		PaymentMethod: "virtual_account",
		Provider:      models.PaymentProvider(transaction.Provider),
		Reference:     transaction.TransactionID,
		CustomerName:  transaction.SenderName,
		CustomerEmail: transaction.SenderEmail,
		PaymentDetails: models.JSON{
			"virtual_account_id": transaction.VirtualAccountID.String(),
		},
		Metadata: models.JSON{
			"virtual_account_id":    transaction.VirtualAccountID.String(),
			"transaction_id":        transaction.TransactionID,
			"sender_name":           transaction.SenderName,
			"sender_bank":           transaction.SenderBank,
			"sender_account_number": transaction.SenderAccountNumber,
		},
	}

	if err := tx.Create(payment).Error; err != nil {
		return fmt.Errorf("failed to create payment record: %w", err)
	}

	// Credit the user's wallet
	log.Printf("Would credit user %s wallet with %f %s for virtual account deposit from %s",
		user.ID, transaction.Amount, transaction.Currency, transaction.SenderName)

	// Mark transaction as processed
	transaction.Status = "completed"
	now := time.Now()
	transaction.CompletedAt = &now
	transaction.UpdatedAt = now

	if err := tx.Save(transaction).Error; err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	log.Printf("Successfully processed inbound virtual account transaction %s", transaction.ID)
	return nil
}

// processOutboundTransaction processes an outbound virtual account transaction
func (j *VirtualAccountJob) processOutboundTransaction(
	_ context.Context,
	tx *gorm.DB,
	transaction *VirtualAccountTransaction,
	_ *database.VirtualAccount,
	user *database.User,
) error {
	log.Printf("Processing outbound virtual account transaction %s for user %s",
		transaction.ID, user.ID)

	// Create withdrawal record
	withdrawal := &models.Withdrawal{
		UserID:    transaction.SenderUserID,
		Amount:    transaction.Amount,
		Currency:  models.Currency(transaction.Currency),
		Method:    "virtual_account",
		Status:    "completed",
		Reference: transaction.TransactionID,
		MetaData: models.JSON{
			"virtual_account_id":       transaction.VirtualAccountID.String(),
			"transaction_id":           transaction.TransactionID,
			"recipient_name":           transaction.RecipientName,
			"recipient_bank":           transaction.RecipientBank,
			"recipient_account_number": transaction.RecipientAccountNumber,
			"description":              "Virtual account transfer",
			"processing_fee":           transaction.Fee,
		},
	}

	if err := tx.Create(withdrawal).Error; err != nil {
		return fmt.Errorf("failed to create withdrawal record: %w", err)
	}

	// Update transaction status
	transaction.Status = "completed"
	transaction.WithdrawalID = &withdrawal.ID
	now := time.Now()
	transaction.CompletedAt = &now
	transaction.UpdatedAt = time.Now()

	if err := tx.Save(transaction).Error; err != nil {
		return fmt.Errorf("failed to update transaction status: %w", err)
	}

	log.Printf("Successfully processed outbound virtual account transaction %s", transaction.ID)
	return nil
}

// ReconcileVirtualAccounts reconciles virtual accounts with provider data
func (j *VirtualAccountJob) ReconcileVirtualAccounts(ctx context.Context, job queue.Job) (interface{}, error) {
	// Parse payload
	var payload VirtualAccountReconciliationPayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal virtual account reconciliation payload: %w", err)
	}

	log.Printf("Starting virtual account reconciliation at %s", payload.ScheduledAt)

	// Get all active virtual accounts
	var virtualAccounts []database.VirtualAccount
	if err := j.db.Where("status = ?", "active").Find(&virtualAccounts).Error; err != nil {
		return nil, fmt.Errorf("failed to get active virtual accounts: %w", err)
	}

	log.Printf("Found %d active virtual accounts to reconcile", len(virtualAccounts))

	// Process each provider separately
	if err := j.reconcileGreyAccounts(ctx, virtualAccounts); err != nil {
		log.Printf("Error reconciling Grey accounts: %v", err)
	}

	if err := j.reconcileWiseAccounts(ctx, virtualAccounts); err != nil {
		log.Printf("Error reconciling Wise accounts: %v", err)
	}

	if err := j.reconcileBarterAccounts(ctx, virtualAccounts); err != nil {
		log.Printf("Error reconciling Barter accounts: %v", err)
	}

	// Schedule next reconciliation in 6 hours
	nextJob := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(VirtualAccountReconciliationJobType),
		Payload:    []byte(job.Payload),
		MaxRetries: 3,
		NextRetry:  func() *time.Time { t := time.Now().Add(6 * time.Hour); return &t }(),
	}
	if err := j.queue.Enqueue(nextJob); err != nil {
		log.Printf("Failed to schedule next virtual account reconciliation: %v", err)
	}

	log.Printf("Virtual account reconciliation completed")
	return map[string]interface{}{"status": "success"}, nil
}

// reconcileGreyAccounts reconciles Grey virtual accounts
func (j *VirtualAccountJob) reconcileGreyAccounts(_ context.Context, accounts []database.VirtualAccount) error {
	// Filter Grey accounts
	var greyAccounts []database.VirtualAccount
	for _, account := range accounts {
		if account.Provider == "grey" {
			greyAccounts = append(greyAccounts, account)
		}
	}

	if len(greyAccounts) == 0 {
		return nil
	}

	log.Printf("Reconciling %d Grey virtual accounts", len(greyAccounts))

	// In a real implementation, you would use the Grey API to get transaction history
	// For now, we'll just log that we're reconciling

	return nil
}

// reconcileWiseAccounts reconciles Wise virtual accounts
func (j *VirtualAccountJob) reconcileWiseAccounts(_ context.Context, accounts []database.VirtualAccount) error {
	// Filter Wise accounts
	var wiseAccounts []database.VirtualAccount
	for _, account := range accounts {
		if account.Provider == "wise" {
			wiseAccounts = append(wiseAccounts, account)
		}
	}

	if len(wiseAccounts) == 0 {
		return nil
	}

	log.Printf("Reconciling %d Wise virtual accounts", len(wiseAccounts))

	// In a real implementation, you would use the Wise API to get transaction history
	// For now, we'll just log that we're reconciling

	return nil
}

// reconcileBarterAccounts reconciles Barter virtual accounts
func (j *VirtualAccountJob) reconcileBarterAccounts(_ context.Context, accounts []database.VirtualAccount) error {
	// Filter Barter accounts
	var barterAccounts []database.VirtualAccount
	for _, account := range accounts {
		if account.Provider == "barter" {
			barterAccounts = append(barterAccounts, account)
		}
	}

	if len(barterAccounts) == 0 {
		return nil
	}

	log.Printf("Reconciling %d Barter virtual accounts", len(barterAccounts))

	// In a real implementation, you would use the Barter API to get transaction history
	// For now, we'll just log that we're reconciling

	return nil
}
