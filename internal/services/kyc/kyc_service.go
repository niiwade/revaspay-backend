package kyc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/models"
	"gorm.io/gorm"
)

// KYCService handles KYC verification operations
type KYCService struct {
	db          *gorm.DB
	diditConfig config.DiditConfig
}

// NewKYCService creates a new KYC service
func NewKYCService(db *gorm.DB, diditConfig config.DiditConfig) *KYCService {
	return &KYCService{
		db:          db,
		diditConfig: diditConfig,
	}
}

// SubmitKYCVerification submits a KYC verification request
func (s *KYCService) SubmitKYCVerification(userID uuid.UUID, documentType string, documentFiles []string, selfieFile string) (*models.KYCVerification, error) {
	// Check if user already has a verification in progress
	var existingVerification models.KYCVerification
	err := s.db.Where("user_id = ? AND status IN ?", userID, []models.KYCStatus{
		models.KYCStatusPending,
		models.KYCStatusInProgress,
	}).First(&existingVerification).Error

	if err == nil {
		return nil, errors.New("user already has a verification in progress")
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("error checking existing verification: %w", err)
	}

	// Create new verification
	verification := models.KYCVerification{
		UserID: userID,
		Status: models.KYCStatusPending,
	}

	// Create verification document records
	var documents []models.KYCDocument
	for _, file := range documentFiles {
		documents = append(documents, models.KYCDocument{
			Type:     models.DocumentTypeID,
			FilePath: file,
		})
	}

	// Add selfie document
	documents = append(documents, models.KYCDocument{
		Type:     models.DocumentTypeSelfie,
		FilePath: selfieFile,
	})

	// Start transaction
	tx := s.db.Begin()
	if err := tx.Error; err != nil {
		return nil, fmt.Errorf("error starting transaction: %w", err)
	}

	// Save verification
	if err := tx.Create(&verification).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating verification: %w", err)
	}

	// Save documents
	for i := range documents {
		documents[i].VerificationID = verification.ID
		if err := tx.Create(&documents[i]).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("error creating document: %w", err)
		}
	}

	// Create history record
	history := models.KYCVerificationHistory{
		VerificationID:  verification.ID,
		PreviousStatus: models.KYCStatusPending,
		NewStatus:      models.KYCStatusPending,
		Notes:          new(string),
	}
	*history.Notes = "Verification submitted"

	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating history: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return &verification, nil
}

// GetKYCStatus gets the KYC status for a user
func (s *KYCService) GetKYCStatus(userID uuid.UUID) (*models.KYCVerification, error) {
	var verification models.KYCVerification
	err := s.db.Where("user_id = ?", userID).Order("submitted_at DESC").First(&verification).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No verification found
		}
		return nil, fmt.Errorf("error finding verification: %w", err)
	}

	return &verification, nil
}

// UpdateKYCStatus updates the status of a KYC verification
func (s *KYCService) UpdateKYCStatus(verificationID uuid.UUID, status string, notes string) error {
	var verification models.KYCVerification
	if err := s.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		return fmt.Errorf("error finding verification: %w", err)
	}

	// Start transaction
	tx := s.db.Begin()
	if err := tx.Error; err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	// Update verification status
	verification.Status = models.KYCStatus(status)

	if err := tx.Save(&verification).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("error updating verification: %w", err)
	}

	// Create history record
	history := models.KYCVerificationHistory{
		VerificationID:  verification.ID,
		PreviousStatus: verification.Status,
		NewStatus:      models.KYCStatus(status),
		Notes:          new(string),
	}
	*history.Notes = notes

	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("error creating history: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// ProcessKYCVerification processes a KYC verification job
func (s *KYCService) ProcessKYCVerification(ctx context.Context, verificationID uuid.UUID) error {
	var verification models.KYCVerification
	if err := s.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		return fmt.Errorf("error finding verification: %w", err)
	}

	// Get documents
	var documents []models.KYCDocument
	if err := s.db.Where("verification_id = ?", verificationID).Find(&documents).Error; err != nil {
		return fmt.Errorf("error finding documents: %w", err)
	}

	// Prepare Didit API request
	// This is a placeholder for the actual API integration
	log.Printf("Processing KYC verification %s with Didit API", verificationID)
	log.Printf("Using API key: %s and Client ID: %s, Environment: %s", s.diditConfig.APIKey, s.diditConfig.ClientID, s.diditConfig.Environment)

	// Update verification status to processing
	if err := s.UpdateKYCStatus(verificationID, string(models.KYCStatusInProgress), "Verification sent to Didit"); err != nil {
		return fmt.Errorf("error updating status: %w", err)
	}

	// In a real implementation, we would send the documents to Didit API
	// and handle the response asynchronously via webhook
	// For now, we'll just simulate a successful verification after a delay
	time.Sleep(2 * time.Second)

	// Update verification status to in progress (waiting for admin approval)
	if err := s.UpdateKYCStatus(verificationID, string(models.KYCStatusInProgress), "Verification processed by Didit, waiting for admin approval"); err != nil {
		return fmt.Errorf("error updating status: %w", err)
	}

	return nil
}

// HandleDiditWebhook handles webhooks from Didit
func (s *KYCService) HandleDiditWebhook(payload []byte) error {
	// Parse webhook payload
	var webhookData map[string]interface{}
	if err := json.Unmarshal(payload, &webhookData); err != nil {
		return fmt.Errorf("error parsing webhook payload: %w", err)
	}

	// Extract verification ID and result
	verificationIDStr, ok := webhookData["partner_id_ref"].(string)
	if !ok {
		return errors.New("missing partner_id_ref in webhook payload")
	}

	verificationID, err := uuid.Parse(verificationIDStr)
	if err != nil {
		return fmt.Errorf("invalid verification ID: %w", err)
	}

	// Extract result
	resultObj, ok := webhookData["result"].(map[string]interface{})
	if !ok {
		return errors.New("missing result in webhook payload")
	}

	resultCode, ok := resultObj["ResultCode"].(float64)
	if !ok {
		return errors.New("missing ResultCode in webhook payload")
	}

	var status models.KYCStatus
	var notes string

	if resultCode == 1 {
		// Success
		status = models.KYCStatusInProgress
		notes = "Verification successful with Didit, waiting for admin approval"
	} else {
		// Failed
		status = models.KYCStatusRejected
		notes = fmt.Sprintf("Verification failed with Didit: %v", resultObj["ResultText"])
	}

	// Update verification status
	if err := s.UpdateKYCStatus(verificationID, string(status), notes); err != nil {
		return fmt.Errorf("error updating status: %w", err)
	}

	return nil
}

// GetPendingVerifications gets all pending KYC verifications
func (s *KYCService) GetPendingVerifications() ([]models.KYCVerification, error) {
	var verifications []models.KYCVerification
	if err := s.db.Where("status = ?", models.KYCStatusInProgress).Find(&verifications).Error; err != nil {
		return nil, fmt.Errorf("error finding verifications: %w", err)
	}

	return verifications, nil
}

// ApproveVerification approves a KYC verification
func (s *KYCService) ApproveVerification(verificationID uuid.UUID, adminID uuid.UUID, notes string) error {
	return s.UpdateKYCStatus(verificationID, string(models.KYCStatusApproved), fmt.Sprintf("Approved by admin %s: %s", adminID, notes))
}

// RejectVerification rejects a KYC verification
func (s *KYCService) RejectVerification(verificationID uuid.UUID, adminID uuid.UUID, notes string) error {
	return s.UpdateKYCStatus(verificationID, string(models.KYCStatusRejected), fmt.Sprintf("Rejected by admin %s: %s", adminID, notes))
}
