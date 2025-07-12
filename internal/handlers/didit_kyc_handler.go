package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/services/kyc"
	"gorm.io/gorm"
)

// DiditKYCHandler handles KYC verification using Didit
type DiditKYCHandler struct {
	db           *gorm.DB
	diditService *kyc.DiditService
	uploadsDir   string
}

// NewDiditKYCHandler creates a new Didit KYC handler
func NewDiditKYCHandler(db *gorm.DB) (*DiditKYCHandler, error) {
	// Create Didit service
	diditService, err := kyc.NewDiditService(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create Didit service: %w", err)
	}

	// Ensure uploads directory exists
	uploadsDir := filepath.Join("uploads", "kyc")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create uploads directory: %w", err)
	}

	return &DiditKYCHandler{
		db:           db,
		diditService: diditService,
		uploadsDir:   uploadsDir,
	}, nil
}

// InitiateKYCVerification creates a new KYC verification session
func (h *DiditKYCHandler) InitiateKYCVerification(c *gin.Context) {
	// Get user ID from the JWT token
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Check if user already has a pending or approved verification
	var existingVerifications []models.KYCVerification
	if err := h.db.Where("user_id = ? AND status IN ?", userID, []models.KYCStatus{models.KYCStatusPending, models.KYCStatusInProgress, models.KYCStatusApproved}).Find(&existingVerifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing verifications"})
		return
	}

	if len(existingVerifications) > 0 {
		// User already has a pending or approved verification
		c.JSON(http.StatusConflict, gin.H{
			"error": "You already have a KYC verification in progress or approved",
			"verification": gin.H{
				"id":               existingVerifications[0].ID,
				"status":           existingVerifications[0].Status,
				"verification_url": existingVerifications[0].VerificationURL,
			},
		})
		return
	}

	// Create a new verification session with Didit
	verification, err := h.diditService.CreateVerificationSession(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create verification session: %v", err)})
		return
	}

	// Return the verification details
	c.JSON(http.StatusOK, gin.H{
		"message":        "KYC verification session created successfully",
		"verification_id": verification.ID,
		"status":         verification.Status,
		"verification_url": verification.VerificationURL,
	})
}

// GetKYCStatus returns the KYC status for a user
func (h *DiditKYCHandler) GetKYCStatus(c *gin.Context) {
	// Get user ID from the JWT token
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get the latest verification for the user
	var verification models.KYCVerification
	result := h.db.Where("user_id = ?", userID).Order("created_at DESC").First(&verification)
	
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// No verification found
			c.JSON(http.StatusOK, gin.H{
				"status": "not_submitted",
				"message": "No KYC verification has been submitted",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve KYC status"})
		return
	}

	// Return the verification status
	response := gin.H{
		"verification_id":   verification.ID,
		"status":           verification.Status,
		"created_at":       verification.CreatedAt,
		"updated_at":       verification.UpdatedAt,
	}

	// Add additional information based on status
	if verification.Status == models.KYCStatusApproved {
		response["full_name"] = verification.FullName
		response["id_doc_type"] = verification.IDDocType
		response["id_doc_country"] = verification.IDDocCountry
		if verification.IDDocExpiry != nil {
			response["id_doc_expiry"] = verification.IDDocExpiry
		}
	} else if verification.Status == models.KYCStatusRejected {
		response["rejection_reason"] = verification.RejectionReason
	}

	// Add verification URL if available and status is pending or in progress
	if verification.VerificationURL != "" && (verification.Status == models.KYCStatusPending || verification.Status == models.KYCStatusInProgress) {
		response["verification_url"] = verification.VerificationURL
	}

	c.JSON(http.StatusOK, response)
}

// UploadDocument handles document upload for KYC verification
func (h *DiditKYCHandler) UploadDocument(c *gin.Context) {
	// Get user ID from the JWT token
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get verification ID from the request
	verificationIDStr := c.Param("id")
	if verificationIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification ID is required"})
		return
	}

	verificationID, err := uuid.Parse(verificationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification ID"})
		return
	}

	// Verify that the verification belongs to the user
	var verification models.KYCVerification
	if err := h.db.First(&verification, "id = ? AND user_id = ?", verificationID, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Verification not found or does not belong to you"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve verification"})
		}
		return
	}

	// Parse the multipart form
	if err := c.Request.ParseMultipartForm(10 << 20); // 10 MB max
		err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form"})
		return
	}

	// Get document type from form
	docTypeStr := c.PostForm("document_type")
	if docTypeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type is required"})
		return
	}

	// Validate document type
	var docType models.DocumentType
	switch docTypeStr {
	case "id":
		docType = models.DocumentTypeID
	case "passport":
		docType = models.DocumentTypePassport
	case "license":
		docType = models.DocumentTypeLicense
	case "selfie":
		docType = models.DocumentTypeSelfie
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document type"})
		return
	}

	// Get the file from the form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer file.Close()

	// Generate a unique filename
	filename := fmt.Sprintf("%s_%s_%s%s", userID, docTypeStr, time.Now().Format("20060102150405"), filepath.Ext(header.Filename))
	filePath := filepath.Join(h.uploadsDir, filename)

	// Create the file
	out, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create file"})
		return
	}
	defer out.Close()

	// Copy the file data
	_, err = io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Upload the document to the verification
	document, err := h.diditService.UploadDocument(verificationID, docType, filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to upload document: %v", err)})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"message":      "Document uploaded successfully",
		"document_id":  document.ID,
		"document_type": document.Type,
		"file_name":    document.FileName,
	})
}

// HandleDiditWebhook processes webhook notifications from Didit
func (h *DiditKYCHandler) HandleDiditWebhook(c *gin.Context) {
	// Get the webhook signature from the header
	signature := c.GetHeader("X-Didit-Signature")
	if signature == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing webhook signature"})
		return
	}

	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Process the webhook
	if err := h.diditService.ProcessWebhook(body, signature); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to process webhook: %v", err)})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{"message": "Webhook processed successfully"})
}

// GetUserVerifications returns all verifications for a user
func (h *DiditKYCHandler) GetUserVerifications(c *gin.Context) {
	// Get user ID from the JWT token
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get all verifications for the user
	verifications, err := h.diditService.GetUserVerifications(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve verifications: %v", err)})
		return
	}

	// Return the verifications
	c.JSON(http.StatusOK, gin.H{
		"verifications": verifications,
	})
}

// GetPendingVerifications returns all pending verifications for admin review
func (h *DiditKYCHandler) GetPendingVerifications(c *gin.Context) {
	// Check if user is admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get page and limit from query parameters
	page, _ := c.GetQuery("page")
	limit, _ := c.GetQuery("limit")

	pageNum := 1
	limitNum := 10

	if page != "" {
		if val, err := parseInt(page); err == nil && val > 0 {
			pageNum = val
		}
	}

	if limit != "" {
		if val, err := parseInt(limit); err == nil && val > 0 {
			limitNum = val
		}
	}

	offset := (pageNum - 1) * limitNum

	// Get pending verifications
	var verifications []models.KYCVerification
	var total int64

	h.db.Model(&models.KYCVerification{}).Where("status IN ?", []models.KYCStatus{models.KYCStatusPending, models.KYCStatusInProgress}).Count(&total)
	
	if err := h.db.Preload("User").Where("status IN ?", []models.KYCStatus{models.KYCStatusPending, models.KYCStatusInProgress}).
		Order("created_at DESC").Offset(offset).Limit(limitNum).Find(&verifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to retrieve pending verifications: %v", err)})
		return
	}

	// Return the verifications
	c.JSON(http.StatusOK, gin.H{
		"verifications": verifications,
		"pagination": gin.H{
			"total":  total,
			"page":   pageNum,
			"limit":  limitNum,
			"pages":  (total + int64(limitNum) - 1) / int64(limitNum),
		},
	})
}

// GetVerificationByID returns a specific verification by ID
func (h *DiditKYCHandler) GetVerificationByID(c *gin.Context) {
	// Check if user is admin or the verification owner
	isAdmin := c.GetBool("is_admin")
	userIDStr := c.GetString("user_id")
	
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Get verification ID from the request
	verificationIDStr := c.Param("id")
	if verificationIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Verification ID is required"})
		return
	}

	verificationID, err := uuid.Parse(verificationIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification ID"})
		return
	}

	// Get the verification
	var verification models.KYCVerification
	if err := h.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Verification not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve verification"})
		}
		return
	}

	// Check if the user is authorized to view this verification
	if !isAdmin && verification.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to view this verification"})
		return
	}

	// Get verification history
	var history []models.KYCVerificationHistory
	if err := h.db.Where("verification_id = ?", verificationID).Order("created_at DESC").Find(&history).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve verification history"})
		return
	}

	// Get verification documents
	var documents []models.KYCDocument
	if err := h.db.Where("verification_id = ?", verificationID).Find(&documents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve verification documents"})
		return
	}

	// Get user details
	var user models.User
	if err := h.db.First(&user, verification.UserID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user details"})
		return
	}

	// Return the verification details
	c.JSON(http.StatusOK, gin.H{
		"verification": verification,
		"user": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"username": user.Username,
			"name":     user.FirstName + " " + user.LastName,
		},
		"history":   history,
		"documents": documents,
	})
}

// UpdateVerificationStatus updates the status of a verification (admin only)
func (h *DiditKYCHandler) UpdateVerificationStatus(c *gin.Context) {
	// Check if user is admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get admin ID for audit
	adminIDStr := c.GetString("user_id")
	if adminIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin ID not found"})
		return
	}

	adminID, err := uuid.Parse(adminIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid admin ID"})
		return
	}

	// Parse request body
	var request struct {
		VerificationID  string           `json:"verification_id" binding:"required"`
		Status          models.KYCStatus `json:"status" binding:"required"`
		Notes           string           `json:"notes"`
		RejectionReason string           `json:"rejection_reason"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request: %v", err)})
		return
	}

	// Validate status
	if request.Status != models.KYCStatusApproved && request.Status != models.KYCStatusRejected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Status must be 'approved' or 'rejected'"})
		return
	}

	// If status is rejected, rejection reason is required
	if request.Status == models.KYCStatusRejected && request.RejectionReason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required"})
		return
	}

	// Parse verification ID
	verificationID, err := uuid.Parse(request.VerificationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid verification ID"})
		return
	}

	// Get the verification
	var verification models.KYCVerification
	if err := h.db.First(&verification, "id = ?", verificationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Verification not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve verification"})
		}
		return
	}

	// Check if the verification is already in the requested status
	if verification.Status == request.Status {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Verification is already %s", request.Status)})
		return
	}

	// Start a transaction
	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start transaction"})
		return
	}

	// Record previous status for history
	previousStatus := verification.Status

	// Update verification status
	verification.Status = request.Status
	if request.Status == models.KYCStatusRejected {
		verification.RejectionReason = &request.RejectionReason
	}
	if request.Notes != "" {
		verification.AdminNotes = &request.Notes
	}

	// Save the updated verification
	if err := tx.Save(&verification).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update verification status"})
		return
	}

	// Create history record
	history := models.KYCVerificationHistory{
		VerificationID: verificationID,
		PreviousStatus: previousStatus,
		NewStatus:      request.Status,
		ChangedBy:      adminID,
		CreatedAt:      time.Now(),
	}

	if request.Notes != "" {
		history.Notes = &request.Notes
	}

	if err := tx.Create(&history).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create history record"})
		return
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to commit transaction"})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"message":      "Verification status updated successfully",
		"verification": verification,
	})
}

// Helper function to parse integer values
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

// RegisterDiditKYCRoutes registers the Didit KYC routes
func RegisterDiditKYCRoutes(router *gin.RouterGroup, db *gorm.DB) error {
	handler, err := NewDiditKYCHandler(db)
	if err != nil {
		return fmt.Errorf("failed to create Didit KYC handler: %w", err)
	}

	kycRoutes := router.Group("/kyc")
	{
		kycRoutes.GET("/status", handler.GetKYCStatus)
		kycRoutes.POST("/initiate", handler.InitiateKYCVerification)
		kycRoutes.POST("/:id/upload", handler.UploadDocument)
		kycRoutes.GET("/verifications", handler.GetUserVerifications)
		
		// Webhook endpoint
		kycRoutes.POST("/webhook", handler.HandleDiditWebhook)
		
		// Admin routes
		adminRoutes := kycRoutes.Group("/admin")
		{
			adminRoutes.GET("/pending", handler.GetPendingVerifications)
			adminRoutes.GET("/:id", handler.GetVerificationByID)
			adminRoutes.PUT("/status", handler.UpdateVerificationStatus)
		}
	}

	return nil
}
