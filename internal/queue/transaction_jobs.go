package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/crypto"
	"gorm.io/gorm"
)

// SendTransactionPayload represents the payload for sending a blockchain transaction
type SendTransactionPayload struct {
	TransactionID     uuid.UUID `json:"transaction_id"`
	FromWalletID      uuid.UUID `json:"from_wallet_id"`
	ToAddress         string    `json:"to_address"`
	Amount            string    `json:"amount"`
	TokenSymbol       string    `json:"token_symbol"`
	InternationalPaymentID uuid.UUID `json:"international_payment_id,omitempty"`
}

// TransactionResult represents the result of a transaction
type TransactionResult struct {
	TransactionHash string    `json:"transaction_hash"`
	BlockNumber     uint64    `json:"block_number,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
	Status          string    `json:"status"`
}

// UpdateTransactionStatusPayload represents the payload for updating a transaction status
type UpdateTransactionStatusPayload struct {
	TransactionID   uuid.UUID `json:"transaction_id"`
	TransactionHash string    `json:"transaction_hash"`
}

// NewTransactionJobHandlers creates and registers transaction job handlers
func NewTransactionJobHandlers(db *gorm.DB, baseService *crypto.BaseService) map[JobType]JobHandler {
	handlers := make(map[JobType]JobHandler)

	// Handler for sending blockchain transactions
	handlers[JobTypeSendTransaction] = func(ctx context.Context, job Job) (interface{}, error) {
		var payload SendTransactionPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal send transaction payload: %w", err)
		}

		log.Printf("Processing send transaction job for transaction ID: %s", payload.TransactionID)

		// Get wallet from database
		var wallet database.CryptoWallet
		if err := db.First(&wallet, "id = ?", payload.FromWalletID).Error; err != nil {
			return nil, fmt.Errorf("failed to get wallet: %w", err)
		}

		// Convert amount string to big.Int
		amount := new(big.Int)
		amount.SetString(payload.Amount, 10) // Assuming amount is in wei format as a decimal string

		// Send transaction
		txHash, err := baseService.SendTransaction(&wallet, payload.ToAddress, amount)
		if err != nil {
			// Update transaction status to failed
			if updateErr := db.Model(&database.CryptoTransaction{}).
				Where("id = ?", payload.TransactionID).
				Updates(map[string]interface{}{
					"status":       "failed",
					"error":        err.Error(),
					"updated_at":   time.Now(),
				}).Error; updateErr != nil {
				log.Printf("Failed to update transaction status: %v", updateErr)
			}

			// If this is part of an international payment, update that too
			if payload.InternationalPaymentID != uuid.Nil {
				if updateErr := db.Model(&database.InternationalPayment{}).
					Where("id = ?", payload.InternationalPaymentID).
					Updates(map[string]interface{}{
						"status":     "failed",
						"error":      err.Error(),
						"updated_at": time.Now(),
					}).Error; updateErr != nil {
					log.Printf("Failed to update international payment status: %v", updateErr)
				}
			}

			return nil, fmt.Errorf("failed to send transaction: %w", err)
		}

		// Update transaction with hash
		if err := db.Model(&database.CryptoTransaction{}).
			Where("id = ?", payload.TransactionID).
			Updates(map[string]interface{}{
				"transaction_hash": txHash,
				"status":           "pending",
				"updated_at":       time.Now(),
			}).Error; err != nil {
			return nil, fmt.Errorf("failed to update transaction with hash: %w", err)
		}

		// Queue a job to check transaction status after some time
		// This would typically be handled by a webhook or event listener in production
		// For now, we'll simulate with a delayed job
		time.AfterFunc(30*time.Second, func() {
			// In a real system, this would be a separate job queued for execution
			go checkAndUpdateTransactionStatus(db, baseService, txHash, payload.TransactionID, payload.InternationalPaymentID)
		})

		return TransactionResult{
			TransactionHash: txHash,
			Timestamp:       time.Now(),
			Status:          "pending",
		}, nil
	}

	// Handler for updating transaction status
	handlers[JobTypeUpdateTransactionStatus] = func(ctx context.Context, job Job) (interface{}, error) {
		var payload UpdateTransactionStatusPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal update transaction status payload: %w", err)
		}

		log.Printf("Processing update transaction status job for transaction ID: %s", payload.TransactionID)

		// Get transaction details from blockchain
		txDetails, err := baseService.GetTransactionDetails(payload.TransactionHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get transaction details: %w", err)
		}

		// Update transaction status in database
		status := "confirmed"
		if !txDetails.Success {
			status = "failed"
		}

		if err := db.Model(&database.CryptoTransaction{}).
			Where("id = ?", payload.TransactionID).
			Updates(map[string]interface{}{
				"status":       status,
				"block_number": txDetails.BlockNumber,
				"block_hash":   txDetails.BlockHash,
				"gas_used":     txDetails.GasUsed,
				"updated_at":   time.Now(),
			}).Error; err != nil {
			return nil, fmt.Errorf("failed to update transaction status: %w", err)
		}

		// Get the transaction to check if it's part of an international payment
		var tx database.CryptoTransaction
		if err := db.First(&tx, "id = ?", payload.TransactionID).Error; err != nil {
			log.Printf("Failed to get transaction: %v", err)
		} else if tx.InternationalPaymentID != uuid.Nil {
			// Update international payment status
			if err := db.Model(&database.InternationalPayment{}).
				Where("id = ?", tx.InternationalPaymentID).
				Updates(map[string]interface{}{
					"status":     status,
					"updated_at": time.Now(),
				}).Error; err != nil {
				log.Printf("Failed to update international payment status: %v", err)
			}
		}

		return TransactionResult{
			TransactionHash: payload.TransactionHash,
			BlockNumber:     txDetails.BlockNumber,
			Timestamp:       time.Now(),
			Status:          status,
		}, nil
	}

	return handlers
}

// Helper function to check and update transaction status
func checkAndUpdateTransactionStatus(db *gorm.DB, baseService *crypto.BaseService, txHash string, transactionID, internationalPaymentID uuid.UUID) {
	// Get transaction details from blockchain
	txDetails, err := baseService.GetTransactionDetails(txHash)
	if err != nil {
		log.Printf("Failed to get transaction details: %v", err)
		return
	}

	// Update transaction status in database
	status := "confirmed"
	if !txDetails.Success {
		status = "failed"
	}

	if err := db.Model(&database.CryptoTransaction{}).
		Where("id = ?", transactionID).
		Updates(map[string]interface{}{
			"status":       status,
			"block_number": txDetails.BlockNumber,
			"block_hash":   txDetails.BlockHash,
			"gas_used":     txDetails.GasUsed,
			"updated_at":   time.Now(),
		}).Error; err != nil {
		log.Printf("Failed to update transaction status: %v", err)
		return
	}

	// If this is part of an international payment, update that too
	if internationalPaymentID != uuid.Nil {
		if err := db.Model(&database.InternationalPayment{}).
			Where("id = ?", internationalPaymentID).
			Updates(map[string]interface{}{
				"status":     status,
				"updated_at": time.Now(),
			}).Error; err != nil {
			log.Printf("Failed to update international payment status: %v", err)
		}
	}

	log.Printf("Updated transaction %s status to %s", transactionID, status)
}
