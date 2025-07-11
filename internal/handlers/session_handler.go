package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security/audit"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// SessionHandler handles session management
type SessionHandler struct {
	db         *gorm.DB
	auditLogger *audit.Logger
}

// NewSessionHandler creates a new session handler
func NewSessionHandler(db *gorm.DB) *SessionHandler {
	return &SessionHandler{
		db: db,
		auditLogger: audit.NewLogger(db),
	}
}

// CreateSession creates a new session for a user
func (h *SessionHandler) CreateSession(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get user details to generate token
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Generate tokens using JWT utility
	tokens, err := utils.GenerateTokenPair(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Set expiry time for session
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Get user agent and IP address
	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	// Create device info
	deviceInfo := &database.SessionDevice{
		DeviceID:   uuid.New().String(),
		Browser:    extractBrowserInfo(userAgent),
		OS:         extractOSInfo(userAgent),
		DeviceType: extractDeviceType(userAgent),
	}

	// Create session metadata
	metadata := &database.SessionMetadata{
		LastActiveAt:  time.Now(),
		LastActiveIP:  ipAddress,
		ActivityCount: 1,
		AuthMethod:    "password", // or other auth method if applicable
	}

	// Create enhanced session
	session, err := database.CreateEnhancedSession(h.db, userID.(uuid.UUID), tokens.RefreshToken, userAgent, ipAddress, expiresAt, deviceInfo, metadata)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	// Log session creation
	auditMetadata := map[string]interface{}{
		"session_id":   session.ID.String(),
		"ip_address":   ipAddress,
		"user_agent":   userAgent,
		"expires_at":   expiresAt,
	}

	err = h.auditLogger.LogWithContext(
		c,
		audit.EventTypeSession,
		audit.SeverityInfo,
		"Session created",
		&user.ID,
		&session.ID,
		ipAddress,
		userAgent,
		true,
		auditMetadata,
	)

	if err != nil {
		log.Printf("Failed to log session creation: %v", err)
	}

	// Get session metadata for response
	sessionMetadata, _ := session.GetMetadata()
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Session created successfully",
		"session": gin.H{
			"id":          session.ID,
			"expires_at":  session.ExpiresAt,
			"device_info": gin.H{
				"device_type": sessionMetadata.DeviceType,
				"device_os":   sessionMetadata.DeviceOS,
			},
		},
		"tokens": tokens,
	})
}

// GetActiveSessions gets all active sessions for a user
func (h *SessionHandler) GetActiveSessions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get enhanced sessions
	var sessions []database.EnhancedSession
	if err := h.db.Where("user_id = ? AND expires_at > ? AND status = ?", 
		userID, time.Now(), database.SessionStatusActive).Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get sessions"})
		return
	}

	// Prepare response
	var sessionResponses []gin.H
	for _, session := range sessions {
		// Get session metadata
		metadata, err := session.GetMetadata()
		if err != nil {
			log.Printf("Failed to get session metadata for session %s: %v", session.ID, err)
			continue
		}

		// Build response with enhanced information
		sessionResponse := gin.H{
			"id":             session.ID,
			"user_agent":     session.UserAgent,
			"ip_address":     session.IPAddress,
			"created_at":     session.CreatedAt,
			"expires_at":     session.ExpiresAt,
			"last_active_at": session.LastActiveAt,
			"status":         string(session.Status),
			"rotation_count": session.RotationCount,
		}

		// Add device information if available
		if metadata.DeviceType != "" || metadata.DeviceOS != "" {
			sessionResponse["device"] = gin.H{
				"type":    metadata.DeviceType,
				"os":      metadata.DeviceOS,
				"name":    metadata.DeviceName,
				"version": metadata.DeviceVersion,
			}
		}

		// Add location information if available
		if metadata.Country != "" || metadata.City != "" {
			sessionResponse["location"] = gin.H{
				"country": metadata.Country,
				"city":    metadata.City,
				"region":  metadata.Region,
			}
		}

		// Add activity information
		sessionResponse["activity"] = gin.H{
			"count":        metadata.ActivityCount,
			"last_active":  metadata.LastActiveAt,
			"last_ip":      metadata.LastActiveIP,
		}

		// Add security information
		sessionResponse["security"] = gin.H{
			"risk_score":         metadata.RiskScore,
			"mfa_verified":       metadata.MFAVerified,
			"force_password_reset": metadata.ForcePasswordReset,
		}

		sessionResponses = append(sessionResponses, sessionResponse)
	}

	// Log the request
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeSession,
		audit.SeverityInfo,
		"Active sessions retrieved",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"session_count": len(sessions),
		},
	)

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessionResponses,
	})
}

// RevokeSession revokes a specific session
func (h *SessionHandler) RevokeSession(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get session ID from path
	sessionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	// Check if session belongs to user
	var session database.EnhancedSession
	if err := h.db.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	// Get current session ID (the session being used for this request)
	currentSessionID, currentSessionExists := c.Get("session_id")
	
	// Check if trying to revoke current session
	isSelfRevoke := currentSessionExists && currentSessionID.(uuid.UUID) == sessionID

	// Update session status to revoked
	session.Status = database.SessionStatusRevoked
	if err := h.db.Save(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke session"})
		return
	}

	// Log the session revocation
	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	metadata := map[string]interface{}{
		"session_id":     session.ID.String(),
		"is_self_revoke": isSelfRevoke,
		"revoked_at":     time.Now(),
	}

	err = h.auditLogger.LogWithContext(
		c,
		audit.EventTypeSession,
		audit.SeverityInfo,
		"Session revoked",
		userID.(*uuid.UUID),
		&session.ID,
		ipAddress,
		userAgent,
		true,
		metadata,
	)

	if err != nil {
		log.Printf("Failed to log session revocation: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Session revoked successfully",
		"revoked_at": time.Now(),
		"is_current_session": isSelfRevoke,
	})
}

// RevokeAllSessions revokes all sessions for a user
func (h *SessionHandler) RevokeAllSessions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get current session ID (the session being used for this request)
	currentSessionID, currentSessionExists := c.Get("session_id")

	// Find all active sessions for the user
	var sessions []database.EnhancedSession
	if err := h.db.Where("user_id = ? AND status = ?", userID, database.SessionStatusActive).Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find sessions"})
		return
	}

	// Count of sessions revoked
	revokedCount := 0

	// Optionally exclude current session
	excludeCurrentSession := c.Query("exclude_current") == "true"

	// Revoke all sessions
	for _, session := range sessions {
		// Skip current session if requested
		if excludeCurrentSession && currentSessionExists && currentSessionID.(uuid.UUID) == session.ID {
			continue
		}

		// Update session status to revoked
		session.Status = database.SessionStatusRevoked
		if err := h.db.Save(&session).Error; err != nil {
			log.Printf("Failed to revoke session %s: %v", session.ID, err)
			continue
		}

		revokedCount++
	}

	// Log the session revocation
	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	metadata := map[string]interface{}{
		"revoked_count":      revokedCount,
		"total_sessions":     len(sessions),
		"exclude_current":    excludeCurrentSession,
		"revoked_at":         time.Now(),
	}

	err := h.auditLogger.LogWithContext(
		c,
		audit.EventTypeSession,
		audit.SeverityWarning, // Higher severity since this is a mass revocation
		"All sessions revoked",
		userID.(*uuid.UUID),
		nil,
		ipAddress,
		userAgent,
		true,
		metadata,
	)

	if err != nil {
		log.Printf("Failed to log session revocation: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "All sessions revoked successfully",
		"revoked_count": revokedCount,
		"revoked_at":    time.Now(),
	})
}
