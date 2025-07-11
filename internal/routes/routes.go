package routes

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/handlers"
	"github.com/revaspay/backend/internal/middleware"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/services/crypto"
	"github.com/revaspay/backend/internal/utils"
)

// RegisterAuthRoutes registers authentication routes
func RegisterAuthRoutes(router *gin.Engine, authHandler *handlers.AuthHandler, sessionHandler *handlers.SessionHandler, enhancedSessionHandler *handlers.EnhancedSessionHandler, mfaHandler *handlers.MFAHandler, passwordHandler *handlers.PasswordHandler, recoveryHandler *handlers.RecoveryHandler, sessionSecurityHandler *handlers.SessionSecurityHandler, rateLimiter *middleware.RateLimiter, securityMiddleware *middleware.SecurityMiddleware, csrfConfig middleware.CSRFConfig) {
	// Apply rate limiting to auth routes
	authGroup := router.Group("/api/auth")
	authGroup.Use(rateLimiter.AuthRateLimiterMiddleware())
	{
		authGroup.POST("/signup", authHandler.Signup)
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.RefreshToken)
		authGroup.POST("/forgot-password", authHandler.ForgotPassword)
		authGroup.POST("/reset-password", passwordHandler.ResetPassword)
		authGroup.GET("/verify-email", authHandler.VerifyEmail)
		authGroup.POST("/send-verification", authHandler.SendVerificationEmail)
		authGroup.POST("/google", authHandler.GoogleAuth)
		
		// Account recovery routes
		authGroup.POST("/recovery/initiate", recoveryHandler.InitiateRecovery)
		authGroup.POST("/recovery/verify-questions", recoveryHandler.VerifySecurityQuestions)
		authGroup.GET("/recovery/validate-token", recoveryHandler.ValidateRecoveryToken)
		authGroup.POST("/recovery/complete", recoveryHandler.CompleteRecovery)
	}

	// MFA routes
	mfaGroup := router.Group("/api/mfa")
	mfaGroup.Use(middleware.AuthMiddleware())
	{
		mfaGroup.GET("/status", mfaHandler.GetMFAStatus)
		mfaGroup.POST("/setup-totp", mfaHandler.SetupTOTP)
		mfaGroup.POST("/verify-totp", mfaHandler.VerifyTOTP)
		mfaGroup.POST("/disable", mfaHandler.DisableMFA)
		mfaGroup.POST("/generate-backup-codes", mfaHandler.GenerateBackupCodes)
	}

	// Public MFA verification endpoint (used during login)
	router.POST("/api/mfa/verify", mfaHandler.VerifyMFACode)

	// Basic Session management routes (protected by auth middleware)
	sessionGroup := router.Group("/api/sessions")
	sessionGroup.Use(middleware.AuthMiddleware())
	{
		sessionGroup.GET("/", sessionHandler.GetActiveSessions)
		sessionGroup.POST("/", sessionHandler.CreateSession)
		sessionGroup.DELETE("/:id", sessionHandler.RevokeSession)
		sessionGroup.DELETE("/all", sessionHandler.RevokeAllSessions)
	}
	
	// Enhanced Session management routes with security features
	enhancedSessionGroup := router.Group("/api/security/sessions")
	enhancedSessionGroup.Use(middleware.AuthMiddleware(), securityMiddleware.VerifySessionStatus())
	{
		enhancedSessionGroup.GET("/", enhancedSessionHandler.GetEnhancedSessions)
		enhancedSessionGroup.POST("/", enhancedSessionHandler.CreateEnhancedSession)
		enhancedSessionGroup.DELETE("/:id", enhancedSessionHandler.RevokeEnhancedSession)
		enhancedSessionGroup.DELETE("/others", enhancedSessionHandler.RevokeAllOtherSessions)
		enhancedSessionGroup.PUT("/:id/trust", enhancedSessionHandler.MarkDeviceAsTrusted)
		
		// Session security endpoints
		enhancedSessionGroup.GET("/risk", sessionSecurityHandler.EvaluateSessionRisk)
		enhancedSessionGroup.POST("/verify", sessionSecurityHandler.VerifySessionSecurity)
		enhancedSessionGroup.POST("/revoke-risky", sessionSecurityHandler.RevokeRiskySessions)
		enhancedSessionGroup.GET("/:id/security-history", sessionSecurityHandler.GetSessionSecurityHistory)
	}
	
	// Admin security endpoints
	adminSecurityGroup := router.Group("/api/admin/security")
	adminSecurityGroup.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
	{
		adminSecurityGroup.POST("/force-mfa", enhancedSessionHandler.ForceMFAVerification)
		adminSecurityGroup.POST("/force-password-reset", enhancedSessionHandler.ForcePasswordReset)
		adminSecurityGroup.POST("/suspend-sessions", enhancedSessionHandler.SuspendSuspiciousSessions)
	}
}

// placeholderHandler is a temporary handler that returns a 501 Not Implemented response
// It's used to make the code compile while we're still developing these features
func placeholderHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "This feature is not yet implemented",
	})
}

// RegisterRoutes configures all API routes
func RegisterRoutes(router *gin.Engine, db *gorm.DB, jobQueue *queue.Queue) {
	// Initialize security middleware
	
	// Setup rate limiter - 60 requests per minute per IP, 5 auth attempts per minute
	rateLimiter := middleware.NewRateLimiter(60, 10, 5, 3)
	
	// Setup security middleware for risk-based authentication
	securityMiddleware := middleware.NewSecurityMiddleware(db)
	
	// Setup secure headers
	secureHeadersConfig := middleware.DefaultSecureHeadersConfig()
	secureHeadersConfig.HSTSMaxAge = 31536000 * time.Second // 1 year
	secureHeadersConfig.HSTSIncludeSubdomains = true
	secureHeadersConfig.HSTSPreload = true
	
	// Setup CSRF protection
	csrfSecret := os.Getenv("CSRF_SECRET")
	if csrfSecret == "" {
		csrfSecret = "change-me-in-production" // Default for development
	}
	csrfConfig := middleware.DefaultCSRFConfig()
	csrfConfig.Secret = csrfSecret
	csrfConfig.ExcludePaths = []string{"/api/webhooks/*", "/health"}
	
	// Initialize audit logger
	auditLogger := utils.NewAuditLogger(db)
	
	// Initialize session security handler
	sessionSecurityHandler := handlers.NewSessionSecurityHandler(db)
	
	// Apply global middleware
	router.Use(middleware.SecureHeadersMiddleware(secureHeadersConfig))
	router.Use(rateLimiter.IPRateLimiterMiddleware())
	router.Use(securityMiddleware.BruteForceProtection())
	
	// Apply session activity tracking to authenticated routes
	router.Use(securityMiddleware.SessionActivity())
	
	// Apply risk-based authentication to sensitive routes
	router.Use(securityMiddleware.RiskBasedAuthentication())
	
	// Apply session security middleware to detect suspicious sessions
	router.Use(sessionSecurityHandler.SessionSecurityMiddleware())
	
	// Apply CSRF protection to state-changing routes
	// This protects against cross-site request forgery attacks
	router.Use(middleware.CSRFMiddleware(csrfConfig))
	// Load configuration
	_ = config.New() // Load config but we don't need it directly
	
	// Create crypto service
	baseService := crypto.NewBaseService(db)
	
	// Create handlers with database access
	authHandler := handlers.NewAuthHandler(db)
	userHandler := handlers.NewUserHandler(db)
	sessionHandler := handlers.NewSessionHandler(db)
	enhancedSessionHandler := handlers.NewEnhancedSessionHandler(db)
	kycHandler := handlers.NewKYCHandler(db)
	webhookHandler := handlers.NewWebhookHandler(db, baseService, jobQueue)
	mfaHandler := handlers.NewMFAHandler(db, auditLogger)
	profileHandler := handlers.NewProfileHandler(db)
	securityQuestionHandler := handlers.NewSecurityQuestionHandler(db)
	passwordHandler := handlers.NewPasswordHandler(db)
	recoveryHandler := handlers.NewRecoveryHandler(db)
	// sessionSecurityHandler already initialized above
	
	// Configure MFA with default settings
	// MFA is already initialized with default config in the handler constructor
	
	// Placeholder handlers for other features
	// These will be implemented as we build out those features
	// For now, we'll use stub handlers to make the routes compile
	
	// Register authentication routes
	RegisterAuthRoutes(router, authHandler, sessionHandler, enhancedSessionHandler, mfaHandler, passwordHandler, recoveryHandler, sessionSecurityHandler, rateLimiter, securityMiddleware, csrfConfig)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API v1 group
	v1 := router.Group("/api")
	{
		// Auth routes already registered above
		// Public routes
		v1.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})
		v1.GET("/version", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"version": "1.0.0"})
		})
		
		// Public security question verification endpoint (used during account recovery)
		v1.POST("/auth/verify-security-questions", securityQuestionHandler.VerifySecurityQuestions)
		
		// Webhook routes - no authentication but verified by signature
		webhooks := router.Group("/webhooks")
		{
			// Payment provider webhooks
			webhooks.POST("/paystack", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Paystack webhook received"})
			})
			webhooks.POST("/flutterwave", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Flutterwave webhook received"})
			})
			webhooks.POST("/stripe", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Stripe webhook received"})
			})
			
			// KYC verification webhooks
			webhooks.POST("/kyc/smile", kycHandler.HandleSmileIdentityWebhook)
			
			// Blockchain transaction webhooks
			webhooks.POST("/blockchain/transaction", webhookHandler.BlockchainTransactionWebhook)
			
			// Bank transfer webhooks
			webhooks.POST("/bank/transfer", webhookHandler.BankTransferWebhook)
			
			// Exchange rate webhooks
			webhooks.POST("/exchange/rates", webhookHandler.ExchangeRateWebhook)
			
			// MTN MoMo webhooks
			webhooks.POST("/momo/payment", placeholderHandler)
			webhooks.POST("/momo/disbursement", placeholderHandler)
		}

		// Protected routes - require authentication
		protected := v1.Group("/")
		protected.Use(middleware.AuthMiddleware())
		// Apply CSRF protection to all state-changing endpoints
		protected.Use(middleware.CSRFMiddleware(csrfConfig))
		{
			// User routes
			user := protected.Group("/user")
			{
				// Profile management
				user.GET("/profile", profileHandler.GetProfile)
				user.PUT("/profile", profileHandler.UpdateProfile)
				user.POST("/profile/image", profileHandler.UploadProfileImage)
				user.DELETE("/profile/image", profileHandler.DeleteProfileImage)
				
				// Password management
				user.PUT("/password", passwordHandler.UpdatePassword)
				user.POST("/password/evaluate", passwordHandler.EvaluatePasswordStrength)
				
				// 2FA management
				user.POST("/2fa/enable", userHandler.Enable2FA)
				user.POST("/2fa/verify", userHandler.Verify2FA)
				user.POST("/2fa/disable", userHandler.Disable2FA)
				
				// Security questions management
				securityQuestions := user.Group("/security-questions")
				{
					securityQuestions.GET("/", securityQuestionHandler.GetUserSecurityQuestions)
					securityQuestions.GET("/available", securityQuestionHandler.GetSecurityQuestions)
					securityQuestions.POST("/", securityQuestionHandler.SetSecurityQuestionAnswer)
					securityQuestions.DELETE("/:id", securityQuestionHandler.DeleteSecurityQuestionAnswer)
				}
			}
			
			// KYC routes
			kycRoutes := protected.Group("/kyc")
			{
				kycRoutes.GET("/status", kycHandler.GetKYCStatus)
				kycRoutes.POST("/submit", kycHandler.SubmitKYC)
			}
			
			// Wallet routes - will be implemented later
			protected.GET("/wallet", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Wallet endpoint"})
			})
			protected.GET("/wallet/transactions", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Wallet transactions endpoint"})
			})
			protected.PUT("/wallet/settings", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Update wallet settings endpoint"})
			})
			protected.GET("/wallet/balance", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Wallet balance endpoint"})
			})
			
			// Banking routes for Ghanaian bank integration
			banking := protected.Group("/banking")
			{
				banking.POST("/link-account", placeholderHandler)
				banking.GET("/accounts", placeholderHandler)
				banking.GET("/accounts/:id", placeholderHandler)
				banking.PUT("/accounts/:id", placeholderHandler)
				banking.DELETE("/accounts/:id", placeholderHandler)
				banking.GET("/banks", placeholderHandler)
				banking.POST("/verify-account", placeholderHandler)
			}
			
			// Crypto wallet routes for Base blockchain
			crypto := protected.Group("/crypto")
			{
				crypto.POST("/wallets", placeholderHandler)
				crypto.GET("/wallets", placeholderHandler)
				crypto.GET("/wallets/:id", placeholderHandler)
				crypto.GET("/wallets/:id/transactions", placeholderHandler)
				crypto.GET("/transactions/:id", placeholderHandler)
			}
			
			// International payment routes
			intl := protected.Group("/international-payments")
			{
				intl.POST("/", placeholderHandler)
				intl.GET("/", placeholderHandler)
				intl.GET("/:id", placeholderHandler)
				intl.GET("/:id/compliance-report", placeholderHandler)
			}
			
			// Transaction routes
			protected.GET("/transactions", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Transactions endpoint"})
			})
			
			// Payment routes - will be implemented later
			protected.POST("/payment-links", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Create payment link endpoint"})
			})
			protected.GET("/payment-links", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get payment links endpoint"})
			})
			protected.GET("/payment-links/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get payment link endpoint"})
			})
			protected.PUT("/payment-links/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Update payment link endpoint"})
			})
			protected.DELETE("/payment-links/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Delete payment link endpoint"})
			})
			
			// Withdrawal routes - will be implemented later
			protected.POST("/withdraw", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Create withdrawal endpoint"})
			})
			protected.GET("/withdrawals", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get withdrawals endpoint"})
			})
			protected.GET("/withdrawals/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get withdrawal endpoint"})
			})
			
			// MTN MoMo API routes
			momo := protected.Group("/momo")
			{
				// Collection (payment) endpoints
				momo.POST("/request-payment", placeholderHandler)
				momo.POST("/check-payment", placeholderHandler)
				
				// Disbursement endpoints
				momo.POST("/disburse", placeholderHandler)
				momo.POST("/check-disbursement", placeholderHandler)
			}
			
			// Subscription routes - will be implemented later
			protected.POST("/subscription-plans", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Create subscription plan endpoint"})
			})
			protected.GET("/subscription-plans", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get subscription plans endpoint"})
			})
			protected.PUT("/subscription-plans/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Update subscription plan endpoint"})
			})
			protected.DELETE("/subscription-plans/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Delete subscription plan endpoint"})
			})
			protected.GET("/subscriptions", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get subscriptions endpoint"})
			})
			
			// Virtual account routes - will be implemented later
			protected.POST("/virtual-accounts", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Create virtual account endpoint"})
			})
			protected.GET("/virtual-accounts", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get virtual accounts endpoint"})
			})
			protected.DELETE("/virtual-accounts/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Delete virtual account endpoint"})
			})
			
			// Referral routes - will be implemented later
			protected.GET("/referrals", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get referrals endpoint"})
			})
			protected.GET("/referrals/stats", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Get referral stats endpoint"})
			})
		}

		// Admin routes - require admin role
		admin := v1.Group("/admin")
		admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
		{
			// Admin user management
			admin.GET("/users", userHandler.GetAllUsers)
			admin.GET("/users/:id", userHandler.GetUserByID)
			admin.PUT("/users/:id/verify", userHandler.VerifyUser)
			
			// Admin transaction management
			admin.GET("/transactions", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin transactions endpoint"})
			})
			
			// Admin KYC management
			admin.GET("/kyc/pending", kycHandler.GetPendingKYC)
			admin.GET("/kyc/:id", kycHandler.GetKYCByID)
			admin.PUT("/kyc/:id/approve", kycHandler.ApproveKYC)
			admin.PUT("/kyc/:id/reject", kycHandler.RejectKYC)
			admin.PUT("/kyc/status", kycHandler.UpdateKYCStatus)
			
			// Admin wallet management - will be implemented later
			admin.GET("/withdrawals", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin get all withdrawals endpoint"})
			})
			admin.PUT("/withdrawals/:id/process", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin process withdrawal endpoint"})
			})
			
			// Admin international payment management
			admin.GET("/international-payments", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin get all international payments endpoint"})
			})
			admin.GET("/international-payments/:id", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin get international payment details endpoint"})
			})
			admin.GET("/compliance-reports", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin get all compliance reports endpoint"})
			})
			admin.GET("/bank-accounts", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"message": "Admin get all bank accounts endpoint"})
			})
		}
	}
}
