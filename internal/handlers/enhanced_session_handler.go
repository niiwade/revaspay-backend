package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// EnhancedSessionHandler handles advanced session management with security features
type EnhancedSessionHandler struct {
	db           *gorm.DB
	riskAssessor *security.RiskAssessor
}

// NewEnhancedSessionHandler creates a new enhanced session handler
func NewEnhancedSessionHandler(db *gorm.DB) *EnhancedSessionHandler {
	return &EnhancedSessionHandler{
		db:           db,
		riskAssessor: security.NewRiskAssessor(db),
	}
}

// CreateEnhancedSession creates a new session with security risk assessment
func (h *EnhancedSessionHandler) CreateEnhancedSession(c *gin.Context) {
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

	// Get user agent and IP address
	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	// Perform risk assessment
	assessment, err := h.riskAssessor.AssessLoginRisk(userID.(uuid.UUID), ipAddress, userAgent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assess login risk"})
		return
	}

	// Check if login should be blocked
	if assessment.Action == "block" {
		c.JSON(http.StatusForbidden, gin.H{
			"error":       "Suspicious login blocked",
			"risk_score":  assessment.Score,
			"assessment":  assessment.AssessmentID,
			"require_mfa": true,
		})
		return
	}

	// Check if MFA is required but not enabled for user
	if assessment.RequireMFA && !user.TwoFactorEnabled {
		// For users without 2FA, we'll allow login but recommend enabling it
		c.Set("recommend_2fa", true)
	}

	// Generate tokens using JWT utility
	tokens, err := utils.GenerateTokenPair(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Set expiry time for session
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	// Create device info
	deviceInfo := &database.SessionDevice{
		DeviceType:    h.detectDeviceType(userAgent),
		Browser:       h.detectBrowser(userAgent),
		OS:            h.detectOS(userAgent),
		TrustedDevice: false,
	}

	// Create metadata
	now := time.Now()
	metadata := &database.SessionMetadata{
		LastActiveAt:   now,
		LastLocationIP: ipAddress,
		RiskScore:      int(assessment.Score), // Convert float64 to int
		AuthMethod:     "password", // This should be dynamic based on actual auth method
	}

	// If MFA was used, record it
	if c.GetBool("mfa_verified") {
		metadata.MFAVerifiedAt = &now
	}

	// Create enhanced session
	session, err := database.CreateEnhancedSession(
		h.db, 
		userID.(uuid.UUID), 
		tokens.RefreshToken, 
		userAgent, 
		ipAddress, 
		expiresAt,
		deviceInfo,
		metadata,
	)
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	// Record successful login for risk assessment
	h.riskAssessor.RecordSuccessfulLogin(userID.(uuid.UUID), session.ID, ipAddress, userAgent)

	// Update risk metadata
	h.riskAssessor.UpdateSessionRiskMetadata(session.ID, assessment)

	response := gin.H{
		"message": "Session created successfully",
		"session": gin.H{
			"id":         session.ID,
			"expires_at": session.ExpiresAt,
		},
		"tokens": tokens,
	}

	// Add MFA recommendation if needed
	if c.GetBool("recommend_2fa") {
		response["security_recommendation"] = "We recommend enabling two-factor authentication for additional security"
	}

	// If this is a challenge, indicate additional verification may be needed
	if assessment.Action == "challenge" {
		response["verification_required"] = true
	}

	c.JSON(http.StatusOK, response)
}

// GetEnhancedSessions gets all active sessions with detailed information
func (h *EnhancedSessionHandler) GetEnhancedSessions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get sessions
	sessions, err := database.GetActiveSessions(h.db, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get sessions"})
		return
	}

	// Prepare response
	var sessionResponses []gin.H
	for _, session := range sessions {
		// Get device info
		deviceInfo, _ := session.GetDeviceInfo()
		
		// Get metadata
		metadata, _ := session.GetMetadata()
		
		sessionResponse := gin.H{
			"id":           session.ID,
			"status":       session.Status,
			"user_agent":   session.UserAgent,
			"ip_address":   session.IPAddress,
			"created_at":   session.CreatedAt,
			// EnhancedSession doesn't have UpdatedAt field
			"expires_at":   session.ExpiresAt,
			"device_type":  "unknown",
			"browser":      "unknown",
			"os":           "unknown",
			"last_active":  session.CreatedAt,
			"is_current":   false,
		}
		
		// Add device info if available
		if deviceInfo != nil {
			sessionResponse["device_type"] = deviceInfo.DeviceType
			sessionResponse["browser"] = deviceInfo.Browser
			sessionResponse["os"] = deviceInfo.OS
			sessionResponse["trusted_device"] = deviceInfo.TrustedDevice
		}
		
		// Add metadata if available
		if metadata != nil {
			sessionResponse["last_active"] = metadata.LastActiveAt
			sessionResponse["risk_score"] = metadata.RiskScore
			
			if metadata.City != "" {
				sessionResponse["location"] = metadata.City
				if metadata.Region != "" {
					sessionResponse["location"] = metadata.City + ", " + metadata.Region
				}
			}
		}
		
		// Check if this is the current session
		currentSessionID, exists := c.Get("session_id")
		if exists && currentSessionID.(uuid.UUID) == session.ID {
			sessionResponse["is_current"] = true
		}
		
		sessionResponses = append(sessionResponses, sessionResponse)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessionResponses,
	})
}

// RevokeEnhancedSession revokes a specific session with reason
func (h *EnhancedSessionHandler) RevokeEnhancedSession(c *gin.Context) {
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

	// Get reason from request
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "User initiated logout"
	}

	// Revoke session
	if err := database.RevokeSession(h.db, sessionID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Session revoked successfully",
	})
}

// RevokeAllOtherSessions revokes all sessions except the current one
func (h *EnhancedSessionHandler) RevokeAllOtherSessions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get current session ID
	currentSessionID, exists := c.Get("session_id")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Current session not found"})
		return
	}

	// Get reason from request
	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Reason = "User initiated logout from all other devices"
	}

	// Revoke all other sessions
	if err := database.RevokeAllUserSessionsExcept(
		h.db, 
		userID.(uuid.UUID), 
		currentSessionID.(uuid.UUID),
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "All other sessions revoked successfully",
	})
}

// ForceMFAVerification forces MFA verification for all user sessions
func (h *EnhancedSessionHandler) ForceMFAVerification(c *gin.Context) {
	// Admin only endpoint
	isAdmin, exists := c.Get("is_admin")
	if !exists || !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get target user ID from request
	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Force MFA verification
	if err := database.ForceMFAVerification(h.db, req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to force MFA verification"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "MFA verification forced for all sessions",
	})
}

// ForcePasswordReset forces password reset for all user sessions
func (h *EnhancedSessionHandler) ForcePasswordReset(c *gin.Context) {
	// Admin only endpoint
	isAdmin, exists := c.Get("is_admin")
	if !exists || !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get target user ID from request
	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Force password reset
	if err := database.ForcePasswordReset(h.db, req.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to force password reset"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password reset forced for all sessions",
	})
}

// SuspendSuspiciousSessions suspends sessions that are deemed suspicious
func (h *EnhancedSessionHandler) SuspendSuspiciousSessions(c *gin.Context) {
	// Admin only endpoint
	isAdmin, exists := c.Get("is_admin")
	if !exists || !isAdmin.(bool) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get target user ID from request
	var req struct {
		UserID uuid.UUID `json:"user_id" binding:"required"`
		Reason string    `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Reason == "" {
		req.Reason = "Suspicious activity detected"
	}

	// Suspend suspicious sessions
	adminID := c.GetString("user_id")
	adminUUID, _ := uuid.Parse(adminID)
	
	if err := database.SuspendSuspiciousSessions(h.db, req.UserID, &adminUUID, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to suspend suspicious sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Suspicious sessions suspended",
	})
}

// MarkDeviceAsTrusted marks a device as trusted
func (h *EnhancedSessionHandler) MarkDeviceAsTrusted(c *gin.Context) {
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

	// Get device info
	deviceInfo, err := session.GetDeviceInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get device info"})
		return
	}

	// Mark as trusted
	deviceInfo.TrustedDevice = true
	deviceInfo.LastVerifiedAt = time.Now().Format(time.RFC3339)

	// Update device info
	if err := session.SetDeviceInfo(deviceInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update device info"})
		return
	}

	// Save to database
	if err := h.db.Model(&database.EnhancedSession{}).
		Where("id = ?", sessionID).
		Update("device_fingerprint", session.DeviceFingerprint).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save device info"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Device marked as trusted",
	})
}

// Helper methods

// detectDeviceType detects the device type from user agent
func (h *EnhancedSessionHandler) detectDeviceType(userAgent string) string {
	// Simple detection logic - in production use a proper user agent parser
	if utils.ContainsAny(userAgent, []string{"iPhone", "iPad", "Android", "Mobile"}) {
		if utils.ContainsAny(userAgent, []string{"iPad", "Tablet"}) {
			return "tablet"
		}
		return "mobile"
	}
	return "desktop"
}

// detectBrowser detects the browser from user agent
func (h *EnhancedSessionHandler) detectBrowser(userAgent string) string {
	// Simple detection logic - in production use a proper user agent parser
	if utils.Contains(userAgent, "Chrome") {
		return "chrome"
	} else if utils.Contains(userAgent, "Firefox") {
		return "firefox"
	} else if utils.Contains(userAgent, "Safari") {
		return "safari"
	} else if utils.Contains(userAgent, "Edge") {
		return "edge"
	} else if utils.Contains(userAgent, "Opera") {
		return "opera"
	}
	return "unknown"
}

// detectOS detects the operating system from user agent
func (h *EnhancedSessionHandler) detectOS(userAgent string) string {
	// Simple detection logic - in production use a proper user agent parser
	if utils.Contains(userAgent, "Windows") {
		return "windows"
	} else if utils.Contains(userAgent, "Mac OS") {
		return "macos"
	} else if utils.Contains(userAgent, "Linux") {
		return "linux"
	} else if utils.Contains(userAgent, "Android") {
		return "android"
	} else if utils.ContainsAny(userAgent, []string{"iPhone", "iPad", "iOS"}) {
		return "ios"
	}
	return "unknown"
}
