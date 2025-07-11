package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security"
	"github.com/revaspay/backend/internal/security/audit"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// RecoveryHandler handles account recovery operations
type RecoveryHandler struct {
	db          *gorm.DB
	auditLogger *audit.Logger
	protection  *security.RecoveryProtection
}

// RecoveryRequest represents a request to initiate account recovery
type RecoveryRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// RecoveryVerifyRequest represents a request to verify security questions
type RecoveryVerifyRequest struct {
	Email   string            `json:"email" binding:"required,email"`
	Answers map[string]string `json:"answers" binding:"required"` // Map of question ID to answer
}

// Using database.RecoveryToken instead of defining it here

// NewRecoveryHandler creates a new recovery handler
func NewRecoveryHandler(db *gorm.DB) *RecoveryHandler {
	return &RecoveryHandler{
		db:          db,
		auditLogger: audit.NewLogger(db),
		protection:  security.NewRecoveryProtection(security.DefaultRecoveryProtectionConfig()),
	}
}

// InitiateRecovery initiates the account recovery process
func (h *RecoveryHandler) InitiateRecovery(c *gin.Context) {
	var req RecoveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Find user by email
	user, err := database.FindUserByEmail(h.db, req.Email)
	if err != nil {
		// Don't reveal if the email exists or not for security
		c.JSON(http.StatusOK, gin.H{
			"message": "If your email is registered, you will receive recovery instructions",
		})
		return
	}

	// Check if user has security questions set up
	questions, err := database.GetUserSecurityQuestions(h.db, user.ID)
	if err != nil || len(questions) < 2 {
		// User doesn't have enough security questions set up
		// Send email with link to reset password via email verification
		go h.sendRecoveryEmail(user)

		// Log the recovery attempt
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityInfo,
			"Account recovery initiated via email",
			&user.ID,
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			true,
			map[string]interface{}{
				"email": req.Email,
				"method": "email",
			},
		)

		c.JSON(http.StatusOK, gin.H{
			"message": "If your email is registered, you will receive recovery instructions",
			"recovery_method": "email",
		})
		return
	}

	// User has security questions, return them for verification
	var questionList []gin.H
	for _, q := range questions {
		questionList = append(questionList, gin.H{
			"id":       q.ID.String(),
			"question": q.SecurityQuestion.Question,
		})
	}

	// Log the recovery attempt
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeAuth,
		audit.SeverityInfo,
		"Account recovery initiated via security questions",
		&user.ID,
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"email": req.Email,
			"method": "security_questions",
		},
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Please answer your security questions to recover your account",
		"recovery_method": "security_questions",
		"questions": questionList,
	})
}

// VerifySecurityQuestions verifies security question answers for account recovery
func (h *RecoveryHandler) VerifySecurityQuestions(c *gin.Context) {
	var req RecoveryVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check for brute force attempts
	blocked, lockoutEnd := h.protection.IsBlocked(req.Email, nil, c.ClientIP())
	if blocked {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "Too many failed attempts",
			"blocked_until": lockoutEnd.Unix(),
		})
		return
	}

	// Find user by email
	user, err := database.FindUserByEmail(h.db, req.Email)
	if err != nil {
		// Don't reveal if the email exists or not
		c.JSON(http.StatusOK, gin.H{"verified": false})
		return
	}

	// Convert string map to UUID map
	questionAnswers := make(map[uuid.UUID]string)
	for questionIDStr, answer := range req.Answers {
		questionID, err := uuid.Parse(questionIDStr)
		if err != nil {
			continue
		}
		questionAnswers[questionID] = answer
	}

	// Verify answers
	verified, err := database.VerifyUserSecurityQuestions(h.db, user.ID, questionAnswers)
	if err != nil || !verified {
		// Record failed attempt for brute force protection
		h.protection.RecordFailedAttempt(req.Email, &user.ID, c.ClientIP(), c.Request.UserAgent())
		
		// Log failed verification
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed security questions verification for account recovery",
			&user.ID,
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			false,
			map[string]interface{}{
				"email": req.Email,
			},
		)

		c.JSON(http.StatusOK, gin.H{
			"verified": false,
			"message":  "Security questions verification failed",
		})
		return
	}

	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate recovery token"})
		return
	}
	tokenStr := hex.EncodeToString(tokenBytes)
	
	// Create recovery token
	recoveryToken, err := database.CreateAccountRecoveryToken(h.db, user.ID, tokenStr, time.Now().Add(1*time.Hour))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate recovery token"})
		return
	}

	// Log successful verification
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeAuth,
		audit.SeverityInfo,
		"Successful security questions verification for account recovery",
		&user.ID,
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"email": req.Email,
		},
	)

	c.JSON(http.StatusOK, gin.H{
		"verified": true,
		"message":  "Security questions verified successfully",
		"token":    recoveryToken.Token,
	})
}

// This function has been replaced by direct calls to database.CreateRecoveryToken

// sendRecoveryEmail sends a recovery email to the user
func (h *RecoveryHandler) sendRecoveryEmail(user *database.User) {
	// Generate password reset token
	token := utils.GenerateSecureToken(32)
	// GenerateSecureToken doesn't return an error

	// Create password reset token record
	resetToken := database.PasswordResetToken{
		UserID:    user.ID.String(),
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(), // Token expires in 24 hours
		CreatedAt: time.Now().Unix(),
	}

	// Save to database
	if err := h.db.Create(&resetToken).Error; err != nil {
		return
	}

	// In a real implementation, we would send an email with a link to reset password
	// For now, we'll just log it
	// TODO: Implement email sending
}

// ValidateRecoveryToken validates a recovery token
func (h *RecoveryHandler) ValidateRecoveryToken(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
		return
	}

	// Check for brute force attempts
	blocked, lockoutEnd := h.protection.IsBlocked("", nil, c.ClientIP())
	if blocked {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "Too many failed attempts",
			"blocked_until": lockoutEnd.Unix(),
		})
		return
	}

	// Find token in database
	recoveryToken, err := database.GetRecoveryTokenByToken(h.db, token)
	if err != nil {
		// Record failed attempt
		h.protection.RecordFailedAttempt("", nil, c.ClientIP(), c.Request.UserAgent())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired token"})
		return
	}

	// Check if token is expired
	if time.Now().After(recoveryToken.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token has expired"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid": true,
		"user_id": recoveryToken.UserID,
	})
}

// CompleteRecovery completes the account recovery process
func (h *RecoveryHandler) CompleteRecovery(c *gin.Context) {
	var req struct {
		Token           string `json:"token" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
		ConfirmPassword string `json:"confirm_password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check for brute force attempts
	blocked, lockoutEnd := h.protection.IsBlocked("", nil, c.ClientIP())
	if blocked {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":   "Too many failed attempts",
			"blocked_until": lockoutEnd.Unix(),
		})
		return
	}

	// Validate password confirmation
	if req.NewPassword != req.ConfirmPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
		return
	}

	// Find token in database
	recoveryToken, err := database.GetRecoveryTokenByToken(h.db, req.Token)
	if err != nil {
		// Record failed attempt
		h.protection.RecordFailedAttempt("", nil, c.ClientIP(), c.Request.UserAgent())
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired token"})
		return
	}

	// Check if token is expired
	if time.Now().After(recoveryToken.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token has expired"})
		return
	}

	// Get user
	var user database.User
	if err := h.db.First(&user, "id = ?", recoveryToken.UserID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
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

	// Mark token as used
	if err := database.MarkRecoveryTokenAsUsed(h.db, recoveryToken.ID); err != nil {
		// Just log the error but don't fail the request
		h.auditLogger.LogWithContext(
			context.Background(),
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed to mark recovery token as used",
			&user.ID,
			nil,
			"",
			"",
			false,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	// Revoke all sessions for this user
	// Using RevokeAllUserSessionsExcept with a nil UUID to revoke all sessions
	emptyUUID := uuid.Nil
	if err := database.RevokeAllUserSessionsExcept(h.db, user.ID, emptyUUID); err != nil {
		// Just log the error but don't fail the request
		h.auditLogger.LogWithContext(
			context.Background(),
			audit.EventTypeAuth,
			audit.SeverityWarning,
			"Failed to revoke sessions after account recovery",
			&user.ID,
			nil,
			"",
			"",
			false,
			map[string]interface{}{
				"error": err.Error(),
			},
		)
	}

	// Log successful recovery
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeAuth,
		audit.SeverityInfo,
		"Account recovered successfully",
		&user.ID,
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Account recovered successfully",
	})
}
