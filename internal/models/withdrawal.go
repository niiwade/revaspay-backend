package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Withdrawal represents a withdrawal request
type Withdrawal struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID        uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	User          User           `gorm:"foreignKey:UserID" json:"-"`
	WalletID      uuid.UUID      `gorm:"type:uuid;index" json:"wallet_id"`
	Wallet        Wallet         `gorm:"foreignKey:WalletID" json:"-"`
	Amount        float64        `gorm:"type:decimal(20,8);not null" json:"amount"`
	Currency      Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	Method        string         `gorm:"type:varchar(50);not null" json:"method"` // bank, mobile_money, crypto
	DestinationID uuid.UUID      `gorm:"type:uuid" json:"destination_id"`         // ID of bank account, mobile money, or crypto address
	Status        string         `gorm:"type:varchar(20);not null" json:"status"` // pending, processing, completed, failed
	Reference     string         `gorm:"type:varchar(100)" json:"reference"`
	Description   string         `gorm:"type:text" json:"description"`
	MetaData      JSON           `gorm:"type:jsonb" json:"metadata"`
	ProcessingFee float64        `gorm:"type:decimal(20,8);default:0" json:"processing_fee"`
	InitiatedAt   time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"initiated_at"`
	ProcessedAt   *time.Time     `json:"processed_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	FailedAt      *time.Time     `json:"failed_at"`
	FailureReason string         `gorm:"type:text" json:"failure_reason"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// WithdrawalHistory represents the history of a withdrawal's status changes
type WithdrawalHistory struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	WithdrawalID uuid.UUID `gorm:"type:uuid;index" json:"withdrawal_id"`
	Withdrawal   Withdrawal `gorm:"foreignKey:WithdrawalID" json:"-"`
	Status       string    `gorm:"type:varchar(20);not null" json:"status"`
	Notes        string    `gorm:"type:text" json:"notes"`
	ChangedBy    uuid.UUID `gorm:"type:uuid" json:"changed_by"` // User ID who made the change
	CreatedAt    time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
}
