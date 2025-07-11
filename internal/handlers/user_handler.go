package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserHandler handles user related requests
type UserHandler struct {
	db *gorm.DB
}

// TwoFactorSetupResponse represents the response for 2FA setup
type TwoFactorSetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qr_code_url"`
}

// TwoFactorVerifyRequest represents the request to verify a 2FA code
type TwoFactorVerifyRequest struct {
	Code string `json:"code" binding:"required"`
}

// PasswordUpdateRequest represents the request to update a password
type PasswordUpdateRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

// NewUserHandler creates a new user handler
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// GetProfile returns the user's profile
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"id":                user.ID,
		"username":          user.Username,
		"email":             user.Email,
		"first_name":        user.FirstName,
		"last_name":         user.LastName,
		"profile_pic_url":   user.ProfilePicURL,
		"is_verified":       user.IsVerified,
		"is_admin":          user.IsAdmin,
		"two_factor_enabled": user.TwoFactorEnabled,
		"referral_code":     user.ReferralCode,
		"created_at":        user.CreatedAt,
		"updated_at":        user.UpdatedAt,
	})
}

// UpdateProfile updates the user's profile
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Create a map for updated fields to avoid updating sensitive fields
	var updateData map[string]interface{}
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Remove fields that shouldn't be updated directly
	delete(updateData, "id")
	delete(updateData, "email") // Email updates should be handled separately with verification
	delete(updateData, "password")
	delete(updateData, "is_verified")
	delete(updateData, "is_admin")
	delete(updateData, "two_factor_enabled")
	delete(updateData, "two_factor_secret")
	delete(updateData, "referral_code")
	delete(updateData, "created_at")
	delete(updateData, "updated_at")
	delete(updateData, "deleted_at")
	
	// Update user with allowed fields
	if err := h.db.Model(&user).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}
	
	// Refresh user data
	h.db.First(&user, "id = ?", userID)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user": gin.H{
			"id":                user.ID,
			"username":          user.Username,
			"email":             user.Email,
			"first_name":        user.FirstName,
			"last_name":         user.LastName,
			"profile_pic_url":   user.ProfilePicURL,
			"is_verified":       user.IsVerified,
			"two_factor_enabled": user.TwoFactorEnabled,
			"referral_code":     user.ReferralCode,
		},
	})
}

// GetUserByID returns a specific user by ID (admin only)
func (h *UserHandler) GetUserByID(c *gin.Context) {
	userID := c.Param("id")
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	c.JSON(http.StatusOK, user)
}

// GetAllUsers returns all users (admin only)
func (h *UserHandler) GetAllUsers(c *gin.Context) {
	var users []database.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch users"})
		return
	}
	
	c.JSON(http.StatusOK, users)
}

// VerifyUser verifies a user (admin only)
func (h *UserHandler) VerifyUser(c *gin.Context) {
	userID := c.Param("id")
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	user.IsVerified = true
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify user"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "User verified successfully"})
}

// Enable2FA initiates 2FA setup for a user
func (h *UserHandler) Enable2FA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Check if 2FA is already enabled
	if user.TwoFactorEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor authentication is already enabled"})
		return
	}
	
	// Generate new TOTP secret
	secret := utils.GenerateOTPSecret()
	
	// Generate QR code URL
	qrCodeURL := utils.GenerateOTPQRCode(secret, user.Email, "RevasPay")
	
	// Store secret temporarily (not enabled yet until verified)
	user.TwoFactorSecret = secret
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save 2FA secret"})
		return
	}
	
	c.JSON(http.StatusOK, TwoFactorSetupResponse{
		Secret:    secret,
		QRCodeURL: qrCodeURL,
	})
}

// Verify2FA verifies a 2FA code and enables 2FA for the user
func (h *UserHandler) Verify2FA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req TwoFactorVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Verify TOTP code
	if !utils.ValidateTOTP(user.TwoFactorSecret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code"})
		return
	}
	
	// Enable 2FA
	user.TwoFactorEnabled = true
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable two-factor authentication"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication enabled successfully"})
}

// Disable2FA disables 2FA for the user
func (h *UserHandler) Disable2FA(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req TwoFactorVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Check if 2FA is enabled
	if !user.TwoFactorEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Two-factor authentication is not enabled"})
		return
	}
	
	// Verify TOTP code before disabling
	if !utils.ValidateTOTP(user.TwoFactorSecret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification code"})
		return
	}
	
	// Disable 2FA
	user.TwoFactorEnabled = false
	user.TwoFactorSecret = "" // Clear the secret
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable two-factor authentication"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Two-factor authentication disabled successfully"})
}

// UpdatePassword updates the user's password
func (h *UserHandler) UpdatePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")
	
	var req PasswordUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	
	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Current password is incorrect"})
		return
	}
	
	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}
	
	// Update password
	user.Password = string(hashedPassword)
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully"})
}
