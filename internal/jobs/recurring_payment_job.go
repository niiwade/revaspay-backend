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
	// RecurringPaymentCheckJobType is the job type for checking recurring payments
	RecurringPaymentCheckJobType = "recurring_payment_check"
	
	// ProcessRecurringPaymentJobType is the job type for processing a recurring payment
	ProcessRecurringPaymentJobType = "process_recurring_payment"
)

// RecurringPaymentCheckPayload represents the payload for a recurring payment check job
type RecurringPaymentCheckPayload struct {
	ScheduledAt time.Time `json:"scheduled_at"`
}

// ProcessRecurringPaymentPayload represents the payload for processing a recurring payment
type ProcessRecurringPaymentPayload struct {
	SubscriptionID uuid.UUID `json:"subscription_id"`
}

// RecurringPaymentJob handles recurring payments for subscriptions
type RecurringPaymentJob struct {
	db            *gorm.DB
	queue         queue.QueueInterface
	paymentSvc    *payment.PaymentService
	walletSvc     *wallet.WalletService
}

// NewRecurringPaymentJob creates a new recurring payment job handler
func NewRecurringPaymentJob(db *gorm.DB, q queue.QueueInterface, paymentSvc *payment.PaymentService, walletSvc *wallet.WalletService) *RecurringPaymentJob {
	return &RecurringPaymentJob{
		db:         db,
		queue:      q,
		paymentSvc: paymentSvc,
		walletSvc:  walletSvc,
	}
}

// RegisterJobHandlers registers the recurring payment job handlers
func RegisterRecurringPaymentJobHandlers(q queue.QueueInterface, db *gorm.DB, paymentSvc *payment.PaymentService, walletSvc *wallet.WalletService) {
	handler := NewRecurringPaymentJob(db, q, paymentSvc, walletSvc)
	
	// Wrap the handler methods to match queue.JobHandler signature
	checkHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		err := handler.CheckRecurringPayments(ctx, job)
		if err != nil {
			return nil, err
		}
		return map[string]string{"status": "success"}, nil
	}
	
	processHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		err := handler.ProcessRecurringPayment(ctx, job)
		if err != nil {
			return nil, err
		}
		return map[string]string{"status": "success"}, nil
	}
	
	q.RegisterHandler(RecurringPaymentCheckJobType, checkHandler)
	q.RegisterHandler(ProcessRecurringPaymentJobType, processHandler)
}

// ScheduleRecurringPaymentCheck schedules a job to check for recurring payments
func (j *RecurringPaymentJob) ScheduleRecurringPaymentCheck() error {
	payload := RecurringPaymentCheckPayload{
		ScheduledAt: time.Now(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal recurring payment check payload: %w", err)
	}

	job := &queue.Job{
		Type:       RecurringPaymentCheckJobType,
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// CheckRecurringPayments checks for subscriptions that need to be billed
func (j *RecurringPaymentJob) CheckRecurringPayments(ctx context.Context, job queue.Job) error {
	log.Println("Checking for recurring payments due")

	// Get current time
	now := time.Now()

	// Find active subscriptions due for billing
	var subscriptions []models.Subscription
	if err := j.db.Where("status = ? AND next_payment_date <= ?", "active", now).Find(&subscriptions).Error; err != nil {
		return fmt.Errorf("failed to find subscriptions: %w", err)
	}

	log.Printf("Found %d subscriptions due for billing", len(subscriptions))

	// Process each subscription
	for _, subscription := range subscriptions {
		// Enqueue a job to process this subscription payment
		if err := j.enqueueProcessRecurringPayment(subscription.ID); err != nil {
			log.Printf("Failed to enqueue recurring payment for subscription %s: %v", subscription.ID, err)
			continue
		}

		log.Printf("Enqueued recurring payment for subscription %s", subscription.ID)
	}

	// Schedule the next check in 1 hour
	nextPayload := RecurringPaymentCheckPayload{
		ScheduledAt: time.Now().Add(1 * time.Hour),
	}
	
	nextPayloadBytes, err := json.Marshal(nextPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal next recurring payment check payload: %w", err)
	}
	
	nextRunTime := time.Now().Add(1 * time.Hour)
nextJob := &queue.Job{
		Type:       RecurringPaymentCheckJobType,
		Payload:    nextPayloadBytes,
		MaxRetries: 3,
		NextRetry:  &nextRunTime,
	}

	if err := j.queue.Enqueue(nextJob); err != nil {
		log.Printf("Failed to schedule next recurring payment check: %v", err)
	}

	return nil
}

// enqueueProcessRecurringPayment enqueues a job to process a recurring payment
func (j *RecurringPaymentJob) enqueueProcessRecurringPayment(subscriptionID uuid.UUID) error {
	payload := ProcessRecurringPaymentPayload{
		SubscriptionID: subscriptionID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal process recurring payment payload: %w", err)
	}

	job := &queue.Job{
		Type:       ProcessRecurringPaymentJobType,
		Payload:    payloadBytes,
		MaxRetries: 3,
	}

	return j.queue.Enqueue(job)
}

// ProcessRecurringPayment processes a recurring payment for a subscription
func (j *RecurringPaymentJob) ProcessRecurringPayment(ctx context.Context, job queue.Job) error {
	// Parse payload
	var payload ProcessRecurringPaymentPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal process recurring payment payload: %w", err)
	}

	// Get subscription
	var subscription models.Subscription
	if err := j.db.First(&subscription, "id = ?", payload.SubscriptionID).Error; err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Get subscription plan
	var plan models.SubscriptionPlan
	if err := j.db.First(&plan, "id = ?", subscription.PlanID).Error; err != nil {
		return fmt.Errorf("failed to get subscription plan: %w", err)
	}

	// Get subscriber
	var subscriber models.User
	if err := j.db.First(&subscriber, "id = ?", subscription.SubscriberID).Error; err != nil {
		return fmt.Errorf("failed to get subscriber: %w", err)
	}

	// Get creator
	var creator models.User
	if err := j.db.First(&creator, "id = ?", plan.UserID).Error; err != nil {
		return fmt.Errorf("failed to get creator: %w", err)
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

	// Create payment record
	metadataMap := map[string]interface{}{
		"subscription_id": subscription.ID.String(),
		"plan_id":         plan.ID.String(),
		"subscriber_id":   subscriber.ID.String(),
		"recurring":       true,
	}
	
	// No need to marshal to JSON bytes anymore
	
	payment := models.Payment{
		ID:            uuid.New(),
		UserID:        plan.UserID,
		Amount:        plan.Amount,
		Currency:      plan.Currency,
		Status:        models.PaymentStatusPending,
		PaymentMethod: "subscription",
		CustomerEmail: subscriber.Email,
		CustomerName:  fmt.Sprintf("%s %s", subscriber.FirstName, subscriber.LastName),
		Reference:     uuid.New().String(),
		Metadata:      models.JSON(metadataMap),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := tx.Create(&payment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create payment record: %w", err)
	}

	// Try to charge the subscriber's saved payment method
	// This is a placeholder - in a real implementation, you would use the payment provider SDK
	// to charge the customer's saved payment method
	
	// For now, we'll simulate a successful payment
	payment.Status = models.PaymentStatusCompleted
	payment.UpdatedAt = time.Now()

	if err := tx.Save(&payment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update payment status: %w", err)
	}

	// Credit the creator's wallet
	creditMetadataMap := map[string]interface{}{
		"payment_id":      payment.ID.String(),
		"subscription_id": subscription.ID.String(),
		"plan_id":         plan.ID.String(),
		"subscriber_id":   subscriber.ID.String(),
	}
	
	_, creditErr := j.walletSvc.Credit(
		plan.UserID,
		plan.Amount * 0.95, // Apply platform fee (5%)
		string(plan.Currency),
		fmt.Sprintf("Subscription payment from %s", subscriber.Email),
		"Subscription payment", // Add description parameter
		creditMetadataMap,
	)
	if creditErr != nil {
		tx.Rollback()
		return fmt.Errorf("failed to credit creator wallet: %w", creditErr)
	}

	// Update subscription next payment date
	nextPaymentDate := subscription.NextPaymentDate
	if nextPaymentDate == nil {
		now := time.Now()
		nextPaymentDate = &now
	}
	
	switch plan.Interval {
	case "monthly":
		*nextPaymentDate = nextPaymentDate.AddDate(0, 1, 0)
	case "quarterly":
		*nextPaymentDate = nextPaymentDate.AddDate(0, 3, 0)
	case "yearly":
		*nextPaymentDate = nextPaymentDate.AddDate(1, 0, 0)
	}

	now := time.Now()
	subscription.LastPaymentDate = &now
	subscription.NextPaymentDate = nextPaymentDate
	subscription.UpdatedAt = time.Now()

	if err := tx.Save(&subscription).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Create subscription payment record
	subscriptionPayment := models.SubscriptionPayment{
		Base: models.Base{
			ID:        uuid.New(),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		SubscriptionID: subscription.ID,
		PaymentID:      payment.ID,
		PeriodStart:    subscription.CurrentPeriodStart,
		PeriodEnd:      subscription.CurrentPeriodEnd,
		Status:         models.PaymentStatusCompleted,
	}

	if err := tx.Create(&subscriptionPayment).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to create subscription payment record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully processed recurring payment for subscription %s", subscription.ID)
	return nil
}
