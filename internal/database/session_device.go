package database

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionDevice contains information about the device used for a session
type SessionDevice struct {
	DeviceType    string `json:"device_type,omitempty"`
	DeviceID      string `json:"device_id,omitempty"`
	Browser       string `json:"browser,omitempty"`
	OS            string `json:"os,omitempty"`
	TrustedDevice bool   `json:"trusted_device"`
	LastVerifiedAt string `json:"last_verified_at,omitempty"`
}

// GetDeviceInfo returns the device information from the session
func (s *EnhancedSession) GetDeviceInfo() (*SessionDevice, error) {
	if s.DeviceFingerprint == "" {
		return &SessionDevice{}, nil
	}
	
	var deviceInfo SessionDevice
	err := json.Unmarshal([]byte(s.DeviceFingerprint), &deviceInfo)
	if err != nil {
		return nil, err
	}
	
	return &deviceInfo, nil
}

// SetDeviceInfo sets the device information for the session
func (s *EnhancedSession) SetDeviceInfo(deviceInfo *SessionDevice) error {
	deviceInfoBytes, err := json.Marshal(deviceInfo)
	if err != nil {
		return err
	}
	
	s.DeviceFingerprint = string(deviceInfoBytes)
	return nil
}

// GetActiveSessions returns all active sessions for a user
func GetActiveSessions(db *gorm.DB, userID uuid.UUID) ([]EnhancedSession, error) {
	var sessions []EnhancedSession
	err := db.Where("user_id = ? AND status = ?", userID, SessionStatusActive).Find(&sessions).Error
	return sessions, err
}

// RevokeSession revokes a specific session
func RevokeSession(db *gorm.DB, sessionID uuid.UUID, reason string) error {
	return db.Model(&EnhancedSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"status": SessionStatusRevoked,
		}).Error
}

// RevokeAllUserSessionsExcept revokes all sessions for a user except the specified one
func RevokeAllUserSessionsExcept(db *gorm.DB, userID uuid.UUID, exceptSessionID uuid.UUID) error {
	return db.Model(&EnhancedSession{}).
		Where("user_id = ? AND id != ? AND status = ?", userID, exceptSessionID, SessionStatusActive).
		Updates(map[string]interface{}{
			"status": SessionStatusRevoked,
		}).Error
}

// ForceMFAVerification forces MFA verification for all user sessions
func ForceMFAVerification(db *gorm.DB, userID uuid.UUID) error {
	// Get all active sessions
	var sessions []EnhancedSession
	if err := db.Where("user_id = ? AND status = ?", userID, SessionStatusActive).Find(&sessions).Error; err != nil {
		return err
	}

	// Update each session's metadata to require MFA
	for _, session := range sessions {
		metadata, err := session.GetMetadata()
		if err != nil {
			continue
		}

		metadata.MFAVerified = false
		if err := session.SetMetadata(metadata); err != nil {
			continue
		}

		db.Save(&session)
	}

	return nil
}

// ForcePasswordReset forces password reset for all user sessions
func ForcePasswordReset(db *gorm.DB, userID uuid.UUID) error {
	// Get all active sessions
	var sessions []EnhancedSession
	if err := db.Where("user_id = ? AND status = ?", userID, SessionStatusActive).Find(&sessions).Error; err != nil {
		return err
	}

	// Update each session's metadata to require password reset
	for _, session := range sessions {
		metadata, err := session.GetMetadata()
		if err != nil {
			continue
		}

		metadata.ForcePasswordReset = true
		if err := session.SetMetadata(metadata); err != nil {
			continue
		}

		db.Save(&session)
	}

	return nil
}

// SuspendSuspiciousSessions suspends sessions that are deemed suspicious
func SuspendSuspiciousSessions(db *gorm.DB, userID uuid.UUID, adminID *uuid.UUID, reason string) error {
	return db.Model(&EnhancedSession{}).
		Where("user_id = ? AND status = ? AND risk_level IN ?", userID, SessionStatusActive, []string{"high", "critical"}).
		Updates(map[string]interface{}{
			"status": SessionStatusSuspicious,
		}).Error
}
