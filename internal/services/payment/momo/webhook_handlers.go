package momo

import (
	"fmt"
	"time"

	"github.com/revaspay/backend/internal/database"
)

// ProcessPaymentNotification processes a payment notification from MTN MoMo webhook
func (s *MoMoService) ProcessPaymentNotification(notification PaymentNotification) error {
	// Find the transaction in the database
	var transaction database.MoMoTransaction
	result := s.db.Where("reference_id = ?", notification.ReferenceID).First(&transaction)
	if result.Error != nil {
		return result.Error
	}

	// Update the transaction status
	transaction.Status = s.mapMoMoStatusToTransactionStatus(notification.Status)
	transaction.UpdatedAt = time.Now()

	// Save the updated transaction
	if err := s.db.Save(&transaction).Error; err != nil {
		return err
	}

	// If the transaction is successful, update the user's wallet
	if transaction.Status == "SUCCESSFUL" {
		// TODO: Update user wallet balance
		// This would typically involve finding the user's wallet and adding the amount
		// For now, we'll just log that this would happen
		fmt.Printf("Would update user %s wallet with amount %.2f\n", transaction.UserID, transaction.Amount)
	}

	return nil
}

// ProcessDisbursementNotification processes a disbursement notification from MTN MoMo webhook
func (s *MoMoService) ProcessDisbursementNotification(notification DisbursementNotification) error {
	// Find the disbursement in the database
	var disbursement database.MoMoDisbursement
	result := s.db.Where("reference_id = ?", notification.ReferenceID).First(&disbursement)
	if result.Error != nil {
		return result.Error
	}

	// Update the disbursement status
	disbursement.Status = s.mapMoMoStatusToDisbursementStatus(notification.Status)
	disbursement.UpdatedAt = time.Now()

	// Save the updated disbursement
	if err := s.db.Save(&disbursement).Error; err != nil {
		return err
	}

	return nil
}

// mapMoMoStatusToTransactionStatus maps MTN MoMo API status to our internal transaction status
func (s *MoMoService) mapMoMoStatusToTransactionStatus(momoStatus string) string {
	switch momoStatus {
	case "SUCCESSFUL":
		return "SUCCESSFUL"
	case "FAILED":
		return "FAILED"
	case "REJECTED":
		return "FAILED"
	case "TIMEOUT":
		return "FAILED"
	case "ONGOING":
		return "PENDING"
	case "PENDING":
		return "PENDING"
	default:
		return "PENDING"
	}
}

// mapMoMoStatusToDisbursementStatus maps MTN MoMo API status to our internal disbursement status
func (s *MoMoService) mapMoMoStatusToDisbursementStatus(momoStatus string) string {
	switch momoStatus {
	case "SUCCESSFUL":
		return "SUCCESSFUL"
	case "FAILED":
		return "FAILED"
	case "REJECTED":
		return "FAILED"
	case "TIMEOUT":
		return "FAILED"
	case "ONGOING":
		return "PENDING"
	case "PENDING":
		return "PENDING"
	default:
		return "PENDING"
	}
}
