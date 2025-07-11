package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/email"
	"github.com/revaspay/backend/internal/utils"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

// AuthHandler handles authentication related requests
type AuthHandler struct {
	db          *gorm.DB
	emailService *email.EmailService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{
		db:          db,
		emailService: email.NewEmailService(),
	}
}

// SignupRequest represents the request body for signup
type SignupRequest struct {
	Username     string `json:"username" binding:"required"`
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required,min=8"`
	FirstName    string `json:"first_name" binding:"required"`
	LastName     string `json:"last_name" binding:"required"`
	ReferralCode string `json:"referral_code"`
}

// LoginRequest represents the request body for login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	TOTPCode string `json:"totp_code"` // Optional, required only if 2FA is enabled
}

// TokenResponse represents the response for token requests
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"`
}

// Signup handles user registration
func (h *AuthHandler) Signup(c *gin.Context) {
	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user already exists
	var existingUser database.User
	if result := h.db.Where("email = ? OR username = ?", req.Email, req.Username).First(&existingUser); result.RowsAffected > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Email or username already in use"})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}

	// Generate referral code
	referralCode := utils.GenerateReferralCode(8)

	// Find referrer if referral code provided
	var referrerID *uuid.UUID
	if req.ReferralCode != "" {
		var referrer database.User
		if result := h.db.Where("referral_code = ?", req.ReferralCode).First(&referrer); result.RowsAffected > 0 {
			referrerID = &referrer.ID
		}
	}

	// Create user
	user := database.User{
		Username:     req.Username,
		Email:        req.Email,
		Password:     string(hashedPassword),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		ReferralCode: referralCode,
		ReferredBy:   referrerID,
	}

	tx := h.db.Begin()

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
		return
	}

	// Create wallet for user
	wallet := database.Wallet{
		UserID:   user.ID,
		Balance:  0,
		Currency: "USD",
	}

	if err := tx.Create(&wallet).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
		return
	}

	// Create referral if user was referred
	if referrerID != nil {
		referral := database.Referral{
			ReferrerID: *referrerID,
			ReferredID: user.ID,
			Status:     "pending",
			Currency:   "USD",
		}

		if err := tx.Create(&referral).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create referral"})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete registration"})
		return
	}

	// Generate tokens
	tokens, err := generateTokens(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User registered successfully",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
		},
		"tokens": tokens,
	})
}

// Login handles user authentication
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by email
	var user database.User
	if err := h.db.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Check if 2FA is enabled
	if user.TwoFactorEnabled {
		// Verify TOTP code
		if req.TOTPCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "2FA code required", "require_2fa": true})
			return
		}

		// Verify TOTP code
		valid := utils.ValidateTOTP(user.TwoFactorSecret, req.TOTPCode)
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid 2FA code"})
			return
		}
	}

	// Generate tokens
	tokens, err := generateTokens(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Create session
	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	
	_, err = database.CreateSession(h.db, user.ID, tokens.RefreshToken, userAgent, ipAddress, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"firstName": user.FirstName,
			"lastName":  user.LastName,
			"isAdmin":   user.IsAdmin,
		},
		"tokens": tokens,
	})
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find session by refresh token
	session, err := database.FindSessionByRefreshToken(h.db, req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired refresh token"})
		return
	}

	// Validate the refresh token
	claims, err := utils.ValidateToken(req.RefreshToken)
	if err != nil {
		// Invalidate the session if token is invalid
		_ = database.InvalidateSession(h.db, session.ID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Verify that the token belongs to the session's user
	if claims.UserID != session.UserID {
		_ = database.InvalidateSession(h.db, session.ID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token mismatch"})
		return
	}

	// Get user
	var user database.User
	if err := h.db.First(&user, "id = ?", session.UserID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Generate tokens
	tokens, err := generateTokens(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Update the session with the new refresh token
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if err := database.UpdateSession(h.db, session.ID, tokens.RefreshToken, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Token refreshed successfully",
		"tokens":  tokens,
	})
}

// PasswordResetToken represents a password reset token in the database
type PasswordResetToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid" json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ForgotPassword initiates the password reset process
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if user exists
	var user database.User
	if result := h.db.Where("email = ?", req.Email).First(&user); result.RowsAffected == 0 {
		// Don't reveal that the email doesn't exist for security reasons
		c.JSON(http.StatusOK, gin.H{"message": "If your email is registered, you will receive a password reset link"})
		return
	}

	// Generate password reset token
	token := utils.GenerateSecureToken(32)
	// Token expires in 24 hours
	expires := time.Now().Add(24 * time.Hour).Unix()

	// Save token to database
	resetToken := database.PasswordResetToken{
		ID:        uuid.New().String(),
		UserID:    user.ID.String(),
		Token:     token,
		ExpiresAt: expires,
		CreatedAt: time.Now().Unix(),
	}

	if err := h.db.Create(&resetToken).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	// Send password reset email with token
	err := h.emailService.SendPasswordResetEmail(user.Email, user.Username, token)
	if err != nil {
		// Log the error but don't reveal it to the user
		c.JSON(http.StatusOK, gin.H{"message": "If your email is registered, you will receive a password reset link"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "If your email is registered, you will receive a password reset link",
	})
}

// ResetPassword handles password reset
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find token in database
	var token PasswordResetToken
	if result := h.db.Where("token = ?", req.Token).First(&token); result.RowsAffected == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired reset token"})
		return
	}
	
	// Check if token is expired
	if time.Now().After(token.ExpiresAt) {
		// Delete expired token
		h.db.Delete(&token)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reset token has expired"})
		return
	}
	
	// Find user
	var user database.User
	if err := h.db.First(&user, "id = ?", token.UserID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password reset"})
		return
	}
	
	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
		return
	}
	
	// Update user's password
	user.Password = string(hashedPassword)
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}
	
	// Delete used token
	h.db.Delete(&token)
	
	c.JSON(http.StatusOK, gin.H{"message": "Password has been reset successfully"})
}

// ResendVerificationEmail resends a verification email with enhanced retry mechanism
func (h *AuthHandler) ResendVerificationEmail(c *gin.Context) {
	// Get token from query params or request body
	token := c.Query("token")
	if token == "" {
		// Try to get from JSON body
		var req struct {
			Token string `json:"token"`
			Email string `json:"email"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			token = req.Token
			
			// If no token but email provided, try to find the most recent token for this email
			if token == "" && req.Email != "" {
				var user database.User
				if result := h.db.Where("email = ?", req.Email).First(&user); result.RowsAffected > 0 {
					// Find most recent pending token
					tokens, err := database.GetPendingVerificationTokensByUser(h.db, user.ID)
					if err == nil && len(tokens) > 0 {
						// Use the most recent token (they're sorted by creation date)
						token = tokens[0].Token
					}
				}
			}
		}
	}
	
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing token or email"})
		return
	}

	// Find token in database
	verificationToken, err := database.GetEmailVerificationToken(h.db, token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user
	var user database.User
	if result := h.db.First(&user, verificationToken.UserID); result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Check if user is already verified
	if user.IsVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Check rate limiting for verification attempts
	exceeded, err := database.CheckVerificationRateLimit(h.db, verificationToken.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check verification rate limit"})
		return
	}

	if exceeded {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many verification attempts. Please try again later.",
			"retry_after": 3600, // 1 hour in seconds
			"retry_after_minutes": 60,
			"status": "rate_limited",
		})
		return
	}

	// Increment attempt count
	if err := database.IncrementVerificationAttempt(h.db, verificationToken.ID); err != nil {
		log.Printf("Failed to increment verification attempt: %v", err)
		// Continue anyway - don't fail the request just for this
	}

	// Get frontend URL for retry link
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://revaspay.com"
	}
	retryURL := fmt.Sprintf("%s/auth/resend-verification?token=%s", frontendURL, verificationToken.Token)

	// Send verification email with token
	err = h.emailService.SendVerificationEmail(user.Email, user.Username, token)
	if err != nil {
		log.Printf("Failed to resend verification email to %s: %v", user.Email, err)
		
		c.JSON(http.StatusOK, gin.H{
			"message": "Verification email processing",
			"status": "pending",
			"retry_url": retryURL,
			"retry_after": 60, // Suggest retry after 1 minute
			"attempt": verificationToken.AttemptCount,
			"expires_at": verificationToken.ExpiresAt,
		})
		return
	}

	// Log the verification email resend
	log.Printf("Verification email resent to %s (user ID: %s, attempt: %d, token ID: %s)",
		user.Email, user.ID.String(), verificationToken.AttemptCount, verificationToken.ID.String())

	c.JSON(http.StatusOK, gin.H{
		"message": "Verification email resent successfully",
		"status": "sent",
		"retry_url": retryURL,
		"attempt": verificationToken.AttemptCount,
		"expires_at": verificationToken.ExpiresAt,
	})
}

// SendVerificationEmail sends a verification email to the user
func (h *AuthHandler) SendVerificationEmail(c *gin.Context) {
	// Get user ID from JWT token
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	uid, err := uuid.Parse(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Find user
	var user database.User
	if result := h.db.First(&user, uid); result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Check if user is already verified
	if user.IsVerified {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email already verified"})
		return
	}

	// Check rate limit for verification attempts
	exceeded, err := database.CheckVerificationRateLimit(h.db, uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check verification rate limit"})
		return
	}

	if exceeded {
		// Return detailed rate limit information
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "Too many verification attempts. Please try again later.",
			"retry_after": 3600, // 1 hour in seconds
			"retry_after_minutes": 60,
			"status": "rate_limited",
		})
		return
	}

	// Generate verification token
	token := utils.GenerateSecureToken(32)
	// Token expires in 48 hours
	expiresAt := time.Now().Add(48 * time.Hour)

	// Save token to database using the enhanced function
	verificationToken, err := database.CreateEmailVerificationToken(h.db, uid, token, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
		return
	}

	// Get frontend URL for retry link
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://revaspay.com"
	}
	retryURL := fmt.Sprintf("%s/auth/resend-verification?token=%s", frontendURL, verificationToken.Token)

	// Send verification email with token
	err = h.emailService.SendVerificationEmail(user.Email, user.Username, token)
	if err != nil {
		// Log the error but don't fail the request
		// This allows the frontend to implement retry logic
		log.Printf("Failed to send verification email to %s: %v", user.Email, err)
		
		c.JSON(http.StatusOK, gin.H{
			"message": "Verification email processing",
			"status": "pending",
			"retry_url": retryURL,
			"retry_after": 60, // Suggest retry after 1 minute
			"dev_token": verificationToken.Token, // Remove in production
		})
		return
	}

	// For development, return the token in the response
	c.JSON(http.StatusOK, gin.H{
		"message": "Verification email sent successfully",
		"status": "sent",
		"retry_url": retryURL, // Include retry URL even on success for frontend convenience
		"dev_token": verificationToken.Token, // Remove in production
	})
}

// VerifyEmail handles email verification
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	// Get token from query params
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing token"})
		return
	}

	// Find token in database using enhanced function
	verificationToken, err := database.GetEmailVerificationToken(h.db, token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user
	var user database.User
	if result := h.db.First(&user, verificationToken.UserID); result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// Update user's verified status
	user.IsVerified = true
	now := time.Now()
	user.EmailVerifiedAt = &now
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify email"})
		return
	}

	// Mark verification as complete
	if err := database.CompleteVerification(h.db, verificationToken.ID); err != nil {
		// Log error but don't fail the request since user is already verified
		fmt.Printf("Failed to mark verification as complete: %v\n", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email verified successfully",
	})
}

// GoogleUserInfo holds the user information returned from Google
type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
}

// GoogleAuthRequest represents the request body for Google authentication
type GoogleAuthRequest struct {
	Code         string `json:"code" binding:"required"`
	RedirectURI  string `json:"redirect_uri" binding:"required"`
	ClientID     string `json:"client_id"` // Optional, can use env var
	ClientSecret string `json:"client_secret"` // Optional, can use env var
}

// GoogleAuth handles Google OAuth authentication
func (h *AuthHandler) GoogleAuth(c *gin.Context) {
	var req GoogleAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get client ID and secret from request or environment
	clientID := req.ClientID
	if clientID == "" {
		clientID = os.Getenv("GOOGLE_CLIENT_ID")
		if clientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Google client ID not provided"})
			return
		}
	}

	clientSecret := req.ClientSecret
	if clientSecret == "" {
		clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
		if clientSecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Google client secret not provided"})
			return
		}
	}

	// Configure OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  req.RedirectURI,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	// Exchange authorization code for token
	token, err := oauth2Config.Exchange(context.Background(), req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Failed to exchange token: %v", err)})
		return
	}

	// Get user info from Google
	userInfo, err := getUserInfoFromGoogle(token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get user info: %v", err)})
		return
	}

	if !userInfo.VerifiedEmail {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email not verified with Google"})
		return
	}

	// Check if user exists
	var user database.User
	result := h.db.Where("email = ?", userInfo.Email).First(&user)

	// Start transaction
	tx := h.db.Begin()

	if result.RowsAffected == 0 {
		// User doesn't exist, create new user
		username := utils.GenerateUsername(userInfo.Email)

		// Generate a random password for OAuth users
		password := utils.GenerateSecureToken(16)
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
			return
		}

		// Generate referral code
		referralCode := utils.GenerateReferralCode(8)

		// Create user
		user = database.User{
			Username:      username,
			Email:         userInfo.Email,
			Password:      string(hashedPassword),
			FirstName:     userInfo.GivenName,
			LastName:      userInfo.FamilyName,
			ProfilePicURL: userInfo.Picture,
			ReferralCode:  referralCode,
			IsVerified:    true, // Google already verified the email
		}

		if err := tx.Create(&user).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user"})
			return
		}

		// Create wallet for user
		wallet := database.Wallet{
			UserID:   user.ID,
			Balance:  0,
			Currency: "USD",
		}

		if err := tx.Create(&wallet).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create wallet"})
			return
		}
	} else {
		// Update user's profile picture if it has changed
		if user.ProfilePicURL != userInfo.Picture {
			user.ProfilePicURL = userInfo.Picture
			if err := tx.Save(&user).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
				return
			}
		}
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete authentication"})
		return
	}

	// Generate tokens
	tokens, err := generateTokens(user.ID, user.Email, user.IsAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate tokens"})
		return
	}

	// Create a session record
	userAgent := c.GetHeader("User-Agent")
	ipAddress := c.ClientIP()
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	
	_, err = database.CreateSession(h.db, user.ID, tokens.RefreshToken, userAgent, ipAddress, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Authentication successful",
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"firstName": user.FirstName,
			"lastName":  user.LastName,
			"isAdmin":   user.IsAdmin,
		},
		"tokens": tokens,
	})
}

// getUserInfoFromGoogle gets the user info from Google using the access token
func getUserInfoFromGoogle(accessToken string) (*GoogleUserInfo, error) {
	url := "https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to get user info from Google")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var userInfo GoogleUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

// generateTokens creates access and refresh tokens
func generateTokens(userID uuid.UUID, email string, isAdmin bool) (utils.TokenPair, error) {
	// Directly use the utils.GenerateTokenPair function
	return utils.GenerateTokenPair(userID, email, isAdmin)
}
