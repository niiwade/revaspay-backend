package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/services/compliance"
	"github.com/revaspay/backend/internal/services/payment"
	"gorm.io/gorm"
)

// InternationalPaymentHandler handles international payment requests
type InternationalPaymentHandler struct {
	db                *gorm.DB
	paymentService    *payment.InternationalPaymentService
	complianceService *compliance.GhanaComplianceService
}

// NewInternationalPaymentHandler creates a new international payment handler
func NewInternationalPaymentHandler(db *gorm.DB) *InternationalPaymentHandler {
	return &InternationalPaymentHandler{
		db:                db,
		paymentService:    payment.NewInternationalPaymentService(db),
		complianceService: compliance.NewGhanaComplianceService(db),
	}
}

// InitiatePayment initiates an international payment
func (h *InternationalPaymentHandler) InitiatePayment(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Parse request
	var req struct {
		VendorName    string  `json:"vendor_name" binding:"required"`
		VendorAddress string  `json:"vendor_address" binding:"required"`
		Amount        float64 `json:"amount" binding:"required,gt=0"`
		Description   string  `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate transaction with compliance service
	passed, checks, err := h.complianceService.ValidateTransaction(userID.(uuid.UUID), req.Amount, "international_payment", req.VendorName, req.VendorAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Compliance check failed: " + err.Error()})
		return
	}

	if !passed {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "failed",
			"message": "Transaction failed compliance checks",
			"checks":  checks,
		})
		return
	}

	// Create payment request
	paymentReq := payment.PaymentRequest{
		VendorName:    req.VendorName,
		VendorAddress: req.VendorAddress,
		Amount:        req.Amount,
		Description:   req.Description,
	}

	// Process payment
	payment, err := h.paymentService.ProcessPayment(userID.(uuid.UUID), paymentReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "International payment initiated successfully",
		"data":    payment,
	})
}

// GetPayments retrieves all international payments for a user
func (h *InternationalPaymentHandler) GetPayments(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get payments
	payments, err := h.paymentService.GetPayments(userID.(uuid.UUID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   payments,
	})
}

// GetPayment retrieves a specific international payment
func (h *InternationalPaymentHandler) GetPayment(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get payment ID from URL
	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment ID"})
		return
	}

	// Get payment
	payment, err := h.paymentService.GetPayment(paymentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if payment belongs to user
	if payment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to view this payment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   payment,
	})
}

// GetComplianceReport generates a compliance report for a payment
func (h *InternationalPaymentHandler) GetComplianceReport(c *gin.Context) {
	// Get user ID from context (set by auth middleware)
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Get payment ID from URL
	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment ID"})
		return
	}

	// Get payment
	payment, err := h.paymentService.GetPayment(paymentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if payment belongs to user
	if payment.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have permission to view this payment"})
		return
	}

	// Generate compliance report
	report, err := h.complianceService.GenerateComplianceReport(userID.(uuid.UUID), paymentID, "international_payment")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate compliance report: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   report,
	})
}
