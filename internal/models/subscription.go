package models

import (
	"time"

	"github.com/google/uuid"
)

// BillingInterval represents the billing interval for a subscription
type BillingInterval string

const (
	BillingIntervalMonthly   BillingInterval = "monthly"
	BillingIntervalQuarterly BillingInterval = "quarterly"
	BillingIntervalYearly    BillingInterval = "yearly"
)

// SubscriptionStatus represents the status of a subscription
type SubscriptionStatus string

const (
	SubscriptionStatusActive     SubscriptionStatus = "active"
	SubscriptionStatusInactive   SubscriptionStatus = "inactive"
	SubscriptionStatusCancelled  SubscriptionStatus = "cancelled"
	SubscriptionStatusExpired    SubscriptionStatus = "expired"
	SubscriptionStatusPastDue    SubscriptionStatus = "past_due"
	SubscriptionStatusIncomplete SubscriptionStatus = "incomplete"
)

// SubscriptionPlan represents a subscription plan
type SubscriptionPlan struct {
	Base
	UserID          uuid.UUID      `gorm:"type:uuid;index" json:"user_id"`
	User            User           `gorm:"foreignKey:UserID" json:"-"`
	Name            string         `gorm:"type:varchar(255);not null" json:"name"`
	Description     string         `gorm:"type:text" json:"description"`
	Amount          float64        `gorm:"type:decimal(20,8);not null" json:"amount"`
	Currency        Currency       `gorm:"type:varchar(3);not null" json:"currency"`
	Interval        BillingInterval `gorm:"type:varchar(20);not null" json:"interval"`
	Active          bool           `gorm:"default:true" json:"active"`
	Features        JSON           `gorm:"type:jsonb" json:"features"`
	TrialDays       int            `gorm:"default:0" json:"trial_days"`
	Metadata        JSON           `gorm:"type:jsonb" json:"metadata"`
	SubscriberCount int            `gorm:"-" json:"subscriber_count,omitempty"`
}

// Subscription represents a user subscription to a plan
type Subscription struct {
	Base
	UserID            uuid.UUID         `gorm:"type:uuid;index" json:"user_id"`
	User              User              `gorm:"foreignKey:UserID" json:"-"`
	PlanID            uuid.UUID         `gorm:"type:uuid;index" json:"plan_id"`
	Plan              SubscriptionPlan  `gorm:"foreignKey:PlanID" json:"-"`
	SubscriberID      uuid.UUID         `gorm:"type:uuid;index" json:"subscriber_id"`
	Subscriber        User              `gorm:"foreignKey:SubscriberID" json:"-"`
	Status            SubscriptionStatus `gorm:"type:varchar(20);not null" json:"status"`
	CurrentPeriodStart time.Time        `json:"current_period_start"`
	CurrentPeriodEnd   time.Time        `json:"current_period_end"`
	CancelAtPeriodEnd  bool             `gorm:"default:false" json:"cancel_at_period_end"`
	CancelledAt        *time.Time       `json:"cancelled_at"`
	TrialStart         *time.Time       `json:"trial_start"`
	TrialEnd           *time.Time       `json:"trial_end"`
	PaymentMethod      string           `gorm:"type:varchar(50)" json:"payment_method"`
	PaymentMethodDetails JSON            `gorm:"type:jsonb" json:"payment_method_details"`
	LastPaymentDate     *time.Time      `json:"last_payment_date"`
	NextPaymentDate     *time.Time      `json:"next_payment_date"`
	FailedPaymentCount  int             `gorm:"default:0" json:"failed_payment_count"`
	Metadata            JSON            `gorm:"type:jsonb" json:"metadata"`
}

// SubscriptionPayment represents a payment for a subscription
type SubscriptionPayment struct {
	Base
	SubscriptionID uuid.UUID      `gorm:"type:uuid;index" json:"subscription_id"`
	Subscription   Subscription   `gorm:"foreignKey:SubscriptionID" json:"-"`
	PaymentID      uuid.UUID      `gorm:"type:uuid;index" json:"payment_id"`
	Payment        Payment        `gorm:"foreignKey:PaymentID" json:"-"`
	PeriodStart    time.Time      `json:"period_start"`
	PeriodEnd      time.Time      `json:"period_end"`
	Status         PaymentStatus  `gorm:"type:varchar(20);not null" json:"status"`
	RetryCount     int            `gorm:"default:0" json:"retry_count"`
	NextRetryDate  *time.Time     `json:"next_retry_date"`
}
