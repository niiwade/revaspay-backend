package payment

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/crypto"
	"github.com/revaspay/backend/internal/services/exchange"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// InternationalPaymentService handles international payments using Base blockchain
type InternationalPaymentService struct {
	db              *gorm.DB
	baseService     *crypto.BaseService
	exchangeService *exchange.ExchangeRateService
	queue           *queue.Queue
}

// NewInternationalPaymentService creates a new international payment service
func NewInternationalPaymentService(db *gorm.DB) *InternationalPaymentService {
	return &InternationalPaymentService{
		db:              db,
		baseService:     crypto.NewBaseService(db),
		exchangeService: exchange.NewExchangeRateService(), // Using free ExchangeRate-API (no API key needed)
		queue:           queue.NewQueue(db),
	}
}

// PaymentRequest represents a request to make an international payment
type PaymentRequest struct {
	VendorName    string  `json:"vendor_name"`
	VendorAddress string  `json:"vendor_address"`
	Amount        float64 `json:"amount"`
	Description   string  `json:"description"`
}

// ProcessPayment handles the full payment flow from bank to blockchain
func (s *InternationalPaymentService) ProcessPayment(userID uuid.UUID, req PaymentRequest) (*database.InternationalPayment, error) {
	// Start database transaction
	tx := s.db.Begin()

	// Get user's bank account
	var bankAccount database.BankAccount
	if err := tx.Where("user_id = ? AND is_active = ? AND country = ?", userID, true, "Ghana").First(&bankAccount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("no active Ghanaian bank account found: %v", err)
	}

	// Get user's Base wallet
	var wallet database.CryptoWallet
	if err := tx.Where("user_id = ? AND wallet_type = ? AND is_active = ?", userID, "BASE", true).First(&wallet).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("no active Base wallet found: %v", err)
	}

	// Create a unique reference for this payment
	reference := utils.GenerateReference("IP")

	// Create bank transaction record
	bankTx := database.GhanaBankTransaction{
		UserID:          userID,
		BankAccountID:   bankAccount.ID,
		TransactionType: "international_payment",
		Amount:          req.Amount,
		Fee:             0, // No conversion fee as per requirements
		Currency:        "GHS",
		Status:          "pending",
		Reference:       reference,
	}

	if err := tx.Create(&bankTx).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Create crypto transaction record (will be updated with actual hash later)
	cryptoTx := database.CryptoTransaction{
		UserID:      userID,
		WalletID:    wallet.ID,
		FromAddress: wallet.Address,
		ToAddress:   req.VendorAddress,
		Amount:      fmt.Sprintf("%f", req.Amount), // This would be converted to crypto amount in production
		Currency:    "USDC",                        // Assuming USDC stablecoin
		Status:      "pending",
	}

	if err := tx.Create(&cryptoTx).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Get real-time exchange rate from GHS to USDC (using USD as proxy)
	exchangeRate, err := s.exchangeService.GetExchangeRate("GHS", "USD")
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to get exchange rate: %w", err)
	}

	// Convert amount from GHS to USDC
	amountCrypto, err := s.exchangeService.ConvertAmount(req.Amount, "GHS", "USD")
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to convert amount: %w", err)
	}

	// Create international payment record
	payment := database.InternationalPayment{
		UserID:            userID,
		BankTransactionID: bankTx.ID,
		CryptoTxID:        cryptoTx.ID,
		VendorName:        req.VendorName,
		VendorAddress:     req.VendorAddress,
		AmountCedis:       req.Amount,
		AmountCrypto:      fmt.Sprintf("%f", amountCrypto),
		ExchangeRate:      exchangeRate,
		Status:            "initiated",
		Description:       req.Description,
	}

	if err := tx.Create(&payment).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Queue payment processing job
	payload := queue.ProcessPaymentPayload{
		PaymentID:        payment.ID,
		UserID:           userID,
		BankAccountID:    bankAccount.ID,
		WalletID:         wallet.ID,
		RecipientAddress: req.VendorAddress,
		Amount:           req.Amount,
		Currency:         "USDC",
		Description:      req.Description,
		Reference:        reference,
	}

	jobID, err := s.queue.EnqueueJob(queue.JobTypeProcessPayment, payload)
	if err != nil {
		// Update payment status to failed if we couldn't queue the job
		s.updatePaymentStatus(payment.ID, "failed", fmt.Sprintf("Failed to queue payment job: %v", err))
		return nil, fmt.Errorf("failed to queue payment job: %w", err)
	}

	// Update payment with job ID
	if err := s.db.Model(&payment).Updates(map[string]interface{}{
		"status":     "queued",
		"updated_at": time.Now(),
		"reference":  fmt.Sprintf("%s:job:%s", reference, jobID),
	}).Error; err != nil {
		return nil, fmt.Errorf("failed to update payment with job ID: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return &payment, nil
}

// updatePaymentStatus updates the status of a payment
func (s *InternationalPaymentService) updatePaymentStatus(paymentID uuid.UUID, status string, reason string) {
	s.db.Model(&database.InternationalPayment{}).Where("id = ?", paymentID).Updates(map[string]interface{}{
		"status":     status,
		"reason":     reason,
		"updated_at": time.Now(),
	})
}

// GetPayment retrieves a specific international payment
func (s *InternationalPaymentService) GetPayment(paymentID uuid.UUID) (*database.InternationalPayment, error) {
	var payment database.InternationalPayment
	if err := s.db.First(&payment, paymentID).Error; err != nil {
		return nil, err
	}
	return &payment, nil
}

// GetPayments retrieves all international payments for a user
func (s *InternationalPaymentService) GetPayments(userID uuid.UUID) ([]database.InternationalPayment, error) {
	var payments []database.InternationalPayment
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&payments).Error; err != nil {
		return nil, err
	}
	return payments, nil
}
