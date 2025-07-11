package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/services/payment/momo"
)

// MoMoWebhookHandler handles MTN Mobile Money API webhook callbacks
type MoMoWebhookHandler struct {
	momoService *momo.MoMoService
}

// NewMoMoWebhookHandler creates a new MoMo webhook handler
func NewMoMoWebhookHandler(momoService *momo.MoMoService) *MoMoWebhookHandler {
	return &MoMoWebhookHandler{
		momoService: momoService,
	}
}

// PaymentNotification handles payment notification webhooks from MTN MoMo
func (h *MoMoWebhookHandler) PaymentNotification(c *gin.Context) {
	// Read the request body
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Parse the webhook payload
	var notification momo.PaymentNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Process the payment notification
	err = h.momoService.ProcessPaymentNotification(notification)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// DisbursementNotification handles disbursement notification webhooks from MTN MoMo
func (h *MoMoWebhookHandler) DisbursementNotification(c *gin.Context) {
	// Read the request body
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Parse the webhook payload
	var notification momo.DisbursementNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Process the disbursement notification
	err = h.momoService.ProcessDisbursementNotification(notification)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
