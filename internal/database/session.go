package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Session represents a user session with refresh token
type Session struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID       uuid.UUID `gorm:"type:uuid" json:"user_id"`
	RefreshToken string    `json:"refresh_token"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	User User `json:"-"`
}

// CreateSession creates a new session for a user
func CreateSession(db *gorm.DB, userID uuid.UUID, refreshToken, userAgent, ipAddress string, expiresAt time.Time) (*Session, error) {
	session := Session{
		UserID:       userID,
		RefreshToken: refreshToken,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		ExpiresAt:    expiresAt,
	}

	if err := db.Create(&session).Error; err != nil {
		return nil, err
	}

	return &session, nil
}

// FindSessionByRefreshToken finds a session by refresh token
func FindSessionByRefreshToken(db *gorm.DB, refreshToken string) (*Session, error) {
	var session Session
	if err := db.Where("refresh_token = ? AND expires_at > ?", refreshToken, time.Now()).First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// InvalidateSession invalidates a session by deleting it
func InvalidateSession(db *gorm.DB, sessionID uuid.UUID) error {
	return db.Delete(&Session{}, "id = ?", sessionID).Error
}

// InvalidateAllUserSessions invalidates all sessions for a user
func InvalidateAllUserSessions(db *gorm.DB, userID uuid.UUID) error {
	return db.Delete(&Session{}, "user_id = ?", userID).Error
}

// UpdateSession updates a session with a new refresh token and expiry
func UpdateSession(db *gorm.DB, sessionID uuid.UUID, refreshToken string, expiresAt time.Time) error {
	return db.Model(&Session{}).Where("id = ?", sessionID).Updates(map[string]interface{}{
		"refresh_token": refreshToken,
		"expires_at":    expiresAt,
		"updated_at":    time.Now(),
	}).Error
}

// CleanupExpiredSessions removes all expired sessions
func CleanupExpiredSessions(db *gorm.DB) error {
	return db.Delete(&Session{}, "expires_at < ?", time.Now()).Error
}
