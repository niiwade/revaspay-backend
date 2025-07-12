package models

import (
	"time"

	"github.com/google/uuid"
)

// MoMoTransactionType represents the type of MTN Mobile Money transaction
type MoMoTransactionType string

const (
	MoMoTransactionTypeCollection   MoMoTransactionType = "collection"   // Receiving money from customer
	MoMoTransactionTypeDisbursement MoMoTransactionType = "disbursement" // Sending money to customer
)

// MoMoTransactionStatus represents the status of an MTN Mobile Money transaction
type MoMoTransactionStatus string

const (
	MoMoTransactionStatusPending   MoMoTransactionStatus = "pending"
	MoMoTransactionStatusSucceeded MoMoTransactionStatus = "succeeded"
	MoMoTransactionStatusFailed    MoMoTransactionStatus = "failed"
	MoMoTransactionStatusExpired   MoMoTransactionStatus = "expired"
	MoMoTransactionStatusCancelled MoMoTransactionStatus = "cancelled"
)

// MoMoTransaction represents an MTN Mobile Money transaction
type MoMoTransaction struct {
	Base
	UserID          uuid.UUID            `gorm:"type:uuid;index" json:"user_id"`
	User            User                 `gorm:"foreignKey:UserID" json:"-"`
	Type            MoMoTransactionType  `gorm:"type:varchar(20);not null" json:"type"`
	Status          MoMoTransactionStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Amount          float64              `gorm:"type:decimal(20,8);not null" json:"amount"`
	Currency        Currency             `gorm:"type:varchar(3);not null;default:'GHS'" json:"currency"`
	PhoneNumber     string               `gorm:"type:varchar(20);not null" json:"phone_number"`
	CountryCode     string               `gorm:"type:varchar(5);not null;default:'GH'" json:"country_code"`
	Reference       string               `gorm:"type:varchar(100);uniqueIndex" json:"reference"`
	ExternalID      string               `gorm:"type:varchar(100)" json:"external_id"`
	PayerMessage    string               `gorm:"type:varchar(255)" json:"payer_message"`
	PayeeNote       string               `gorm:"type:varchar(255)" json:"payee_note"`
	FinancialID     string               `gorm:"type:varchar(100)" json:"financial_id"`
	Reason          string               `gorm:"type:varchar(255)" json:"reason"`
	Fee             float64              `gorm:"type:decimal(20,8);default:0" json:"fee"`
	Description     string               `gorm:"type:text" json:"description"`
	PaymentID       *uuid.UUID           `gorm:"type:uuid;index" json:"payment_id,omitempty"`
	Payment         *Payment             `gorm:"foreignKey:PaymentID" json:"-"`
	WithdrawalID    *uuid.UUID           `gorm:"type:uuid;index" json:"withdrawal_id,omitempty"`
	Withdrawal      *Withdrawal          `gorm:"foreignKey:WithdrawalID" json:"-"`
	RequestData     JSON                 `gorm:"type:jsonb" json:"request_data"`
	ResponseData    JSON                 `gorm:"type:jsonb" json:"response_data"`
	WebhookData     JSON                 `gorm:"type:jsonb" json:"webhook_data"`
	WebhookReceived bool                 `gorm:"default:false" json:"webhook_received"`
	CompletedAt     *time.Time           `json:"completed_at"`
	FailedAt        *time.Time           `json:"failed_at"`
	ErrorMessage    string               `gorm:"type:text" json:"error_message"`
	RetryCount      int                  `gorm:"default:0" json:"retry_count"`
	LastRetryAt     *time.Time           `json:"last_retry_at"`
	Metadata        JSON                 `gorm:"type:jsonb" json:"metadata"`
}
// Note: Withdrawal model is defined in models.go
