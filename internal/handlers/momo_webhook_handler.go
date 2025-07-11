package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/api"
	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/services/payment/momo"
	"gorm.io/gorm"
)

// MoMoWebhookHandler handles MTN Mobile Money API webhook callbacks
type MoMoWebhookHandler struct {
	handler *api.MoMoWebhookHandler
}

// NewMoMoWebhookHandler creates a new MoMo webhook handler
func NewMoMoWebhookHandler(db *gorm.DB, cfg *config.Config) *MoMoWebhookHandler {
	// Initialize the MoMo service
	momoService := momo.InitMoMoService(db, cfg)
	
	// Create the API handler
	apiHandler := api.NewMoMoWebhookHandler(momoService)
	
	return &MoMoWebhookHandler{
		handler: apiHandler,
	}
}

// PaymentNotification handles payment notification webhooks
func (h *MoMoWebhookHandler) PaymentNotification(c *gin.Context) {
	h.handler.PaymentNotification(c)
}

// DisbursementNotification handles disbursement notification webhooks
func (h *MoMoWebhookHandler) DisbursementNotification(c *gin.Context) {
	h.handler.DisbursementNotification(c)
}
