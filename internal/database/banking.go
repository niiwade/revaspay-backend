package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BankAccount represents a user's bank account
type BankAccount struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	AccountNumber string         `json:"account_number"`
	AccountName   string         `json:"account_name"`
	BankName      string         `json:"bank_name"`
	BankCode      string         `json:"bank_code"`
	BranchCode    string         `json:"branch_code"`
	Country       string         `json:"country"`
	Currency      string         `json:"currency"`
	IsVerified    bool           `gorm:"default:false" json:"is_verified"`
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// BankWalletLink connects a bank account to a crypto wallet
type BankWalletLink struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	BankAccountID uuid.UUID     `gorm:"type:uuid" json:"bank_account_id"`
	WalletID     uuid.UUID      `gorm:"type:uuid" json:"wallet_id"`
	Status       string         `json:"status"` // active, suspended
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// GhanaBankTransaction represents a transaction with a Ghanaian bank
type GhanaBankTransaction struct {
	ID                uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	BankAccountID     uuid.UUID      `gorm:"type:uuid" json:"bank_account_id"`
	TransactionType   string         `json:"transaction_type"` // deposit, withdrawal, international_payment
	Type              string         `json:"type"`             // send, receive
	Amount            float64        `json:"amount"`
	Fee               float64        `json:"fee"`
	Currency          string         `json:"currency"` // GHS
	Status            string         `json:"status"`   // pending, completed, failed
	Reference         string         `gorm:"uniqueIndex" json:"reference"`
	BankReference     string         `json:"bank_reference"`
	OnchainTxHash     string         `json:"onchain_tx_hash"`
	ComplianceDetails string         `json:"compliance_details"` // JSON string with compliance info
	Description       string         `json:"description"`       // Description of the transaction
	Error             string         `json:"error"`             // Error message if transaction failed
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	CompletedAt       *time.Time     `json:"completed_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// InternationalPayment represents a payment to an international vendor
type InternationalPayment struct {
	ID                uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	BankTransactionID uuid.UUID      `gorm:"type:uuid" json:"bank_transaction_id"`
	CryptoTxID        uuid.UUID      `gorm:"type:uuid" json:"crypto_tx_id"`
	VendorName        string         `json:"vendor_name"`
	VendorAddress     string         `json:"vendor_address"` // Blockchain address
	AmountCedis       float64        `json:"amount_cedis"`
	AmountCrypto      string         `json:"amount_crypto"` // String to preserve precision
	ExchangeRate      float64        `json:"exchange_rate"`
	Status            string         `json:"status"` // initiated, processing, completed, failed
	Description       string         `json:"description"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	CompletedAt       *time.Time     `json:"completed_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}
