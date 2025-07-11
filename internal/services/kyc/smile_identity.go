package kyc

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// SmileIdentityService handles integration with Smile Identity API for KYC verification
type SmileIdentityService struct {
	APIKey      string
	PartnerID   string
	CallbackURL string
	BaseURL     string
	HTTPClient  *http.Client
}

// NewSmileIdentityService creates a new instance of SmileIdentityService
func NewSmileIdentityService() *SmileIdentityService {
	return &SmileIdentityService{
		APIKey:      os.Getenv("SMILE_IDENTITY_API_KEY"),
		PartnerID:   os.Getenv("SMILE_IDENTITY_PARTNER_ID"),
		CallbackURL: os.Getenv("SMILE_IDENTITY_CALLBACK_URL"),
		BaseURL:     os.Getenv("SMILE_IDENTITY_BASE_URL"),
		HTTPClient: &http.Client{
			Timeout: time.Second * 30,
		},
	}
}

// SmileIdentityJobType represents the type of verification job
type SmileIdentityJobType string

const (
	// JobTypeDocumentVerification for document verification only
	JobTypeDocumentVerification SmileIdentityJobType = "document_verification"
	// JobTypeBiometricKYC for biometric verification with ID
	JobTypeBiometricKYC SmileIdentityJobType = "biometric_kyc"
	// JobTypeEnhancedKYC for enhanced KYC with additional checks
	JobTypeEnhancedKYC SmileIdentityJobType = "enhanced_kyc"
)

// SmileIdentityJobRequest represents a job request to Smile Identity API
type SmileIdentityJobRequest struct {
	PartnerID   string                 `json:"partner_id"`
	JobType     SmileIdentityJobType   `json:"job_type"`
	JobID       string                 `json:"job_id"`
	UserID      string                 `json:"user_id"`
	CallbackURL string                 `json:"callback_url"`
	UserData    map[string]interface{} `json:"user_data"`
	Images      map[string]string      `json:"images"` // Base64 encoded images
}

// SmileIdentityJobResponse represents a response from Smile Identity API
type SmileIdentityJobResponse struct {
	Success      bool    `json:"success"`
	JobID        string  `json:"job_id"`
	ResultCode   string  `json:"result_code"`
	ResultText   string  `json:"result_text"`
	Confidence   float64 `json:"confidence"`
	IDNumberMatch bool   `json:"id_number_match"`
	NameMatch    bool    `json:"name_match"`
	DOBMatch     bool    `json:"dob_match"`
}

// SmileIdentityWebhookPayload represents the payload received from Smile Identity webhook
type SmileIdentityWebhookPayload struct {
	JobID            string  `json:"job_id"`
	JobSuccess       bool    `json:"job_success"`
	ResultCode       string  `json:"result_code"`
	ResultText       string  `json:"result_text"`
	ConfidenceScore  float64 `json:"confidence_score"`
	IDNumberMatch    bool    `json:"id_number_match"`
	NameMatch        bool    `json:"name_match"`
	DOBMatch         bool    `json:"dob_match"`
	Timestamp        string  `json:"timestamp"`
	PartnerParams    map[string]interface{} `json:"partner_params"`
}

// EncodeImageToBase64 encodes an image file to base64
func (s *SmileIdentityService) EncodeImageToBase64(filePath string) (string, error) {
	// Read the file
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Encode to base64
	return base64.StdEncoding.EncodeToString(fileData), nil
}

// SubmitVerificationJob submits a verification job to Smile Identity
func (s *SmileIdentityService) SubmitVerificationJob(
	jobType SmileIdentityJobType,
	userID string,
	userData map[string]interface{},
	idFrontPath string,
	idBackPath string,
	selfiePath string,
) (*SmileIdentityJobResponse, error) {
	// Generate a unique job ID
	jobID := fmt.Sprintf("%s-%d", userID, time.Now().UnixNano())

	// Encode images to base64
	idFrontBase64, err := s.EncodeImageToBase64(idFrontPath)
	if err != nil {
		return nil, fmt.Errorf("failed to encode ID front: %w", err)
	}

	// Initialize images map
	images := map[string]string{
		"id_front": idFrontBase64,
	}

	// Add ID back if provided
	if idBackPath != "" {
		idBackBase64, err := s.EncodeImageToBase64(idBackPath)
		if err != nil {
			return nil, fmt.Errorf("failed to encode ID back: %w", err)
		}
		images["id_back"] = idBackBase64
	}

	// Add selfie if provided
	if selfiePath != "" {
		selfieBase64, err := s.EncodeImageToBase64(selfiePath)
		if err != nil {
			return nil, fmt.Errorf("failed to encode selfie: %w", err)
		}
		images["selfie"] = selfieBase64
	}

	// Create job request
	jobRequest := SmileIdentityJobRequest{
		PartnerID:   s.PartnerID,
		JobType:     jobType,
		JobID:       jobID,
		UserID:      userID,
		CallbackURL: s.CallbackURL,
		UserData:    userData,
		Images:      images,
	}

	// Convert to JSON
	requestBody, err := json.Marshal(jobRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/v1/jobs", s.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.APIKey))

	// Send request
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API returned error: %d - %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var jobResponse SmileIdentityJobResponse
	if err := json.Unmarshal(respBody, &jobResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Set job ID from request if not returned in response
	if jobResponse.JobID == "" {
		jobResponse.JobID = jobID
	}

	return &jobResponse, nil
}

// VerifyWebhookSignature verifies the signature of a webhook payload
// This is a placeholder implementation - in a real application, you would
// verify the signature using the shared secret or digital signature
func (s *SmileIdentityService) VerifyWebhookSignature(payload []byte, signature string) bool {
	// In a real implementation, you would verify the signature here
	// For now, we'll just return true
	return true
}
