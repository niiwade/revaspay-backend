package handlers

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/security/audit"
	"github.com/revaspay/backend/internal/utils"
	"gorm.io/gorm"
)

// ProfileHandler handles user profile management
type ProfileHandler struct {
	db          *gorm.DB
	auditLogger *audit.Logger
	uploadDir   string
}

// ProfileUpdateRequest represents a request to update a user profile
type ProfileUpdateRequest struct {
	FirstName    *string `json:"first_name"`
	LastName     *string `json:"last_name"`
	DisplayName  *string `json:"display_name"`
	Bio          *string `json:"bio"`
	PhoneNumber  *string `json:"phone_number"`
	CountryCode  *string `json:"country_code"`
	BusinessName *string `json:"business_name"`
	Website      *string `json:"website"`
	SocialLinks  map[string]string `json:"social_links"`
}

// NewProfileHandler creates a new profile handler
func NewProfileHandler(db *gorm.DB) *ProfileHandler {
	// Create uploads directory if it doesn't exist
	uploadDir := "./uploads/profiles"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("Failed to create upload directory: %v", err)
	}

	return &ProfileHandler{
		db:          db,
		auditLogger: audit.NewLogger(db),
		uploadDir:   uploadDir,
	}
}

// GetProfile gets the user's profile
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get user profile
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Log profile view
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Profile viewed",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	// Return profile data
	c.JSON(http.StatusOK, gin.H{
		"profile": gin.H{
			"id":            user.ID,
			"email":         user.Email,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"display_name":  user.DisplayName,
			"profile_image": user.ProfileImage,
			"bio":           user.Bio,
			"phone_number":  user.PhoneNumber,
			"country_code":  user.CountryCode,
			"business_name": user.BusinessName,
			"website":       user.Website,
			"social_links":  user.SocialLinks,
			"created_at":    user.CreatedAt,
			"updated_at":    user.UpdatedAt,
			"verified":      user.Verified,
			"is_admin":      user.IsAdmin,
		},
	})
}

// UpdateProfile updates the user's profile
func (h *ProfileHandler) UpdateProfile(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request
	var req ProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Get user profile
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Update fields if provided
	updated := false
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
		updated = true
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
		updated = true
	}
	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
		updated = true
	}
	if req.Bio != nil {
		user.Bio = *req.Bio
		updated = true
	}
	if req.PhoneNumber != nil {
		user.PhoneNumber = *req.PhoneNumber
		updated = true
	}
	if req.CountryCode != nil {
		user.CountryCode = *req.CountryCode
		updated = true
	}
	if req.BusinessName != nil {
		user.BusinessName = *req.BusinessName
		updated = true
	}
	if req.Website != nil {
		user.Website = *req.Website
		updated = true
	}
	if req.SocialLinks != nil && len(req.SocialLinks) > 0 {
		user.SocialLinks = req.SocialLinks
		updated = true
	}

	// Save changes if any field was updated
	if updated {
		if err := h.db.Save(&user).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
			return
		}

		// Log profile update
		h.auditLogger.LogWithContext(
			c,
			audit.EventTypeProfile,
			audit.SeverityInfo,
			"Profile updated",
			userID.(*uuid.UUID),
			nil,
			c.ClientIP(),
			c.Request.UserAgent(),
			true,
			nil,
		)
	}

	// Return updated profile
	c.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"profile": gin.H{
			"id":            user.ID,
			"email":         user.Email,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"display_name":  user.DisplayName,
			"profile_image": user.ProfileImage,
			"bio":           user.Bio,
			"phone_number":  user.PhoneNumber,
			"country_code":  user.CountryCode,
			"business_name": user.BusinessName,
			"website":       user.Website,
			"social_links":  user.SocialLinks,
			"created_at":    user.CreatedAt,
			"updated_at":    user.UpdatedAt,
		},
	})
}

// UploadProfileImage uploads a profile image
func (h *ProfileHandler) UploadProfileImage(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get file from request
	file, header, err := c.Request.FormFile("profile_image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Validate file type
	if !isValidImageType(header.Filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Only JPG, PNG, and GIF are allowed"})
		return
	}

	// Generate unique filename
	filename := generateUniqueFilename(userID.(uuid.UUID), filepath.Ext(header.Filename))
	filepath := filepath.Join(h.uploadDir, filename)

	// Create file
	out, err := os.Create(filepath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}
	defer out.Close()

	// Copy file content
	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Update user profile
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Delete old profile image if exists
	if user.ProfileImage != "" && user.ProfileImage != filename {
		// Use path/filepath package explicitly to avoid any shadowing issues
		oldFilepath := path.Join(h.uploadDir, user.ProfileImage)
		if err := os.Remove(oldFilepath); err != nil {
			log.Printf("Failed to delete old profile image: %v", err)
		}
	}

	// Update profile image
	user.ProfileImage = filename
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	// Log profile image upload
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Profile image uploaded",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		map[string]interface{}{
			"filename": filename,
			"filesize": header.Size,
		},
	)

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"message": "Profile image uploaded successfully",
		"profile_image": filename,
		"profile_image_url": fmt.Sprintf("/uploads/profiles/%s", filename),
	})
}

// DeleteProfileImage deletes a profile image
func (h *ProfileHandler) DeleteProfileImage(c *gin.Context) {
	// Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get user profile
	var user database.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find user"})
		return
	}

	// Check if user has a profile image
	if user.ProfileImage == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No profile image to delete"})
		return
	}

	// Delete profile image
	filepath := filepath.Join(h.uploadDir, user.ProfileImage)
	if err := os.Remove(filepath); err != nil {
		log.Printf("Failed to delete profile image: %v", err)
	}

	// Update user profile
	user.ProfileImage = ""
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update profile"})
		return
	}

	// Log profile image deletion
	h.auditLogger.LogWithContext(
		c,
		audit.EventTypeProfile,
		audit.SeverityInfo,
		"Profile image deleted",
		userID.(*uuid.UUID),
		nil,
		c.ClientIP(),
		c.Request.UserAgent(),
		true,
		nil,
	)

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"message": "Profile image deleted successfully",
	})
}

// Helper functions

// isValidImageType checks if the file is a valid image type
func isValidImageType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif"
}

// generateUniqueFilename generates a unique filename for a profile image
func generateUniqueFilename(userID uuid.UUID, ext string) string {
	randomStr, err := utils.GenerateRandomString(8)
	if err != nil {
		// Fallback to timestamp if random generation fails
		randomStr = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s%s", userID.String(), randomStr, ext)
}

// validateFile validates a file upload
func validateFile(file multipart.File, header *multipart.FileHeader, maxSize int64) error {
	// Check file size
	if header.Size > maxSize {
		return fmt.Errorf("file too large (max %d bytes)", maxSize)
	}

	// Check file type
	if !isValidImageType(header.Filename) {
		return fmt.Errorf("invalid file type")
	}

	return nil
}
