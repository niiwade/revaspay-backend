package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
	"github.com/revaspay/backend/internal/config"
	"github.com/revaspay/backend/internal/database"
	"github.com/revaspay/backend/internal/handlers"
	"github.com/revaspay/backend/internal/jobs"
	"github.com/revaspay/backend/internal/middleware"
	"github.com/revaspay/backend/internal/models"
	"github.com/revaspay/backend/internal/queue"
	"github.com/revaspay/backend/internal/routes"
	"github.com/revaspay/backend/internal/services/kyc"
	"github.com/revaspay/backend/internal/services/payment"
	"github.com/revaspay/backend/internal/services/payment/providers/paystack"
	"github.com/revaspay/backend/internal/services/wallet"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize configuration
	cfg := config.LoadConfig()

	// Initialize database
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.URL, // Use URL from config instead of separate Address and Password
		Password: "",          // Password is included in the URL if needed
		DB:       0,            // Default DB
	})

	// Test Redis connection
	ctx := context.Background()
	if _, err := redisClient.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create Redis-backed queue instance
	redisQueue := queue.NewRedisQueue(redisClient, db)
	
	// Create queue adapter that implements QueueInterface
	queueAdapter := queue.NewQueueAdapter(redisQueue)

	// Initialize services
	walletService := wallet.NewWalletService(db)
	
	// Initialize KYC service
	kycService := kyc.NewKYCService(db, cfg.Didit)
	
	// Initialize payment providers
	paystackProvider := paystack.NewPaystackProvider(paystack.PaystackConfig{
		SecretKey: cfg.Paystack.SecretKey,
		PublicKey: cfg.Paystack.PublicKey,
	})
	
	// Initialize payment service with both DB and wallet service
	paymentService := payment.NewPaymentService(db, walletService)
	
	// Register payment providers
	paymentService.RegisterProvider(models.PaymentProviderPaystack, paystackProvider)
	// Temporarily disabled due to missing implementations
	// paymentService.RegisterProvider(models.PaymentProviderStripe, stripeProvider)
	// paymentService.RegisterProvider(models.PaymentProviderPaypal, paypalProvider)
	
	// Register all job handlers
	jobs.RegisterPaymentWebhookJobHandlers(queueAdapter, db, paymentService, walletService)
	jobs.RegisterRecurringPaymentJobHandlers(queueAdapter, db, paymentService, walletService)
	// Create and register withdrawal job handlers
	withdrawalJob := jobs.NewWithdrawalJob(db, queueAdapter, paymentService, walletService)
	withdrawalJob.RegisterHandlers(queueAdapter)
	jobs.RegisterKYCVerificationJobHandlers(queueAdapter, db, kycService)
	jobs.RegisterVirtualAccountJobHandlers(queueAdapter, db, paymentService, walletService)
	
	// Register referral reward job handlers
	jobs.RegisterReferralRewardJobHandlers(queueAdapter, db, walletService)
	
	// Initialize security middleware
	securityMiddleware := middleware.NewSecurityMiddleware(db)
	
	// Initialize handlers
	paymentHandler := handlers.NewPaymentHandler(paymentService)
	
	// Initialize Gin router
	router := gin.Default()
	
	// Apply global middleware
	router.Use(gin.Logger()) // Use built-in logger instead of custom middleware
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) { // Simple CORS middleware
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})
	router.Use(securityMiddleware.BruteForceProtection())
	router.Use(securityMiddleware.SessionActivity())
	
	// Setup routes
	routes.SetupPaymentRoutes(router, paymentHandler)
	
	// Start background job processor
	jobProcessor := queue.NewJobProcessor(redisQueue, 10) // 10 worker goroutines
	go jobProcessor.Start()
	
	// Schedule recurring jobs
	jobs.ScheduleRecurringJobs(queueAdapter, db, paymentService, walletService)
	
	// Start server
	srv := startServer(router, cfg.Server.Port)
	
	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")
	
	// Stop job processor
	jobProcessor.Stop()
	
	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Shutdown server
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	
	log.Println("Server exiting")
}



// startServer starts the HTTP server
func startServer(router *gin.Engine, port string) *http.Server {
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}
	
	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()
	
	log.Printf("Server started on port %s", port)
	return srv
}
