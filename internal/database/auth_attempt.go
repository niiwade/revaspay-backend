package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuthAttempt tracks authentication attempts for security purposes
type AuthAttempt struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID      *uuid.UUID `gorm:"type:uuid" json:"user_id"` // Can be null for attempts with non-existent users
	Email       string    `json:"email"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	Success     bool      `json:"success"`
	AttemptType string    `json:"attempt_type"` // login, password_reset, etc.
	CreatedAt   time.Time `json:"created_at"`
}

// AccountLockout tracks account lockouts due to failed authentication attempts
type AccountLockout struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID      uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Reason      string    `json:"reason"`
	LockedUntil time.Time `json:"locked_until"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	UnlockedAt  *time.Time `json:"unlocked_at"`
	UnlockedBy  *uuid.UUID `gorm:"type:uuid" json:"unlocked_by"`
}

// CreateAuthAttempt records an authentication attempt
func CreateAuthAttempt(db *gorm.DB, userID *uuid.UUID, email, ipAddress, userAgent, attemptType string, success bool) (*AuthAttempt, error) {
	attempt := AuthAttempt{
		UserID:      userID,
		Email:       email,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		Success:     success,
		AttemptType: attemptType,
		CreatedAt:   time.Now(),
	}

	if err := db.Create(&attempt).Error; err != nil {
		return nil, err
	}

	return &attempt, nil
}

// GetRecentAuthAttempts gets recent authentication attempts for a user or email
func GetRecentAuthAttempts(db *gorm.DB, userID *uuid.UUID, email string, minutes int) ([]AuthAttempt, error) {
	var attempts []AuthAttempt
	query := db.Where("created_at > ?", time.Now().Add(-time.Duration(minutes)*time.Minute))

	if userID != nil {
		query = query.Where("user_id = ?", userID)
	} else if email != "" {
		query = query.Where("email = ?", email)
	}

	if err := query.Order("created_at DESC").Find(&attempts).Error; err != nil {
		return nil, err
	}

	return attempts, nil
}

// CountFailedAttempts counts failed authentication attempts within a time window
func CountFailedAttempts(db *gorm.DB, userID *uuid.UUID, email, ipAddress string, minutes int) (int64, error) {
	var count int64
	query := db.Model(&AuthAttempt{}).
		Where("success = ? AND created_at > ?", false, time.Now().Add(-time.Duration(minutes)*time.Minute))

	if userID != nil {
		query = query.Where("user_id = ?", userID)
	} else if email != "" {
		query = query.Where("email = ?", email)
	}

	if ipAddress != "" {
		query = query.Where("ip_address = ?", ipAddress)
	}

	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

// LockAccount locks a user account for a specified duration
func LockAccount(db *gorm.DB, userID uuid.UUID, reason string, durationMinutes int) (*AccountLockout, error) {
	// Check if account is already locked
	var existingLockout AccountLockout
	result := db.Where("user_id = ? AND is_active = ?", userID, true).First(&existingLockout)
	
	if result.RowsAffected > 0 {
		// Account already locked, extend the lockout period
		existingLockout.LockedUntil = time.Now().Add(time.Duration(durationMinutes) * time.Minute)
		existingLockout.UpdatedAt = time.Now()
		
		if err := db.Save(&existingLockout).Error; err != nil {
			return nil, err
		}
		
		return &existingLockout, nil
	}
	
	// Create new lockout
	lockout := AccountLockout{
		UserID:      userID,
		Reason:      reason,
		LockedUntil: time.Now().Add(time.Duration(durationMinutes) * time.Minute),
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	
	if err := db.Create(&lockout).Error; err != nil {
		return nil, err
	}
	
	// Update user's locked status
	if err := db.Model(&User{}).Where("id = ?", userID).Update("is_locked", true).Error; err != nil {
		return nil, err
	}
	
	return &lockout, nil
}

// UnlockAccount unlocks a user account
func UnlockAccount(db *gorm.DB, userID uuid.UUID, unlockedBy *uuid.UUID) error {
	now := time.Now()
	
	// Update all active lockouts for this user
	if err := db.Model(&AccountLockout{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Updates(map[string]interface{}{
			"is_active":   false,
			"unlocked_at": now,
			"unlocked_by": unlockedBy,
			"updated_at":  now,
		}).Error; err != nil {
		return err
	}
	
	// Update user's locked status
	if err := db.Model(&User{}).Where("id = ?", userID).Update("is_locked", false).Error; err != nil {
		return err
	}
	
	return nil
}

// IsAccountLocked checks if a user account is currently locked
func IsAccountLocked(db *gorm.DB, userID uuid.UUID) (bool, *AccountLockout, error) {
	var lockout AccountLockout
	result := db.Where("user_id = ? AND is_active = ? AND locked_until > ?", userID, true, time.Now()).First(&lockout)
	
	if result.Error == gorm.ErrRecordNotFound {
		return false, nil, nil
	}
	
	if result.Error != nil {
		return false, nil, result.Error
	}
	
	return true, &lockout, nil
}

// CleanupExpiredLockouts deactivates lockouts that have expired
func CleanupExpiredLockouts(db *gorm.DB) error {
	now := time.Now()
	
	// Get list of user IDs with expired lockouts
	var userIDs []uuid.UUID
	if err := db.Model(&AccountLockout{}).
		Where("is_active = ? AND locked_until < ?", true, now).
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	
	// Update lockouts
	if err := db.Model(&AccountLockout{}).
		Where("is_active = ? AND locked_until < ?", true, now).
		Updates(map[string]interface{}{
			"is_active":   false,
			"unlocked_at": now,
			"updated_at":  now,
		}).Error; err != nil {
		return err
	}
	
	// Update users
	if len(userIDs) > 0 {
		if err := db.Model(&User{}).Where("id IN ?", userIDs).Update("is_locked", false).Error; err != nil {
			return err
		}
	}
	
	return nil
}
