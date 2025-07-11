package handlers

import (
	"errors"
	"fmt"

	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/services/kyc"
)

// KYCStatus represents the status of a KYC verification
type KYCStatus string

const (
	KYCStatusNotSubmitted KYCStatus = "not_submitted"
	KYCStatusPending      KYCStatus = "pending"
	KYCStatusApproved     KYCStatus = "approved"
	KYCStatusRejected     KYCStatus = "rejected"
)

// KYCSubmission represents a KYC verification submission
type KYCSubmission struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	IDDocumentPath   string    `json:"id_document_path"`
	SelfiePath       string    `json:"selfie_path"`
	AddressProofPath string    `json:"address_proof_path,omitempty"`
	Status           KYCStatus `json:"status"`
	SubmittedAt      time.Time `json:"submitted_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Notes            string    `json:"notes,omitempty"`
}

// KYCHandler handles KYC verification related requests
type KYCHandler struct {
	DB             *gorm.DB
	SmileIDService *kyc.SmileIdentityService
	UploadsDir     string
}

// NewKYCHandler creates a new KYC handler
func NewKYCHandler(db *gorm.DB) *KYCHandler {
	// Ensure uploads directory exists
	uploadsDir := filepath.Join("uploads", "kyc")
	os.MkdirAll(uploadsDir, 0755)

	return &KYCHandler{
		DB:             db,
		SmileIDService: kyc.NewSmileIdentityService(),
		UploadsDir:     uploadsDir,
	}
}

// GetKYCStatus returns the KYC status for a user
func (h *KYCHandler) GetKYCStatus(c *gin.Context) {
	// Get the user ID from the JWT token
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

	// Fetch the KYC status from the database
	var kyc database.KYC
	result := h.DB.Where("user_id = ?", userID).First(&kyc)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// No KYC record found, create a new one with status "not_submitted"
			kyc = database.KYC{
				UserID: userID,
				Status: string(database.KYCStatusNotSubmitted),
			}
			h.DB.Create(&kyc)
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch KYC status"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":               kyc.ID,
		"user_id":          kyc.UserID,
		"status":           kyc.Status,
		"id_type":          kyc.IDType,
		"id_number":        kyc.IDNumber,
		"id_front_url":     kyc.IDFrontURL,
		"id_back_url":      kyc.IDBackURL,
		"selfie_url":       kyc.SelfieURL,
		"submitted_at":     kyc.CreatedAt,
		"updated_at":       kyc.UpdatedAt,
		"verified_at":      kyc.VerifiedAt,
		"rejection_reason": kyc.RejectionReason,
	})
}

// SubmitKYC handles KYC document submission
func (h *KYCHandler) SubmitKYC(c *gin.Context) {
	// Get the user ID from the JWT token
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

	// Check if user already has a KYC submission
	var existingKYC database.KYC
	result := h.DB.Where("user_id = ?", userID).First(&existingKYC)
	if result.Error == nil && existingKYC.Status != string(database.KYCStatusRejected) && existingKYC.Status != string(database.KYCStatusNotSubmitted) {
		c.JSON(http.StatusConflict, gin.H{"error": "KYC verification already in progress or approved"})
		return
	}

	// Parse form data
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data"})
		return
	}

	// Get form fields
	fullName := c.PostForm("full_name")
	if fullName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Full name is required"})
		return
	}

	dateOfBirth := c.PostForm("date_of_birth")
	if dateOfBirth == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Date of birth is required"})
		return
	}

	address := c.PostForm("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Address is required"})
		return
	}

	country := c.PostForm("country")
	if country == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Country is required"})
		return
	}

	documentType := c.PostForm("document_type")
	if documentType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document type is required"})
		return
	}

	// Validate document type
	var docType database.DocumentType
	switch documentType {
	case "passport":
		docType = database.DocumentTypePassport
	case "id_card":
		docType = database.DocumentTypeIDCard
	case "drivers_license":
		docType = database.DocumentTypeDriversLicense
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid document type"})
		return
	}

	// Create uploads directory if it doesn't exist
	uploadsDir := filepath.Join(h.UploadsDir, userID.String())
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Process ID document front
	idDocumentFront, err := c.FormFile("id_document_front")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID document front is required"})
		return
	}
	idDocumentFrontPath := filepath.Join(uploadsDir, fmt.Sprintf("id_front_%s%s", uuid.New().String(), filepath.Ext(idDocumentFront.Filename)))
	if err := c.SaveUploadedFile(idDocumentFront, idDocumentFrontPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save ID document front"})
		return
	}

	// Process ID document back (optional for passport)
	var idDocumentBackPath string
	if docType != database.DocumentTypePassport {
		idDocumentBack, err := c.FormFile("id_document_back")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ID document back is required for ID card and driver's license"})
			return
		}
		idDocumentBackPath = filepath.Join(uploadsDir, fmt.Sprintf("id_back_%s%s", uuid.New().String(), filepath.Ext(idDocumentBack.Filename)))
		if err := c.SaveUploadedFile(idDocumentBack, idDocumentBackPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save ID document back"})
			return
		}
	}

	// Process selfie
	selfie, err := c.FormFile("selfie")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Selfie is required"})
		return
	}
	selfiePath := filepath.Join(uploadsDir, fmt.Sprintf("selfie_%s%s", uuid.New().String(), filepath.Ext(selfie.Filename)))
	if err := c.SaveUploadedFile(selfie, selfiePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save selfie"})
		return
	}

	// Process address proof (optional)
	var addressProofPath string
	addressProof, err := c.FormFile("address_proof")
	if err == nil {
		addressProofPath = filepath.Join(uploadsDir, fmt.Sprintf("address_proof_%s%s", uuid.New().String(), filepath.Ext(addressProof.Filename)))
		if err := c.SaveUploadedFile(addressProof, addressProofPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save address proof"})
			return
		}
	}

	// Create or update KYC record
	kycID := uuid.New()
	if result.Error == nil {
		kycID = existingKYC.ID
	}

	// ID number is already captured in the form data and used in the KYC struct

	// Create KYC record in database
	kyc := database.KYC{
		ID:              kycID,
		UserID:          userID,
		Status:          string(database.KYCStatusPending),
		IDType:          string(docType),
		IDNumber:        c.PostForm("id_number"),
		IDFrontURL:      idDocumentFrontPath,
		IDBackURL:       idDocumentBackPath,
		SelfieURL:       selfiePath,
	}

	// Save to database
	if result.Error == nil {
		// Update existing record
		h.DB.Model(&kyc).Updates(kyc)
	} else {
		// Create new record
		h.DB.Create(&kyc)
	}

	// Submit to Smile Identity for verification (in a production environment, this would be done asynchronously)
	go func() {
		// This would be handled by a background job in production
		// In a real implementation, we would call the Smile Identity service
		// For now, we'll just log it
		fmt.Printf("Would submit KYC verification for user %s at %s\n", userID, time.Now().Format(time.RFC3339))
		
		// Note: The SmileIdentity service integration would need to be updated
		// to match the actual KYC model fields
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "KYC documents submitted successfully",
		"kyc_id":  kyc.ID,
		"status":  kyc.Status,
	})
}

// UpdateKYCStatus updates the status of a KYC submission (admin only)
func (h *KYCHandler) UpdateKYCStatus(c *gin.Context) {
	// Check if the user is an admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get admin user ID for audit trail
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

	// Check if data was passed from ApproveKYC or RejectKYC methods
	var request struct {
		KYCID           string             `json:"kyc_id" binding:"required"`
		Status          database.KYCStatus `json:"status" binding:"required"`
		RejectionReason string             `json:"rejection_reason"`
		Notes           string             `json:"notes"`
	}

	// Check if data was passed from ApproveKYC or RejectKYC methods
	kycUpdateData, exists := c.Get("_kycUpdateData")
	if exists {
		// Data was passed from ApproveKYC or RejectKYC
		data, ok := kycUpdateData.(gin.H)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid KYC update data format"})
			return
		}

		// Extract data from the context
		request.KYCID = data["kyc_id"].(string)
		request.Status = data["status"].(database.KYCStatus)
		request.Notes = data["notes"].(string)

		// Extract rejection reason if present
		if rejectionReason, exists := data["rejection_reason"]; exists {
			request.RejectionReason = rejectionReason.(string)
		}
	} else {
		// Parse request body for direct API calls
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Validate status
	if request.Status != database.KYCStatusApproved && request.Status != database.KYCStatusRejected && request.Status != database.KYCStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	// Parse KYC ID
	kycID, err := uuid.Parse(request.KYCID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid KYC ID"})
		return
	}

	// Get KYC record from database
	var kyc database.KYC
	result := h.DB.First(&kyc, kycID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch KYC record"})
		}
		return
	}

	// Check if KYC is in a state that can be updated
	if kyc.Status == string(database.KYCStatusNotSubmitted) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot update KYC that has not been submitted"})
		return
	}

	// Get previous status for history
	previousStatus := kyc.Status

	// Set verification timestamp
	now := time.Now()

	// Update KYC record
	updates := database.KYC{
		Status:     string(request.Status),
		VerifiedAt: &now,
	}

	// Add rejection reason if status is rejected
	if request.Status == database.KYCStatusRejected {
		if request.RejectionReason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Rejection reason is required when rejecting KYC"})
			return
		}
		updates.RejectionReason = request.RejectionReason
	}

	// Update the KYC record
	h.DB.Model(&kyc).Updates(updates)

	// Create KYC history record for audit trail
	kycHistory := database.KYCHistory{
		KYCID:          kycID,
		PreviousStatus: database.KYCStatus(previousStatus),
		NewStatus:      request.Status,
		Comment:        request.Notes,
		ChangedBy:      adminID,
		CreatedAt:      now,
	}

	h.DB.Create(&kycHistory)

	// If status is approved, trigger any post-approval processes
	if request.Status == database.KYCStatusApproved {
		// In a real application, we might want to notify the user
		// or trigger other processes like wallet activation
		go h.handleKYCApproval(kyc)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "KYC status updated successfully",
		"kyc_id":      kyc.ID,
		"status":      request.Status,
		"verified_by": adminID,
		"verified_at": now,
	})
}

// handleKYCApproval handles any post-approval processes
func (h *KYCHandler) handleKYCApproval(kyc database.KYC) {
	// In a real application, this would:
	// 1. Send notification to the user
	// 2. Activate the user's wallet or other features
	// 3. Update user's verification level
	// 4. Log the approval in an audit system

	// For now, we'll just log it
	fmt.Printf("KYC approved for user %s at %s\n", kyc.UserID, time.Now().Format(time.RFC3339))
}

// HandleSmileIdentityWebhook processes callbacks from Smile Identity
func (h *KYCHandler) HandleSmileIdentityWebhook(c *gin.Context) {
	// Parse webhook payload
	var webhook kyc.SmileIdentityWebhookPayload
	if err := c.ShouldBindJSON(&webhook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook payload"})
		return
	}

	// Verify webhook signature (in a real application)
	// This would validate that the webhook is actually from Smile Identity
	// using a shared secret or digital signature

	// Find the KYC record by user ID (since we don't have smile_job_id field)
	// In a real implementation, you would need a way to map webhook callbacks to KYC records
	// For now, we'll just use a placeholder query that will likely not find anything
	var kycRecord database.KYC
	result := h.DB.Where("id = ?", uuid.New()).First(&kycRecord)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found for job ID"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch KYC record"})
		}
		return
	}

	// Get previous status for history
	previousStatus := kycRecord.Status

	// Update KYC record with webhook data
	// Note: Since the KYC model doesn't have Smile Identity fields, 
	// we'll just update the status if needed
	updates := database.KYC{}

	// Update status based on verification result
	if webhook.JobSuccess && webhook.ResultCode == "1000" {
		// Smile Identity verification passed
		// In a production system, you might want to keep it as pending for manual review
		// or automatically approve based on confidence scores and matching flags

		// For this implementation, we'll keep it as pending for manual review
		// updates.Status = string(database.KYCStatusApproved)
	} else if !webhook.JobSuccess || webhook.ResultCode != "1000" {
		// Smile Identity verification failed
		// We'll keep it as pending but update the failure reason
		// Since we don't have a Notes field, we'll just log it
		fmt.Printf("Smile Identity verification failed: %s\n", webhook.ResultText)
	}

	// Update the KYC record
	h.DB.Model(&kycRecord).Updates(updates)

	// Create KYC history record for audit trail if status changed
	if kycRecord.Status != previousStatus {
		kycHistory := database.KYCHistory{
			KYCID:          kycRecord.ID,
			PreviousStatus: database.KYCStatus(previousStatus),
			NewStatus:      database.KYCStatus(kycRecord.Status),
			Comment:        fmt.Sprintf("Status updated by Smile Identity webhook: %s", webhook.ResultText),
			ChangedBy:      uuid.Nil, // System change
			CreatedAt:      time.Now(),
		}

		h.DB.Create(&kycHistory)
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"message": "Webhook processed successfully",
		"job_id":  webhook.JobID,
	})
}

// GetPendingKYC returns all pending KYC submissions for admin review
func (h *KYCHandler) GetPendingKYC(c *gin.Context) {
	// Check if the user is an admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get pagination parameters
	page := 1
	pageSize := 10

	// Parse query parameters if provided
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")

	// Convert to integers
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
		pageSize = ps
	}

	// Calculate offset
	offset := (page - 1) * pageSize

	// Get pending KYC submissions
	var kycSubmissions []database.KYC
	result := h.DB.Where("status = ?", string(database.KYCStatusPending)).Order("created_at desc").Offset(offset).Limit(pageSize).Find(&kycSubmissions)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch pending KYC submissions"})
		return
	}

	// Get total count for pagination
	var count int64
	h.DB.Model(&database.KYC{}).Where("status = ?", string(database.KYCStatusPending)).Count(&count)

	// Prepare response
	response := gin.H{
		"kyc_submissions": kycSubmissions,
		"pagination": gin.H{
			"total":       count,
			"page":        page,
			"page_size":   pageSize,
			"total_pages": int(math.Ceil(float64(count) / float64(pageSize))),
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetKYCByID returns a specific KYC submission by ID for admin review
func (h *KYCHandler) GetKYCByID(c *gin.Context) {
	// Check if the user is an admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get KYC ID from URL parameter
	kycIDStr := c.Param("id")
	if kycIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "KYC ID is required"})
		return
	}

	// Parse KYC ID
	kycID, err := uuid.Parse(kycIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid KYC ID"})
		return
	}

	// Get KYC record from database
	var kyc database.KYC
	result := h.DB.First(&kyc, kycID)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "KYC record not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch KYC record"})
		}
		return
	}

	// Get verification history
	var history []database.KYCHistory
	h.DB.Where("kyc_id = ?", kycID).Order("created_at desc").Find(&history)

	// Get user details
	var user database.User
	h.DB.First(&user, kyc.UserID)

	// Prepare response
	response := gin.H{
		"kyc": kyc,
		"user": gin.H{
			"id":       user.ID,
			"email":    user.Email,
			"username": user.Username,
		},
		"history": history,
	}

	c.JSON(http.StatusOK, response)
}

// ApproveKYC approves a KYC submission
func (h *KYCHandler) ApproveKYC(c *gin.Context) {
	// Check if the user is an admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get admin user ID for audit trail
	adminIDStr := c.GetString("user_id")
	if adminIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin ID not found"})
		return
	}

	// Get KYC ID from URL parameter
	kycIDStr := c.Param("id")
	if kycIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "KYC ID is required"})
		return
	}

	// Parse request body
	var request struct {
		Notes string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create a modified context with the approval data
	c.Request.URL.Path = "/kyc/admin/status"
	c.Set("_kycUpdateData", gin.H{
		"kyc_id": kycIDStr,
		"status": database.KYCStatusApproved,
		"notes":  request.Notes,
	})

	// Use the UpdateKYCStatus method to handle the approval
	h.UpdateKYCStatus(c)
}

// RejectKYC rejects a KYC submission
func (h *KYCHandler) RejectKYC(c *gin.Context) {
	// Check if the user is an admin
	isAdmin := c.GetBool("is_admin")
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	// Get admin user ID for audit trail
	adminIDStr := c.GetString("user_id")
	if adminIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Admin ID not found"})
		return
	}

	// Get KYC ID from URL parameter
	kycIDStr := c.Param("id")
	if kycIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "KYC ID is required"})
		return
	}

	// Parse request body
	var request struct {
		RejectionReason string `json:"rejection_reason" binding:"required"`
		Notes           string `json:"notes"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create a modified context with the rejection data
	c.Request.URL.Path = "/kyc/admin/status"
	c.Set("_kycUpdateData", gin.H{
		"kyc_id":           kycIDStr,
		"status":           database.KYCStatusRejected,
		"rejection_reason": request.RejectionReason,
		"notes":            request.Notes,
	})

	// Use the UpdateKYCStatus method to handle the rejection
	h.UpdateKYCStatus(c)
}

// RegisterKYCRoutes registers the KYC routes
func RegisterKYCRoutes(router *gin.RouterGroup, db *gorm.DB) {
	handler := NewKYCHandler(db)

	kycRoutes := router.Group("/kyc")
	{
		kycRoutes.GET("/status", handler.GetKYCStatus)
		kycRoutes.POST("/submit", handler.SubmitKYC)

		// Admin routes
		adminRoutes := kycRoutes.Group("/admin")
		{
			adminRoutes.GET("/pending", handler.GetPendingKYC)
			adminRoutes.GET("/:id", handler.GetKYCByID)
			adminRoutes.PUT("/:id/approve", handler.ApproveKYC)
			adminRoutes.PUT("/:id/reject", handler.RejectKYC)
			adminRoutes.PUT("/status", handler.UpdateKYCStatus)
		}
	}
}
