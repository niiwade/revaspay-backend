package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/handlers"
	"github.com/revaspay/backend/internal/middleware"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/crypto"
	"gorm.io/gorm"
)

// SetupWebhookRoutes configures routes for webhook endpoints
func SetupWebhookRoutes(router *gin.Engine, db *gorm.DB, jobQueue *queue.Queue) {
	// Create services
	baseService := crypto.NewBaseService(db)
	
	// Create webhook handler
	webhookHandler := handlers.NewWebhookHandler(db, baseService, jobQueue)
	
	// Webhook routes group
	webhookGroup := router.Group("/api/v1/webhooks")
	{
		// Public webhook endpoints with API key authentication
		// These endpoints are called by external services
		
		// Blockchain transaction webhooks
		webhookGroup.POST("/blockchain/transaction", webhookHandler.BlockchainTransactionWebhook)
		
		// Bank transfer webhooks
		webhookGroup.POST("/bank/transfer", webhookHandler.BankTransferWebhook)
		
		// Exchange rate webhooks
		webhookGroup.POST("/exchange/rates", webhookHandler.ExchangeRateWebhook)
		
		// Admin webhook endpoints (require authentication)
		adminWebhookGroup := webhookGroup.Group("/admin")
		adminWebhookGroup.Use(middleware.AuthMiddleware())
		adminWebhookGroup.Use(middleware.AdminMiddleware())
		{
			// Admin-only webhook management endpoints could go here
			// For example, to view webhook logs, resend webhooks, etc.
		}
	}
}
