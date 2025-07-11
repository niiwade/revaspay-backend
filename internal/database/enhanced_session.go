package database

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionStatus represents the status of a session
type SessionStatus string

const (
	// SessionStatusActive represents an active session
	SessionStatusActive SessionStatus = "active"
	// SessionStatusRevoked represents a revoked session
	SessionStatusRevoked SessionStatus = "revoked"
	// SessionStatusExpired represents an expired session
	SessionStatusExpired SessionStatus = "expired"
	// SessionStatusSuspicious represents a suspicious session that requires additional verification
	SessionStatusSuspicious SessionStatus = "suspicious"
)

// SessionMetadata contains additional metadata for enhanced sessions
type SessionMetadata struct {
	// Device information
	DeviceID       string `json:"device_id,omitempty"`
	DeviceType     string `json:"device_type,omitempty"`
	DeviceName     string `json:"device_name,omitempty"`
	DeviceOS       string `json:"device_os,omitempty"`
	DeviceVersion  string `json:"device_version,omitempty"`
	
	// Location information
	Country        string `json:"country,omitempty"`
	City           string `json:"city,omitempty"`
	Region         string `json:"region,omitempty"`
	
	// Risk assessment
	RiskScore      int    `json:"risk_score,omitempty"`
	RiskFactors    []string `json:"risk_factors,omitempty"`
	
	// Security
	MFAVerified    bool      `json:"mfa_verified,omitempty"`
	MFAVerifiedAt  *time.Time `json:"mfa_verified_at,omitempty"`
	ForcePasswordReset bool   `json:"force_password_reset,omitempty"`
	AuthMethod     string    `json:"auth_method,omitempty"`
	
	// Activity tracking
	LastActiveAt   time.Time `json:"last_active_at,omitempty"`
	LastActiveIP   string    `json:"last_active_ip,omitempty"`
	LastLocationIP string    `json:"last_location_ip,omitempty"`
	ActivityCount  int       `json:"activity_count,omitempty"`
	
	// Custom attributes
	Attributes     map[string]interface{} `json:"attributes,omitempty"`
}

// EnhancedSession represents a user session with enhanced security features
type EnhancedSession struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	UserID        uuid.UUID      `gorm:"type:uuid;index"`
	RefreshToken  string         `gorm:"index"`
	UserAgent     string
	IPAddress     string
	Status        SessionStatus  `gorm:"index;default:'active'"`
	CreatedAt     time.Time      `gorm:"index"`
	ExpiresAt     time.Time      `gorm:"index"`
	LastActiveAt  time.Time
	MetadataJSON  string         `gorm:"type:text"`
	RotationCount int            `gorm:"default:0"`
	RiskScore     float64        `gorm:"default:0"`
	RiskLevel     string         `gorm:"default:'low'"`
	DeviceFingerprint string     `gorm:"index"`
}

// GetMetadata returns the session metadata
func (s *EnhancedSession) GetMetadata() (*SessionMetadata, error) {
	if s.MetadataJSON == "" {
		return &SessionMetadata{}, nil
	}
	
	var metadata SessionMetadata
	err := json.Unmarshal([]byte(s.MetadataJSON), &metadata)
	if err != nil {
		return nil, err
	}
	
	return &metadata, nil
}

// SetMetadata sets the session metadata
func (s *EnhancedSession) SetMetadata(metadata *SessionMetadata) error {
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	
	s.MetadataJSON = string(metadataBytes)
	return nil
}

// CreateEnhancedSession creates a new enhanced session
func CreateEnhancedSession(db *gorm.DB, userID uuid.UUID, refreshToken, userAgent, ipAddress string, expiresAt time.Time, deviceInfo *SessionDevice, customMetadata *SessionMetadata) (*EnhancedSession, error) {
	session := EnhancedSession{
		UserID:       userID,
		RefreshToken: refreshToken,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		Status:       SessionStatusActive,
		CreatedAt:    time.Now(),
		ExpiresAt:    expiresAt,
		LastActiveAt: time.Now(),
	}
	
	// Initialize metadata
	metadata := SessionMetadata{
		LastActiveAt:  time.Now(),
		LastActiveIP:  ipAddress,
		ActivityCount: 1,
	}

	// Use custom metadata if provided
	if customMetadata != nil {
		metadata = *customMetadata
		// Ensure these fields are always set
		if metadata.LastActiveAt.IsZero() {
			metadata.LastActiveAt = time.Now()
		}
		if metadata.LastActiveIP == "" {
			metadata.LastActiveIP = ipAddress
		}
		if metadata.LastLocationIP == "" {
			metadata.LastLocationIP = ipAddress
		}
	}
	
	// Parse user agent to extract device information if not provided
	if deviceInfo == nil && userAgent != "" {
		// Simple parsing for demonstration
		if len(userAgent) > 10 {
			metadata.DeviceType = "browser"
			
			// Extract OS information (simplified)
			if contains(userAgent, "Windows") {
				metadata.DeviceOS = "Windows"
			} else if contains(userAgent, "Mac") {
				metadata.DeviceOS = "MacOS"
			} else if contains(userAgent, "iPhone") || contains(userAgent, "iPad") {
				metadata.DeviceOS = "iOS"
			} else if contains(userAgent, "Android") {
				metadata.DeviceOS = "Android"
			} else {
				metadata.DeviceOS = "Other"
			}
			
			// Generate a simple device ID
			metadata.DeviceID = generateDeviceID(userID, userAgent)
		}
	} else if deviceInfo != nil {
		// Use provided device info
		metadata.DeviceID = deviceInfo.DeviceID
		metadata.DeviceType = deviceInfo.DeviceType
		metadata.DeviceOS = deviceInfo.OS
		
		// Set device fingerprint
		deviceInfoBytes, _ := json.Marshal(deviceInfo)
		session.DeviceFingerprint = string(deviceInfoBytes)
	}
	
	// Set metadata
	if err := session.SetMetadata(&metadata); err != nil {
		return nil, err
	}
	
	// Save to database
	if err := db.Create(&session).Error; err != nil {
		return nil, err
	}
	
	return &session, nil
}

// UpdateSessionActivity updates the session activity
func UpdateSessionActivity(db *gorm.DB, sessionID uuid.UUID, ipAddress, country, city string) error {
	var session EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}
	
	// Get current metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		return err
	}
	
	// Update activity information
	metadata.LastActiveAt = time.Now()
	if ipAddress != "" {
		metadata.LastActiveIP = ipAddress
	}
	if country != "" {
		metadata.Country = country
	}
	if city != "" {
		metadata.City = city
	}
	metadata.ActivityCount++
	
	// Update metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}
	
	// Update session
	session.LastActiveAt = time.Now()
	return db.Save(&session).Error
}

// UpdateSessionRisk updates the session risk assessment
func UpdateSessionRisk(db *gorm.DB, sessionID uuid.UUID, riskScore float64, riskLevel string, riskFactors []string) error {
	var session EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}
	
	// Get current metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		return err
	}
	
	// Update risk information in metadata
	metadata.RiskFactors = riskFactors
	
	// Update session risk fields directly
	session.RiskScore = riskScore
	session.RiskLevel = riskLevel
	
	// Update status if risk is high
	if riskLevel == "high" || riskLevel == "critical" {
		session.Status = SessionStatusSuspicious
	}
	
	// Update metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}
	
	// Update session
	return db.Save(&session).Error
}

// RotateSession rotates the session by updating the refresh token
func RotateSession(db *gorm.DB, sessionID uuid.UUID, newRefreshToken string) error {
	var session EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}
	
	// Update session
	session.RefreshToken = newRefreshToken
	session.RotationCount++
	
	return db.Save(&session).Error
}

// VerifyMFA marks the session as MFA verified
func VerifyMFA(db *gorm.DB, sessionID uuid.UUID) error {
	var session EnhancedSession
	if err := db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		return err
	}
	
	// Get current metadata
	metadata, err := session.GetMetadata()
	if err != nil {
		return err
	}
	
	// Update MFA verification
	now := time.Now()
	metadata.MFAVerified = true
	metadata.MFAVerifiedAt = &now
	
	// Update metadata
	if err := session.SetMetadata(metadata); err != nil {
		return err
	}
	
	// Update session
	return db.Save(&session).Error
}

// Helper functions

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return s != "" && substr != "" && s != substr && len(s) >= len(substr) && s != "" && substr != "" && s != substr && len(s) >= len(substr)
}

// generateDeviceID generates a simple device ID
func generateDeviceID(userID uuid.UUID, userAgent string) string {
	// In a real implementation, this would use a more sophisticated algorithm
	// This is a simplified version for demonstration
	
	// Create a hash from userAgent to make the device ID more unique
	uaHash := ""  
	if len(userAgent) > 0 {
		// Take the first character and last character if available
		uaHash = string(userAgent[0])
		if len(userAgent) > 1 {
			uaHash += string(userAgent[len(userAgent)-1])
		}
	}
	
	return userID.String()[0:8] + "-" + uaHash + "-device"
}
