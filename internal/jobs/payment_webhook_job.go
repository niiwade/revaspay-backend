package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/payment"
	"github.com/revaspay/backend/internal/services/wallet"
	"gorm.io/gorm"
)

const (
	// PaymentWebhookJobType is the job type for processing payment webhooks
	PaymentWebhookJobType = "payment_webhook"
)

// PaymentWebhookJobPayload represents the payload for a payment webhook job
type PaymentWebhookJobPayload struct {
	WebhookID uuid.UUID `json:"webhook_id"`
}

// PaymentWebhookJob handles processing of payment webhooks
type PaymentWebhookJob struct {
	db         *gorm.DB
	paymentSvc *payment.PaymentService
	walletSvc  *wallet.WalletService
}

// NewPaymentWebhookJob creates a new payment webhook job handler
func NewPaymentWebhookJob(db *gorm.DB, paymentSvc *payment.PaymentService, walletSvc *wallet.WalletService) *PaymentWebhookJob {
	return &PaymentWebhookJob{
		db:         db,
		paymentSvc: paymentSvc,
		walletSvc:  walletSvc,
	}
}

// RegisterPaymentWebhookJobHandlers registers the payment webhook job handlers
func RegisterPaymentWebhookJobHandlers(q queue.QueueInterface, db *gorm.DB, paymentSvc *payment.PaymentService, walletSvc *wallet.WalletService) {
	handler := NewPaymentWebhookJob(db, paymentSvc, walletSvc)
	// Convert the method to match the queue.JobHandler function signature
	jobHandler := func(ctx context.Context, job queue.Job) (interface{}, error) {
		// Convert queue.Job to *queue.Job for our handler
		jobCopy := job // Make a copy to avoid modifying the original
		err := handler.Handle(ctx, &jobCopy)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success"}, nil
	}
	q.RegisterHandler(queue.JobType(PaymentWebhookJobType), jobHandler)
}

// EnqueuePaymentWebhookJob enqueues a payment webhook job
func EnqueuePaymentWebhookJob(q queue.QueueInterface, webhookID uuid.UUID) error {
	payload := PaymentWebhookJobPayload{
		WebhookID: webhookID,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payment webhook job payload: %w", err)
	}

	job := &queue.Job{
		ID:         uuid.New(),
		Type:       queue.JobType(PaymentWebhookJobType),
		Payload:    payloadBytes,
		MaxRetries: 5, // Retry up to 5 times
	}

	return q.Enqueue(job)
}

// Handle processes a payment webhook job
func (j *PaymentWebhookJob) Handle(ctx context.Context, job *queue.Job) error {
	// Parse payload
	var payload PaymentWebhookJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payment webhook job payload: %w", err)
	}

	// Get webhook from database
	var webhook models.PaymentWebhook
	if err := j.db.First(&webhook, "id = ?", payload.WebhookID).Error; err != nil {
		return fmt.Errorf("failed to get webhook: %w", err)
	}

	// Skip if already processed
	if webhook.Processed {
		log.Printf("Webhook %s already processed, skipping", webhook.ID)
		return nil
	}

	// Process webhook based on provider and event
	var err error
	switch webhook.Provider {
	case models.PaymentProviderPaystack:
		err = j.processPaystackWebhook(&webhook)
	case models.PaymentProviderStripe:
		err = j.processStripeWebhook(&webhook)
	case models.PaymentProviderPayPal:
		err = j.processPayPalWebhook(&webhook)
	case models.PaymentProviderCrypto:
		err = j.processCryptoWebhook(&webhook)
	default:
		err = fmt.Errorf("unsupported payment provider: %s", webhook.Provider)
	}

	if err != nil {
		return fmt.Errorf("failed to process webhook: %w", err)
	}

	// Mark webhook as processed
	if err := j.db.Model(&webhook).Update("processed", true).Error; err != nil {
		return fmt.Errorf("failed to mark webhook as processed: %w", err)
	}

	return nil
}

// processPaystackWebhook processes a Paystack webhook
func (j *PaymentWebhookJob) processPaystackWebhook(webhook *models.PaymentWebhook) error {
	// RawData is already a map[string]interface{}, no need to unmarshal
	data := webhook.RawData

	// Extract event
	event, ok := data["event"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing event")
	}

	// Process based on event
	switch event {
	case "charge.success":
		return j.processPaystackChargeSuccess(webhook, data)
	default:
		log.Printf("Unhandled Paystack event: %s", event)
		return nil
	}
}

// processPaystackChargeSuccess processes a successful Paystack charge
func (j *PaymentWebhookJob) processPaystackChargeSuccess(_ *models.PaymentWebhook, data map[string]interface{}) error {
	// Extract payment reference
	dataObj, ok := data["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid webhook data: missing data")
	}

	reference, ok := dataObj["reference"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing reference")
	}

	// Verify payment
	payment, err := j.paymentSvc.VerifyPayment(reference)
	if err != nil {
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	// Skip if payment is not completed
	if payment.Status != models.PaymentStatusCompleted {
		log.Printf("Payment %s is not completed, status: %s", payment.ID, payment.Status)
		return nil
	}

	// Credit user's wallet
	_, err = j.walletSvc.Credit(
		payment.UserID,
		payment.Amount-payment.Fee, // Credit amount minus fee
		string(payment.Currency),
		fmt.Sprintf("Payment: %s", payment.Reference),
		"Payment from Paystack", // Add description parameter
		map[string]interface{}{
			"payment_id":      payment.ID.String(),
			"payment_method":  payment.PaymentMethod,
			"payment_provider": string(payment.Provider),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	log.Printf("Successfully processed payment %s and credited wallet for user %s", payment.ID, payment.UserID)
	return nil
}

// processStripeWebhook processes a Stripe webhook
func (j *PaymentWebhookJob) processStripeWebhook(webhook *models.PaymentWebhook) error {
	// RawData is already a map[string]interface{}, no need to unmarshal
	data := webhook.RawData

	// Extract event
	event, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing type")
	}

	// Process based on event
	switch event {
	case "payment_intent.succeeded":
		return j.processStripePaymentIntentSucceeded(webhook, data)
	default:
		log.Printf("Unhandled Stripe event: %s", event)
		return nil
	}
}

// processStripePaymentIntentSucceeded processes a successful Stripe payment intent
func (j *PaymentWebhookJob) processStripePaymentIntentSucceeded(_ *models.PaymentWebhook, data map[string]interface{}) error {
	// Extract payment reference
	dataObj, ok := data["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid webhook data: missing data")
	}

	object, ok := dataObj["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid webhook data: missing object")
	}

	reference, ok := object["id"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing id")
	}

	// Verify payment
	payment, err := j.paymentSvc.VerifyPayment(reference)
	if err != nil {
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	// Skip if payment is not completed
	if payment.Status != models.PaymentStatusCompleted {
		log.Printf("Payment %s is not completed, status: %s", payment.ID, payment.Status)
		return nil
	}

	// Credit user's wallet
	_, err = j.walletSvc.Credit(
		payment.UserID,
		payment.Amount-payment.ProviderFee, // Credit amount minus provider fee
		string(payment.Currency),
		fmt.Sprintf("Payment: %s", payment.Reference),
		"Payment from Stripe", // Add description parameter
		map[string]interface{}{
			"payment_id":      payment.ID.String(),
			"payment_method":  payment.PaymentMethod,
			"payment_provider": string(payment.Provider),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	log.Printf("Successfully processed payment %s and credited wallet for user %s", payment.ID, payment.UserID)
	return nil
}

// processPayPalWebhook processes a PayPal webhook
func (j *PaymentWebhookJob) processPayPalWebhook(webhook *models.PaymentWebhook) error {
	// RawData is already a map[string]interface{}, no need to unmarshal
	data := webhook.RawData

	// Extract event
	event, ok := data["event_type"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing event_type")
	}

	// Process based on event
	switch event {
	case "PAYMENT.CAPTURE.COMPLETED":
		return j.processPayPalPaymentCaptureCompleted(webhook, data)
	default:
		log.Printf("Unhandled PayPal event: %s", event)
		return nil
	}
}

// processPayPalPaymentCaptureCompleted processes a completed PayPal payment capture
func (j *PaymentWebhookJob) processPayPalPaymentCaptureCompleted(_ *models.PaymentWebhook, data map[string]interface{}) error {
	// Extract payment reference
	resource, ok := data["resource"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid webhook data: missing resource")
	}

	reference, ok := resource["id"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing id")
	}

	// Verify payment
	payment, err := j.paymentSvc.VerifyPayment(reference)
	if err != nil {
		return fmt.Errorf("failed to verify payment: %w", err)
	}

	// Skip if payment is not completed
	if payment.Status != models.PaymentStatusCompleted {
		log.Printf("Payment %s is not completed, status: %s", payment.ID, payment.Status)
		return nil
	}

	// Credit user's wallet
	_, err = j.walletSvc.Credit(
		payment.UserID,
		payment.Amount-payment.ProviderFee, // Credit amount minus provider fee
		string(payment.Currency),
		fmt.Sprintf("Payment: %s", payment.Reference),
		"Payment from PayPal", // Add description parameter
		map[string]interface{}{
			"payment_id":      payment.ID.String(),
			"payment_method":  payment.PaymentMethod,
			"payment_provider": string(payment.Provider),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	log.Printf("Successfully processed payment %s and credited wallet for user %s", payment.ID, payment.UserID)
	return nil
}

// processCryptoWebhook processes a crypto webhook
func (j *PaymentWebhookJob) processCryptoWebhook(webhook *models.PaymentWebhook) error {
	// RawData is already a map[string]interface{}, no need to unmarshal
	data := webhook.RawData

	// Extract payment reference
	reference, ok := data["payment_id"].(string)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing payment_id")
	}

	// Get crypto payment
	var cryptoPayment models.CryptoPayment
	if err := j.db.Where("payment_id = ?", reference).First(&cryptoPayment).Error; err != nil {
		return fmt.Errorf("failed to get crypto payment: %w", err)
	}

	// Get payment
	var payment models.Payment
	if err := j.db.First(&payment, "id = ?", cryptoPayment.PaymentID).Error; err != nil {
		return fmt.Errorf("failed to get payment: %w", err)
	}

	// Check confirmations
	confirmations, ok := data["confirmations"].(float64)
	if !ok {
		return fmt.Errorf("invalid webhook data: missing confirmations")
	}

	// Update confirmations
	cryptoPayment.Confirmations = int(confirmations)

	// Define required confirmations based on network
	var requiredConfirmations int
	switch cryptoPayment.Network {
	case "ethereum":
		requiredConfirmations = 12
	case "binance":
		requiredConfirmations = 15
	default: // Bitcoin and others
		requiredConfirmations = 6
	}

	// Check if payment is confirmed
	if cryptoPayment.Confirmations >= requiredConfirmations {
		// Mark payment as completed
		payment.Status = models.PaymentStatusCompleted

		// Credit user's wallet
		_, err := j.walletSvc.Credit(
			payment.UserID,
			payment.Amount, // No provider fee for crypto payments
			string(payment.Currency),
			fmt.Sprintf("Crypto Payment: %s", cryptoPayment.TxHash),
			"Cryptocurrency payment", // Add description parameter
			map[string]interface{}{
				"payment_id":       payment.ID.String(),
				"payment_method":   "crypto",
				"crypto_currency":  cryptoPayment.Currency,
				"transaction_hash": cryptoPayment.TxHash,
			},
		)
		if err != nil {
			return fmt.Errorf("failed to credit wallet: %w", err)
		}

		log.Printf("Successfully processed crypto payment %s and credited wallet for user %s", payment.ID, payment.UserID)
	}

	// Save changes
	if err := j.db.Save(&cryptoPayment).Error; err != nil {
		return fmt.Errorf("failed to save crypto payment: %w", err)
	}

	if err := j.db.Save(&payment).Error; err != nil {
		return fmt.Errorf("failed to save payment: %w", err)
	}

	return nil
}
