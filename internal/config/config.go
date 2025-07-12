package config

import (
	"os"
	"strconv"
	"sync"

	"github.com/joho/godotenv"
	"github.com/revaspay/backend/internal/secrets"
)

// Config holds all configuration for the application
type Config struct {
	Database    DatabaseConfig
	Server      ServerConfig
	Redis       RedisConfig
	JWT         JWTConfig
	FrontendURL string
	Environment string
	Paystack    PaystackConfig
	Flutterwave FlutterwaveConfig
	Stripe      StripeConfig
	PayPal      PayPalConfig
	Didit      DiditConfig
	MoMo        MoMoConfig
	
	dopplerClient   *secrets.DopplerClient
	dopplerInitOnce sync.Once
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL      string
	MaxConns int
	MaxIdle  int
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  int
	WriteTimeout int
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret     string
	Expiration int // in hours
}

// PaystackConfig holds Paystack configuration
type PaystackConfig struct {
	SecretKey string
	PublicKey string
}

// FlutterwaveConfig holds Flutterwave configuration
type FlutterwaveConfig struct {
	SecretKey string
	PublicKey string
}

// StripeConfig holds Stripe configuration
type StripeConfig struct {
	SecretKey string
	PublicKey string
	WebhookSecret string
}

// PayPalConfig holds PayPal configuration
type PayPalConfig struct {
	ClientID     string
	ClientSecret string
	Environment  string // sandbox or production
}

// DiditConfig holds Didit KYC verification configuration
type DiditConfig struct {
	APIKey      string
	ClientID    string
	CallbackURL string
	WebhookSecret string
	Environment   string // sandbox or production
}

// MoMoConfig holds MTN Mobile Money API configuration
type MoMoConfig struct {
	SubscriptionKey      string
	CollectionAPIUser    string
	CollectionAPIKey     string
	DisbursementAPIUser  string
	DisbursementAPIKey   string
	UseSandbox           bool
}

// LoadConfig creates a new Config instance with values from environment variables
// It will try to load from .env file first, then from Doppler if available
func LoadConfig() *Config {
	// Try to load .env file for local development
	_ = godotenv.Load()

	// Create config with environment variables or defaults
	config := &Config{
		Database: DatabaseConfig{
			URL:      getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/revaspay?sslmode=disable"),
			MaxConns: getEnvInt("DATABASE_MAX_CONNS", 20),
			MaxIdle:  getEnvInt("DATABASE_MAX_IDLE", 5),
		},
		Server: ServerConfig{
			Port:         getEnv("PORT", "8080"),
			ReadTimeout:  getEnvInt("SERVER_READ_TIMEOUT", 10),
			WriteTimeout: getEnvInt("SERVER_WRITE_TIMEOUT", 10),
		},
		Redis: RedisConfig{
			URL:      getEnv("REDIS_URL", "redis://localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Expiration: getEnvInt("JWT_EXPIRATION", 24),
		},
		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
		Environment: getEnv("ENVIRONMENT", "development"),
		
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
			c.JWT.Secret = getEnv("JWT_SECRET", "your-secret-key")
			
			// Payment provider credentials from environment
			c.Paystack.SecretKey = getEnv("PAYSTACK_SECRET_KEY", "")
			c.Paystack.PublicKey = getEnv("PAYSTACK_PUBLIC_KEY", "")
			
			c.Flutterwave.SecretKey = getEnv("FLUTTERWAVE_SECRET_KEY", "")
			c.Flutterwave.PublicKey = getEnv("FLUTTERWAVE_PUBLIC_KEY", "")
			
			c.Stripe.SecretKey = getEnv("STRIPE_SECRET_KEY", "")
			c.Stripe.PublicKey = getEnv("STRIPE_PUBLIC_KEY", "")
			c.Stripe.WebhookSecret = getEnv("STRIPE_WEBHOOK_SECRET", "")
			
			c.PayPal.ClientID = getEnv("PAYPAL_CLIENT_ID", "")
			c.PayPal.ClientSecret = getEnv("PAYPAL_CLIENT_SECRET", "")
			c.PayPal.Environment = getEnv("PAYPAL_ENVIRONMENT", "sandbox")
			
			c.Didit.APIKey = getEnv("DIDIT_API_KEY", "")
			c.Didit.ClientID = getEnv("DIDIT_CLIENT_ID", "")
			c.Didit.CallbackURL = getEnv("DIDIT_CALLBACK_URL", "")
			c.Didit.WebhookSecret = getEnv("DIDIT_WEBHOOK_SECRET", "")
			c.Didit.Environment = getEnv("DIDIT_ENVIRONMENT", "sandbox")
			
			// MTN MoMo API credentials from environment
			c.MoMo.SubscriptionKey = getEnv("MTN_MOMO_SUBSCRIPTION_KEY", "")
			c.MoMo.CollectionAPIUser = getEnv("MTN_MOMO_COLLECTION_API_USER", "")
			c.MoMo.CollectionAPIKey = getEnv("MTN_MOMO_COLLECTION_API_KEY", "")
			c.MoMo.DisbursementAPIUser = getEnv("MTN_MOMO_DISBURSEMENT_API_USER", "")
			c.MoMo.DisbursementAPIKey = getEnv("MTN_MOMO_DISBURSEMENT_API_KEY", "")
			c.MoMo.UseSandbox = getEnv("MTN_MOMO_USE_SANDBOX", "true") == "true"
			return
		}

		// Get secrets from Doppler with fallback to environment variables
		c.JWT.Secret = c.dopplerClient.GetSecretWithFallback("JWT_SECRET", getEnv("JWT_SECRET", "your-secret-key"))
		
		// Payment provider credentials from Doppler with fallback to environment
		c.Paystack.SecretKey = c.dopplerClient.GetSecretWithFallback("PAYSTACK_SECRET_KEY", getEnv("PAYSTACK_SECRET_KEY", ""))
		c.Paystack.PublicKey = c.dopplerClient.GetSecretWithFallback("PAYSTACK_PUBLIC_KEY", getEnv("PAYSTACK_PUBLIC_KEY", ""))
		
		c.Flutterwave.SecretKey = c.dopplerClient.GetSecretWithFallback("FLUTTERWAVE_SECRET_KEY", getEnv("FLUTTERWAVE_SECRET_KEY", ""))
		c.Flutterwave.PublicKey = c.dopplerClient.GetSecretWithFallback("FLUTTERWAVE_PUBLIC_KEY", getEnv("FLUTTERWAVE_PUBLIC_KEY", ""))
		
		c.Stripe.SecretKey = c.dopplerClient.GetSecretWithFallback("STRIPE_SECRET_KEY", getEnv("STRIPE_SECRET_KEY", ""))
		c.Stripe.PublicKey = c.dopplerClient.GetSecretWithFallback("STRIPE_PUBLIC_KEY", getEnv("STRIPE_PUBLIC_KEY", ""))
		c.Stripe.WebhookSecret = c.dopplerClient.GetSecretWithFallback("STRIPE_WEBHOOK_SECRET", getEnv("STRIPE_WEBHOOK_SECRET", ""))
		
		c.PayPal.ClientID = c.dopplerClient.GetSecretWithFallback("PAYPAL_CLIENT_ID", getEnv("PAYPAL_CLIENT_ID", ""))
		c.PayPal.ClientSecret = c.dopplerClient.GetSecretWithFallback("PAYPAL_CLIENT_SECRET", getEnv("PAYPAL_CLIENT_SECRET", ""))
		c.PayPal.Environment = c.dopplerClient.GetSecretWithFallback("PAYPAL_ENVIRONMENT", getEnv("PAYPAL_ENVIRONMENT", "sandbox"))
		
		c.Didit.APIKey = c.dopplerClient.GetSecretWithFallback("DIDIT_API_KEY", getEnv("DIDIT_API_KEY", ""))
		c.Didit.ClientID = c.dopplerClient.GetSecretWithFallback("DIDIT_CLIENT_ID", getEnv("DIDIT_CLIENT_ID", ""))
		c.Didit.CallbackURL = c.dopplerClient.GetSecretWithFallback("DIDIT_CALLBACK_URL", getEnv("DIDIT_CALLBACK_URL", ""))
		c.Didit.WebhookSecret = c.dopplerClient.GetSecretWithFallback("DIDIT_WEBHOOK_SECRET", getEnv("DIDIT_WEBHOOK_SECRET", ""))
		c.Didit.Environment = c.dopplerClient.GetSecretWithFallback("DIDIT_ENVIRONMENT", getEnv("DIDIT_ENVIRONMENT", "sandbox"))
		
		// MTN MoMo API credentials from Doppler with fallback to environment
		c.MoMo.SubscriptionKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_SUBSCRIPTION_KEY", getEnv("MTN_MOMO_SUBSCRIPTION_KEY", ""))
		c.MoMo.CollectionAPIUser = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_COLLECTION_API_USER", getEnv("MTN_MOMO_COLLECTION_API_USER", ""))
		c.MoMo.CollectionAPIKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_COLLECTION_API_KEY", getEnv("MTN_MOMO_COLLECTION_API_KEY", ""))
		c.MoMo.DisbursementAPIUser = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_DISBURSEMENT_API_USER", getEnv("MTN_MOMO_DISBURSEMENT_API_USER", ""))
		c.MoMo.DisbursementAPIKey = c.dopplerClient.GetSecretWithFallback("MTN_MOMO_DISBURSEMENT_API_KEY", getEnv("MTN_MOMO_DISBURSEMENT_API_KEY", ""))
		
		// Parse boolean value
		useSandbox := c.dopplerClient.GetSecretWithFallback("MTN_MOMO_USE_SANDBOX", getEnv("MTN_MOMO_USE_SANDBOX", "true"))
		c.MoMo.UseSandbox = useSandbox == "true"
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

// getEnvInt retrieves an environment variable as an integer or returns a default value
func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	
	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	
	return intValue
}
