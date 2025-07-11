package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/crypto"
	"gorm.io/gorm"
)

// WebhookHandler handles webhooks from external providers
type WebhookHandler struct {
	db          *gorm.DB
	baseService *crypto.BaseService
	jobQueue    *queue.Queue
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(db *gorm.DB, baseService *crypto.BaseService, jobQueue *queue.Queue) *WebhookHandler {
	return &WebhookHandler{
		db:          db,
		baseService: baseService,
		jobQueue:    jobQueue,
	}
}

// BlockchainTransactionWebhook handles webhooks from blockchain transaction monitoring services
func (h *WebhookHandler) BlockchainTransactionWebhook(c *gin.Context) {
	// Verify webhook signature/auth
	// In production, implement proper authentication for webhooks
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse webhook payload
	var payload struct {
		NetworkID       string `json:"network_id"`
		TransactionHash string `json:"transaction_hash"`
		Status          string `json:"status"`
		BlockNumber     int64  `json:"block_number"`
		Timestamp       int64  `json:"timestamp"`
		FromAddress     string `json:"from_address"`
		ToAddress       string `json:"to_address"`
		Value           string `json:"value"`
		GasUsed         int64  `json:"gas_used"`
		Success         bool   `json:"success"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Printf("Received blockchain webhook for tx: %s, status: %s", payload.TransactionHash, payload.Status)

	// Find transaction in database
	var cryptoTx database.CryptoTransaction
	if err := h.db.Where("tx_hash = ?", payload.TransactionHash).First(&cryptoTx).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// This might be a transaction we're not tracking
			log.Printf("Received webhook for unknown transaction: %s", payload.TransactionHash)
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Update transaction status
	status := "pending"
	switch payload.Status {
	case "confirmed", "success":
		status = "confirmed"
	case "failed", "error":
		status = "failed"
	}

	// Start a database transaction
	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	// Update crypto transaction
	if err := tx.Model(&cryptoTx).Updates(map[string]interface{}{
		"status":       status,
		"block_number": payload.BlockNumber,
		"gas_used":     payload.GasUsed,
		"updated_at":   time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update transaction"})
		return
	}

	// Find related international payment
	var payment database.InternationalPayment
	if err := tx.Where("crypto_tx_id = ?", cryptoTx.ID).First(&payment).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find payment"})
			return
		}
		// No payment associated, just update the transaction
		tx.Commit()
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
		return
	}

	// Update payment status
	if err := tx.Model(&payment).Updates(map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update payment"})
		return
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// Queue notification job if needed
	if status == "confirmed" || status == "failed" {
		notificationPayload := struct {
			PaymentID uuid.UUID `json:"payment_id"`
			Status    string    `json:"status"`
		}{
			PaymentID: payment.ID,
			Status:    status,
		}

		// EnqueueJob handles JSON marshaling internally
		_, err := h.jobQueue.EnqueueJob(queue.JobTypeNotifyPaymentStatus, notificationPayload)
		if err != nil {
			log.Printf("Failed to queue notification job: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// BankTransferWebhook handles webhooks from bank transfer providers
func (h *WebhookHandler) BankTransferWebhook(c *gin.Context) {
	// Verify webhook signature/auth
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse webhook payload
	var payload struct {
		TransactionID  string `json:"transaction_id"`
		Reference      string `json:"reference"`
		Status         string `json:"status"`
		Amount         string `json:"amount"`
		Currency       string `json:"currency"`
		FailureReason  string `json:"failure_reason,omitempty"`
		ProcessedAt    string `json:"processed_at,omitempty"`
		BankIdentifier string `json:"bank_identifier,omitempty"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Printf("Received bank webhook for tx: %s, status: %s", payload.TransactionID, payload.Status)

	// Find bank transaction in database by reference
	var bankTx database.GhanaBankTransaction
	if err := h.db.Where("reference = ?", payload.Reference).First(&bankTx).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Received webhook for unknown bank transaction: %s", payload.Reference)
			c.JSON(http.StatusOK, gin.H{"status": "ignored"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Map external status to our status
	status := "pending"
	switch payload.Status {
	case "completed", "success", "settled":
		status = "completed"
	case "failed", "rejected", "returned":
		status = "failed"
	case "pending", "processing":
		status = "pending"
	}

	// Start a database transaction
	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	// Update bank transaction
	if err := tx.Model(&bankTx).Updates(map[string]interface{}{
		"status":         status,
		"failure_reason": payload.FailureReason,
		"updated_at":     time.Now(),
	}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update transaction"})
		return
	}

	// Find related international payment
	var payment database.InternationalPayment
	if err := tx.Where("bank_transaction_id = ?", bankTx.ID).First(&payment).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find payment"})
			return
		}
		// No payment associated, just update the transaction
		tx.Commit()
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
		return
	}

	// Update payment status if bank transaction failed
	if status == "failed" {
		if err := tx.Model(&payment).Updates(map[string]interface{}{
			"status":     "failed",
			"error":      fmt.Sprintf("Bank transfer failed: %s", payload.FailureReason),
			"updated_at": time.Now(),
		}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update payment"})
			return
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// Queue notification job if needed
	if status == "failed" {
		notificationPayload := struct {
			PaymentID uuid.UUID `json:"payment_id"`
			Status    string    `json:"status"`
		}{
			PaymentID: payment.ID,
			Status:    status,
		}

		// EnqueueJob handles JSON marshaling internally
		_, err := h.jobQueue.EnqueueJob(queue.JobTypeNotifyPaymentStatus, notificationPayload)
		if err != nil {
			log.Printf("Failed to queue notification job: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// ExchangeRateWebhook handles webhooks from exchange rate providers
func (h *WebhookHandler) ExchangeRateWebhook(c *gin.Context) {
	// Verify webhook signature/auth
	apiKey := c.GetHeader("X-API-Key")
	if apiKey == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse webhook payload
	var payload struct {
		BaseCurrency  string             `json:"base_currency"`
		Timestamp     int64              `json:"timestamp"`
		ExchangeRates map[string]float64 `json:"exchange_rates"`
	}

	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}

	log.Printf("Received exchange rate webhook for base currency: %s with %d rates",
		payload.BaseCurrency, len(payload.ExchangeRates))

	// Store exchange rates in database
	// This would typically update a cache or database table with the latest rates

	// For now, just log the rates
	for currency, rate := range payload.ExchangeRates {
		log.Printf("Exchange rate: 1 %s = %.6f %s", payload.BaseCurrency, rate, currency)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
