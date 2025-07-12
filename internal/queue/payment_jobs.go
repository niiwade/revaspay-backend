package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/banking"
	"github.com/revaspay/backend/internal/services/compliance"
	"github.com/revaspay/backend/internal/services/crypto"
	"gorm.io/gorm"
)

// ProcessPaymentPayload represents the payload for processing an international payment
type ProcessPaymentPayload struct {
	PaymentID       uuid.UUID `json:"payment_id"`
	UserID          uuid.UUID `json:"user_id"`
	BankAccountID   uuid.UUID `json:"bank_account_id"`
	WalletID        uuid.UUID `json:"wallet_id"`
	VendorName      string    `json:"vendor_name"`
	RecipientAddress string    `json:"recipient_address"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	Description     string    `json:"description"`
	Reference       string    `json:"reference"`
}

// ComplianceReportPayload represents the payload for generating a compliance report
type ComplianceReportPayload struct {
	UserID          uuid.UUID `json:"user_id"`
	TransactionID   uuid.UUID `json:"transaction_id"`
	TransactionType string    `json:"transaction_type"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	RecipientInfo   string    `json:"recipient_info"`
}

// PaymentResult represents the result of a payment process
type PaymentResult struct {
	PaymentID       uuid.UUID `json:"payment_id"`
	Status          string    `json:"status"`
	BankTxID        uuid.UUID `json:"bank_tx_id,omitempty"`
	CryptoTxID      uuid.UUID `json:"crypto_tx_id,omitempty"`
	TransactionHash string    `json:"transaction_hash,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}

// NewPaymentJobHandlers creates and registers payment job handlers
func NewPaymentJobHandlers(
	db *gorm.DB, 
	baseService *crypto.BaseService,
	bankingService *banking.GhanaBankingService,
	complianceService *compliance.GhanaComplianceService,
	queue *Queue,
) map[JobType]JobHandler {
	handlers := make(map[JobType]JobHandler)

	// Handler for processing international payments
	handlers[JobTypeProcessPayment] = func(ctx context.Context, job Job) (interface{}, error) {
		var payloadData ProcessPaymentPayload
		if err := json.Unmarshal(job.Payload, &payloadData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal process payment payload: %w", err)
		}

		log.Printf("Processing international payment job for payment ID: %s", payloadData.PaymentID)

		// Get payment from database
		var payment database.InternationalPayment
		if err := db.First(&payment, "id = ?", payloadData.PaymentID).Error; err != nil {
			return nil, fmt.Errorf("failed to get payment: %w", err)
		}

		// Update payment status to processing
		if err := db.Model(&payment).Updates(map[string]interface{}{
			"status":     "processing",
			"updated_at": time.Now(),
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to update payment status: %w", err)
		}

		// 1. Perform compliance check
		compliant, checks, err := complianceService.ValidateTransaction(
			payloadData.UserID,
			payloadData.Amount,
			"international_payment",
			payloadData.VendorName,
			payloadData.RecipientAddress,
		)
		if err != nil {
			return handlePaymentError(db, payment.ID, "compliance_error", err.Error())
		}
		if !compliant {
			// Convert compliance checks to error message
			errorMsg := "Compliance checks failed:"
			for _, check := range checks {
				if !check.Passed {
					errorMsg += fmt.Sprintf(" %s: %s;", check.CheckType, check.Description)
				}
			}
			return handlePaymentError(db, payment.ID, "compliance_failed", errorMsg)
		}

		// 2. Create bank transaction record
		bankTx := database.GhanaBankTransaction{
			ID:            uuid.New(),
			UserID:        payloadData.UserID,
			BankAccountID: payloadData.BankAccountID,
			Type:          "debit",
			Amount:        payloadData.Amount,
			Currency:      payloadData.Currency,
			Description:   payloadData.Description,
			Reference:     payloadData.Reference,
			Status:        "pending",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}
		if err := db.Create(&bankTx).Error; err != nil {
			return handlePaymentError(db, payment.ID, "bank_tx_creation_failed", err.Error())
		}

		// 3. Update payment with bank transaction ID
		if err := db.Model(&payment).Updates(map[string]interface{}{
			"bank_transaction_id": bankTx.ID,
			"updated_at":          time.Now(),
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to update payment with bank transaction ID: %w", err)
		}

		// 4. Process bank transaction (in a real system, this would interact with Ghana banking APIs)
		// Simulate bank processing
		time.Sleep(2 * time.Second)

		// Update bank transaction status to completed
		if err := db.Model(&bankTx).Updates(map[string]interface{}{
			"status":     "completed",
			"updated_at": time.Now(),
		}).Error; err != nil {
			return handlePaymentError(db, payment.ID, "bank_tx_update_failed", err.Error())
		}

		// 5. Create crypto transaction record
		// Convert amount to USDC (or appropriate stablecoin)
		// In a real system, this would use an exchange rate service
		exchangeRate := 0.085 // Example GHS to USD rate
		cryptoAmount := payloadData.Amount * exchangeRate

		cryptoTx := database.CryptoTransaction{
			ID:                    uuid.New(),
			UserID:                payloadData.UserID,
			WalletID:              payloadData.WalletID,
			Type:                  "send",
			Amount:                fmt.Sprintf("%.2f", cryptoAmount),
			TokenSymbol:           "USDC",
			RecipientAddress:      payloadData.RecipientAddress,
			Status:                "created",
			InternationalPaymentID: payment.ID,
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		}
		if err := db.Create(&cryptoTx).Error; err != nil {
			return handlePaymentError(db, payment.ID, "crypto_tx_creation_failed", err.Error())
		}

		// 6. Update payment with crypto transaction ID
		if err := db.Model(&payment).Updates(map[string]interface{}{
			"crypto_transaction_id": cryptoTx.ID,
			"updated_at":            time.Now(),
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to update payment with crypto transaction ID: %w", err)
		}

		// 7. Queue job to send the blockchain transaction
		sendTxPayload := SendTransactionPayload{
			TransactionID:         cryptoTx.ID,
			FromWalletID:          payloadData.WalletID,
			ToAddress:             payloadData.RecipientAddress,
			Amount:                cryptoTx.Amount,
			TokenSymbol:           cryptoTx.TokenSymbol,
			InternationalPaymentID: payment.ID,
		}

		jobID, err := queue.EnqueueJob(JobTypeSendTransaction, sendTxPayload)
		if err != nil {
			return handlePaymentError(db, payment.ID, "queue_tx_failed", err.Error())
		}

		log.Printf("Queued send transaction job %s for payment %s", jobID, payment.ID)

		// 8. Generate compliance report asynchronously
		reportPayload := ComplianceReportPayload{
			UserID:          payloadData.UserID,
			TransactionID:   payment.ID,
			TransactionType: "international_payment",
			Amount:          payloadData.Amount,
			Currency:        payloadData.Currency,
			RecipientInfo:   payloadData.RecipientAddress,
		}

		_, err = queue.EnqueueJob(JobTypeGenerateComplianceReport, reportPayload)
		if err != nil {
			// Log but don't fail the payment if compliance report generation fails
			log.Printf("Failed to queue compliance report job: %v", err)
		}

		// Create result to log but we don't return it directly
		result := PaymentResult{
			PaymentID:   payment.ID,
			Status:      "processing",
			BankTxID:    bankTx.ID,
			CryptoTxID:  cryptoTx.ID,
			CompletedAt: time.Now(),
		}
		log.Printf("Payment processing successful: %+v", result)
		return result, nil
	}

	// Handler for generating compliance reports
	handlers[JobTypeGenerateComplianceReport] = func(ctx context.Context, job Job) (interface{}, error) {
		var payloadData ComplianceReportPayload
		if err := json.Unmarshal(job.Payload, &payloadData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal compliance report payload: %w", err)
		}

		log.Printf("Generating compliance report for transaction ID: %s", payloadData.TransactionID)

		// Generate compliance report
		reportJSON, err := complianceService.GenerateComplianceReport(
			payloadData.UserID,
			payloadData.TransactionID,
			payloadData.TransactionType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to generate compliance report: %w", err)
		}

		// Store report in database
		report := database.ComplianceReport{
			ID:                    uuid.New(),
			UserID:                payloadData.UserID,
			InternationalPaymentID: payloadData.TransactionID,
			ReportType:            payloadData.TransactionType,
			ReportData:            reportJSON,
			Status:                "generated",
			CreatedAt:             time.Now(),
			UpdatedAt:             time.Now(),
		}

		if err := db.Create(&report).Error; err != nil {
			return nil, fmt.Errorf("failed to store compliance report: %w", err)
		}

		log.Printf("Compliance report generated successfully: %s", report.ID)
		return report.ID, nil
	}

	return handlers
}

// Helper function to handle payment errors
func handlePaymentError(db *gorm.DB, paymentID uuid.UUID, errorType, errorMessage string) (interface{}, error) {
	err := db.Model(&database.InternationalPayment{}).
		Where("id = ?", paymentID).
		Updates(map[string]interface{}{
			"status":     "failed",
			"error":      fmt.Sprintf("%s: %s", errorType, errorMessage),
			"updated_at": time.Now(),
		}).Error
	if err != nil {
		log.Printf("Failed to update payment status for error: %v", err)
	}
	
	return nil, fmt.Errorf("%s: %s", errorType, errorMessage)
}
