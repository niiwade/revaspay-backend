package database

import (
	"fmt"
	"time"

	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB initializes the database connection with configuration
func InitDB(dbConfig config.DatabaseConfig) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	db, err := gorm.Open(postgres.Open(dbConfig.URL), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(dbConfig.MaxIdle)
	sqlDB.SetMaxOpenConns(dbConfig.MaxConns)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Run migrations
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	// Auto migrate all models
	return db.AutoMigrate(
		// User and authentication
		&models.User{},
		&models.Session{},
		&models.PasswordResetToken{},
		&models.EmailVerificationToken{},
		&models.TwoFactorAuth{},
		&models.LoginAttempt{},

		// KYC verification
		&models.KYCVerification{},
		&models.KYCVerificationHistory{},
		&models.KYCDocument{},

		// Financial
		&models.Wallet{},
		&models.Transaction{},
		&models.Payment{},
		&models.PaymentLink{},
		&models.PaymentWebhook{},
		&models.Withdrawal{},
		&models.VirtualAccount{},
		&models.MoMoTransaction{},

		// Subscriptions
		&models.Subscription{},
		&models.SubscriptionPlan{},
		&models.SubscriptionPayment{},

		// Referrals
		&models.Referral{},
		&models.ReferralReward{},
	)
}
