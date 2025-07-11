package database

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect establishes a connection to the database
func Connect(databaseURL string) (*gorm.DB, error) {
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	db, err := gorm.Open(postgres.Open(databaseURL), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	// Auto migrate all models
	return db.AutoMigrate(
		&User{},
		&KYC{},
		&Wallet{},
		&Transaction{},
		&PaymentLink{},
		&Withdrawal{},
		&Subscription{},
		&SubscriptionPlan{},
		&VirtualAccount{},
		&Referral{},
		&PasswordResetToken{},
		&EmailVerificationToken{},
		&Session{},
	)
}
