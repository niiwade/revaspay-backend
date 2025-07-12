package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Referral represents a referral record
type Referral struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	ReferrerID     uuid.UUID      `gorm:"type:uuid;not null" json:"referrer_id"`
	Referrer       User           `gorm:"foreignKey:ReferrerID" json:"-"`
	ReferredUserID uuid.UUID      `gorm:"type:uuid;not null" json:"referred_user_id"`
	ReferredUser   User           `gorm:"foreignKey:ReferredUserID" json:"-"`
	ReferralCode   string         `gorm:"type:varchar(50);uniqueIndex;not null" json:"referral_code"`
	Status         string         `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	RewardsEarned  float64        `gorm:"type:decimal(20,2);default:0" json:"rewards_earned"`
	LastRewardAt   time.Time      `json:"last_reward_at"`
	CreatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// ReferralReward represents a reward earned from a referral
type ReferralReward struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	ReferralID     uuid.UUID      `gorm:"type:uuid;not null" json:"referral_id"`
	Referral       Referral       `gorm:"foreignKey:ReferralID" json:"-"`
	ReferrerID     uuid.UUID      `gorm:"type:uuid;not null" json:"referrer_id"`
	Referrer       User           `gorm:"foreignKey:ReferrerID" json:"-"`
	ReferredUserID uuid.UUID      `gorm:"type:uuid;not null" json:"referred_user_id"`
	ReferredUser   User           `gorm:"foreignKey:ReferredUserID" json:"-"`
	EventType      string         `gorm:"type:varchar(50);not null" json:"event_type"` // signup, first_payment, kyc_verified, etc.
	Amount         float64        `gorm:"type:decimal(20,2);not null" json:"amount"`
	Currency       string         `gorm:"type:varchar(3);not null" json:"currency"`
	Status         string         `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	CompletedAt    time.Time      `json:"completed_at"`
	CreatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
