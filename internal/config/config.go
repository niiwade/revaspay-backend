package config

import (
	"os"
	"sync"

	"github.com/joho/godotenv"
	"github.com/revaspay/backend/internal/secrets"
)

// Config holds all configuration for the application
type Config struct {
	DatabaseURL     string
	JWTSecret       string
	FrontendURL     string
	Environment     string
	PaystackKey     string
	FlutterwaveKey  string
	StripeKey       string
	SmileIDKey      string
	// MTN MoMo API credentials
	MoMoSubscriptionKey      string
	MoMoCollectionAPIUser    string
	MoMoCollectionAPIKey     string
	MoMoDisbursementAPIUser  string
	MoMoDisbursementAPIKey   string
	MoMoUseSandbox           bool
	dopplerClient   *secrets.DopplerClient
	dopplerInitOnce sync.Once
}

// New creates a new Config instance with values from environment variables
// It will try to load from .env file first, then from Doppler if available
func New() *Config {
	// Try to load .env file for local development
	_ = godotenv.Load()

	// Create config with environment variables or defaults
	config := &Config{
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/revaspay?sslmode=disable"),
		FrontendURL:    getEnv("FRONTEND_URL", "http://localhost:3000"),
		Environment:    getEnv("ENVIRONMENT", "development"),
		// Initialize Doppler client with project and config from env vars or defaults
		dopplerClient: secrets.NewDopplerClient(
			getEnv("DOPPLER_PROJECT", "revaspay"),
			getEnv("DOPPLER_CONFIG", "dev"),
		),
	}

	// Initialize sensitive values with Doppler if possible, otherwise from env
	config.initSecrets()

	return config
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// initSecrets initializes sensitive configuration values from Doppler
func (c *Config) initSecrets() {
	// Initialize Doppler client only once
	c.dopplerInitOnce.Do(func() {
		// Try to initialize Doppler client
		err := c.dopplerClient.Initialize()
		if err != nil {
			// If Doppler initialization fails, fall back to environment variables
			// This allows the application to run without Doppler in development
			c.JWTSecret = getEnv("JWT_SECRET", "your-secret-key")
			c.PaystackKey = getEnv("PAYSTACK_SECRET_KEY", "")
			c.FlutterwaveKey = getEnv("FLUTTERWAVE_SECRET_KEY", "")
			c.StripeKey = getEnv("STRIPE_SECRET_KEY", "")
			c.SmileIDKey = getEnv("SMILE_ID_KEY", "")
			
			// MTN MoMo API credentials from environment
			c.MoMoSubscriptionKey = getEnv("MTN_MOMO_SUBSCRIPTION_KEY", "")
			c.MoMoCollectionAPIUser = getEnv("MTN_MOMO_COLLECTION_API_USER", "")
			c.MoMoCollectionAPIKey = getEnv("MTN_MOMO_COLLECTION_API_KEY", "")
			c.MoMoDisbursementAPIUser = getEnv("MTN_MOMO_DISBURSEMENT_API_USER", "")
			c.MoMoDisbursementAPIKey = getEnv("MTN_MOMO_DISBURSEMENT_API_KEY", "")
			c.MoMoUseSandbox = getEnv("MTN_MOMO_USE_SANDBOX", "true") == "true"
			return
		}

		// Get secrets from Doppler with fallback to environment variables
		c.JWTSecret = c.dopplerClient.GetSecretWithFallback("JWT_SECRET", getEnv("JWT_SECRET", "your-secret-key"))
		c.PaystackKey = c.dopplerClient.GetSecretWithFallback("PAYSTACK_SECRET_KEY", getEnv("PAYSTACK_SECRET_KEY", ""))
		c.FlutterwaveKey = c.dopplerClient.GetSecretWithFallback("FLUTTERWAVE_SECRET_KEY", getEnv("FLUTTERWAVE_SECRET_KEY", ""))
		c.StripeKey = c.dopplerClient.GetSecretWithFallback("STRIPE_SECRET_KEY", getEnv("STRIPE_SECRET_KEY", ""))
		c.SmileIDKey = c.dopplerClient.GetSecretWithFallback("SMILE_ID_KEY", getEnv("SMILE_ID_KEY", ""))
		
		// MTN MoMo API credentials from Doppler with fallback to environment
		c.MoMoSubscriptionKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_SUBSCRIPTION_KEY", getEnv("MTN_MOMO_SUBSCRIPTION_KEY", ""))
		c.MoMoCollectionAPIUser = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_COLLECTION_API_USER", getEnv("MTN_MOMO_COLLECTION_API_USER", ""))
		c.MoMoCollectionAPIKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_COLLECTION_API_KEY", getEnv("MTN_MOMO_COLLECTION_API_KEY", ""))
		c.MoMoDisbursementAPIUser = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_DISBURSEMENT_API_USER", getEnv("MTN_MOMO_DISBURSEMENT_API_USER", ""))
		c.MoMoDisbursementAPIKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_DISBURSEMENT_API_KEY", getEnv("MTN_MOMO_DISBURSEMENT_API_KEY", ""))
		
		// Parse boolean value
		useSandbox := c.dopplerClient.GetSecretWithFallback("MTN_MOMO_USE_SANDBOX", getEnv("MTN_MOMO_USE_SANDBOX", "true"))
		c.MoMoUseSandbox = useSandbox == "true"
	})
}

// GetSecret retrieves a secret from Doppler or environment
func (c *Config) GetSecret(key, defaultValue string) string {
	// Try to initialize if not already done
	c.initSecrets()

	// Try to get from Doppler
	if c.dopplerClient != nil {
		value := c.dopplerClient.GetSecretWithFallback(key, "")
		if value != "" {
			return value
		}
	}

	// Fall back to environment
	return getEnv(key, defaultValue)
}
