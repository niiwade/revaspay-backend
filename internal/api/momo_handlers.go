package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/services/payment/momo"
)

// MoMoHandler handles MTN Mobile Money API endpoints
type MoMoHandler struct {
	momoService *momo.MoMoService
}

// NewMoMoHandler creates a new MoMo handler
func NewMoMoHandler(momoService *momo.MoMoService) *MoMoHandler {
	return &MoMoHandler{
		momoService: momoService,
	}
}

// RequestPaymentRequest represents a request to collect payment via MoMo
type RequestPaymentRequest struct {
	PhoneNumber string  `json:"phoneNumber" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Description string  `json:"description" binding:"required"`
}

// RequestPayment handles payment collection requests
func (h *MoMoHandler) RequestPayment(c *gin.Context) {
	var req RequestPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from authenticated user
	userID := getUserIDFromContext(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Convert string userID to UUID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Request payment
	response, err := h.momoService.RequestPayment(momo.PaymentRequest{
		UserID:      userUUID,
		PhoneNumber: req.PhoneNumber,
		Amount:      req.Amount,
		Description: req.Description,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Payment request initiated",
		"transactionId": response.TransactionID,
		"status":        response.Status,
	})
}

// CheckPaymentStatusRequest represents a request to check payment status
type CheckPaymentStatusRequest struct {
	TransactionID string `json:"transactionId" binding:"required"`
}

// CheckPaymentStatus handles payment status check requests
func (h *MoMoHandler) CheckPaymentStatus(c *gin.Context) {
	var req CheckPaymentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check payment status
	status, err := h.momoService.CheckPaymentStatus(req.TransactionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactionId": req.TransactionID,
		"status":        status,
	})
}

// DisbursePaymentRequest represents a request to disburse funds via MoMo
type DisbursePaymentRequest struct {
	PhoneNumber string  `json:"phoneNumber" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Description string  `json:"description" binding:"required"`
}

// DisbursePayment handles fund disbursement requests
func (h *MoMoHandler) DisbursePayment(c *gin.Context) {
	var req DisbursePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from authenticated user
	userID := getUserIDFromContext(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Convert string userID to UUID
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	// Disburse payment
	response, err := h.momoService.DisbursePayment(momo.DisbursementRequest{
		UserID:      userUUID,
		PhoneNumber: req.PhoneNumber,
		Amount:      req.Amount,
		Description: req.Description,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Disbursement initiated",
		"transactionId": response.TransactionID,
		"status":        response.Status,
	})
}

// CheckDisbursementStatusRequest represents a request to check disbursement status
type CheckDisbursementStatusRequest struct {
	TransactionID string `json:"transactionId" binding:"required"`
}

// CheckDisbursementStatus handles disbursement status check requests
func (h *MoMoHandler) CheckDisbursementStatus(c *gin.Context) {
	var req CheckDisbursementStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check disbursement status
	status, err := h.momoService.CheckDisbursementStatus(req.TransactionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"transactionId": req.TransactionID,
		"status":        status,
	})
}

// Helper function to get user ID from context
// This assumes you have middleware that sets the user ID in the context
func getUserIDFromContext(c *gin.Context) string {
	userID, exists := c.Get("userID")
	if !exists {
		return ""
	}
	return userID.(string)
}
