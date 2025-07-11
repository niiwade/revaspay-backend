package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security"
	"github.com/revaspay/backend/internal/security/audit"
	"gorm.io/gorm"
)

// PasswordHandler handles password-related operations
type PasswordHandler struct {
	db            *gorm.DB
	auditLogger   *audit.Logger
	passwordPolicy *security.PasswordPolicy
}

// PasswordChangeRequest represents a request to update a password
type PasswordChangeRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// PasswordResetRequest represents a request to reset a password
type PasswordResetRequest struct {
	Token           string `json:"token" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
	ConfirmPassword string `json:"confirm_password" binding:"required"`
}

// NewPasswordHandler creates a new password handler
func NewPasswordHandler(db *gorm.DB) *PasswordHandler {
	return &PasswordHandler{
		db:            db,
		auditLogger:   audit.NewLogger(db),
		passwordPolicy: security.DefaultPasswordPolicy(),
	}
}

// UpdatePassword updates a user's password
func (h *PasswordHandler) UpdatePassword(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request
	var req PasswordChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate password confirmation
	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
		return
	}

	// Get user
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}

	// Verify current password
	if !user.CheckPassword(req.CurrentPassword) {
		// Log failed password update attempt
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed password update attempt - incorrect current password",
			userID.(*uuid.UUID),
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			false,
			nil,
		)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}

	// Validate new password against policy
	if err := h.passwordPolicy.ValidatePassword(req.NewPassword, user.Username, user.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update password
	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	// Save user
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user"})
		return
	}

	// Log successful password update
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeAuth,
		audit.SeverityInfo,
		"Password updated successfully",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	// Revoke all other sessions for security
	if err := database.RevokeAllUserSessionsExcept(h.db, *userID.(*uuid.UUID), uuid.MustParse(c.GetString("session_id"))); err != nil {
		// Just log the error but don't fail the request
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed to revoke other sessions after password change",
			userID.(*uuid.UUID),
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			false,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Password updated successfully",
	})
}

// ResetPassword resets a user's password using a reset token
func (h *PasswordHandler) ResetPassword(c *gin.Context) {
	// Parse request
	var req PasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate password confirmation
	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
		return
	}

	// Find reset token
	var resetToken database.PasswordResetToken
	if err := h.db.Where("token = ?", req.Token).First(&resetToken).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired token"})
		return
	}

	// Check if token is expired
	if resetToken.ExpiresAt < time.Now().Unix() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token has expired"})
		return
	}

	// Get user
	userID, err := uuid.Parse(resetToken.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user ID in token"})
		return
	}

	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}

	// Validate new password against policy
	if err := h.passwordPolicy.ValidatePassword(req.NewPassword, user.Username, user.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update password
	if err := user.SetPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	// Save user
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user"})
		return
	}

	// Delete the used token
	if err := h.db.Delete(&resetToken).Error; err != nil {
		// Just log the error but don't fail the request
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed to delete used password reset token",
			&userID,
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			false,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	// Revoke all sessions for this user
	// Using RevokeAllUserSessionsExcept with a nil UUID to revoke all sessions
	emptyUUID := uuid.Nil
	if err := database.RevokeAllUserSessionsExcept(h.db, userID, emptyUUID); err != nil {
		// Just log the error but don't fail the request
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed to revoke sessions after password reset",
			&userID,
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			false,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	// Log successful password reset
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeAuth,
		audit.SeverityInfo,
		"Password reset successfully",
		&userID,
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Password reset successfully",
	})
}

// EvaluatePasswordStrength evaluates the strength of a password
func (h *PasswordHandler) EvaluatePasswordStrength(c *gin.Context) {
	// Parse request
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Evaluate password strength
	strength := security.EvaluatePasswordStrength(req.Password)
	strengthStr := security.PasswordStrengthToString(strength)

	c.JSON(http.StatusOK, gin.H{
		"strength": strengthStr,
		"score":    int(strength),
	})
}
