package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VirtualAccountProvider represents the provider of a virtual account
type VirtualAccountProvider string

const (
	// VirtualAccountProviderGrey represents Grey as a virtual account provider
	VirtualAccountProviderGrey VirtualAccountProvider = "grey"
	// VirtualAccountProviderWise represents Wise as a virtual account provider
	VirtualAccountProviderWise VirtualAccountProvider = "wise"
	// VirtualAccountProviderBarter represents Barter as a virtual account provider
	VirtualAccountProviderBarter VirtualAccountProvider = "barter"
)

// VirtualAccountStatus represents the status of a virtual account
type VirtualAccountStatus string

const (
	// VirtualAccountStatusPending represents a pending virtual account
	VirtualAccountStatusPending VirtualAccountStatus = "pending"
	// VirtualAccountStatusActive represents an active virtual account
	VirtualAccountStatusActive VirtualAccountStatus = "active"
	// VirtualAccountStatusInactive represents an inactive virtual account
	VirtualAccountStatusInactive VirtualAccountStatus = "inactive"
	// VirtualAccountStatusFailed represents a failed virtual account
	VirtualAccountStatusFailed VirtualAccountStatus = "failed"
)

// VirtualAccountCurrency represents the currency of a virtual account
type VirtualAccountCurrency string

const (
	// VirtualAccountCurrencyUSD represents USD currency
	VirtualAccountCurrencyUSD VirtualAccountCurrency = "USD"
	// VirtualAccountCurrencyEUR represents EUR currency
	VirtualAccountCurrencyEUR VirtualAccountCurrency = "EUR"
	// VirtualAccountCurrencyGBP represents GBP currency
	VirtualAccountCurrencyGBP VirtualAccountCurrency = "GBP"
)

// VirtualAccount represents a virtual account for receiving payments
type VirtualAccount struct {
	ID               uuid.UUID             `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID           uuid.UUID             `gorm:"type:uuid;not null" json:"user_id"`
	User             User                  `gorm:"foreignKey:UserID" json:"-"`
	Provider         VirtualAccountProvider `gorm:"type:varchar(20);not null" json:"provider"`
	ProviderAccountID string               `gorm:"type:varchar(255)" json:"provider_account_id"`
	AccountNumber    string               `gorm:"type:varchar(50)" json:"account_number"`
	RoutingNumber    string               `gorm:"type:varchar(50)" json:"routing_number"`
	IBAN             string               `gorm:"type:varchar(50)" json:"iban"`
	SwiftCode        string               `gorm:"type:varchar(20)" json:"swift_code"`
	BankName         string               `gorm:"type:varchar(255)" json:"bank_name"`
	BankAddress      string               `gorm:"type:text" json:"bank_address"`
	AccountName      string               `gorm:"type:varchar(255)" json:"account_name"`
	Currency         VirtualAccountCurrency `gorm:"type:varchar(3);not null" json:"currency"`
	Status           VirtualAccountStatus  `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	Balance          float64              `gorm:"type:decimal(20,2);default:0" json:"balance"`
	LastSyncedAt     *time.Time           `json:"last_synced_at"`
	ProviderData     string               `gorm:"type:jsonb" json:"provider_data"`
	CreatedAt        time.Time            `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt        time.Time            `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt        gorm.DeletedAt       `gorm:"index" json:"-"`
}
