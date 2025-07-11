package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CryptoWallet represents a user's blockchain wallet
type CryptoWallet struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	Address      string         `json:"address"`
	WalletType   string         `json:"wallet_type"` // BASE, ETH, BTC, etc.
	Network      string         `json:"network"`     // base_mainnet, base_testnet, etc.
	EncryptedKey string         `json:"-"`           // Never expose this
	IsActive     bool           `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// CryptoTransaction represents an on-chain transaction
type CryptoTransaction struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID                uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	WalletID              uuid.UUID      `gorm:"type:uuid" json:"wallet_id"`
	TransactionHash       string         `json:"transaction_hash"`
	FromAddress           string         `json:"from_address"`
	ToAddress             string         `json:"to_address"`
	Amount                string         `json:"amount"`           // Store as string to preserve precision
	Currency              string         `json:"currency"`         // ETH, BTC, etc.
	TokenSymbol           string         `json:"token_symbol"`     // USDC, USDT, etc.
	Type                  string         `json:"type"`             // send, receive
	Status                string         `json:"status"`           // created, pending, confirmed, failed
	BlockNumber           uint64         `json:"block_number"`
	BlockHash             string         `json:"block_hash"`
	GasUsed               uint64         `json:"gas_used"`
	NetworkFee            string         `json:"network_fee"`      // Gas fees
	RecipientAddress      string         `json:"recipient_address"` // For payments to vendors
	InternationalPaymentID uuid.UUID      `gorm:"type:uuid" json:"international_payment_id"`
	Error                 string         `json:"error"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	ConfirmedAt           *time.Time     `json:"confirmed_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// TokenBalance represents a user's token balance
type TokenBalance struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid" json:"user_id"`
	WalletID  uuid.UUID      `gorm:"type:uuid" json:"wallet_id"`
	TokenName string         `json:"token_name"` // USDC, USDT, etc.
	Symbol    string         `json:"symbol"`
	Balance   string         `json:"balance"` // Store as string to preserve precision
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
