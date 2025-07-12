package kyc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"gorm.io/gorm"
)

// DiditService handles integration with the Didit API for KYC verification
type DiditService struct {
	db            *gorm.DB
	apiKey        string
	apiBaseURL    string
	webhookSecret string
	workflowID    string
}

// DiditSessionResponse represents the response from creating a verification session
type DiditSessionResponse struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
	Status    string `json:"status"`
}

// DiditWebhookPayload represents the payload received from Didit webhooks
type DiditWebhookPayload struct {
	EventType    string                 `json:"event_type"`
	SessionID    string                 `json:"session_id"`
	Status       string                 `json:"status"`
	VerifiedData map[string]interface{} `json:"verified_data"`
	Documents    []DiditDocument        `json:"documents"`
	ReportURL    string                 `json:"report_url"`
	ErrorDetails *DiditErrorDetails     `json:"error_details"`
	Timestamp    time.Time              `json:"timestamp"`
}

// DiditDocument represents document information from Didit
type DiditDocument struct {
	Type        string `json:"type"`
	DocumentID  string `json:"document_id"`
	CountryCode string `json:"country_code"`
	Number      string `json:"number"`
	ExpiryDate  string `json:"expiry_date"`
}

// DiditErrorDetails represents error information from Didit
type DiditErrorDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewDiditService creates a new instance of DiditService
func NewDiditService(db *gorm.DB) (*DiditService, error) {
	apiKey := os.Getenv("DIDIT_API_KEY")
	if apiKey == "" {
		return nil, errors.New("DIDIT_API_KEY environment variable is not set")
	}

	webhookSecret := os.Getenv("DIDIT_WEBHOOK_SECRET")
	if webhookSecret == "" {
		return nil, errors.New("DIDIT_WEBHOOK_SECRET environment variable is not set")
	}

	workflowID := os.Getenv("DIDIT_KYC_WORKFLOW_ID")
	if workflowID == "" {
		return nil, errors.New("DIDIT_KYC_WORKFLOW_ID environment variable is not set")
	}

	return &DiditService{
		db:            db,
		apiKey:        apiKey,
		apiBaseURL:    "https://api.didit.me/v2",
		webhookSecret: webhookSecret,
		workflowID:    workflowID,
	}, nil
}

// CreateVerificationSession creates a new KYC verification session for a user
func (s *DiditService) CreateVerificationSession(userID uuid.UUID) (*models.KYCVerification, error) {
	// Check if user exists
	var user models.User
	if err := s.db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Create payload for Didit API
	payload := map[string]interface{}{
		"workflow_id": s.workflowID,
		"metadata": map[string]string{
			"user_id": userID.String(),
		},
	}

	// Convert payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", s.apiBaseURL+"/session/", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	// Send request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var sessionResp DiditSessionResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Create KYC verification record
	verification := &models.KYCVerification{
		UserID:         userID,
		Status:         models.KYCStatusPending,
		SessionID:      sessionResp.SessionID,
		WorkflowID:     s.workflowID,
		VerificationURL: sessionResp.URL,
	}

	// Save to database
	if err := s.db.Create(verification).Error; err != nil {
		return nil, fmt.Errorf("failed to create verification record: %w", err)
	}

	return verification, nil
}

// UploadDocument uploads a document for KYC verification
func (s *DiditService) UploadDocument(verificationID uuid.UUID, docType models.DocumentType, filePath string) (*models.KYCDocument, error) {
	// Check if verification exists
	var verification models.KYCVerification
	if err := s.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		return nil, fmt.Errorf("verification not found: %w", err)
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Create document record
	document := &models.KYCDocument{
		VerificationID: verificationID,
		Type:           docType,
		FilePath:       filePath,
		FileName:       filepath.Base(filePath),
		FileSize:       fileInfo.Size(),
		UploadedAt:     time.Now(),
	}

	// Save to database
	if err := s.db.Create(document).Error; err != nil {
		return nil, fmt.Errorf("failed to create document record: %w", err)
	}

	return document, nil
}

// ProcessWebhook processes webhook notifications from Didit
func (s *DiditService) ProcessWebhook(payload []byte, signature string) error {
	// Verify webhook signature
	// Note: Implement signature verification based on Didit's documentation

	// Parse webhook payload
	var webhookPayload DiditWebhookPayload
	if err := json.Unmarshal(payload, &webhookPayload); err != nil {
		return fmt.Errorf("failed to unmarshal webhook payload: %w", err)
	}

	// Find the verification by session ID
	var verification models.KYCVerification
	if err := s.db.First(&verification, "session_id = ?", webhookPayload.SessionID).Error; err != nil {
		return fmt.Errorf("verification not found for session ID %s: %w", webhookPayload.SessionID, err)
	}

	// Record previous status for history
	previousStatus := verification.Status

	// Update verification status based on webhook event
	switch webhookPayload.Status {
	case "completed":
		verification.Status = models.KYCStatusApproved
		
		// Extract verified data if available
		if webhookPayload.VerifiedData != nil {
			// Extract name if available
			if name, ok := webhookPayload.VerifiedData["full_name"].(string); ok {
				verification.FullName = &name
			}
			
			// Extract DOB if available
			if dobStr, ok := webhookPayload.VerifiedData["date_of_birth"].(string); ok {
				dob, err := time.Parse("2006-01-02", dobStr)
				if err == nil {
					verification.DateOfBirth = &dob
				}
			}
			
			// Extract address if available
			if address, ok := webhookPayload.VerifiedData["address"].(string); ok {
				verification.Address = &address
			}
		}
		
		// Extract document information
		if len(webhookPayload.Documents) > 0 {
			doc := webhookPayload.Documents[0]
			docType := models.DocumentType(doc.Type)
			verification.IDDocType = &docType
			verification.IDDocNumber = &doc.Number
			verification.IDDocCountry = &doc.CountryCode
			
			if doc.ExpiryDate != "" {
				expiry, err := time.Parse("2006-01-02", doc.ExpiryDate)
				if err == nil {
					verification.IDDocExpiry = &expiry
				}
			}
		}
		
		// Save report URL if available
		if webhookPayload.ReportURL != "" {
			verification.ReportURL = &webhookPayload.ReportURL
		}
		
	case "rejected":
		verification.Status = models.KYCStatusRejected
		
		// Save rejection reason if available
		if webhookPayload.ErrorDetails != nil {
			verification.RejectionReason = &webhookPayload.ErrorDetails.Message
		}
		
	case "expired":
		verification.Status = models.KYCStatusExpired
		
	case "in_progress":
		verification.Status = models.KYCStatusInProgress
		
	default:
		// Keep status as is for unknown webhook events
	}

	// Update verification record
	if err := s.db.Save(&verification).Error; err != nil {
		return fmt.Errorf("failed to update verification: %w", err)
	}

	// Create history record if status changed
	if previousStatus != verification.Status {
		history := models.KYCVerificationHistory{
			VerificationID: verification.ID,
			PreviousStatus: previousStatus,
			NewStatus:      verification.Status,
			CreatedAt:      time.Now(),
		}
		
		if err := s.db.Create(&history).Error; err != nil {
			return fmt.Errorf("failed to create history record: %w", err)
		}
	}

	return nil
}

// GetVerificationStatus retrieves the current status of a verification
func (s *DiditService) GetVerificationStatus(verificationID uuid.UUID) (*models.KYCVerification, error) {
	var verification models.KYCVerification
	if err := s.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		return nil, fmt.Errorf("verification not found: %w", err)
	}
	return &verification, nil
}

// GetUserVerifications retrieves all verifications for a user
func (s *DiditService) GetUserVerifications(userID uuid.UUID) ([]models.KYCVerification, error) {
	var verifications []models.KYCVerification
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&verifications).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve verifications: %w", err)
	}
	return verifications, nil
}

// GetVerificationDocuments retrieves all documents for a verification
func (s *DiditService) GetVerificationDocuments(verificationID uuid.UUID) ([]models.KYCDocument, error) {
	var documents []models.KYCDocument
	if err := s.db.Where("verification_id = ?", verificationID).Find(&documents).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve documents: %w", err)
	}
	return documents, nil
}
