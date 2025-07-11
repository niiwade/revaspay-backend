package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security/audit"
	"gorm.io/gorm"
)

// SecurityQuestionHandler handles security question operations
type SecurityQuestionHandler struct {
	db          *gorm.DB
	auditLogger *audit.Logger
}

// SecurityQuestionAnswerRequest represents a request to set a security question answer
type SecurityQuestionAnswerRequest struct {
	QuestionID uuid.UUID `json:"question_id" binding:"required"`
	Answer     string    `json:"answer" binding:"required"`
}

// VerifySecurityQuestionsRequest represents a request to verify security question answers
type VerifySecurityQuestionsRequest struct {
	Answers map[string]string `json:"answers" binding:"required"` // Map of question ID to answer
}

// NewSecurityQuestionHandler creates a new security question handler
func NewSecurityQuestionHandler(db *gorm.DB) *SecurityQuestionHandler {
	return &SecurityQuestionHandler{
		db:          db,
		auditLogger: audit.NewLogger(db),
	}
}

// GetSecurityQuestions gets all available security questions
func (h *SecurityQuestionHandler) GetSecurityQuestions(c *gin.Context) {
	questions, err := database.GetSecurityQuestions(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get security questions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"questions": questions,
	})
}

// GetUserSecurityQuestions gets all security questions for the authenticated user
func (h *SecurityQuestionHandler) GetUserSecurityQuestions(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get user's security questions
	questions, err := database.GetUserSecurityQuestions(h.db, userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get security questions"})
		return
	}

	// Prepare response without exposing answer hashes
	var response []gin.H
	for _, q := range questions {
		response = append(response, gin.H{
			"id":                  q.ID,
			"security_question_id": q.SecurityQuestionID,
			"question":            q.SecurityQuestion.Question,
			"created_at":          q.CreatedAt,
			"updated_at":          q.UpdatedAt,
		})
	}

	// Log the request
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Security questions retrieved",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	c.JSON(http.StatusOK, gin.H{
		"security_questions": response,
	})
}

// SetSecurityQuestionAnswer sets a security question answer for the authenticated user
func (h *SecurityQuestionHandler) SetSecurityQuestionAnswer(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request
	var req SecurityQuestionAnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Check if the question exists
	var question database.SecurityQuestion
	if err := h.db.First(&question, "id = ?", req.QuestionID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid security question"})
		return
	}

	// Check if the user already has an answer for this question
	var existingAnswer database.UserSecurityQuestion
	result := h.db.Where("user_id = ? AND security_question_id = ?", userID, req.QuestionID).First(&existingAnswer)
	
	var userQuestion *database.UserSecurityQuestion
	var err error

	if result.Error == nil {
		// Update existing answer
		err = database.UpdateUserSecurityQuestionAnswer(h.db, existingAnswer.ID, req.Answer)
		userQuestion = &existingAnswer
	} else {
		// Create new answer
		userQuestion, err = database.CreateUserSecurityQuestion(h.db, userID.(uuid.UUID), req.QuestionID, req.Answer)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set security question answer"})
		return
	}

	// Log the action
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Security question answer set",
		userID.(*uuid.UUID),
		&userQuestion.ID,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"question_id": req.QuestionID.String(),
		},
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Security question answer set successfully",
		"security_question": gin.H{
			"id":                  userQuestion.ID,
			"security_question_id": userQuestion.SecurityQuestionID,
			"question":            question.Question,
			"created_at":          userQuestion.CreatedAt,
			"updated_at":          userQuestion.UpdatedAt,
		},
	})
}

// DeleteSecurityQuestionAnswer deletes a security question answer
func (h *SecurityQuestionHandler) DeleteSecurityQuestionAnswer(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get security question ID from path
	questionID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid security question ID"})
		return
	}

	// Check if the user has an answer for this question
	var userQuestion database.UserSecurityQuestion
	if err := h.db.Where("id = ? AND user_id = ?", questionID, userID).First(&userQuestion).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Security question answer not found"})
		return
	}

	// Delete the answer
	if err := database.DeleteUserSecurityQuestion(h.db, userQuestion.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete security question answer"})
		return
	}

	// Log the action
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Security question answer deleted",
		userID.(*uuid.UUID),
		&userQuestion.ID,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"question_id": userQuestion.SecurityQuestionID.String(),
		},
	)

	c.JSON(http.StatusOK, gin.H{
		"message": "Security question answer deleted successfully",
	})
}

// VerifySecurityQuestions verifies security question answers for account recovery
func (h *SecurityQuestionHandler) VerifySecurityQuestions(c *gin.Context) {
	// Parse request
	var req VerifySecurityQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get email from query parameter
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email is required"})
		return
	}

	// Find user by email
	user, err := database.FindUserByEmail(h.db, email)
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
			log.Printf("Invalid question ID format: %s", questionIDStr)
			continue
		}
		questionAnswers[questionID] = answer
	}

	// Verify answers
	verified, err := database.VerifyUserSecurityQuestions(h.db, user.ID, questionAnswers)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify security questions"})
		return
	}

	// Log the verification attempt
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityWarning, // Higher severity for account recovery attempts
		"Security questions verification attempt",
		&user.ID,
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		verified,
		map[string]interface{}{
			"email":    email,
			"verified": verified,
		},
	)

	if verified {
		// Generate a password reset token if verification is successful
		// This would typically be handled by the auth handler, but we'll include it here for completeness
		c.JSON(http.StatusOK, gin.H{
			"verified": true,
			"message":  "Security questions verified successfully",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"verified": false,
			"message":  "Security questions verification failed",
		})
	}
}
