package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security"
	"github.com/revaspay/backend/internal/security/audit"
	"gorm.io/gorm"
)

// SecurityMiddleware provides security-related middleware functions
type SecurityMiddleware struct {
	db           *gorm.DB
	riskAssessor *security.RiskAssessor
	auditLogger  *audit.Logger
	// Brute force protection settings
	maxFailedAttempts int
	lockoutDuration   time.Duration
}

// NewSecurityMiddleware creates a new security middleware
func NewSecurityMiddleware(db *gorm.DB) *SecurityMiddleware {
	return &SecurityMiddleware{
		db:               db,
		riskAssessor:     security.NewRiskAssessor(db),
		auditLogger:      audit.NewLogger(db),
		maxFailedAttempts: 5,                  // 5 failed attempts before lockout
		lockoutDuration:   15 * time.Minute,   // 15 minute lockout duration
	}
}

// BruteForceProtection checks for brute force attempts
func (m *SecurityMiddleware) BruteForceProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		ipAddress := c.ClientIP()
		userAgent := c.Request.UserAgent()

		// Authentication routes that should be protected
		protectedRoutes := map[string]bool{
			"/api/auth/login":         true,
			"/api/auth/refresh-token": true,
			"/api/auth/reset-password": true,
			"/api/auth/mfa/verify":    true,
		}

		if protectedRoutes[c.FullPath()] {
			// Extract user ID or email if available in request body
			var userID *uuid.UUID
			var userEmail string

			// Save the request body
			bodyBytes, err := c.GetRawData()
			if err == nil {
				// Restore the request body for later handlers
				c.Request.Body = http.MaxBytesReader(c.Writer, http.NoBody, int64(len(bodyBytes)))

				// Try to extract email from request
				var reqData map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &reqData); err == nil {
					if email, ok := reqData["email"].(string); ok && email != "" {
						userEmail = email
						// Look up user by email
						var user database.User
						if err := m.db.Where("email = ?", email).First(&user).Error; err == nil {
							userID = &user.ID
						}
					}
				}

				// Restore the request body
				c.Request.Body = http.MaxBytesReader(c.Writer, io.NopCloser(bytes.NewReader(bodyBytes)), int64(len(bodyBytes)))
			}

			// Check for IP-based lockout
			var failedAttempts []database.FailedLoginAttempt
			lockoutTime := time.Now().Add(-m.lockoutDuration)

			if err := m.db.Where("ip_address = ? AND created_at > ?", ipAddress, lockoutTime).Find(&failedAttempts).Error; err == nil {
				if len(failedAttempts) >= m.maxFailedAttempts {
					// Calculate time until lockout expires
					oldestAttempt := failedAttempts[0].CreatedAt
					for _, attempt := range failedAttempts {
						if attempt.CreatedAt.Before(oldestAttempt) {
							oldestAttempt = attempt.CreatedAt
						}
					}
					
					lockoutExpiry := oldestAttempt.Add(m.lockoutDuration)
					retryAfter := int(time.Until(lockoutExpiry).Seconds())
					if retryAfter < 0 {
						retryAfter = 1 // Minimum 1 second
					}

					// Log the blocked attempt
					metadata := map[string]interface{}{
						"ip_address":      ipAddress,
						"attempt_count":   len(failedAttempts),
						"lockout_expires": lockoutExpiry,
					}
					
					if userEmail != "" {
						metadata["email"] = userEmail
					}

					// Log with audit logger
					if err := m.auditLogger.LogWithContext(
						c,
						audit.EventTypeAuth,
						audit.SeverityWarning,
						"Blocked brute force attempt",
						userID,
						nil,
						ipAddress,
						userAgent,
						false,
						metadata,
					); err != nil {
						log.Printf("Failed to log brute force attempt: %v", err)
					}

					// Return 429 Too Many Requests with Retry-After header
					c.Header("Retry-After", string(retryAfter))
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
						"error":      "Account temporarily locked due to too many failed login attempts",
						"retry_after": retryAfter,
					})
					return
				}
			}

			// Check for account-specific lockout if we have a user ID
			if userID != nil {
				var userFailedAttempts []database.FailedLoginAttempt
				if err := m.db.Where("user_id = ? AND created_at > ?", userID, lockoutTime).Find(&userFailedAttempts).Error; err == nil {
					if len(userFailedAttempts) >= m.maxFailedAttempts {
						// Calculate time until lockout expires
						oldestAttempt := userFailedAttempts[0].CreatedAt
						for _, attempt := range userFailedAttempts {
							if attempt.CreatedAt.Before(oldestAttempt) {
								oldestAttempt = attempt.CreatedAt
							}
						}
						
						lockoutExpiry := oldestAttempt.Add(m.lockoutDuration)
						retryAfter := int(time.Until(lockoutExpiry).Seconds())
						if retryAfter < 0 {
							retryAfter = 1 // Minimum 1 second
						}

						// Log the blocked attempt
						metadata := map[string]interface{}{
							"ip_address":      ipAddress,
							"attempt_count":   len(userFailedAttempts),
							"lockout_expires": lockoutExpiry,
						}
						
						if userEmail != "" {
							metadata["email"] = userEmail
						}

						// Log with audit logger
						if err := m.auditLogger.LogWithContext(
							c,
							audit.EventTypeAuth,
							audit.SeverityWarning,
							"Account temporarily locked due to too many failed login attempts",
							userID,
							nil,
							ipAddress,
							userAgent,
							false,
							metadata,
						); err != nil {
							log.Printf("Failed to log account lockout: %v", err)
						}

						// Return 429 Too Many Requests with Retry-After header
						c.Header("Retry-After", string(retryAfter))
						c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
							"error":      "Account temporarily locked due to too many failed login attempts",
							"retry_after": retryAfter,
						})
						return
					}
				}
			}
		}

		c.Next()
	}
}

// SessionActivity updates session activity and performs risk assessment
func (m *SecurityMiddleware) SessionActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for authentication routes
		if c.FullPath() == "/api/auth/login" || c.FullPath() == "/api/auth/register" {
			c.Next()
			return
		}

		// Get session ID from context (set by auth middleware)
		sessionID, exists := c.Get("session_id")
		if !exists {
			c.Next()
			return
		}

		// Update session activity
		database.UpdateSessionActivity(m.db, sessionID.(uuid.UUID), c.ClientIP(), "", "") // Using the version in enhanced_session.go

		c.Next()
	}
}

// RiskBasedAuthentication performs risk assessment on authenticated requests
func (m *SecurityMiddleware) RiskBasedAuthentication() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for authentication routes
		if c.FullPath() == "/api/auth/login" || c.FullPath() == "/api/auth/register" {
			c.Next()
			return
		}

		// Check if user is authenticated (set by auth middleware)
		_, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		// Get session ID from context
		sessionID, exists := c.Get("session_id")
		if !exists {
			c.Next()
			return
		}

		// Get session
		var session database.EnhancedSession
		if err := m.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
			c.Next()
			return
		}

		// Get metadata
		metadata, err := session.GetMetadata()
		if err != nil || metadata == nil {
			c.Next()
			return
		}

		// Check if session requires additional verification
		if metadata.RiskScore > 50 {
			// For high-risk sessions, check if MFA was verified recently
			mfaRequired := true

			if metadata.MFAVerifiedAt != nil {
				// If MFA was verified in the last 30 minutes, don't require it again
				if time.Since(*metadata.MFAVerifiedAt) < 30*time.Minute {
					mfaRequired = false
				}
			}

			if mfaRequired {
				// For sensitive operations, require MFA verification
				if isSensitiveOperation(c.FullPath()) {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"error":           "Additional verification required",
						"require_mfa":     true,
						"verification_id": uuid.New().String(),
					})
					return
				}
			}
		}

		// Check if password reset is required
		if metadata.ForcePasswordReset {
			// Allow only password reset endpoint
			if c.FullPath() != "/api/auth/reset-password" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":                "Password reset required",
					"force_password_reset": true,
				})
				return
			}
		}

		c.Next()
	}
}

// VerifySessionStatus checks if the session is active
func (m *SecurityMiddleware) VerifySessionStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for authentication routes
		if c.FullPath() == "/api/auth/login" || c.FullPath() == "/api/auth/register" {
			c.Next()
			return
		}

		// Get session ID from context
		sessionID, exists := c.Get("session_id")
		if !exists {
			c.Next()
			return
		}

		// Check session status
		var session database.EnhancedSession
		if err := m.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
			c.Next()
			return
		}

		// Check if session is active
		if session.Status != database.SessionStatusActive {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":  "Session is no longer active",
				"status": string(session.Status),
			})
			return
		}

		c.Next()
	}
}

// Helper functions

// isSensitiveOperation checks if the operation is sensitive and requires additional verification
func isSensitiveOperation(path string) bool {
	sensitiveOperations := []string{
		"/api/wallet/withdraw",
		"/api/account/update-password",
		"/api/account/update-email",
		"/api/account/update-2fa",
		"/api/account/delete",
		"/api/payment/create-link",
		"/api/admin/",
	}

	for _, op := range sensitiveOperations {
		if strings.HasPrefix(path, op) {
			return true
		}
	}

	return false
}
