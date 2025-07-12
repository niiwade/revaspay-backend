package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PaymentProvider represents supported payment providers
type PaymentProvider string

const (
	PaymentProviderPaystack    PaymentProvider = "paystack"
	PaymentProviderStripe      PaymentProvider = "stripe"
	PaymentProviderFlutterwave PaymentProvider = "flutterwave"
	PaymentProviderPayPal      PaymentProvider = "paypal"
	PaymentProviderCrypto      PaymentProvider = "crypto"
)

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

// PaymentLink represents a payment link for collecting payments
type PaymentLink struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID      uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	User        User           `gorm:"foreignKey:UserID" json:"-"`
	Title       string         `gorm:"type:varchar(255);not null" json:"title"`
	Description string         `gorm:"type:text" json:"description"`
	Amount      float64        `gorm:"type:decimal(20,8)" json:"amount"`
	Currency    Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	Slug        string         `gorm:"type:varchar(100);uniqueIndex" json:"slug"`
	Active      bool           `gorm:"default:true" json:"active"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	Metadata    JSON           `gorm:"type:jsonb" json:"metadata"`
	CreatedAt   time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// Payment represents a payment transaction
type Payment struct {
	ID              uuid.UUID       `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID          uuid.UUID       `gorm:"type:uuid;index" json:"user_id"`
	User            User            `gorm:"foreignKey:UserID" json:"-"`
	PaymentLinkID   *uuid.UUID      `gorm:"type:uuid;index" json:"payment_link_id,omitempty"`
	PaymentLink     *PaymentLink    `gorm:"foreignKey:PaymentLinkID" json:"-"`
	Amount          float64         `gorm:"type:decimal(20,8);not null" json:"amount"`
	Fee             float64         `gorm:"type:decimal(20,8);default:0" json:"fee"`
	Currency        Currency        `gorm:"type:varchar(3);not null" json:"currency"`
	Provider        PaymentProvider `gorm:"type:varchar(20);not null" json:"provider"`
	ProviderFee     float64         `gorm:"type:decimal(20,8);default:0" json:"provider_fee"`
	Status          PaymentStatus   `gorm:"type:varchar(20);not null" json:"status"`
	Reference       string          `gorm:"type:varchar(100);uniqueIndex" json:"reference"`
	ProviderRef     string          `gorm:"type:varchar(100)" json:"provider_ref"`
	CustomerEmail   string          `gorm:"type:varchar(255)" json:"customer_email"`
	CustomerName    string          `gorm:"type:varchar(255)" json:"customer_name"`
	PaymentMethod   string          `gorm:"type:varchar(50)" json:"payment_method"` // card, bank_transfer, mobile_money, crypto
	PaymentDetails  JSON            `gorm:"type:jsonb" json:"payment_details"`      // Card details, crypto tx hash, etc.
	Metadata        JSON            `gorm:"type:jsonb" json:"metadata"`
	ReceiptURL      string          `gorm:"type:varchar(255)" json:"receipt_url"`
	WebhookReceived bool            `gorm:"default:false" json:"webhook_received"`
	WebhookData     JSON            `gorm:"type:jsonb" json:"webhook_data"`
	CreatedAt       time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt       gorm.DeletedAt  `gorm:"index" json:"-"`
}

// PaymentWebhook represents a webhook received from a payment provider
type PaymentWebhook struct {
	ID          uuid.UUID       `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	Provider    PaymentProvider `gorm:"type:varchar(20);not null" json:"provider"`
	Event       string          `gorm:"type:varchar(100)" json:"event"`
	Reference   string          `gorm:"type:varchar(100);index" json:"reference"`
	PaymentID   *uuid.UUID      `gorm:"type:uuid;index" json:"payment_id,omitempty"`
	Payment     *Payment        `gorm:"foreignKey:PaymentID" json:"-"`
	RawData     JSON            `gorm:"type:jsonb" json:"raw_data"`
	Processed   bool            `gorm:"default:false" json:"processed"`
	ProcessedAt *time.Time      `json:"processed_at"`
	CreatedAt   time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// CryptoPayment represents a cryptocurrency payment
type CryptoPayment struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	PaymentID     uuid.UUID      `gorm:"type:uuid;index" json:"payment_id"`
	Payment       Payment        `gorm:"foreignKey:PaymentID" json:"-"`
	Network       string         `gorm:"type:varchar(50);not null" json:"network"` // ethereum, bitcoin, etc.
	Currency      string         `gorm:"type:varchar(10);not null" json:"currency"` // BTC, ETH, USDT, etc.
	Address       string         `gorm:"type:varchar(100);not null" json:"address"`
	Amount        string         `gorm:"type:varchar(50);not null" json:"amount"` // String to handle precise crypto amounts
	TxHash        string         `gorm:"type:varchar(100)" json:"tx_hash"`
	Confirmations int            `gorm:"default:0" json:"confirmations"`
	Status        PaymentStatus  `gorm:"type:varchar(20);not null" json:"status"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
