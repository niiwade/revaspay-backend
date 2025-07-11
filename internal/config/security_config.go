package config

import (
	"os"
	"time"
)

// SecurityConfig holds all security-related configuration
type SecurityConfig struct {
	// Rate limiting
	IPRateLimit         int
	IPRateBurst         int
	AuthRateLimit       int
	AuthRateBurst       int
	RateLimitCleanupMin int

	// CSRF protection
	CSRFSecret      string
	CSRFExcludePaths []string

	// Secure headers
	HSTSMaxAge            time.Duration
	HSTSIncludeSubdomains bool
	HSTSPreload           bool
	CSPDirectives         map[string]string
	CORSAllowedOrigins    []string

	// Session security
	SessionMaxAge        int
	SessionRenewAfter    int
	SessionInactivityMax int

	// MFA settings
	MFAIssuer     string
	MFADigits     int
	MFAPeriod     uint
	MFABackupCodes int
}

// DefaultSecurityConfig returns the default security configuration
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		// Rate limiting - 60 requests per minute per IP, 5 auth attempts per minute
		IPRateLimit:         60.0,
		IPRateBurst:         10.0,
		AuthRateLimit:       5.0,
		AuthRateBurst:       3.0,
		RateLimitCleanupMin: 5,

		// CSRF protection
		CSRFSecret:       getEnvOrDefault("CSRF_SECRET", "change-me-in-production"),
		CSRFExcludePaths: []string{"/api/webhooks/*", "/health"},

		// Secure headers
		HSTSMaxAge:            31536000 * time.Second, // 1 year
		HSTSIncludeSubdomains: true,
		HSTSPreload:           true,
		CSPDirectives: map[string]string{
			"default-src": "'self'",
			"script-src":  "'self' 'unsafe-inline'",
			"style-src":   "'self' 'unsafe-inline'",
			"img-src":     "'self' data:",
			"connect-src": "'self'",
			"font-src":    "'self'",
			"object-src":  "'none'",
			"frame-src":   "'self'",
		},
		CORSAllowedOrigins: []string{"*"}, // Should be restricted in production

		// Session security
		SessionMaxAge:        86400 * 7, // 7 days
		SessionRenewAfter:    3600 * 12, // 12 hours
		SessionInactivityMax: 3600,      // 1 hour

		// MFA settings
		MFAIssuer:     "RevasPay",
		MFADigits:     6,
		MFAPeriod:     30,
		MFABackupCodes: 10,
	}
}

// getEnvOrDefault gets an environment variable or returns a default value
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// NOTE: The InitSecurityMiddleware function has been moved to routes.go to avoid import cycles.
// Security middleware is now initialized directly in the routes.go file.
// This is a design decision to avoid circular dependencies between packages.
//
// The security configuration is still defined here, but the actual middleware
// initialization happens in routes.go where all the routes are registered.
//
// This approach allows us to maintain a clean separation of concerns while
// avoiding import cycles.

// GetMFAConfig returns the MFA configuration
// This function is commented out to avoid import cycles
// MFA configuration is now handled directly in the MFA handler
/*
func GetMFAConfig() {
	// MFA configuration is now handled directly in the MFA handler
	config := DefaultSecurityConfig()
	return utils.MFAConfig{
		Issuer:     config.MFAIssuer,
		Digits:     otp.DigitsSix, // Using otp.Digits type
		Period:     config.MFAPeriod,
		BackupCodeCount: config.MFABackupCodes,
	}
}
*/
