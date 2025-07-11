package banking

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/crypto"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// BankAccountDetails holds the details for a bank account
type BankAccountDetails struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name"`
	BankCode      string `json:"bank_code"`
	BranchCode    string `json:"branch_code"`
}

// GhanaBankingService handles interactions with Ghanaian banks
type GhanaBankingService struct {
	db          *gorm.DB
	baseService *crypto.BaseService
}

// NewGhanaBankingService creates a new Ghana banking service
func NewGhanaBankingService(db *gorm.DB) *GhanaBankingService {
	return &GhanaBankingService{
		db:          db,
		baseService: crypto.NewBaseService(db),
	}
}

// LinkBankAccount connects a user's Ghanaian bank account to their RevasPay account
func (s *GhanaBankingService) LinkBankAccount(userID uuid.UUID, bankDetails BankAccountDetails) (*database.BankAccount, error) {
	// Verify bank account details with Ghana banking API
	// In production, this would call an actual API
	verified, err := s.verifyGhanaianBankAccount(bankDetails)
	if err != nil || !verified {
		return nil, fmt.Errorf("bank account verification failed: %v", err)
	}

	// Start transaction
	tx := s.db.Begin()

	// Create bank account record
	bankAccount := &database.BankAccount{
		UserID:        userID,
		AccountNumber: bankDetails.AccountNumber,
		AccountName:   bankDetails.AccountName,
		BankName:      bankDetails.BankName,
		BankCode:      bankDetails.BankCode,
		BranchCode:    bankDetails.BranchCode,
		Country:       "Ghana",
		Currency:      "GHS",
		IsVerified:    true,
		IsActive:      true,
	}

	if err := tx.Create(bankAccount).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Check if user already has a Base wallet
	var wallet database.CryptoWallet
	result := tx.Where("user_id = ? AND wallet_type = ?", userID, "BASE").First(&wallet)

	// If no wallet exists, create one
	if result.RowsAffected == 0 {
		newWallet, err := s.baseService.CreateBaseWallet(userID)
		if err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create Base wallet: %v", err)
		}
		wallet = *newWallet
	}

	// Create bank-wallet link
	link := &database.BankWalletLink{
		UserID:       userID,
		BankAccountID: bankAccount.ID,
		WalletID:     wallet.ID,
		Status:       "active",
	}

	if err := tx.Create(link).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return bankAccount, nil
}

// GetBankAccounts retrieves all bank accounts for a user
func (s *GhanaBankingService) GetBankAccounts(userID uuid.UUID) ([]database.BankAccount, error) {
	var accounts []database.BankAccount
	if err := s.db.Where("user_id = ?", userID).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

// ProcessInternationalPayment handles payments to international vendors using Cedis
func (s *GhanaBankingService) ProcessInternationalPayment(
	userID uuid.UUID,
	vendorName string,
	vendorAddress string,
	amountCedis float64,
	description string,
) (*database.InternationalPayment, error) {
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

	// Generate reference
	reference := utils.GenerateReference("INTL")

	// Create bank transaction record
	bankTx := database.GhanaBankTransaction{
		UserID:          userID,
		BankAccountID:   bankAccount.ID,
		TransactionType: "international_payment",
		Amount:          amountCedis,
		Fee:             0, // No conversion fee as per requirements
		Currency:        "GHS",
		Status:          "pending",
		Reference:       reference,
		ComplianceDetails: s.generateComplianceDetails(userID, vendorAddress, amountCedis),
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
		ToAddress:   vendorAddress,
		Amount:      fmt.Sprintf("%f", amountCedis), // This would be converted to crypto amount in production
		Currency:    "USDC",                         // Assuming USDC stablecoin
		Status:      "pending",
	}

	if err := tx.Create(&cryptoTx).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Create international payment record
	payment := database.InternationalPayment{
		UserID:            userID,
		BankTransactionID: bankTx.ID,
		CryptoTxID:        cryptoTx.ID,
		VendorName:        vendorName,
		VendorAddress:     vendorAddress,
		AmountCedis:       amountCedis,
		AmountCrypto:      fmt.Sprintf("%f", amountCedis), // This would be converted in production
		ExchangeRate:      1.0,                            // This would be actual rate in production
		Status:            "initiated",
		Description:       description,
	}

	if err := tx.Create(&payment).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// In production, we would now trigger an async job to:
	// 1. Process the bank transaction
	// 2. Execute the on-chain transaction
	// 3. Update the records with the results

	return &payment, nil
}

// GetInternationalPayments retrieves all international payments for a user
func (s *GhanaBankingService) GetInternationalPayments(userID uuid.UUID) ([]database.InternationalPayment, error) {
	var payments []database.InternationalPayment
	if err := s.db.Where("user_id = ?", userID).Find(&payments).Error; err != nil {
		return nil, err
	}
	return payments, nil
}

// Helper methods

// verifyGhanaianBankAccount verifies a bank account with Ghana banking API
// In production, this would call an actual API
func (s *GhanaBankingService) verifyGhanaianBankAccount(details BankAccountDetails) (bool, error) {
	// Simulate verification
	// In production, this would call the actual Ghana banking API
	return true, nil
}

// generateComplianceDetails creates a JSON string with compliance information
func (s *GhanaBankingService) generateComplianceDetails(userID uuid.UUID, vendorAddress string, amount float64) string {
	// In production, this would include actual compliance checks
	details := map[string]interface{}{
		"timestamp":      time.Now().Format(time.RFC3339),
		"user_id":        userID.String(),
		"vendor_address": vendorAddress,
		"amount":         amount,
		"currency":       "GHS",
		"checks": map[string]interface{}{
			"aml_check":      "passed",
			"kyc_verified":   true,
			"limit_check":    "within_limits",
			"sanction_check": "not_sanctioned",
		},
	}

	jsonData, err := json.Marshal(details)
	if err != nil {
		return "{}"
	}
	return string(jsonData)
}
