package wallet

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"gorm.io/gorm"
)

// WalletService handles wallet operations
type WalletService struct {
	db *gorm.DB
}

// NewWalletService creates a new wallet service
func NewWalletService(db *gorm.DB) *WalletService {
	return &WalletService{db: db}
}

// GetOrCreateWallet gets a user's wallet or creates one if it doesn't exist
func (s *WalletService) GetOrCreateWallet(userID uuid.UUID, currency models.Currency) (*models.Wallet, error) {
	var wallet models.Wallet
	
	// Try to find existing wallet
	err := s.db.Where("user_id = ? AND currency = ?", userID, currency).First(&wallet).Error
	if err == nil {
		return &wallet, nil
	}
	
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("error finding wallet: %w", err)
	}
	
	// Create new wallet
	wallet = models.Wallet{
		UserID:    userID,
		Currency:  currency,
		Balance:   0,
		Available: 0,
	}
	
	if err := s.db.Create(&wallet).Error; err != nil {
		return nil, fmt.Errorf("error creating wallet: %w", err)
	}
	
	return &wallet, nil
}

// GetWallets gets all wallets for a user
func (s *WalletService) GetWallets(userID uuid.UUID) ([]models.Wallet, error) {
	var wallets []models.Wallet
	if err := s.db.Where("user_id = ?", userID).Find(&wallets).Error; err != nil {
		return nil, fmt.Errorf("error finding wallets: %w", err)
	}
	return wallets, nil
}

// GetWallet gets a specific wallet by ID
func (s *WalletService) GetWallet(walletID uuid.UUID) (*models.Wallet, error) {
	var wallet models.Wallet
	if err := s.db.First(&wallet, "id = ?", walletID).Error; err != nil {
		return nil, fmt.Errorf("error finding wallet: %w", err)
	}
	return &wallet, nil
}

// Credit adds funds to a wallet
func (s *WalletService) Credit(walletID uuid.UUID, amount float64, txType string, reference string, description string, metadata map[string]interface{}) (*models.Transaction, error) {
	var wallet models.Wallet
	
	// Use a transaction to ensure atomicity
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	
	// Get wallet with lock
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&wallet, "id = ?", walletID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error finding wallet: %w", err)
	}
	
	// Record balance before
	balanceBefore := wallet.Balance
	
	// Update wallet balance
	wallet.Balance += amount
	wallet.Available += amount
	if err := tx.Save(&wallet).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error updating wallet balance: %w", err)
	}
	
	// Create transaction record
	transaction := models.Transaction{
		WalletID:      walletID,
		Type:          txType,
		Amount:        amount,
		Currency:      wallet.Currency,
		Status:        "completed",
		Reference:     reference,
		Description:   description,
		MetaData:      metadata, // models.JSON is already a map[string]interface{}
		BalanceBefore: balanceBefore,
		BalanceAfter:  wallet.Balance,
	}
	
	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating transaction record: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}
	
	return &transaction, nil
}

// CreditWithTx adds funds to a wallet using an existing transaction
func (s *WalletService) CreditWithTx(tx *gorm.DB, walletID uuid.UUID, amount float64, txType string, reference string, description string, metadata map[string]interface{}) error {
	var wallet models.Wallet
	
	// Get wallet with lock
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&wallet, "id = ?", walletID).Error; err != nil {
		return fmt.Errorf("error finding wallet: %w", err)
	}
	
	// Record balance before
	balanceBefore := wallet.Balance
	
	// Update wallet balance
	wallet.Balance += amount
	wallet.Available += amount
	if err := tx.Save(&wallet).Error; err != nil {
		return fmt.Errorf("error updating wallet balance: %w", err)
	}
	
	// Create transaction record
	transaction := models.Transaction{
		WalletID:      walletID,
		Type:          txType,
		Amount:        amount,
		Currency:      wallet.Currency,
		Status:        "completed",
		Reference:     reference,
		Description:   description,
		MetaData:      metadata, // models.JSON is already a map[string]interface{}
		BalanceBefore: balanceBefore,
		BalanceAfter:  wallet.Balance,
	}
	
	if err := tx.Create(&transaction).Error; err != nil {
		return fmt.Errorf("error creating transaction record: %w", err)
	}
	
	return nil
}

// Debit removes funds from a wallet
func (s *WalletService) Debit(walletID uuid.UUID, amount float64, txType string, reference string, description string, metadata map[string]interface{}) (*models.Transaction, error) {
	var wallet models.Wallet
	
	// Use a transaction to ensure atomicity
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	
	// Get wallet with lock
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&wallet, "id = ?", walletID).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error finding wallet: %w", err)
	}
	
	// Check if sufficient funds
	if wallet.Available < amount {
		tx.Rollback()
		return nil, errors.New("insufficient funds")
	}
	
	// Record balance before
	balanceBefore := wallet.Balance
	
	// Update wallet balance
	wallet.Balance -= amount
	wallet.Available -= amount
	if err := tx.Save(&wallet).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error updating wallet balance: %w", err)
	}
	
	// Create transaction record
	transaction := models.Transaction{
		WalletID:      walletID,
		Type:          txType,
		Amount:        -amount, // Negative for debit
		Currency:      wallet.Currency,
		Status:        "completed",
		Reference:     reference,
		Description:   description,
		MetaData:      metadata, // models.JSON is already a map[string]interface{}
		BalanceBefore: balanceBefore,
		BalanceAfter:  wallet.Balance,
	}
	
	if err := tx.Create(&transaction).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("error creating transaction record: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("error committing transaction: %w", err)
	}
	
	return &transaction, nil
}

// GetTransactionHistory gets transaction history for a wallet
func (s *WalletService) GetTransactionHistory(walletID uuid.UUID, page, pageSize int) ([]models.Transaction, int64, error) {
	var transactions []models.Transaction
	var total int64
	
	// Count total records
	if err := s.db.Model(&models.Transaction{}).Where("wallet_id = ?", walletID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("error counting transactions: %w", err)
	}
	
	// Get paginated records
	offset := (page - 1) * pageSize
	if err := s.db.Where("wallet_id = ?", walletID).Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&transactions).Error; err != nil {
		return nil, 0, fmt.Errorf("error finding transactions: %w", err)
	}
	
	return transactions, total, nil
}

// GetAutoWithdrawConfig gets auto-withdraw configuration for a user
func (s *WalletService) GetAutoWithdrawConfig(userID uuid.UUID) (*models.AutoWithdrawConfig, error) {
	var config models.AutoWithdrawConfig
	if err := s.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No config found, not an error
		}
		return nil, fmt.Errorf("error finding auto-withdraw config: %w", err)
	}
	return &config, nil
}

// UpdateAutoWithdrawConfig updates or creates auto-withdraw configuration for a user
func (s *WalletService) UpdateAutoWithdrawConfig(userID uuid.UUID, enabled bool, threshold float64, currency models.Currency, withdrawMethod string, destinationID uuid.UUID) (*models.AutoWithdrawConfig, error) {
	var config models.AutoWithdrawConfig
	
	// Try to find existing config
	result := s.db.Where("user_id = ?", userID).First(&config)
	
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// Create new config
			config = models.AutoWithdrawConfig{
				UserID:         userID,
				Enabled:        enabled,
				Threshold:      threshold,
				Currency:       currency,
				WithdrawMethod: withdrawMethod,
				DestinationID:  destinationID,
			}
			if err := s.db.Create(&config).Error; err != nil {
				return nil, fmt.Errorf("error creating auto-withdraw config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("error finding auto-withdraw config: %w", result.Error)
		}
	} else {
		// Update existing config
		config.Enabled = enabled
		config.Threshold = threshold
		config.Currency = currency
		config.WithdrawMethod = withdrawMethod
		config.DestinationID = destinationID
		
		if err := s.db.Save(&config).Error; err != nil {
			return nil, fmt.Errorf("error updating auto-withdraw config: %w", err)
		}
	}
	
	return &config, nil
}
