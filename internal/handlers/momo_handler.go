package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/api"
	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/services/payment/momo"
	"gorm.io/gorm"
)

// MoMoHandler handles MTN Mobile Money API endpoints
type MoMoHandler struct {
	handler *api.MoMoHandler
}

// NewMoMoHandler creates a new MoMo handler
func NewMoMoHandler(db *gorm.DB, cfg *config.Config) *MoMoHandler {
	// Initialize the MoMo service
	momoService := momo.InitMoMoService(db, cfg)
	
	// Create the API handler
	apiHandler := api.NewMoMoHandler(momoService)
	
	return &MoMoHandler{
		handler: apiHandler,
	}
}

// RequestPayment handles payment collection requests
func (h *MoMoHandler) RequestPayment(c *gin.Context) {
	h.handler.RequestPayment(c)
}

// CheckPaymentStatus handles payment status check requests
func (h *MoMoHandler) CheckPaymentStatus(c *gin.Context) {
	h.handler.CheckPaymentStatus(c)
}

// DisbursePayment handles fund disbursement requests
func (h *MoMoHandler) DisbursePayment(c *gin.Context) {
	h.handler.DisbursePayment(c)
}

// CheckDisbursementStatus handles disbursement status check requests
func (h *MoMoHandler) CheckDisbursementStatus(c *gin.Context) {
	h.handler.CheckDisbursementStatus(c)
}
