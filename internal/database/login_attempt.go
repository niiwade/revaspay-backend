package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LoginAttempt represents a login attempt record
type LoginAttempt struct {
	ID        uint       `gorm:"primaryKey"`
	UserID    uuid.UUID  `gorm:"type:uuid;index"`
	IPAddress string     `gorm:"index"`
	UserAgent string
	Success   bool       `gorm:"index"`
	SessionID *uuid.UUID `gorm:"type:uuid;index"`
	CreatedAt time.Time  `gorm:"index"`
	DeletedAt gorm.DeletedAt
}

// CreateLoginAttempt creates a new login attempt record
func CreateLoginAttempt(db *gorm.DB, userID uuid.UUID, ipAddress, userAgent string, success bool, sessionID *uuid.UUID) error {
	attempt := LoginAttempt{
		UserID:    userID,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Success:   success,
		SessionID: sessionID,
		CreatedAt: time.Now(),
	}

	return db.Create(&attempt).Error
}

// GetRecentFailedAttempts gets the number of recent failed login attempts for a user
func GetRecentFailedAttempts(db *gorm.DB, userID uuid.UUID, duration time.Duration) (int64, error) {
	var count int64
	err := db.Model(&LoginAttempt{}).
		Where("user_id = ? AND success = ? AND created_at > ?", userID, false, time.Now().Add(-duration)).
		Count(&count).Error

	return count, err
}

// GetRecentFailedAttemptsFromIP gets the number of recent failed login attempts from an IP address
func GetRecentFailedAttemptsFromIP(db *gorm.DB, ipAddress string, duration time.Duration) (int64, error) {
	var count int64
	err := db.Model(&LoginAttempt{}).
		Where("ip_address = ? AND success = ? AND created_at > ?", ipAddress, false, time.Now().Add(-duration)).
		Count(&count).Error

	return count, err
}

// UpdateSessionActivityWithAction updates the last activity time for a session with action and resource tracking
func UpdateSessionActivityWithAction(db *gorm.DB, sessionID uuid.UUID, ipAddress, action, resource string) error {
	// Get session
	var session EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}

	// Update last active time
	session.LastActiveAt = time.Now()

	// Get metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		return err
	}

	// Update metadata
	metadata.LastActiveAt = time.Now()
	metadata.LastActiveIP = ipAddress
	metadata.ActivityCount++

	// Add to attributes if action and resource provided
	if action != "" && resource != "" {
		if metadata.Attributes == nil {
			metadata.Attributes = make(map[string]interface{})
		}
		
		// Add activity to attributes
		activities, ok := metadata.Attributes["recent_activities"].([]map[string]interface{})
		if !ok {
			activities = make([]map[string]interface{}, 0)
		}
		
		// Add new activity
		newActivity := map[string]interface{}{
			"time":     time.Now().Format(time.RFC3339),
			"action":   action,
			"resource": resource,
			"ip":       ipAddress,
		}
		
		// Keep only last 10 activities
		if len(activities) >= 10 {
			activities = activities[1:]
		}
		activities = append(activities, newActivity)
		
		metadata.Attributes["recent_activities"] = activities
	}

	// Save metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}

	// Save session
	return db.Save(&session).Error
}
