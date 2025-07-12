package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Session represents a user session
type Session struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID       uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	User         User           `gorm:"foreignKey:UserID" json:"-"`
	Token        string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"token"`
	RefreshToken string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"refresh_token"`
	UserAgent    string         `gorm:"type:text" json:"user_agent"`
	IPAddress    string         `gorm:"type:varchar(45)" json:"ip_address"`
	ExpiresAt    time.Time      `json:"expires_at"`
	LastUsedAt   time.Time      `json:"last_used_at"`
	CreatedAt    time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"-"`
	Token     string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	UsedAt    *time.Time     `json:"used_at"`
	CreatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// EmailVerificationToken represents an email verification token
type EmailVerificationToken struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	User      User           `gorm:"foreignKey:UserID" json:"-"`
	Email     string         `gorm:"type:varchar(255);not null" json:"email"`
	Token     string         `gorm:"type:varchar(255);uniqueIndex;not null" json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	VerifiedAt *time.Time    `json:"verified_at"`
	CreatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TwoFactorAuth represents a two-factor authentication configuration
type TwoFactorAuth struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex" json:"user_id"`
	User          User           `gorm:"foreignKey:UserID" json:"-"`
	Secret        string         `gorm:"type:varchar(255);not null" json:"secret"`
	BackupCodes   string         `gorm:"type:jsonb" json:"backup_codes"`
	Enabled       bool           `gorm:"default:false" json:"enabled"`
	VerifiedAt    *time.Time     `json:"verified_at"`
	LastUsedAt    *time.Time     `json:"last_used_at"`
	CreatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// LoginAttempt represents a login attempt record for security monitoring
type LoginAttempt struct {
	ID        uuid.UUID      `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	UserID    *uuid.UUID     `gorm:"type:uuid" json:"user_id"`
	User      *User          `gorm:"foreignKey:UserID" json:"-"`
	Email     string         `gorm:"type:varchar(255)" json:"email"`
	IPAddress string         `gorm:"type:varchar(45)" json:"ip_address"`
	UserAgent string         `gorm:"type:text" json:"user_agent"`
	Success   bool           `gorm:"default:false" json:"success"`
	Reason    string         `gorm:"type:varchar(255)" json:"reason"`
	CreatedAt time.Time      `gorm:"default:CURRENT_TIMESTAMP" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
