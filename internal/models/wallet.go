package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Currency represents supported currencies
type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
	CurrencyGBP Currency = "GBP"
	CurrencyNGN Currency = "NGN"
	CurrencyGHS Currency = "GHS"
	CurrencyKES Currency = "KES"
	CurrencyZAR Currency = "ZAR"
)

// Wallet represents a user's wallet
type Wallet struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"-"`
	Currency  Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	Balance   float64        `gorm:"type:decimal(20,8);default:0" json:"balance"`
	Available float64        `gorm:"type:decimal(20,8);default:0" json:"available"` // Available balance (excluding pending)
	CreatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Transaction represents a wallet transaction
type Transaction struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	WalletID      uuid.UUID      `gorm:"type:uuid;index" json:"wallet_id"`
	Wallet        Wallet         `gorm:"foreignKey:WalletID" json:"-"`
	Type          string         `gorm:"type:varchar(50);not null" json:"type"` // deposit, withdrawal, transfer, fee
	Amount        float64        `gorm:"type:decimal(20,8);not null" json:"amount"`
	Fee           float64        `gorm:"type:decimal(20,8);default:0" json:"fee"`
	Currency      Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	Status        string         `gorm:"type:varchar(20);not null" json:"status"` // pending, completed, failed
	Reference     string         `gorm:"type:varchar(100)" json:"reference"`
	Description   string         `gorm:"type:text" json:"description"`
	MetaData      JSON           `gorm:"type:jsonb" json:"metadata"`
	BalanceBefore float64        `gorm:"type:decimal(20,8)" json:"balance_before"`
	BalanceAfter  float64        `gorm:"type:decimal(20,8)" json:"balance_after"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// AutoWithdrawConfig represents a user's auto-withdraw configuration
type AutoWithdrawConfig struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID         uuid.UUID      `gorm:"type:uuid;uniqueIndex" json:"user_id"`
	User           User           `gorm:"foreignKey:UserID" json:"-"`
	Enabled        bool           `gorm:"default:false" json:"enabled"`
	Threshold      float64        `gorm:"type:decimal(20,8);default:100" json:"threshold"`
	Currency       Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	WithdrawMethod string         `gorm:"type:varchar(50)" json:"withdraw_method"` // bank, mobile_money, crypto
	DestinationID  uuid.UUID      `gorm:"type:uuid" json:"destination_id"`         // ID of bank account, mobile money, or crypto address
	CreatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
