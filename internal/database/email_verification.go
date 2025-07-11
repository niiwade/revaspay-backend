package database

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailVerificationStatus represents the status of an email verification attempt
type EmailVerificationStatus string

const (
	VerificationStatusPending  EmailVerificationStatus = "pending"
	VerificationStatusComplete EmailVerificationStatus = "complete"
	VerificationStatusExpired  EmailVerificationStatus = "expired"
)

// EmailVerificationToken represents an email verification token in the database
type EmailVerificationToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID    uuid.UUID `gorm:"type:uuid"`
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
	Status    EmailVerificationStatus `gorm:"default:'pending'"`
	AttemptCount int `gorm:"default:0"`
	LastAttemptAt *time.Time
	
	// Relationships
	User User `json:"-"`
}

// CreateEmailVerificationToken creates a new email verification token
func CreateEmailVerificationToken(db *gorm.DB, userID uuid.UUID, token string, expiresAt time.Time) (*EmailVerificationToken, error) {
	// Invalidate any existing tokens for this user
	if err := db.Model(&EmailVerificationToken{}).
		Where("user_id = ? AND status = ?", userID, VerificationStatusPending).
		Updates(map[string]interface{}{
			"status": VerificationStatusExpired,
		}).Error; err != nil {
		return nil, err
	}

	// Create new token
	verificationToken := EmailVerificationToken{
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt,
		Status:    VerificationStatusPending,
	}

	if err := db.Create(&verificationToken).Error; err != nil {
		return nil, err
	}

	return &verificationToken, nil
}

// GetEmailVerificationToken retrieves an email verification token
func GetEmailVerificationToken(db *gorm.DB, token string) (*EmailVerificationToken, error) {
	var verificationToken EmailVerificationToken
	if err := db.Where("token = ? AND status = ?", token, VerificationStatusPending).First(&verificationToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid or expired verification token")
		}
		return nil, err
	}

	// Check if token is expired
	if time.Now().After(verificationToken.ExpiresAt) {
		// Mark as expired
		db.Model(&verificationToken).Updates(map[string]interface{}{
			"status": VerificationStatusExpired,
		})
		return nil, errors.New("verification token has expired")
	}

	return &verificationToken, nil
}

// GetPendingVerificationTokensByUser retrieves all pending verification tokens for a user
func GetPendingVerificationTokensByUser(db *gorm.DB, userID uuid.UUID) ([]EmailVerificationToken, error) {
	var tokens []EmailVerificationToken
	if err := db.Where("user_id = ? AND status = ?", userID, VerificationStatusPending).Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// IncrementVerificationAttempt increments the attempt count for a verification token
func IncrementVerificationAttempt(db *gorm.DB, tokenID uuid.UUID) error {
	now := time.Now()
	return db.Model(&EmailVerificationToken{}).
		Where("id = ?", tokenID).
		Updates(map[string]interface{}{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_attempt_at": now,
		}).Error
}

// CompleteVerification marks a verification token as complete
func CompleteVerification(db *gorm.DB, tokenID uuid.UUID) error {
	return db.Model(&EmailVerificationToken{}).
		Where("id = ?", tokenID).
		Updates(map[string]interface{}{
			"status": VerificationStatusComplete,
		}).Error
}

// CheckVerificationRateLimit checks if a user has exceeded verification attempts
func CheckVerificationRateLimit(db *gorm.DB, userID uuid.UUID) (bool, error) {
	var count int64
	
	// Count attempts in the last hour
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	
	if err := db.Model(&EmailVerificationToken{}).
		Where("user_id = ? AND last_attempt_at > ?", userID, oneHourAgo).
		Count(&count).Error; err != nil {
		return false, err
	}
	
	// Limit to 5 attempts per hour
	return count >= 5, nil
}
