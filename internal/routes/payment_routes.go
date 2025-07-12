package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/revaspay/backend/internal/handlers"
	"github.com/revaspay/backend/internal/middleware"
)

// SetupPaymentRoutes sets up payment routes
func SetupPaymentRoutes(router *gin.Engine, paymentHandler *handlers.PaymentHandler) {
	// API routes (authenticated)
	api := router.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Payment links
		paymentLinks := api.Group("/payment-links")
		{
			paymentLinks.POST("", paymentHandler.CreatePaymentLink)
			paymentLinks.GET("", paymentHandler.GetPaymentLinks)
			paymentLinks.GET("/:id", paymentHandler.GetPaymentLink)
			paymentLinks.PUT("/:id", paymentHandler.UpdatePaymentLink)
			paymentLinks.DELETE("/:id", paymentHandler.DeletePaymentLink)
		}

		// Payments
		payments := api.Group("/payments")
		{
			payments.POST("", paymentHandler.InitiatePayment)
			payments.GET("", paymentHandler.GetPayments)
			payments.GET("/:id", paymentHandler.GetPayment)
			payments.GET("/verify/:reference", paymentHandler.VerifyPayment)
		}

		// Crypto payments
		crypto := api.Group("/crypto")
		{
			crypto.POST("/payments", paymentHandler.InitiateCryptoPayment)
		}
	}

	// Public routes
	public := router.Group("/public")
	{
		// Payment from link
		public.POST("/pay/:slug", paymentHandler.InitiatePaymentFromLink)
		public.GET("/verify/:reference", paymentHandler.VerifyPayment)
	}

	// Webhook routes (no authentication)
	webhooks := router.Group("/webhooks")
	{
		webhooks.POST("/paystack", paymentHandler.ProcessPaystackWebhook)
		webhooks.POST("/stripe", paymentHandler.ProcessStripeWebhook)
		webhooks.POST("/paypal", paymentHandler.ProcessPayPalWebhook)
		webhooks.POST("/crypto", paymentHandler.ProcessCryptoWebhook)
	}
}
