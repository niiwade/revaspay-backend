package database

import (
	"log"

	"github.com/revaspay/backend/internal/security/audit"
	"gorm.io/gorm"
)

// RunMigrations runs all database migrations
func RunMigrations(db *gorm.DB) {
	log.Println("Running database migrations...")

	// Auto migrate all models
	err := db.AutoMigrate(
		&User{},
		&Wallet{},
		&Transaction{},
		&KYC{},
		&SubscriptionPlan{},
		&Subscription{},
		&VirtualAccount{},
		&Referral{},
		&PasswordResetToken{},
		&EmailVerificationToken{},
		&MoMoTransaction{},
		&MoMoDisbursement{},
		&Session{},
		&EnhancedSession{},
		&FailedLoginAttempt{},
		&SecurityQuestion{},
		&UserSecurityQuestion{},
		&RecoveryToken{},
		&audit.AuditLog{},
	)

	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	log.Println("Migrations completed successfully")
}

// PasswordResetToken represents a password reset token
type PasswordResetToken struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"index"`
	Token     string `gorm:"uniqueIndex"`
	ExpiresAt int64  `gorm:"index"`
	CreatedAt int64
}

// EmailVerificationToken represents an email verification token
