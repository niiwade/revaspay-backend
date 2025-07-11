package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FailedLoginAttempt represents a failed login attempt
type FailedLoginAttempt struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID    *uuid.UUID `gorm:"type:uuid;index"`
	IPAddress string    `gorm:"index"`
	UserAgent string
	Email     string    `gorm:"index"`
	Reason    string
	CreatedAt time.Time `gorm:"index"`
}

// RecordFailedLoginAttempt records a failed login attempt
func RecordFailedLoginAttempt(db *gorm.DB, userID *uuid.UUID, email, ipAddress, userAgent, reason string) error {
	attempt := FailedLoginAttempt{
		UserID:    userID,
		Email:     email,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Reason:    reason,
		CreatedAt: time.Now(),
	}
	
	return db.Create(&attempt).Error
}

// ClearFailedLoginAttempts clears all failed login attempts for a user
func ClearFailedLoginAttempts(db *gorm.DB, userID uuid.UUID) error {
	return db.Where("user_id = ?", userID).Delete(&FailedLoginAttempt{}).Error
}

// GetFailedLoginAttempts gets all failed login attempts for a user or IP address within a time period
func GetFailedLoginAttempts(db *gorm.DB, userID *uuid.UUID, ipAddress string, since time.Time) ([]FailedLoginAttempt, error) {
	var attempts []FailedLoginAttempt
	query := db.Where("created_at > ?", since)
	
	if userID != nil {
		query = query.Where("user_id = ?", userID)
	}
	
	if ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}
	
	err := query.Order("created_at DESC").Find(&attempts).Error
	return attempts, err
}
