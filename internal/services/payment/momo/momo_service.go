package momo

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"gorm.io/gorm"
)

// MoMoPaymentStatus represents the status of a MoMo payment
type MoMoPaymentStatus string

const (
	MoMoStatusPending    MoMoPaymentStatus = "PENDING"
	MoMoStatusSuccessful MoMoPaymentStatus = "SUCCESSFUL"
	MoMoStatusFailed     MoMoPaymentStatus = "FAILED"
	MoMoStatusExpired    MoMoPaymentStatus = "EXPIRED"
	MoMoStatusCancelled  MoMoPaymentStatus = "CANCELLED"
	MoMoStatusOngoing    MoMoPaymentStatus = "ONGOING"
)

// MoMoService handles MTN Mobile Money payments
type MoMoService struct {
	db     *gorm.DB
	client *MoMoClient
}

// NewMoMoService creates a new MoMo service
func NewMoMoService(db *gorm.DB, subscriptionKey, collectionAPIUser, collectionAPIKey,
	disbursementAPIUser, disbursementAPIKey string, useSandbox bool) *MoMoService {
	return &MoMoService{
		db:     db,
		client: NewMoMoClient(subscriptionKey, collectionAPIUser, collectionAPIKey, disbursementAPIUser, disbursementAPIKey, useSandbox),
	}
}

// PaymentRequest represents a request to collect payment via MoMo
type PaymentRequest struct {
	UserID       uuid.UUID
	PhoneNumber  string
	Amount       float64
	Description  string
	CallbackURL  string
	ReferenceID  string
	CurrencyCode string
}

// PaymentResponse represents the response from a payment request
type PaymentResponse struct {
	TransactionID string
	ReferenceID   string
	Status        MoMoPaymentStatus
	Message       string
}

// RequestPayment initiates a payment request to a mobile money user
func (s *MoMoService) RequestPayment(req PaymentRequest) (*PaymentResponse, error) {
	// Format phone number to international format if needed
	phoneNumber := formatPhoneNumber(req.PhoneNumber)

	// Create a unique reference if not provided
	referenceID := req.ReferenceID
	if referenceID == "" {
		referenceID = fmt.Sprintf("RP-%s", uuid.New().String()[:8])
	}

	// Set currency to GHS if not specified
	currency := req.CurrencyCode
	if currency == "" {
		currency = "GHS"
	}

	// Create MoMo payment request
	momoRequest := RequestToPayRequest{
		Amount:       fmt.Sprintf("%.2f", req.Amount),
		Currency:     currency,
		ExternalID:   referenceID,
		PayerMessage: req.Description,
		PayeeNote:    fmt.Sprintf("RevasPay payment: %s", req.Description),
		Payer: Payer{
			PartyIDType: "MSISDN",
			PartyID:     phoneNumber,
		},
	}

	// Send request to MoMo API
	transactionID, err := s.client.RequestToPay(momoRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate MoMo payment: %w", err)
	}

	// Create transaction record in database
	tx := database.MoMoTransaction{
		UserID:        req.UserID,
		TransactionID: transactionID,
		ReferenceID:   referenceID,
		PhoneNumber:   phoneNumber,
		Amount:        req.Amount,
		Currency:      currency,
		Description:   req.Description,
		Status:        string(MoMoStatusPending),
		CallbackURL:   req.CallbackURL,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.db.Create(&tx).Error; err != nil {
		return nil, fmt.Errorf("failed to save MoMo transaction: %w", err)
	}

	return &PaymentResponse{
		TransactionID: transactionID,
		ReferenceID:   referenceID,
		Status:        MoMoStatusPending,
		Message:       "Payment request initiated successfully",
	}, nil
}

// CheckPaymentStatus checks the status of a MoMo payment
func (s *MoMoService) CheckPaymentStatus(transactionID string) (*PaymentResponse, error) {
	// Get transaction from database
	var tx database.MoMoTransaction
	if err := s.db.Where("transaction_id = ?", transactionID).First(&tx).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("transaction not found: %s", transactionID)
		}
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	// Check status from MoMo API
	status, err := s.client.GetTransactionStatus(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to check payment status: %w", err)
	}

	// Update transaction status in database
	momoStatus := mapMoMoStatus(status.Status)
	if err := s.db.Model(&tx).Updates(map[string]interface{}{
		"status":       string(momoStatus),
		"updated_at":   time.Now(),
		"financial_id": status.FinancialTransactionID,
		"reason":       status.Reason,
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update transaction status: %w", err)
	}

	return &PaymentResponse{
		TransactionID: transactionID,
		ReferenceID:   tx.ReferenceID,
		Status:        momoStatus,
		Message:       status.Reason,
	}, nil
}

// DisbursementRequest represents a request to disburse funds via MoMo
type DisbursementRequest struct {
	UserID       uuid.UUID
	PhoneNumber  string
	Amount       float64
	Description  string
	ReferenceID  string
	CurrencyCode string
}

// DisbursementResponse represents the response from a disbursement request
type DisbursementResponse struct {
	TransactionID string
	ReferenceID   string
	Status        string
}

// DisbursePayment sends money to a mobile money user
func (s *MoMoService) DisbursePayment(req DisbursementRequest) (*DisbursementResponse, error) {
	// Format phone number to international format if needed
	phoneNumber := formatPhoneNumber(req.PhoneNumber)

	// Create a unique reference if not provided
	referenceID := req.ReferenceID
	if referenceID == "" {
		referenceID = fmt.Sprintf("RP-DISB-%s", uuid.New().String()[:8])
	}

	// Set currency to GHS if not specified
	currency := req.CurrencyCode
	if currency == "" {
		currency = "GHS"
	}

	// Create MoMo transfer request
	transferRequest := TransferRequest{
		Amount:       fmt.Sprintf("%.2f", req.Amount),
		Currency:     currency,
		ExternalID:   referenceID,
		PayerMessage: req.Description,
		PayeeNote:    fmt.Sprintf("RevasPay disbursement: %s", req.Description),
		Payee: Payer{
			PartyIDType: "MSISDN",
			PartyID:     phoneNumber,
		},
	}

	// Send request to MoMo API
	transactionID, err := s.client.Transfer(transferRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate MoMo disbursement: %w", err)
	}

	// Create a new disbursement record
	disbursement := database.MoMoDisbursement{
		ID:            uuid.New(),
		UserID:        req.UserID,
		TransactionID: transactionID,
		ReferenceID:   referenceID,
		Amount:        req.Amount,
		Currency:      "GHS",
		PhoneNumber:   phoneNumber,
		Status:        "PENDING",
		Description:   req.Description,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := s.db.Create(&disbursement).Error; err != nil {
		return nil, fmt.Errorf("failed to save MoMo disbursement: %w", err)
	}

	return &DisbursementResponse{
		TransactionID: transactionID,
		ReferenceID:   referenceID,
		Status:        "PENDING",
	}, nil
}

// CheckDisbursementStatus checks the status of a disbursement
func (s *MoMoService) CheckDisbursementStatus(transactionID string) (string, error) {
	// Find the disbursement in the database
	var disbursement database.MoMoDisbursement
	result := s.db.Where("transaction_id = ?", transactionID).First(&disbursement)
	if result.Error != nil {
		return "", result.Error
	}

	// If the status is not pending, return the current status
	if disbursement.Status != "PENDING" {
		return disbursement.Status, nil
	}

	// Check the status with the MoMo API
	status, err := s.client.GetTransferStatus(disbursement.ReferenceID)
	if err != nil {
		return "", fmt.Errorf("failed to check disbursement status: %w", err)
	}

	// Update disbursement status in database
	disbursement.Status = s.mapMoMoStatusToDisbursementStatus(status.Status)
	disbursement.UpdatedAt = time.Now()
	if err := s.db.Save(&disbursement).Error; err != nil {
		return "", fmt.Errorf("failed to update disbursement status: %w", err)
	}

	return disbursement.Status, nil
}

// GetBalance gets the account balance for both collection and disbursement
func (s *MoMoService) GetBalance() (map[string]string, error) {
	// Get collection balance
	collectionBalance, err := s.client.GetAccountBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get collection balance: %w", err)
	}

	// Get disbursement balance
	disbursementBalance, err := s.client.GetDisbursementBalance()
	if err != nil {
		return nil, fmt.Errorf("failed to get disbursement balance: %w", err)
	}

	return map[string]string{
		"collection":   collectionBalance.AvailableBalance,
		"disbursement": disbursementBalance.AvailableBalance,
		"currency":     collectionBalance.Currency,
	}, nil
}

// Helper function to format phone number to international format
func formatPhoneNumber(phone string) string {
	// If phone number starts with 0, replace with Ghana country code
	if len(phone) > 0 && phone[0] == '0' {
		return "233" + phone[1:]
	}

	// If phone number starts with +, remove the +
	if len(phone) > 0 && phone[0] == '+' {
		return phone[1:]
	}

	return phone
}

// Helper function to map MoMo status to our status enum
func mapMoMoStatus(status string) MoMoPaymentStatus {
	switch status {
	case "SUCCESSFUL":
		return MoMoStatusSuccessful
	case "FAILED":
		return MoMoStatusFailed
	case "REJECTED":
		return MoMoStatusFailed
	case "TIMEOUT":
		return MoMoStatusExpired
	case "ONGOING":
		return MoMoStatusOngoing
	case "PENDING":
		return MoMoStatusPending
	default:
		return MoMoStatusPending
	}
}
