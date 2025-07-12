package handlers

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/services/payment"
)

// PaymentHandler handles payment-related requests
type PaymentHandler struct {
	paymentService *payment.PaymentService
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(paymentService *payment.PaymentService) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
	}
}

// CreatePaymentLinkRequest represents a request to create a payment link
type CreatePaymentLinkRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Description string                 `json:"description"`
	Amount      float64                `json:"amount" binding:"required,gt=0"`
	Currency    models.Currency        `json:"currency" binding:"required"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// CreatePaymentLink creates a new payment link
func (h *PaymentHandler) CreatePaymentLink(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Parse request
	var req CreatePaymentLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create payment link
	paymentLink, err := h.paymentService.CreatePaymentLink(
		user.ID,
		req.Title,
		req.Description,
		req.Amount,
		req.Currency,
		req.Metadata,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment link
	c.JSON(http.StatusCreated, gin.H{
		"status":       "success",
		"payment_link": paymentLink,
		"payment_url":  "https://revaspay.com/pay/" + paymentLink.Slug,
	})
}

// GetPaymentLinks gets all payment links for the authenticated user
func (h *PaymentHandler) GetPaymentLinks(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get payment links
	paymentLinks, err := h.paymentService.GetUserPaymentLinks(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment links
	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"payment_links": paymentLinks,
	})
}

// GetPaymentLink gets a payment link by ID
func (h *PaymentHandler) GetPaymentLink(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get payment link ID
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment link ID"})
		return
	}

	// Get payment link
	paymentLink, err := h.paymentService.GetPaymentLink(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment link not found"})
		return
	}

	// Check if user owns the payment link
	if paymentLink.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// Return payment link
	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"payment_link": paymentLink,
		"payment_url":  "https://revaspay.com/pay/" + paymentLink.Slug,
	})
}

// UpdatePaymentLinkRequest represents a request to update a payment link
type UpdatePaymentLinkRequest struct {
	Title       *string                 `json:"title"`
	Description *string                 `json:"description"`
	Amount      *float64                `json:"amount"`
	Currency    *models.Currency        `json:"currency"`
	Active      *bool                   `json:"active"`
	Metadata    *map[string]interface{} `json:"metadata"`
}

// UpdatePaymentLink updates a payment link
func (h *PaymentHandler) UpdatePaymentLink(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get payment link ID
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment link ID"})
		return
	}

	// Parse request
	var req UpdatePaymentLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Prepare updates
	updates := make(map[string]interface{})
	if req.Title != nil {
		updates["title"] = *req.Title
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Amount != nil {
		if *req.Amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be greater than 0"})
			return
		}
		updates["amount"] = *req.Amount
	}
	if req.Currency != nil {
		updates["currency"] = *req.Currency
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}
	if req.Metadata != nil {
		// Convert map to interface{} for storage
		updates["metadata"] = *req.Metadata
	}

	// Update payment link
	paymentLink, err := h.paymentService.UpdatePaymentLink(id, user.ID, updates)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment link
	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"payment_link": paymentLink,
		"payment_url":  "https://revaspay.com/pay/" + paymentLink.Slug,
	})
}

// DeletePaymentLink deletes a payment link
func (h *PaymentHandler) DeletePaymentLink(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get payment link ID
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment link ID"})
		return
	}

	// Delete payment link
	if err := h.paymentService.DeletePaymentLink(id, user.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// InitiatePaymentRequest represents a request to initiate a payment
type InitiatePaymentRequest struct {
	Provider      models.PaymentProvider `json:"provider" binding:"required"`
	Amount        float64                `json:"amount" binding:"required,gt=0"`
	Currency      models.Currency        `json:"currency" binding:"required"`
	Description   string                 `json:"description"`
	CustomerEmail string                 `json:"customer_email" binding:"required,email"`
	CustomerName  string                 `json:"customer_name" binding:"required"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// InitiatePayment initiates a payment
func (h *PaymentHandler) InitiatePayment(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Parse request
	var req InitiatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Adjust arguments to match service method signature
	payment, checkoutURL, err := h.paymentService.InitiatePayment(
		user.ID,
		req.Provider,
		req.Amount,
		req.Currency,
		req.Description,
		req.CustomerEmail,
		req.Metadata,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment
	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"payment":      payment,
		"checkout_url": checkoutURL,
	})
}

// InitiatePaymentFromLinkRequest represents a request to initiate a payment from a link
type InitiatePaymentFromLinkRequest struct {
	Provider      models.PaymentProvider `json:"provider" binding:"required"`
	CustomerEmail string                 `json:"customer_email" binding:"required,email"`
	CustomerName  string                 `json:"customer_name" binding:"required"`
}

// InitiatePaymentFromLink initiates a payment from a payment link
func (h *PaymentHandler) InitiatePaymentFromLink(c *gin.Context) {
	// Get payment link slug
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment link slug"})
		return
	}

	// Parse request
	var req InitiatePaymentFromLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// First, get the payment link by slug to get its ID
	paymentLink, err := h.paymentService.GetPaymentLinkBySlug(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment link not found"})
		return
	}

	// Now use the UUID from the payment link
	payment, checkoutURL, err := h.paymentService.InitiatePaymentFromLink(
		paymentLink.ID,
		req.Provider,
		req.CustomerEmail,
		req.CustomerName,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment
	c.JSON(http.StatusOK, gin.H{
		"status":       "success",
		"payment":      payment,
		"checkout_url": checkoutURL,
	})
}

// VerifyPayment verifies a payment
func (h *PaymentHandler) VerifyPayment(c *gin.Context) {
	// Get payment reference
	reference := c.Param("reference")
	if reference == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment reference"})
		return
	}

	// Verify payment
	payment, err := h.paymentService.VerifyPayment(reference)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"payment": payment,
	})
}

// GetPayments gets all payments for the authenticated user
func (h *PaymentHandler) GetPayments(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get pagination parameters
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "10")

	// Parse pagination parameters
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// Get payments
	payments, total, err := h.paymentService.GetUserPayments(user.ID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payments with proper type conversion for pagination calculation
	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"payments": payments,
		"meta": gin.H{
			"page":       page,
			"page_size":  pageSize,
			"total":      total,
			"total_page": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	})
}

// GetPayment gets a payment by ID
func (h *PaymentHandler) GetPayment(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Get payment ID
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payment ID"})
		return
	}

	// Get payment
	payment, err := h.paymentService.GetPayment(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "payment not found"})
		return
	}

	// Check if user owns the payment
	if payment.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// Return payment
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"payment": payment,
	})
}

// InitiateCryptoPaymentRequest represents a request to initiate a crypto payment
type InitiateCryptoPaymentRequest struct {
	Amount         float64                `json:"amount" binding:"required,gt=0"`
	Currency       models.Currency        `json:"currency" binding:"required"`
	Network        string                 `json:"network" binding:"required"`
	CryptoCurrency string                 `json:"crypto_currency" binding:"required"`
	Metadata       map[string]interface{} `json:"metadata"`
}

// InitiateCryptoPayment initiates a cryptocurrency payment
func (h *PaymentHandler) InitiateCryptoPayment(c *gin.Context) {
	// Get authenticated user from context
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid user in context"})
		return
	}

	// Parse request
	var req InitiateCryptoPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Initiate crypto payment
	payment, cryptoPayment, err := h.paymentService.InitiateCryptoPayment(
		user.ID,
		req.Amount,
		req.Currency,
		req.Network,
		req.CryptoCurrency,
		req.Metadata,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return payment
	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"payment":        payment,
		"crypto_payment": cryptoPayment,
	})
}

// ProcessPaystackWebhook processes a webhook from Paystack
func (h *PaymentHandler) ProcessPaystackWebhook(c *gin.Context) {
	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Process webhook
	webhook, err := h.paymentService.ProcessWebhook(models.PaymentProviderPaystack, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"webhook": webhook,
	})
}

// ProcessStripeWebhook processes a webhook from Stripe
func (h *PaymentHandler) ProcessStripeWebhook(c *gin.Context) {
	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Process webhook
	webhook, err := h.paymentService.ProcessWebhook(models.PaymentProviderStripe, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"webhook": webhook,
	})
}

// ProcessPayPalWebhook processes a webhook from PayPal
func (h *PaymentHandler) ProcessPayPalWebhook(c *gin.Context) {
	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Process webhook
	webhook, err := h.paymentService.ProcessWebhook(models.PaymentProviderPayPal, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"webhook": webhook,
	})
}

// ProcessCryptoWebhook processes a webhook from crypto provider
func (h *PaymentHandler) ProcessCryptoWebhook(c *gin.Context) {
	// Read request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Process webhook
	webhook, err := h.paymentService.ProcessWebhook(models.PaymentProviderCrypto, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"webhook": webhook,
	})
}
