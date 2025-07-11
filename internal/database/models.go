package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a creator or admin user in the system
type User struct {
	ID               uuid.UUID         `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	Username         string            `gorm:"uniqueIndex" json:"username"`
	Email            string            `gorm:"uniqueIndex;not null" json:"email"`
	Password         string            `gorm:"not null" json:"-"`
	FirstName        string            `json:"first_name"`
	LastName         string            `json:"last_name"`
	DisplayName      string            `json:"display_name"`
	ProfilePicURL    string            `json:"profile_pic_url"`
	ProfileImage     string            `json:"profile_image"`
	Bio              string            `json:"bio"`
	PhoneNumber      string            `json:"phone_number"`
	CountryCode      string            `json:"country_code"`
	BusinessName     string            `json:"business_name"`
	Website          string            `json:"website"`
	SocialLinks      map[string]string `gorm:"type:jsonb" json:"social_links"`
	IsVerified       bool              `gorm:"default:false" json:"is_verified"`
	Verified         bool              `gorm:"default:false" json:"verified"`
	EmailVerifiedAt  *time.Time        `json:"email_verified_at"`
	IsAdmin          bool              `gorm:"default:false" json:"is_admin"`
	TwoFactorEnabled bool              `gorm:"default:false" json:"two_factor_enabled"`
	TwoFactorSecret  string            `json:"-"`
	LastLoginAt      *time.Time        `json:"last_login_at"`
	PasswordReset    bool              `gorm:"default:false" json:"password_reset"`
	ReferralCode     string            `gorm:"uniqueIndex" json:"referral_code"`
	ReferredBy       *uuid.UUID        `gorm:"type:uuid" json:"referred_by"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        gorm.DeletedAt    `gorm:"index" json:"-"`

	// Relationships
	Wallet          Wallet           `json:"wallet"`
	KYC             KYC              `json:"kyc"`
	PaymentLinks    []PaymentLink    `json:"payment_links,omitempty"`
	Withdrawals     []Withdrawal     `json:"withdrawals,omitempty"`
	Subscriptions   []Subscription   `json:"subscriptions,omitempty"`
	VirtualAccounts []VirtualAccount `json:"virtual_accounts,omitempty"`
	Referrals       []Referral       `json:"referrals,omitempty"`
}

// KYC represents the Know Your Customer verification for a user
type KYC struct {
	ID              uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID          uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	IDType          string     `json:"id_type"` // passport, national_id, drivers_license
	IDNumber        string     `json:"id_number"`
	IDFrontURL      string     `json:"id_front_url"`
	IDBackURL       string     `json:"id_back_url"`
	SelfieURL       string     `json:"selfie_url"`
	Status          string     `json:"status"` // pending, approved, rejected
	RejectionReason string     `json:"rejection_reason"`
	VerifiedAt      *time.Time `json:"verified_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Wallet represents a user's balance
type Wallet struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID `gorm:"type:uuid;uniqueIndex" json:"user_id"`
	Balance           float64   `json:"balance"`
	Currency          string    `gorm:"default:USD" json:"currency"`
	AutoWithdraw      bool      `gorm:"default:false" json:"auto_withdraw"`
	WithdrawThreshold float64   `json:"withdraw_threshold"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Relationships
	Transactions []Transaction `json:"transactions,omitempty"`
}

// Transaction represents a financial transaction in the system
type Transaction struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	WalletID          uuid.UUID `gorm:"type:uuid" json:"wallet_id"`
	Type              string    `json:"type"` // deposit, withdrawal, refund, fee
	Amount            float64   `json:"amount"`
	Fee               float64   `json:"fee"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"` // pending, completed, failed
	Reference         string    `gorm:"uniqueIndex" json:"reference"`
	Description       string    `json:"description"`
	MetaData          string    `json:"meta_data"`          // JSON string for additional data
	ProviderReference string    `json:"provider_reference"` // Reference from payment provider
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PaymentLink represents a payment link created by a user
type PaymentLink struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Amount      float64    `json:"amount"`
	Currency    string     `json:"currency"`
	Type        string     `json:"type"` // one_time, donation
	Slug        string     `gorm:"uniqueIndex" json:"slug"`
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Withdrawal represents a withdrawal request from a user
type Withdrawal struct {
	ID                uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID  `gorm:"type:uuid" json:"user_id"`
	Amount            float64    `json:"amount"`
	Fee               float64    `json:"fee"`
	Currency          string     `json:"currency"`
	Method            string     `json:"method"` // bank_transfer, mobile_money, crypto
	Status            string     `json:"status"` // pending, processing, completed, failed
	Reference         string     `gorm:"uniqueIndex" json:"reference"`
	AccountDetails    string     `json:"account_details"` // JSON string with account info
	ProviderReference string     `json:"provider_reference"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ProcessedAt       *time.Time `json:"processed_at"`
}

// SubscriptionPlan represents a recurring payment plan created by a user
type SubscriptionPlan struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Interval    string    `json:"interval"` // monthly, quarterly, yearly
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relationships
	Subscriptions []Subscription `json:"subscriptions,omitempty"`
}

// Subscription represents a user's subscription to a plan
type Subscription struct {
	ID                   uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	PlanID               uuid.UUID  `gorm:"type:uuid" json:"plan_id"`
	UserID               uuid.UUID  `gorm:"type:uuid" json:"user_id"` // Creator
	SubscriberEmail      string     `json:"subscriber_email"`
	SubscriberName       string     `json:"subscriber_name"`
	Status               string     `json:"status"` // active, canceled, expired
	CurrentPeriodStart   time.Time  `json:"current_period_start"`
	CurrentPeriodEnd     time.Time  `json:"current_period_end"`
	CancelAtPeriodEnd    bool       `gorm:"default:false" json:"cancel_at_period_end"`
	PaymentMethod        string     `json:"payment_method"`
	PaymentMethodDetails string     `json:"payment_method_details"` // JSON string
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CanceledAt           *time.Time `json:"canceled_at"`
}

// VirtualAccount represents a virtual bank account for receiving payments
type VirtualAccount struct {
	ID            uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Provider      string    `json:"provider"` // grey, wise, barter
	AccountNumber string    `json:"account_number"`
	AccountName   string    `json:"account_name"`
	BankName      string    `json:"bank_name"`
	Currency      string    `json:"currency"`
	Country       string    `json:"country"`
	IsActive      bool      `gorm:"default:true" json:"is_active"`
	ProviderData  string    `json:"provider_data"` // JSON string with provider details
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Referral represents a user referral
type Referral struct {
	ID         uuid.UUID  `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	ReferrerID uuid.UUID  `gorm:"type:uuid" json:"referrer_id"`
	ReferredID uuid.UUID  `gorm:"type:uuid" json:"referred_id"`
	Status     string     `json:"status"` // pending, qualified, paid
	Commission float64    `json:"commission"`
	Currency   string     `json:"currency"`
	PaidAt     *time.Time `json:"paid_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
