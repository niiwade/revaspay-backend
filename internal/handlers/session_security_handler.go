package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security"
	"github.com/revaspay/backend/internal/security/audit"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// SessionSecurityHandler handles session security operations
type SessionSecurityHandler struct {
	db             *gorm.DB
	auditLogger    *audit.Logger
	riskEvaluator  *security.SessionRiskEvaluator
}

// NewSessionSecurityHandler creates a new session security handler
func NewSessionSecurityHandler(db *gorm.DB) *SessionSecurityHandler {
	return &SessionSecurityHandler{
		db:             db,
		auditLogger:    audit.NewLogger(db),
		riskEvaluator:  security.NewSessionRiskEvaluator(db),
	}
}

// SessionRiskResponse represents the response for session risk assessment
type SessionRiskResponse struct {
	SessionID       string  `json:"session_id"`
	RiskScore       float64 `json:"risk_score"`
	RiskLevel       string  `json:"risk_level"`
	RequiresAction  bool    `json:"requires_action"`
	RecommendedAction string `json:"recommended_action,omitempty"`
}

// EvaluateSessionRisk evaluates the risk of the current session
func (h *SessionSecurityHandler) EvaluateSessionRisk(c *gin.Context) {
	// Get session ID from context
	sessionID, exists := c.Get("sessionID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "No active session"})
		return
	}

	// Convert to UUID
	sessionUUID, err := uuid.Parse(sessionID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	// Evaluate session risk
	score, riskLevel, factorScores := h.riskEvaluator.EvaluateSession(sessionUUID)

	// Update session with risk score
	err = security.UpdateSessionRiskScore(h.db, sessionUUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session risk"})
		return
	}

	// Determine if action is required
	requiresAction := riskLevel == security.RiskLevelHigh || riskLevel == security.RiskLevelCritical

	// Determine recommended action
	recommendedAction := ""
	if requiresAction {
		if riskLevel == security.RiskLevelCritical {
			recommendedAction = "verify_identity"
		} else {
			recommendedAction = "verify_device"
		}
	}

	// Log risk assessment
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeSecurity,
		audit.SeverityInfo,
		"Session risk assessment performed",
		nil,
		&sessionUUID,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"risk_score": score,
			"risk_level": riskLevel,
			"factors":    factorScores,
		},
	)

	// Return response
	c.JSON(http.StatusOK, SessionRiskResponse{
		SessionID:         sessionUUID.String(),
		RiskScore:         score,
		RiskLevel:         string(riskLevel),
		RequiresAction:    requiresAction,
		RecommendedAction: recommendedAction,
	})
}

// VerifySessionSecurity verifies the security of a session
func (h *SessionSecurityHandler) VerifySessionSecurity(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		VerificationType string `json:"verification_type" binding:"required"`
		VerificationData string `json:"verification_data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Parse session ID
	sessionUUID, err := uuid.Parse(req.SessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	// Get session
	var session database.EnhancedSession
	if err := h.db.Where("id = ?", sessionUUID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	// Check verification type
	switch req.VerificationType {
	case "mfa":
		// Verify MFA code
		userID := session.UserID
		
		// Get user's MFA settings
		mfaEnabled, err := database.IsMFAEnabled(h.db, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check MFA status"})
			return
		}

		if !mfaEnabled {
			c.JSON(http.StatusBadRequest, gin.H{"error": "MFA not enabled for this user"})
			return
		}

		// Verify MFA code
		verified, err := database.VerifyMFACode(h.db, userID, database.MFAMethodTOTP, req.VerificationData)
		if err != nil || !verified {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid MFA code"})
			
			// Log failed verification
			h.auditLogger.LogWithContext(
				c,
				audit.EventTypeSecurity,
				audit.SeverityWarning,
				"Failed MFA verification for suspicious session",
				&userID,
				&sessionUUID,
				c.ClientIP(),
				c.Request.UserAgent(),
				false,
				nil,
			)
			return
		}

		// Mark session as MFA verified
		if err := database.VerifyMFA(h.db, sessionUUID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
			return
		}

		// Update session status
		if err := h.db.Model(&session).Update("status", database.SessionStatusActive).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session status"})
			return
		}

		// Log successful verification
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeSecurity,
			audit.SeverityInfo,
			"Successful MFA verification for suspicious session",
			&userID,
			&sessionUUID,
			c.ClientIP(),
			c.Request.UserAgent(),
			true,
			nil,
		)

	case "security_questions":
		// Verify security questions
		userID := session.UserID
		
		// Parse verification data as question answers
		var answers map[string]string
		if err := utils.ParseJSONInto(req.VerificationData, &answers); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification data"})
			return
		}

		// Convert to map of question ID to answer
		questionAnswers := make(map[uuid.UUID]string)
		for questionIDStr, answer := range answers {
			questionID, err := uuid.Parse(questionIDStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid question ID format"})
				return
			}
			questionAnswers[questionID] = answer
		}

		// Verify security questions
		verified, err := database.VerifyUserSecurityQuestions(h.db, userID, questionAnswers)
		if err != nil || !verified {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid security question answers"})
			
			// Log failed verification
			h.auditLogger.LogWithContext(
				c,
				audit.EventTypeSecurity,
				audit.SeverityWarning,
				"Failed security question verification for suspicious session",
				&userID,
				&sessionUUID,
				c.ClientIP(),
				c.Request.UserAgent(),
				false,
				nil,
			)
			return
		}

		// Update session status
		if err := h.db.Model(&session).Update("status", database.SessionStatusActive).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session status"})
			return
		}

		// Log successful verification
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeSecurity,
			audit.SeverityInfo,
			"Successful security question verification for suspicious session",
			&userID,
			&sessionUUID,
			c.ClientIP(),
			c.Request.UserAgent(),
			true,
			nil,
		)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported verification type"})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"verified": true,
		"session_id": sessionUUID.String(),
		"status": "active",
	})
}

// RevokeRiskySessions revokes all risky sessions for a user
func (h *SessionSecurityHandler) RevokeRiskySessions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse user ID
	userUUID, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Find all risky sessions for the user
	var sessions []database.EnhancedSession
	if err := h.db.Where("user_id = ? AND (risk_level = ? OR risk_level = ?)", 
		userUUID, security.RiskLevelHigh, security.RiskLevelCritical).
		Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch sessions"})
		return
	}

	// Revoke all risky sessions
	revokedCount := 0
	for _, session := range sessions {
		if err := h.db.Model(&session).Updates(map[string]interface{}{
			"status": database.SessionStatusRevoked,
		}).Error; err != nil {
			continue
		}
		revokedCount++

		// Log session revocation
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeSecurity,
			audit.SeverityInfo,
			"Revoked risky session",
			&userUUID,
			&session.ID,
			c.ClientIP(),
			c.Request.UserAgent(),
			true,
			map[string]interface{}{
				"risk_score": session.RiskScore,
				"risk_level": session.RiskLevel,
			},
		)
	}

	// Return response
	c.JSON(http.StatusOK, gin.H{
		"revoked_sessions": revokedCount,
		"total_risky_sessions": len(sessions),
	})
}

// GetSessionSecurityHistory gets the security history for a session
func (h *SessionSecurityHandler) GetSessionSecurityHistory(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Session ID is required"})
		return
	}

	// Parse session ID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	// Get session
	var session database.EnhancedSession
	if err := h.db.Where("id = ?", sessionUUID).First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	// Get user ID from context
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse user ID
	userUUID, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Ensure the session belongs to the user
	if session.UserID != userUUID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to access this session"})
		return
	}

	// Get audit logs for the session
	auditLogs, err := h.auditLogger.GetAuditLogsForSession(sessionUUID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch audit logs"})
		return
	}

	// Return response
	c.JSON(http.StatusOK, gin.H{
		"session_id": sessionUUID.String(),
		"created_at": session.CreatedAt,
		"expires_at": session.ExpiresAt,
		"status": session.Status,
		"risk_score": session.RiskScore,
		"risk_level": session.RiskLevel,
		"audit_logs": auditLogs,
	})
}

// SessionSecurityMiddleware is a middleware that checks session security
func (h *SessionSecurityHandler) SessionSecurityMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for authentication routes
		if c.Request.URL.Path == "/api/auth/login" || 
		   c.Request.URL.Path == "/api/auth/register" ||
		   c.Request.URL.Path == "/api/auth/refresh" {
			c.Next()
			return
		}

		// Get session ID from context
		sessionID, exists := c.Get("sessionID")
		if !exists {
			c.Next() // Let the auth middleware handle this
			return
		}

		// Parse session ID
		sessionUUID, err := uuid.Parse(sessionID.(string))
		if err != nil {
			c.Next() // Let the auth middleware handle this
			return
		}

		// Check if session is suspicious
		needsVerification, riskLevel := security.CheckSessionRisk(h.db, sessionUUID)
		if needsVerification {
			// For API requests, return 403 with additional info
			if c.GetHeader("Accept") == "application/json" || c.Request.Method != "GET" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "Additional verification required",
					"risk_level": riskLevel,
					"session_id": sessionUUID.String(),
					"verification_required": true,
				})
				return
			}
			
			// For web requests, redirect to verification page
			c.Redirect(http.StatusFound, "/verify-session?session_id="+sessionUUID.String())
			c.Abort()
			return
		}

		// Update session activity
		go func(db *gorm.DB, sid uuid.UUID, ip string) {
			database.UpdateSessionActivityWithAction(db, sid, ip, "session_security_check", "middleware")
		}(h.db, sessionUUID, c.ClientIP())

		c.Next()
	}
}
